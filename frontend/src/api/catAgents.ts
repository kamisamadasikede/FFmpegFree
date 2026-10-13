/**
 * Cat 多服务商（契约 v0.36 / 设计 v0.4.1）。
 * 库值 cat_build|claude_cli|codex_cli；界面「Grok Build」「Claude」「Codex」。
 * 图标这期只用几何标，不嵌官方 logo。
 */
export const CAT_AGENT_BUILD = 'cat_build' as const
export const CAT_AGENT_CLAUDE = 'claude_cli' as const
export const CAT_AGENT_CODEX = 'codex_cli' as const

/** v0.36 可创建的三种 agentKind（会话锁定后不可变） */
export type CatProviderKind = typeof CAT_AGENT_BUILD | typeof CAT_AGENT_CLAUDE | typeof CAT_AGENT_CODEX

/** 含延后种（胶囊置灰「下一期开放」）与历史脏数据展示 */
export type CatAgentKind = CatProviderKind | 'cat_cli' | 'cat_code' | 'cat_agent'

/** 固定顺序：Grok Build → Claude → Codex（设置块 / 关于页 / 欢迎胶囊主排） */
export const CAT_PROVIDER_ORDER: readonly CatProviderKind[] = [CAT_AGENT_BUILD, CAT_AGENT_CLAUDE, CAT_AGENT_CODEX]

export const CAT_PROVIDER_LABEL: Record<CatProviderKind, string> = {
  cat_build: 'Grok Build',
  claude_cli: 'Claude',
  codex_cli: 'Codex',
}

export function isProviderKind(k: string | undefined | null): k is CatProviderKind {
  return k === CAT_AGENT_BUILD || k === CAT_AGENT_CLAUDE || k === CAT_AGENT_CODEX
}

export function normalizeProviderKind(k: string | undefined | null): CatProviderKind {
  return isProviderKind(k) ? k : CAT_AGENT_BUILD
}

export function providerLabel(k: string | undefined | null): string {
  return isProviderKind(k) ? CAT_PROVIDER_LABEL[k] : CAT_PROVIDER_LABEL.cat_build
}
