/**
 * 契约 6.19.17：CAT_REPLY_FAILED（可重试），detail 四行 key=value：
 *   reason=cli_outdated / agentKind=claude_cli|codex_cli|cat_build / cliVersion=（可空）/ raw=（stderr，换行转义成字面 \n）
 * CLI 版本过旧的行内错误（字段名集中在 CLI_OUTDATED_FIELDS，契约 6.19.17 / v0.43.1）。
 * 不加新错误码：失败的 detail 带 reason=cli_outdated、cliVersion、raw。
 * detail 可能是对象、JSON 字符串，或多行 key=value 字符串（raw 放最后时其后全部算 raw）。
 */
import type { CatAgentKind } from '@/api/cat'
import { providerLabel } from '@/api/catAgents'
import { t } from '@/i18n'

export const CLI_OUTDATED_FIELDS = { reason: 'reason', agent: 'agentKind', version: 'cliVersion', raw: 'raw' } as const
export const CLI_OUTDATED_REASON = 'cli_outdated'

export interface CatCliOutdated {
  /** 显示名，同设置页：Claude / Codex / Grok Build */
  cli: string
  version?: string
  raw?: string
}

export const CAT_CLI_OUTDATED_COPY = {
  body: (cli: string) => t('cat.cliOutdated.body', { cli: cli }),
  version: (v: string) => t('cat.cliOutdated.version', { v0: v }),
  get retry() { return t('cat.cliOutdated.retry') },
  get detail() { return t('cat.cliOutdated.detail') },
  get copy() { return t('cat.cliOutdated.copy') },
  get copied() { return t('cat.cliOutdated.copied') },
}

/** 库值 → 设置页同款显示名（cat_build = Grok Build） */
export function cliLabel(kind?: CatAgentKind | string): string {
  const k = kind === 'grok_build' || kind === 'grok' ? 'cat_build' : kind === 'claude' ? 'claude_cli' : kind === 'codex' ? 'codex_cli' : kind
  return providerLabel(k)
}

function fromString(s: string): Record<string, string> {
  const t = s.trim()
  if (t.startsWith('{')) {
    try {
      const o = JSON.parse(t)
      if (o && typeof o === 'object') return fromObject(o as Record<string, unknown>)
    } catch { /* 不是 JSON，按 key=value */ }
  }
  const out: Record<string, string> = {}
  const lines = s.split(/\r?\n/)
  for (let i = 0; i < lines.length; i++) {
    // 一行里可能是「reason=cli_outdated cliVersion=2.0.1」，也可能每行一个
    const line = lines[i]
    const rawAt = line.search(new RegExp(`(^|[\\s;,&])${CLI_OUTDATED_FIELDS.raw}=`))
    const head = rawAt >= 0 ? line.slice(0, rawAt) : line
    for (const m of head.matchAll(/([A-Za-z_][\w]*)=("([^"]*)t('cat.misc.s_6a61639612')