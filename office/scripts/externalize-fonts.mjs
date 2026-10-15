// Move repeated data-URL fonts out of the built stylesheets.
// Each unique font is written once under runtime/fonts and referenced by /fonts/<hash>.<ext>.
import { createHash } from 'node:crypto'
import fs from 'node:fs/promises'
import path from 'node:path'

const extensions = new Set(['ttf', 'woff', 'woff2'])

export async function externalizeFonts(outDir) {
  const fontDir = path.join(outDir, 'fonts')
  await fs.mkdir(fontDir, { recursive: true })
  const names = (await fs.readdir(outDir)).filter(name => name.endsWith('.css'))
  const written = new Set()
  const changed = []
  for (const name of names) {
    const file = path.join(outDir, name)
    const text = await fs.readFile(file, 'utf8')
    let next = ''
    let cursor = 0
    let count = 0
    while (cursor < text.length) {
      const at = text.indexOf('data:font/', cursor)
      if (at < 0) {
        next += text.slice(cursor)
        break
      }
      const lead = text.slice(Math.max(0, at - 6), at)
      const headerEnd = text.indexOf(';base64,', at)
      const ext = headerEnd > at && headerEnd - at < 24 ? text.slice(at + 'data:font/'.length, headerEnd) : ''
      if (!lead.includes('url(') || !extensions.has(ext)) {
        next += text.slice(cursor, at + 'data:font/'.length)
        cursor = at + 'data:font/'.length
        continue
      }
      let end = headerEnd + ';base64,'.length
      while (end < text.length && isBase64(text.charCodeAt(end))) end += 1
      const bytes = Buffer.from(text.slice(headerEnd + ';base64,'.length, end), 'base64')
      if (bytes.length < 64) {
        next += text.slice(cursor, end)
        cursor = end
        continue
      }
      const font = createHash('sha256').update(bytes).digest('hex').slice(0, 16) + '.' + ext
      if (!written.has(font)) {
        await fs.writeFile(path.join(fontDir, font), bytes)
        written.add(font)
      }
      next += text.slice(cursor, at) + '/fonts/' + font
      cursor = end
      count += 1
    }
    if (count > 0) {
      await fs.writeFile(file, next)
      changed.push(name)
    }
  }
  return { changed, fonts: written.size }
}

function isBase64(code) {
  return (code >= 48 && code <= 57) || (code >= 65 && code <= 90) || (code >= 97 && code <= 122) || code === 43 || code === 47 || code === 61
}

if (path.resolve(process.argv[1] || '') === path.resolve(import.meta.filename)) {
  const out = path.resolve(import.meta.dirname, '../../internal/genoffice/runtime')
  const result = await externalizeFonts(out)
  const manifestPath = path.join(out, 'manifest.json')
  const manifest = JSON.parse(await fs.readFile(manifestPath, 'utf8'))
  for (const name of result.changed) {
    manifest.integrity[name] = createHash('sha256').update(await fs.readFile(path.join(out, name))).digest('hex')
  }
  await fs.writeFile(manifestPath, JSON.stringify(manifest, null, 2) + '\n')
  console.log('Externalized ' + result.fonts + ' fonts from ' + (result.changed.join(', ') || 'no stylesheet'))
}
