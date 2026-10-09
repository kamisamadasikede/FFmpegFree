<template>
  <div class="jt">
    <!-- 工具栏：格式化 / 压缩 / 转义 / 去转义 / 复制，右侧自动格式化开关 -->
    <div class="tb" role="toolbar" aria-label="JSON 工具栏">
      <span class="grow" />
      <button class="btn pri" type="button" @click="run('format')">格式化</button>
      <button class="btn" type="button" @click="run('compact')">压缩</button>
      <button class="btn" type="button" @click="run('escape')">转义</button>
      <button class="btn" type="button" @click="run('unescape')">去转义</button>
      <button class="btn" type="button" @click="copyResult"><FIcon name="doc" :size="15" />复制</button>
      <span class="vs" />
      <button class="af" type="button" role="switch" :aria-checked="autoFormat" @click="autoFormat = !autoFormat">
        <span class="switch" :class="{ on: autoFormat }" />自动格式化
      </button>
    </div>

    <div class="hrow">
      <!-- 输入 -->
      <section
        class="panel"
        aria-label="输入"
        @dragenter.capture="onDragEnter"
        @dragover.capture="onDragOver"
        @dragleave.capture="onDragLeave"
        @drop.capture="onDrop"
      >
        <header class="phead"><h2>输入</h2><span class="sp" /><span class="sub">粘贴或拖入 .json 文件</span></header>
        <div class="edwrap">
          <div ref="inputEl" class="ed" />
          <div v-if="dragging" class="dropov" aria-hidden="true">
            <div class="in">
              <FIcon name="upload" :size="32" :stroke="1.6" />
              <b>松开以载入 JSON 文件</b>
              <small>仅支持 .json，最大 10 MB</small>
            </div>
          </div>
        </div>
        <footer class="sbar" aria-live="polite">
          <template v-if="dropMsg">
            <span class="bad msg"><FIcon name="warn" :size="13" />{{ dropMsg }}</span>
          </template>
          <template v-else-if="inputErr">
            <button class="bad" type="button" title="跳转到出错位置" @click="gotoError">
              <FIcon name="warn" :size="13" />第 {{ inputErr.line }} 行，第 {{ inputErr.column }} 列：{{ inputErr.message }}
            </button>
          </template>
          <span v-else-if="inputText.trim()" class="ok"><FIcon name="check" :size="13" />有效 JSON</span>
          <span v-else>未输入</span>
          <span class="sp" />
          <span>{{ inputLines }} 行 · {{ inputIndent }}</span>
        </footer>
      </section>

      <!-- 结果 -->
      <section class="panel" aria-label="结果">
        <header class="phead">
          <h2>结果</h2>
          <span class="sp" />
          <button class="zoom" type="button" title="放大查看" aria-label="放大查看结果" :disabled="showEmpty" @click="openZoom">
            <FIcon name="zin" :size="15" />
          </button>
          <div class="seg" role="tablist" aria-label="结果视图">
            <button
              v-for="v in VIEWS"
              :key="v.key"
              type="button"
              role="tab"
              :aria-selected="view === v.key"
              :class="{ on: view === v.key }"
              @click="view = v.key"
            >
              {{ v.label }}
            </button>
          </div>
        </header>
        <div class="edwrap">
          <div v-show="showCode" ref="resultEl" class="ed" />
          <JsonTree v-if="showTree" :text="result.text" />
          <div v-if="treeTooDeep" class="rempty">
            <div class="ic"><FIcon name="doc" :size="20" /></div>
            嵌套层级过深，请切换到代码视图
          </div>
          <div v-if="showEmpty" class="rempty">
            <template v-if="svcError">
              <div class="ic err"><FIcon name="warn" :size="20" /></div>
              {{ svcError }}
            </template>
            <template v-else>
              <div class="ic"><FIcon name="doc" :size="20" /></div>
              {{ emptyText }}
            </template>
          </div>
        </div>
        <footer class="sbar">
          <template v-if="showEmpty || !result.text">
            <span class="sp" /><span>UTF-8</span>
          </template>
          <template v-else>
            <span v-if="result.kind === 'json'" class="ok"><FIcon name="check" :size="13" />有效 JSON</span>
            <span v-else>{{ result.mode === 'escape' ? '已转义' : '已去转义' }}</span>
            <span>{{ resultLines }} 行<template v-if="result.kind === 'json'"> · {{ resultIndent }}</template></span>
            <span class="sp" /><span>UTF-8</span>
          </template>
        </footer>
      </section>
    </div>

    <Teleport to="body">
      <div v-if="zoomOpen" class="zmask" @mousedown.self="closeZoom">
        <div class="zdlg" role="dialog" aria-modal="true" aria-labelledby="json-zoom-title">
          <header class="phead">
            <h2 id="json-zoom-title">结果</h2>
            <span class="sp" />
            <div class="seg" role="tablist" aria-label="结果视图">
              <button
                v-for="v in VIEWS"
                :key="v.key"
                type="button"
                role="tab"
                :aria-selected="view === v.key"
                :class="{ on: view === v.key }"
                @click="view = v.key"
              >
                {{ v.label }}
              </button>
            </div>
            <button ref="zoomCloseEl" class="zoom" type="button" title="关闭" aria-label="关闭" @click="closeZoom">
              <FIcon name="x" :size="16" />
            </button>
          </header>
          <div class="edwrap">
            <div v-show="showCode" ref="zoomEl" class="ed" />
            <JsonTree v-if="showTree" :text="result.text" />
            <div v-if="treeTooDeep" class="rempty">
              <div class="ic"><FIcon name="doc" :size="20" /></div>
              嵌套层级过深，请切换到代码视图
            </div>
            <div v-if="showEmpty" class="rempty">
              <template v-if="svcError">
                <div class="ic err"><FIcon name="warn" :size="20" /></div>
                {{ svcError }}
              </template>
              <template v-else>
                <div class="ic"><FIcon name="doc" :size="20" /></div>
                {{ emptyText }}
              </template>
            </div>
          </div>
          <footer class="sbar">
            <template v-if="showEmpty || !result.text">
              <span class="sp" /><span>UTF-8</span>
            </template>
            <template v-else>
              <span v-if="result.kind === 'json'" class="ok"><FIcon name="check" :size="13" />有效 JSON</span>
              <span v-else>{{ result.mode === 'escape' ? '已转义' : '已去转义' }}</span>
              <span>{{ resultLines }} 行<template v-if="result.kind === 'json'"> · {{ resultIndent }}</template></span>
              <span class="sp" /><span>UTF-8</span>
            </template>
          </footer>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import JsonTree from '@/components/json/JsonTree.vue'
import { formatJson } from '@/api/json/json'
import { toAppError } from '@/api/call'
import { applyMonacoTheme, monaco, monoFontFamily } from '@/utils/monacoJson'
import {
  countLines,
  describeIndent,
  describeServiceError,
  describeSyntax,
  errorTokenSpan,
  escapeText,
  localFormat,
  localValidate,
  unescapeText,
  type SyntaxErr,
} from '@/utils/jsonText'

type Mode = 'format' | 'compact' | 'escape' | 'unescape'
const VIEWS = [
  { key: 'code', label: '代码' },
  { key: 'tree', label: '树形' },
] as const

const INDENT = 2
const MAX_FILE = 10 * 1024 * 1024
const DEBOUNCE_MS = 300
const MSG_MS = 4000

const autoFormat = ref(true) // 默认开启
const view = ref<'code' | 'tree'>('code')
const zoomOpen = ref(false)
const inputEl = ref<HTMLElement | null>(null)
const resultEl = ref<HTMLElement | null>(null)
const zoomEl = ref<HTMLElement | null>(null)
const zoomCloseEl = ref<HTMLButtonElement | null>(null)

const inputText = ref('')
const inputErr = ref<SyntaxErr | null>(null)
/** 结果区的一行中文错误（转义失败、后端非语法错误）；永远不显示标题「出错了」和错误码 */
const svcError = ref('')
const result = ref<{ mode: Mode; kind: 'json' | 'text'; text: string }>({ mode: 'format', kind: 'json', text: '' })
const dragging = ref(false)
const dropMsg = ref('')
const treeTooDeep = ref(false)

const inputLines = computed(() => countLines(inputText.value))
const inputIndent = computed(() => describeIndent(inputText.value))
const resultLines = computed(() => countLines(result.value.text))
const resultIndent = computed(() => describeIndent(result.value.text))

// 有语法错误时，格式化 / 压缩的结果作废；转义 / 去转义不要求输入是合法 JSON
const errorBlocksResult = computed(() => !!inputErr.value && (result.value.mode === 'format' || result.value.mode === 'compact'))
const showEmpty = computed(() => !!svcError.value || errorBlocksResult.value || !result.value.text)
const showCode = computed(() => !showEmpty.value && (view.value === 'code' || result.value.kind !== 'json'))
const showTree = computed(() => !showEmpty.value && view.value === 'tree' && result.value.kind === 'json' && !treeTooDeep.value)
const emptyText = computed(() =>
  inputErr.value ? '修正错误后自动显示结果' : autoFormat.value ? '在左侧输入或拖入 JSON，结果会自动显示' : '点击「格式化」查看结果',
)

// ---------- Monaco ----------
let inputEditor: monaco.editor.IStandaloneCodeEditor | null = null
let resultEditor: monaco.editor.IStandaloneCodeEditor | null = null
let zoomEditor: monaco.editor.IStandaloneCodeEditor | null = null
let errDecos: monaco.editor.IEditorDecorationsCollection | null = null
let themeObserver: MutationObserver | null = null

const baseOptions = (): monaco.editor.IStandaloneEditorConstructionOptions => ({
  language: 'json',
  automaticLayout: true,
  fontSize: 13,
  lineHeight: 24,
  fontFamily: monoFontFamily(),
  minimap: { enabled: false },
  scrollBeyondLastLine: false,
  renderLineHighlight: 'none',
  overviewRulerLanes: 0,
  overviewRulerBorder: false,
  hideCursorInOverviewRuler: true,
  lineNumbersMinChars: 3,
  lineDecorationsWidth: 8,
  padding: { top: 8, bottom: 8 },
  tabSize: INDENT,
  insertSpaces: true,
  detectIndentation: false,
  quickSuggestions: false,
  suggestOnTriggerCharacters: false,
  contextmenu: true,
  scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8 },
  folding: false,
  guides: { indentation: true, bracketPairs: false },
})

let timer: ReturnType<typeof setTimeout> | null = null
let seq = 0

function scheduleCheck(immediate = false) {
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => void check(), immediate ? 0 : DEBOUNCE_MS)
}

const hasBinding = () => !!(window as unknown as { go?: { app?: { JsonService?: unknown } } }).go?.app?.JsonService

interface FormatOut {
  formatted: string
  error: string
  errorPos: { line: number; column: number }
}
/** 优先走 Wails 的 JsonService；浏览器里没有绑定时退回本地实现（仅开发预览用）。 */
async function svcFormat(json: string, compact: boolean): Promise<FormatOut> {
  if (!hasBinding()) return localFormat(json, compact, INDENT)
  return formatJson({ json, indent: INDENT, compact })
}

/** 输入变化后：校验语法，自动格式化开启时同时更新结果。 */
async function check() {
  const my = ++seq
  const text = inputEditor?.getValue() ?? ''
  inputText.value = text
  svcError.value = ''
  if (!text.trim()) {
    inputErr.value = null
    result.value = { mode: 'format', kind: 'json', text: '' }
    return
  }
  try {
    // 自动格式化关闭时也要校验，所以总是调用 Format；只是关闭时不采用它的格式化结果
    const r = await svcFormat(text, false)
    if (my !== seq) return
    if (r.error) {
      inputErr.value = { line: r.errorPos.line, column: r.errorPos.column, message: describeSyntax(r.error) }
      return
    }
    inputErr.value = null
    if (autoFormat.value) setResult('format', r.formatted)
  } catch (e) {
    if (my !== seq) return
    inputErr.value = null
    const err = toAppError(e)
    svcError.value = describeServiceError(err.code, err.message, 'format')
  }
}

function setResult(mode: Mode, text: string) {
  const kind = mode === 'escape' ? 'text' : mode === 'unescape' ? (localValidate(text) ? 'text' : 'json') : 'json'
  result.value = { mode, kind, text }
}

async function run(mode: Mode) {
  const text = inputEditor?.getValue() ?? ''
  svcError.value = ''
  if (!text.trim()) {
    inputText.value = text
    return
  }
  treeTooDeep.value = false
  if (mode === 'escape') return setResult(mode, escapeText(text))
  if (mode === 'unescape') {
    try {
      return setResult(mode, unescapeText(text))
    } catch (e) {
      svcError.value = describeServiceError('INVALID_ARGUMENT', (e as Error).message, 'unescape')
      return
    }
  }
  const my = ++seq
  try {
    const r = await svcFormat(text, mode === 'compact')
    if (my !== seq) return
    inputText.value = text
    if (r.error) {
      inputErr.value = { line: r.errorPos.line, column: r.errorPos.column, message: describeSyntax(r.error) }
      result.value = { mode, kind: 'json', text: '' } // 结果区显示「修正错误后自动显示结果」
      return
    }
    inputErr.value = null
    setResult(mode, r.formatted)
  } catch (e) {
    if (my !== seq) return
    const err = toAppError(e)
    svcError.value = describeServiceError(err.code, err.message, mode)
  }
}

// 错误标记：红色波浪线 + 槽位红点 + 整行淡红底
function paintError(err: SyntaxErr | null) {
  if (!inputEditor || !errDecos) return
  const model = inputEditor.getModel()
  if (!err || !model) return errDecos.clear()
  const line = Math.min(Math.max(1, err.line), model.getLineCount())
  // 波浪线只标出错的那个 token，不拖到行尾（err.column 是 1 基）
  const span = errorTokenSpan(model.getLineContent(line), err.column - 1)
  const start = span.start + 1
  const end = start + span.length
  errDecos.set([
    { range: new monaco.Range(line, 1, line, 1), options: { isWholeLine: true, className: 'ff-json-errline', glyphMarginClassName: 'ff-json-dot', marginClassName: 'ff-json-margin' } },
    { range: new monaco.Range(line, start, line, end), options: { inlineClassName: 'ff-json-squiggle' } },
  ])
}
watch(inputErr, paintError)

function gotoError() {
  const e = inputErr.value
  if (!e || !inputEditor) return
  const model = inputEditor.getModel()!
  const line = Math.min(Math.max(1, e.line), model.getLineCount())
  const col = Math.min(Math.max(1, e.column), model.getLineMaxColumn(line))
  inputEditor.setPosition({ lineNumber: line, column: col })
  inputEditor.revealPositionInCenter({ lineNumber: line, column: col })
  inputEditor.focus()
}

// 结果编辑器随结果文本 / 类型更新
watch(
  () => [result.value.text, result.value.kind] as const,
  ([text, kind]) => {
    const model = resultEditor?.getModel()
    if (!resultEditor || !model) return
    monaco.editor.setModelLanguage(model, kind === 'json' ? 'json' : 'plaintext')
    if (resultEditor.getValue() !== text) resultEditor.setValue(text)
  },
)
watch(showCode, (v) => {
  if (v) nextTick(() => resultEditor?.layout())
  if (zoomOpen.value) void mountZoomEditor()
})

function openZoom() {
  if (showEmpty.value) return
  zoomOpen.value = true
}
function closeZoom() {
  zoomOpen.value = false
}
function onZoomKey(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  e.preventDefault()
  e.stopPropagation()
  closeZoom()
}
function syncZoomEditor() {
  if (!zoomEditor) return
  const model = zoomEditor.getModel()
  if (!model) return
  monaco.editor.setModelLanguage(model, result.value.kind === 'json' ? 'json' : 'plaintext')
  if (zoomEditor.getValue() !== result.value.text) zoomEditor.setValue(result.value.text)
}
async function mountZoomEditor() {
  await nextTick()
  if (!zoomOpen.value || !showCode.value || !zoomEl.value) {
    zoomEditor?.dispose()
    zoomEditor = null
    return
  }
  if (!zoomEditor) {
    zoomEditor = monaco.editor.create(zoomEl.value, {
      ...baseOptions(),
      value: result.value.text,
      readOnly: true,
      domReadOnly: true,
      glyphMargin: false,
      wordWrap: 'on',
      fontSize: 14,
    })
    const model = zoomEditor.getModel()
    if (model) monaco.editor.setModelLanguage(model, result.value.kind === 'json' ? 'json' : 'plaintext')
  } else {
    syncZoomEditor()
    zoomEditor.layout()
  }
}
watch(zoomOpen, (open) => {
  if (open) {
    document.addEventListener('keydown', onZoomKey, true)
    void mountZoomEditor().then(() => zoomCloseEl.value?.focus())
  } else {
    document.removeEventListener('keydown', onZoomKey, true)
    zoomEditor?.dispose()
    zoomEditor = null
  }
})
watch(
  () => [result.value.text, result.value.kind] as const,
  () => {
    if (zoomOpen.value) syncZoomEditor()
  },
)

// 自动格式化：打开时立刻按当前输入格式化一次
watch(autoFormat, (on) => on && scheduleCheck(true))

// ---------- 复制 ----------
async function copyResult() {
  const text = result.value.text
  if (!text || showEmpty.value) return void ElMessage.warning('暂无可复制内容')
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    const ta = document.createElement('textarea')
    ta.value = text
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    ta.remove()
  }
  ElMessage.success('已复制')
}

// ---------- 拖入 .json ----------
let dragDepth = 0
const hasFiles = (e: DragEvent) => !!e.dataTransfer && Array.from(e.dataTransfer.types).includes('Files')
function onDragEnter(e: DragEvent) {
  if (!hasFiles(e)) return
  e.preventDefault()
  dragDepth++
  dragging.value = true
}
function onDragOver(e: DragEvent) {
  if (!hasFiles(e)) return
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
}
function onDragLeave(e: DragEvent) {
  if (!hasFiles(e)) return
  dragDepth = Math.max(0, dragDepth - 1)
  if (dragDepth === 0) dragging.value = false
}

let msgTimer: ReturnType<typeof setTimeout> | null = null
/** 状态栏左侧的红字提示，4 秒后自动消失，新提示会替换旧的并重置计时。 */
function flashMsg(text: string) {
  if (msgTimer) clearTimeout(msgTimer)
  dropMsg.value = text
  msgTimer = setTimeout(() => {
    dropMsg.value = ''
    msgTimer = null
  }, MSG_MS)
}

async function onDrop(e: DragEvent) {
  if (!hasFiles(e)) return
  e.preventDefault()
  e.stopPropagation()
  dragDepth = 0
  dragging.value = false
  const file = e.dataTransfer?.files?.[0]
  if (!file) return
  if (!/\.json$/i.test(file.name)) return flashMsg('只能载入 .json 文件')
  if (file.size > MAX_FILE) return flashMsg('文件超过 10 MB')
  try {
    const text = (await file.text()).replace(/^\uFEFF/, '')
    inputEditor?.setValue(text)
    inputEditor?.setPosition({ lineNumber: 1, column: 1 })
    scheduleCheck(true)
  } catch {
    flashMsg('读取文件失败')
  }
}

// ---------- 生命周期 ----------
onMounted(() => {
  applyMonacoTheme()
  inputEditor = monaco.editor.create(inputEl.value!, { ...baseOptions(), value: '', glyphMargin: true, wordWrap: 'on' })
  resultEditor = monaco.editor.create(resultEl.value!, { ...baseOptions(), value: '', readOnly: true, domReadOnly: true, glyphMargin: false })
  errDecos = inputEditor.createDecorationsCollection()
  inputEditor.onDidChangeModelContent(() => scheduleCheck())
  // 主题跟随 html.dark 实时切换
  themeObserver = new MutationObserver(() => applyMonacoTheme())
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  // 深度嵌套等极端情况树解析失败时给出提示
  watch([showTree, () => result.value.text], async () => {
    if (view.value !== 'tree' || result.value.kind !== 'json' || !result.value.text) return void (treeTooDeep.value = false)
    const { parseTree } = await import('@/utils/jsonTree')
    treeTooDeep.value = parseTree(result.value.text) === null
  })
})

onBeforeUnmount(() => {
  if (timer) clearTimeout(timer)
  if (msgTimer) clearTimeout(msgTimer)
  themeObserver?.disconnect()
  document.removeEventListener('keydown', onZoomKey, true)
  inputEditor?.dispose()
  resultEditor?.dispose()
  zoomEditor?.dispose()
})
</script>

<style scoped>
.jt {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-4);
}
.tb {
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  flex: none;
}
.grow {
  flex: 1;
}
.vs {
  width: 1px;
  height: 16px;
  background: var(--ff-border);
  margin: 0 var(--ff-space-1);
}
.btn {
  height: 28px;
  padding: 0 var(--ff-space-3);
  border-radius: var(--ff-radius-md);
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font: inherit;
  font-size: var(--ff-fs-sm);
  white-space: nowrap;
  cursor: pointer;
  transition: background var(--ff-dur-fast) var(--ff-ease);
}
.btn:hover {
  background: var(--ff-bg-hover);
}
.btn.pri {
  /* 与 element-override.css 的共享主按钮一致：底色 --ff-badge-bg（浅色 #2f5cd9 配白字 5.75:1，暗色 #5b8cff 配近黑字 6.19:1） */
  background: var(--ff-badge-bg);
  border-color: var(--ff-badge-bg);
  color: var(--ff-on-primary);
}
.btn.pri:hover {
  background: var(--ff-primary-hover);
  border-color: var(--ff-primary-hover);
}
:root:not(.dark) .btn.pri:hover {
  /* 浅色下 primary-hover 与 badge-bg 同色，加深一点保留悬停反馈（同 element-override.css） */
  background: color-mix(in srgb, var(--ff-primary-hover) 88%, #000);
  border-color: color-mix(in srgb, var(--ff-primary-hover) 88%, #000);
}
.btn:focus-visible,
.af:focus-visible,
.seg button:focus-visible,
.zoom:focus-visible,
.bad:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 1px;
}
.af {
  display: inline-flex;
  align-items: center;
  gap: var(--ff-space-2);
  height: 28px;
  padding: 0;
  border: 0;
  background: none;
  font: inherit;
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-2);
  white-space: nowrap;
  cursor: pointer;
}
.switch {
  width: 28px;
  height: 16px;
  border-radius: 8px;
  background: var(--ff-text-3); /* 关闭态：白滑块在 --ff-border 上只有 1.25:1 */
  position: relative;
  flex: none;
  transition: background var(--ff-dur-fast) var(--ff-ease);
}
.switch::after {
  content: '';
  position: absolute;
  left: 2px;
  top: 2px;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--ff-switch-knob);
  transition: transform var(--ff-dur-fast) var(--ff-ease);
}
.switch.on {
  background: var(--ff-primary);
}
.switch.on::after {
  transform: translateX(12px);
}

/* 两个面板等宽 */
.hrow {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: var(--ff-space-4);
}
.panel {
  flex: 1 1 0;
  width: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 0; /* 覆盖 base.css 全局 .panel 的 padding，让表头分隔线 / 状态栏上边线 / 拖入虚线框贴满面板 */
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-lg);
}
.phead {
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  height: 48px;
  flex: none;
  padding: 0 var(--ff-space-4);
  border-bottom: 1px solid var(--ff-border);
}
.phead h2 {
  margin: 0;
  font-size: var(--ff-fs-md);
  font-weight: 600;
}
.sp {
  flex: 1;
}
.sub {
  color: var(--ff-text-2);
  font-size: var(--ff-fs-xs);
}
.seg {
  display: flex;
  width: 120px;
  background: var(--ff-bg-hover);
  border-radius: var(--ff-radius-md);
  padding: 2px;
}
.seg button {
  flex: 1;
  height: 24px;
  border: 0;
  border-radius: var(--ff-radius-sm);
  background: none;
  font: inherit;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  cursor: pointer;
}
.seg button.on {
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  font-weight: 500;
  box-shadow: var(--ff-shadow-1);
}
.zoom {
  width: 28px;
  height: 28px;
  flex: none;
  display: grid;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: var(--ff-radius-md);
  background: transparent;
  color: var(--ff-text-2);
  cursor: pointer;
}
.zoom:hover:not(:disabled) {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.zoom:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
.zmask {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background: rgba(0, 0, 0, 0.45);
  display: grid;
  place-items: center;
}
.zdlg {
  width: calc(100vw - 48px);
  height: calc(100vh - 48px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: 12px;
  box-shadow: var(--ff-shadow-dialog);
}

.edwrap {
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--ff-bg-surface);
}
.ed {
  flex: 1;
  min-height: 0;
  text-align: left;
}

.sbar {
  height: 28px;
  flex: none;
  border-top: 1px solid var(--ff-border);
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 var(--ff-space-3);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  white-space: nowrap;
  overflow: hidden;
}
.ok {
  color: var(--ff-success-text);
  display: flex;
  align-items: center;
  gap: var(--ff-space-1);
}
.bad {
  color: var(--ff-danger-text);
  display: flex;
  align-items: center;
  gap: var(--ff-space-1);
  padding: 0;
  border: 0;
  background: none;
  font: inherit;
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bad.msg {
  text-decoration: none;
  cursor: default;
}

.rempty {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--ff-space-3);
  padding: 0 var(--ff-space-6);
  color: var(--ff-text-2);
  font-size: var(--ff-fs-sm);
  background: var(--ff-bg-surface);
}
.rempty .ic {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--ff-bg-hover);
  color: var(--ff-text-3);
  display: grid;
  place-items: center;
}
.rempty .ic.err {
  color: var(--ff-danger-text);
  background: color-mix(in srgb, var(--ff-danger) 12%, transparent);
}

/* 拖入文件：只盖住输入面板的编辑区，不拦截鼠标事件 */
.dropov {
  position: absolute;
  inset: 0;
  z-index: 3;
  background: color-mix(in srgb, var(--ff-primary) 8%, transparent);
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}
.dropov::before {
  content: '';
  position: absolute;
  inset: 4px;
  border: 2px dashed var(--ff-primary);
  border-radius: var(--ff-radius-md);
}
.dropov .in {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--ff-space-2);
  color: var(--ff-primary-text);
}
.dropov b {
  font-size: var(--ff-fs-lg);
  font-weight: 600;
  line-height: 24px;
}
.dropov small {
  font-size: var(--ff-fs-xs);
  line-height: 16px;
  color: var(--ff-text-2);
}

/* Monaco 内部节点由 Monaco 创建，需要 :deep */
.ed :deep(.ff-json-errline) {
  background: color-mix(in srgb, var(--ff-danger) 8%, transparent);
}
.ed :deep(.ff-json-dot)::before {
  content: '';
  display: block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--ff-danger);
}
.ed :deep(.ff-json-dot) {
  display: flex;
  align-items: center;
  justify-content: center;
}
.ed :deep(.ff-json-margin + .line-numbers) {
  color: var(--ff-danger-text) !important;
  font-weight: 700;
}
.ed :deep(.ff-json-squiggle) {
  text-decoration: underline wavy var(--ff-danger);
  text-decoration-thickness: 1px;
  text-underline-offset: 4px;
  text-decoration-skip-ink: none;
}
</style>
