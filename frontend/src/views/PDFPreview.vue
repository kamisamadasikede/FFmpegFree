<template>
  <section class="panel viewer-panel" :class="{ hover: simHover }" :style="dropStyle" aria-label="PDF 预览">
    <div class="tb">
      <FIcon name="doc" :size="18" class="pdficon" />
      <MiddleEllipsis v-if="current" class="nm" :text="current.name" />
      <span v-else class="nm ph">{{ DOC_PDF_NOT_OPENED }}</span>
      <span v-if="hasPdf" class="sub">{{ metaText }}</span>
      <div class="zm" role="group" aria-label="缩放">
        <button type="button" class="zb" aria-label="缩小" :disabled="!hasPdf || scale <= ZOOM_STEPS[0]" @click="scale = nextZoom(scale, -1)"><FIcon name="zout" :size="16" /></button>
        <span class="zv" :class="{ off: !hasPdf }" aria-live="polite">{{ Math.round(scale * 100) }}%</span>
        <button type="button" class="zb" aria-label="放大" :disabled="!hasPdf || scale >= ZOOM_STEPS[ZOOM_STEPS.length - 1]" @click="scale = nextZoom(scale, 1)"><FIcon name="zin" :size="16" /></button>
      </div>
      <button type="button" class="btn" :disabled="picking" @click="choose">选择 PDF</button>
    </div>

    <div v-if="demo" class="demo" role="status"><FIcon name="info" :size="14" />{{ DOC_DEMO_NOTE }}</div>

    <div class="pv">
      <div class="th" aria-label="页面缩略图">
        <template v-if="hasPdf">
          <div ref="thumbEl" class="thscroll" @scroll.passive="onThumbScroll">
            <div :style="{ height: (win.from - 1) * THUMB_SLOT + 'px' }" />
            <button
              v-for="n in winPages"
              :key="n"
              type="button"
              class="slot"
              :aria-label="`第 ${n} 页`"
              :aria-current="n === page ? 'page' : undefined"
              @click="page = n"
            >
              <span class="tp" :class="{ on: n === page }"><VuePDF :pdf="pdf" :page="n" :fit-parent="true" /></span>
              <span class="tn" :class="{ on: n === page }">{{ n }}</span>
            </button>
            <div :style="{ height: Math.max(0, pages - win.to) * THUMB_SLOT + 'px' }" />
          </div>
        </template>
        <template v-else-if="phase === 'loading'">
          <div v-for="n in 5" :key="n" class="slot sk"><span class="tp sk" /></div>
        </template>
      </div>

      <div class="vwrap" :aria-busy="phase === 'loading'">
        <!-- VuePDF 要在页面画好之前就挂上（它的 loaded 事件才会结束加载态），所以画布不放在加载分支里，加载态盖在上面 -->
        <div v-if="hasPdf" ref="scrollEl" class="vw">
          <div class="pgs">
            <div class="paper"><VuePDF :pdf="pdf" :page="page" :scale="scale" @loaded="onLoaded" @error="onRenderError" /></div>
          </div>
        </div>
        <div v-if="hasPdf" class="pbar" role="group" aria-label="翻页">
          <button type="button" class="zb sm" aria-label="上一页" :disabled="page <= 1" @click="page--"><FIcon name="left" :size="16" /></button>
          <input v-model="pageInput" class="pn" inputmode="numeric" aria-label="页码" @keydown.enter.prevent="jump" @blur="jump" />
          <span aria-live="polite">/ {{ pages }}</span>
          <button type="button" class="zb sm" aria-label="下一页" :disabled="page >= pages" @click="page++"><FIcon name="right" :size="16" /></button>
        </div>

        <div v-if="showLoading" class="ld" role="status">
          <span class="spin" aria-hidden="true" />
          <b>{{ DOC_PDF_LOADING_TITLE }}</b>
          <div class="bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="loadPct" aria-label="读取进度"><i :style="{ width: loadPct + '%' }" /></div>
          <span v-if="loadTotal > 0">已读取 {{ formatBytes(loadRead) }} / {{ formatBytes(loadTotal) }}（{{ loadPct }}%）</span>
          <button type="button" class="btn" @click="cancelLoad">取消</button>
        </div>

        <div v-else-if="phase === 'password'" class="pw" role="group" aria-label="输入密码">
          <div class="card">
            <div class="ic"><FIcon name="lock" :size="24" /></div>
            <h5>{{ DOC_PDF_PASSWORD_TITLE }}</h5>
            <p>{{ DOC_PDF_PASSWORD_PROMPT }}</p>
            <form @submit.prevent="submitPassword">
              <input
                ref="pwInput"
                v-model="password"
                type="password"
                class="pwin"
                :class="{ bad: passwordWrong }"
                autocomplete="off"
                :placeholder="DOC_PDF_PASSWORD_PLACEHOLDER"
                :aria-label="DOC_PDF_PASSWORD_PLACEHOLDER"
                :aria-invalid="passwordWrong"
                :aria-describedby="passwordWrong ? 'pw-err' : undefined"
              />
              <div v-if="passwordWrong" id="pw-err" class="pwerr" role="alert">{{ DOC_PDF_PASSWORD_WRONG }}</div>
              <div class="pwacts">
                <button type="button" class="btn" @click="cancelPassword">取消</button>
                <button type="submit" class="btn pri" :disabled="!password">打开</button>
              </div>
            </form>
          </div>
        </div>

        <div v-else-if="phase === 'error' && errView" class="fail" role="alert">
          <div class="ic"><FIcon name="warn" :size="24" /></div>
          <h5>{{ DOC_PDF_ERROR_TITLE }}</h5>
          <p>{{ errView.text }}</p>
          <div class="acts">
            <button v-if="errView.retry && current" type="button" class="btn" @click="open(current.path)">重试</button>
            <button type="button" class="btn pri" @click="choose">{{ errView.retry ? '选择其他 PDF' : '选择其他 PDF' }}</button>
          </div>
        </div>

        <div v-else-if="phase === 'empty'" class="drop" role="button" tabindex="0" aria-label="选择 PDF 文件" @keydown.enter.prevent="choose" @keydown.space.prevent="choose">
          <div class="ic"><FIcon name="doc" :size="24" /></div>
          <template v-if="simHover">
            <b>{{ DOC_PDF_DROP_HOVER }}</b>
          </template>
          <template v-else>
            <b>{{ DOC_PDF_EMPTY_TITLE }}</b>
            <span>{{ DOC_PDF_EMPTY_HINT }}</span>
            <button type="button" class="btn pri" :disabled="picking" @click="choose">选择 PDF</button>
          </template>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
// PDF 预览（契约 6.12.4，设计说明 3）：OpenPDF → ≤ 64 MiB 用 ReadPDFChunk 读整份交给 pdf.js；更大的走 /local/<token> 按 Range 加载（HEAD 探测，404 重新 OpenPDF 一次）。
// 渲染完全在前端（@tato30/vue-pdf）；加密 PDF 由 pdf.js 的 onPassword 回调在页面内输入密码，密码只放组件内存，后端不接触、不落盘、不写日志。
// 错误文案统一走 errors/errorMessages.ts（pdfErrorView / DOC_PDF_*）。页面在 KeepAlive 里：切到 Office 转 PDF 再回来，当前打开的 PDF 不丢。
import { computed, nextTick, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { VuePDF, usePDF } from '@tato30/vue-pdf'
import FIcon from '@/components/icon/FIcon.vue'
import MiddleEllipsis from '@/components/docs/MiddleEllipsis.vue'
import { toAppError } from '@/api/call'
import { DEFAULT_DOC_LIMITS, DEMO_PDF_PATH, PDF_FILE_FILTER, getDocCapabilities, isDocSim, loadPDF, type DocLimits, type PDFSource } from '@/api/doc'
import { simParam } from '@/api/sim'
import { pickFiles } from '@/api/system'
import { hasWailsBackend } from '@/services/wails'
import {
  DOC_DEMO_NOTE, DOC_PDF_DROP_HOVER, DOC_PDF_EMPTY_HINT, DOC_PDF_EMPTY_TITLE, DOC_PDF_ERROR_TITLE, DOC_PDF_LOADING_TITLE, DOC_PDF_NOT_OPENED,
  DOC_PDF_PARSE_FAILED_CODE, DOC_PDF_PASSWORD_PLACEHOLDER, DOC_PDF_PASSWORD_PROMPT, DOC_PDF_PASSWORD_TITLE, DOC_PDF_PASSWORD_WRONG, pdfErrorView, type PdfErrorView,
} from '@/errors/errorMessages'
import { dropHandlers, useDocsStore } from '@/stores/docs'
import { formatBytes } from '@/utils/format'
import { ZOOM_STEPS, nextZoom, thumbWindow } from '@/utils/docLogic'
import { ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()
const docs = useDocsStore()
const demo = isDocSim()
const dropStyle = { '--wails-drop-target': 'drop' } as Record<string, string>
const simHover = demo && simParam('sim_hover') === '1'

type Phase = 'empty' | 'loading' | 'ready' | 'error' | 'password'
const phase = ref<Phase>('empty')
const limits = ref<DocLimits>(DEFAULT_DOC_LIMITS)

// ---- pdf.js ----
const src = shallowRef<Uint8Array | string | null>(null)
const password = ref('')
const passwordWrong = ref(false)
const pwInput = ref<HTMLInputElement | null>(null)
let updatePassword: ((pw: string) => void) | null = null
let urlMode = false // 大文件走 Range 时，读取进度用 pdf.js 的 onProgress
const { pdf, pages } = usePDF(src, {
  onProgress: (p: { loaded: number; total: number }) => {
    if (urlMode && p?.total) {
      loadRead.value = p.loaded
      loadTotal.value = p.total
    }
  },
  onPassword: (update, reason: unknown) => {
    updatePassword = update
    passwordWrong.value = reason === 2 // pdfjs PasswordResponses：NEED_PASSWORD = 1，INCORRECT_PASSWORD = 2
    password.value = ''
    setPhase('password')
    nextTick(() => pwInput.value?.focus())
  },
  onError: () => onRenderError(),
})
function submitPassword() {
  if (!updatePassword || !password.value) return
  const pw = password.value
  password.value = ''
  setPhase('loading')
  loadRead.value = 0
  loadTotal.value = 0
  updatePassword(pw)
}
function cancelPassword() {
  password.value = ''
  updatePassword = null
  reset()
}

const page = ref(1)
const pageInput = ref('1')
const scale = ref(1) // 不记忆：每次打开都是 100%
const hasPdf = computed(() => !!src.value && !!pdf.value && pages.value > 0 && phase.value !== 'error' && phase.value !== 'password')
watch(page, (n) => (pageInput.value = String(n)))
function jump() {
  const n = Math.trunc(Number(pageInput.value))
  page.value = Number.isFinite(n) && n >= 1 ? Math.min(n, Math.max(1, pages.value)) : page.value
  pageInput.value = String(page.value)
}

const current = ref<PDFSource | null>(null)
const errView = ref<PdfErrorView | null>(null)
const picking = ref(false)
const scrollEl = ref<HTMLElement | null>(null)
const metaText = computed(() => [`${pages.value} 页`, formatBytes(current.value?.size ?? 0)].join(' · '))

// ---- 加载态：200ms 内完成就不出现（防闪烁）----
const loadRead = ref(0)
const loadTotal = ref(0)
const loadPct = computed(() => (loadTotal.value > 0 ? Math.min(100, Math.floor((loadRead.value / loadTotal.value) * 100)) : 0))
const delayed = ref(false)
let delayTimer: ReturnType<typeof setTimeout> | undefined
function setPhase(p: Phase) {
  phase.value = p
  clearTimeout(delayTimer)
  delayed.value = false
  if (p === 'loading') delayTimer = setTimeout(() => (delayed.value = true), 200)
}
const showLoading = computed(() => phase.value === 'loading' && (delayed.value || simLoading))

let seq = 0
let abort: AbortController | null = null

function reset() {
  abort?.abort()
  seq++
  src.value = null
  current.value = null
  errView.value = null
  urlMode = false
  docs.currentPath = ''
  setPhase('empty')
}
function fail(path: string, view: PdfErrorView) {
  current.value = current.value?.path === path ? current.value : { id: '', path, name: path.split(/[\\/]/).pop() || path, size: 0, url: '' }
  src.value = null
  errView.value = view
  setPhase('error')
}

async function open(path: string) {
  if (!path) return
  abort?.abort()
  abort = new AbortController()
  const my = ++seq
  errView.value = null
  src.value = null
  urlMode = false
  loadRead.value = 0
  loadTotal.value = 0
  docs.currentPath = path
  current.value = { id: '', path, name: path.split(/[\\/]/).pop() || path, size: 0, url: '' }
  setPhase('loading')
  try {
    const r = await loadPDF(
      path,
      (read, total) => {
        if (my !== seq) return
        loadRead.value = read
        loadTotal.value = total
      },
      abort.signal,
    )
    if (my !== seq) return
    current.value = r.src
    page.value = 1
    pageInput.value = '1'
    scale.value = 1
    urlMode = !!r.url
    if (r.url) loadTotal.value = r.src.size
    src.value = r.data ?? r.url ?? null // 小文件：字节；大文件：/local/<token>（pdf.js 按 Range 加载）
    docs.loadRecent() // OpenPDF 已写入最近列表
  } catch (e) {
    if (my !== seq) return
    const err = toAppError(e)
    if (err.code === 'CANCELED') return
    fail(path, pdfErrorView(err.code, err.message, limits.value.maxPdfBytes, err.detail))
  }
}

function cancelLoad() {
  reset() // 丢弃读取循环：AbortController + 序号，不需要后端取消接口
}

function onLoaded() {
  if (phase.value === 'loading') setPhase('ready')
  if (phase.value !== 'ready') return
}
function onRenderError() {
  const p = current.value?.path ?? ''
  fail(p, pdfErrorView(DOC_PDF_PARSE_FAILED_CODE, '', limits.value.maxPdfBytes))
}

// ---- 缩略图窗口（可视范围 ± 2 屏才渲染，5000 页不能一次渲染完）----
const THUMB_SLOT = 116 // 88 高纸张 + 4 间距 + 16 页号 + 8 下边距
const thumbEl = ref<HTMLElement | null>(null)
const thumbScroll = ref(0)
const thumbView = ref(600)
function onThumbScroll() {
  thumbScroll.value = thumbEl.value?.scrollTop ?? 0
  thumbView.value = thumbEl.value?.clientHeight ?? 600
}
const win = computed(() => thumbWindow(thumbScroll.value, thumbView.value, THUMB_SLOT, pages.value))
const winPages = computed(() => {
  const out: number[] = []
  for (let n = win.value.from; n <= win.value.to; n++) out.push(n)
  return out
})
watch(page, async (n) => {
  await nextTick()
  if (scrollEl.value) scrollEl.value.scrollTop = 0
  const el = thumbEl.value
  if (el) {
    const top = (n - 1) * THUMB_SLOT
    if (top < el.scrollTop) el.scrollTop = top
    else if (top + THUMB_SLOT > el.scrollTop + el.clientHeight) el.scrollTop = top + THUMB_SLOT - el.clientHeight
  }
})
watch(pages, async () => {
  await nextTick()
  onThumbScroll()
})

// ---- 选择 / 拖入 ----
async function choose() {
  if (picking.value) return
  picking.value = true
  try {
    const paths = hasWailsBackend() ? await pickFiles(PDF_FILE_FILTER, false) : [DEMO_PDF_PATH]
    if (paths[0]) await open(paths[0])
  } catch (e) {
    const err = toAppError(e)
    ElMessage.error(pdfErrorView(err.code, err.message, limits.value.maxPdfBytes, err.detail).text)
  } finally {
    picking.value = false
  }
}
function onDrop(paths: string[]) {
  const p = paths.find((x) => x.toLowerCase().endsWith('.pdf'))
  if (p) open(p)
  else ElMessage.warning('请拖入 PDF 文件')
}

// ---- 从最近列表 / Office 页的「打开 PDF」进来：?path=…（用完即清，刷新不重复打开）；被从最近列表移除的正在看的文件要清掉 ----
function consumeQuery() {
  if (route.path !== '/docs/pdf') return
  const q = route.query.path
  if (typeof q === 'string' && q) {
    open(q)
    router.replace({ path: route.path })
  }
}
watch(() => route.query.path, consumeQuery)
watch(
  () => docs.removedPath,
  (p) => {
    // 句柄已被撤销：清掉当前预览，避免继续按旧句柄读取
    if (p && current.value?.path === p) reset()
  },
)

// 仅模拟环境的预览参数（真实环境不读）：sim_view=loading|pass|wrong 直接显示加载中 / 需要密码 / 密码错误
let simLoading = false
function applySimView() {
  const v = simParam('sim_view')
  if (!demo || !v) return
  current.value = { id: 'sim', path: '/Users/me/Documents/2026 Q3 产品回顾.pdf', name: '2026 Q3 产品回顾.pdf', size: 42 * 1024 * 1024, url: '' }
  if (v === 'loading') {
    simLoading = true
    loadRead.value = Math.round(18.6 * 1024 * 1024)
    loadTotal.value = 42 * 1024 * 1024
    phase.value = 'loading'
  } else if (v === 'pass' || v === 'wrong') {
    passwordWrong.value = v === 'wrong'
    phase.value = 'password'
  }
}

onMounted(() => {
  getDocCapabilities().then((c) => (limits.value = c.limits)).catch(() => undefined)
  applySimView()
  consumeQuery()
})
onActivated(() => {
  dropHandlers.pdf = onDrop
  docs.currentPath = current.value?.path ?? ''
  docs.loadRecent()
  consumeQuery()
})
onDeactivated(() => {
  delete dropHandlers.pdf
})
onBeforeUnmount(() => {
  abort?.abort()
  seq++
  clearTimeout(delayTimer)
})
</script>

<style scoped>
.viewer-panel {
  flex: 1;
  min-width: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}
.viewer-panel.wails-drop-target-active,
.viewer-panel.hover {
  border-color: var(--ff-primary);
}
.tb {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--ff-border);
  min-height: 52px;
  box-sizing: border-box;
}
.pdficon {
  color: var(--ff-text-2);
  flex: none;
}
.nm {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
  color: var(--ff-text-1);
}
.nm.ph {
  color: var(--ff-text-3);
  font-weight: 400;
  font-size: 13px;
}
.sub {
  font-size: 12px;
  color: var(--ff-text-2);
  white-space: nowrap;
}
.zm {
  display: flex;
  align-items: center;
  flex: none;
}
.zb {
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--ff-text-2);
  display: grid;
  place-items: center;
  cursor: pointer;
}
.zb.sm {
  width: 24px;
  height: 24px;
}
.zb:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.zb:disabled {
  color: var(--ff-text-3);
  cursor: not-allowed;
}
.zb:focus-visible,
.btn:focus-visible,
.slot:focus-visible,
.pn:focus-visible,
.pwin:focus-visible,
.drop:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.zv {
  width: 48px;
  text-align: center;
  font-size: 12px;
  color: var(--ff-text-1);
}
.zv.off {
  color: var(--ff-text-3);
}
.demo {
  flex: none;
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  padding: var(--ff-space-2) var(--ff-space-4);
  border-bottom: 1px solid var(--ff-border);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.pv {
  flex: 1;
  min-height: 0;
  display: flex;
}
.th {
  width: 104px;
  flex: none;
  border-right: 1px solid var(--ff-border);
  overflow: hidden;
  padding: 12px 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  box-sizing: border-box;
}
.thscroll {
  width: 100%;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
}
.slot {
  width: 100%;
  height: 116px;
  padding: 0;
  border: 0;
  background: transparent;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  font: inherit;
  box-sizing: border-box;
}
.slot.sk {
  height: auto;
  margin-bottom: 8px;
}
.tp {
  width: 64px;
  min-height: 88px;
  flex: none;
  background: #fff; /* PDF 页面本身是白底，暗色主题下也保持白纸，不做反相 */
  border: 1px solid var(--ff-border);
  border-radius: 4px;
  overflow: hidden;
  line-height: 0;
  box-sizing: border-box;
}
.tp.sk {
  height: 88px;
  background: var(--ff-bg-hover);
  border-color: transparent;
}
.tp.on {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.tp :deep(canvas) {
  width: 100% !important;
  height: auto !important;
}
.tn {
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-text-2);
}
.tn.on {
  color: var(--ff-primary-text);
  font-weight: 500;
}
.vwrap {
  flex: 1;
  min-width: 0;
  position: relative;
  display: flex;
  background: var(--ff-bg-app);
}
.vw {
  flex: 1;
  min-width: 0;
  overflow: auto; /* 200% 时出现横向滚动条 */
}
.pgs {
  width: max-content;
  min-width: 100%;
  box-sizing: border-box;
  padding: 16px 16px 64px;
  display: flex;
  flex-direction: column;
  align-items: center;
}
.paper {
  background: #fff;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  border-radius: 4px;
  line-height: 0;
}
.pbar {
  position: absolute;
  bottom: 16px;
  left: 50%;
  transform: translateX(-50%);
  height: 36px;
  border-radius: 18px;
  background: var(--ff-bg-elevated);
  border: 1px solid var(--ff-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.14);
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 8px;
  font-size: 12px;
  color: var(--ff-text-2);
  white-space: nowrap;
}
.pn {
  width: 40px;
  min-width: 32px;
  height: 24px;
  box-sizing: border-box;
  border: 1px solid var(--ff-border);
  border-radius: 4px;
  text-align: center;
  color: var(--ff-text-1);
  background: var(--ff-bg-surface);
  font: inherit;
  font-size: 12px;
}
.ld,
.fail,
.pw {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  text-align: center;
  padding: 24px;
  background: var(--ff-bg-app);
  z-index: 1;
}
.pw {
  display: grid;
  place-items: center;
}
.ld b,
.fail h5,
.pw h5 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--ff-text-1);
}
.ld span,
.fail .code {
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-text-2);
}
.spin {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  border: 3px solid var(--ff-border);
  border-top-color: var(--ff-primary);
  animation: pvspin 1s linear infinite;
  display: block;
  margin-bottom: 8px;
}
@keyframes pvspin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .spin {
    animation: none;
  }
}
.bar {
  width: 240px;
  height: 4px;
  border-radius: 2px;
  background: var(--ff-border);
  overflow: hidden;
  margin: 4px 0;
}
.bar i {
  display: block;
  height: 100%;
  background: var(--ff-primary);
  border-radius: 2px;
  transition: width var(--ff-dur-progress) linear;
}
.fail .ic,
.drop .ic {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  margin-bottom: 8px;
}
.fail .ic {
  background: color-mix(in srgb, var(--ff-danger) 12%, transparent);
  color: var(--ff-danger-text);
}
.fail p {
  margin: 0;
  max-width: 320px;
  font-size: 13px;
  line-height: 20px;
  color: var(--ff-text-2);
}
.fail .code {
  font-family: var(--ff-font-mono);
}
.acts {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
.drop {
  position: absolute;
  inset: 16px;
  border: 1.5px dashed var(--ff-border);
  border-radius: 10px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  text-align: center;
  padding: 24px;
}
.viewer-panel.wails-drop-target-active .drop,
.viewer-panel.hover .drop {
  border-color: var(--ff-primary);
  background: color-mix(in srgb, var(--ff-primary) 8%, transparent);
}
.drop .ic {
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
}
.drop b {
  font-size: 14px;
  font-weight: 600;
  color: var(--ff-text-1);
}
.drop span {
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-text-2);
}
.drop .btn {
  margin-top: 8px;
}
.pw .card {
  width: 320px;
  box-sizing: border-box;
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: 10px;
  padding: 24px;
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
  text-align: center;
}
.pw .ic {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
  display: grid;
  place-items: center;
  margin: 0 auto 8px;
}
.pw p {
  margin: 0 0 8px;
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-text-2);
}
.pw form {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.pwin {
  height: 32px;
  box-sizing: border-box;
  padding: 0 var(--ff-space-3);
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-md);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  font: inherit;
  font-size: 13px;
  text-align: left;
}
.pwin.bad {
  border-color: var(--ff-danger);
}
.pwerr {
  text-align: left;
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-danger-text);
}
.pwacts {
  display: flex;
  justify-content: center;
  gap: 8px;
  margin-top: 8px;
}
.btn {
  height: 28px;
  padding: 0 12px;
  border-radius: var(--ff-radius-md);
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  display: inline-flex;
  align-items: center;
  gap: var(--ff-space-2);
  font: inherit;
  font-size: var(--ff-fs-sm);
  white-space: nowrap;
  cursor: pointer;
}
.btn:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.btn.pri {
  background: var(--ff-badge-bg);
  border-color: var(--ff-badge-bg);
  color: var(--ff-on-primary);
}
.btn.pri:hover:not(:disabled) {
  background: var(--ff-primary-hover);
  border-color: var(--ff-primary-hover);
}
</style>
