/**
 * DocService v0.26（契约 6.12.9~6.12.22）。
 * DOC_V26_BACKEND_READY=false 或无 Wails 绑定时走模拟；绑定落地后改 flags 即可。
 */
import { AppError, callService, toAppError } from '@/api/call'
import { DOC_V26_BACKEND_READY } from '@/api/flags'
import { simDelay, simParam } from '@/api/sim'
import { emitSimEvent, hasWailsBackend, onSimEvent, onTaskEvent } from '@/services/wails'
import { DOC_LINUX_MISSING, DOC_LINUX_OUTDATED } from '@/utils/docV26Text'

export { DOC_V26_BACKEND_READY }

export type DocComponentState =
  | 'checking'
  | 'ready'
  | 'missing'
  | 'outdated'
  | 'downloading'
  | 'preparing'
  | 'failed'

/** 文档组件来源。'' = 未就绪；后续版本可能新增值，前端按未知值兜底显示 */
// eslint-disable-next-line @typescript-eslint/ban-types
export type DocComponentSource = '' | 'system' | 'downloaded' | 'office' | 'wps' | (string & {})

/** v0.27（6.12.28）：本机检测到的引擎，按自动顺序。本包只类型化 + 模拟，不做引擎下拉 / Office / WPS 界面（下一包） */
export interface DocEngineInfo {
  /** office | wps | component（开放联合，后续可能加） */
  id: 'office' | 'wps' | 'component' | (string & {})
  /** 后端给的显示名（Microsoft Office / WPS / 文档组件），前端直接显示 */
  name: string
  version: string
  /** 只有已安装的 component 有：downloaded | system */
  source?: 'downloaded' | 'system' | (string & {})
  /** office / wps 恒为 true；component 没下载时 false */
  installed: boolean
  families: ('text' | 'sheet' | 'slide' | (string & {}))[]
  available: boolean
}

export interface DocComponentStatus {
  /** v0.27 起：文档转换整体是否可用——任何一个引擎可用就是 ready；都不可用时等于 componentState。文档页下载引导只看它 */
  state: DocComponentState
  /** v0.27 新增：文档组件自己的状态（取值同 v0.26 的 state）。下载 / 准备 / 过旧 / 失败、设置页「文档组件」块看它 */
  componentState: DocComponentState
  /** v0.27 新增：全部引擎，始终输出（没有时 []） */
  engines: DocEngineInfo[]
  version: string
  /** ready 时 office | wps（v0.27）| system | downloaded；其他 ""。开放联合，后续再加值不用改这里 */
  source: DocComponentSource
  canDownload: boolean
  /** 安装包字节；界面“约 xxx MB” */
  downloadBytes: number
  /** v0.26.1（架构师定）：解包后占用字节（约 1.5 GiB）；没有下载源（Linux）为 0。0 时不写“安装后约占用”半句 */
  installBytes: number
  phase?: 'downloading' | 'preparing'
  receivedBytes?: number
  error?: { code: string; message: string; detail?: string } | null
}

export interface DocTarget {
  ext: string
  displayName: string
  needsComponent: boolean
  simple: boolean
  available: boolean
  /** v0.27：能做这个转换的引擎 id（只供排查，界面不显示） */
  engines?: string[]
  hintKey?: string
  hint?: string
  disabledReason?: string
}

export interface DocSourceFormats {
  ext: string
  aliases?: string[]
  family: 'text' | 'sheet' | 'slide'
  targets: DocTarget[]
}

export interface DocFormatMatrix {
  componentReady: boolean
  inputs: string[]
  sources: DocSourceFormats[]
}

/** 契约 6.12.14：DocSource = ConvertSource（6.14.2 / 6.15.3，media 恒空）+ ext / family / sheetCount */
export interface DocSource {
  sourceId: string
  path?: string
  originalPath: string
  storedPath?: string
  addedAt?: number
  copiedBytes?: number
  totalBytes?: number
  copyError?: { code: string; message: string; detail?: string } | null
  name: string
  ext: string
  family: 'text' | 'sheet' | 'slide'
  sheetCount: number
  copyState?: string
  sizeBytes?: number
  recordCount?: number
  lastActivityAt?: number
}

export interface AddDocSourceResult {
  path: string
  source?: DocSource
  error?: { code: string; message: string; detail?: string } | null
}

export interface DocSubmitRequest {
  sourceIds: string[]
  target: string
  outputDir: string
}

export interface DocComponentProgress {
  phase: 'downloading' | 'preparing' | 'done'
  receivedBytes: number
  totalBytes: number
  progress?: number
}

export interface DocQueueItem {
  id: string
  queuePosition: number
}

const live = () => DOC_V26_BACKEND_READY && hasWailsBackend()
export const docV26IsReal = (): boolean => live()
export const docV26On = (): boolean => true // 本包始终打开 v0.26 界面；数据源由 isReal 决定

// ─── 模拟状态 ───
// 模拟层内部的 state 就是「文档组件自己的状态」；对外经 v27() 换成 v0.27 的形状（state = 整体、componentState、engines）
type SimStatus = Omit<DocComponentStatus, 'componentState' | 'engines'>
let simStatus: SimStatus = {
  state: 'missing',
  version: '',
  source: '',
  canDownload: true,
  downloadBytes: 373_252_096, // 契约 Windows MSI 实测
  installBytes: 1_610_612_736, // v0.26.1 约 1.5 GiB
}
let simDismissed = false
let simProgress: DocComponentProgress | null = null
let simTimer: ReturnType<typeof setInterval> | null = null
let simSources: DocSource[] = []
let simOs: 'win' | 'mac' | 'linux' = 'win'

function previewOs(): 'win' | 'mac' | 'linux' {
  const o = simParam('os') || simParam('doc_os')
  if (o === 'linux' || o === 'mac') return o
  return 'win'
}

function previewState(): DocComponentState | null {
  const s = simParam('doc') || simParam('doccomp')
  if (!s) return null
  const map: Record<string, DocComponentState> = {
    ready: 'ready',
    missing: 'missing',
    outdated: 'outdated',
    guide: 'missing',
    dl: 'downloading',
    downloading: 'downloading',
    prep: 'preparing',
    preparing: 'preparing',
    dlfail: 'failed',
    failed: 'failed',
    checking: 'checking',
  }
  return map[s] ?? null
}

function applyPreviewStatus() {
  simOs = previewOs()
  const st = previewState()
  const canDl = simOs !== 'linux'
  const base: SimStatus = {
    state: st ?? 'missing',
    version: '',
    source: '',
    canDownload: canDl,
    downloadBytes: canDl ? 373_252_096 : 0,
    installBytes: canDl ? 1_610_612_736 : 0,
  }
  if (st === 'ready') {
    simStatus = { ...base, state: 'ready', version: simParam('ver') === 'none' ? '' : '26.2.6.3', source: 'downloaded' }
  } else if (st === 'outdated') {
    simStatus = {
      ...base,
      state: 'outdated',
      version: '7.0.0',
      error: {
        code: 'DOC_COMPONENT_NOT_READY',
        message: canDl ? '文档组件版本太旧，请重新下载。' : DOC_LINUX_OUTDATED,
      },
    }
  } else if (st === 'downloading') {
    simStatus = { ...base, state: 'downloading', phase: 'downloading', receivedBytes: Math.round(0.42 * base.downloadBytes) }
    simProgress = { phase: 'downloading', receivedBytes: simStatus.receivedBytes!, totalBytes: base.downloadBytes, progress: 0.42 }
  } else if (st === 'preparing') {
    simStatus = { ...base, state: 'preparing', phase: 'preparing', receivedBytes: base.downloadBytes }
    simProgress = { phase: 'preparing', receivedBytes: base.downloadBytes, totalBytes: base.downloadBytes }
  } else if (st === 'failed') {
    const code = simParam('doc_err') || 'DOC_DOWNLOAD_FAILED'
    const detail =
      code === 'CONVERT_DISK_FULL'
        ? 'reason=no_space\nneedBytes=2516582400\nfreeBytes=536870912'
        : undefined
    simStatus = {
      ...base,
      state: 'failed',
      receivedBytes: Math.round(0.42 * base.downloadBytes),
      error: {
        code,
        message:
          code === 'CONVERT_DISK_FULL'
            ? '磁盘空间不足'
            : code === 'DOC_CHECKSUM_FAILED'
              ? '下载的文档组件校验失败，请重试。'
              : code === 'DOC_COMPONENT_INSTALL_FAILED'
                ? '文档组件准备失败，请重试。'
                : '文档组件下载失败，请检查网络后重试。',
        detail,
      },
    }
  } else if (st === 'checking') {
    simStatus = { ...base, state: 'checking' }
  } else {
    // missing
    simStatus = {
      ...base,
      state: 'missing',
      error: canDl ? null : { code: 'DOC_COMPONENT_NOT_READY', message: DOC_LINUX_MISSING },
    }
  }
  if (simParam('doc') === 'slim' || simParam('doc') === 'fb-word' || simParam('doc') === 'fb-md') simDismissed = true
}

const TEXT = ['doc', 'docx', 'odt', 'rtf', 'txt', 'html', 'md'] as const
const SHEET = ['xls', 'xlsx', 'ods', 'csv'] as const
const SLIDE = ['ppt', 'pptx', 'odp'] as const
const LABELS: Record<string, string> = {
  pdf: 'PDF',
  docx: 'Word (DOCX)',
  doc: 'Word 97-2003 (DOC)',
  odt: '开放文档 (ODT)',
  rtf: '富文本 (RTF)',
  txt: '纯文本 (TXT)',
  html: '网页 (HTML)',
  md: 'Markdown (MD)',
  xlsx: 'Excel (XLSX)',
  xls: 'Excel 97-2003 (XLS)',
  ods: '开放表格 (ODS)',
  csv: '逗号分隔 (CSV)',
  pptx: 'PPT (PPTX)',
  ppt: 'PPT 97-2003 (PPT)',
  odp: '开放演示 (ODP)',
}

function familyOf(ext: string): 'text' | 'sheet' | 'slide' {
  if ((SHEET as readonly string[]).includes(ext)) return 'sheet'
  if ((SLIDE as readonly string[]).includes(ext)) return 'slide'
  return 'text'
}

function targetsFor(ext: string, ready: boolean): DocTarget[] {
  const fam = familyOf(ext)
  const peers = fam === 'text' ? TEXT : fam === 'sheet' ? SHEET : SLIDE
  const list = ['pdf', ...peers.filter((e) => e !== ext)]
  return list.map((t) => {
    const goPure = (ext === 'md' && t === 'html') || (ext === 'html' && t === 'md')
    // needsComponent：与组件当前状态无关；md↔html 为 false
    const needC = !goPure
    const simp = !ready && t === 'pdf' && ['docx', 'odt', 'txt'].includes(ext)
    const avail = !needC || ready || simp
    let hintKey = ''
    let hint = ''
    if (t === 'md') {
      hintKey = 'md_lossy'
      hint = '转成 Markdown 只保留文字和基本格式，图片和复杂表格会丢失。'
    } else if (t === 'csv' && (SHEET as readonly string[]).includes(ext) && ext !== 'csv') {
      hintKey = 'csv_first_sheet'
      hint = '转成 CSV 只会保留第一个工作表。'
    } else if (simp) {
      hintKey = 'simple_mode'
      hint = '下载文档组件后可保留图片和排版'
    }
    return {
      ext: t,
      displayName: LABELS[t] ?? t.toUpperCase(),
      needsComponent: needC,
      simple: simp,
      available: avail,
      hintKey: hintKey || undefined,
      hint: hint || undefined,
      disabledReason: avail ? undefined : '需要文档组件',
    }
  })
}

function buildMatrix(ready: boolean): DocFormatMatrix {
  const inputs = [...TEXT, 'htm', 'markdown', ...SHEET, ...SLIDE]
  const sources: DocSourceFormats[] = [
    ...TEXT.map((ext) => ({
      ext,
      aliases: ext === 'html' ? ['htm'] : ext === 'md' ? ['markdown'] : undefined,
      family: 'text' as const,
      targets: targetsFor(ext, ready),
    })),
    ...SHEET.map((ext) => ({ ext, family: 'sheet' as const, targets: targetsFor(ext, ready) })),
    ...SLIDE.map((ext) => ({ ext, family: 'slide' as const, targets: targetsFor(ext, ready) })),
  ]
  return { componentReady: ready, inputs, sources }
}

function emitStatus() {
  emitSimEvent('doc:component', v27(simStatus))
}

/** 预览参数 &engine=office|wps：模拟本机装了 Office / WPS（只在 Windows 模拟） */
function simLocalEngine(): 'office' | 'wps' | 'both' | '' {
  const e = simParam('engine')
  return simOs === 'win' && (e === 'office' || e === 'wps' || e === 'both') ? e : ''
}

function v27(s: SimStatus): DocComponentStatus {
  const local = simLocalEngine()
  const engines: DocEngineInfo[] = []
  if (local === 'office' || local === 'both') engines.push({ id: 'office', name: 'Microsoft Office', version: '16.0.17928.20114', installed: true, families: ['text', 'sheet', 'slide'], available: true })
  if (local === 'wps' || local === 'both') engines.push({ id: 'wps', name: 'WPS', version: '12.1.0.18276', installed: true, families: ['text', 'sheet', 'slide'], available: true })
  const compReady = s.state === 'ready'
  if (simOs !== 'linux' || compReady)
    engines.push({ id: 'component', name: '文档组件', version: compReady ? s.version : '', ...(compReady ? { source: s.source || 'downloaded' } : {}), installed: compReady, families: ['text', 'sheet', 'slide'], available: compReady })
  const overallReady = !!local || compReady
  return {
    ...s,
    state: overallReady ? 'ready' : s.state,
    componentState: s.state,
    engines,
    source: (local === 'both' ? 'office' : local) || (compReady ? s.source : ''),
    version: local ? engines[0].version : s.version,
  }
}

function emitProgress(p: DocComponentProgress) {
  simProgress = p
  emitSimEvent('doc:component-progress', p)
}

// ─── 公共 API ───

export async function getDocComponentStatus(): Promise<DocComponentStatus> {
  if (live()) return callService<DocComponentStatus>('DocService', 'GetDocComponentStatus')
  applyPreviewStatus()
  return v27(simStatus)
}

export async function getFormatMatrix(): Promise<DocFormatMatrix> {
  if (live()) return callService<DocFormatMatrix>('DocService', 'GetFormatMatrix')
  applyPreviewStatus()
  return buildMatrix(v27(simStatus).state === 'ready')
}

export async function installDocComponent(mirror = ''): Promise<DocComponentStatus> {
  if (live()) return callService<DocComponentStatus>('DocService', 'InstallDocComponent', mirror)
  applyPreviewStatus()
  if (!simStatus.canDownload) {
    throw new AppError('UNSUPPORTED_PLATFORM', DOC_LINUX_MISSING)
  }
  if (simStatus.state === 'downloading' || simStatus.state === 'preparing') return v27(simStatus)
  // 模拟磁盘满
  if (simParam('doc_err') === 'CONVERT_DISK_FULL' && simStatus.state !== 'failed') {
    simStatus = {
      ...simStatus,
      state: 'failed',
      error: {
        code: 'CONVERT_DISK_FULL',
        message: '磁盘空间不足',
        detail: 'reason=no_space\nneedBytes=2516582400\nfreeBytes=536870912',
      },
    }
    emitStatus()
    return v27(simStatus)
  }
  const total = simStatus.downloadBytes || 373_252_096
  let received = simStatus.receivedBytes ?? 0
  simStatus = { ...simStatus, state: 'downloading', phase: 'downloading', receivedBytes: received, error: null }
  emitStatus()
  if (simTimer) clearInterval(simTimer)
  simTimer = setInterval(() => {
    received = Math.min(total, received + total / 25)
    const progress = received / total
    simStatus = { ...simStatus, receivedBytes: Math.round(received) }
    emitProgress({ phase: 'downloading', receivedBytes: Math.round(received), totalBytes: total, progress })
    if (received >= total) {
      if (simTimer) clearInterval(simTimer)
      simTimer = null
      simStatus = { ...simStatus, state: 'preparing', phase: 'preparing' }
      emitStatus()
      emitProgress({ phase: 'preparing', receivedBytes: total, totalBytes: total })
      setTimeout(() => {
        simStatus = {
          state: 'ready',
          version: '26.2.6.3',
          source: 'downloaded',
          canDownload: true,
          downloadBytes: total,
          installBytes: simStatus.installBytes,
        }
        emitProgress({ phase: 'done', receivedBytes: total, totalBytes: total, progress: 1 })
        emitStatus()
      }, 1500)
    }
  }, 200)
  return v27(simStatus)
}

export async function cancelDocComponentInstall(): Promise<void> {
  if (live()) {
    await callService('DocService', 'CancelDocComponentInstall')
    return
  }
  if (simTimer) clearInterval(simTimer)
  simTimer = null
  applyPreviewStatus()
  simStatus = {
    ...simStatus,
    state: 'missing',
    phase: undefined,
    error: null,
  }
  emitStatus()
}

export async function recheckDocComponent(): Promise<DocComponentStatus> {
  if (live()) return callService<DocComponentStatus>('DocService', 'RecheckDocComponent')
  applyPreviewStatus()
  emitStatus()
  return v27(simStatus)
}

export function dismissDocGuide() {
  simDismissed = true
}
export function isDocGuideDismissed() {
  return simDismissed
}
export function resetDocGuideDismissed() {
  simDismissed = false
}

export async function addDocSources(paths: string[]): Promise<AddDocSourceResult[]> {
  if (live()) return callService<AddDocSourceResult[]>('DocService', 'AddDocSources', paths)
  await simDelay(80)
  return paths.map((path) => {
    const name = path.split(/[\\/]/).pop() ?? path
    const raw = (name.split('.').pop() ?? '').toLowerCase()
    const ext = raw === 'htm' ? 'html' : raw === 'markdown' ? 'md' : raw
    if (ext === 'pdf') {
      return { path, error: { code: 'DOC_PDF_INPUT_UNSUPPORTED', message: 'PDF 暂时不能转成其他格式。' } }
    }
    const inputs = [...TEXT, ...SHEET, ...SLIDE]
    if (!(inputs as readonly string[]).includes(ext)) {
      return { path, error: { code: 'DOC_FORMAT_UNSUPPORTED', message: '不支持这种文件。' } }
    }
    if (/密码|password|lock/i.test(name)) {
      return { path, error: { code: 'DOC_ENCRYPTED', message: '这个文件有密码保护，不能转换。请先去掉密码再添加。' } }
    }
    if (/损坏|corrupt|坏/i.test(name)) {
      return { path, error: { code: 'DOC_CORRUPT', message: '文件打不开，可能已损坏或不是有效的文档。' } }
    }
    const fam = familyOf(ext)
    let sheetCount = 0
    if (fam === 'sheet') {
      if (ext === 'csv') sheetCount = 1
      else if (/单表|one/i.test(name)) sheetCount = 1
      else if (/未知|unknown/i.test(name)) sheetCount = -1
      else sheetCount = 3
    }
    const id = `docsrc-${Math.random().toString(36).slice(2, 10)}`
    const source: DocSource = {
      sourceId: id,
      originalPath: path,
      storedPath: path,
      name,
      ext,
      family: fam,
      sheetCount,
      copyState: 'ready',
      sizeBytes: 1024 * 100,
      recordCount: 0,
      lastActivityAt: Date.now(),
    }
    simSources.unshift(source)
    return { path, source }
  })
}

export function watchDocComponent(cb: (s: DocComponentStatus) => void): () => void {
  return onTaskEvent<DocComponentStatus>('doc:component', cb)
}
export function watchDocProgress(cb: (p: DocComponentProgress) => void): () => void {
  return onTaskEvent<DocComponentProgress>('doc:component-progress', cb)
}
export function watchDocQueue(cb: (q: { items: DocQueueItem[] }) => void): () => void {
  return onTaskEvent<{ items: DocQueueItem[] }>('doc:queue', cb)
}

/** 模拟用：列出已添加源（页面 store 自己持有；这里给测试） */
export function listSimDocSources(): DocSource[] {
  return [...simSources]
}

export function clearSimDocSources() {
  simSources = []
}

/** 记录 = Task（doc_convert / 带 sourceId 的 office_pdf），只取页面要用的字段 */
export interface DocRecord {
  id: string
  type: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  title: string
  outputPath: string
  progress: number
  params?: string
  sourceId?: string
  queuePosition?: number
  error?: { code: string; message: string; detail?: string } | null
  /** 成功记录的结果（6.14.6 warnings；v0.27 engine = office | wps | component | go | simple） */
  result?: { engine?: 'office' | 'wps' | 'component' | 'go' | 'simple' | (string & {}); warnings?: string[] } | null
  createdAt: number
  startedAt?: number
  finishedAt?: number
  version?: number
}

export interface DocSourceEntry {
  source: DocSource
  records: DocRecord[]
  recordCount: number
}
export interface DocSourcePage {
  items: DocSourceEntry[]
  total: number
}
export interface ConvertSubmitResult {
  tasks: DocRecord[]
  skipped?: { sourceId: string; reason: string }[]
}

export async function listDocSources(f: { limit: number; offset: number; recordLimit: number; status?: string }): Promise<DocSourcePage> {
  if (live()) return callService<DocSourcePage>('DocService', 'ListDocSources', { ...f, status: f.status ?? '' })
  return { items: [], total: 0 }
}

export async function searchDocSources(f: { keyword: string; limit: number; offset: number; recordLimit: number; status?: string }): Promise<DocSourcePage> {
  if (live()) return callService<DocSourcePage>('DocService', 'SearchDocSources', { ...f, status: f.status ?? '' })
  return { items: [], total: 0 }
}

export async function submitDocConvert(req: DocSubmitRequest): Promise<ConvertSubmitResult> {
  if (live()) return callService<ConvertSubmitResult>('DocService', 'SubmitDocConvert', req)
  throw new AppError('UNSUPPORTED', '模拟模式由页面 store 处理')
}

/** 按 id 的操作复用 TaskService / ConvertService（契约 6.12.14） */
export async function retryDocRecord(id: string): Promise<void> {
  if (live()) await callService('TaskService', 'Retry', id)
}
export async function cancelDocRecord(id: string): Promise<void> {
  if (live()) await callService('TaskService', 'Cancel', id)
}
export async function deleteDocSource(sourceId: string): Promise<void> {
  if (live()) await callService('ConvertService', 'DeleteSource', sourceId, false)
}
export async function deleteDocRecords(ids: string[]): Promise<void> {
  if (live()) await callService('ConvertService', 'DeleteRecords', ids, false)
}

export function watchDocTasks(h: {
  created?: (t: DocRecord) => void
  progress?: (p: { id: string; progress: number; version?: number }) => void
  status?: (p: Partial<DocRecord> & { id: string }) => void
}): () => void {
  const offs = [
    h.created ? onTaskEvent<DocRecord>('task:created', h.created) : () => {},
    h.progress ? onTaskEvent<{ id: string; progress: number }>('task:progress', h.progress) : () => {},
    h.status ? onTaskEvent<Partial<DocRecord> & { id: string }>('task:status', h.status) : () => {},
  ]
  return () => offs.forEach((f) => f())
}

export { toAppError, LABELS as DOC_TARGET_LABELS, familyOf as docFamilyOf, onSimEvent }
