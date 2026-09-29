// node scripts/check-edit.mjs —— 把 src/utils/editLogic.check.ts 用 esbuild 打包后在 node 里运行（纯逻辑，不需要浏览器），失败时退出码 1
import { build } from 'esbuild'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const root = fileURLToPath(new URL('..', import.meta.url))
const dir = mkdtempSync(join(tmpdir(), 'editcheck-'))
const out = join(dir, 'check.mjs')
// services/wails.ts 在模块加载时读 window.location.search
globalThis.window = { location: { search: '' } }
globalThis.location = globalThis.window.location
try {
  await build({ entryPoints: [join(root, 'src/utils/editLogic.check.ts')], bundle: true, format: 'esm', platform: 'node', outfile: out, logLevel: 'error', alias: { '@': join(root, 'src') }, loader: { '.vue': 'empty' } })
  const { runEditChecks } = await import(pathToFileURL(out).href)
  const fails = runEditChecks()
  if (fails.length) {
    console.error(fails.join('\n'))
    process.exitCode = 1
  } else console.log('edit 纯逻辑自检通过')
} finally {
  rmSync(dir, { recursive: true, force: true })
  process.exit(process.exitCode ?? 0)
}
