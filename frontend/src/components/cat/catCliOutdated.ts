/**
 * 契约 6.19.17：CAT_REPLY_FAILED（可重试），detail 四行 key=value：
 *   reason=cli_outdated / agentKind=claude_cli|codex_cli|cat_build / cliVersion=（可空）/ raw=（stderr，换行转义成字面 \n）
 * CLI 版本过旧的行内错误（字段名集中在 CLI_OUTDATED_FIELDS，契约 6.19.17 / v0.43.1）。
 * 不加新错误码：失败的 detail 带 reason=cli_outdated、cliVersion、raw。
 * detail 可能是对象、JSON 字符串，或多行 key=value 字符串（raw 放最后时其后全部算 raw）。
 */
import type { CatAgentKind } from '@/api/cat'
import { providerLabel } from '@/api/catAgents'

export const CLI_OUTDATED_FIELDS = { reason: 'reason', agent: 'agentKind', version: 'cliVersion', raw: 'raw' } as const
export const CLI_OUTDATED_REASON = 'cli_outdated'

export interface CatCliOutdated {
  /** 显示名，同设置页：Claude / Codex / Grok Build */
  cli: string
  version?: string
  raw?: string
}

export const CAT_CLI_OUTDATED_COPY = {
  body: (cli: string) => `${cli} CLI 版本过旧，请升级后重试。`,
  version: (v: string) => `当前版本 ${v}`,
  retry: '重试',
  detail: '查看详情',
  copy: '复制',
  copied: '已复制',
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
    for (const m of head.matchAll(/([A-Za-z_][\w]*)=("([^"]*)"|[^\s;,&]*)/g)) out[m[1]] = m[3] ?? m[2]
    if (rawAt >= 0) {
      const start = line.indexOf(`${CLI_OUTDATED_FIELDS.raw}=`, rawAt) + CLI_OUTDATED_FIELDS.raw.length + 1
      out[CLI_OUTDATED_FIELDS.raw] = [line.slice(start), ...lines.slice(i + 1)].join('\n').trim()
      break
    }
  }
  return out
}

function fromObject(o: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(o)) if (v != null) out[k] = typeof v === 'string' ? v : String(v)
  return out
}

/** detail（对象 / JSON / key=value）→ 平铺字段；v0.46 provider_responses_unsupported 也用这个解析 */
export function parseFailDetailFields(detail: unknown): Record<string, string> {
  if (detail == null || detail === '') return {}
  return typeof detail === 'string' ? fromString(detail) : typeof detail === 'object' ? fromObject(detail as Record<string, unknown>) : {}
}

/** 不是 cli_outdated 就返回 null，界面照旧 */
export function parseCliOutdated(detail: unknown, kind?: CatAgentKind | string): CatCliOutdated | null {
  if (detail == null || detail === '') return null
  const f = typeof detail === 'string' ? fromString(detail) : typeof detail === 'object' ? fromObject(detail as Record<string, unknown>) : {}
  if ((f[CLI_OUTDATED_FIELDS.reason] || '').trim() !== CLI_OUTDATED_REASON) return null
  const version = (f[CLI_OUTDATED_FIELDS.version] || '').trim()
  // raw 里的字面 \n（及 \r\n、\t）还原成真换行；\\ 还原成反斜杠
  const raw = (f[CLI_OUTDATED_FIELDS.raw] || '')
    .replace(/\\(\\|r\\n|n|t)/g, (_, c: string) => (c === '\\' ? '\\' : c === 't' ? '\t' : '\n'))
    .trim()
  const agent = (f[CLI_OUTDATED_FIELDS.agent] || '').trim()
  return { cli: cliLabel(agent || kind), ...(version ? { version } : {}), ...(raw ? { raw } : {}) }
}
