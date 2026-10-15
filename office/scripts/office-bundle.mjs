import fs from 'node:fs/promises'
import path from 'node:path'

// Host-only load hooks. Vendor files stay unmodified on disk.
export const officeBundle = {
  name: 'ffc-office-bundle',
  setup(build) {
    build.onResolve({ filter: /\?url$/ }, async args => {
      const spec = args.path.replace(/\?url$/, '')
      if (spec.includes('pdf.worker')) return { path: args.path, namespace: 'ffc-url' }
      const resolved = await build.resolve(spec, { resolveDir: args.resolveDir, kind: 'import-statement' })
      if (resolved.errors.length) return { errors: resolved.errors }
      return { path: resolved.path, namespace: 'ffc-asset-url' }
    })
    build.onResolve({ filter: /\?raw$/ }, args => ({
      path: path.resolve(args.resolveDir, args.path.replace(/\?raw$/, '')),
      namespace: 'ffc-raw',
    }))
    build.onLoad({ filter: /.*/, namespace: 'ffc-url' }, args => {
      const spec = args.path.replace(/\?url$/, '')
      const href = spec.includes('pdf.worker') ? '/pdf.worker.min.mjs' : ''
      return { contents: `export default ${JSON.stringify(href)}`, loader: 'js' }
    })
    build.onLoad({ filter: /.*/, namespace: 'ffc-asset-url' }, async args => {
      const data = await fs.readFile(args.path)
      const ext = path.extname(args.path).toLowerCase()
      const mime = ext === '.ttf' ? 'font/ttf' : ext === '.woff' ? 'font/woff' : ext === '.woff2' ? 'font/woff2' : 'application/octet-stream'
      return { contents: `export default ${JSON.stringify('data:' + mime + ';base64,' + data.toString('base64'))}`, loader: 'js' }
    })
    build.onLoad({ filter: /.*/, namespace: 'ffc-raw' }, async args => {
      const text = await fs.readFile(args.path, 'utf8')
      return { contents: `export default ${JSON.stringify(text)}`, loader: 'js' }
    })
    build.onLoad({ filter: /parse-off-thread\.ts$/ }, async ({ path }) => {
      let contents = await fs.readFile(path, 'utf8')
      const needle = "worker = new Worker(new URL('./parse-worker.ts', import.meta.url), { type: 'module' })"
      if (!contents.includes(needle)) throw new Error('Upstream parse worker changed; review the embedding profile')
      return { contents: contents.replace(needle, "throw new Error('inline')"), loader: 'ts' }
    })
  },
}
