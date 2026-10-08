/**
 * 契约 v0.24 / v0.24.1 的真实后端调用（6.15–6.17）。后端还没实现、绑定还没生成，所以先用 api/call.ts 的 callService 按名字调
 * window.go.app.<Service>.<Method>；只在 convertV24IsReal()（CONVERT_V2_BACKEND_READY 和 CONVERT_V24_BACKEND_READY 都为 true 且在 Wails 里）时使用。
 * 联调时：后端合入后 `wails generate module`，把这里换成生成的绑定（ConvertService / SystemService），并在 convertRecordsBinding.ts 末尾加上
 * FormatEntry / StorageDirs / ConvertSubmitResult / ReconvertRequest 的生成类型对照，同时去掉那里对 V024*Extra 键的 Omit。
 * Go 侧切片为 nil 时 JSON 是 null，这里统一补成 []。
 */
import { callService } from '@/api/call'
import { onEvent } from '@/services/wails'
import type {
  ConvertSource, ConvertSubmitRequest, ConvertSubmitResult, CopyEvent, FormatEntry, ReconvertRequest, StorageDirs, StorageDirsUpdate, V023Task,
} from '@/api/convertRecords'

const arr = <T>(v: T[] | null | undefined): T[] => v ?? []
const CS = 'ConvertService'
const SS = 'SystemService'

export const GetFormatCatalog = async (): Promise<FormatEntry[]> =>
  arr(await callService<FormatEntry[] | null>(CS, 'GetFormatCatalog')).map((f) => ({ ...f, aliases: arr(f.aliases), presets: arr(f.presets) }))
export const GetStorageDirs = (): Promise<StorageDirs> => callService<StorageDirs>(SS, 'GetStorageDirs')
export const SetStorageDirs = (req: StorageDirsUpdate): Promise<StorageDirs> => callService<StorageDirs>(SS, 'SetStorageDirs', { outputDir: req.outputDir, uploadsDir: req.uploadsDir })
export const OpenStorageFolder = async (kind: 'output' | 'uploads'): Promise<void> => void (await callService(SS, 'OpenStorageFolder', kind))
export const CancelCopy = async (sourceId: string): Promise<void> => void (await callService(CS, 'CancelCopy', sourceId))
export const RetryCopy = (sourceId: string): Promise<ConvertSource> => callService<ConvertSource>(CS, 'RetryCopy', sourceId)
export const SubmitSources = async (req: ConvertSubmitRequest): Promise<ConvertSubmitResult> => {
  const r = await callService<{ tasks?: V023Task[] | null; skipped?: ConvertSubmitResult['skipped'] | null } | null>(CS, 'SubmitSources', req)
  return { tasks: arr(r?.tasks), skipped: arr(r?.skipped) }
}
/** 不给 presetId / options 时不带这两个键（= 沿用原来的参数） */
export const Reconvert = (req: ReconvertRequest): Promise<V023Task> =>
  callService<V023Task>(CS, 'Reconvert', { taskId: req.taskId, ...(req.presetId ? { presetId: req.presetId } : {}), ...(req.options ? { options: req.options } : {}) })
/** v0.24.1 */
export const TakeInterruptedReconverts = async (): Promise<number> => (await callService<number | null>(CS, 'TakeInterruptedReconverts')) ?? 0
export const onCopy = (cb: (e: CopyEvent) => void): (() => void) => onEvent<CopyEvent>('convert:copy', cb)
