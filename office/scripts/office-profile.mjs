// Remove the upstream AI ribbon at bundle time. Vendor files stay unmodified on disk.
function once(contents, needle, label) {
  const at = contents.indexOf(needle)
  if (at < 0 || contents.indexOf(needle, at + needle.length) >= 0) throw new Error(label + ' marker changed')
  return at
}
function cutFrom(contents, start, end, label) {
  const i = once(contents, start, label)
  const j = contents.indexOf(end, i + start.length)
  if (j < 0) throw new Error(label + ' end changed')
  return contents.slice(0, i) + contents.slice(j)
}
function cutElement(contents, inner, label) {
  const at = once(contents, inner, label)
  const start = contents.lastIndexOf('<button', at)
  const end = contents.indexOf('</button>', at)
  if (start < 0 || end < 0) throw new Error(label + ' button changed')
  return contents.slice(0, start) + contents.slice(end + '</button>'.length)
}
function cutGroup(contents, inner, label) {
  const at = once(contents, inner, label)
  const start = contents.lastIndexOf('<div className="ribbon-group">', at)
  const sep = contents.indexOf('<div className="ribbon-sep" />', at)
  if (start < 0 || sep < 0) throw new Error(label + ' group changed')
  return contents.slice(0, start) + contents.slice(sep + '<div className="ribbon-sep" />'.length)
}
function cutThrough(contents, start, end, label) {
  const i = once(contents, start, label)
  const j = contents.indexOf(end, i)
  if (j < 0) throw new Error(label + ' end changed')
  return contents.slice(0, i) + contents.slice(j + end.length)
}
function cutAiCluster(contents, inner, end, label) {
  const at = once(contents, inner, label)
  const start = contents.lastIndexOf('<div className="ribbon-group">', at)
  const stop = contents.indexOf(end, at)
  if (start < 0 || stop < 0) throw new Error(label + ' cluster changed')
  return contents.slice(0, start) + contents.slice(stop)
}
function gone(text, needle, label) {
  if (text.includes(needle)) throw new Error(label + ' still contains ' + needle)
}

export function stripOfficeAi(filePath, contents) {
  const name = filePath.replaceAll('\\', '/')
  let text = contents.replaceAll('\r\n', '\n')
  if (name.endsWith('/apps/docs/src/renderer/components/Ribbon.tsx')) {
    text = cutFrom(text, '{/* ---- Genspark AI (first slot: entry + one-click AI actions) ---- */}', '{/* ---- Clipboard ---- */}', 'docs home AI')
  } else if (name.endsWith('/apps/docs/src/renderer/components/ribbon-tabs.tsx')) {
    text = cutElement(text, "t('ribbonEditorTip')", 'docs editor AI')
    text = cutGroup(text, "t('ribbonTranslateTip')", 'docs translate AI')
    text = cutElement(text, "t('ribbonAiCommentsTip'", 'docs comment AI')
    text = cutElement(text, "t('ribbonAiRevisionsTip'", 'docs revision AI')
  } else if (name.endsWith('/apps/docs/src/renderer/components/ContextMenu.tsx')) {
    text = cutFrom(text, '<div className="ctx-sep" />\n      {item(t(\'appSynonyms\'), {', '{/* Withholding a span.', 'docs context AI')
  } else if (name.endsWith('/apps/pdf/src/renderer/App.tsx')) {
    text = cutFrom(text, '{/* ---- Genspark AI (first slot: entry + one-click AI actions, docs parity) ---- */}', '{markupGroup}', 'pdf home AI')
    text = cutAiCluster(text, "data-tip={t('aiReviewSummaryBtn')}", '{markupGroup}', 'pdf review AI')
    text = cutElement(text, "data-tip={t('aiFillFormBtn')}", 'pdf form AI')
  } else if (name.endsWith('/apps/markdown/src/renderer/components/Ribbon.tsx')) {
    text = cutThrough(text, '  const aiPresets = [', '  ] as const\n', 'markdown presets')
    text = cutAiCluster(text, '<span>Genspark AI</span>', '{!sourceMode && (', 'markdown AI')
  } else if (name.endsWith('/apps/html/src/renderer/components/Ribbon.tsx')) {
    text = cutAiCluster(text, '<span>Genspark AI</span>', '<div className="ribbon-group">', 'html AI')
  } else if (name.endsWith('/apps/sheets/src/renderer/ExcelShell.tsx')) {
    text = cutFrom(text, "<RibbonGroup label={t('appGroupAiAssistant')}>", "<RibbonGroup label={t('appGroupClipboard')}>", 'sheets AI')
  } else return null
  gone(text, 'Genspark AI', name)
  gone(text, 'aiSummarizeBtn', name)
  gone(text, 'aiPolishBtn', name)
  gone(text, 'aiTidyBtn', name)
  gone(text, 'aiMindMapBtn', name)
  gone(text, 'ribbonEditorTip', name)
  gone(text, 'ribbonTranslateTip', name)
  gone(text, 'ribbonAiComments', name)
  gone(text, 'ribbonAiRevisions', name)
  gone(text, 'aiReviewSummaryBtn', name)
  gone(text, 'aiFillFormBtn', name)
  gone(text, 'appSynonyms', name)
  return text
}

export const officeAiProfile = {
  name: 'ffc-office-ai-profile',
  setup(build) {
    build.onLoad({ filter: /vendor[\\/]apps[\\/](docs|pdf|markdown|html|sheets)[\\/]src[\\/]renderer[\\/].*\.tsx$/ }, async args => {
      const { readFile } = await import('node:fs/promises')
      const original = await readFile(args.path, 'utf8')
      const contents = stripOfficeAi(args.path, original)
      if (contents == null) return null
      return { contents, loader: 'tsx' }
    })
  },
}
