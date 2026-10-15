import { createHash, randomUUID } from 'node:crypto'
import fs from 'node:fs/promises'
import path from 'node:path'
import { createInterface } from 'node:readline'
import { pathToFileURL } from 'node:url'
import {
  openPptx, createBlankPptx, savePptx, commitSaved, extractMergeSlideSource,
  elementDurableId, elementCNvPrId, slideDurableId, type OpenedPptx,
} from '@genoffice/pptx-engine'
import { runTxn, opGuide, opGuideCatalog, type Op } from '@genoffice/pptx-ops'
import { buildRenderSlide, HeuristicMetrics, OpentypeMetrics, imageSizeFromBytes } from '@genoffice/pptx-render'
import { findSystemFont } from '@genoffice/font-metrics'
import { parsePageSpec, buildPagePptx, auditSlideFindings } from '@genoffice/pipelines/slides'
import * as opentype from 'opentype.js'
import { syncTextAutofit } from './autofit'
import { editorCommand } from './editor-host'

const MAX_FILE = 50 * 1024 * 1024
const MAX_HISTORY = 12
const sessions = new Map<string, Session>()
const paths = new Map<string, string>()
const historyBudget = 48 * 1024 * 1024
const fontCache = new Map<string, any>()
const missingFonts = new Set<string>()
const metrics = new OpentypeMetrics(({ fontFamily: family, bold, italic }) => {
  const key = `${family}/${bold}/${italic}`
  if (fontCache.has(key)) { if (!fontCache.get(key)) missingFonts.add(family); return fontCache.get(key) }
  let font
  try {
    const bytes = findSystemFont(`${family}${bold ? ' Bold' : ''}${italic ? ' Italic' : ''}`, family)
    if (bytes) font = opentype.parse(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength))
  } catch { /* the fallback is reported to the user */ }
  if (!font) missingFonts.add(family)
  fontCache.set(key, font)
  return font
}, new HeuristicMetrics())

type Session = {
  id: string; file: string; root: string; opened?: OpenedPptx; revision: number; savedRevision: number
  sequence: number; disk: string; state: string; error: string; warnings: string[]; renders: any[]
  undo: Uint8Array[]; redo: Uint8Array[]; requests: Map<string, { fingerprint: string; result: any }>
  changedSlides: number[]; chain: Promise<any>; saveTimer?: ReturnType<typeof setTimeout>; autoSave?: boolean
}
class OfficeError extends Error {
  constructor(public code: string, message: string, public detail?: any) { super(message) }
}
function fail(code: string, text: string, detail?: any): never { throw new OfficeError(code, text, detail) }
function queue<T>(s: Session, work: () => Promise<T>): Promise<T> {
  const pending = s.chain.catch(() => undefined).then(work)
  s.chain = pending.catch(() => undefined)
  return pending
}
function keyOf(file: string) { return process.platform === 'win32' ? file.toLowerCase() : file }
function inside(root: string, file: string) {
  const rel = path.relative(root, file)
  return !path.isAbsolute(rel) && rel !== '..' && !rel.startsWith('..' + path.sep)
}
async function authorize(root: string, file: string) {
  if (!path.isAbsolute(root) || !path.isAbsolute(file) || !inside(root, file)) fail('PATH_DENIED', '文件不在授权工作区中')
  const canonicalRoot = await fs.realpath(root)
  let probe = file
  while (probe !== path.dirname(probe)) {
    try {
      const resolved = await fs.realpath(probe)
      if (!inside(canonicalRoot, resolved) && resolved !== canonicalRoot) fail('PATH_DENIED', '文件链接指向工作区外部')
      return
    } catch (e: any) { if (e.code !== 'ENOENT') throw e }
    probe = path.dirname(probe)
  }
  fail('PATH_DENIED', '无法验证文件目录')
}
async function diskVersion(s: Session) {
  await authorize(s.root, s.file)
  try {
    const st = await fs.stat(s.file)
    if (!st.isFile() || st.size > MAX_FILE) fail('TOO_LARGE', 'PPT 文件大小超过 50 MB 或不是普通文件')
    return `${st.size}:${st.mtimeMs}`
  } catch (e: any) { if (e.code === 'ENOENT') return ''; throw e }
}
function changed(s: Session) { s.sequence++ }
function summary(s: Session) {
  return { documentId: s.id, revision: s.revision, savedRevision: s.savedRevision, sequence: s.sequence,
    state: s.state, error: s.error, warnings: s.warnings, slideCount: s.opened?.deck.slides.length ?? 0,
    changedSlides: s.changedSlides, canUndo: s.undo.length > 0, canRedo: s.redo.length > 0 }
}
function render(s: Session) {
  missingFonts.clear()
  if (!s.opened) { s.renders = []; return }
  const opened = s.opened
  stableIds(opened)
  s.renders = opened.deck.slides.map((slide, index) => buildRenderSlide(slide, opened.deck.size, {
    fitWidthPx: 1280, metrics, slideNo: index + 1,
    media: (ref: string) => {
      const bytes = opened.archive.readBytes(ref)
      if (!bytes) return undefined
      const ext = path.extname(ref).toLowerCase()
      const mime: Record<string, string> = { '.png': 'image/png', '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg', '.gif': 'image/gif', '.webp': 'image/webp', '.svg': 'image/svg+xml', '.bmp': 'image/bmp' }
      if (!mime[ext]) {
        const warning = `此图片格式暂不支持实时预览：${ext || ref}，可使用静态预览核对`
        if (!s.warnings.includes(warning)) s.warnings.push(warning)
        return undefined
      }
      return `data:${mime[ext]};base64,${Buffer.from(bytes).toString('base64')}`
    },
  }))
  if (missingFonts.size) s.warnings = [...s.warnings.filter(w => !w.startsWith('字体度量')), `字体度量使用替代：${[...missingFonts].slice(0, 8).join('、')}`]
}
// The official UI keeps sourceId in selection/edit overlays. Parsing mints new
// transient IDs, so use the upstream cNvPr alias (stable across save/undo) there.
// Agent tools may still address the stronger creationId durable form.
function stableIds(opened: OpenedPptx, records?: any[]) {
  const aliases = new Map<string, string>()
  const visit = (el: any) => {
    const stable = elementCNvPrId(el) || elementDurableId(el) || el.id
    aliases.set(el.id, stable); el.id = stable
    el.children?.forEach(visit)
  }
  opened.deck.slides.forEach(slide => slide.elements.forEach(visit))
  for (const record of records ?? []) {
    if (record.created) record.created = record.created.map((id: string) => aliases.get(id) || id)
    if (record.after?.elementId) record.after.elementId = aliases.get(record.after.elementId) || record.after.elementId
  }
}
async function refresh(s: Session) {
  const latest = await diskVersion(s)
  if (latest === s.disk) return
  if (s.opened && s.revision !== s.savedRevision) {
    if (s.state !== 'conflict') { s.state = 'conflict'; s.error = '文件已被外部应用修改，请先保存副本或重新打开'; changed(s) }
    return
  }
  if (!latest) {
    s.disk = ''; s.state = 'waiting'; s.error = '等待文件写入'; changed(s); return
  }
  try {
    const opened = await openPptx(await fs.readFile(s.file))
    if (opened.deck.slides.length > 500) fail('TOO_LARGE', '暂不支持超过 500 页的演示文稿')
    s.opened = opened; s.disk = latest; s.revision++; s.savedRevision = s.revision
    s.undo = []; s.redo = []; s.state = 'ready'; s.error = ''; s.warnings = []; render(s); changed(s)
  } catch (e: any) {
    s.state = 'writing'; s.error = '文件尚未完整写入或无法解析：' + e.message; changed(s)
  }
}
function historyPush(list: Uint8Array[], bytes: Uint8Array) {
  list.push(bytes)
  while (list.length > MAX_HISTORY || list.reduce((n, b) => n + b.length, 0) > historyBudget) list.shift()
}
function ensureEdit(s: Session, request: any) {
  if (!s.opened) fail('NOT_READY', '请先调用 open 创建文档')
  if (s.state === 'conflict') fail('FILE_CONFLICT', s.error, summary(s))
  if (!Number.isInteger(request.expectedRevision) || request.expectedRevision !== s.revision) fail('REVISION_CONFLICT', '文档已变化，请重新读取后修改', summary(s))
  if (typeof request.requestId !== 'string' || !request.requestId.trim() || request.requestId.length > 128) fail('INVALID_REQUEST', '修改需要唯一 requestId')
}
function replay(s: Session, req: any) {
  if (!req.requestId) return undefined
  const hit = s.requests.get(req.requestId)
  if (!hit) return undefined
  if (hit.fingerprint !== JSON.stringify(req)) fail('REQUEST_CONFLICT', 'requestId 已被其他内容使用')
  return hit.result
}
function remember(s: Session, req: any, result: any) {
  s.requests.set(req.requestId, { fingerprint: JSON.stringify(req), result })
  if (s.requests.size > 256) s.requests.delete(s.requests.keys().next().value!)
}
const recoveryRoot = process.env.FFC_OFFICE_RECOVERY || ''
async function writeAtomic(file: string, bytes: Uint8Array, exclusive = false) {
  await fs.mkdir(path.dirname(file), { recursive: true })
  const tmp = path.join(path.dirname(file), `.${path.basename(file)}.${randomUUID()}.tmp`)
  try {
    await fs.writeFile(tmp, bytes, { flag: 'wx', mode: 0o600 })
    const handle = await fs.open(tmp, 'r+')
    try { await handle.sync() } finally { await handle.close() }
    for (let attempt = 0; ; attempt++) {
      try { if (exclusive) await fs.link(tmp, file); else await fs.rename(tmp, file); break }
      catch (e: any) {
        if (attempt >= 3 || !['EPERM', 'EBUSY', 'EACCES'].includes(e.code)) throw e
        await new Promise(r => setTimeout(r, 120 * (attempt + 1)))
      }
    }
  } finally { await fs.rm(tmp, { force: true }).catch(() => undefined) }
}
async function save(s: Session) {
  if (!s.opened) fail('NOT_READY', '没有可保存的文档')
  if (await diskVersion(s) !== s.disk || s.state === 'conflict') fail('FILE_CONFLICT', '外部文件版本已变化，未覆盖原文件')
  const bytes = await savePptx(s.opened)
  await authorize(s.root, s.file)
  if (await diskVersion(s) !== s.disk) fail('FILE_CONFLICT', '保存期间外部文件版本已变化')
  await writeAtomic(s.file, bytes)
  commitSaved(s.opened)
  s.disk = await diskVersion(s); s.savedRevision = s.revision; s.state = 'ready'; s.error = ''; changed(s)
  return summary(s)
}
function scheduleSave(s: Session) {
  clearTimeout(s.saveTimer)
  if (s.autoSave === false) return
  s.saveTimer = setTimeout(() => queue(s, async () => {
    try { await save(s) }
    catch (e: any) { s.state = e.code === 'FILE_CONFLICT' ? 'conflict' : 'save-error'; s.error = e.message; changed(s) }
  }), 200)
}
async function apply(s: Session, req: any, ops: Op[]) {
  const hit = replay(s, req); if (hit) return hit
  await refresh(s); ensureEdit(s, req)
  if (!Array.isArray(ops) || !ops.length || ops.length > 50) fail('INVALID_OPS', '每批需要 1–50 个操作')
  const plan = runTxn(s.opened!, { ops, dryRun: true })
  if (plan.failures?.length) fail('INVALID_OPS', plan.failures.map(f => `[${f.index}] ${f.error}`).join('\n'))
  if (req.dryRun) return { ...summary(s), plan: plan.plan, dryRun: true }
  const before = await savePptx(s.opened!)
  const result = runTxn(s.opened!, { ops, isolation: 'atomic' })
  if (!result.applied) fail('INVALID_OPS', result.failures?.map(f => f.error).join('\n') || '事务未执行')
  try {
    stableIds(s.opened!, result.records)
    syncTextAutofit(s.opened!, result.records ?? [], metrics)
    // Serialize/reopen materializes XML-only patches (tables/charts/themes) consistently.
    // The same bytes provide a crash-recovery checkpoint before acknowledging the edit.
    const bytes = await savePptx(s.opened!)
    const materialized = await openPptx(bytes)
    if (recoveryRoot) {
      const checkpoint = path.join(recoveryRoot, createHash('sha256').update(s.file).digest('hex') + '.pptx')
      await writeAtomic(checkpoint, bytes)
    }
    s.opened = materialized
    render(s)
  } catch (e) { s.opened = await openPptx(before); render(s); throw e }
  historyPush(s.undo, before); s.redo = []; s.revision++; s.state = 'ready'; s.error = ''
  s.changedSlides = [...new Set((result.records ?? []).map(r => {
    const index = s.opened!.deck.slides.findIndex(slide => slideDurableId(slide) === r.slideId)
    return index >= 0 ? index : typeof (r.after as any)?.index === 'number' ? (r.after as any).index : 0
  }))]
  changed(s); scheduleSave(s)
  const response = { ...summary(s), records: result.records?.map(r => ({ op: r.op.op, created: r.created, after: r.after })) }
  remember(s, req, response)
  return response
}
function deckRead(s: Session) {
  const info = (el: any): any => ({ id: elementDurableId(el) || el.id, nativeId: el.id, type: el.type,
    name: el.name, box: el.transform?.offset,
    text: el.text?.paragraphs?.map((p: any) => p.runs.map((r: any) => r.text).join('')).join('\n').slice(0, 1500),
    ...(el.children ? { coordinates: 'local-to-group', children: el.children.map(info) } : {}) })
  return { ...summary(s), emuPerPx: 9525, slideSize: s.opened?.deck.size,
    slides: s.opened?.deck.slides.map((slide, index) => ({ index, id: slideDurableId(slide), elements: slide.elements.map(info) })) ?? [] }
}
export async function dispatch(req: any): Promise<any> {
  if (req.action === 'hello') return { protocol: 1, upstream: '2a607bd49bc341f74a60a8fa852183dfb6ce3c1f', node: process.version }
  if (req.action === 'guide') {
    const guide = req.group ? opGuide(req.group) : opGuideCatalog()
    if (!guide) fail('INVALID_GROUP', '操作指南分组无效：' + opGuideCatalog())
    return guide
  }
  if (req.action === 'open') {
    const file = path.resolve(req.path), root = path.resolve(req.root)
    if (path.extname(file).toLowerCase() !== '.pptx') fail('UNSUPPORTED', '当前画布支持 PPTX')
    await authorize(root, file)
    const known = paths.get(keyOf(file))
    if (known) return queue(sessions.get(known)!, async () => {
      const s = sessions.get(known)!
      await refresh(s)
      if (req.create && !s.opened) { s.opened = await openPptx(await createBlankPptx()); render(s); s.state = 'ready'; s.error = ''; changed(s) }
      return summary(s)
    })
    const s: Session = { id: randomUUID(), file, root, revision: 0, savedRevision: 0, sequence: 0, disk: '', state: 'waiting', error: '', warnings: [], renders: [], undo: [], redo: [], requests: new Map(), changedSlides: [], chain: Promise.resolve() }
    sessions.set(s.id, s); paths.set(keyOf(file), s.id)
    try {
      return await queue(s, async () => {
        await refresh(s)
        if (req.create && !s.opened) { s.opened = await openPptx(await createBlankPptx()); s.state = 'ready'; render(s); changed(s) }
        return summary(s)
      })
    } catch (e) { sessions.delete(s.id); paths.delete(keyOf(file)); throw e }
  }
  const s = sessions.get(req.documentId)
  if (!s) fail('NOT_FOUND', '办公会话不存在，请重新打开文档')
  return queue(s, async () => {
    await authorize(s.root, s.file)
    switch (req.action) {
      case 'editor': {
        await refresh(s)
        if (!s.opened) fail('NOT_READY', '文档尚未写入，请等待 Agent 完成第一步')
        if (req.method === 'setAutoSavePref') {
          s.autoSave = req.args?.[0] === true
          clearTimeout(s.saveTimer)
          if (s.autoSave && s.revision !== s.savedRevision) scheduleSave(s)
          return { state: summary(s) }
        }
        const result = await editorCommand({
          get: () => ({ opened: s.opened!, renders: s.renders, file: s.file }),
          apply: ops => apply(s, req, ops),
          preview: async ops => {
            ensureEdit(s, req)
            const opened = await openPptx(await savePptx(s.opened!))
            const outcome = runTxn(opened, { ops })
            if (!outcome.applied) fail('INVALID_OP', outcome.failures?.map(f => f.error).join('; ') || '无效的预览操作')
            const scratch = { ...s, opened, renders: [] }
            render(scratch)
            return scratch.renders
          },
          save: () => save(s),
          history: async action => {
            ensureEdit(s, req)
            const from = action === 'undo' ? s.undo : s.redo, to = action === 'undo' ? s.redo : s.undo
            if (!from.length) return summary(s)
            const opened = await openPptx(from[from.length - 1])
            historyPush(to, await savePptx(s.opened!)); from.pop(); s.opened = opened; s.revision++
            s.changedSlides = []; render(s); changed(s); scheduleSave(s)
            return summary(s)
          },
        }, req.method, req.args ?? [])
        return { value: result, state: { ...summary(s), slides: s.renders } }
      }
      case 'read': await refresh(s); return deckRead(s)
      case 'state':
        await refresh(s)
        return { ...summary(s), ...(req.since !== s.sequence ? { slides: s.renders } : {}) }
      case 'apply': return apply(s, req, req.ops)
      case 'page': {
        const hit = replay(s, req); if (hit) return hit
        await refresh(s); ensureEdit(s, req)
        const parsed = parsePageSpec(JSON.stringify(req.page), undefined, undefined, { localImages: true })
        if (!parsed.ok) fail('INVALID_SPEC', parsed.error)
        const built = await buildPagePptx(parsed.spec, {
          fontMetrics: metrics,
          fetchImage: async (source: string) => {
            if (/^data:image\/(png|jpeg|webp|gif|bmp);base64,/.test(source)) {
              const [head, tail] = source.split(','); const bytes = Buffer.from(tail, 'base64')
              if (bytes.length > 10 * 1024 * 1024) fail('TOO_LARGE', '图片超过 10 MB')
              return { bytes, ext: head.includes('jpeg') ? 'jpg' : head.split('/')[1].split(';')[0] }
            }
            if (/^[a-z]+:\/\//i.test(source)) fail('PATH_DENIED', '请使用工作区内的图片，远端下载由 Agent 单独处理')
            const file = path.resolve(s.root, source)
            await authorize(s.root, file)
            const st = await fs.stat(file)
            if (st.size > 10 * 1024 * 1024) fail('TOO_LARGE', '图片超过 10 MB')
            return { bytes: await fs.readFile(file), ext: path.extname(file).slice(1) }
          },
          imageDims: bytes => imageSizeFromBytes(bytes),
        })
        const source = await extractMergeSlideSource(built.bytes)
        if (!source) fail('BUILD_FAILED', '单页生成失败')
        let at = req.index
        let replace = req.replace === true
        if (s.revision === 0 && s.opened!.deck.slides.length === 1 && s.opened!.deck.slides[0].elements.length === 0 && at == null) { at = 0; replace = true }
        const result = await apply(s, req, [{ op: 'insertSlidePptx', source, ...(at != null ? { at } : {}), replace }])
        s.warnings = [...new Set([...s.warnings, ...parsed.warnings, ...built.imageWarnings, ...built.imageFailures.map(v => '图片未载入：' + v)])]
        changed(s)
        const response = { ...result, ...summary(s) }; remember(s, req, response); return response
      }
      case 'save':
        if (req.expectedRevision !== undefined && req.expectedRevision !== s.revision) fail('REVISION_CONFLICT', '请重新读取版本后保存', summary(s))
        if (req.copyPath) {
          if (!s.opened) fail('NOT_READY', '没有可保存的文档')
          const copy = path.resolve(s.root, req.copyPath)
          if (path.extname(copy).toLowerCase() !== '.pptx') fail('UNSUPPORTED', '副本需要使用 .pptx 格式')
          await authorize(s.root, copy)
          try { await fs.stat(copy); fail('OUTPUT_EXISTS', '副本路径已存在，请使用另一个文件名') } catch(e: any) { if (e.code !== 'ENOENT') throw e }
          try { await writeAtomic(copy, await savePptx(s.opened), true) }
          catch (error: any) { if (error.code === 'EEXIST') fail('OUTPUT_EXISTS', '副本路径已存在，请使用另一个文件名'); throw error }
          return { ...summary(s), savedCopyPath: path.relative(s.root, copy), savedCopyRevision: s.revision }
        }
        return save(s)
      case 'audit':
        return { ...summary(s), findings: s.renders.map((slide, index) => ({ index, findings: auditSlideFindings(slide) })) }
      case 'undo':
      case 'redo': {
        const hit = replay(s, req); if (hit) return hit
        await refresh(s); ensureEdit(s, req)
        const from = req.action === 'undo' ? s.undo : s.redo, to = req.action === 'undo' ? s.redo : s.undo
        if (!from.length) return summary(s)
        const next = await openPptx(from[from.length - 1])
        historyPush(to, await savePptx(s.opened!)); from.pop(); s.opened = next; s.revision++
        s.changedSlides = []; render(s); changed(s); scheduleSave(s)
        const result = summary(s); remember(s, req, result); return result
      }
      default: fail('UNSUPPORTED', '未知办公操作')
    }
  })
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  const lines = createInterface({ input: process.stdin, crlfDelay: Infinity })
  lines.on('line', line => {
    if (line.length > 24 * 1024 * 1024) { process.stderr.write('Office request too large\n'); return }
    let message: any
    try { message = JSON.parse(line) } catch { return }
    dispatch(message.request).then(
      result => process.stdout.write(JSON.stringify({ id: message.id, result }) + '\n'),
      (e: any) => process.stdout.write(JSON.stringify({ id: message.id, error: { code: e.code || 'OFFICE_ERROR', message: e.message, detail: e.detail } }) + '\n'),
    )
  })
  lines.on('close', () => {
    Promise.allSettled([...sessions.values()].map(s => queue(s, async () => {
      clearTimeout(s.saveTimer)
      if (s.opened && s.revision !== s.savedRevision) await save(s)
    }))).finally(() => process.exit(0))
  })
}
