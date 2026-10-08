/**
 * 转换记录的真实后端调用：直接用 Wails 生成的绑定（wailsjs/go/app/ConvertService、TaskService，类型来自 wailsjs/go/models.ts 的
 * convert / store / task 命名空间）。后端 #83 / #84 / #85（契约 v0.23–v0.23.3）已合入 v2，绑定按仓库做法用 `wails generate module` 生成并提交。
 * 只在 CONVERT_V2_BACKEND_READY 为 true 且在 Wails 里时使用（api/convertRecords.ts 的 convertV2IsReal）。
 * Go 侧切片为 nil 时 JSON 是 null，这里统一补成 []。入参用生成的 createFrom 规范成后端的形状。
 * 前端自己的类型和生成类型的对应关系在文件末尾做编译期检查（字段改名 / 少字段时 vue-tsc 报错）。
 */
import { call } from '@/api/call'
import * as CS from '../../wailsjs/go/app/ConvertService'
import * as TS from '../../wailsjs/go/app/TaskService'
import { convert, ffmpeg, store, task } from '../../wailsjs/go/models'
import type {
  AddSourceResult, ConvertSearchFilter, ConvertSourceFilter, ConvertSourcePage, ConvertSubmitRequest, DeleteResult, PreviewURL,
  ConvertSourceEntry, RecordOptions, SourcePathCheck, TaskPage, TaskPathCheck, V023Task,
} from '@/api/convertRecords'
import type { TaskStatus } from '@/stores/tasks'

const arr = <T>(v: T[] | null | undefined): T[] => v ?? []
/** 生成类型 → 前端类型：字段一致（见末尾检查），status / error 在前端是更窄的联合类型 */
const as = <T>(v: unknown): T => v as T
const entry = (e: convert.ConvertSourceEntry | null | undefined): ConvertSourceEntry => ({ ...as<ConvertSourceEntry>(e), records: arr(as<V023Task[] | null>(e?.records)) })
const page = (p: convert.ConvertSourcePage | null | undefined): ConvertSourcePage => ({ items: arr(p?.items).map(entry), total: p?.total ?? 0 })
const taskPage = (p: store.TaskPage | null | undefined): TaskPage => ({ items: arr(as<V023Task[] | null>(p?.items)), total: p?.total ?? 0 })
const delResult = (r: task.DeleteResult | null | undefined): DeleteResult => ({
  deletedTaskIds: arr(r?.deletedTaskIds), deletedSourceIds: arr(r?.deletedSourceIds), deletedFiles: r?.deletedFiles ?? 0, failures: arr(r?.failures),
})
const opts = (o: RecordOptions) => ffmpeg.ConvertOptions.createFrom(o)

export const AddSources = async (paths: string[]) => arr(as<AddSourceResult[] | null>(await call(CS.AddSources(paths))))
/** v0.23.1 / v0.23.2：status 总是带上（缺省 ''） */
export const ListSources = async (f: ConvertSourceFilter) => page(await call(CS.ListSources(convert.ConvertSourceFilter.createFrom({ ...f, status: f.status ?? '' }))))
export const ListSourceRecords = async (sourceId: string, limit: number, offset: number): Promise<TaskPage> => taskPage(await call(CS.ListSourceRecords(sourceId, limit, offset)))
export const SearchSources = async (f: ConvertSearchFilter) => page(await call(CS.SearchSources(convert.ConvertSearchFilter.createFrom({ ...f, status: f.status ?? '' }))))
export const CheckSources = async (ids: string[]) => arr(as<SourcePathCheck[] | null>(await call(CS.CheckSources(ids))))
export const PreviewOutputName = (sourceId: string, o: RecordOptions, outputDir: string) => call(CS.PreviewOutputName(sourceId, opts(o), outputDir))
export const SubmitSources = async (req: ConvertSubmitRequest) => arr(as<V023Task[] | null>(await call(CS.SubmitSources(convert.ConvertSubmitRequest.createFrom({ ...req, options: opts(req.options) })))))
export const Reconvert = async (taskId: string) => as<V023Task>(await call(CS.Reconvert(taskId)))
export const DeleteRecords = async (ids: string[], deleteOutputs: boolean) => delResult(await call(CS.DeleteRecords(ids, deleteOutputs)))
export const DeleteSource = async (sourceId: string, deleteOutputs: boolean) => delResult(await call(CS.DeleteSource(sourceId, deleteOutputs)))
export const GetSourcePreviewURL = async (sourceId: string): Promise<PreviewURL> => await call(CS.GetSourcePreviewURL(sourceId))
export const OpenSourceWithSystem = async (sourceId: string) => void (await call(CS.OpenSourceWithSystem(sourceId)))
export const RevealSource = async (sourceId: string) => void (await call(CS.RevealSource(sourceId)))
/** v0.23.1：完成记录“打开所在文件夹”；文件不在 → NOT_FOUND reason=file，记录不在 → reason=record */
export const RevealRecord = async (taskId: string) => void (await call(CS.RevealRecord(taskId)))
/** v0.23.1：不存在 → NOT_FOUND reason=record */
export const GetSource = async (sourceId: string): Promise<ConvertSourceEntry> => entry(await call(CS.GetSource(sourceId)))
/** 返回 data:image/jpeg;base64,... */
export const GetSourceThumbnail = (sourceId: string) => call(CS.GetSourceThumbnail(sourceId))
export const GetRecordThumbnail = (taskId: string) => call(CS.GetRecordThumbnail(taskId))

export const HideFinishedInTaskCenter = async () => (await call(TS.HideFinishedInTaskCenter())) ?? 0
export const UnhideInTaskCenter = async (ids: string[]) => void (await call(TS.UnhideInTaskCenter(ids)))
export const List = async (f: { types: string[]; statuses: TaskStatus[]; limit: number; offset: number; includeHidden: boolean }): Promise<TaskPage> =>
  taskPage(await call(TS.List(store.TaskFilter.createFrom(f))))
export const CheckPaths = async (ids: string[]) => arr(as<TaskPathCheck[] | null>(await call(TS.CheckPaths(ids))))
export const GetPreviewURL = async (taskId: string, which: 'input' | 'output'): Promise<PreviewURL> => await call(TS.GetPreviewURL(taskId, which))
export const OpenWithSystem = async (taskId: string, which: 'input' | 'output') => void (await call(TS.OpenWithSystem(taskId, which)))

// ---------------- 编译期检查：前端类型 ↔ 生成类型（Go 结构体的 json 名）----------------
/** A 的每个键 B 都有，B 的每个必填键 A 也有 */
type SameKeys<A, B> = [Exclude<keyof A, keyof B>, Exclude<{ [K in keyof B]-?: undefined extends B[K] ? never : K }[keyof B], keyof A>] extends [never, never] ? true : { onlyFrontend: Exclude<keyof A, keyof B>; missingRequired: Exclude<{ [K in keyof B]-?: undefined extends B[K] ? never : K }[keyof B], keyof A> }
type Fn<T> = { [K in keyof T as T[K] extends (...a: never[]) => unknown ? K : never]: T[K] }
type Data<T> = Omit<T, keyof Fn<T>>
const ok = <T extends true>(): T => true as T
export const BINDING_SHAPES_OK = [
  ok<SameKeys<V023Task, Data<store.Task>>>(),
  ok<SameKeys<NonNullable<V023Task['result']>, Data<store.TaskResult>>>(),
  ok<SameKeys<ConvertSourceEntry, Data<convert.ConvertSourceEntry>>>(),
  ok<SameKeys<ConvertSourceEntry['source'], Data<store.ConvertSource>>>(),
  ok<SameKeys<ConvertSourceFilter, Data<convert.ConvertSourceFilter>>>(),
  ok<SameKeys<ConvertSearchFilter, Data<convert.ConvertSearchFilter>>>(),
  ok<SameKeys<ConvertSubmitRequest, Data<convert.ConvertSubmitRequest>>>(),
  ok<SameKeys<RecordOptions, Data<ffmpeg.ConvertOptions>>>(),
  ok<SameKeys<AddSourceResult, Data<convert.AddSourceResult>>>(),
  ok<SameKeys<SourcePathCheck, Data<convert.SourcePathCheck>>>(),
  ok<SameKeys<TaskPathCheck, Data<task.TaskPathCheck>>>(),
  ok<SameKeys<DeleteResult, Data<task.DeleteResult>>>(),
  ok<SameKeys<DeleteResult['failures'][number], Data<task.DeleteFailure>>>(),
  ok<SameKeys<PreviewURL, Data<convert.PreviewURL>>>(),
] as const
