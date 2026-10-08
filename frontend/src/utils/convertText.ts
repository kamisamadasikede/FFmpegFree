/** 转换页 v2 的纯函数：冲突判断（沿用 S1 逻辑）、预设名、记录时间、媒体信息文字。没有副作用，自检直接调用。 */
import { formatBytes, formatShortClock } from '@/utils/format'
import { channelText, codecName, sampleRateText } from '@/utils/mediaText'

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
 * 子记录第 2 行 / 预览底栏 / 任务中心转换行第二行共用的“参数”部分（设计 §7.3 第 14 条，已定 UI 10-08）：
 * - presetId 非空（内置或保存过的预设）→ 预设名；悬停提示 = paramsSummary（preset=true）
 * - presetId 为空且 paramsSummary 非空（自定义参数、旧 Submit）→ “自定义 · 摘要”；悬停提示 = 整行文字（preset=false）
 * - 三个快照都为空（v0.23 之前的旧任务）→ title
 * 前端只拼接、不解析 paramsSummary。
 */
export function recordParamsText(r: { presetId?: string; presetName?: string; paramsSummary?: string; title: string }): { text: string; tip: string; preset: boolean } {
  if (r.presetId) return { text: (r.presetName && splitPresetName(r.presetName).title) || r.presetName || '自定义', tip: r.paramsSummary ?? '', preset: true }
  if (r.paramsSummary) return { text: `自定义 · ${r.paramsSummary}`, tip: '', preset: false }
  if (!r.presetName) return { text: r.title, tip: '', preset: false }
  return { text: '自定义', tip: '', preset: false }
}
/** 带前缀（时间 / “… 转换”）和后缀（设备）的整行文字 + 悬停提示：预设记录的提示是 paramsSummary，其余是整行 */
export function recordLine(r: { presetId?: string; presetName?: string; paramsSummary?: string; title: string }, head: string[], tail: string[] = []): { text: string; title: string } {
  const p = recordParamsText(r)
  const text = [...head, p.text, ...tail].filter(Boolean).join(' · ')
  return { text, title: p.preset && p.tip ? p.tip : text }
}

/** 删除单条记录的 toast（§四 9）：“已删除 n 条记录。”，有没删成的追加 deleteFailureText */
export function deleteResultText(r: { deletedTaskIds: string[]; failures: { reason: string; message: string; path?: string }[] }): string {
  return [`已删除 ${r.deletedTaskIds.length} 条记录。`, deleteFailureText(r.failures)].filter(Boolean).join('')
}

/**
 * 删除 / 移除后有没删成的（DeleteResult.failures）——文案已定（产品经理 10-08），改文案只改这里：
 * - 有路径的 = 文件没删成：“有 k 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。”，带“打开所在文件夹”（第一个非空路径）；
 * - 路径为空的（still_running：转换 10 秒内没停下来）单独计数：“有 k 条转换没能及时停止，它们的输出文件没有删除，请稍后手动删除。”（产品经理定稿 10-08），不带文件夹操作；
 * - 两种都有：文件句在前，still_running 句在后；“打开所在文件夹”只在有非空路径时出现。页面用警告样式、8 秒。
 */
export const DELETE_FAILED_FILES = (k: number) => `有 ${k} 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。`
export const DELETE_STILL_RUNNING = (k: number) => `有 ${k} 条转换没能及时停止，它们的输出文件没有删除，请稍后手动删除。`
/** 删除失败提示上“打开所在文件夹”失败（契约 v0.23.3）：文件被移走 NOT_FOUND → 固定文案；其他（含超过 10 分钟 / 重启后的 INVALID_ARGUMENT）→ 普通错误提示（错误自己的文案） */
export const REVEAL_DELETE_FAILURE_NOT_FOUND = '找不到这个文件。'
export const revealDeleteFailureText = (e: { code: string; message: string }): string => (e.code === 'NOT_FOUND' ? REVEAL_DELETE_FAILURE_NOT_FOUND : e.message)
export function deleteFailureNotice(failures: readonly { reason: string; path?: string }[]): { text: string; path: string } {
  const files = failures.filter((f) => !!f.path)
  const running = failures.length - files.length
  return { text: [files.length ? DELETE_FAILED_FILES(files.length) : '', running ? DELETE_STILL_RUNNING(running) : ''].join(''), path: files[0]?.path ?? '' }
}
export const deleteFailureText = (failures: readonly { reason: string; path?: string }[]): string => deleteFailureNotice(failures).text

/**
 * 源文件行“从列表移除”后的 toast（定稿 产品经理 10-08）：n = deletedTaskIds.length，m = deletedFiles。
 * 有记录：已从列表移除“名称”和 n 条记录。删了输出：…和 n 条记录，并删除了 m 个文件。没有记录：已从列表移除“名称”。（不出现“0 条记录”）
 * 拆成 before / name / after，页面把名称单独放进可省略、带 title 的 span；改文案只改这里。
 */
export function sourceRemovedParts(name: string, n: number, m: number): { before: string; name: string; after: string } {
  const tail = n > 0 ? `和 ${n} 条记录${m > 0 ? `，并删除了 ${m} 个文件` : ''}。` : '。'
  return { before: '已从列表移除“', name, after: `”${tail}` }
}
export const sourceRemovedText = (name: string, n: number, m: number): string => {
  const p = sourceRemovedParts(name, n, m)
  return p.before + p.name + p.after
}

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

interface InfoLike { width?: number; height?: number; videoCodec?: string; audioCodec?: string; duration?: number; size?: number; sampleRate?: number; channels?: number; hasVideo?: boolean; hasAudio?: boolean }
/** 有画面的按视频显示，其余按音频 */
export const isAudioOnly = (i?: InfoLike): boolean => !!i && (i.hasVideo === false || !i.width)

/** 父行信息：视频「分辨率 · 编码 · 时长 · 大小」（无声视频在编码后加“没有声音”）；音频「采样率 · 声道 · 时长 · 大小」 */
export function sourceMetaText(i: InfoLike): string {
  const parts: string[] = []
  if (!isAudioOnly(i)) {
    parts.push(`${i.width}×${i.height}`, codecName(i.videoCodec))
    if (i.hasAudio === false) parts.push('没有声音')
  } else {
    parts.push(sampleRateText(i.sampleRate), channelText(i.channels))
    if (!i.sampleRate && !i.channels) parts.push(codecName(i.audioCodec))
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
