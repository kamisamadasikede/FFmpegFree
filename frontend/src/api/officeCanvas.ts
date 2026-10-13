/**
 * 办公画布组件（契约 v0.42 §6.19.16）。
 * OFFICE_CANVAS_BACKEND_READY=true 且有 Wails：调 OfficeCanvasService；否则走模拟（?cat_sim=office*）。
 */
import { AppError, call, callService, toAppError } from '@/api/call'
import { OFFICE_CANVAS_BACKEND_READY } from '@/api/flags'
import { emitSimEvent, hasWailsBackend, onTaskEvent } from '@/services/wails'
import * as SystemBinding from '../../wailsjs/go/app/SystemService'

export { OFFICE_CANVAS_BACKEND_READY }

export type OfficeCanvasState =
  | 'checking'
  | 'ready'
  | 'missing'
  | 'outdated'
  | 'downloading'
  | 'preparing'
  | 'failed'

export interface OfficeCanvasStatus {
  state: OfficeCanvasState
  version: string
  source: string
  canDownload: boolean
  downloadBytes: number
  installBytes: number
  phase?: string
  receivedBytes?: number
  error?: { code: string; message: string; detail?: string } | null
}

export interface OfficeCanvasProgress {
  phase: string
  receivedBytes: number
  totalBytes: number
  progress?: number
}

export interface OfficeSlide {
  id: string
  title?: string
  bullets?: string[]
}

export interface OfficeDeck {
  title?: string
  slides: OfficeSlide[]
}

/** cat:artifact kind=slides（契约 6.19.16.5） */
export type OfficeArtifactStatus = 'streaming' | 'ready' | 'exporting' | 'exported' | 'failed'

export interface OfficeArtifact {
  kind: 'slides'
  id: string
  convId: string
  status: OfficeArtifactStatus
  deck?: OfficeDeck
  previewHtmlPath?: string
  exportPath?: string
  error?: { code: string; message: string; detail?: string } | null
}

export interface ExportOfficeCanvasPptxRequest {
  convId: string
  artifactId: string
  destPath?: string
}

export interface ExportOfficeCanvasPptxResult {
  path: string
  bytes: number
}

export const OFFICE_CANVAS_COPY = {
  needTitle: '需要办公画布组件',
  needBody: '用于幻灯片实时预览与导出。下载后即可使用。',
  download: '下载',
  later: '稍后',
  cancel: '取消',
  downloading: '正在下载办公画布组件…',
  failedUnconfigured: '暂时无法下载办公画布组件，请稍后重试。',
  exportBtn: '导出 PPTX',
  exportFailPrefix: '导出失败：',
  toolLabel: '演示文稿',
  toolPending: '待下载组件',
  toolPreview: '实时预览中',
  openFolder: '打开组件文件夹',
  generating: '生成中',
  ghost: '正在生成后续页面…',
} as const

export const OFFICE_EVENTS = {
  canvas: 'office:canvas',
  progress: 'office:canvas-progress',
  artifact: 'cat:artifact',
} as const

const GUIDE_BYTES = 16 * 1024 * 1024
const live = () => OFFICE_CANVAS_BACKEND_READY && hasWailsBackend()
export const isOfficeCanvasSim = (): boolean => !live()

function mapStatus(raw: unknown): OfficeCanvasStatus {
  const s = (raw ?? {}) as Partial<OfficeCanvasStatus>
  const err = s.error
  return {
    state: (s.state as OfficeCanvasState) || 'missing',
    version: s.version ?? '',
    source: s.source ?? '',
    canDownload: s.canDownload === true,
    downloadBytes: typeof s.downloadBytes === 'number' ? s.downloadBytes : GUIDE_BYTES,
    installBytes: typeof s.installBytes === 'number' ? s.installBytes : 0,
    ...(s.phase ? { phase: s.phase } : {}),
    ...(typeof s.receivedBytes === 'number' ? { receivedBytes: s.receivedBytes } : {}),
    error:
      err && typeof (err as { code?: string }).code === 'string'
        ? {
            code: (err as { code: string }).code,
            message: (err as { message?: string }).message ?? '',
            detail: (err as { detail?: string }).detail,
          }
        : null,
  }
}

function mapProgress(raw: unknown): OfficeCanvasProgress {
  const p = (raw ?? {}) as Partial<OfficeCanvasProgress>
  return {
    phase: p.phase ?? 'downloading',
    receivedBytes: typeof p.receivedBytes === 'number' ? p.receivedBytes : 0,
    totalBytes: typeof p.totalBytes === 'number' ? p.totalBytes : GUIDE_BYTES,
    ...(typeof p.progress === 'number' ? { progress: p.progress } : {}),
  }
}

function mapArtifact(raw: unknown): OfficeArtifact | null {
  const a = (raw ?? {}) as Partial<OfficeArtifact> & { artifactId?: string; convID?: string }
  if (a.kind !== 'slides') return null
  const id = a.id || a.artifactId
  const convId = a.convId || a.convID
  if (!id || !convId) return null
  return {
    kind: 'slides',
    id,
    convId,
    status: (a.status as OfficeArtifactStatus) || 'streaming',
    deck: a.deck,
    previewHtmlPath: a.previewHtmlPath,
    exportPath: a.exportPath,
    error: a.error ?? null,
  }
}

/** 模拟状态：由 cat_sim / 本地可变副本驱动 */
let simStatus: OfficeCanvasStatus = {
  state: 'missing',
  version: '',
  source: '',
  canDownload: true,
  downloadBytes: GUIDE_BYTES,
  installBytes: GUIDE_BYTES,
  error: null,
}

export function peekOfficeSimStatus(): OfficeCanvasStatus {
  return { ...simStatus }
}

export function setOfficeSimStatus(patch: Partial<OfficeCanvasStatus>) {
  simStatus = mapStatus({ ...simStatus, ...patch })
  emitSimEvent(OFFICE_EVENTS.canvas, { ...simStatus })
}

/** 走查：mock ≥3 页 IR */
export const MOCK_OFFICE_DECK: OfficeDeck = {
  title: '项目介绍',
  slides: [
    { id: 's1', title: '项目介绍', bullets: ['字幕项目 · 2026 Q4'] },
    { id: 's2', title: '核心能力', bullets: ['校对', '时间轴', '批量导出'] },
    { id: 's3', title: '路线图', bullets: ['克隆项目', 'Git 冲突', '办公画布'] },
    { id: 's4', title: '下一步', bullets: ['真机验收与反馈'] },
  ],
}

export async function getOfficeCanvasStatus(): Promise<OfficeCanvasStatus> {
  if (!live()) return { ...simStatus }
  try {
    return mapStatus(await callService('OfficeCanvasService', 'GetOfficeCanvasStatus'))
  } catch (e) {
    console.error('GetOfficeCanvasStatus failed', toAppError(e).message)
    return mapStatus({ state: 'failed', canDownload: true, downloadBytes: GUIDE_BYTES, error: toAppError(e) })
  }
}

export async function installOfficeCanvas(): Promise<void> {
  if (!live()) {
    // 未配置包：失败文案锁定（契约 6.19.16.3）
    if (simStatus.state === 'missing' && !simStatus.canDownload) {
      const err = { code: 'LANG_DOWNLOAD_FAILED', message: OFFICE_CANVAS_COPY.failedUnconfigured, detail: 'reason=not_configured' }
      setOfficeSimStatus({ state: 'failed', error: err, phase: undefined })
      throw new AppError('LANG_DOWNLOAD_FAILED', OFFICE_CANVAS_COPY.failedUnconfigured, 'reason=not_configured')
    }
    // 有 canDownload 的走查：模拟进度后失败（未配置）或成功（officepreview）
    setOfficeSimStatus({ state: 'downloading', phase: 'downloading', receivedBytes: 0, error: null })
    const total = simStatus.downloadBytes || GUIDE_BYTES
    for (let i = 1; i <= 5; i++) {
      await new Promise((r) => setTimeout(r, 80))
      const received = Math.round((i / 5) * total * 0.58)
      emitSimEvent(OFFICE_EVENTS.progress, { phase: 'downloading', receivedBytes: received, totalBytes: total, progress: i / 5 * 0.58 })
      setOfficeSimStatus({ state: 'downloading', phase: 'downloading', receivedBytes: received })
    }
    // 默认未配置失败（与真机 TBD URL 一致）；?cat_sim=officeready 则进 ready
    const q = typeof window !== 'undefined' ? new URLSearchParams(window.location.search).get('cat_sim') ?? '' : ''
    if (q.split(',').map((x) => x.trim()).includes('officeready')) {
      setOfficeSimStatus({ state: 'ready', source: 'downloaded', version: '0.0.0-sim', phase: undefined, receivedBytes: total, canDownload: true, error: null })
      return
    }
    const err = { code: 'LANG_DOWNLOAD_FAILED', message: OFFICE_CANVAS_COPY.failedUnconfigured, detail: 'reason=not_configured' }
    setOfficeSimStatus({ state: 'failed', error: err, phase: undefined })
    throw new AppError('LANG_DOWNLOAD_FAILED', OFFICE_CANVAS_COPY.failedUnconfigured, 'reason=not_configured')
  }
  await callService('OfficeCanvasService', 'InstallOfficeCanvas')
}

export async function cancelOfficeCanvasInstall(): Promise<void> {
  if (!live()) {
    setOfficeSimStatus({ state: 'missing', phase: undefined, receivedBytes: 0, error: null })
    return
  }
  await callService('OfficeCanvasService', 'CancelOfficeCanvasInstall')
}

export async function recheckOfficeCanvas(): Promise<OfficeCanvasStatus> {
  if (!live()) {
    emitSimEvent(OFFICE_EVENTS.canvas, { ...simStatus })
    return { ...simStatus }
  }
  return mapStatus(await callService('OfficeCanvasService', 'RecheckOfficeCanvas'))
}

export async function exportOfficeCanvasPptx(req: ExportOfficeCanvasPptxRequest): Promise<ExportOfficeCanvasPptxResult> {
  if (!live()) {
    if (simStatus.state !== 'ready') {
      throw new AppError('INTERNAL', OFFICE_CANVAS_COPY.exportFailPrefix + '组件尚未就绪')
    }
    if (!req.convId || !req.artifactId) {
      throw new AppError('INVALID_ARGUMENT', '请指定对话与演示文稿')
    }
    return { path: req.destPath || '/tmp/project-intro.pptx', bytes: 12800 }
  }
  return await callService<ExportOfficeCanvasPptxResult>('OfficeCanvasService', 'ExportOfficeCanvasPptx', req)
}

export async function openOfficeCanvasFolder(): Promise<void> {
  if (!live()) {
    if (simStatus.state !== 'ready') throw new AppError('NOT_FOUND', '办公画布组件还没有就绪。', 'reason=component')
    return
  }
  await call(SystemBinding.OpenStorageFolder('office_canvas'))
}

export function watchOfficeCanvas(cb: (s: OfficeCanvasStatus) => void): () => void {
  return onTaskEvent<unknown>(OFFICE_EVENTS.canvas, (raw) => cb(mapStatus(raw)))
}

export function watchOfficeCanvasProgress(cb: (p: OfficeCanvasProgress) => void): () => void {
  return onTaskEvent<unknown>(OFFICE_EVENTS.progress, (raw) => cb(mapProgress(raw)))
}

export function watchOfficeArtifact(cb: (a: OfficeArtifact) => void): () => void {
  return onTaskEvent<unknown>(OFFICE_EVENTS.artifact, (raw) => {
    const a = mapArtifact(raw)
    if (a) cb(a)
  })
}

/** 走查：推一条 slides artifact（含 mock deck） */
export function emitOfficeSimArtifact(partial?: Partial<OfficeArtifact>) {
  const art: OfficeArtifact = {
    id: partial?.id ?? 'art-slides-1',
    convId: partial?.convId ?? 'sim-conv',
    status: partial?.status ?? 'streaming',
    deck: partial?.deck ?? {
      title: MOCK_OFFICE_DECK.title,
      slides: MOCK_OFFICE_DECK.slides.slice(0, 3),
    },
    ...partial,
    kind: 'slides',
  }
  emitSimEvent(OFFICE_EVENTS.artifact, art)
}
