// 剪辑页纯逻辑（不依赖 Vue / 浏览器）：限制、同轨重叠与 0.12 秒相接、切割、吸附、时间码、文件名长度、导出错误定位。
// 自检：npm run check:edit（见 scripts/check-edit.mjs，不引入测试框架）。
import { clipTimelineLength, findTrackOverlap, wouldOverlap, type AudioClip, type VideoClip } from '@/api/edit'
import { FALLBACK_DESCRIPTION, taskErrorMessages } from '@/errors/errorMessages'
import { parseDetailHead } from '@/api/call'

export type AnyClip = VideoClip | AudioClip

// ───────── 契约限制（EditService 6.11 + 产品补充）─────────
export const MAX_VIDEO_TRACKS = 8
export const MAX_AUDIO_TRACKS = 8
export const MAX_CLIPS = 100
/** 素材库上限：先按 100（产品经理尚未最终确认，原型写的是 200；契约 EditProject.sources 上限 200） */
export const MAX_SOURCES = 100
export const MAX_TIMELINE_SEC = 6 * 3600
/** 片段最短 0.1 秒（契约只要求 outSec > inSec，前端定 0.1） */
export const MIN_CLIP_SEC = 0.1
/** 同轨相邻片段间隙 ≤ 0.12 秒视为首尾相接（后端 filtergraph 用 xfade / concat），> 0.12 秒是空隙（导出补黑场和静音） */
export const TOUCH_GAP_SEC = 0.12
/** 判重叠的浮点容差，与 api/edit.ts 的 findTrackOverlap 一致（1ms） */
export const OVERLAP_EPS = 0.001
/** 输出文件名上限（字符数，按 Unicode 字符算） */
export const MAX_NAME_CHARS = 100
/** Windows 整条输出路径上限 */
export const WIN_MAX_PATH = 259
/** 一次删除达到这个数量才弹确认（设计师方案，产品经理尚未确认） */
export const DELETE_CONFIRM_MIN = 5
export const MAX_PROJECT_NAME = 80
export const FRAME_FPS = 30

export const LIMIT_TEXT = '限制：视频轨 8 条、音频轨 8 条，片段 100 个，时间线 6 小时'

// ───────── 逐字文案（设计说明 2.11 / 3.3 / 4.2）─────────
export const TEXT = {
  overlapToast: '同一轨道上的片段不能重叠，画中画请放到不同轨道',
  /** 产品经理还没审这句文案（设计说明问题 12） */
  overlapAfterInline: '会和后面的片段重叠',
  clipLimitToast: `片段数量已达上限 ${MAX_CLIPS} 个，不能再添加`,
  timelineLimitToast: '时间线最长 6 小时，放不下这个片段',
  sourceLimitToast: `素材库最多 ${MAX_SOURCES} 个素材`,
  trackLimitTip: '视频轨和音频轨最多各 8 条',
  trackMismatch: '这个素材不能放在这条轨道',
  noClips: '时间线上还没有片段',
  splitNeedSelect: '先选中一个片段，并把播放头放在片段中间',
  splitFull: '片段已达 100 个上限，不能再切割',
  addFull: '片段已达 100 个上限，不能再添加',
  nameTooLong: '文件名或保存位置的路径太长，请缩短',
  nameTooLongTip: '先改短文件名或保存位置的路径',
  noAudio: '没有音频轨道，导出的视频将没有声音',
  exportBusyTip: '正在导出，完成后才能再次导出',
  exportEmptyTip: '先把素材加到时间线才能导出',
  saveEmptyTip: '没有可保存的内容',
  ffmpegTip: '需要先安装 ffmpeg',
  warningGeneric: '有一处会被自动调整，不影响导出。',
  transitionOver: '转场不能超过较短片段的一半',
  transitionTooShort: '相邻片段太短，放不下转场',
  transitionIgnored: '部分转场因片段太短未生效',
} as const

export const rangeText = (min: number, max: number) => `请输入 ${min} 到 ${max} 之间的数`

// ───────── 轨道 ─────────
export const isVideoTrackId = (id: string) => /^V[1-8]$/.test(id)
export const isAudioTrackId = (id: string) => /^A[1-8]$/.test(id)
export const trackNo = (id: string) => Number(id.slice(1)) || 0

// ───────── 时间 ─────────
export const round6 = (n: number) => Math.round(n * 1e6) / 1e6
export const clipLen = (c: AnyClip) => clipTimelineLength(c)
export const clipEnd = (c: AnyClip) => round6(c.startSec + clipTimelineLength(c))

/** 时间线总长 = max(startSec + (outSec-inSec)/speed)，音视频一起算 */
export function timelineDuration(clips: readonly AnyClip[]): number {
  let m = 0
  for (const c of clips) m = Math.max(m, clipEnd(c))
  return m
}

export type GapKind = 'overlap' | 'touch' | 'gap'
/**
 * 同轨相邻两个片段的关系：后一个开始早于前一个结束（容差 1ms）= 重叠；
 * 间隙 ≤ 0.12 秒 = 首尾相接；> 0.12 秒 = 空隙（导出补黑场和静音）。
 */
export function gapKind(prevEnd: number, nextStart: number): GapKind {
  const d = nextStart - prevEnd
  if (d < -OVERLAP_EPS) return 'overlap'
  return d <= TOUCH_GAP_SEC + 1e-9 ? 'touch' : 'gap'
}

/** 同一轨道上按开始时间排好序的片段 */
export function clipsOnTrack<T extends AnyClip>(clips: readonly T[], trackId: string): T[] {
  return clips.filter((c) => c.trackId === trackId).sort((a, b) => a.startSec - b.startSec)
}

/** 与 clip 首尾相接的后一个片段（同轨，间隙 ≤ 0.12 秒）；没有返回 null。转场只在首尾相接的片段之间有效 */
export function nextTouching<T extends AnyClip>(clips: readonly T[], clip: T): T | null {
  const list = clipsOnTrack(clips, clip.trackId)
  const i = list.findIndex((c) => c.id === clip.id)
  const next = i >= 0 ? list[i + 1] : undefined
  return next && gapKind(clipEnd(clip), next.startSec) === 'touch' ? next : null
}

/** 转场时长上限：不超过相邻两个片段中较短者的一半（契约），并且不超过 2 秒 */
export function maxTransitionSec(clip: AnyClip, next: AnyClip): number {
  return Math.min(2, Math.floor((Math.min(clipLen(clip), clipLen(next)) / 2) * 100) / 100)
}
export const MIN_TRANSITION_SEC = 0.1
/** 上限不足 0.1 秒：放不下转场，“转场”下拉 aria-disabled */
export const transitionFits = (max: number) => max >= MIN_TRANSITION_SEC - 1e-9
/**
 * 用户手动输入的转场时长：超过上限直接限制为上限（不回退旧值），over = 是否触发“转场不能超过较短片段的一半”提示；
 * 小于 0.1 秒取 0.1。默认时长 / 拖动导致的超限用 clampSilently，不提示。
 */
export function clampTransitionInput(v: number, max: number): { value: number; over: boolean } {
  const m = Math.max(MIN_TRANSITION_SEC, max)
  if (v > m + 1e-9) return { value: m, over: true }
  return { value: Math.max(MIN_TRANSITION_SEC, Math.round(v * 100) / 100), over: false }
}
export const clampSilently = (v: number, max: number) => Math.min(v || 0.5, Math.max(MIN_TRANSITION_SEC, max))

// ───────── 重叠 / 限制判断（拖动、放置、改数值都走这里）─────────
/** candidate 放到位后与同轨其它片段的冲突（返回冲突片段 id，没有 null），直接用接口层的 wouldOverlap */
export function overlapWith(clips: readonly AnyClip[], candidate: AnyClip): string | null {
  return wouldOverlap(clips as AnyClip[], candidate)
}
export const hasAnyOverlap = (clips: readonly AnyClip[]) => findTrackOverlap(clips as AnyClip[])

/** 放上 / 改动 candidate 后时间线总长是否超过 6 小时（忽略 candidate 自己原来的样子） */
export function exceedsTimeline(clips: readonly AnyClip[], candidate: AnyClip): boolean {
  const others = clips.filter((c) => c.id !== candidate.id)
  return Math.max(timelineDuration(others), clipEnd(candidate)) > MAX_TIMELINE_SEC + 1e-6
}
export const clipCountFull = (count: number) => count >= MAX_CLIPS

export type PlaceProblem = 'track' | 'overlap' | 'timeline' | 'count' | null
/**
 * 把 candidate 放到某条轨道的判断顺序：片段数（新增才算）→ 轨道类型 → 同轨重叠 → 6 小时。
 * kindOk 由调用方按素材的音 / 视频流算出。
 */
export function placeProblem(clips: readonly AnyClip[], candidate: AnyClip, opts: { adding: boolean; kindOk: boolean }): PlaceProblem {
  if (opts.adding && clipCountFull(clips.length)) return 'count'
  if (!opts.kindOk) return 'track'
  if (overlapWith(clips, candidate)) return 'overlap'
  if (exceedsTimeline(clips, candidate)) return 'timeline'
  return null
}
export function placeProblemText(p: PlaceProblem): string {
  return p === 'track' ? TEXT.trackMismatch : p === 'overlap' ? TEXT.overlapToast : p === 'timeline' ? TEXT.timelineLimitToast : p === 'count' ? TEXT.clipLimitToast : ''
}

// ───────── 切割 ─────────
/** 切点离片段两端各至少 0.1 秒 */
export function splitPointOk(c: AnyClip, at: number): boolean {
  const t = at - c.startSec
  return t >= MIN_CLIP_SEC - 1e-9 && clipLen(c) - t >= MIN_CLIP_SEC - 1e-9
}

/** 要切割哪个片段：选中的（播放头必须在它里面）；没选中但播放头下只有一个片段时切那个 */
export function resolveSplitTarget(clips: readonly AnyClip[], selectedId: string | null, playhead: number): AnyClip | null {
  if (selectedId) {
    const c = clips.find((x) => x.id === selectedId)
    return c && splitPointOk(c, playhead) ? c : null
  }
  const under = clips.filter((c) => playhead > c.startSec && playhead < clipEnd(c))
  return under.length === 1 && splitPointOk(under[0], playhead) ? under[0] : null
}

/** 切割不可用的原因（可用返回 null）：没有片段 → 已 100 个 → 没选中 / 播放头不在片段中间 */
export function splitBlockReason(clips: readonly AnyClip[], selectedId: string | null, playhead: number): string | null {
  if (!clips.length) return TEXT.noClips
  if (clipCountFull(clips.length)) return TEXT.splitFull
  return resolveSplitTarget(clips, selectedId, playhead) ? null : TEXT.splitNeedSelect
}

/**
 * 把一个片段在时间线时刻 at 拆成两个：前一个 outSec = 切点，后一个 inSec = 切点、startSec = at（速度换算后保持连续）。
 * 转场留在后一个片段上（它才是原片段的“后面”），前一个片段转场清空。切不了（离两端 < 0.1 秒）返回 null。
 */
export function splitClip<T extends AnyClip>(c: T, at: number, ids: [string, string]): [T, T] | null {
  if (!splitPointOk(c, at)) return null
  const speed = c.speed > 0 ? c.speed : 1
  const cut = round6(c.inSec + (at - c.startSec) * speed)
  const a = { ...c, id: ids[0], outSec: cut } as T
  const b = { ...c, id: ids[1], inSec: cut, startSec: round6(at) } as T
  if ('transitionToNext' in a) {
    ;(a as VideoClip).transitionToNext = 'none'
    ;(a as VideoClip).transitionDurationSec = 0
  }
  return [a, b]
}

// ───────── 吸附 ─────────
export const SNAP_PX = 8
/** 吸附点：其它片段的起止点、播放头、0 秒 */
export function snapPoints(clips: readonly AnyClip[], excludeId: string | null, playhead: number): number[] {
  const pts = [0, playhead]
  for (const c of clips) if (c.id !== excludeId) pts.push(c.startSec, clipEnd(c))
  return pts
}
/** 起点或终点靠近某个吸附点（阈值内）就吸过去；返回新的起点和吸到的点（没有吸附为 null） */
export function snapStart(start: number, length: number, points: readonly number[], thresholdSec: number): { start: number; at: number | null } {
  let best: { d: number; start: number; at: number } | null = null
  for (const p of points) {
    for (const [edge, off] of [[start, 0], [start + length, length]] as const) {
      const d = Math.abs(edge - p)
      if (d <= thresholdSec && (!best || d < best.d)) best = { d, start: p - off, at: p }
    }
  }
  return best && best.start >= 0 ? { start: round6(best.start), at: best.at } : { start, at: null }
}

// ───────── 时间码 ─────────
const p2 = (n: number) => String(n).padStart(2, '0')
/** 显示用时间码 mm:ss.ff（帧按 30 帧显示，仅显示用途）；≥ 1 小时带 hh: */
export function formatTC(t: number, fps = FRAME_FPS): string {
  const s = Math.max(0, t)
  let whole = Math.floor(s + 1e-9)
  let f = Math.round((s - whole) * fps)
  if (f >= fps) {
    whole += 1
    f = 0
  }
  const h = Math.floor(whole / 3600)
  return `${h ? p2(h) + ':' : ''}${p2(Math.floor((whole % 3600) / 60))}:${p2(whole % 60)}.${p2(f)}`
}
/** 标尺 / 用量用：mm:ss，≥ 1 小时 hh:mm:ss；forceHours 时始终三段（06:00:00） */
export function formatClock(t: number, forceHours = false): string {
  const s = Math.max(0, Math.round(t))
  const h = Math.floor(s / 3600)
  return `${h || forceHours ? p2(h) + ':' : ''}${p2(Math.floor((s % 3600) / 60))}:${p2(s % 60)}`
}
/** 解析输入的时间："12"、"12.5"、"1:02.5"、"01:02:03.5"；解析不了返回 null */
export function parseTC(text: string): number | null {
  const t = text.trim()
  if (!t || !/^\d+(:\d+){0,2}(\.\d+)?$/.test(t)) return null
  const parts = t.split(':').map(Number)
  let sec = 0
  for (const p of parts) sec = sec * 60 + p
  return Number.isFinite(sec) ? sec : null
}

// ───────── 文件名 / 路径 ─────────
export const nameLength = (name: string) => [...name].length
export const nameTooLong = (name: string) => nameLength(name) > MAX_NAME_CHARS
/** 整条输出路径（含扩展名和最坏情况的 " (1)" 后缀）是否超过 Windows 的 259；只在 Windows 计算 */
export function pathTooLong(dir: string, name: string, format: string, isWindows: boolean): boolean {
  if (!isWindows) return false
  const total = dir.replace(/[\\/]+$/, '').length + 1 + name.length + 1 + format.length + ' (1)'.length
  return total > WIN_MAX_PATH
}
/** 导出弹框里文件名 / 路径的错误文案（没有错误返回 null）：名字超 100 字符，或 Windows 整条路径超 259 */
export function exportNameError(name: string, dir: string, format: string, isWindows: boolean): { text: string; path: boolean } | null {
  if (nameTooLong(name)) return { text: TEXT.nameTooLong, path: false }
  if (pathTooLong(dir, name, format, isWindows)) return { text: TEXT.nameTooLong, path: true }
  return null
}
export const isWindowsPlatform = () => typeof navigator !== 'undefined' && /win/i.test(navigator.platform || navigator.userAgent || '')

// ───────── 校验提示（ValidateProject 的 warnings：code 枚举只追加、可选 clipId、message）─────────
export interface EditWarning {
  code: string
  clipId?: string
  message: string
}
/** 已知 code → 文案（按 code 出文案；未知 code 用通用文案）。label 是“片段 3”/“V1 片段 3”，找不到片段时为空 */
export const WARNING_TEXT: Record<string, (label: string) => string> = {
  OUT_EXCEEDS_DURATION: (label) => `${label || '有一个片段'} 的出点超过素材时长，导出时会截到素材结尾。`,
}
/**
 * 规范化 warnings。结构化形式 {code, clipId?, message} 原样接收；
 * 接口层模拟目前还返回旧的字符串形式（"clip <id> outSec 超过素材时长，已截断"），这里按同一句式解析出 code / clipId，其余字符串按未知 code 处理。
 */
/** 导出时按空隙补黑场 / 静音的提示码：界面不提示（设计说明 2.14），只在校验提示里过滤掉 */
export const SILENT_WARNING_CODES = ['CLIP_GAP', 'LEADING_GAP']
export const isSilentWarning = (w: EditWarning) => SILENT_WARNING_CODES.includes(w.code.toUpperCase())
export const hasTransitionIgnored = (ws: readonly EditWarning[]) => ws.some((w) => w.code.toUpperCase() === 'TRANSITION_IGNORED')
export function normalizeWarnings(raw: unknown): EditWarning[] {
  if (!Array.isArray(raw)) return []
  return raw.map((w): EditWarning => {
    if (typeof w === 'string') {
      const m = /^clip (\S+) outSec 超过素材时长/.exec(w)
      if (m) return { code: 'OUT_EXCEEDS_DURATION', clipId: m[1], message: w }
      const t = /^(transition_ignored|clip_gap|leading_gap)\b\s*(?:clip (\S+))?/i.exec(w)
      return t ? { code: t[1].toUpperCase(), clipId: t[2], message: w } : { code: 'UNKNOWN', message: w }
    }
    const o = (w ?? {}) as Partial<EditWarning>
    return { code: String(o.code ?? 'UNKNOWN'), clipId: o.clipId || undefined, message: String(o.message ?? '') }
  })
}
export function warningText(w: EditWarning, label = ''): string {
  const f = WARNING_TEXT[w.code.toUpperCase()]
  return f ? f(label) : TEXT.warningGeneric
}

// ───────── clip 定位（导出失败：detail 第一行 clip=<id> path=<路径>）─────────
export interface ClipProject {
  videoTrack: readonly VideoClip[]
  audioTrack: readonly AudioClip[]
}
export function locateClip(p: ClipProject, clipId: string): { trackId: string; n: number; multi: boolean } | null {
  const all: AnyClip[] = [...p.videoTrack, ...p.audioTrack]
  const c = all.find((x) => x.id === clipId)
  if (!c) return null
  const n = clipsOnTrack(all, c.trackId).findIndex((x) => x.id === clipId) + 1
  const multi = new Set(all.map((x) => x.trackId)).size > 1
  return { trackId: c.trackId, n, multi }
}
/** “片段 3”；同一工程有多条有片段的轨道时写成“V1 片段 3” */
export function clipNumberLabel(p: ClipProject, clipId: string): string {
  const l = locateClip(p, clipId)
  return l ? `${l.multi ? l.trackId + ' ' : ''}片段 ${l.n}` : ''
}

export interface ExportErrorView {
  title: string
  /** 已拼好“片段 N 出错：”的说明 */
  text: string
  code: string
  clipId: string | null
  actions: ('locate' | 'retry' | 'changeOutput' | 'log')[]
}
/**
 * 导出失败错误行：标题固定“导出失败”，只有 CONVERT_DISK_FULL 用专门标题；
 * detail 首行的 clip id 能在工程里找到时，说明前拼“片段 N 出错：”并给“定位片段”；找不到（已删除）或是 project 就不拼、没有定位。
 * detail 原文不显示在界面上（只给“查看日志”）。
 */
export function exportErrorView(err: { code: string; message?: string; detail?: string }, p: ClipProject): ExportErrorView {
  const disk = err.code === 'CONVERT_DISK_FULL'
  const head = parseDetailHead(err.detail)
  const label = head.clipId ? clipNumberLabel(p, head.clipId) : ''
  const base = disk ? taskErrorMessages.CONVERT_DISK_FULL.description : (err.message ?? '').trim() || FALLBACK_DESCRIPTION
  const clipId = label ? head.clipId! : null
  return {
    title: disk ? taskErrorMessages.CONVERT_DISK_FULL.title : '导出失败',
    text: label && !disk ? `${label} 出错：${base}` : base,
    code: err.code,
    clipId,
    actions: disk ? ['retry', 'changeOutput', 'log'] : clipId ? ['locate', 'retry', 'log'] : ['retry', 'log'],
  }
}

// ───────── 删除确认 ─────────
export const deleteNeedsConfirm = (n: number) => n >= DELETE_CONFIRM_MIN

// ───────── 素材信息文字 ─────────
export function resolutionTier(height: number): string {
  if (!height) return ''
  for (const t of [2160, 1440, 1080, 720, 480]) if (height >= t) return `${t}p`
  return `${height}p`
}
