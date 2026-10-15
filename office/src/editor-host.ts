/** GenOffice Slides preload intents translated to the canonical native ops.
 * Translation follows upstream apps/slides/src/main/slides-main.ts (Apache-2.0).
 * No Electron IPC, arbitrary filesystem access, or second AI runtime is exposed.
 */
import {
  getSlideNotes, getSlideComments, getSlideTransition, getSlideAnimations, elementSpid,
  getSections, getSlideLinks, getRunLinks, getElementLink, readHeaderFooter,
  listSlideLayouts, builtinLayoutInfos, ensureBuiltinLayout, BUILTIN_LAYOUT_PREFIX,
  findGroupChild, copyElementData, copySlide, getChartElementData, savePptx, slideDurableId,
  type OpenedPptx,
} from '@genoffice/pptx-engine'
import type { Op } from '@genoffice/pptx-ops'

type Host = {
  get(): { opened: OpenedPptx; renders: any[]; file: string }
  apply(ops: Op[]): Promise<any>
  preview(ops: Op[]): Promise<any[]>
  save(): Promise<any>
  history(action: 'undo' | 'redo'): Promise<any>
}
const elementClip = new Map<string, any[]>(), slideClip = new Map<string, any[]>()
function unsupported(method: string): never { throw new Error(`此桌面功能尚未接入 FFC：${method}。请在 Agent 中处理文档文件。`) }
const omit = (value: any, keys: string[]) => Object.fromEntries(Object.entries(value).filter(([key]) => !keys.includes(key)))
export async function editorCommand(host: Host, method: string, args: any[]) {
  const a = args[0] ?? {}, { opened, file } = host.get()
  const slideIndex = typeof a === 'number' ? a : a.slideIndex ?? 0
  const slide = opened.deck.slides[slideIndex]
  const target = (id = a.sourceId) => ({ slide: slideIndex, ...(id ? { el: id } : {}) })
  const group = a.groupId ? { group: a.groupId } : {}
  const toEmu = (px: number) => Math.round(px * opened.deck.size.cx / (a.fitWidthPx || 1280))
  const offset = (item = a) => ({ x: toEmu(item.xPx), y: toEmu(item.yPx), cx: toEmu(item.wPx), cy: toEmu(item.hPx) })
  const all = () => host.get().renders
  const render = () => all()[slideIndex]
  const apply = async (ops: any[], value: (result: any) => any = render) => {
    if (a.preview) {
      const renders = await host.preview(ops)
      return renders[slideIndex]
    }
    const result = await host.apply(ops); return value(result)
  }
  const element = (op: string, payload: any = {}) => apply([{ op, target: target(), ...group, ...payload }])
  const many = (op: string, payload: any = {}) => apply(a.sourceIds.map((id: string) => ({ op, target: target(id), ...group, ...payload })))
  const inserted = (result: any) => ({ slide: render(), sourceId: result.records?.[0]?.created?.[0] })
  const layoutPath = (name?: string) => name?.startsWith(BUILTIN_LAYOUT_PREFIX)
    ? ensureBuiltinLayout(opened.archive, opened.deck.size, name.slice(BUILTIN_LAYOUT_PREFIX.length)) ?? undefined : name
  switch (method) {
    case 'open': case 'openPptxPath':
      return { path: file, slides: all(), size: opened.deck.size, defaultFont: 'Microsoft YaHei' }
    case 'getRenderSlides': return all()
    case 'getSlideSize': return opened.deck.size
    case 'getNotes': return slide ? getSlideNotes(opened.archive, slide.path) : ''
    case 'getComments': return slide ? getSlideComments(opened.archive, slide.path) : []
    case 'getSections': return getSections(opened)
    case 'getTransition': return slide ? getSlideTransition(slide) : 'none'
    case 'getAnimations': return slide ? getSlideAnimations(slide).flatMap(animation => {
      const el = slide.elements.find(el => elementSpid(el) === animation.spid)
      return el ? [{ ...animation, sourceId: el.id, targetName: el.name || el.type }] : []
    }) : []
    case 'getShapeKeys': return slide?.elements.map(el => ({ sourceId: el.id, spid: elementSpid(el), name: el.name })) ?? []
    case 'getSlideLinks': return getSlideLinks(opened, slideIndex).map(({ elementId, target }) => ({ sourceId: elementId, target }))
    case 'getRunLinks': return getRunLinks(opened, slideIndex).map(({ elementId, ...rest }) => ({ sourceId: elementId, ...rest }))
    case 'getLink': return getElementLink(opened, slideIndex, args[1])
    case 'getHeaderFooter': return slide ? readHeaderFooter(slide) : { footer: null, slideNum: false, date: null }
    case 'getChartData': return slide ? getChartElementData(slide, args[1]) : null
    case 'getLayouts': {
      const layouts = listSlideLayouts(opened.archive)
      return { layouts: [...layouts, ...builtinLayoutInfos(opened.deck.size, new Set(layouts.map(l => l.name)))], size: opened.deck.size }
    }
    case 'hasSlideClipboard': return slideClip.has(file)
    case 'clipboardProbe': return elementClip.has(file) || slideClip.has(file)
    case 'copyElementsImage': return false // delayed PNG clipboard write is optional; native clipboard is retained
    case 'beginHistoryBatch': return false // each native transaction remains independently undoable
    case 'endHistoryBatch': return undefined
    case 'cancelTransformPreview': return render() // previews are isolated snapshots, never persisted
    case 'save': {
      try { await host.save(); return { ok: true, path: file, slides: all() } }
      catch (error: any) { return { ok: false, error: error.message } }
    }
    case 'download': return { base64: Buffer.from(await savePptx(opened)).toString('base64'), name: file.split(/[\\/]/).pop() }
    case 'undo': case 'redo': await host.history(method); return all()
    case 'editText': return element('setText', { paragraphs: a.paragraphs })
    case 'setElementFont': return many('setFont', { font: omit(a, ['slideIndex', 'sourceIds', 'groupId']) })
    case 'setElementParagraphFormat': return many('setParagraphFormat', { format: omit(a, ['slideIndex', 'sourceIds', 'groupId']) })
    case 'editTransform': case 'editTransformMulti': case 'batchEditTransform': {
      const items = method === 'editTransform' ? [a] : a.items
      return apply(items.map((item: any) => {
        let box = offset(item)
        if (item.groupId) {
          const child = findGroupChild(slide, item.groupId, item.sourceId)
          if (!child) throw new Error('组内元素已变化，请重新选择')
          const ch = child.grp.childOffset, ext = child.grp.transform.offset
          const sx = ch?.cx ? ext.cx / ch.cx : 1, sy = ch?.cy ? ext.cy / ch.cy : 1
          box = { x: toEmu(item.xPx / sx) + (ch?.x ?? ext.x), y: toEmu(item.yPx / sy) + (ch?.y ?? ext.y), cx: toEmu(item.wPx / sx), cy: toEmu(item.hPx / sy) }
        }
        return { op: 'setTransform', target: target(item.sourceId), box, rotDeg: item.rotationDeg,
          ...(item.groupId ? { group: item.groupId } : { resizeTableGrid: true }) }
      }))
    }
    case 'addElement': return apply([{ op: 'addElement', target: target(), kind: a.kind, offset: offset(),
      paragraphs: a.paragraphs || (a.text ? a.text.split('\n').map((text: string) => ({ runs: [{ text }] })) : undefined),
      fill: a.fillColor, stroke: a.stroke ? { color: a.stroke.color, widthEmu: Math.round(a.stroke.widthPt * 12700) } : undefined, bodyPr: a.bodyPr }], inserted)
    case 'deleteElement': return element('deleteElement')
    case 'deleteElements': return many('deleteElement')
    case 'editFill': {
      const g = a.fill?.gradient
      return element('setFill', { fill: g ? { stops: g.stops ?? [{ pos: 0, color: g.from }, { pos: 1, color: g.to }],
        ...(g.path || g.radial ? { path: g.path || 'circle', ...(g.center ? { fillTo: { l: g.center.x, t: g.center.y, r: 1 - g.center.x, b: 1 - g.center.y } } : {}) } : { angle: Math.round((g.angleDeg || 0) * 60000) }) } : a.fill })
    }
    case 'editStroke': return element('setStroke', { stroke: a.stroke ? { ...omit(a.stroke, ['widthPt', 'gradient']), widthEmu: Math.round(a.stroke.widthPt * 12700),
      ...(a.stroke.gradient ? { gradient: { stops: a.stroke.gradient.stops, angle: Math.round(a.stroke.gradient.angleDeg * 60000) } } : {}) } : null })
    case 'flipElements': return apply([{ op: 'flipElements', target: target(), ...group, els: a.sourceIds, axis: a.axis }])
    case 'reorderElement': return element('reorderElement', { dir: a.dir })
    case 'changeShape': return element('setShapeGeometry', { prst: a.prst })
    case 'setShapeAdjust': return element('setShapeAdjust', { adjust: a.adjust })
    case 'setTextAnchor': return element('setTextAnchor', { anchor: a.anchor })
    case 'setTextBodyProps': return element('setTextBodyProps', { props: a.props })
    case 'setEffects': return element('setEffects', { effects: a.effects })
    case 'editPictureOpacity': return element('setPictureOpacity', { opacity: a.opacity })
    case 'editPictureSrcRect': return element('setPictureSrcRect', { srcRect: a.srcRect })
    case 'editConnectorEndpoints': return element('setConnectorEndpoints', { p1: { x: toEmu(a.x1Px), y: toEmu(a.y1Px) }, p2: { x: toEmu(a.x2Px), y: toEmu(a.y2Px) }, start: a.start, end: a.end })
    case 'groupElements': return apply([{ op: 'groupElements', target: target(), els: a.sourceIds }], r => ({ slide: render(), groupId: r.records[0].created[0] }))
    case 'ungroupElement': return element('ungroupElement')
    case 'copyElements': {
      const items = a.sourceIds.map((id: string) => {
        const el = slide?.elements.find(el => el.id === id)
        return el ? copyElementData(opened, slide, el) : undefined
      })
      if (items.some((v: any) => !v)) throw new Error('选中元素已变化，请重新选择')
      elementClip.set(file, items); return true
    }
    case 'pasteElements': return apply([{ op: 'pasteElements', target: target(), items: elementClip.get(file) || [], dx: toEmu(a.dxPx ?? 16), dy: toEmu(a.dyPx ?? 16) }], r => ({ slide: render(), sourceIds: r.records.flatMap((record: any) => record.created || []) }))
    case 'duplicateElements': return apply([{ op: 'pasteElements', target: target(), items: a.sourceIds.map((id: string) => copyElementData(opened, slide, slide.elements.find(el => el.id === id)!)), dx: toEmu(a.dxPx ?? 16), dy: toEmu(a.dyPx ?? 16) }], r => ({ slide: render(), sourceIds: r.records.flatMap((record: any) => record.created || []) }))
    case 'addBlankSlide': return apply([{ op: 'addBlankSlide', target: { slide: a.sourceIndex } }, ...(a.before ? [{ op: 'moveSlide', target: { slide: a.sourceIndex }, to: a.sourceIndex + 1 }] : [])], () => ({ slides: all(), index: a.before ? a.sourceIndex : a.sourceIndex + 1 }))
    case 'addSlide': return apply([{ op: 'duplicateSlide', target: { slide: a.sourceIndex }, clearText: a.clearText }], () => ({ slides: all(), index: a.sourceIndex + 1 }))
    case 'addSlideWithLayout': return apply([{ op: 'addSlideWithLayout', target: { slide: a.sourceIndex }, layoutPath: layoutPath(a.layoutPath) }], () => ({ slides: all(), index: a.sourceIndex + 1 }))
    case 'setSlideLayout': return element('setSlideLayout', a.layoutPath ? { layoutPath: layoutPath(a.layoutPath) } : { reset: true })
    case 'deleteSlide': return apply([{ op: 'deleteSlide', target: { slide: a } }], all)
    case 'deleteSlides': return apply([...a.slideIndexes].sort((x: number, y: number) => y - x).map((index: number) => ({ op: 'deleteSlide', target: { slide: index } })), all)
    case 'duplicateSlides': {
      const selected = [...a.slideIndexes].sort((x: number, y: number) => x - y), end = selected.at(-1)
      return apply(selected.map((index: number, i: number) => ({ op: 'pasteSlide', afterIndex: end + i, bundle: copySlide(opened, index), mode: 'source' })), () => ({ slides: all(), index: end + 1 }))
    }
    case 'moveSlide': return apply([{ op: 'moveSlide', target: { slide: a.fromIndex }, to: a.toIndex }], all)
    case 'moveSlides': {
      const selected = [...a.slideIndexes].sort((x: number, y: number) => x - y)
      const ids = selected.map((index: number) => slideDurableId(opened.deck.slides[index]))
      const base = a.insertAt - selected.filter((index: number) => index < a.insertAt).length
      // Moving left is applied forward; moving right backward keeps the block order.
      const order = base > selected[0] ? ids.map((id, i) => ({ id, i })).reverse() : ids.map((id, i) => ({ id, i }))
      return apply(order.map(({ id, i }) => ({ op: 'moveSlide', target: { slide: id }, to: base + i })), all)
    }
    case 'copySlides': slideClip.set(file, a.slideIndexes.map((index: number) => copySlide(opened, index))); return true
    case 'pasteSlide': return apply((slideClip.get(file) || []).map((bundle, i) => ({ op: 'pasteSlide', afterIndex: a.afterIndex + i, mode: a.mode, bundle })), () => ({ slides: all(), index: a.afterIndex + 1, count: slideClip.get(file)?.length || 0 }))
    case 'editBackground': return apply((a.slideIndex < 0 ? opened.deck.slides.map((_, index) => index) : [slideIndex]).map(index => ({ op: 'setBackground', target: { slide: index }, ...omit(a, ['slideIndex', 'fitWidthPx']), kind: a.kind === 'hideGraphics' ? 'graphics' : a.kind })), all)
    case 'setSlideSize': return apply([{ op: 'setSlideSize', cx: a.cx, cy: a.cy }], all)
    case 'applyTheme': return apply([{ op: 'applyTheme', ...omit(a, ['fitWidthPx']) }], all)
    case 'findReplace': return apply([{ op: 'findReplace', ...a }], r => ({ slides: all(), count: r.records?.[0]?.after?.count ?? 0 }))
    case 'setNotes': return apply([{ op: 'setNotes', target: target(), text: a.text }], () => true)
    case 'addComment': await element('addComment', { text: a.text, author: 'FFC' }); return getSlideComments(host.get().opened.archive, host.get().opened.deck.slides[slideIndex].path)
    case 'deleteComment': await element('deleteComment', { authorId: a.authorId, idx: a.idx }); return getSlideComments(host.get().opened.archive, host.get().opened.deck.slides[slideIndex].path)
    case 'setSections': return apply([{ op: 'setSections', sections: a }], () => getSections(host.get().opened))
    case 'addSection': case 'renameSection': case 'removeSection': case 'moveSection': return apply([{ op: method, ...a }], () => getSections(host.get().opened))
    case 'setTransition': return apply((a.slideIndex === -1 ? opened.deck.slides.map((_, i) => i) : [slideIndex]).map(index => ({ op: 'setTransition', target: { slide: index }, kind: a.kind })), () => true)
    case 'setAnimations': return apply([{ op: 'setAnimations', target: target(), items: a.items }], () => true)
    case 'setAdvanceTimes': return apply(a.times.map((time: any) => ({ op: 'setAdvanceTime', target: { slide: time.slideIndex }, ms: time.ms })), () => true)
    case 'setSlideHidden': return element('setHidden', { hidden: a.hidden })
    case 'setSlidesHidden': return apply(a.slideIndexes.map((index: number) => ({ op: 'setHidden', target: { slide: index }, hidden: a.hidden })), all)
    case 'setLink': return element('setLink', { link: a.target })
    case 'applyHeaderFooter': return apply([{ op: 'applyHeaderFooter', settings: { footer: a.footer ?? null, slideNum: !!a.slideNum, date: a.date ?? null, dateAuto: a.dateAuto } }], all)
    case 'addTable': return apply([{ op: 'addTable', target: target(), offset: offset(), rows: a.rows, cols: a.cols,
      ...(a.rowHeightEmu ? { rowHeightsEmu: Array(a.rows).fill(a.rowHeightEmu) } : {}) }], inserted)
    case 'addChart': return apply([{ op: 'addChart', target: target(), offset: offset(), ...omit(a, ['slideIndex', 'fitWidthPx', 'xPx', 'yPx', 'wPx', 'hPx']), kind: a.kind === 'barH' ? 'bar' : a.kind, ...(a.kind === 'barH' ? { barDir: 'bar' } : {}) }], inserted)
    case 'addSmartArt': return apply([{ op: 'addSmartArt', target: target(), offset: offset(), layout: a.layout, items: a.items }], inserted)
    case 'addImageBytes': {
      let rect = offset()
      if (a.naturalPx) {
        const factor = Math.min(1, 1000 / a.naturalPx.width, 560 / a.naturalPx.height), w = a.naturalPx.width * factor, h = a.naturalPx.height * factor
        rect = { x: toEmu((a.centerPx?.x ?? 640) - w / 2), y: toEmu((a.centerPx?.y ?? 360) - h / 2), cx: toEmu(w), cy: toEmu(h) }
      }
      return apply([{ op: 'addPicture', target: target(), offset: rect, bytes: a.base64, ext: a.ext, name: a.name }], inserted)
    }
    case 'replacePictureBytes': return element('replacePicture', { bytes: a.base64, ext: a.ext, keepSrcRect: a.keepSrcRect })
    case 'editTableCell': return element('setTableCell', { row: a.row, col: a.col, paragraphs: a.paragraphs })
    case 'tableStructure': case 'tableMerge': return apply([{ op: method, target: target(), ...omit(a, ['slideIndex', 'sourceId']) }], r => ({ slide: render(), sourceId: r.records[0].after.elementId }))
    case 'setTableColWidth': return element('setTableColWidth', { col: a.col, wEmu: toEmu(a.wPx) })
    case 'setTableRowHeight': return element('setTableRowHeight', { row: a.row, hEmu: toEmu(a.hPx) })
    case 'setTableCellAnchor': return element('setTableCellAnchor', { row: a.row, col: a.col, anchor: a.anchor })
    case 'editTableStyle': return element('setTableStyle', omit(a, ['slideIndex', 'sourceId']))
    case 'editChart': return element('setChart', { patch: omit(a, ['slideIndex', 'sourceId']) })
    default: unsupported(method)
  }
}
