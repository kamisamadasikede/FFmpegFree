<template>
  <div ref="host" class="ed" />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { applyMonacoTheme, monaco, monoFontFamily } from './catMonaco'

const props = defineProps<{ text: string; language: string; readOnly?: boolean }>()
const emit = defineEmits<{ change: [value: string]; save: [] }>()

const host = ref<HTMLElement | null>(null)
let editor: monaco.editor.IStandaloneCodeEditor | null = null
let observer: MutationObserver | null = null

onMounted(() => {
  const el = host.value
  if (!el) return
  applyMonacoTheme()
  const dark = document.documentElement.classList.contains('dark')
  editor = monaco.editor.create(el, {
    value: props.text,
    language: props.language || 'plaintext',
    readOnly: !!props.readOnly,
    theme: dark ? 'ff-dark' : 'ff-light',
    minimap: { enabled: false },
    fontSize: 13,
    fontFamily: monoFontFamily(),
    wordWrap: 'on',
    scrollBeyondLastLine: false,
    automaticLayout: true,
    tabSize: 2,
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
  observer = new MutationObserver(() => applyMonacoTheme())
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

watch(
  () => props.language,
  (lang) => {
    const model = editor?.getModel()
    if (model) monaco.editor.setModelLanguage(model, lang || 'plaintext')
  },
)

watch(
  () => props.readOnly,
  (v) => editor?.updateOptions({ readOnly: !!v }),
)

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
