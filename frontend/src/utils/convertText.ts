/** 转换页 v2 的纯函数：冲突判断（沿用 S1 逻辑）、预设名、记录时间、媒体信息文字。没有副作用，自检直接调用。 */
import { shallowRef } from 'vue'
import { formatBytes, formatShortClock } from '@/utils/format'
import { audioCodecText, channelText, codecName, sampleRateText, videoCodecText } from '@/utils/mediaText'

export const VIDEO_CONTAINERS: readonly string[] = ['mp4', 'mkv', 'mov', 'webm', 'avi', 'flv', 'gif']
export const AUDIO_CONTAINERS: readonly string[] = ['mp3', 'aac', 'm4a', 'wav', 'flac', 'ogg', 'opus']
export const isAudioContainer = (c: string): boolean => AUDIO_CONTAINERS.includes((c ?? '').toLowerCase())

/** 冲突提示（产品决定 v1）：标题不变，不显示错误码；说明里的出路改成“取消勾选”（列表里没有“移出”了） */
export const CONFLICT_TITLE = '这个文件不能用当前预设'
export const CONFLICT_NO_VIDEO = '没有画面，不能转成视频格式。请换一个音频预设，或取消勾选。'
export const CONFLICT_NO_AUDIO = '没有声音，不能转成音频格式。请换一个视频预设，或取消勾选。'

/**
 * 当前预设与文件不兼容的原因，null = 兼容。只在读取成功（probeOk）后判断——还没读完的不算冲突；
 * 读取成功的媒体 hasVideo / hasAudio 缺失按 false（后端 omitempty，api/media.ts 已归一化，这里再兜一次）。
 */
export function conflictReason(info: { hasVideo?: boolean; hasAudio?: boolean } | undefined, probeOk: boolean, container: string | undefined): string | null {
  if (!probeOk || !info || !container) return null
  const c = container.toLowerCase()
  if (VIDEO_CONTAINERS.includes(c) && info.hasVideo !== true) return CONFLICT_NO_VIDEO
  if (AUDIO_CONTAINERS.includes(c) && info.hasAudio !== true) return CONFLICT_NO_AUDIO
  return null
}

/** 预设名「MP4（H.264 + AAC，通用）」→ 标题 MP4 + 说明 H.264 + AAC，通用 */
export function splitPresetName(name: string): { title: string; sub: string } {
  const m = (name ?? '').match(/^(.*?)[（(](.*)[）)]\s*$/)
  return m ? { title: m[1].trim(), sub: m[2].trim() } : { title: name ?? '', sub: '' }
}

/**
 * 预设显示标题（预设卡片、子记录第 2 行、任务中心第二行、预览底栏共用一套规则；走查 G2）：
 * 括号前的标题；同一个标题在预设列表里出现不止一次（MP4 H.264 / MP4 H.265）时补编码简称，写成「MP4 · H.265」。
 */
export function dupPresetTitles(names: readonly string[]): Set<string> {
  const seen = new Set<string>()
  const dup = new Set<string>()
  for (const n of names) {
    const t = splitPresetName(n).title
    if (seen.has(t)) dup.add(t)
    seen.add(t)
  }
  return dup
}
export function presetShortTitle(name: string, codec: string | undefined, dup: ReadonlySet<string>): string {
  const t = splitPresetName(name).title
  const c = dup.has(t) ? codecName(codec) : ''
  return c ? `${t} · ${c}` : t
}
/** 当前预设列表里重名的标题（ListPresets 取到后由转换页 store / 任务中心写入；记录第 2 行据此补编码） */
const presetDup = shallowRef<ReadonlySet<string>>(new Set())
export function setPresetCatalog(names: readonly string[]): void {
  presetDup.value = dupPresetTitles(names)
}

/**
 * 子记录第 2 行 / 预览底栏 / 任务中心转换行第二行共用的“参数”部分（设计 §7.3 第 14 条，已定 UI 10-08）：
 * - presetId 非空（内置或保存过的预设）→ 预设名；悬停提示 = paramsSummary（preset=true）
 * - presetId 为空且 paramsSummary 非空（自定义参数、旧 Submit）→ “自定义 · 摘要”；悬停提示 = 整行文字（preset=false）
 * - 三个快照都为空（v0.23 之前的旧任务）→ title
 * 前端只拼接、不解析 paramsSummary。
 */
type RecordSnap = { presetId?: string; presetName?: string; paramsSummary?: string; title: string; options?: { videoCodec?: string; audioCodec?: string } }
export function recordParamsText(r: RecordSnap, dup: ReadonlySet<string> = presetDup.value): { text: string; tip: string; preset: boolean } {
  if (r.presetId) return { text: (r.presetName && presetShortTitle(r.presetName, r.options?.videoCodec || r.options?.audioCodec, dup)) || r.presetName || '自定义', tip: r.paramsSummary ?? '', preset: true }
  if (r.paramsSummary) return { text: `自定义 · ${r.paramsSummary}`, tip: '', preset: false }
  if (!r.presetName) return { text: r.title, tip: '', preset: false }
  return { text: '自定义', tip: '', preset: false }
}
/** 带前缀（时间 / “… 转换”）和后缀（设备）的整行文字 + 悬停提示：预设记录的提示是 paramsSummary，其余是整行 */
export function recordLine(r: RecordSnap, head: string[], tail: string[] = []): { text: string; title: string } {
  const p = recordParamsText(r)
  const text = [...head, p.text, ...tail].filter(Boolean).join(' · ')
  return { text, title: p.preset && p.tip ? p.tip : text }
}

/**
 * 删除 / 移除后的 toast——改文案只改这一段（产品经理 10-08 定稿，设计说明 §五、§八 第 29 / 32 条）。
 * n = deletedTaskIds.length，m = deletedFiles，k = 各类 failures 条数；行保留 = sourceId 不在 deletedSourceIds 里。
 * 拼法：主句 + still_running 句 + 文件句（按 reason 分类，每类一句，顺序：被占用 → 没有权限 → 其他）；有 failures 时页面用警告样式、8 秒，
 * “打开所在文件夹”只在有非空 path 时出现（取第一条非空 path；与分类无关）。
 * reason（契约 6.14.4 / internal/task/delete.go）：in_use / permission / not_task_output / io / still_running；未知的 reason 算“其他”。
 * - 源文件行移除成功：已从列表移除“名称”和 n 条记录。（删了输出：…，并删除了 m 个文件。没有记录：已从列表移除“名称”。）
 * - 源文件行因 still_running 保留：已删除 n 条记录。有 k 条转换没能及时停止，“名称”仍保留在列表里，请稍后再移除。（n = 0 时去掉第一句；不以“已从列表移除”开头）
 * - 删除记录（单条 / 批量）：已删除 n 条记录。有 k 条转换没能及时停止，请稍后再删除。（有 still_running 且 n = 0 时去掉第一句）
 * - 文件没删成（产品经理 10-08 确认，DeleteSource / DeleteRecords 共用）：
 *   in_use：有 k 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。
 *   permission：有 k 个文件没有权限删除，请手动删除。
 *   其他（not_task_output / io / 未知）：有 k 个文件没能删除，请手动删除。
 * 名称单独一段（{ name }），页面放进可省略、悬停看全名的 span。
 */
export type ToastPart = string | { name: string }
export const DELETED_RECORDS = (n: number) => `已删除 ${n} 条记录。`
export const DELETE_FAILED_FILES = (k: number) => `有 ${k} 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。`
export const DELETE_FAILED_PERMISSION = (k: number) => `有 ${k} 个文件没有权限删除，请手动删除。`
export const DELETE_FAILED_OTHER = (k: number) => `有 ${k} 个文件没能删除，请手动删除。`
/** DeleteFailure.reason（契约 6.14.4；Go：internal/task/delete.go DeleteInUse … DeleteStillRunning） */
export const DELETE_REASON = { inUse: 'in_use', permission: 'permission', notTaskOutput: 'not_task_output', io: 'io', stillRunning: 'still_running' } as const
export const SOURCE_KEPT_STILL_RUNNING = (k: number, name: string): ToastPart[] => [`有 ${k} 条转换没能及时停止，“`, { name }, '”仍保留在列表里，请稍后再移除。']
export const DELETE_RECORDS_STILL_RUNNING = (k: number) => `有 ${k} 条转换没能及时停止，请稍后再删除。`
export function sourceRemovedParts(name: string, n: number, m: number): ToastPart[] {
  const tail = n > 0 ? `和 ${n} 条记录${m > 0 ? `，并删除了 ${m} 个文件` : ''}。` : '。'
  return ['已从列表移除“', { name }, `”${tail}`]
}
export interface DeleteToast { parts: ToastPart[]; warn: boolean; path: string }
export function deleteToast(
  ask: { kind: 'source' | 'record'; id: string; name: string },
  r: { deletedTaskIds: string[]; deletedSourceIds: string[]; deletedFiles: number; failures: readonly { reason: string; path?: string }[] },
): DeleteToast {
  const n = r.deletedTaskIds.length
  const count = (pred: (reason: string) => boolean) => r.failures.filter((f) => pred(f.reason)).length
  const k = count((x) => x === DELETE_REASON.stillRunning)
  const inUse = count((x) => x === DELETE_REASON.inUse)
  const perm = count((x) => x === DELETE_REASON.permission)
  const other = count((x) => x !== DELETE_REASON.stillRunning && x !== DELETE_REASON.inUse && x !== DELETE_REASON.permission)
  const parts: ToastPart[] = []
  if (ask.kind === 'source') {
    if (k > 0 && !r.deletedSourceIds.includes(ask.id)) parts.push(...(n > 0 ? [DELETED_RECORDS(n)] : []), ...SOURCE_KEPT_STILL_RUNNING(k, ask.name))
    else parts.push(...sourceRemovedParts(ask.name, n, r.deletedFiles))
  } else {
    if (n > 0 || k === 0) parts.push(DELETED_RECORDS(n))
    if (k > 0) parts.push(DELETE_RECORDS_STILL_RUNNING(k))
  }
  if (inUse) parts.push(DELETE_FAILED_FILES(inUse))
  if (perm) parts.push(DELETE_FAILED_PERMISSION(perm))
  if (other) parts.push(DELETE_FAILED_OTHER(other))
  return { parts, warn: r.failures.length > 0, path: r.failures.find((f) => !!f.path)?.path ?? '' }
}
/** 纯文字（检查 / 无障碍用） */
export const toastText = (parts: readonly ToastPart[]): string => parts.map((p) => (typeof p === 'string' ? p : p.name)).join('')
/**
 * 删除失败提示上“打开所在文件夹”失败（契约 v0.23.3；设计说明 §7.2、§八 第 29 / 30 条）：
 * 文件被移走 NOT_FOUND、超过 10 分钟 / 重启后 INVALID_ARGUMENT → 都写“找不到这个文件”（不带句号，按说明）；其他错误用错误本身的文案。
 */
export const REVEAL_DELETE_FAILURE_NOT_FOUND = '找不到这个文件'
export const revealDeleteFailureText = (e: { code: string; message: string }): string => (e.code === 'NOT_FOUND' || e.code === 'INVALID_ARGUMENT' ? REVEAL_DELETE_FAILURE_NOT_FOUND : e.message)

/** 记录时间：今天 11:48 / 昨天 21:14 / 9月28日 16:40；跨年带年份 2025年9月28日 16:40 */
export function formatRecordTime(ms: number, now: number = Date.now()): string {
  if (!ms) return ''
  const d = new Date(ms)
  const n = new Date(now)
  const p = (x: number) => String(x).padStart(2, '0')
  const hm = `${p(d.getHours())}:${p(d.getMinutes())}`
  const day0 = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime()
  const diff = Math.round((day0(n) - day0(d)) / 86_400_000)
  if (diff === 0) return `今天 ${hm}`
  if (diff === 1) return `昨天 ${hm}`
  const md = `${d.getMonth() + 1}月${d.getDate()}日 ${hm}`
  return d.getFullYear() === n.getFullYear() ? md : `${d.getFullYear()}年${md}`
}

/** 是否是今天（分组“今天 / 更早”和默认展开用） */
export function isToday(ms: number, now: number = Date.now()): boolean {
  return !!ms && new Date(ms).toDateString() === new Date(now).toDateString()
}

interface InfoLike { width?: number; height?: number; videoCodec?: string; audioCodec?: string; videoCodecName?: string; audioCodecName?: string; duration?: number; size?: number; sampleRate?: number; channels?: number; hasVideo?: boolean; hasAudio?: boolean }
/** 有画面的按视频显示，其余按音频 */
export const isAudioOnly = (i?: InfoLike): boolean => !!i && (i.hasVideo === false || !i.width)

/** 父行信息：视频「分辨率 · 编码 · 时长 · 大小」（无声视频在编码后加“没有声音”）；音频「采样率 · 声道 · 时长 · 大小」 */
export function sourceMetaText(i: InfoLike): string {
  const parts: string[] = []
  if (!isAudioOnly(i)) {
    parts.push(`${i.width}×${i.height}`, videoCodecText(i))
    if (i.hasAudio === false) parts.push('没有声音')
  } else {
    parts.push(sampleRateText(i.sampleRate), channelText(i.channels))
    if (!i.sampleRate && !i.channels) parts.push(audioCodecText(i))
  }
  parts.push(formatShortClock(i.duration ?? 0), i.size ? formatBytes(i.size) : '')
  return parts.filter(Boolean).join(' · ')
}

/** 总进度：进行中 + 排队中的平均（排队按 0 算）；剩余时间取最大的那个 */
export function totalProgress(items: readonly { status: string; progress: number; etaSec: number }[]): { running: number; queued: number; pct: number; etaSec: number } {
  const act = items.filter((t) => t.status === 'running' || t.status === 'queued')
  const running = act.filter((t) => t.status === 'running').length
  if (!act.length) return { running: 0, queued: 0, pct: 0, etaSec: 0 }
  const sum = act.reduce((n, t) => n + (t.status === 'running' ? Math.min(1, Math.max(0, t.progress || 0)) : 0), 0)
  const eta = act.reduce((m, t) => Math.max(m, t.status === 'running' ? t.etaSec || 0 : 0), 0)
  return { running, queued: act.length - running, pct: Math.round((sum / act.length) * 100), etaSec: eta }
}

/** “剩余约 3 分钟”：不到 1 分钟按秒，1 小时以内按分钟（向上取整），更长“1 小时 5 分钟” */
export function roughEta(sec: number): string {
  if (!isFinite(sec) || sec <= 0) return ''
  if (sec < 60) return `${Math.ceil(sec)} 秒`
  if (sec < 3600) return `${Math.ceil(sec / 60)} 分钟`
  const h = Math.floor(sec / 3600)
  const m = Math.ceil((sec % 3600) / 60)
  return m ? `${h} 小时 ${m} 分钟` : `${h} 小时`
}

/** 子任务进行中的“剩余 0:52” */
export function shortEta(sec: number): string {
  if (!isFinite(sec) || sec <= 0) return ''
  const s = Math.round(sec)
  const p = (n: number) => String(n).padStart(2, '0')
  return s >= 3600 ? `${Math.floor(s / 3600)}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}` : `${Math.floor(s / 60)}:${p(s % 60)}`
}
