/**
 * 契约 v0.44.4 §6.19.18.9 / v0.44.5–v0.44.6 §6.19.18.7：文案与读路径辅助。
 * 字段名以 wails 生成的 models.ts 为准（Settings.defaultModeByAgent、ConversationDetail.lastTurn、CatTurnState.retryable）。
 */
import { CAT_PROVIDER_ORDER, type CatProviderKind } from '@/api/catAgents'

/** 6.19.18.7 第 6 条定稿文案 */
export const CAT_RETRY_GONE_TEXT = '这条回复已无法重试，请重新发送。'
/** 不能重试时的失败行；可重试时仍用 CAT_COPY.replyFailed「回复没生成出来，请重试。」 */
export const CAT_REPLY_FAILED_FINAL = '回复没生成出来。'
/** 设置页选 danger 时常驻的橙色小字（产品定稿） */
export const CAT_DEFAULT_DANGER_NOTE = '新对话默认全部放开，Cat 助手将不经检查执行命令和修改文件。'

export type CatModeByAgent = Record<CatProviderKind, string>
export const emptyModeByAgent = (): CatModeByAgent => ({ cat_build: '', claude_cli: '', codex_cli: '' })

type Raw = Record<string, unknown>
const obj = (v: unknown): Raw => (v && typeof v === 'object' ? (v as Raw) : {})
const str = (v: unknown) => (typeof v === 'string' ? v : '')

/**
 * 读 Settings.defaultModeByAgent（生成类型已有）。
 * 本地 mock / 旧自检载荷若仍带 defaultApprovalMode，按契约第 3 条给三家各填一份旧值，用时再走 LEGACY_MODE_MAP。
 * 真机 Get 已由后端迁移，不再返回 defaultApprovalMode。
 */
export function readModeByAgent(raw: unknown): CatModeByAgent {
  const r = obj(raw)
  const out = emptyModeByAgent()
  const src = r.defaultModeByAgent
  if (src && typeof src === 'object') {
    for (const k of CAT_PROVIDER_ORDER) {
      const v = str((src as Raw)[k]).trim()
      if (v) out[k] = v
    }
  }
  const any = CAT_PROVIDER_ORDER.some((k) => !!out[k])
  if (!any) {
    const legacy = str(r.defaultApprovalMode).trim()
    if (legacy) for (const k of CAT_PROVIDER_ORDER) out[k] = legacy
  }
  return out
}

/** CatTurnState / 失败事件：retryable === true */
export const readRetryable = (raw: unknown): boolean => obj(raw).retryable === true

/** ConversationDetail.lastTurn：failed && retryable → turnId；缺 lastTurn → '' */
export function readRetryableTurn(rawDetail: unknown): string {
  const last = obj(obj(rawDetail).lastTurn)
  if (str(last.status) !== 'failed' || !readRetryable(last)) return ''
  return str(last.turnId)
}

/** RetryCatTurn → INVALID_ARGUMENT reason=retry_not_allowed */
export function isRetryNotAllowed(err: { code?: string; detail?: unknown }): boolean {
  return err.code === 'INVALID_ARGUMENT' && /reason=retry_not_allowed/.test(typeof err.detail === 'string' ? err.detail : JSON.stringify(err.detail ?? ''))
}
