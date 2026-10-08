/**
 * 转换页 v0.24 的纯函数和文案（契约 v0.24 / v0.24.1 §6.15–6.17；设计说明 §十三、§八 第 35–72 条）。
 * 格式搜索、复制状态、提交跳过提示、重转入口 / 结果提示、存储目录、时长偏短。没有副作用，自检（api/convertV24.check.ts）直接调用。
 */
import { formatBytes } from '@/utils/format'

// ---------------- 格式目录与搜索（§八 第 35–38、46、63 条） ----------------
export type FormatCategory = 'video' | 'audio' | 'image'
export const FORMAT_CATEGORIES: readonly FormatCategory[] = ['video', 'audio', 'image']
export const FORMAT_CATEGORY_LABEL: Record<FormatCategory, string> = { video: '视频', audio: '音频', image: '图片' }
/** 搜索、格式块只用到的字段（FormatEntry 的子集） */
export interface FormatLike {
  category: FormatCategory | (string & {})
  extension: string
  displayName: string
  aliases: readonly string[]
  encodable: boolean
}
export const FORMAT_SEARCH_PLACEHOLDER = '搜索格式，如 MP4、铃声、动图'
export const FORMAT_SEARCH_CLEAR = '清空搜索'
export const formatFoundText = (n: number) => `找到 ${n} 个格式`
export const formatNoneText = (k: string) => `没有找到和“${k}”相关的格式`
export const IMAGE_FROM_VIDEO_NOTE = '视频文件会截取第 1 秒的画面，保存为图片。'
export const M4R_NOTE = 'iPhone 铃声最长 40 秒，更长的文件可能不能设为铃声。'

/** 搜索命中的排序档（越小越靠前）：0 扩展名完全相同 → 1 扩展名开头相同 → 2 显示名命中（含扩展名中间命中）→ 3 别名命中；-1 没命中 */
export function formatRank(f: FormatLike, q: string): number {
  const k = q.trim().toLowerCase()
  if (!k) return -1
  const ext = f.extension.toLowerCase()
  if (ext === k) return 0
  if (ext.startsWith(k)) return 1
  if (ext.includes(k) || f.displayName.toLowerCase().includes(k)) return 2
  if (f.aliases.some((a) => a.toLowerCase().includes(k))) return 3
  return -1
}
/** 格式块第二行：平时是 aliases[0]；搜索时只靠别名命中，换成命中的那个别名（如 M4R 搜“苹果”显示“苹果铃声”） */
export function formatSubText(f: FormatLike, q = ''): string {
  const k = q.trim().toLowerCase()
  const first = f.aliases[0] ?? ''
  if (!k || formatRank(f, k) !== 3 || first.toLowerCase().includes(k)) return first
  return f.aliases.find((a) => a.toLowerCase().includes(k)) ?? first
}
export interface FormatHit<T extends FormatLike> { entry: T; rank: number; sub: string }
export interface FormatHitGroup<T extends FormatLike> { category: FormatCategory; label: string; items: FormatHit<T>[] }
/**
 * 前端搜索（不加延迟、不区分大小写、子串匹配扩展名 / 显示名 / 别名）：按分类分组（视频 / 音频 / 图片），组内按排序档，同档按目录顺序。
 * 不可输出的格式也在结果里（置灰）。空关键字返回 []。
 */
export function searchFormats<T extends FormatLike>(catalog: readonly T[], q: string): FormatHitGroup<T>[] {
  const k = q.trim()
  if (!k) return []
  const hits = catalog.map((entry, i) => ({ entry, rank: formatRank(entry, k), sub: formatSubText(entry, k), i })).filter((h) => h.rank >= 0)
  return FORMAT_CATEGORIES.map((c) => ({
    category: c,
    label: FORMAT_CATEGORY_LABEL[c],
    items: hits.filter((h) => h.entry.category === c).sort((a, b) => a.rank - b.rank || a.i - b.i).map(({ entry, rank, sub }) => ({ entry, rank, sub })),
  })).filter((g) => g.items.length)
}
export const formatHitCount = (groups: readonly { items: readonly unknown[] }[]): number => groups.reduce((n, g) => n + g.items.length, 0)
/** Enter：选中第一个可用（encodable）的结果（按显示顺序）；没有返回 undefined */
export function firstUsableHit<T extends FormatLike>(groups: readonly FormatHitGroup<T>[]): T | undefined {
  for (const g of groups) for (const h of g.items) if (h.entry.encodable) return h.entry
  return undefined
}
/** 命中的字用主色浅底高亮：把文字切成 [普通, 命中, 普通]（只高亮第一处，不区分大小写） */
export function highlightParts(text: string, q: string): { t: string; hit: boolean }[] {
  const k = q.trim().toLowerCase()
  const i = k ? text.toLowerCase().indexOf(k) : -1
  if (i < 0) return [{ t: text, hit: false }]
  return [{ t: text.slice(0, i), hit: false }, { t: text.slice(i, i + k.length), hit: true }, { t: text.slice(i + k.length), hit: false }].filter((p) => p.t)
}
/** 格式块的 title：可输出时“MP4（通用视频、手机视频、MPEG-4）”，不可输出时是后端的 reason */
export function formatTileTitle(f: FormatLike & { reason?: string }): string {
  if (!f.encodable) return f.reason || '当前转换组件不支持输出这个格式'
  return f.aliases.length ? `${f.displayName}（${f.aliases.join('、')}）` : f.displayName
}

// ---------------- 复制状态（§八 第 39–41、44、53 条） ----------------
export type CopyState = 'none' | 'copying' | 'ready' | 'failed' | 'canceled'
export const COPY_CANCEL_LABEL = '取消复制'
export const COPY_RUNNING_NOTE = '正在复制到程序的上传文件夹，复制完成后才能转换。'
export const COPY_PREVIEW_TIP_RUNNING = '复制完成后才能预览'
export const COPY_PREVIEW_TIP_FAILED = '复制失败，无法预览'
export const COPY_PREVIEW_TIP_CANCELED = '已取消复制，无法预览'
export const COPY_CHECK_TIP_FAILED = '复制失败，不能转换'
export const COPY_CHECK_TIP_CANCELED = '已取消复制，不能转换'
export const COPY_GO_TIP = '文件复制完成后才能转换'
export const COPY_GO_HINT_ALL = '选中的文件还在复制'
export const copySkipHint = (k: number) => `将跳过 ${k} 个还在复制的文件`
export const mixedSkipHint = (k: number) => `将跳过 ${k} 个文件`
/** 复制中的进度文字“45% · 2.9 GB / 6.4 GB”；copiedBytes=0 是“等待复制”，不显示百分比（返回 ''） */
export function copyProgressText(copied: number, total: number): string {
  if (!copied || !total) return ''
  return `${copyPct(copied, total)}% · ${formatBytes(copied)} / ${formatBytes(total)}`
}
export const copyPct = (copied: number, total: number): number => (total > 0 ? Math.min(100, Math.max(0, Math.floor((copied / total) * 100))) : 0)
/** 复制状态的标签（ready / none 没有标签） */
export function copyTag(state: CopyState | undefined, copied = 0): { cls: string; text: string; icon: boolean } | null {
  if (state === 'copying') return copied > 0 ? { cls: 't-run t-cp', text: '复制中', icon: false } : { cls: 't-q t-wait', text: '等待复制', icon: false }
  if (state === 'failed') return { cls: 't-warn', text: '复制失败', icon: true }
  if (state === 'canceled') return { cls: 't-cx', text: '已取消复制', icon: false }
  return null
}
/** 复制失败的错误条要不要“打开存储设置”：CONVERT_DISK_FULL 或 detail 首行 reason=no_space */
export const isNoSpace = (e?: { code?: string; detail?: string } | null): boolean => !!e && (e.code === 'CONVERT_DISK_FULL' || /^reason=no_space/.test(e.detail ?? ''))
/** 第三行原路径：文件夹部分可以省略，文件名保留 */
export function splitPathTail(p: string): { head: string; tail: string } {
  const i = Math.max(p.lastIndexOf('\\'), p.lastIndexOf('/'))
  return { head: p.slice(0, i + 1), tail: p.slice(i + 1) }
}
/** “保存到”和设置页的路径框：省略中间、保留最后两段（如 D:\Progra…\FFmpegFree\output） */
export function splitPathLastTwo(p: string): { head: string; tail: string } {
  const seps = [...p].map((c, i) => (c === '\\' || c === '/' ? i : -1)).filter((i) => i > 0)
  if (seps.length < 2) return { head: '', tail: p }
  const cut = seps[seps.length - 2]
  return { head: p.slice(0, cut), tail: p.slice(cut) }
}
/** 源文件行悬停：原文件 / 复制件两行 + 说明（旧行只有原文件一行） */
export function sourcePathTip(s: { originalPath?: string; path: string; storedPath?: string; copyState?: CopyState }): { rows: { label: string; path: string }[]; note: string } {
  const orig = s.originalPath || s.path
  if (!s.copyState || s.copyState === 'none' || !s.storedPath) return { rows: [{ label: '原文件', path: orig }], note: '这个文件是旧版本添加的，直接读取原文件。' }
  const suffix = s.copyState === 'copying' ? '（复制中）' : s.copyState === 'failed' ? '（复制失败）' : s.copyState === 'canceled' ? '（已取消复制）' : ''
  return { rows: [{ label: '原文件', path: orig }, { label: '复制件', path: s.storedPath + suffix }], note: '转换和预览读取复制件。' }
}

// ---------------- 提交（§八 第 41、53、69 条） ----------------
export type SkipReason = 'copying' | 'copy_failed' | 'copy_canceled'
/** 部分跳过时提交成功后的提示（普通提示 4 秒）；没有跳过返回 ''（不提示） */
export function submitSkipToast(started: number, skipped: readonly { reason: string }[]): string {
  if (!skipped.length) return ''
  const copying = skipped.filter((s) => s.reason === 'copying').length
  const failed = skipped.length - copying
  return [`已开始转换 ${started} 个文件。`, copying ? `有 ${copying} 个文件还在复制，已跳过，复制完成后再转换。` : '', failed ? `有 ${failed} 个文件没有复制成功，已跳过。` : ''].join('')
}
/** 全部没复制好、SubmitSources 报错（TASK_CONFLICT reason=copy_not_ready）时的提示，勾选不变 */
export const SUBMIT_NOT_READY_TOAST = '选中的文件还没有复制完成，复制完成后才能转换。'
/** Preview 收到 TASK_CONFLICT（复制中）的兜底提示（后端 10-08 给的） */
export const PREVIEW_COPYING_TOAST = '文件还在复制，复制完成后才能预览'
export const COPY_RETRY_LABEL = '重试复制'
export const OPEN_STORAGE_SETTINGS = '打开存储设置'
/** AddSources 里 UNSUPPORTED reason=format 的项汇总成一句（§八 第 68 条） */
export const addRejectedText = (k: number) => `有 ${k} 个文件不是支持的格式，没有添加。`
export const isUnsupportedFormat = (e?: { code?: string; detail?: string } | null): boolean => !!e && e.code === 'UNSUPPORTED' && /^reason=format/.test(e.detail ?? '')

// ---------------- 重转（§八 第 58、59、64、65、67、70、71 条） ----------------
export type ReconvertMode = 'replace' | 'regenerate' | ''
export type ReconvertBlock = 'invalid_state' | 'output_moved' | 'source_missing' | 'copy_not_ready' | (string & {})
export const RECONVERT_MENU = '重转…'
export const RECONVERT_CANCEL = '取消重转'
export const RECONVERT_TITLE = '重转并覆盖原来的结果？'
export const RECONVERT_FORMAT_NOTE = '格式不能更改，要换格式请新转一条。'
export const RECONVERT_BLOCK_TIP: Record<string, string> = {
  source_missing: '源文件已不存在，不能重转',
  output_moved: '原来的位置已经有别的文件，不能重转',
  // 设计说明没有写这两种（菜单里一般遇不到：重转只在完成的记录上出现；复制没就绪的行很少有完成的记录），先用这两句，见交付说明
  copy_not_ready: '文件复制完成后才能重转',
  invalid_state: '这条记录现在不能重转',
}
/**
 * “重转…”菜单项的状态：按 TaskPathCheck.reconvertMode / reconvertBlock（v0.24.1）。源文件和输出同时有问题时后端优先报 source_missing，
 * 前端自己已经知道源文件不在（CheckSources / 打开失败）时也优先报源文件。没有检查结果（还没查 / 查失败）时按本地知道的情况推断。
 */
export function reconvertState(check: { reconvertMode?: string; reconvertBlock?: string } | undefined, local: { sourceGone: boolean; outputGone: boolean }): { mode: ReconvertMode; tip: string } {
  if (local.sourceGone) return { mode: '', tip: RECONVERT_BLOCK_TIP.source_missing }
  if (check && typeof check.reconvertMode === 'string') {
    const m = check.reconvertMode
    if (m === 'replace' || m === 'regenerate') return { mode: m, tip: '' }
    const b = check.reconvertBlock || 'invalid_state'
    return { mode: '', tip: RECONVERT_BLOCK_TIP[b] ?? RECONVERT_BLOCK_TIP.invalid_state }
  }
  return { mode: local.outputGone ? 'regenerate' : 'replace', tip: '' }
}
export const reconvertBody = (mode: 'replace' | 'regenerate', name: string) =>
  mode === 'regenerate' ? `原来的文件已经不在了，会按原来的参数重新生成“${name}”。` : `完成后会替换“${name}”，文件名不变。转换失败或取消时，原来的文件保持不变。`
export const reconvertKeepLabel = (current: string) => `沿用原来的参数（${current}）`
export const reconvertDoneToast = (name: string) => `已重转“${name}”。`
/** 重转失败的提示（警告色 8 秒）：重新生成失败“重转失败，请稍后重试。”；覆盖失败按 detail 的 reason=in_use 多一句 */
export function reconvertFailToast(mode: 'replace' | 'regenerate', err?: { detail?: string } | null): string {
  if (mode === 'regenerate') return '重转失败，请稍后重试。'
  return /^reason=in_use/.test(err?.detail ?? '') ? '重转失败，原来的文件没有变动。文件可能正在被其他程序使用，请关闭后再重转。' : '重转失败，原来的文件没有变动。'
}
/**
 * Reconvert 的同步错误给用户看的文字（产品 10-08：凡是重转都说“重转”，“重新转换”只留给失败 / 已取消记录的按钮）。
 * 按 reason 用前端文案，不直接显示后端 message（后端 6.17.1 的 format_change 文案写的是“重新转换”）。
 */
export function reconvertErrorText(e: { code: string; message: string; detail?: string }): string {
  const reason = /^reason=(\w+)/.exec(e.detail ?? '')?.[1] ?? ''
  if (e.code === 'TASK_CONFLICT' && reason === 'invalid_state') return '这条记录正在重转，或者现在不能重转'
  if (e.code === 'TASK_CONFLICT' && reason === 'output_moved') return RECONVERT_BLOCK_TIP.output_moved
  if (e.code === 'TASK_CONFLICT' && (reason === 'copying' || reason === 'copy_failed')) return RECONVERT_BLOCK_TIP.copy_not_ready
  if (e.code === 'NOT_FOUND' && reason === 'file') return RECONVERT_BLOCK_TIP.source_missing
  if (e.code === 'NOT_FOUND' && reason === 'record') return '这条记录已被删除，不能重转'
  if (e.code === 'INVALID_ARGUMENT' && reason === 'format_change') return '重转不能更换格式，要换格式请新转一条。'
  if (e.code === 'UNSUPPORTED') return '当前转换组件不支持输出这个格式，不能重转'
  return e.message || '重转失败，请稍后重试。'
}
/** 启动时 TakeInterruptedReconverts 的提示（普通 4 秒，只提示一次）；n ≤ 0 不提示 */
export const interruptedReconvertsText = (n: number): string => (n > 0 ? `上次退出时有 ${n} 条重转被中断，原来的文件没有变动。` : '')

// ---------------- 时长偏短（§八 第 55 条） ----------------
export const SHORT_OUTPUT = 'short_output'
export const SHORT_TAG = '时长偏短'
export const SHORT_TIP = '转换结果比原文件短很多，原文件可能已损坏，请预览检查。'
export const hasShortOutput = (r?: { warnings?: readonly string[] } | null): boolean => !!r?.warnings?.includes(SHORT_OUTPUT)

// ---------------- 存储（§八 第 42、43、45 条） ----------------
export const FALLBACK_BANNER = '程序所在文件夹无法写入，文件已改存到：'
export const FALLBACK_SAVE_HINT = '已改存到用户数据目录'
export const FALLBACK_SETTINGS_NOTE = '程序所在文件夹无法写入，已改存到用户数据目录。'
export const STORAGE_CHANGED_TOAST = '已更改，之后的新文件会保存到这里。'
export const STORAGE_FOOT_NOTE = '修改后只对之后添加或转换的文件生效。已有的文件不会搬动，原来的记录仍指向原来的位置。'
/** 改存横条：fellBack 且不是两个目录都自定义（6.15.1 第 6 条） */
export const showFallbackBanner = (d: { fellBack: boolean; outputCustom: boolean; uploadsCustom: boolean } | null | undefined): boolean => !!d && d.fellBack && !(d.outputCustom && d.uploadsCustom)

// ---------------- 从列表移除（X6，§八 第 49、72 条） ----------------
export const SOURCE_REMOVE_TITLE_EMPTY = '从列表移除这个文件'
/** 没有记录的行：按 copyState（v0.24.1，架构师 10-08）——none（旧行）只从列表移除；其余（copying / ready / failed / canceled）提到复制件 */
export const sourceRemoveEmptyBody = (copyState: CopyState | undefined): { text: string; bold: string } =>
  !copyState || copyState === 'none' ? { text: '只从列表移除，', bold: '不删除原文件' } : { text: '只删除程序里的复制件，', bold: '不删除原文件' }
