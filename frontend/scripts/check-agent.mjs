import { build } from 'esbuild'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const root = fileURLToPath(new URL('..', import.meta.url))
const dir = mkdtempSync(join(tmpdir(), 'agentcheck-'))
const out = join(dir, 'check.mjs')
globalThis.window = { location: { search: '' } }
globalThis.location = globalThis.window.location
try {
  await build({
    stdin: {
      resolveDir: root,
      contents: `
        import { catCommandsChecks } from './src/views/cat/catCommands.check'
        import { catThinkChecks } from './src/views/cat/catThink.check'
        import { catDraftsChecks } from './src/views/cat/catDrafts.check'
        import { catCLIUpdatesChecks } from './src/views/cat/catCLIUpdates.check'
        import { catFollowChecks } from './src/views/cat/catFollow.check'
        import { catPendingChecks } from './src/views/cat/catPending.check'
        import { catStopChecks } from './src/views/cat/catStop.check'
        import { catTypewriterChecks } from './src/components/cat/catTypewriter.check'
        import { catStreamChecks } from './src/api/catStream.check'
        import { catDetectChecks } from './src/api/catDetect.check'
        export async function run() {
          const failures = []
          let count = 0
          const eq = (name, got, want) => {
            count++
            if (JSON.stringify(got) !== JSON.stringify(want)) failures.push(name + ': ' + JSON.stringify(got) + ' != ' + JSON.stringify(want))
          }
          await catThinkChecks(eq)
          catCommandsChecks(eq)
          catDraftsChecks(eq)
          await catCLIUpdatesChecks(eq)
          for (const check of [catFollowChecks, catPendingChecks, catStopChecks, catTypewriterChecks, catStreamChecks, catDetectChecks]) check(eq)
          return { failures, count }
        }`,
    },
    bundle: true, format: 'esm', platform: 'node', outfile: out, logLevel: 'error',
    alias: { '@': join(root, 'src') }, loader: { '.vue': 'empty' }, define: { 'import.meta.env.DEV': 'false' },
  })
  const { failures, count } = await (await import(pathToFileURL(out).href)).run()
  if (failures.length) { console.error(failures.join('\n')); process.exitCode = 1 }
  else console.log(`Agent checks passed (${count} assertions)`)
} catch (e) { console.error(e); process.exitCode = 1 }
finally { rmSync(dir, { recursive: true, force: true }); process.exit(process.exitCode ?? 0) }
