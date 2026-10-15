// Copy the official Docs, PDF, HTML, and Markdown renderers without editing them.
// Electron preload stays out; ffc bridges live under src/.
import fs from 'node:fs'
import path from 'node:path'
const source = path.resolve(process.argv[2] || '')
if (!fs.existsSync(path.join(source, 'apps/docs/src/renderer/main.tsx'))) throw new Error('Pass the fixed GenOffice source checkout')
const vendor = path.resolve(import.meta.dirname, '../vendor')
function copyTree(from, to) {
  fs.rmSync(to, { recursive: true, force: true })
  fs.cpSync(from, to, { recursive: true, filter: src => !src.split(path.sep).includes('node_modules') })
}
for (const app of ['docs', 'pdf', 'html', 'markdown', 'sheets']) {
  for (const folder of ['renderer', 'shared']) {
    copyTree(path.join(source, 'apps', app, 'src', folder), path.join(vendor, 'apps', app, 'src', folder))
  }
}
copyTree(path.join(source, 'apps/sheets/src/ai'), path.join(vendor, 'apps/sheets/src/ai'))
const gateway = path.join(source, 'packages/xlsx-gateway')
const gatewayOut = path.join(vendor, 'packages/xlsx-gateway')
fs.rmSync(gatewayOut, { recursive: true, force: true })
fs.mkdirSync(gatewayOut, { recursive: true })
fs.copyFileSync(path.join(gateway, 'package.json'), path.join(gatewayOut, 'package.json'))
copyTree(path.join(gateway, 'src'), path.join(gatewayOut, 'src'))
const script = path.join(source, 'packages/pdf2docx/src/script.ts')
const scriptOut = path.join(vendor, 'packages/pdf2docx/src/script.ts')
fs.mkdirSync(path.dirname(scriptOut), { recursive: true })
fs.copyFileSync(script, scriptOut)
fs.writeFileSync(path.join(vendor, 'packages/pdf2docx/package.json'), JSON.stringify({
  name: '@genoffice/pdf2docx', version: '0.1.0', private: true, license: 'Apache-2.0', type: 'module',
  description: 'Script classifier imported by the official PDF page. The rest of the conversion pipeline is not embedded.',
}, null, 2) + '\n')
const upstream = path.join(vendor, 'UPSTREAM.json')
const current = JSON.parse(fs.readFileSync(upstream, 'utf8'))
current.changes = 'Unmodified engine and canvas sources; Docs, PDF, HTML, Markdown, and Sheets renderers are copied unmodified. ffc host adapters are outside vendor.'
fs.writeFileSync(upstream, JSON.stringify(current, null, 2) + '\n')
console.log('Copied official Docs, PDF, HTML, Markdown, and Sheets renderers')
