/**
 * SubmitSources 的返回与“副本没就绪”的提示（契约 v0.24 / v0.24.1 §6.15.4 第 6 条；包 20 最小接入，完整的复制进度随 v0.24 前端）。
 * 后端先把添加的文件复制到上传目录：选中的行一部分还没复制好时只提交就绪的，其余放进 skipped；一行都没就绪时整体 TASK_CONFLICT。
 * 用户文案说“准备中”，不说“复制”（产品经理 10-08）。
 */
import type { TaskError } from '@/stores/tasks'

export interface SkippedSource {
  sourceId: string
  /** "copying" | "copy_failed" | "copy_canceled"（不认识的值按“没能准备好”处理） */
  reason: string
}
export interface ConvertSubmitResult<T> {
  tasks: T[]
  skipped: SkippedSource[]
}

/** 兼容旧形状：v0.24 之前直接返回 Task[]；Go 的 nil 切片是 null */
export function submitResultOf<T>(raw: unknown): ConvertSubmitResult<T> {
  if (Array.isArray(raw)) return { tasks: raw as T[], skipped: [] }
  const r = (raw ?? {}) as { tasks?: T[] | null; skipped?: SkippedSource[] | null }
  return { tasks: Array.isArray(r.tasks) ? r.tasks : [], skipped: Array.isArray(r.skipped) ? r.skipped.filter((s) => !!s && typeof s.sourceId === 'string') : [] }
}

/** 产品经理定稿（10-08；D5：复制失败的行有“重试”按钮后，“请重新添加”改成“请在列表里点“重试””）：普通（info）toast，原样使用 */
export const SUBMIT_COPYING_TEXT = '文件还在准备中，准备好后再点转换。'
export const SUBMIT_COPY_FAILED_TEXT = '文件没能准备好，请在列表里点“重试”后再转换。'
export const skippedCopyingText = (n: number) => `有 ${n} 个文件还在准备中，已转换其余文件。准备好后再点转换。`
export const skippedFailedText = (n: number) => `有 ${n} 个文件没能准备好，已转换其余文件。请在列表里点“重试”后再转换。`
/** 两类都有时，第二句不再重复“已转换其余文件” */
export const skippedFailedAlsoText = (n: number) => `另有 ${n} 个文件没能准备好，请在列表里点“重试”。`

/** 部分跳过的提示：还在复制（copying）/ 复制失败或已取消（copy_failed、copy_canceled，及不认识的值）分开说，按个数，不带文件名 */
export function skippedNotice(skipped: SkippedSource[]): string {
  const copying = skipped.filter((s) => s.reason === 'copying').length
  const broken = skipped.length - copying
  if (copying && broken) return skippedCopyingText(copying) + skippedFailedAlsoText(broken)
  if (copying) return skippedCopyingText(copying)
  if (broken) return skippedFailedText(broken)
  return ''
}

/** 整体失败里属于“副本没就绪”的（TASK_CONFLICT reason=copying / copy_failed）给短提示；其余返回 ''（照旧显示错误） */
export function submitCopyErrorText(e: Pick<TaskError, 'code' | 'detail'>): string {
  if (e.code !== 'TASK_CONFLICT') return ''
  const m = /^reason=([a-z0-9_]+)$/.exec((e.detail ?? '').split('\n')[0].trim())
  if (m?.[1] === 'copying') return SUBMIT_COPYING_TEXT
  if (m?.[1] === 'copy_failed') return SUBMIT_COPY_FAILED_TEXT
  return ''
}
