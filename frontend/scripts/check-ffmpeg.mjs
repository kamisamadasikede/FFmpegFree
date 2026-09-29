// node scripts/check-ffmpeg.mjs —— 把 src/stores/ffmpeg.check.ts 用 esbuild 打包后在 node 里运行（假的 window.go，不需要浏览器），失败时退出码 1
import { build } from 'esbuild'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const root = fileURLToPath(new URL('..', import.meta.url))
const dir = mkdtempSync(join(tmpdir(), 'ffcheck-'))
const out = join(dir, 'check.mjs')
try {
  await build({ entryPoints: [join(root, 'src/stores/ffmpeg.check.ts')], bundle: true, format: 'esm', platform: 'node', outfile: out, logLevel: 'error', alias: { '@': join(root, 'src') }, loader: { '.vue': 'empty' } })
  const mod = await import(pathToFileURL(out).href)
  // store 在模块加载时读 window.go，必须在 import store 之前装好假的 Wails
  const w = mod.installFakeWails()
  globalThis.window = w
  globalThis.location = w.location
  const fails = await mod.runFFmpegChecks(w)
  if (fails.length) {
    console.error(fails.join('\n'))
    process.exitCode = 1
  } else console.log('ffmpeg store 自检通过')
} finally {
  rmSync(dir, { recursive: true, force: true })
  process.exit(process.exitCode ?? 0)
}
