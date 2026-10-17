import fs from 'node:fs/promises'

// Keep upstream source intact on disk. Adapt the embedded editor's concurrency
// behavior and remove the four AI shortcuts the user explicitly declined.
export const editorProfile = {
  name: 'ffc-office-editor-profile',
  setup(build) {
    build.onLoad({ filter: /vendor[\\/]slides[\\/]renderer[\\/](App\.tsx|TextEditOverlay\.tsx|components[\\/]RibbonHomeTab\.tsx|table-actions\.ts|file-actions\.ts)$/ }, async ({ path }) => {
      let contents = await fs.readFile(path, 'utf8')
      contents = contents.replaceAll('\r\n', '\n')
      if (path.endsWith('RibbonHomeTab.tsx')) {
        const block = /\s*<Group label="Genspark AI">[\s\S]*?<\/Group>/
        if (!block.test(contents)) throw new Error('Upstream AI ribbon structure changed; review the embedding profile')
        contents = contents.replace(block, '')
      } else if (path.endsWith('TextEditOverlay.tsx')) {
        const keydown = "        onKeyDown={(e) => {\n          if (e.key === 'Escape' || e.key === 'F2') {"
        if (!contents.includes(keydown)) throw new Error('Upstream overlay keys changed; review conflict cancellation')
        contents = contents.replace(keydown, "        onKeyDown={(e) => {\n          if (e.key === 'Escape' && e.currentTarget.dataset.ffcConflict === 'true') {\n            e.preventDefault()\n            onCancel()\n            return\n          }\n          if (e.key === 'Escape' || e.key === 'F2') {")
      } else if (path.endsWith('table-actions.ts')) {
        const commit = '  if (updated) ctx.applySlide(ctx.current, updated)\n  ctx.setEditingCell(null)'
        if (!contents.includes(commit)) throw new Error('Upstream cell commit changed; review conflict handling')
        contents = contents.replace(commit, '  if (!updated) return\n' + commit)
      } else if (path.endsWith('file-actions.ts')) {
        const save = '    await flushActiveEdit(ctx)\n    await ctx.flushNotes()'
        if (contents.split(save).length !== 3) throw new Error('Upstream save changed; review active edit handling')
        contents = contents.replace(save, '    await flushActiveEdit(ctx)\n    if (ctx.editingActiveRef.current) return false\n    await ctx.flushNotes()')
        contents = contents.replace(save, '    await flushActiveEdit(ctx)\n    if (ctx.editingActiveRef.current) return\n    await ctx.flushNotes()')
      } else {
        const overlay = '  TextEditOverlay,\n'
        if (!contents.includes(overlay)) throw new Error('Upstream text overlay import changed')
        contents = "import { TextEditOverlay } from '../../../src/text-edit-overlay'\n" + contents.replace(overlay, '')
        const start = contents.indexOf('<div className="stage-ai-bar">')
        const next = contents.indexOf('<div\n                          ref={stageScaleRef}', start)
        const nextCRLF = contents.indexOf('<div\r\n                          ref={stageScaleRef}', start)
        const end = next >= 0 ? next : nextCRLF
        if (start < 0 || end <= start) throw new Error('Upstream AI canvas bar structure changed; review the embedding profile')
        contents = contents.slice(0, start) + contents.slice(end)
        // A version conflict must retain the official rich-text overlay and its
        // draft. Upstream's standalone IPC returns null then dismisses it.
        const commit = /(const updated = await window\.slidesApi\.editText\([\s\S]*?\}\)\n)(      if \(updated\))/
        if (!commit.test(contents.replaceAll('\r\n', '\n'))) throw new Error('Upstream text commit changed; review conflict handling')
        contents = contents.replaceAll('\r\n', '\n').replace(commit, '$1      if (!updated) return\n$2')
        // The shared-session broadcast keeps the page the user is viewing. Agent
        // generation has to follow the page it is drawing or editing instead.
        const stay = `      window.slidesApi.onDeckChanged?.(({ slides: all }) => {
        setSlides(all)
        setCurrent((c) => Math.min(c, Math.max(0, all.length - 1)))`
        if (!contents.includes(stay)) throw new Error('Upstream deck broadcast changed; review live page following')
        contents = contents.replace(stay, `      window.slidesApi.onDeckChanged?.((payload) => {
        const all = payload.slides
        setSlides(all)
        const focus = payload.agentSlide
        const viewing = ctxRef.current
        const agentPage = typeof focus === 'number' && focus >= 0 && focus < all.length ? Math.floor(focus) : -1
        if (agentPage >= 0 && viewing && agentPage !== viewing.current && !viewing.editing && !viewing.editingCell) {
          setCurrent(agentPage)
          setSelectedSlides([agentPage])
          setSelectedIds([])
          setEditing(null)
        } else {
          setCurrent((c) => Math.min(c, Math.max(0, all.length - 1)))
        }`)
      }
      return { contents, loader: path.endsWith('.tsx') ? 'tsx' : 'ts' }
    })
  },
}
