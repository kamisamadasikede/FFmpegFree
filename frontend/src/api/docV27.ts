/**
 * DocService / SystemService v0.27 ~ v0.27.2 新接口（契约 6.12.28~6.12.57）。
 * DOC_V27_BACKEND_READY=false 或无 Wails 绑定时全部走模拟；绑定落地后改 flags 即可（名字、参数形状照契约）。
 *
 * 模拟参数（只在模拟层生效）：
 *   ?engine=office|wps|both     本机有 Office / WPS（引擎下拉、记录引擎行）
 *   ?pv=loading                 预览一直停在「正在生成预览…」
 *   ?pv=<encrypted|ppt_busy|busy|failed|needs|too_large>  所有预览强制成该状态
 *   ?save_err=<file_changed|converting|copying|in_use|permission|backup|io|encoding|too_large|malformed|format|disk|missing|checksum>
 *   ?doc_os=linux               预览不给下载按钮
 * 文件名触发：含「密码」加密、「占用」被占用（pptx 演示 / 其他文档）、「损坏」失败、「大」太大、「长」csv/文本超长、「宏」docm 类。
 */
import * as SystemBinding from '../../wailsjs/go/app/SystemService'
import { system } from '../../wailsjs/go/models'
import { AppError, call, callService, type AppErrorCode } from '@/api/call'
import { DOC_V27_BACKEND_READY } from '@/api/flags'
import { simParam } from '@/api/sim'
import { simPdfBytes } from '@/api/doc'
import { emitSimEvent, hasWailsBackend, onTaskEvent } from '@/services/wails'
import type { DocEnginePref } from '@/utils/docV27Text'

export { DOC_V27_BACKEND_READY }
const live = () => DOC_V27_BACKEND_READY && hasWailsBackend()
export const docV27IsReal = (): boolean => live()

// ─── 类型（6.12.32.1 / 6.12.38 / 6.12.48） ───
export type DocPreviewKind = 'pdf' | 'text' | 'csv' | 'html' | 'md' | 'raw' | 'unavailable' | (string & {})
export type DocPreviewState = 'generating' | 'ready' | 'failed' | (string & {})
export type DocEditBlock = 'format' | 'too_large' | 'encoding' | 'malformed' | 'converting' | 'missing' | (string & {})
export interface DocPreviewRequest {
  sourceId?: string
  taskId?: string
}
export interface DocPreview {
  previewId: string
  kind: DocPreviewKind
  state: DocPreviewState
  name: string
  ext: string
  url?: string
  text?: string
  rows?: string[][]
  totalRows?: number
  truncated?: boolean
  sizeBytes: number
  reason?: string
  error?: { code: string; message: string; detail?: string } | null
  editable: boolean
  editBlock?: DocEditBlock
  revision?: string
  encoding?: 'utf8' | 'utf8_bom' | 'gbk' | (string & {})
  lineEnding?: 'crlf' | 'lf' | (string & {})
  rawUrl?: string
}
export interface DocPreviewEvent {
  previewId: string
  state: DocPreviewState
  kind: DocPreviewKind
  url?: string
  error?: { code: string; message: string; detail?: string } | null
}
export interface DocSaveRequest extends DocPreviewRequest {
  revision: string
  text?: string
  rows?: string[][]
}
export interface DocSaveAsRequest extends DocPreviewRequest {
  targetPath: string
  text?: string
  rows?: string[][]
  encoding?: 'keep' | 'utf8'
}
export interface DocSaveResult {
  path: string
  revision: string
  sizeBytes: number
  savedAt: number
}
export interface DocBinarySaveBegin extends DocPreviewRequest {
  mode: 'overwrite' | 'save_as'
  targetPath?: string
  revision?: string
  totalBytes: number
}
export interface DocBinarySaveSession {
  saveId: string
  maxChunkBytes: number
  expiresAt: number
}
export interface DocBinarySaveResult extends DocSaveResult {
  backupPath?: string
}
export interface FileFilter {
  name: string
  patterns: string[]
}

/** 文本类能编辑的扩展名（6.12.37） */
export const TEXT_EDIT_EXTS = ['txt', 'md', 'markdown', 'html', 'htm', 'csv'] as const
export const DOCX_EDIT_MAX_BYTES = 20 * 1024 * 1024
export const RAW_CHUNK_BYTES = 4 * 1024 * 1024
export const MAX_LIVE_PREVIEWS = 8

const sameFormat: Record<string, string[]> = { txt: ['txt'], md: ['md', 'markdown'], markdown: ['md', 'markdown'], html: ['html', 'htm'], htm: ['html', 'htm'], csv: ['csv'], docx: ['docx'] }
/** 另存为对话框的过滤器（只能同一种格式） */
export function saveFilters(ext: string): FileFilter[] {
  const e = ext.toLowerCase()
  const pats = (sameFormat[e] ?? [e]).map((x) => `*.${x}`)
  const names: Record<string, string> = { txt: '文本文件', md: 'Markdown 文件', markdown: 'Markdown 文件', html: '网页文件', htm: '网页文件', csv: 'CSV 文件', docx: 'Word 文档' }
  return [{ name: names[e] ?? '同类文件', patterns: pats }]
}
export const extOf = (p: string): string => (/\.([^./\\]+)$/.exec(p)?.[1] ?? '').toLowerCase()
export const isSameFormat = (a: string, b: string): boolean => (sameFormat[a.toLowerCase()] ?? [a.toLowerCase()]).includes(b.toLowerCase())

// ─── 工具：base64 / sha256 / 分段 ───
export function bytesToBase64(b: Uint8Array): string {
  let s = ''
  const step = 0x8000
  for (let i = 0; i < b.length; i += step) s += String.fromCharCode.apply(null, Array.from(b.subarray(i, i + step)))
  return btoa(s)
}
export async function sha256Hex(b: Uint8Array): Promise<string> {
  const d = await crypto.subtle.digest('SHA-256', b as unknown as ArrayBuffer)
  return Array.from(new Uint8Array(d), (x) => x.toString(16).padStart(2, '0')).join('')
}
/** [start, endExclusive) 分段，每段 ≤ max */
export function chunkRanges(total: number, max: number): [number, number][] {
  const out: [number, number][] = []
  const m = Math.max(1, Math.floor(max))
  for (let s = 0; s < total; s += m) out.push([s, Math.min(total, s + m)])
  return out
}

/**
 * raw 文件按 Range 分段取（6.12.32.1：每段 ≤ 4 MiB，拼成整份）。blob: 地址（模拟）一次读完。
 * 先 HEAD 拿长度（sizeHint 有就直接用）；服务端不认 Range（200 整体）时直接用整体。
 */
export async function fetchRawBytes(url: string, sizeHint = 0, opts: { fetcher?: typeof fetch; signal?: AbortSignal; chunk?: number } = {}): Promise<Uint8Array> {
  const f = opts.fetcher ?? fetch
  if (url.startsWith('blob:') || url.startsWith('data:')) return new Uint8Array(await (await f(url, { signal: opts.signal })).arrayBuffer())
  let total = sizeHint
  if (!total) {
    const h = await f(url, { method: 'HEAD', signal: opts.signal })
    if (h.status === 404) throw new AppError('NOT_FOUND', 'gone')
    total = Number(h.headers.get('content-length') ?? 0)
  }
  if (!total) throw new AppError('IO_ERROR', 'empty')
  const out = new Uint8Array(total)
  for (const [s, e] of chunkRanges(total, opts.chunk ?? RAW_CHUNK_BYTES)) {
    const r = await f(url, { headers: { Range: `bytes=${s}-${e - 1}` }, signal: opts.signal })
    if (r.status === 404) throw new AppError('NOT_FOUND', 'gone')
    if (!r.ok) throw new AppError('IO_ERROR', 'range')
    const buf = new Uint8Array(await r.arrayBuffer())
    if (r.status === 200) return buf // 不认 Range：整份
    out.set(buf.subarray(0, e - s), s)
  }
  return out
}

// ─── 设置：docEngine（6.12.29，GetSettings → 改 → UpdateSettings 整体写回） ───
let simEngine: DocEnginePref = 'auto'
export async function getDocEngine(): Promise<DocEnginePref> {
  if (!live()) return simEngine
  const s = (await call(SystemBinding.GetSettings())) as unknown as { docEngine?: string }
  const v = s?.docEngine ?? ''
  return (['auto', 'office', 'wps', 'component'].includes(v) ? v : 'auto') as DocEnginePref
}
export async function setDocEngine(pref: DocEnginePref): Promise<void> {
  if (!live()) {
    simEngine = pref
    return
  }
  const s = (await call(SystemBinding.GetSettings())) as unknown as Record<string, unknown>
  // 现有绑定的 Settings 类还没有 docEngine 字段，createFrom 会把它丢掉：这里直接整体传对象（后端按 JSON 解）
  await call(SystemBinding.UpdateSettings({ ...s, docEngine: pref } as unknown as system.Settings))
}

/** 6.12.54：打开应用下载的文档组件目录 */
export async function openDocComponentFolder(): Promise<void> {
  if (!hasWailsBackend()) return
  await call(SystemBinding.OpenStorageFolder('doc_component'))
}

/** 6.12.43：系统保存对话框；取消返回 "" */
export async function saveFileDialog(defaultName: string, filters: FileFilter[]): Promise<string> {
  if (!live()) {
    const forced = simParam('save_as_path')
    if (forced !== null) return forced
    const base = defaultName.replace(/^.*[\\/]/, '')
    const dot = base.lastIndexOf('.')
    return `/Users/me/Documents/${dot > 0 ? `${base.slice(0, dot)}（副本）${base.slice(dot)}` : `${base}（副本）`}`
  }
  const fn = (SystemBinding as unknown as { SaveFileDialog: (n: string, f: unknown[]) => Promise<string> }).SaveFileDialog
  return (await call(fn(defaultName, filters.map((x) => system.FileFilter.createFrom(x))))) ?? ''
}

// ─── 打开 / 显示（复用已有方法） ───
export async function openWithSystem(req: DocPreviewRequest): Promise<void> {
  if (!hasWailsBackend()) return
  if (req.sourceId) await callService('ConvertService', 'OpenSourceWithSystem', req.sourceId)
  else if (req.taskId) await callService('TaskService', 'OpenWithSystem', req.taskId, 'output')
}
export async function revealDoc(req: DocPreviewRequest): Promise<void> {
  if (!hasWailsBackend()) return
  if (req.sourceId) await callService('ConvertService', 'RevealSource', req.sourceId)
  else if (req.taskId) await callService('ConvertService', 'RevealRecord', req.taskId)
}
export async function revealPath(path: string): Promise<void> {
  if (!hasWailsBackend()) return
  await call(SystemBinding.RevealInFolder(path))
}
export async function reconvertRecord(taskId: string): Promise<void> {
  if (!hasWailsBackend() || !live()) return
  await callService('ConvertService', 'Reconvert', { taskId })
}

// ─── 预览 ───
export function watchDocPreview(cb: (e: DocPreviewEvent) => void): () => void {
  return onTaskEvent<DocPreviewEvent>('doc:preview', cb)
}
export async function cancelDocPreview(previewId: string): Promise<void> {
  if (live()) {
    await callService('DocService', 'CancelDocPreview', previewId)
    return
  }
  const t = simTimers.get(previewId)
  if (t) clearTimeout(t)
  simTimers.delete(previewId)
  const u = simUrls.get(previewId)
  if (u) u.forEach((x) => URL.revokeObjectURL(x))
  simUrls.delete(previewId)
}

/** 模拟用的提示：页面知道的文件信息（真实后端不需要） */
export interface SimPreviewHint {
  name: string
  sizeBytes: number
  /** 有可用引擎（state=ready） */
  engineReady: boolean
  /** 这一行有排队 / 运行中的转换 */
  converting?: boolean
  /** 原文件不在了 */
  missing?: boolean
}

export async function getDocPreview(req: DocPreviewRequest, hint?: SimPreviewHint): Promise<DocPreview> {
  if (live()) return callService<DocPreview>('DocService', 'GetDocPreview', req)
  return simPreview(req, hint ?? { name: 'file.txt', sizeBytes: 1024, engineReady: true })
}

export async function saveDocText(req: DocSaveRequest): Promise<DocSaveResult> {
  if (live()) return callService<DocSaveResult>('DocService', 'SaveDocText', req)
  await wait(350)
  simSaveError('text')
  const k = keyOf(req)
  const cur = simFiles.get(k)
  if (cur && cur.revision !== req.revision) throw mkErr('TASK_CONFLICT', 'file_changed')
  const rev = simRev(req.text ?? JSON.stringify(req.rows))
  simFiles.set(k, { text: req.text, rows: req.rows, revision: rev })
  return { path: cur?.path ?? '/Users/me/Documents/file', revision: rev, sizeBytes: (req.text ?? '').length, savedAt: Date.now() }
}
export async function saveDocTextAs(req: DocSaveAsRequest): Promise<DocSaveResult> {
  if (live()) return callService<DocSaveResult>('DocService', 'SaveDocTextAs', req)
  await wait(350)
  simSaveError('textAs', req.encoding)
  return { path: req.targetPath, revision: simRev(req.text ?? ''), sizeBytes: (req.text ?? '').length, savedAt: Date.now() }
}

// ─── docx 分段保存（6.12.49） ───
export async function beginDocBinarySave(req: DocBinarySaveBegin): Promise<DocBinarySaveSession> {
  if (live()) return callService<DocBinarySaveSession>('DocService', 'BeginDocBinarySave', req)
  if (req.totalBytes > DOCX_EDIT_MAX_BYTES) throw mkErr('INVALID_ARGUMENT', 'too_large')
  const saveId = `sim-save-${++simSeq}`
  simSessions.set(saveId, { req, got: 0, next: 0 })
  return { saveId, maxChunkBytes: Number(simParam('save_chunk') ?? 0) || RAW_CHUNK_BYTES, expiresAt: Date.now() + 600_000 }
}
export async function appendDocBinaryChunk(req: { saveId: string; seq: number; data: string }): Promise<{ receivedBytes: number }> {
  if (live()) return callService('DocService', 'AppendDocBinaryChunk', req)
  const s = simSessions.get(req.saveId)
  if (!s) throw mkErr('NOT_FOUND', 'save_session')
  if (req.seq !== s.next) throw mkErr('INVALID_ARGUMENT', 'chunk_order')
  s.next++
  s.got += atob(req.data).length
  await wait(60)
  return { receivedBytes: s.got }
}
export async function commitDocBinarySave(req: { saveId: string; sha256: string }): Promise<DocBinarySaveResult> {
  if (live()) return callService<DocBinarySaveResult>('DocService', 'CommitDocBinarySave', req)
  const s = simSessions.get(req.saveId)
  simSessions.delete(req.saveId)
  if (!s) throw mkErr('NOT_FOUND', 'save_session')
  if (s.got !== s.req.totalBytes || !/^[0-9a-f]{64}$/.test(req.sha256)) throw mkErr('INVALID_ARGUMENT', 'checksum')
  await wait(300)
  simSaveError(s.req.mode === 'overwrite' ? 'docx' : 'docxAs')
  const path = s.req.mode === 'save_as' ? s.req.targetPath ?? '' : '/Users/me/Documents/季度报告.docx'
  const now = new Date()
  const p2 = (n: number) => String(n).padStart(2, '0')
  const stamp = `${now.getFullYear()}${p2(now.getMonth() + 1)}${p2(now.getDate())}-${p2(now.getHours())}${p2(now.getMinutes())}${p2(now.getSeconds())}`
  return {
    path,
    revision: req.sha256,
    sizeBytes: s.got,
    savedAt: Date.now(),
    backupPath: s.req.mode === 'overwrite' ? path.replace(/\.docx$/i, `.bak-${stamp}.docx`) : undefined,
  }
}
export async function abortDocBinarySave(saveId: string): Promise<void> {
  if (live()) {
    await callService('DocService', 'AbortDocBinarySave', { saveId })
    return
  }
  simSessions.delete(saveId)
}
export const simSessionCount = (): number => simSessions.size

/**
 * 整个 docx 保存流程：sha256 → Begin → Append（按 maxChunkBytes）→ Commit；任何一步失败（含 signal 取消）都 Abort，再把错误抛给调用方。
 */
export async function saveDocxBytes(
  bytes: Uint8Array,
  target: DocPreviewRequest & { mode: 'overwrite' | 'save_as'; targetPath?: string; revision?: string },
  opts: { onProgress?: (sent: number, total: number) => void; signal?: AbortSignal } = {},
): Promise<DocBinarySaveResult> {
  const sha = await sha256Hex(bytes)
  const begin: DocBinarySaveBegin = { mode: target.mode, totalBytes: bytes.length }
  if (target.sourceId) begin.sourceId = target.sourceId
  else begin.taskId = target.taskId
  if (target.mode === 'save_as') begin.targetPath = target.targetPath
  else begin.revision = target.revision
  const sess = await beginDocBinarySave(begin)
  try {
    let seq = 0
    for (const [s, e] of chunkRanges(bytes.length, Math.min(sess.maxChunkBytes || RAW_CHUNK_BYTES, RAW_CHUNK_BYTES))) {
      if (opts.signal?.aborted) throw new AppError('CANCELED', 'canceled')
      await appendDocBinaryChunk({ saveId: sess.saveId, seq: seq++, data: bytesToBase64(bytes.subarray(s, e)) })
      opts.onProgress?.(e, bytes.length)
    }
    if (opts.signal?.aborted) throw new AppError('CANCELED', 'canceled')
    return await commitDocBinarySave({ saveId: sess.saveId, sha256: sha })
  } catch (e) {
    await abortDocBinarySave(sess.saveId).catch(() => {})
    throw e
  }
}

// ─── 模拟 ───
const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))
let simSeq = 0
const simTimers = new Map<string, ReturnType<typeof setTimeout>>()
const simUrls = new Map<string, string[]>()
const simSessions = new Map<string, { req: DocBinarySaveBegin; got: number; next: number }>()
const simFiles = new Map<string, { text?: string; rows?: string[][]; revision: string; path?: string }>()
const keyOf = (r: DocPreviewRequest) => (r.sourceId ? `s:${r.sourceId}` : `t:${r.taskId}`)
function simRev(s: string): string {
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619) >>> 0
  return h.toString(16).padStart(8, '0').repeat(8)
}
function mkErr(code: AppErrorCode, reason?: string): AppError {
  const e = new AppError(code, '模拟错误', reason ? `reason=${reason}` : undefined)
  e.reason = reason
  return e
}
function simSaveError(mode: 'text' | 'textAs' | 'docx' | 'docxAs', encoding?: string) {
  const v = simParam('save_err')
  if (!v) return
  // 另存为只在 save_err_as=1 时出错（这样「另存为」能走通）
  const asMode = mode === 'textAs' || mode === 'docxAs'
  if (asMode && simParam('save_err_as') !== '1' && v !== 'appdir') return
  if (v === 'encoding' && encoding === 'utf8') return
  const map: Record<string, [AppErrorCode, string?]> = {
    file_changed: ['TASK_CONFLICT', 'file_changed'],
    converting: ['TASK_CONFLICT', 'converting'],
    copying: ['TASK_CONFLICT', 'copying'],
    saving: ['TASK_CONFLICT', 'saving'],
    in_use: ['IO_ERROR', 'in_use'],
    permission: ['IO_ERROR', 'permission'],
    backup: ['IO_ERROR', 'backup'],
    io: ['IO_ERROR', 'io'],
    encoding: ['INVALID_ARGUMENT', 'encoding'],
    too_large: ['INVALID_ARGUMENT', 'too_large'],
    malformed: ['INVALID_ARGUMENT', 'malformed'],
    format: ['INVALID_ARGUMENT', 'format'],
    checksum: ['INVALID_ARGUMENT', 'checksum'],
    appdir: ['INVALID_ARGUMENT'],
    disk: ['CONVERT_DISK_FULL'],
    missing: ['NOT_FOUND', 'file'],
  }
  const m = map[v]
  if (m) throw mkErr(m[0], m[1])
}

function simUrl(previewId: string, bytes: Uint8Array | ArrayBuffer, mime: string): string {
  const u = URL.createObjectURL(new Blob([bytes as BlobPart], { type: mime }))
  simUrls.set(previewId, [...(simUrls.get(previewId) ?? []), u])
  return u
}

const SIM_MD = `# 季度复盘

这是一份 **Markdown** 示例，左边改右边会跟着变。

## 本季度完成

- 文档转换支持本机 Office / WPS
- 预览窗口统一
- 表格和文本可以直接编辑

| 项目 | 负责人 | 进度 |
| --- | --- | --- |
| 预览 | 小林 | 90% |
| 编辑 | 小周 | 70% |

> 引用：链接只显示文字，[官网](https://example.com) 不会被打开。

\`\`\`
npm run build
\`\`\`

<script>alert(1)</script> 原始 HTML 会显示成文字。

![远程图片不加载](https://example.com/a.png)
`

const SIM_HTML = `<!doctype html><html><head><meta charset="utf-8"><title>活动通知</title>
<style>h1{color:#3370ff} .tip{background:#fff7e6;padding:8px 12px;border-radius:6px}</style>
<script>alert('x')</script></head><body>
<h1>周五团建活动通知</h1>
<p>时间：<b>10 月 17 日 14:00</b>，地点：三楼大会议室。</p>
<p class="tip">请提前 10 分钟到场。</p>
<ul><li>签到</li><li>分组游戏</li><li>晚餐</li></ul>
<p><a href="https://example.com" onclick="alert(1)">报名链接</a>（只显示文字）</p>
<img src="https://example.com/x.png" alt="[远程图片]" onerror="alert(1)">
</body></html>`

const SIM_TXT = Array.from({ length: 40 }, (_, i) => (i === 0 ? '会议纪要 2026-10-09' : i % 7 === 0 ? '' : `${i}. 讨论事项第 ${i} 条：确认排期、负责人和验收标准。`)).join('\n')

function simCsvRows(n: number): string[][] {
  const head = ['日期', '部门', '项目', '预算（元）', '实际（元）', '备注']
  const deps = ['市场部', '研发部', '行政部', '销售部']
  const rows = [head]
  for (let i = 1; i < n; i++) rows.push([`2026-${String((i % 12) + 1).padStart(2, '0')}-${String((i % 27) + 1).padStart(2, '0')}`, deps[i % 4], `项目 ${i}`, String(1000 + ((i * 137) % 9000)), String(900 + ((i * 211) % 9500)), i % 5 === 0 ? '待复核' : ''])
  return rows
}

async function simDocxBytes(title: string): Promise<Uint8Array> {
  const JSZip = (await import('jszip')).default
  const z = new JSZip()
  z.file('[Content_Types].xml', '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>')
  z.file('_rels/.rels', '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>')
  const p = (t: string, opt: { b?: boolean; sz?: number } = {}) => `<w:p><w:r><w:rPr>${opt.b ? '<w:b/>' : ''}${opt.sz ? `<w:sz w:val="${opt.sz}"/>` : ''}</w:rPr><w:t xml:space="preserve">${t}</w:t></w:r></w:p>`
  const body = [
    p(title, { b: true, sz: 40 }),
    p('一、项目概况', { b: true, sz: 28 }),
    p('本季度共完成 12 个版本发布，文档转换新增本机 Office / WPS 引擎，预览窗口统一了 PDF、表格和文本的显示方式。'),
    p('二、关键数据', { b: true, sz: 28 }),
    p('转换成功率 98.6%，平均耗时 3.2 秒；预览打开时间中位数 0.8 秒。'),
    p('三、下季度计划', { b: true, sz: 28 }),
    p('1. 视频预览迁移到统一预览窗口。'),
    p('2. docx 在应用里直接编辑。'),
    p('3. 继续优化大文件的简易预览。'),
  ].join('')
  z.file('word/document.xml', `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>${body}<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`)
  return z.generateAsync({ type: 'uint8array' })
}

async function simXlsxBytes(): Promise<ArrayBuffer> {
  const XLSX = await import('xlsx')
  const wb = XLSX.utils.book_new()
  const s1 = [['季度', '收入（万元）', '支出（万元）', '利润（万元）', '同比', '备注'], ['Q1', 320, 210, 110, '12%', ''], ['Q2', 360, 230, 130, '18%', '新品上线'], ['Q3', 410, 260, 150, '21%', ''], ['Q4', 450, 280, 170, '25%', '预计'], ['合计', 1540, 980, 560, '', '']]
  XLSX.utils.book_append_sheet(wb, XLSX.utils.aoa_to_sheet(s1), '汇总')
  XLSX.utils.book_append_sheet(wb, XLSX.utils.aoa_to_sheet(simCsvRows(40).map((r) => r.slice(0, 5))), '明细')
  XLSX.utils.book_append_sheet(wb, XLSX.utils.aoa_to_sheet([['说明'], ['本表由财务部整理，数据截至 9 月 30 日。']]), '说明')
  return XLSX.write(wb, { type: 'array', bookType: 'xlsx' }) as ArrayBuffer
}

function simForced(): string | null {
  return simParam('pv')
}

async function simPreview(req: DocPreviewRequest, hint: SimPreviewHint): Promise<DocPreview> {
  await wait(120)
  const previewId = `sim-pv-${++simSeq}`
  const name = hint.name
  const ext = extOf(name)
  const base: DocPreview = { previewId, kind: 'unavailable', state: 'ready', name, ext, sizeBytes: hint.sizeBytes, editable: false }
  const forced = simForced()
  const failed = (code: string): DocPreview => ({ ...base, kind: 'pdf', state: 'failed', editBlock: 'format', error: { code, message: '模拟' } })
  if (forced === 'encrypted' || name.includes('密码')) return failed('DOC_ENCRYPTED')
  if (forced === 'ppt_busy' || (name.includes('占用') && /^pptx?$/.test(ext))) return failed('DOC_PRESENTATION_BUSY')
  if (forced === 'busy' || name.includes('占用')) return failed('DOC_ENGINE_BUSY')
  if (forced === 'failed' || name.includes('损坏')) return failed('DOC_CORRUPT')
  const office = ['doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'odt', 'ods', 'odp', 'rtf'].includes(ext)
  const big = name.includes('大') || hint.sizeBytes > 50 * 1024 * 1024
  if (forced === 'needs' || (office && !hint.engineReady && (forced !== 'too_large') && !['docx', 'xlsx'].includes(ext)))
    return { ...base, kind: 'unavailable', reason: 'needs_component', editBlock: 'format' }
  if (forced === 'too_large' || (office && !hint.engineReady && big)) return { ...base, kind: 'unavailable', reason: 'too_large_for_simple', editBlock: 'format' }
  const conv = hint.converting ? 'converting' : undefined
  const missing = hint.missing || name.includes('不在') ? 'missing' : undefined
  if (ext === 'docx' || ext === 'xlsx' || office || ext === 'pdf') {
    const docx = ext === 'docx'
    const raw = docx || ext === 'xlsx' ? (docx ? await simDocxBytes(name.replace(/\.[^.]+$/, '')) : new Uint8Array(await simXlsxBytes())) : null
    const mime = docx ? 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' : 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
    const rawUrl = raw ? simUrl(previewId, raw, mime) : undefined
    const tooBig = hint.sizeBytes > DOCX_EDIT_MAX_BYTES
    const macro = name.includes('宏')
    const edit = docx ? { editable: !tooBig && !conv && !missing && !macro, editBlock: missing ?? (tooBig ? 'too_large' : macro ? 'malformed' : conv), rawUrl: tooBig ? undefined : rawUrl, revision: tooBig ? undefined : simRev(name) } : { editable: false, editBlock: 'format' }
    if (!hint.engineReady && raw) return { ...base, ...edit, kind: 'raw', url: rawUrl }
    const pages = name.includes('页') || docx ? 12 : 3
    const pdfUrl = simUrl(previewId, simPdfBytes(0, pages), 'application/pdf')
    if (ext === 'pdf' || simParam('pv') === 'cached') return { ...base, ...edit, kind: 'pdf', url: pdfUrl }
    // 要引擎生成：先 generating，过一会发 doc:preview
    if (simParam('pv') !== 'loading') {
      simTimers.set(previewId, setTimeout(() => {
        simTimers.delete(previewId)
        emitSimEvent('doc:preview', { previewId, state: 'ready', kind: 'pdf', url: pdfUrl } satisfies DocPreviewEvent)
      }, 1400))
    }
    return { ...base, ...edit, kind: 'pdf', state: 'generating' }
  }
  const k = keyOf(req)
  const stored = simFiles.get(k)
  const long = name.includes('长')
  const textMeta = { encoding: name.includes('GBK') ? 'gbk' : 'utf8', lineEnding: 'crlf' } as const
  if (ext === 'csv') {
    const all = long ? simCsvRows(1000) : stored?.rows ?? simCsvRows(24)
    const totalRows = long ? 5234 : all.length
    const rev = stored?.revision ?? simRev(JSON.stringify(all))
    simFiles.set(k, { rows: all, revision: rev })
    const blk = missing ?? (totalRows > 1000 ? 'too_large' : name.includes('格式') ? 'malformed' : conv)
    return { ...base, kind: 'csv', rows: all, totalRows, editable: !blk, editBlock: blk, revision: rev, ...textMeta }
  }
  const kind = ext === 'md' || ext === 'markdown' ? 'md' : ext === 'html' || ext === 'htm' ? 'html' : 'text'
  let text = stored?.text ?? (kind === 'md' ? SIM_MD : kind === 'html' ? SIM_HTML : SIM_TXT)
  if (long && !stored) text = Array.from({ length: 400 }, (_, i) => `第 ${i + 1} 行：这是一个很长的文件，只显示前面一部分。`).join('\n')
  const rev = stored?.revision ?? simRev(text)
  simFiles.set(k, { text, revision: rev })
  const blk = missing ?? (long ? 'too_large' : name.includes('编码') ? 'encoding' : conv)
  return { ...base, kind, text, truncated: long, editable: !blk, editBlock: blk, revision: rev, ...textMeta }
}
