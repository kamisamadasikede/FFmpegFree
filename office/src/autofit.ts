// Adapted from GenOffice slides-main.ts at the pinned upstream commit.
// Copyright 2026 Mainfunc, Inc. Apache-2.0. Host-specific code lives outside vendor.
import { matchesElementRef, slideDurableId, patchBodyPrAutofit, type OpenedPptx } from '@genoffice/pptx-engine'
import { buildRenderSlide, layoutText, makeViewport, type FontMetricsProvider } from '@genoffice/pptx-render'

// runTxn edits the model; PowerPoint autofit write-back is a host responsibility.
export function syncTextAutofit(opened: OpenedPptx, records: any[], metrics: FontMetricsProvider) {
  const width = opened.deck.size.cx / 9525
  for (const record of records) {
    if (!['setText', 'setFont', 'setParagraphFormat'].includes(record.op.op) || record.op.group || record.after?.levelDirty) continue
    const index = opened.deck.slides.findIndex(s => slideDurableId(s) === record.slideId)
    const slide = opened.deck.slides[index]
    const el: any = slide?.elements.find(e => matchesElementRef(e, record.op.target?.el))
    if (!el?.text || !['resize', 'shrink'].includes(el.text.autofit)) continue
    const render = buildRenderSlide(slide, opened.deck.size, { fitWidthPx: width, metrics })
    const node: any = render.nodes.find(n => n.sourceId === el.id)
    if (!node?.text) continue
    const text = node.text
    if (el.text.autofit === 'resize') {
      const neededHeight = Math.max(1, text.contentHeight + text.insets.t + text.insets.b)
      const widest = Math.max(0, ...text.lines.flatMap((l: any) => l.runs.map((r: any) => r.x + r.widthPx)))
      const neededWidth = text.wrap ? node.box.w : Math.max(1, widest + text.insets.l + text.insets.r)
      if (Math.abs(neededHeight - node.box.h) > .25 || Math.abs(neededWidth - node.box.w) > .25) {
        const align = text.lines[0]?.align
        const shift = align === 'right' ? node.box.w - neededWidth : align === 'center' ? (node.box.w - neededWidth) / 2 : 0
        el.transform.offset = { ...el.transform.offset, x: el.transform.offset.x + Math.round(shift * 9525), cx: Math.round(neededWidth * 9525), cy: Math.round(neededHeight * 9525) }
        el.dirtyTransform = true
      }
    } else {
      const fitted = layoutText({ body: el.text, boxWidthPx: node.box.w, boxHeightPx: node.box.h,
        metrics, vp: makeViewport(opened.deck.size, width), refitAutofit: true })
      el.text.fontScale = fitted.fontScale
      el.text.lnSpcReduction = fitted.lnSpcReduction ?? 0
      el.anchor.originalXml = patchBodyPrAutofit(el.anchor.originalXml, fitted.fontScale, fitted.lnSpcReduction ?? 0)
      slide.structureDirty = true
    }
  }
}
