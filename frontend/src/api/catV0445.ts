/**
 * 契约 v0.44.4 §6.19.18.9 / v0.44.5 §6.19.18.7 第 7 条：新字段名唯一一处映射。
 * 后端 PR 合入、wails 绑定重生成后，若字段名有出入只改这里。
 */
import { CAT_PROVIDER_ORDER, type CatProviderKind } from '@/api/catAgents'

export const V0445_FIELDS = {
  /** CatSettings.defaultModeByAgent：key = agentKind，value = 该家 modes[].id（"" = 该家默认） */
  defaultModeByAgent: 'defaultModeByAgent',
  /** v0.44.4 起废弃，只读兼容（Save 不再写） */
  legacyApprovalMode: 'defaultApprovalMode',
  /** turn / 失败事件上的 retryable */
  retryable: 'retryable',
  /** GetCatConversation 里的 turn 列表（后端实际形状待定：turns[] 或 lastTurn） */
  turns: 'turns',
  lastTurn: 'lastTurn',
  turnId: 'turnId',
  turnStatus: 'status',
} as const

/** 6.19.18.7 第 6 条定稿文案 */
export const CAT_RETRY_GONE_TEXT = '这条回复已无法重试，请重新发送。'
/** 设置页选 danger 时常驻的橙色小字（产品 10-14 定稿） */
export const CAT_DEFAULT_DANGER_NOTE = '新对话默认全部放开，Cat 助手将不经检查执行命令和修改文件。'

type Raw = Record<string, unknown>
const obj = (v: unknown): Raw => (v && typeof v === 'object' ? (v as Raw) : {})
const str = (v: unknown) => (typeof v === 'string' ? v : '')

export type CatModeByAgent = Record<CatProviderKind, string>
export const emptyModeByAgent = (): CatModeByAgent => ({ cat_build: '', claude_cli: '', codex_cli: '' })

/**
 * 读设置：有 defaultModeByAgent 用它；只有旧 defaultApprovalMode 时原样给三家各填一份旧值，
 * 由 resolveModeId（各家「旧值」列 LEGACY_MODE_MAP，认不出 → 该家最保守默认）在用时映射。
 */
export function readModeByAgent(raw: unknown): { byAgent: CatModeByAgent; legacy: string } {
  const r = obj(raw)
  const out = emptyModeByAgent()
  const src = r[V0445_FIELDS.defaultModeByAgent]
  const legacy = str(r[V0445_FIELDS.legacyApprovalMode]).trim()
  let any = false
  if (src && typeof src === 'object') {
    for (const k of CAT_PROVIDER_ORDER) {
      const v = str((src as Raw)[k]).trim()
      if (v) { out[k] = v; any = true }
    }
  }
  if (!any && legacy) for (const k of CAT_PROVIDER_ORDER) out[k] = legacy
  return { byAgent: out, legacy }
}

/** 一个 turn（或失败事件载荷）上的 retryable；缺字段 = false（应用重启后同样为 false） */
export const readRetryable = (raw: unknown): boolean => obj(raw)[V0445_FIELDS.retryable] === true

/** GetCatConversation：最后一轮若 status=failed 且 retryable=true → 返回其 turnId，否则 '' */
export function readRetryableTurn(rawDetail: unknown): string {
  const r = obj(rawDetail)
  const F = V0445_FIELDS
  const list = Array.isArray(r[F.turns]) ? (r[F.turns] as unknown[]) : []
  const last = obj(list.length ? list[list.length - 1] : r[F.lastTurn])
  if (str(last[F.turnStatus]) !== 'failed' || !readRetryable(last)) return ''
  return str(last[F.turnId]) || str(last.id)
}

/** RetryCatTurn 报 INVALID_ARGUMENT reason=retry_not_allowed */
export function isRetryNotAllowed(err: { code?: string; detail?: unknown }): boolean {
  return err.code === 'INVALID_ARGUMENT' && /reason=retry_not_allowed/.test(typeof err.detail === 'string' ? err.detail : JSON.stringify(err.detail ?? ''))
}
