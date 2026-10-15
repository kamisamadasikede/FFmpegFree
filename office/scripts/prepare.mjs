import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
const root = path.resolve(import.meta.dirname, '..')
if (!fs.existsSync(path.join(root, 'node_modules/esbuild'))) {
  if (!process.env.npm_execpath) throw new Error('Run npm ci --ignore-scripts in office before building')
  const installed = spawnSync(process.execPath, [process.env.npm_execpath, 'ci', '--ignore-scripts', '--no-audit', '--no-fund'], { cwd: root, stdio: 'inherit', windowsHide: true })
  if (installed.status !== 0) process.exit(installed.status || 1)
}
await import('./build.mjs')
