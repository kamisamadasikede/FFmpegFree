/**
 * DocService 接口层（契约 v0.12 / 6.12，#23 最新提交为准，尚未冻结）。
 *
 * DOC_BACKEND_READY（api/flags.ts）= false：本地模拟（内存里的最近 PDF 列表 + 假 PDF 字节 + api/sim.ts 的定时器转换任务）；
 * = true：window.go.app.DocService.*（callService；绑定不存在抛 UNSUPPORTED）。
 * 选文件用 SystemService.PickFiles（api/system.ts）；取消转换 = TaskService.Cancel，重试 = TaskService.Retry（office_pdf 注册了重试工厂）。
 * DocService 任何方法都不返回 FFMPEG_NOT_FOUND / PROBE_FAILED / PROCESS_FAILED，不依赖 ffmpeg，始终可用。
 *
 * 契约要点：
 * - 支持 docx / xlsx / pptx（仅文本，不保留图片和样式）；doc xls ppt odt ods odp rtf csv txt pages numbers key 及加密文档 → UNSUPPORTED。
 * - ConvertToPDF 先整体校验再提交，任何一个不通过整体失败、不提交任何任务，detail 第一行是出错文件路径。
 * - 预览：OpenPDF 登记句柄 → ReadPDFChunk 分块（≤ 1 MiB）读整份（≤ 64 MiB，主路径）；更大的文件用 PDFSource.url 走 Range（Windows 未验证，架构师定：验证不通过则上限降为 64 MiB，OpenPDF 对更大的文件 INVALID_ARGUMENT）。
 * - “实验性”标签优先按 DocCapabilities.experimental 显示（后端字段，尚未写进契约，见 README 疑问）；模拟层默认 true。
 */
import { AppError, callService } from '@/api/call'
import { DOC_BACKEND_READY } from '@/api/flags'
import { createSimTask, injectionDetail, simDelay, simError, simInjection, simParam } from '@/api/sim'
import { toApiTask, type ApiTask } from '@/api/taskTypes'

export { DOC_BACKEND_READY }

// ───────────── 契约类型（§6.12.2，字段一一对应）─────────────

export interface DocFormat {
  /** 不带点，小写 */
  ext: string
  supported: boolean
  /** "text-only"（支持的三种）| "" */
  fidelity: 'text-only' | ''
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
  /** 实际读到的字节数 */
  length: number
  /** offset+length >= 文件当前大小 */
  eof: boolean
  /** Go 的 []byte，JSON 里是 base64 字符串（模拟层也返回 base64，统一走 decodeChunk） */
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
  if (DOC_BACKEND_READY) return await callService<DocCapabilities>('DocService', 'GetDocCapabilities')
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
  if (DOC_BACKEND_READY) return ((await callService<unknown[] | null>('DocService', 'ConvertToPDF', inputs, outputDir)) ?? []).map(toApiTask)
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

/** 模拟用的最小 PDF 字节（一页空白） */
function simPdfBytes(size: number): Uint8Array {
  const head = new TextEncoder().encode('%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n')
  const out = new Uint8Array(Math.max(size, head.length))
  out.set(head)
  return out
}

/** 校验并登记一个 PDF，返回句柄；同时写入最近列表（OpenPDF 是唯一的写入点）。转换产出的 PDF 不自动进历史，需要预览时对 outputPath 调用它 */
export async function openPDF(path: string): Promise<PDFSource> {
  if (DOC_BACKEND_READY) return await callService<PDFSource>('DocService', 'OpenPDF', path)
  await simDelay(80)
  const inj = simInjection()
  if (inj && inj.when === 'call') simError(inj.code, '模拟错误', injectionDetail(inj))
  if (!isAbs(path)) simError('INVALID_ARGUMENT', '路径必须是绝对路径')
  const name = baseName(path)
  if (name.startsWith('缺失') || name.startsWith('missing')) simError('NOT_FOUND', '文件不存在')
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
  if (DOC_BACKEND_READY) return await callService<PDFChunk>('DocService', 'ReadPDFChunk', id, offset, length)
  if (length < 1 || length > DEFAULT_DOC_LIMITS.chunkBytes) simError('INVALID_ARGUMENT', `length 需要在 1~${DEFAULT_DOC_LIMITS.chunkBytes} 之间`)
  if (offset < 0) simError('INVALID_ARGUMENT', 'offset 不能为负')
  const h = simHandles.get(id)
  if (!h) return simError('NOT_FOUND', '句柄不存在，请重新打开')
  if (offset >= h.size) return { offset, length: 0, eof: true, data: '' }
  const n = Math.min(length, h.size - offset)
  const bytes = simPdfBytes(h.size).subarray(offset, offset + n)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return { offset, length: n, eof: offset + n >= h.size, data: btoa(bin) }
}

/** base64（Go []byte 在 JSON 里的形态）→ Uint8Array */
export function decodeChunk(chunk: PDFChunk): Uint8Array {
  const bin = atob(chunk.data || '')
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

/**
 * 主路径：循环 ReadPDFChunk 直到 eof，拼成 Uint8Array 交给 pdf.js（size ≤ wholeLoadBytes）。
 * size 超过 wholeLoadBytes 时抛 UNSUPPORTED，调用方改用 src.url 走 Range（或在降级后提示文件太大）。
 * onProgress(已读, 总大小)。文件在读取期间被改动（size 变化）需要重新 OpenPDF。
 */
export async function readWholePDF(src: PDFSource, limits: DocLimits = DEFAULT_DOC_LIMITS, onProgress?: (read: number, total: number) => void, signal?: AbortSignal): Promise<Uint8Array> {
  if (src.size > limits.wholeLoadBytes) throw new AppError('UNSUPPORTED', '文件较大，需要按需加载')
  const buf = new Uint8Array(src.size)
  let off = 0
  while (true) {
    if (signal?.aborted) throw new AppError('CANCELED', '已取消')
    const c = await readPDFChunk(src.id, off, limits.chunkBytes)
    const bytes = decodeChunk(c)
    if (off + bytes.length > buf.length) throw new AppError('IO_ERROR', '文件在读取期间发生了变化，请重新打开')
    buf.set(bytes, off)
    off += bytes.length
    onProgress?.(off, src.size)
    if (c.eof || bytes.length === 0) break
  }
  return off === buf.length ? buf : buf.subarray(0, off)
}

/** 最近打开的 PDF，按 openedAt 倒序。limit 默认 20，最大 200（0 = 默认，越界 INVALID_ARGUMENT） */
export async function listRecentPDFs(limit = 0): Promise<PDFFile[]> {
  if (DOC_BACKEND_READY) return (await callService<PDFFile[] | null>('DocService', 'ListRecentPDFs', limit)) ?? []
  if (limit < 0 || limit > 200) simError('INVALID_ARGUMENT', 'limit 范围 0~200')
  return [...simRecent.values()].sort((a, b) => b.openedAt - a.openedAt).slice(0, limit || 20)
}

/** 只删记录，不删文件；一次最多 500 个 id */
export async function removeRecentPDFs(ids: string[]): Promise<void> {
  if (DOC_BACKEND_READY) {
    await callService('DocService', 'RemoveRecentPDFs', ids)
    return
  }
  if (ids.length > 500) simError('INVALID_ARGUMENT', '一次最多 500 个')
  for (const id of ids) simRecent.delete(id)
}
