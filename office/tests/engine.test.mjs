import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { randomUUID } from 'node:crypto'
import { dispatch } from '../../internal/genoffice/runtime/engine.mjs'
import JSZip from 'jszip'

process.env.FFC_OFFICE_PAINT_MS = '0'

const page = title => ({ background: '#EFF5FF', elements: [
  { type: 'shape', shape: 'rect', x: 0, y: 0, w: 1280, h: 18, fill: '#2874D0' },
  { type: 'text', x: 72, y: 100, w: 1120, h: 120, paragraphs: [{ runs: [{ text: title, sizePt: 36, bold: true, color: '#203B60', font: 'Microsoft YaHei' }] }] },
  { type: 'text', x: 72, y: 280, w: 1040, h: 100, paragraphs: [{ runs: [{ text: '每完成一页，立即看到原生幻灯片。', sizePt: 22, font: 'Microsoft YaHei' }] }] },
] })
test('native incremental generation, targeted edits, idempotency, undo and round trip', async t => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'ffc-office-'))
  t.after(async () => { await new Promise(r => setTimeout(r, 400)); await fs.rm(root, { recursive: true, force: true }) })
  const file = path.join(root, 'live.pptx')
  let state = await dispatch({ action: 'open', root, path: file, create: true })
  const id = state.documentId
  assert.equal(state.slideCount, 1)
  const first = { action: 'page', documentId: id, expectedRevision: state.revision, requestId: randomUUID(), page: page('ffc · 第一页') }
  state = await dispatch(first)
  assert.equal(state.slideCount, 1, 'initial blank page must be replaced')
  let visible = await dispatch({ action: 'state', documentId: id, since: -1 })
  assert.equal(visible.slides.length, 1)
  assert.ok(visible.slides[0].nodes.some(n => n.text?.lines?.some(l => l.runs.map(r => r.text).join('').includes('第一页'))))
  assert.equal((await dispatch(first)).revision, state.revision, 'retries must not add a second page')
  for (const title of ['ffc · 第二页', 'ffc · 第三页']) state = await dispatch({ action: 'page', documentId: id, expectedRevision: state.revision, requestId: randomUUID(), page: page(title) })
  assert.equal(state.slideCount, 3)
  let deck = await dispatch({ action: 'read', documentId: id })
  const heading = deck.slides[2].elements.find(el => el.text?.includes('第三页'))
  const req = { action: 'apply', documentId: id, expectedRevision: deck.revision, requestId: randomUUID(), ops: [
    { op: 'setText', target: { slide: deck.slides[2].id, el: heading.id }, paragraphs: [{ srcPara: 0, runs: [{ text: '第三页 · 已实时修改', srcPara: 0, srcRun: 0 }] }] },
  ] }
  state = await dispatch(req)
  const fontReq = { action: 'apply', documentId: id, expectedRevision: state.revision, requestId: randomUUID(), ops: [{ op: 'setFont', target: { slide: deck.slides[2].id, el: heading.id }, font: { fontSizePt: 48 } }] }
  const enlarged = await dispatch(fontReq)
  const layout = await dispatch({ action: 'state', documentId: id, since: -1 })
  const titleNode = layout.slides[2].nodes.find(n => n.durableId === heading.id)
  assert.equal(titleNode.text.lines[0].runs[0].fontSizePx, 64)
  assert.ok(titleNode.box.h >= titleNode.text.contentHeight, 'resized font must write back the native text box height')
  state = await dispatch({ action: 'undo', documentId: id, expectedRevision: enlarged.revision, requestId: randomUUID() })
  await assert.rejects(dispatch({ ...req, requestId: randomUUID() }), e => e.code === 'REVISION_CONFLICT')
  deck = await dispatch({ action: 'read', documentId: id })
  assert.match(deck.slides[2].elements.find(el => el.id === heading.id).text, /已实时修改/)
  state = await dispatch({ action: 'undo', documentId: id, expectedRevision: state.revision, requestId: randomUUID() })
  assert.match((await dispatch({ action: 'read', documentId: id })).slides[2].elements.find(el => el.id === heading.id).text, /ffc · 第三页/)
  state = await dispatch({ action: 'redo', documentId: id, expectedRevision: state.revision, requestId: randomUUID() })
  const saved = await dispatch({ action: 'save', documentId: id, expectedRevision: state.revision })
  assert.equal(saved.savedRevision, saved.revision)
  const zip = await JSZip.loadAsync(await fs.readFile(file))
  const slideXML = await zip.file('ppt/slides/slide3.xml')?.async('string')
  const slideParts = Object.keys(zip.files).filter(n => /^ppt\/slides\/slide\d+\.xml$/.test(n))
  assert.equal(slideParts.length, 3)
  assert.ok((await Promise.all(slideParts.map(n => zip.file(n).async('string')))).some(xml => xml.includes('已实时修改') && xml.includes('<p:sp>')))
  // A second path forces a fresh parse rather than returning the existing live session.
  const copy = path.join(root, 'roundtrip.pptx'); await fs.copyFile(file, copy)
  const reopened = await dispatch({ action: 'open', root, path: copy })
  assert.equal(reopened.slideCount, 3)
  assert.match(JSON.stringify(await dispatch({ action: 'read', documentId: reopened.documentId })), /已实时修改/)
})
test('protects external modifications and supports independent document sessions', async t => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'ffc-conflict-'))
  t.after(async () => { await new Promise(r => setTimeout(r, 500)); await fs.rm(root, { recursive: true, force: true }) })
  const a = await dispatch({ action: 'open', root, path: path.join(root, 'a.pptx'), create: true })
  const b = await dispatch({ action: 'open', root, path: path.join(root, 'b.pptx'), create: true })
  assert.notEqual(a.documentId, b.documentId)
  const added = await dispatch({ action: 'page', documentId: a.documentId, expectedRevision: a.revision, requestId: randomUUID(), page: page('待保存修改') })
  await fs.writeFile(path.join(root, 'a.pptx'), 'external newer content')
  await assert.rejects(dispatch({ action: 'save', documentId: a.documentId }), e => e.code === 'FILE_CONFLICT')
  assert.equal(await fs.readFile(path.join(root, 'a.pptx'), 'utf8'), 'external newer content')
  assert.equal((await dispatch({ action: 'state', documentId: a.documentId })).state, 'conflict')
  const copy = await dispatch({ action: 'save', documentId: a.documentId, copyPath: 'recovered.pptx', expectedRevision: added.revision })
  assert.equal(copy.savedCopyRevision, added.revision)
  assert.ok((await fs.stat(path.join(root, 'recovered.pptx'))).size > 1000)
  await assert.rejects(dispatch({ action: 'save', documentId: a.documentId, copyPath: 'recovered.pptx' }), e => e.code === 'OUTPUT_EXISTS')
  const addedB = await dispatch({ action: 'page', documentId: b.documentId, expectedRevision: b.revision, requestId: randomUUID(), page: page('独立文档可继续') })
  await dispatch({ action: 'save', documentId: b.documentId })
  assert.equal(addedB.slideCount, 1)
})
test('one page draws element by element before the call returns', async t => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'ffc-draw-'))
  t.after(async () => { await new Promise(r => setTimeout(r, 200)); await fs.rm(root, { recursive: true, force: true }) })
  const file = path.join(root, 'draw.pptx')
  const state = await dispatch({ action: 'open', root, path: file, create: true })
  const spec = { elements: ['ONE', 'TWO', 'THREE'].map((text, index) => ({
    type: 'text', x: 72, y: 80 + index * 140, w: 1000, h: 100,
    paragraphs: [{ runs: [{ text, sizePt: 28, font: 'Microsoft YaHei' }] }],
  })) }
  const request = {
    action: 'page', documentId: state.documentId, expectedRevision: state.revision,
    requestId: randomUUID(), paintMs: 70, page: spec,
  }
  let partial = false
  const pending = dispatch(request)
  const deadline = Date.now() + 5000
  while (Date.now() < deadline && !partial) {
    const snap = await dispatch({ action: 'state', documentId: state.documentId, since: -1 })
    const blob = JSON.stringify(snap.slides ?? [])
    partial = blob.includes('ONE') && !blob.includes('THREE')
    if (!partial) await new Promise(r => setTimeout(r, 15))
  }
  const done = await pending
  assert.equal(partial, true)
  assert.equal(done.slideCount, 1)
  const final = JSON.stringify((await dispatch({ action: 'state', documentId: state.documentId, since: -1 })).slides ?? [])
  assert.ok(final.includes('ONE') && final.includes('TWO') && final.includes('THREE'))
  assert.equal((await dispatch(request)).revision, done.revision)
})
test('rejects workspace escapes and invalid operation batches without changing the deck', async t => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'ffc-path-'))
  t.after(() => fs.rm(root, { recursive: true, force: true }))
  await assert.rejects(dispatch({ action: 'open', root, path: path.join(root, '../outside.pptx'), create: true }), e => e.code === 'PATH_DENIED')
  const state = await dispatch({ action: 'open', root, path: path.join(root, 'deck.pptx'), create: true })
  await assert.rejects(dispatch({ action: 'apply', documentId: state.documentId, expectedRevision: state.revision, requestId: randomUUID(), ops: [{ op: 'deleteSlide', target: { slide: 100 } }] }), e => e.code === 'INVALID_OPS')
  const after = await dispatch({ action: 'read', documentId: state.documentId })
  assert.equal(after.revision, state.revision)
  assert.equal(after.slideCount, 1)
})
test('official editor intents produce editable native text, shapes, slides, tables and charts', async t => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'ffc-editor-'))
  t.after(async () => { await new Promise(r => setTimeout(r, 400)); await fs.rm(root, { recursive: true, force: true }) })
  const file = path.join(root, 'editor.pptx')
  let state = await dispatch({ action: 'open', root, path: file, create: true })
  const documentId = state.documentId
  const call = async (method, ...args) => {
    const response = await dispatch({ action: 'editor', documentId, method, args, expectedRevision: state.revision, requestId: randomUUID() })
    state = response.state; return response.value
  }
  const open = await call('open')
  assert.equal(open.slides.length, 1)
  assert.ok((await call('getLayouts')).layouts.length > 0)
  assert.equal(await call('getNotes', 0), '')
  assert.deepEqual(await call('getSections'), [])
  const box = { slideIndex: 0, fitWidthPx: 1280, xPx: 100, yPx: 100, wPx: 600, hPx: 150 }
  const heading = await call('addElement', { ...box, kind: 'textbox', text: '直接使用官方编辑器' })
  const sourceId = heading.sourceId
  assert.ok(sourceId)
  await call('setElementFont', { slideIndex: 0, sourceIds: [sourceId], fontSizePt: 40, bold: true })
  await call('setElementParagraphFormat', { slideIndex: 0, sourceIds: [sourceId], align: 'center' })
  await call('editText', { slideIndex: 0, sourceId, paragraphs: [{ srcPara: 0, runs: [{ text: '原生文字已编辑', srcRun: 0 }] }] })
  const beforePreview = state.revision
  const preview = await call('editTransform', { ...box, sourceId, xPx: 180, rotationDeg: 0, preview: true })
  assert.equal(state.revision, beforePreview, 'geometry preview must not dirty or save the shared deck')
  assert.equal(preview.nodes.find(n => n.sourceId === sourceId).box.x, 180)
  assert.equal((await call('getRenderSlides'))[0].nodes.find(n => n.sourceId === sourceId).box.x, 100)
  await call('editTransform', { ...box, sourceId, xPx: 180, rotationDeg: 0 })
  assert.equal(state.revision, beforePreview + 1)
  await call('undo')
  assert.equal((await call('getRenderSlides'))[0].nodes.find(n => n.sourceId === sourceId).box.x, 100)
  await call('redo')
  const shape = await call('addElement', { ...box, yPx: 300, wPx: 150, hPx: 120, kind: 'rect', fillColor: '#00AABB' })
  await call('editFill', { slideIndex: 0, sourceId: shape.sourceId, fill: '#FF8800' })
  await call('editStroke', { slideIndex: 0, sourceId: shape.sourceId, stroke: { color: '#223344', widthPt: 2 } })
  await call('duplicateElements', { ...box, sourceIds: [shape.sourceId], dxPx: 200, dyPx: 0 })
  await call('copyElements', { slideIndex: 0, sourceIds: [sourceId] })
  assert.equal(await call('clipboardProbe'), true)
  await call('pasteElements', { slideIndex: 0, fitWidthPx: 1280 })
  const table = await call('addTable', { ...box, yPx: 450, hPx: 180, rows: 2, cols: 3 })
  await call('editTableCell', { slideIndex: 0, sourceId: table.sourceId, row: 0, col: 0, paragraphs: [{ runs: [{ text: '可编辑表格' }] }] })
  await call('setTableColWidth', { ...box, sourceId: table.sourceId, col: 0, wPx: 220 })
  await call('editTableStyle', { slideIndex: 0, sourceId: table.sourceId, bandRow: true })
  const chart = await call('addChart', { ...box, xPx: 780, wPx: 360, kind: 'bar', categories: ['甲', '乙'], series: [{ name: '数据', values: [3, 7] }] })
  assert.equal((await call('getChartData', 0, chart.sourceId)).series[0].values[1], 7)
  await call('setNotes', { slideIndex: 0, text: '演讲备注保留在原生文件里' })
  assert.match(await call('getNotes', 0), /演讲备注/)
  await call('setTransition', { slideIndex: 0, kind: 'morph' })
  assert.equal(await call('getTransition', 0), 'morph')
  await call('addBlankSlide', { sourceIndex: 0, fitWidthPx: 1280 })
  assert.equal(state.slideCount, 2)
  await call('duplicateSlides', { slideIndexes: [0], fitWidthPx: 1280 })
  assert.equal(state.slideCount, 3)
  await call('moveSlides', { slideIndexes: [0, 1], insertAt: 3 })
  await call('deleteSlides', { slideIndexes: [0] })
  assert.equal(state.slideCount, 2)
  const saved = await call('save')
  assert.equal(saved.ok, true)
  const zip = await JSZip.loadAsync(await fs.readFile(file))
  const parts = await Promise.all(Object.keys(zip.files).filter(name => name.endsWith('.xml')).map(name => zip.file(name).async('string')))
  assert.ok(parts.some(xml => xml.includes('原生文字已编辑') && xml.includes('<p:sp>')))
  assert.ok(parts.some(xml => xml.includes('<a:tbl>')))
  assert.ok(Object.keys(zip.files).some(name => /^ppt\/charts\/chart\d+\.xml$/.test(name)))
  assert.ok(parts.some(xml => xml.includes('演讲备注')))
  await assert.rejects(dispatch({ action: 'editor', documentId, method: 'editText', args: [{ slideIndex: 0, sourceId, paragraphs: [] }], expectedRevision: state.revision - 1, requestId: randomUUID() }), e => e.code === 'REVISION_CONFLICT')
})
