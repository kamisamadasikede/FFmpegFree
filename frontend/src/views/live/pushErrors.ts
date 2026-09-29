import type { AppError } from '@/api/call'
import { errorMessages, liveSourceGoneText, liveFailureMessage, liveStartErrorLine, liveUrlInvalidText, taskConflictText, LIVE_PUSH_REJECTED_TEXT, LIVE_SRT_PASSPHRASE_TEXT } from '@/errors/errorMessages'

/** 推流表单上一次只显示一条错误：地址类 → 地址框下；口令类 → 口令框下；其余表单级（设计稿 v0.2 §7 第 4 条） */
export interface PushFormError {
  where: 'addr' | 'key' | 'form' | 'source'
  text: string
}

/**
 * Start* 同步错误 / 任务连接阶段失败 → 表单错误（文案全部来自 errors/errorMessages.ts，不含地址 / 口令 / 推流码）。
 * scheme 只是兜底（detail 首行 scheme= 优先，见 liveFailureMessage）。
 */
export function pushErrorToForm(err: Pick<AppError, 'code' | 'message' | 'detail' | 'reason' | 'scheme'> & { kind?: string }, scheme = ''): PushFormError {
  switch (err.code) {
    case 'LIVE_URL_INVALID':
      return { where: 'addr', text: liveUrlInvalidText(err.reason) }
    case 'TASK_CONFLICT':
      return { where: err.reason === 'duplicate_url' ? 'addr' : 'form', text: taskConflictText(err.reason) }
    case 'LIVE_PUSH_REJECTED':
      return { where: 'key', text: LIVE_PUSH_REJECTED_TEXT }
    case 'INVALID_ARGUMENT':
      if (err.reason === 'srt_passphrase_length') return { where: 'key', text: LIVE_SRT_PASSPHRASE_TEXT }
      break
    case 'UNSUPPORTED':
    case 'LIVE_CONNECT_FAILED':
      return { where: 'form', text: liveStartErrorLine(err, { scheme })?.description ?? '' }
    case 'LIVE_SOURCE_GONE':
      return { where: 'source', text: liveSourceGoneText(err.kind) } // 显示在来源选择器下方；不带窗口标题，不显示 detail
    case 'SCREEN_PERMISSION_DENIED':
    case 'UNSUPPORTED_PLATFORM':
      return { where: 'form', text: errorMessages[err.code].description }
  }
  return { where: 'form', text: liveFailureMessage(err, scheme) || err.message }
}
