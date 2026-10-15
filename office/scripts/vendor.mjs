// Upstream sources are unmodified; copy only the renderer's runtime import graph.
import fs from 'node:fs'
import path from 'node:path'
const source = path.resolve(process.argv[2] || '')
if (!fs.existsSync(path.join(source, 'packages/pptx-engine/src/index.ts'))) throw new Error('Pass the fixed GenOffice source checkout')
const target = path.resolve(import.meta.dirname, '../vendor')
const visited = new Set()
function copyRenderer(file) {
  if (visited.has(file)) return
  visited.add(file)
  const text = fs.readFileSync(file, 'utf8')
  const rel = path.relative(path.join(source, 'apps/slides/src'), file)
  const out = path.join(target, 'slides', rel)
  fs.mkdirSync(path.dirname(out), { recursive: true })
  fs.copyFileSync(file, out)
  for (const match of text.matchAll(/(?:from\s*|import\s*)['"](\.[^'"]+)['"]/g)) {
    const base = path.resolve(path.dirname(file), match[1])
    const found = [base, base + '.ts', base + '.tsx', path.join(base, 'index.ts')].find(p => fs.existsSync(p) && fs.statSync(p).isFile())
    if (found) copyRenderer(found)
  }
}
// Embed the actual Slides application, including its ribbon and editing overlays.
// Electron's preload is replaced by src/slides-bridge.ts; vendor UI stays unchanged.
for (const folder of ['renderer', 'shared']) {
  fs.cpSync(path.join(source, 'apps/slides/src', folder), path.join(target, 'slides', folder), { recursive: true })
}
for (const name of ['ui', 'i18n', 'ai-provider']) {
  const dest = path.join(target, 'packages', name)
  fs.mkdirSync(dest, { recursive: true })
  fs.copyFileSync(path.join(source, 'packages', name, 'package.json'), path.join(dest, 'package.json'))
  fs.cpSync(path.join(source, 'packages', name, 'src'), path.join(dest, 'src'), { recursive: true })
}
for (const name of ['headless-export', 'safe-external-url']) {
  fs.mkdirSync(path.join(target, 'desktop-utils'), { recursive: true })
  fs.copyFileSync(path.join(source, 'packages/electron-utils/src', name + '.ts'), path.join(target, 'desktop-utils', name + '.ts'))
}
fs.copyFileSync(path.join(source, 'apps/docs/src/renderer/fonts/LICENSE-OFL.txt'), path.join(target, 'LICENSE-OFL.txt'))
copyRenderer(path.join(source, 'apps/slides/src/renderer/App.tsx'))
// Font metrics use the same CJK classifier as the official canvas.
copyRenderer(path.join(source, 'apps/slides/src/shared/cjk-script.ts'))
fs.writeFileSync(path.join(target, 'UPSTREAM.json'), JSON.stringify({
  repository: 'https://github.com/genspark-ai/genoffice',
  commit: '2a607bd49bc341f74a60a8fa852183dfb6ce3c1f',
  license: 'Apache-2.0',
  changes: 'Unmodified engine and canvas sources; ffc host adapters are outside vendor.',
}, null, 2) + '\n')
console.log(`Copied ${visited.size} canvas dependencies`)
