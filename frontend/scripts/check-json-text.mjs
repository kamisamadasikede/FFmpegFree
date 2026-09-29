// node scripts/check-json-text.mjs —— 把 jsonText.check.ts 用 esbuild 打包后运行，失败时退出码 1
import { build } from 'esbuild'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const entry = fileURLToPath(new URL('../src/utils/jsonText.check.ts', import.meta.url))
const dir = mkdtempSync(join(tmpdir(), 'jsontext-'))
const out = join(dir, 'check.mjs')
try {
  await build({ entryPoints: [entry], bundle: true, format: 'esm', platform: 'node', outfile: out, logLevel: 'error' })
  const { runJsonTextChecks } = await import(pathToFileURL(out).href)
  const fails = runJsonTextChecks()
  if (fails.length) {
    console.error(fails.join('\n'))
    process.exit(1)
  }
  console.log('jsonText 自检通过')
} finally {
  rmSync(dir, { recursive: true, force: true })
}
