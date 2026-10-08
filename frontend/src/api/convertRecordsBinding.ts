/**
 * 转换记录的真实后端调用，名字和形状按契约 v0.23（PR #82，head 04f118c，§6.14.3）。#82 还没合入、绑定还没生成，
 * 所以走 callService（window.go.app.<Service>.<Method>）：绑定不存在时抛 UNSUPPORTED，不会白屏。
 * 只在 CONVERT_V2_BACKEND_READY 为 true 且在 Wails 里时使用。名字如果在合入前又改了，只改 V023_METHODS。
 */
import { callService } from '@/api/call'
import type {
  AddSourceResult, ConvertSearchFilter, ConvertSourceFilter, ConvertSourcePage, ConvertSubmitRequest, DeleteResult, PreviewURL,
  ConvertSourceEntry, RecordOptions, SourcePathCheck, TaskPage, TaskPathCheck, V023Task,
} from '@/api/convertRecords'
import type { TaskStatus } from '@/stores/tasks'

export const V023_METHODS = {
  AddSources: ['ConvertService', 'AddSources'],
  ListSources: ['ConvertService', 'ListSources'],
  ListSourceRecords: ['ConvertService', 'ListSourceRecords'],
  SearchSources: ['ConvertService', 'SearchSources'],
  CheckSources: ['ConvertService', 'CheckSources'],
  PreviewOutputName: ['ConvertService', 'PreviewOutputName'],
  SubmitSources: ['ConvertService', 'SubmitSources'],
  Reconvert: ['ConvertService', 'Reconvert'],
  DeleteRecords: ['ConvertService', 'DeleteRecords'],
  DeleteSource: ['ConvertService', 'DeleteSource'],
  GetSourcePreviewURL: ['ConvertService', 'GetSourcePreviewURL'],
  OpenSourceWithSystem: ['ConvertService', 'OpenSourceWithSystem'],
  RevealSource: ['ConvertService', 'RevealSource'],
  /** v0.23.1（后端实现 PR 里加）：完成记录“打开所在文件夹”；文件不在 → NOT_FOUND reason=file */
  RevealRecord: ['ConvertService', 'RevealRecord'],
  /** v0.23.1（后端实现 PR 里加）：取单个源文件行，(sourceId) → ConvertSourceEntry（任务中心“在转换页查看”定位不在已加载页里的行） */
  GetSource: ['ConvertService', 'GetSource'],
  HideFinishedInTaskCenter: ['TaskService', 'HideFinishedInTaskCenter'],
  GetSourceThumbnail: ['ConvertService', 'GetSourceThumbnail'],
  GetRecordThumbnail: ['ConvertService', 'GetRecordThumbnail'],
  /** (ids []string) error */
  UnhideInTaskCenter: ['TaskService', 'UnhideInTaskCenter'],
  /** 已有；v0.23 TaskFilter 加 includeHidden */
  List: ['TaskService', 'List'],
  CheckPaths: ['TaskService', 'CheckPaths'],
  GetPreviewURL: ['TaskService', 'GetPreviewURL'],
  OpenWithSystem: ['TaskService', 'OpenWithSystem'],
} as const

const svc = <T>(k: keyof typeof V023_METHODS, ...args: unknown[]): Promise<T> => callService<T>(V023_METHODS[k][0], V023_METHODS[k][1], ...args)
const arr = <T>(v: T[] | null | undefined): T[] => v ?? []
const page = (p: ConvertSourcePage | null): ConvertSourcePage => ({ items: arr(p?.items).map((e) => ({ ...e, records: arr(e.records) })), total: p?.total ?? 0 })

export const AddSources = async (paths: string[]) => arr(await svc<AddSourceResult[]>('AddSources', paths))
export const ListSources = async (f: ConvertSourceFilter) => page(await svc<ConvertSourcePage>('ListSources', f))
export const ListSourceRecords = async (sourceId: string, limit: number, offset: number): Promise<TaskPage> => {
  const p = await svc<TaskPage>('ListSourceRecords', sourceId, limit, offset)
  return { items: arr(p?.items), total: p?.total ?? 0 }
}
export const SearchSources = async (f: ConvertSearchFilter) => page(await svc<ConvertSourcePage>('SearchSources', f))
export const CheckSources = async (ids: string[]) => arr(await svc<SourcePathCheck[]>('CheckSources', ids))
export const PreviewOutputName = (sourceId: string, opts: RecordOptions, outputDir: string) => svc<string>('PreviewOutputName', sourceId, opts, outputDir)
export const SubmitSources = async (req: ConvertSubmitRequest) => arr(await svc<V023Task[]>('SubmitSources', req))
export const Reconvert = (taskId: string) => svc<V023Task>('Reconvert', taskId)
const delResult = (r: DeleteResult | null): DeleteResult => ({ deletedTaskIds: arr(r?.deletedTaskIds), deletedSourceIds: arr(r?.deletedSourceIds), deletedFiles: r?.deletedFiles ?? 0, failures: arr(r?.failures) })
export const DeleteRecords = async (ids: string[], deleteOutputs: boolean) => delResult(await svc<DeleteResult>('DeleteRecords', ids, deleteOutputs))
export const DeleteSource = async (sourceId: string, deleteOutputs: boolean) => delResult(await svc<DeleteResult>('DeleteSource', sourceId, deleteOutputs))
export const GetSourcePreviewURL = (sourceId: string) => svc<PreviewURL>('GetSourcePreviewURL', sourceId)
export const OpenSourceWithSystem = async (sourceId: string) => void (await svc('OpenSourceWithSystem', sourceId))
export const RevealSource = async (sourceId: string) => void (await svc('RevealSource', sourceId))
export const RevealRecord = async (taskId: string) => void (await svc('RevealRecord', taskId))
/** 不存在 → NOT_FOUND reason=record */
export const GetSource = async (sourceId: string): Promise<ConvertSourceEntry> => {
  const e = await svc<ConvertSourceEntry>('GetSource', sourceId)
  return { ...e, records: arr(e?.records) }
}

/** 返回 data:image/jpeg;base64,... */
export const GetSourceThumbnail = (sourceId: string) => svc<string>('GetSourceThumbnail', sourceId)
export const GetRecordThumbnail = (taskId: string) => svc<string>('GetRecordThumbnail', taskId)

export const HideFinishedInTaskCenter = async () => (await svc<number>('HideFinishedInTaskCenter')) ?? 0
export const UnhideInTaskCenter = async (ids: string[]) => void (await svc('UnhideInTaskCenter', ids))
export const List = async (f: { types: string[]; statuses: TaskStatus[]; limit: number; offset: number; includeHidden: boolean }): Promise<TaskPage> => {
  const p = await svc<TaskPage>('List', f)
  return { items: arr(p?.items), total: p?.total ?? 0 }
}
export const CheckPaths = async (ids: string[]) => arr(await svc<TaskPathCheck[]>('CheckPaths', ids))
export const GetPreviewURL = (taskId: string, which: 'input' | 'output') => svc<PreviewURL>('GetPreviewURL', taskId, which)
export const OpenWithSystem = async (taskId: string, which: 'input' | 'output') => void (await svc('OpenWithSystem', taskId, which))
