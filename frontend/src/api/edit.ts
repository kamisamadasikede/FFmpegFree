/**
 * EditService 接口层（契约 v0.11 / 6.11，#22 最新提交为准，尚未冻结）。
 *
 * EDIT_BACKEND_READY（api/flags.ts）= false：本地模拟（内存里的工程库 + api/sim.ts 的定时器导出任务）；
 * = true：window.go.app.EditService.*（callService；绑定不存在抛 UNSUPPORTED）。
 * 素材不在 EditService：选文件 SystemService.PickFiles（api/system.ts 的 pickFiles）、探测 MediaService.Probe（api/media.ts 的 probeFiles）、缩略图 thumbnailOf。
 * 取消导出 = TaskService.Cancel（tasks store 的 cancel）；重试 = TaskService.Retry（edit_export 注册了重试工厂）。
 *
 * 契约要点（前端相关）：
 * - Render → Export，edit_render → edit_export。数值 / 枚举越界一律 INVALID_ARGUMENT（不再静默截断）。
 * - 同一轨道上 clip 时间重叠 → INVALID_ARGUMENT，但只在 ValidateProject 和 Export 里报；SaveProject 只校验数量上限，草稿可以带重叠保存（架构师决定 5）。
 *   前端在拖拽 / 放置时就要拦住：见 findTrackOverlap；画中画用不同轨道。
 * - outSec 必须 > inSec，0 不表示“到结尾”，outSec=0 一律 INVALID_ARGUMENT：素材加入 clip 时用探测到的时长填实际值（见 newVideoClip / newAudioClip / fillOutSec）。
 * - clip 级错误的 detail 第一行 `clip=<id> path=<path>`（结构性错误是 `project`）；AppError.clipId / .path 已解析（api/call.ts），clip id 字符集 [A-Za-z0-9_-]。
 * - 预览：GetPreviewURL 返回 /local/<token>（进程内有效，重启失效）。遇到 404（token 失效 / 文件被删）要重新调用 GetPreviewURL：见 createPreviewSource。
 */
import { AppError, callService, toAppError } from '@/api/call'
import { EDIT_BACKEND_READY } from '@/api/flags'
import { createSimTask, injectionDetail, simDelay, simError, simInjection, simParam } from '@/api/sim'
import { toApiTask, type ApiTask } from '@/api/taskTypes'

export { EDIT_BACKEND_READY }

// ───────────── 契约类型（§6.11.1，字段一一对应）─────────────

export type EditFormat = 'mp4' | 'mov' | 'mkv' | 'webm'
export type EffectPreset = 'none' | 'grayscale' | 'sepia' | 'vintage' | 'cinematic'
export type TransitionName = 'none' | 'fade' | 'wipeleft' | 'wiperight' | 'slideleft' | 'slideright' | 'circleopen' | 'circleclose' | 'dissolve'

export interface EditOutput {
  /** mp4 | mov | mkv | webm，空 = mp4 */
  format: EditFormat | ''
  /** 16~7680，0 = 1280；导出时向下取偶数 */
  width: number
  /** 16~4320，0 = 720 */
  height: number
  /** (0,120]，0 = 30 */
  fps: number
}

export interface VideoClip {
  /** 前端生成的唯一串（1~64 字符，字符集 [A-Za-z0-9_-]），错误 detail 用它定位；同一工程内唯一 */
  id: string
  /** 绝对路径 */
  path: string
  /** V1~V8，编号大的盖在上面 */
  trackId: string
  startSec: number
  inSec: number
  /** 必须 > inSec；0 不表示到结尾（一律 INVALID_ARGUMENT），加入 clip 时填探测到的素材时长 */
  outSec: number
  /** 0.25~4，0 = 1 */
  speed: number
  effectPreset: EffectPreset | ''
  transitionToNext: TransitionName | ''
  /** 0 = 0.5；范围 0.1~2，且不超过相邻两个 clip 中较短者的一半 */
  transitionDurationSec: number
  /** 0~4 */
  blur: number
}

export interface AudioClip {
  id: string
  path: string
  /** A1~A8 */
  trackId: string
  startSec: number
  inSec: number
  outSec: number
  /** 0.25~4 */
  speed: number
  /** 0~4，0 = 静音；空值不可区分，前端必须显式传 1 */
  volume: number
}

export interface GlobalEffects {
  /** -0.5~0.5 */
  brightness: number
  /** 0.5~2，0 视为 1 */
  contrast: number
  /** 0~2，0 视为 1 */
  saturation: number
  /** 0~2 */
  sharpen: number
}

export interface EditProject {
  /** 当前 1；大于 1 的工程 LoadProject 返回 UNSUPPORTED */
  schemaVersion: number
  /** 新建时空 */
  id: string
  /** 去首尾空白后 1~80 字 */
  name: string
  /** 素材库：绝对路径，去重，最多 200 个，只是列表，不保证存在 */
  sources: string[]
  output: EditOutput
  videoTrack: VideoClip[]
  audioTrack: AudioClip[]
  effects: GlobalEffects
  /** 只读，Save 时由后端写 */
  updatedAt: number
}

export interface EditExportOptions {
  /** 不含扩展名；空 = 工程名。净化规则见 6.11.3「输出文件名」（允许 CJK，最长 100 字符） */
  outputName: string
  /** 绝对路径；空 = Settings.defaultOutputDir，仍空 = 第一个 clip 所在文件夹 */
  outputDir: string
}

export interface EditPlan {
  durationSec: number
  clipCount: number
  inputs: string[]
  hasAudio: boolean
  warnings: string[]
}

export interface EditProjectMeta {
  id: string
  name: string
  durationSec: number
  clipCount: number
  updatedAt: number
}

export interface LoadedProject {
  project: EditProject
  /** 工程里引用但磁盘上已不存在的素材（sources 与 clip 的并集） */
  missingPaths: string[]
}

export interface PreviewURL {
  /** 形如 /local/<token>，直接给 <video src> / <audio src> */
  url: string
  mime: string
  size: number
}

export const EDIT_SCHEMA_VERSION = 1
export const CLIP_ID_RE = /^[A-Za-z0-9_-]{1,64}$/
export const VIDEO_TRACK_RE = /^V[1-8]$/
export const AUDIO_TRACK_RE = /^A[1-8]$/
/** GetPreviewURL 允许的扩展名（v1 允许列表） */
export const PREVIEW_EXTS = ['mp4', 'mov', 'avi', 'mkv', 'flv', 'webm', 'm4v', 'mp3', 'wav', 'aac', 'm4a', 'flac', 'ogg']
const AUDIO_ONLY_EXTS = ['mp3', 'wav', 'aac', 'm4a', 'flac', 'ogg']

/** 新建一个空工程（视频轨不能为空才能 Export/Validate；Save 只做结构与范围校验） */
export function newEditProject(name = '未命名工程'): EditProject {
  return {
    schemaVersion: EDIT_SCHEMA_VERSION, id: '', name, sources: [],
    output: { format: 'mp4', width: 1280, height: 720, fps: 30 },
    videoTrack: [], audioTrack: [],
    effects: { brightness: 0, contrast: 1, saturation: 1, sharpen: 0 },
    updatedAt: 0,
  }
}

/** 素材时长（探测所得，秒）必须是正的有限数，否则没法给 outSec 填实际值 */
function requireDuration(durationSec: number): number {
  if (!(Number.isFinite(durationSec) && durationSec > 0)) throw new AppError('INVALID_ARGUMENT', '素材时长未知，先探测素材再加入时间线')
  return durationSec
}

/** 素材加入视频轨：outSec 填探测到的时长（不能是 0）。durationSec 未知（≤0）时抛 INVALID_ARGUMENT，调用方应先 MediaService.Probe */
export function newVideoClip(o: { path: string; durationSec: number; trackId?: string; startSec?: number; id?: string }): VideoClip {
  return {
    id: o.id ?? newClipId('v'), path: o.path, trackId: o.trackId ?? 'V1', startSec: o.startSec ?? 0, inSec: 0, outSec: requireDuration(o.durationSec),
    speed: 1, effectPreset: 'none', transitionToNext: 'none', transitionDurationSec: 0, blur: 0,
  }
}

/** 素材加入音轨：同 newVideoClip；volume 显式 1（0 = 静音，空值不可区分） */
export function newAudioClip(o: { path: string; durationSec: number; trackId?: string; startSec?: number; id?: string }): AudioClip {
  return { id: o.id ?? newClipId('a'), path: o.path, trackId: o.trackId ?? 'A1', startSec: o.startSec ?? 0, inSec: 0, outSec: requireDuration(o.durationSec), speed: 1, volume: 1 }
}

/** 旧数据 / 手改后 outSec ≤ inSec（含 0）时，用探测到的时长补成实际值；已合法的不动 */
export function fillOutSec<T extends AnyClip>(clip: T, durationSec: number): T {
  return clip.outSec > clip.inSec ? clip : { ...clip, outSec: Math.max(requireDuration(durationSec), clip.inSec + 0.05) }
}

/** 生成合法的 clip id（[A-Za-z0-9_-]，≤ 64） */
export function newClipId(prefix = 'c'): string {
  return `${prefix}${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
}

// ───────────── 前端校验（拖拽 / 放置时用；模拟层和后端共用同一规则）─────────────

type AnyClip = VideoClip | AudioClip
/** clip 在时间线上占的长度（秒）：(outSec - inSec) / speed。outSec 必须 > inSec（0 不再表示到结尾），不合法的按 0 长度算 */
export function clipTimelineLength(c: AnyClip): number {
  const out = c.outSec
  const speed = c.speed > 0 ? c.speed : 1
  return Math.max(0, (out - c.inSec) / speed)
}

export interface TrackOverlap {
  trackId: string
  a: string
  b: string
}

/**
 * 同一轨道上的 clip 时间重叠检测（后端判 INVALID_ARGUMENT）。首尾相接（end == start，容差 1ms）不算重叠。
 * outSec 已经是实际值，不再需要素材时长。前端在拖拽和放置 clip 时调用它拦住，画中画请放到不同轨道。
 */
export function findTrackOverlap(clips: AnyClip[]): TrackOverlap | null {
  const byTrack = new Map<string, { id: string; start: number; end: number }[]>()
  for (const c of clips) {
    const len = clipTimelineLength(c)
    const list = byTrack.get(c.trackId) ?? []
    list.push({ id: c.id, start: c.startSec, end: c.startSec + len })
    byTrack.set(c.trackId, list)
  }
  for (const [trackId, list] of byTrack) {
    list.sort((x, y) => x.start - y.start)
    for (let i = 1; i < list.length; i++) {
      if (list[i].start < list[i - 1].end - 0.001) return { trackId, a: list[i - 1].id, b: list[i].id }
    }
  }
  return null
}

/** 放置 / 拖动 candidate 后是否会与同轨道其他 clip 重叠（忽略自己）。返回冲突的 clip id，没有返回 null */
export function wouldOverlap(clips: AnyClip[], candidate: AnyClip): string | null {
  const others = clips.filter((c) => c.id !== candidate.id)
  const r = findTrackOverlap([...others, candidate])
  if (!r) return null
  return r.a === candidate.id ? r.b : r.b === candidate.id ? r.a : null
}

// ───────────── 结构校验（ValidateProject / Export 用；SaveProject 只校验数量上限，见 checkSaveLimits）─────────────

const MAX_CLIPS = 100
const MAX_TIMELINE_SEC = 6 * 3600
const MAX_PROJECT_BYTES = 1024 * 1024
const TRANSITIONS: readonly string[] = ['none', 'fade', 'wipeleft', 'wiperight', 'slideleft', 'slideright', 'circleopen', 'circleclose', 'dissolve']
const EFFECTS: readonly string[] = ['none', 'grayscale', 'sepia', 'vintage', 'cinematic']
const FORMATS: readonly string[] = ['mp4', 'mov', 'mkv', 'webm']
const isAbs = (p: string) => /^([A-Za-z]:[\\/]|\/|\\\\)/.test(p)
const extOf = (p: string) => (p.split('.').pop() ?? '').toLowerCase()
const baseName = (p: string) => p.split(/[\\/]/).pop() || p

function bad(head: string, why: string): never {
  // 契约：detail 第一行 `clip=<id> path=<path>`，结构性错误写 `project`，之后是原因
  return simError('INVALID_ARGUMENT', why, `${head}\n${why}`)
}

/** SaveProject 的校验：只查数量上限（素材库 200、clip 总数 100、序列化后 1 MiB），其余（范围、同轨重叠）都不查，草稿可以保存 */
export function checkSaveLimits(p: EditProject): void {
  if (p.sources.length > 200) bad('project', '素材库最多 200 个')
  if (p.videoTrack.length + p.audioTrack.length > MAX_CLIPS) bad('project', `clip 总数最多 ${MAX_CLIPS} 个`)
  if (new TextEncoder().encode(JSON.stringify(p)).length > MAX_PROJECT_BYTES) bad('project', '工程超过 1 MiB')
}

/** 结构与范围校验（不探测素材、不要求文件存在），含同轨重叠。Validate / Export 用；模拟层在此基础上再做素材检查 */
export function checkStructure(p: EditProject, opts: { requireVideo: boolean }): void {
  if (p.schemaVersion !== EDIT_SCHEMA_VERSION && p.schemaVersion !== 0) bad('project', 'schemaVersion 不是 1')
  const name = p.name.trim()
  if (name.length < 1 || [...name].length > 80) bad('project', '工程名需要 1~80 个字')
  if (p.sources.length > 200) bad('project', '素材库最多 200 个')
  const o = p.output
  if (o.format && !FORMATS.includes(o.format)) bad('project', `输出格式只能是 ${FORMATS.join(' / ')}`)
  if (o.width !== 0 && (o.width < 16 || o.width > 7680)) bad('project', '输出宽度需要在 16~7680 之间')
  if (o.height !== 0 && (o.height < 16 || o.height > 4320)) bad('project', '输出高度需要在 16~4320 之间')
  if (o.fps !== 0 && !(o.fps > 0 && o.fps <= 120)) bad('project', '输出帧率需要在 (0,120] 之间')
  if (opts.requireVideo && p.videoTrack.length === 0) bad('project', '视频轨不能为空')
  if (p.videoTrack.length + p.audioTrack.length > MAX_CLIPS) bad('project', `clip 总数最多 ${MAX_CLIPS} 个`)
  const e = p.effects
  if (e.brightness < -0.5 || e.brightness > 0.5) bad('project', '亮度需要在 -0.5~0.5 之间')
  if (e.contrast !== 0 && (e.contrast < 0.5 || e.contrast > 2)) bad('project', '对比度需要在 0.5~2 之间')
  if (e.saturation < 0 || e.saturation > 2) bad('project', '饱和度需要在 0~2 之间')
  if (e.sharpen < 0 || e.sharpen > 2) bad('project', '锐化需要在 0~2 之间')
  const ids = new Set<string>()
  const each = (c: AnyClip) => {
    const h = `clip=${c.id} path=${c.path}`
    if (!CLIP_ID_RE.test(c.id)) bad(h, 'clip id 需要 1~64 个字符，只能用字母、数字、下划线和连字符')
    if (ids.has(c.id)) bad(h, 'clip id 重复')
    ids.add(c.id)
    if (c.startSec < 0) bad(h, 'startSec 不能为负')
    if (c.inSec < 0) bad(h, 'inSec 不能为负')
    if (!(c.outSec > c.inSec)) bad(h, 'outSec 必须大于 inSec（0 不表示到素材结尾，请填探测到的时长）')
    if (c.speed !== 0 && (c.speed < 0.25 || c.speed > 4)) bad(h, 'speed 需要在 0.25~4 之间')
  }
  for (const c of p.videoTrack) {
    each(c)
    const h = `clip=${c.id} path=${c.path}`
    if (!VIDEO_TRACK_RE.test(c.trackId)) bad(h, 'trackId 需要是 V1~V8')
    if (c.effectPreset && !EFFECTS.includes(c.effectPreset)) bad(h, 'effectPreset 不在枚举里')
    if (c.transitionToNext && !TRANSITIONS.includes(c.transitionToNext)) bad(h, 'transitionToNext 不在枚举里')
    if (c.transitionDurationSec !== 0 && (c.transitionDurationSec < 0.1 || c.transitionDurationSec > 2)) bad(h, 'transitionDurationSec 需要在 0.1~2 之间')
    if (c.blur < 0 || c.blur > 4) bad(h, 'blur 需要在 0~4 之间')
  }
  for (const c of p.audioTrack) {
    each(c)
    const h = `clip=${c.id} path=${c.path}`
    if (!AUDIO_TRACK_RE.test(c.trackId)) bad(h, 'trackId 需要是 A1~A8')
    if (c.volume < 0 || c.volume > 4) bad(h, 'volume 需要在 0~4 之间')
  }
  // 同一轨道时间重叠 → INVALID_ARGUMENT（只在 Validate / Export 报，Save 不查）
  for (const list of [p.videoTrack, p.audioTrack] as AnyClip[][]) {
    const r = findTrackOverlap(list)
    if (r) bad(`clip=${r.b} path=${list.find((c) => c.id === r.b)?.path ?? ''}`, `轨道 ${r.trackId} 上的 clip ${r.a} 与 ${r.b} 时间重叠`)
  }
  if (new TextEncoder().encode(JSON.stringify(p)).length > MAX_PROJECT_BYTES) bad('project', '工程超过 1 MiB')
}

// ───────────── 输出文件名净化（§6.11.3，模拟层用；后端才是权威）─────────────

const WIN_RESERVED = /^(CON|PRN|AUX|NUL|COM[0-9¹²³]|LPT[0-9¹²³])$/i

/** outputName 净化：删控制字符与 \ / : * ? " < > |，去首尾空白和尾部的点与空格，避开 Windows 保留设备名（前加下划线），按字符截断 100，为空 → edit */
export function sanitizeOutputName(raw: string, projectName = ''): string {
  const clean = (s: string) => {
    let t = [...s].filter((ch) => { const c = ch.codePointAt(0)!; return !(c <= 0x1f || (c >= 0x7f && c <= 0x9f)) && !'\\/:*?"<>|'.includes(ch) }).join('')
    t = t.trim().replace(/[. ]+$/g, '')
    // 保留名与扩展名无关：CON.txt 也是保留名，取第一个点之前的部分判断
    if (WIN_RESERVED.test(t.split('.')[0].trim())) t = '_' + t
    return t
  }
  let n = clean(raw.trim() || projectName.trim())
  n = clean([...n].slice(0, 100).join(''))
  return n || 'edit'
}

// ───────────── 模拟层内部状态 ─────────────

const simProjects = new Map<string, EditProject>()
let simSeq = 0
const simDuration = (path: string) => 60 + (baseName(path).length % 7) * 10

function simCheckMaterials(p: EditProject): EditPlan {
  const warnings: string[] = []
  const inputs: string[] = []
  const durOf = (path: string) => simDuration(path)
  const each = (c: AnyClip, kind: 'video' | 'audio') => {
    const h = `clip=${c.id} path=${c.path}`
    if (!isAbs(c.path)) bad(h, '素材路径必须是绝对路径')
    const name = baseName(c.path)
    if (name.startsWith('缺失') || name.startsWith('missing')) simError('NOT_FOUND', '素材文件不存在', `${h}\n文件不存在`)
    if (name.startsWith('损坏') || name.startsWith('broken')) simError('PROBE_FAILED', '无法解析素材', `${h}\nInvalid data found when processing input`)
    if (!name.includes('.')) simError('INVALID_ARGUMENT', '素材是目录', `${h}\n是目录，不是文件`)
    const audioOnly = AUDIO_ONLY_EXTS.includes(extOf(c.path))
    if (kind === 'video' && audioOnly) bad(h, '视频 clip 的素材必须有视频流')
    if (kind === 'audio' && name.startsWith('无声')) bad(h, '音频 clip 的素材必须有音频流')
    const d = durOf(c.path)
    if (c.inSec >= d) bad(h, `inSec 不能超过素材时长（${d} 秒）`)
    if (c.outSec > d + 0.05) warnings.push(`clip ${c.id} outSec 超过素材时长，已截断`)
    if (!inputs.includes(c.path)) inputs.push(c.path)
  }
  p.videoTrack.forEach((c) => each(c, 'video'))
  p.audioTrack.forEach((c) => each(c, 'audio'))
  const all = [...p.videoTrack, ...p.audioTrack]
  const duration = Math.max(0, ...all.map((c) => c.startSec + clipTimelineLength(c)))
  if (duration > MAX_TIMELINE_SEC) bad('project', '时间线总长不能超过 6 小时')
  return { durationSec: duration, clipCount: all.length, inputs, hasAudio: p.audioTrack.length > 0, warnings }
}

function simInject(): void {
  const inj = simInjection()
  if (inj && inj.when === 'call') simError(inj.code, '模拟错误', injectionDetail(inj))
}

// ───────────── 服务方法 ─────────────

/** 不落盘、不启动导出；探测素材并做全部校验，返回规范化后的时长与警告 */
export async function validateProject(project: EditProject): Promise<EditPlan> {
  if (EDIT_BACKEND_READY) return await callService<EditPlan>('EditService', 'ValidateProject', project)
  await simDelay(200)
  simInject()
  checkStructure(project, { requireVideo: true })
  return simCheckMaterials(project)
}

/**
 * 提交一个 edit_export 任务（batch 池）。先整体校验再提交，任何一项失败不产生任务。
 * 进度走 task:progress，取消走 TaskService.Cancel。模拟：?sim_err=PROCESS_FAILED / CONVERT_DISK_FULL 让导出中途失败。
 */
export async function exportProject(project: EditProject, opts: EditExportOptions): Promise<ApiTask> {
  if (EDIT_BACKEND_READY) return toApiTask(await callService('EditService', 'Export', project, opts))
  await simDelay(200)
  simInject()
  if (opts.outputDir && !isAbs(opts.outputDir)) simError('INVALID_ARGUMENT', 'outputDir 必须是绝对路径', 'project\noutputDir 必须是绝对路径')
  checkStructure(project, { requireVideo: true })
  const plan = simCheckMaterials(project)
  const name = sanitizeOutputName(opts.outputName, project.name)
  const fmt = project.output.format || 'mp4'
  const dir = opts.outputDir || '/Users/me/Movies/FFmpegFree'
  const inj = simInjection()
  return createSimTask({
    type: 'edit_export',
    title: `${name}.${fmt}`,
    inputPaths: plan.inputs,
    outputPath: `${dir}/${name}.${fmt}`,
    params: JSON.stringify({ project, options: opts, outputDir: dir }),
    simSeconds: 8,
    mediaSec: plan.durationSec,
    fail: inj && inj.when === 'task' ? { code: inj.code, message: '模拟错误', detail: injectionDetail(inj) ?? 'Error while filtering', atProgress: 0.5 } : undefined,
  })
}

/** 预览用的 /local/<token>。校验：绝对路径、存在、是文件、扩展名在允许列表内，否则 INVALID_ARGUMENT / NOT_FOUND */
export async function getPreviewURL(path: string): Promise<PreviewURL> {
  if (EDIT_BACKEND_READY) return await callService<PreviewURL>('EditService', 'GetPreviewURL', path)
  await simDelay(60)
  simInject()
  if (!isAbs(path)) simError('INVALID_ARGUMENT', '路径必须是绝对路径')
  if (!PREVIEW_EXTS.includes(extOf(path))) simError('INVALID_ARGUMENT', '不支持预览这种文件')
  if (baseName(path).startsWith('缺失') || baseName(path).startsWith('missing')) simError('NOT_FOUND', '文件不存在')
  const token = `sim${(++simSeq).toString(36)}${Math.random().toString(36).slice(2, 8)}`
  simTokens.add(token)
  // ?sim_preview_404=1：第一次拿到的 token 立刻失效，模拟“重启 / 文件被换”后的 404，用来验证前端会重新调用 GetPreviewURL
  if (simParam('sim_preview_404') === '1' && !simPreview404Done) {
    simPreview404Done = true
    simTokens.delete(token)
  }
  return { url: `/local/${token}`, mime: AUDIO_ONLY_EXTS.includes(extOf(path)) ? 'audio/mpeg' : 'video/mp4', size: 128 * 1024 * 1024 }
}
const simTokens = new Set<string>()
let simPreview404Done = false

/** 这个预览地址是否已经失效（404）。真实：HEAD 请求；模拟：token 表 */
export async function isPreviewGone(url: string): Promise<boolean> {
  if (!EDIT_BACKEND_READY) return !simTokens.has(url.replace(/^\/local\//, ''))
  try {
    return (await fetch(url, { method: 'HEAD' })).status === 404
  } catch {
    return false
  }
}

export interface PreviewSource {
  /** 当前可用的地址（未取到前为 ''） */
  readonly url: string
  /** 第一次取地址（或重新取）：调用 GetPreviewURL */
  load(): Promise<PreviewURL>
  /**
   * 绑在 <video> / <audio> 的 error 事件上：确认是 404 后重新调用 GetPreviewURL 并返回新地址；不是 404（解码失败等）返回 null，不重试。
   * 同一个地址连续失败最多重试 2 次，避免文件真的没了时死循环（此时 GetPreviewURL 会抛 NOT_FOUND，由调用方提示素材丢失）。
   */
  onMediaError(): Promise<PreviewURL | null>
}

/** 一个素材的预览地址管理：token 进程内有效（重启失效），遇到 404 重新调用 GetPreviewURL */
export function createPreviewSource(path: string): PreviewSource {
  let current: PreviewURL | null = null
  let retries = 0
  const src: PreviewSource = {
    get url() {
      return current?.url ?? ''
    },
    async load() {
      current = await getPreviewURL(path)
      return current
    },
    async onMediaError() {
      if (!current || retries >= 2) return null
      if (!(await isPreviewGone(current.url))) return null
      retries++
      current = await getPreviewURL(path)
      return current
    },
  }
  return src
}

/** 保存工程。id 空 = 新建；只校验数量上限，不校验同轨重叠和范围，不探测素材、不要求文件存在。后端不做自动保存，前端需要时自行防抖调用 */
export async function saveProject(project: EditProject): Promise<EditProjectMeta> {
  if (EDIT_BACKEND_READY) return await callService<EditProjectMeta>('EditService', 'SaveProject', project)
  await simDelay(100)
  simInject()
  checkSaveLimits(project) // 只查数量上限；同轨重叠、范围问题草稿也能保存，Validate / Export 才报
  let id = project.id
  if (!id) id = `sim-proj-${(++simSeq).toString(36)}`
  else if (!simProjects.has(id)) simError('NOT_FOUND', '工程不存在')
  const saved: EditProject = { ...JSON.parse(JSON.stringify(project)), id, name: project.name.trim(), updatedAt: Date.now() }
  simProjects.set(id, saved)
  return metaOf(saved)
}

function metaOf(p: EditProject): EditProjectMeta {
  const all = [...p.videoTrack, ...p.audioTrack]
  return {
    id: p.id, name: p.name, clipCount: all.length, updatedAt: p.updatedAt,
    durationSec: Math.max(0, ...all.map((c) => c.startSec + clipTimelineLength(c))),
  }
}

/** 载入工程。不因素材丢失而失败，缺失路径放 missingPaths；schemaVersion 过新 → UNSUPPORTED */
export async function loadProject(id: string): Promise<LoadedProject> {
  if (EDIT_BACKEND_READY) return await callService<LoadedProject>('EditService', 'LoadProject', id)
  await simDelay(100)
  simInject()
  const p = simProjects.get(id)
  if (!p) return simError('NOT_FOUND', '工程不存在')
  if (p.schemaVersion > EDIT_SCHEMA_VERSION) simError('UNSUPPORTED', '这个工程来自更新版本的应用，无法打开')
  const paths = new Set([...p.sources, ...p.videoTrack.map((c) => c.path), ...p.audioTrack.map((c) => c.path)])
  const missing = [...paths].filter((x) => baseName(x).startsWith('缺失') || baseName(x).startsWith('missing'))
  return { project: JSON.parse(JSON.stringify(p)), missingPaths: missing }
}

/** 最近的工程，按 updatedAt 倒序。limit 默认 50，最大 200（0 = 默认，越界 INVALID_ARGUMENT） */
export async function listProjects(limit = 0): Promise<EditProjectMeta[]> {
  if (EDIT_BACKEND_READY) return (await callService<EditProjectMeta[] | null>('EditService', 'ListProjects', limit)) ?? []
  if (limit < 0 || limit > 200) simError('INVALID_ARGUMENT', 'limit 范围 0~200')
  return [...simProjects.values()].map(metaOf).sort((a, b) => b.updatedAt - a.updatedAt).slice(0, limit || 50)
}

/** 删除工程记录，不删素材和导出文件；不存在 NOT_FOUND */
export async function deleteProject(id: string): Promise<void> {
  if (EDIT_BACKEND_READY) {
    await callService('EditService', 'DeleteProject', id)
    return
  }
  simInject()
  if (!simProjects.delete(id)) simError('NOT_FOUND', '工程不存在')
}

export { AppError, toAppError }
