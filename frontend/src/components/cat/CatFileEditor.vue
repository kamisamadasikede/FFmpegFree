<template>
  <div ref="host" class="ed" />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { applyMonacoTheme, monaco, monoFontFamily } from './catMonaco'
import type { EditorDiffMarks } from './catInlineDiff'

const props = defineProps<{ text: string; language: string; name?: string; readOnly?: boolean; marks?: EditorDiffMarks | null }>()
const emit = defineEmits<{ change: [value: string]; save: [] }>()

const host = ref<HTMLElement | null>(null)
let editor: monaco.editor.IStandaloneCodeEditor | null = null
let observer: MutationObserver | null = null
let decorationIds: string[] = []
let zoneIds: string[] = []
let marksKey = ''

function languageId(): string {
  if (props.language) return props.language
  const name = props.name || ''
  const dot = name.lastIndexOf('.')
  const ext = dot >= 0 ? name.slice(dot).toLowerCase() : ''
  if (ext) {
    const hit = monaco.languages.getLanguages().find((lang) => lang.extensions?.some((item) => item.toLowerCase() === ext))
    if (hit?.id) return hit.id
  }
  if (name.toLowerCase() === 'dockerfile') return 'dockerfile'
  return 'plaintext'
}

function paintMarks() {
  const ed = editor
  if (!ed) return
  const key = JSON.stringify(props.marks ?? null)
  if (key === marksKey) return
  marksKey = key
  const model = ed.getModel()
  const max = model?.getLineCount() ?? 0
  const marks = props.marks
  const decos: monaco.editor.IModelDeltaDecoration[] = []
  const paintLine = (line: number, className: string) => {
    if (line < 1 || line > max) return
    decos.push({
      range: new monaco.Range(line, 1, line, 1),
      options: { isWholeLine: true, className },
    })
  }
  for (const line of marks?.added ?? []) paintLine(line, 'cat-line-add')
  for (const line of marks?.removed ?? []) paintLine(line, 'cat-line-del')
  decorationIds = ed.deltaDecorations(decorationIds, decos)
  ed.changeViewZones((accessor) => {
    for (const id of zoneIds) accessor.removeZone(id)
    zoneIds = []
    const height = ed.getOption(monaco.editor.EditorOption.lineHeight)
    for (const group of marks?.deleted ?? []) {
      const after = Math.max(0, Math.min(group.after, max))
      const dom = document.createElement('div')
      for (const text of group.lines) {
        const row = document.createElement('div')
        row.className = 'cat-deleted-line'
        row.style.height = `${height}px`
        row.style.lineHeight = `${height}px`
        row.textContent = text.length ? text : ' '
        dom.appendChild(row)
      }
      zoneIds.push(accessor.addZone({
        afterLineNumber: after,
        heightInPx: group.lines.length * height,
        domNode: dom,
        suppressMouseDown: true,
      }))
    }
  })
}

onMounted(() => {
  const el = host.value
  if (!el) return
  applyMonacoTheme()
  const dark = document.documentElement.classList.contains('dark')
  editor = monaco.editor.create(el, {
    value: props.text,
    language: languageId(),
    'semanticHighlighting.enabled': true,
    readOnly: !!props.readOnly,
    theme: dark ? 'ff-dark' : 'ff-light',
    minimap: { enabled: false },
    fontSize: 13,
    fontFamily: monoFontFamily(),
    wordWrap: 'on',
    scrollBeyondLastLine: false,
    automaticLayout: true,
    tabSize: 2,
    lineNumbers: 'on',
    padding: { top: 8, bottom: 8 },
    overviewRulerLanes: 0,
    hideCursorInOverviewRuler: true,
    scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8 },
    unusualLineTerminators: 'off',
    unicodeHighlight: { ambiguousCharacters: false, invisibleCharacters: false },
    fixedOverflowWidgets: true,
  })
  editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => emit('save'))
  editor.onDidChangeModelContent(() => {
    if (editor) emit('change', editor.getValue())
  })
  paintMarks()
  observer = new MutationObserver(() => applyMonacoTheme())
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

watch(() => [props.language, props.name] as const, () => {
  const model = editor?.getModel()
  if (model) monaco.editor.setModelLanguage(model, languageId())
})

watch(
  () => props.readOnly,
  (v) => editor?.updateOptions({ readOnly: !!v }),
)

watch(() => props.marks, () => paintMarks())

onBeforeUnmount(() => {
  observer?.disconnect()
  editor?.dispose()
  editor = null
})
</script>

<style scoped>
.ed {
  flex: 1;
  min-height: 0;
  min-width: 0;
}
</style>

<style>
.cat-line-add {
  background: color-mix(in srgb, var(--ff-success) 18%, transparent);
}
.cat-line-del {
  background: color-mix(in srgb, var(--ff-danger) 16%, transparent);
}
.cat-deleted-line {
  box-sizing: border-box;
  overflow: hidden;
  white-space: pre;
  color: var(--ff-danger-text);
  background: color-mix(in srgb, var(--ff-danger) 16%, transparent);
  font-family: var(--ff-font-mono);
  font-size: 13px;
}
</style>
