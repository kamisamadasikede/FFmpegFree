/**
 * DocService 接口层（契约 v0.12 / 6.12，后端 #29 已合入，绑定在 wailsjs/go/app/DocService）。
 *
 * DOC_BACKEND_READY（api/flags.ts）= true：在 Wails 里直接调用生成的 DocService 绑定（经 call() 解析 AppError）；
 * 纯浏览器（没有 window.go）保留本地模拟（内存里的最近 PDF 列表 + 假 PDF 字节 + api/sim.ts 的定时器转换任务），isDocSim() 为 true，页面据此显示“演示”提示。
 * 选文件用 SystemService.PickFiles（api/system.ts）；取消转换 = TaskService.Cancel，重试 = TaskService.Retry（office_pdf 注册了重试工厂）。
 * DocService 任何方法都不返回 FFMPEG_NOT_FOUND / PROBE_FAILED / PROCESS_FAILED，不依赖 ffmpeg，始终可用。
 *
 * 契约要点：
 * - 支持 docx / xlsx / pptx（仅文本，不保留图片和样式）；doc xls ppt odt ods odp rtf csv txt pages numbers key 及加密文档 → UNSUPPORTED；超过 5000 页 → UNSUPPORTED。
 * - ConvertToPDF 先整体校验再提交，任何一个不通过整体失败、不提交任何任务，detail 第一行是出错文件路径。
 * - 预览：OpenPDF 登记句柄 → ReadPDFChunk 分块（≤ 1 MiB 原始字节）读整份（≤ 64 MiB，主路径）；更大的文件用 PDFSource.url（/local/<token>）走 Range，
 *   用前先 HEAD 探测，404 → 重新 OpenPDF 换 URL，只重试一次。
 * - PDFChunk.data 是 Go string（标准 base64，含 = 填充），直接 atob；length / chunkBytes / size 都是原始字节数，不是 data 的字符数（1 MiB 约编码成 1.4 MiB）。
 * - ListRecentPDFs(limit)：limit ≤ 0 取默认 20，> 200 后端静默截到 200；这里前端也按 200 处理，不传更大的值。
 * - 转换进度走 task:created / task:progress / task:status，刷新后由任务 store 的 ListActive 接回（页面不自己订阅）。
 */
import * as DocBinding from '../../wailsjs/go/app/DocService'
import * as TaskBinding from '../../wailsjs/go/app/TaskService'
import { store as goStore } from '../../wailsjs/go/models'
import { AppError, call, toAppError } from '@/api/call'
import { DOC_BACKEND_READY } from '@/api/flags'
import { createSimTask, injectionDetail, listSimFinished, simDelay, simError, simInjection, simParam } from '@/api/sim'
import { toApiTask, type ApiTask } from '@/api/taskTypes'
import { hasWailsBackend } from '@/services/wails'

export { DOC_BACKEND_READY }

/** 走真实绑定：开关打开且在 Wails 里 */
const live = () => DOC_BACKEND_READY && hasWailsBackend()
/** 当前是浏览器里的本地模拟（页面显示“演示”提示的条件） */
export const isDocSim = (): boolean => !live()

// ───────────── 契约类型（§6.12.2，字段一一对应）─────────────

export interface DocFormat {
  /** 不带点，小写 */
  ext: string
  supported: boolean
  /** "text-only"（支持的三种）| "" */
  fidelity: string
  /** 不支持时的原因 */
  reason: string
}

export interface DocFont {
  /** 是否找到可用字体（内嵌 Noto Sans SC 子集为主，系统 .ttf 补充） */
  available: boolean
  /** noto-sans-sc-embedded / simhei / simsun / arialunicode / dejavu / "" */
  name: string
  /** 主用字体是否覆盖 GB2312 汉字（内嵌字体为 true） */
  cjk: boolean
}

export interface DocLimits {
  /** 50 */
  maxInputsPerSubmit: number
  /** 100 MiB */
  maxInputBytes: number
  /** 5000 */
  maxPages: number
  /** 512 MiB（OpenPDF） */
  maxPdfBytes: number
  /** 1 MiB（ReadPDFChunk 上限） */
  chunkBytes: number
  /** 64 MiB（前端整份读入内存的上限） */
  wholeLoadBytes: number
}

export interface DocCapabilities {
  formats: DocFormat[]
  font: DocFont
  limits: DocLimits
  /**
   * 该功能是否标“实验性”。**契约 v0.12 还没有这个字段**（架构师说明后端会给出）：字段名 / 位置待后端确认。
   * 缺省（undefined，老后端）时按 true 处理：契约规定 Office 转 PDF 首版一律标“实验性”。
   */
  experimental?: boolean
}

export interface PDFSource {
  /** 句柄（128 位随机，进程内有效，重启失效）；同一路径复用同一 id */
  id: string
  path: string
  name: string
  size: number
  /** /local/<token>，仅在 size > limits.wholeLoadBytes 时前端使用；否则不要用 */
  url: string
}

export interface PDFChunk {
  offset: number
  /** 实际读到的原始字节数（不是 data 的字符数） */
  length: number
  /** offset+length >= 文件当前大小 */
  eof: boolean
  /** 读取时文件的当前大小（原始字节）；与 OpenPDF 的 size 不一致说明读取期间文件被改动 */
  size: number
  /** Go string，标准 base64（含 = 填充）；模拟层也返回 base64，统一走 decodeChunk */
  data: string
}

export interface PDFFile {
  /** doc_recent.id（ULID） */
  id: string
  path: string
  name: string
  size: number
  /** Unix 毫秒 */
  openedAt: number
  /** 列表时 stat 的结果，文件已删为 false（记录保留，用户手动移除） */
  exists: boolean
}

export const OFFICE_EXTS = ['docx', 'xlsx', 'pptx'] as const
const KIB = 1024
const MIB = 1024 * KIB

export const DEFAULT_DOC_LIMITS: DocLimits = {
  maxInputsPerSubmit: 50, maxInputBytes: 100 * MIB, maxPages: 5000, maxPdfBytes: 512 * MIB, chunkBytes: 1 * MIB, wholeLoadBytes: 64 * MIB,
}

const SIM_FORMATS: DocFormat[] = [
  { ext: 'docx', supported: true, fidelity: 'text-only', reason: '' },
  { ext: 'xlsx', supported: true, fidelity: 'text-only', reason: '' },
  { ext: 'pptx', supported: true, fidelity: 'text-only', reason: '' },
  ...['doc', 'xls', 'ppt'].map((ext): DocFormat => ({ ext, supported: false, fidelity: '', reason: '旧版二进制格式，请先另存为 docx / xlsx / pptx' })),
  ...['odt', 'ods', 'odp', 'rtf'].map((ext): DocFormat => ({ ext, supported: false, fidelity: '', reason: '暂不支持该格式' })),
]

const isAbs = (p: string) => /^([A-Za-z]:[\\/]|\/|\\\\)/.test(p)
const extOf = (p: string) => (p.split('.').pop() ?? '').toLowerCase()
const baseName = (p: string) => p.split(/[\\/]/).pop() || p
const stem = (p: string) => baseName(p).replace(/\.[^.]*$/, '')

// ───────────── 服务方法 ─────────────

/** 支持的格式、字体状态、限额；不依赖 ffmpeg，随时可调 */
export async function getDocCapabilities(): Promise<DocCapabilities> {
  if (live()) return await call(DocBinding.GetDocCapabilities())
  return { formats: SIM_FORMATS, font: { available: true, name: 'noto-sans-sc-embedded', cjk: true }, limits: DEFAULT_DOC_LIMITS, experimental: true }
}

/** “实验性”标签是否显示：优先按后端给的 experimental，缺省 true */
export function isExperimental(caps: DocCapabilities | null | undefined): boolean {
  return caps?.experimental ?? true
}

/**
 * 批量转换，一个文件一个 office_pdf 任务，返回顺序与 inputs 一致。先整体校验再提交：任何一个不通过整体失败、不提交任何任务，detail 第一行是出错文件路径。
 * 模拟触发（预览参数，见 README）：文件名 `加密…` / `旧…`、扩展名 doc/xls/ppt/csv/txt/odt/rtf → UNSUPPORTED；`损坏…` → INVALID_ARGUMENT；`缺失…` → NOT_FOUND；
 * `?sim_err=CONVERT_DISK_FULL|IO_ERROR|INTERNAL|UNSUPPORTED` 让任务中途失败；其他 ?sim_err=<码> 让 ConvertToPDF 同步抛出。
 */
export async function convertToPDF(inputs: string[], outputDir: string): Promise<ApiTask[]> {
  if (live()) return ((await call(DocBinding.ConvertToPDF(inputs, outputDir))) ?? []).map(toApiTask)
  await simDelay(150)
  const inj = simInjection()
  // 这些码默认让任务在转换中途失败；其余的码（含 sim_when=call）由 ConvertToPDF 同步抛出
  const TASK_CODES = ['CONVERT_DISK_FULL', 'IO_ERROR', 'INTERNAL', 'UNSUPPORTED']
  const asTask = !!inj && (inj.when === 'task' || TASK_CODES.includes(inj.code)) && !(inj.when === 'call' && simParam('sim_when') === 'call')
  if (inj && !asTask) simError(inj.code, '模拟错误', injectionDetail(inj))
  if (inputs.length === 0 || inputs.length > DEFAULT_DOC_LIMITS.maxInputsPerSubmit) simError('INVALID_ARGUMENT', `一次需要 1~${DEFAULT_DOC_LIMITS.maxInputsPerSubmit} 个文件`)
  if (outputDir && !isAbs(outputDir)) simError('INVALID_ARGUMENT', 'outputDir 必须是绝对路径')
  for (const p of inputs) {
    const first = `${p}\n`
    if (!isAbs(p)) simError('INVALID_ARGUMENT', '路径必须是绝对路径', `${first}路径必须是绝对路径`)
    const name = baseName(p)
    if (name.startsWith('缺失') || name.startsWith('missing')) simError('NOT_FOUND', '文件不存在', `${first}文件不存在`)
    if (!name.includes('.')) simError('INVALID_ARGUMENT', '这是文件夹', `${first}是目录，不是文件`)
    const ext = extOf(p)
    if (name.startsWith('加密') || name.startsWith('encrypted')) simError('UNSUPPORTED', '加密文档不支持', `${first}加密文档不支持`)
    if (!(OFFICE_EXTS as readonly string[]).includes(ext)) {
      const legacy = ['doc', 'xls', 'ppt'].includes(ext)
      simError('UNSUPPORTED', '不支持这种格式', `${first}${legacy ? '旧版 Office 格式，请先另存为 docx / xlsx / pptx' : ext === 'csv' || ext === 'txt' ? '暂不支持该格式' : '不支持的格式：' + ext}`)
    }
    if (name.startsWith('损坏') || name.startsWith('broken')) simError('INVALID_ARGUMENT', '不是有效的 OOXML 文件', `${first}不是有效的 OOXML 文件`)
    if (name.startsWith('超大') || name.startsWith('huge')) simError('INVALID_ARGUMENT', '文件超过 100 MiB', `${first}文件超过 100 MiB`)
  }
  const dir = outputDir || '/Users/me/Documents'
  return inputs.map((p, i) => {
    const fail = asTask ? inj : null
    return createSimTask({
      type: 'office_pdf',
      title: `${baseName(p)} → PDF`,
      inputPaths: [p],
      outputPath: `${dir}/${stem(p)}.pdf`,
      params: JSON.stringify({ input: p, outputDir: dir }),
      simSeconds: 4 + i,
      fail: fail ? { code: fail.code, message: '模拟错误', detail: injectionDetail(fail), atProgress: 0.5 } : undefined,
    })
  })
}

// ---- PDF 预览：OpenPDF → ReadPDFChunk ----
const simRecent = new Map<string, PDFFile>()
const simHandles = new Map<string, PDFSource>()
let simSeq = 0

/** 模拟用的最小 PDF 字节（每页一行“演示 PDF”文字；pdf.js 能重建缺失的 xref，所以不写 xref 表）。pages 缺省 1 页 */
function simPdfBytes(size: number, pages = 1): Uint8Array {
  const kids = Array.from({ length: pages }, (_, i) => `${6 + i * 2} 0 R`).join(' ')
  let body = `%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[${kids}]/Count ${pages}>>endobj\n5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n`
  for (let i = 0; i < pages; i++) {
    const content = `BT /F1 28 Tf 72 720 Td (Demo PDF - simulated data - page ${i + 1}) Tj ET`
    body += `${6 + i * 2} 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]/Contents ${7 + i * 2} 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n`
    body += `${7 + i * 2} 0 obj<</Length ${content.length}>>stream\n${content}\nendstream endobj\n`
  }
  body += `trailer<</Root 1 0 R>>\n%%EOF\n`
  const head = new TextEncoder().encode(body)
  const out = new Uint8Array(Math.max(size, head.length))
  out.set(head)
  return out
}
/** 模拟文件的页数：文件名含“多页”= 12 页，其余 1 页；名字以“解析失败”开头的返回一份不是 PDF 的字节（让 pdf.js 报解析失败） */
const simPages = (name: string) => (name.includes('多页') ? 12 : 1)

/** 校验并登记一个 PDF，返回句柄；同时写入最近列表（OpenPDF 是唯一的写入点）。转换产出的 PDF 不自动进历史，需要预览时对 outputPath 调用它 */
export async function openPDF(path: string): Promise<PDFSource> {
  if (live()) return await call(DocBinding.OpenPDF(path))
  await simDelay(80)
  const inj = simInjection()
  if (inj && inj.when === 'call') simError(inj.code, '模拟错误', injectionDetail(inj))
  if (!isAbs(path)) simError('INVALID_ARGUMENT', '路径必须是绝对路径')
  const name = baseName(path)
  if (name.startsWith('缺失') || name.startsWith('missing')) simError('NOT_FOUND', '文件不存在')
  if (name.startsWith('无权限')) simError('IO_ERROR', '读取文件失败')
  if (name.startsWith('被修改')) simError('IO_ERROR', '文件在读取时被替换，请重试', path)
  if (extOf(path) !== 'pdf' || name.startsWith('非pdf')) simError('INVALID_ARGUMENT', '不是 PDF 文件', '不是 PDF 文件')
  const size = name.startsWith('超大') ? 600 * MIB : name.startsWith('大文件') ? 100 * MIB : 2 * MIB
  if (size > DEFAULT_DOC_LIMITS.maxPdfBytes) simError('INVALID_ARGUMENT', '文件超过 512 MiB', '文件超过 512 MiB')
  const existing = [...simHandles.values()].find((h) => h.path === path)
  const src: PDFSource = existing ?? { id: `sim-pdf-${(++simSeq).toString(36)}${Math.random().toString(36).slice(2, 8)}`, path, name, size, url: `/local/sim${simSeq.toString(36)}` }
  simHandles.set(src.id, src)
  const rec = [...simRecent.values()].find((r) => r.path === path)
  simRecent.set(rec?.id ?? `sim-recent-${simSeq}`, { id: rec?.id ?? `sim-recent-${simSeq}`, path, name, size, openedAt: Date.now(), exists: true })
  return src
}

/** 按句柄分块读 PDF 字节。length 1~1 MiB（越界 INVALID_ARGUMENT），offset 不能为负；offset ≥ 文件大小返回 length=0, eof=true；句柄不存在（重启后失效）NOT_FOUND */
export async function readPDFChunk(id: string, offset: number, length: number): Promise<PDFChunk> {
  if (live()) return await call(DocBinding.ReadPDFChunk(id, offset, length))
  if (length < 1 || length > DEFAULT_DOC_LIMITS.chunkBytes) simError('INVALID_ARGUMENT', `length 需要在 1~${DEFAULT_DOC_LIMITS.chunkBytes} 之间`)
  if (offset < 0) simError('INVALID_ARGUMENT', 'offset 不能为负')
  const h = simHandles.get(id)
  if (!h) return simError('NOT_FOUND', '句柄不存在，请重新打开')
  if (offset >= h.size) return { offset, length: 0, eof: true, size: h.size, data: '' }
  const n = Math.min(length, h.size - offset)
  const raw = h.name.startsWith('解析失败') ? new Uint8Array(h.size).fill(65) : simPdfBytes(h.size, simPages(h.name))
  const bytes = raw.subarray(offset, offset + n)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return { offset, length: n, eof: offset + n >= h.size, size: h.size, data: btoa(bin) }
}

/** base64（Go string，标准字母表、含 = 填充）→ Uint8Array。直接 atob，解出的字节数必须等于 chunk.length（原始字节数），不等抛 INTERNAL */
export function decodeChunk(chunk: PDFChunk): Uint8Array {
  const bin = atob(chunk.data || '')
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  if (out.length !== chunk.length) throw new AppError('INTERNAL', '分块长度不一致')
  return out
}

/**
 * 主路径：循环 ReadPDFChunk 直到 eof，拼成 Uint8Array 交给 pdf.js（size ≤ wholeLoadBytes）。
 * size 超过 wholeLoadBytes 时抛 UNSUPPORTED，调用方改用 src.url 走 Range（见 loadPDF）。
 * 读取期间文件大小变了（chunk.size !== src.size）抛 IO_ERROR，由 loadPDF 重新 OpenPDF 只重试一次。
 * onProgress(已读, 总大小)，都是原始字节。
 */
export async function readWholePDF(src: PDFSource, limits: DocLimits = DEFAULT_DOC_LIMITS, onProgress?: (read: number, total: number) => void, signal?: AbortSignal): Promise<Uint8Array> {
  if (src.size > limits.wholeLoadBytes) throw new AppError('UNSUPPORTED', '文件较大，需要按需加载')
  // 每块最多 1 MiB 原始字节（base64 后约 1.4 MiB）
  const chunkLen = Math.max(1, Math.min(limits.chunkBytes, MIB))
  const buf = new Uint8Array(src.size)
  let off = 0
  while (off < src.size) {
    if (signal?.aborted) throw new AppError('CANCELED', '已取消')
    const c = await readPDFChunk(src.id, off, chunkLen)
    if (c.size !== src.size) throw new AppError('IO_ERROR', 'PDF 在读取时被修改')
    const bytes = decodeChunk(c)
    if (off + bytes.length > buf.length) throw new AppError('IO_ERROR', 'PDF 在读取时被修改')
    buf.set(bytes, off)
    off += bytes.length
    onProgress?.(off, src.size)
    if (c.eof) break
    if (bytes.length === 0) throw new AppError('INTERNAL', '读到 0 字节但未结束')
  }
  if (off !== src.size) throw new AppError('IO_ERROR', 'PDF 读取不完整')
  return buf
}

/** loadPDF 的结果：小文件是整份字节（data），大文件是可 Range 加载的 /local/<token> 地址（url） */
export interface LoadedPDF {
  src: PDFSource
  data?: Uint8Array
  url?: string
}

/** HEAD 探测 /local/<token>：200 / 206 可用；404（token 失效、被撤销、重启后）返回 false */
async function urlAlive(url: string): Promise<boolean> {
  try {
    const r = await fetch(url, { method: 'HEAD' })
    return r.status === 200 || r.status === 206
  } catch {
    return false
  }
}

/**
 * 打开并读取一个 PDF（契约 6.12.4 的读取流程）：
 * - size ≤ wholeLoadBytes（64 MiB）：ReadPDFChunk 读整份；句柄失效（NOT_FOUND）或读取期间文件被改动，重新 OpenPDF 只重试一次；
 * - 更大：返回 src.url 给 pdf.js 按 Range 加载；先 HEAD 探测，404 → 重新 OpenPDF 换 URL，只重试一次，仍失败抛 NOT_FOUND。
 * 模拟层不能服务 /local/，大文件在模拟里抛 UNSUPPORTED。
 */
export async function loadPDF(path: string, onProgress?: (read: number, total: number) => void, signal?: AbortSignal): Promise<LoadedPDF> {
  const caps = await getDocCapabilities().catch(() => null)
  const limits = caps?.limits ?? DEFAULT_DOC_LIMITS
  for (let attempt = 0; ; attempt++) {
    const src = await openPDF(path)
    try {
      if (src.size > limits.wholeLoadBytes) {
        if (!live()) throw new AppError('UNSUPPORTED', '演示环境无法预览大文件')
        if (src.url && (await urlAlive(src.url))) return { src, url: src.url }
        throw new AppError('NOT_FOUND', '文件不存在或已被移动')
      }
      return { src, data: await readWholePDF(src, limits, onProgress, signal) }
    } catch (e) {
      const err = toAppError(e)
      const retryable = err.code === 'NOT_FOUND' || (err.code === 'IO_ERROR' && err.message.includes('被修改'))
      if (attempt === 0 && retryable) continue
      throw err
    }
  }
}

/** 最近打开的 PDF，按 openedAt 倒序。limit 默认 20（0 = 默认），最大 200（更大的值按 200 处理） */
export const MAX_RECENT_LIMIT = 200
export async function listRecentPDFs(limit = 0): Promise<PDFFile[]> {
  // 契约：limit ≤ 0 取默认 20，> 200 后端静默截到 200（不报错）；前端不传超过 200 的值
  const n = Math.min(Math.max(0, Math.trunc(limit) || 0), MAX_RECENT_LIMIT)
  if (live()) return (await call(DocBinding.ListRecentPDFs(n))) ?? []
  const mode = simParam('sim_recent') // 仅模拟环境的预览参数：sample = 8 条示例（含一条已被移动），full = 200 条，none = 空
  if (mode === 'none') return []
  if (mode === 'sample' || mode === 'full') return simRecentSample(mode === 'full' ? 200 : 8).slice(0, n || 20)
  return [...simRecent.values()].sort((a, b) => b.openedAt - a.openedAt).slice(0, n || 20)
}

function simRecentSample(count: number): PDFFile[] {
  const d = '/Users/me/Documents/'
  const names = ['2026 Q3 产品回顾.pdf', '用户调研报告.pdf', '渠道数据汇总.pdf', '2026年第三季度华东区域渠道商务拓展与用户增长复盘汇报材料（终稿-已审阅-v12）.pdf', '合同扫描件-乙方留存.pdf', '培训手册.pdf', '会议纪要.pdf', '费用明细.pdf']
  const sizes = [86 * KIB, 1.4 * MIB, 620 * KIB, 12.4 * MIB, 3.2 * MIB, 5.1 * MIB, 210 * KIB, 940 * KIB]
  const now = new Date()
  const at = (dayOffset: number, h: number, m: number) => new Date(now.getFullYear(), now.getMonth(), now.getDate() + dayOffset, h, m).getTime()
  const times = [at(0, 22, 41), at(0, 21, 5), at(0, 19, 30), at(-1, 18, 5), at(-1, 10, 12), at(-3, 16, 40), at(-4, 9, 8), at(-6, 14, 2)]
  return Array.from({ length: count }, (_, i) => {
    const j = i % names.length
    const name = i < names.length ? names[j] : `批量样例-${i + 1}.pdf`
    return { id: `sim-sample-${i}`, path: d + name, name, size: Math.round(sizes[j]), openedAt: i < times.length ? times[i] : times[times.length - 1] - i * 3600_000, exists: !name.startsWith('合同扫描件') }
  })
}

/** 只删记录，不删文件；一次最多 500 个 id */
export async function removeRecentPDFs(ids: string[]): Promise<void> {
  if (live()) {
    await call(DocBinding.RemoveRecentPDFs(ids))
    return
  }
  if (ids.length > 500) simError('INVALID_ARGUMENT', '一次最多 500 个')
  for (const id of ids) simRecent.delete(id)
}

// ---- 转换记录（office_pdf 任务）----

const OFFICE_TERMINAL = ['succeeded', 'failed', 'canceled', 'interrupted']

/** 最近结束的 office_pdf 任务（新的在前）。真实：TaskService.List（刷新页面后接回历史）；模拟：接口层模拟任务 */
export async function listOfficeHistory(limit = 20): Promise<ApiTask[]> {
  if (!live()) return listSimFinished().filter((t) => t.type === 'office_pdf').slice(0, limit)
  const page = await call(TaskBinding.List(goStore.TaskFilter.createFrom({ types: ['office_pdf'], statuses: OFFICE_TERMINAL, limit, offset: 0 })))
  return (page.items ?? []).map(toApiTask)
}

// ---- 选文件 / 演示数据 ----

/** Office 文件选择对话框的过滤器（只列支持的三种；不支持的格式拖进来会得到 UNSUPPORTED 文案） */
export const OFFICE_FILE_FILTER = { name: 'Office 文档', patterns: OFFICE_EXTS.map((e) => `*.${e}`) }
export const PDF_FILE_FILTER = { name: 'PDF 文件', patterns: ['*.pdf'] }

/** 浏览器模拟环境里“选择文件”给出的假路径（真实环境不用） */
export const DEMO_OFFICE_PATHS = ['/Users/me/Documents/用户调研报告.docx', '/Users/me/Documents/2026 Q3 产品回顾.pptx', '/Users/me/Documents/渠道数据汇总.xlsx']
export const DEMO_PDF_PATH = '/Users/me/Documents/2026 Q3 产品回顾.pdf'

/** 文件类型徽标：DOC / XLS / PPT（按扩展名，未知按 DOC） */
export function officeKind(path: string): 'DOC' | 'XLS' | 'PPT' {
  const e = extOf(path)
  return e.startsWith('xls') ? 'XLS' : e.startsWith('ppt') ? 'PPT' : 'DOC'
}
