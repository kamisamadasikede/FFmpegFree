import { parseCliOutdated } from './catCliOutdated'
export function catCliOutdatedChecks(eq0: (n: string, a: unknown, b: unknown) => void) {
const eq = (n: string, a: unknown, b: unknown) => eq0('CLI 过旧 ' + n, JSON.stringify(a), JSON.stringify(b))
eq('obj', parseCliOutdated({ reason: 'cli_outdated', cliVersion: '2.0.1', raw: 'x' }, 'claude_cli'), { cli: 'Claude', version: '2.0.1', raw: 'x' })
eq('kv', parseCliOutdated('reason=cli_outdated\ncliVersion=0.9.0\nraw=error: unknown a\nline2', 'codex_cli'), { cli: 'Codex', version: '0.9.0', raw: 'error: unknown a\nline2' })
eq('json', parseCliOutdated('{"reason":"cli_outdated","raw":"r"}', 'cat_build'), { cli: 'Grok Build', raw: 'r' })
eq('contract', parseCliOutdated('reason=cli_outdated\nagentKind=codex_cli\ncliVersion=\nraw=error: bad\\n\\nUsage: x', 'claude_cli'), { cli: 'Codex', raw: 'error: bad\n\nUsage: x' })
eq('cat_build', parseCliOutdated('reason=cli_outdated\nagentKind=cat_build\ncliVersion=\nraw=x')?.cli, 'Grok Build')
eq('other', parseCliOutdated('reason=auth', 'claude_cli'), null)
eq('empty', parseCliOutdated(undefined), null)
}
