// node scripts/check-api.mjs —— 把 src/api/api.check.ts 用 esbuild 打包后在 node 里运行（模拟层只用定时器和内存，不需要浏览器），失败时退出码 1
import { build } from 'esbuild'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const root = fileURLToPath(new URL('..', import.meta.url))
const entry = join(root, 'src/api/api.check.ts')
const dir = mkdtempSync(join(tmpdir(), 'apicheck-'))
const out = join(dir, 'check.mjs')
// 模拟浏览器环境：services/wails.ts 在模块加载时读 window.location.search
globalThis.window = { location: { search: '' } }
globalThis.location = globalThis.window.location
try {
  await build({ entryPoints: [entry], bundle: true, format: 'esm', platform: 'node', outfile: out, logLevel: 'error', alias: { '@': join(root, 'src') }, loader: { '.vue': 'empty' } })
  const { runApiChecks } = await import(pathToFileURL(out).href)
  const fails = await runApiChecks()
  if (fails.length) {
    console.error(fails.join('\n'))
    process.exitCode = 1
  } else console.log('api 自检通过')
} finally {
  rmSync(dir, { recursive: true, force: true })
  process.exit(process.exitCode ?? 0)
}
