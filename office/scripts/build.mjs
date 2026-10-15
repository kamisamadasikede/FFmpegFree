import { build } from 'esbuild'
import fs from 'node:fs/promises'
import path from 'node:path'
import { createHash } from 'node:crypto'
import { editorProfile } from './editor-profile.mjs'
const root = path.resolve(import.meta.dirname, '..')
const out = path.resolve(root, '../internal/genoffice/runtime')
await fs.mkdir(out, { recursive: true })
const engine = await build({
  absWorkingDir: root, entryPoints: ['src/engine.ts'], outfile: path.join(out, 'engine.mjs'),
  bundle: true, platform: 'node', format: 'esm', target: 'node22',
  banner: { js: "import { createRequire as __ffcCreateRequire } from 'node:module'; const require = __ffcCreateRequire(import.meta.url);" },
  loader: { '.md': 'text' }, minify: true, legalComments: 'eof', metafile: true,
})
const editor = await build({
  absWorkingDir: root, entryPoints: ['src/editor.tsx'], outfile: path.join(out, 'editor.js'),
  bundle: true, platform: 'browser', format: 'iife', target: 'chrome120',
  jsx: 'automatic',
  plugins: [editorProfile],
  external: ['node:*'], define: { 'process.env.NODE_ENV': '"production"' },
  loader: { '.md': 'text', '.ttf': 'dataurl', '.woff': 'dataurl', '.woff2': 'dataurl', '.svg': 'dataurl', '.png': 'dataurl', '.jpg': 'dataurl' },
  alias: Object.fromEntries(['headless-export', 'safe-external-url'].map(name => ['@genoffice/electron-utils/' + name, path.join(root, 'vendor/desktop-utils', name + '.ts')])),
  minify: true, legalComments: 'eof', metafile: true,
})
await fs.copyFile(path.join(root, 'src/editor.html'), path.join(out, 'editor.html'))
await fs.copyFile(path.join(root, 'vendor/LICENSE'), path.join(out, 'LICENSE-GENOFFICE.txt'))
await fs.copyFile(path.join(root, 'vendor/NOTICE'), path.join(out, 'NOTICE-GENOFFICE.txt'))
await fs.copyFile(path.join(root, 'vendor/LICENSE-OFL.txt'), path.join(out, 'LICENSE-OFL.txt'))
const packages = new Map()
for (const input of [...Object.keys(engine.metafile.inputs), ...Object.keys(editor.metafile.inputs)]) {
  const match = input.replaceAll('\\', '/').match(/node_modules\/((?:@[^/]+\/)?[^/]+)/)
  if (!match || packages.has(match[1])) continue
  const folder = path.join(root, 'node_modules', match[1])
  const metadata = JSON.parse(await fs.readFile(path.join(folder, 'package.json'), 'utf8'))
  const licenseFile = (await fs.readdir(folder)).find(name => /^(license|licence|copying)(\.|$)/i.test(name))
  const license = licenseFile ? await fs.readFile(path.join(folder, licenseFile), 'utf8') : `License: ${metadata.license}`
  packages.set(match[1], `${metadata.name}@${metadata.version}\n${license}`)
}
await fs.writeFile(path.join(out, 'THIRD-PARTY-NOTICES.txt'), [...packages.values()].join('\n\n----\n\n'))
const files = ['engine.mjs', 'editor.js', 'editor.css', 'editor.html', 'LICENSE-GENOFFICE.txt', 'NOTICE-GENOFFICE.txt', 'LICENSE-OFL.txt', 'THIRD-PARTY-NOTICES.txt']
const integrity = Object.fromEntries(await Promise.all(files.map(async file => [file, createHash('sha256').update(await fs.readFile(path.join(out, file))).digest('hex')])))
await fs.writeFile(path.join(out, 'manifest.json'), JSON.stringify({ version: 1, upstream: '2a607bd49bc341f74a60a8fa852183dfb6ce3c1f', files, integrity }, null, 2) + '\n')
console.log('Built ffc GenOffice engine and canvas')
