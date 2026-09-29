<template>
  <div class="pdfpage" :style="dropStyle">
    <section class="panel viewer-panel" aria-label="PDF 预览">
      <div class="phead">
        <FIcon name="doc" :size="18" class="pdficon" />
        <h2 :title="current?.path">{{ current?.name || '未打开文件' }}</h2>
        <span v-if="current" class="sub">{{ metaText }}</span>
        <span class="sp" />
        <template v-if="hasPdf">
          <button type="button" class="iconbtn" aria-label="缩小" :disabled="scale <= minScale" @click="zoom(-0.1)"><FIcon name="zout" :size="16" /></button>
          <span class="zoomv" aria-live="polite">{{ Math.round(scale * 100) }}%</span>
          <button type="button" class="iconbtn" aria-label="放大" :disabled="scale >= maxScale" @click="zoom(0.1)"><FIcon name="zin" :size="16" /></button>
        </template>
        <button type="button" class="btn" :disabled="picking" @click="choose"><FIcon name="folder" :size="15" />打开 PDF</button>
      </div>

      <div v-if="demo" class="demo" role="status"><FIcon name="info" :size="14" />{{ DOC_DEMO_NOTE }}</div>

      <div class="body">
        <div v-if="hasPdf" class="thumbs" aria-label="页面缩略图">
          <button
            v-for="n in Math.min(lazyPage, pages)"
            :key="n"
            type="button"
            class="thumb"
            :class="{ on: n === page }"
            :aria-label="`第 ${n} 页`"
            :aria-current="n === page ? 'page' : undefined"
            @click="page = n"
          >
            <span class="pg"><VuePDF :pdf="pdf" :page="n" :fit-parent="true" /></span>
            <span class="pgn">{{ n }}</span>
          </button>
        </div>

        <div class="viewer" :aria-busy="loading">
          <!-- 页面画好之前 VuePDF 就要挂上（它的 loaded 事件才会把 loading 关掉），所以画布不放在 loading 分支里，加载提示盖在上面 -->
          <template v-if="hasPdf">
            <div ref="scrollEl" class="scroll">
              <div class="paper">
                <VuePDF :pdf="pdf" :page="page" :scale="scale" @loaded="onLoaded" @error="onRenderError" />
              </div>
            </div>
            <div class="fbar" role="group" aria-label="翻页">
              <button type="button" class="iconbtn" aria-label="上一页" :disabled="page <= 1" @click="page--"><FIcon name="left" :size="16" /></button>
              <span class="pn" aria-live="polite">{{ page }} / {{ pages }}</span>
              <button type="button" class="iconbtn" aria-label="下一页" :disabled="page >= pages" @click="page++"><FIcon name="right" :size="16" /></button>
            </div>
          </template>

          <div v-if="loading" class="state over" role="status">{{ progressText }}</div>

          <div v-else-if="needPassword" class="state pw over" role="group" aria-label="输入密码">
            <b>{{ DOC_PDF_PASSWORD_TITLE }}</b>
            <span :class="{ bad: passwordWrong }">{{ passwordWrong ? DOC_PDF_PASSWORD_WRONG : DOC_PDF_PASSWORD_PROMPT }}</span>
            <form @submit.prevent="submitPassword">
              <input ref="pwInput" v-model="password" type="password" class="pwin" autocomplete="off" :aria-label="DOC_PDF_PASSWORD_PROMPT" />
              <button type="submit" class="btn pri" :disabled="!password">打开</button>
            </form>
          </div>

          <div v-else-if="error" class="state over" role="alert">
            <FIcon name="warn" :size="16" />
            <span class="bad">{{ error }}</span>
            <button v-if="current" type="button" class="ff-link" @click="open(current.path)">重试</button>
          </div>

          <div v-else-if="!hasPdf" class="state empty">
            <div class="ic"><FIcon name="upload" :size="28" /></div>
            <b>拖入 PDF，或点击「打开 PDF」</b>
            <small>本地读取，不上传</small>
          </div>
        </div>
      </div>
    </section>

    <aside class="panel recent" aria-labelledby="h-recent">
      <div class="phead"><h2 id="h-recent">最近打开</h2></div>
      <div v-if="!recent.length" class="rempty">{{ recentLoading ? '正在读取…' : '还没有打开过 PDF' }}</div>
      <ul v-else class="rlist">
        <li v-for="f in recent" :key="f.id" class="ritem" :class="{ on: current?.path === f.path, gone: !f.exists }">
          <button type="button" class="rmain" :title="f.path" @click="openRecent(f)">
            <FIcon name="doc" :size="16" class="pdficon" />
            <span class="rname">{{ f.name }}</span>
            <span class="rsub">{{ f.exists ? formatBytes(f.size) : '文件不存在' }}</span>
          </button>
          <button type="button" class="iconbtn rdel" :aria-label="`从列表移除 ${f.name}`" title="从列表移除（不删除文件）" @click="removeRecent(f)"><FIcon name="x" :size="14" /></button>
        </li>
      </ul>
    </aside>
  </div>
</template>

<script setup lang="ts">
// PDF 预览（契约 6.12.4）：OpenPDF → ≤ 64 MiB 用 ReadPDFChunk 读整份交给 pdf.js；更大的走 /local/<token> 按 Range 加载（HEAD 探测，404 重新 OpenPDF 一次）。
// 渲染完全在前端（@tato30/vue-pdf）；加密 PDF 由 pdf.js 的 onPassword 回调在页面内输入密码，后端不接触密码。错误文案统一走 errors/errorMessages.ts。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { VuePDF, usePDF } from '@tato30/vue-pdf'
import FIcon from '@/components/icon/FIcon.vue'
import { toAppError } from '@/api/call'
import { DEMO_PDF_PATH, MAX_RECENT_LIMIT, PDF_FILE_FILTER, isDocSim, listRecentPDFs, loadPDF, removeRecentPDFs, type PDFFile, type PDFSource } from '@/api/doc'
import { onFilesDropped } from '@/api/fileDrop'
import { pickFiles } from '@/api/system'
import { hasWailsBackend } from '@/services/wails'
import {
  DOC_DEMO_NOTE, DOC_PDF_PASSWORD_PROMPT, DOC_PDF_PASSWORD_TITLE, DOC_PDF_PASSWORD_WRONG, DOC_PDF_RENDER_FAILED_TEXT, docErrorText,
} from '@/errors/errorMessages'
import { formatBytes } from '@/utils/format'
import { ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()
const demo = isDocSim()
const dropStyle = { '--wails-drop-target': 'drop' } as Record<string, string>

// ---- pdf.js ----
const src = shallowRef<Uint8Array | string | null>(null)
const needPassword = ref(false)
const passwordWrong = ref(false)
const password = ref('')
const pwInput = ref<HTMLInputElement | null>(null)
let updatePassword: ((pw: string) => void) | null = null
const { pdf, pages } = usePDF(src, {
  onPassword: (update, reason: unknown) => {
    updatePassword = update
    passwordWrong.value = reason === 2 // pdfjs PasswordResponses.INCORRECT_PASSWORD
    password.value = ''
    needPassword.value = true
    loading.value = false
    nextTick(() => pwInput.value?.focus())
  },
  onError: () => onRenderError(),
})
function submitPassword() {
  if (!updatePassword || !password.value) return
  loading.value = true
  needPassword.value = false
  updatePassword(password.value)
}

const page = ref(1)
const lazyPage = ref(10)
const scale = ref(1)
const minScale = 0.6
const maxScale = 2.2
const zoom = (d: number) => (scale.value = Math.min(maxScale, Math.max(minScale, +(scale.value + d).toFixed(1))))
const hasPdf = computed(() => !!src.value && !!pdf.value && pages.value > 0 && !error.value)

const current = ref<PDFSource | null>(null)
const loading = ref(false)
const progressText = ref('正在打开…')
const error = ref('')
const pageSizeText = ref('')
const picking = ref(false)
const scrollEl = ref<HTMLElement | null>(null)

const metaText = computed(() => [pages.value > 0 && hasPdf.value ? `${pages.value} 页` : '', formatBytes(current.value?.size ?? 0), pageSizeText.value].filter(Boolean).join(' · '))

let seq = 0
let abort: AbortController | null = null

async function open(path: string) {
  if (!path) return
  abort?.abort()
  abort = new AbortController()
  const my = ++seq
  error.value = ''
  needPassword.value = false
  loading.value = true
  progressText.value = '正在打开…'
  pageSizeText.value = ''
  src.value = null
  try {
    const r = await loadPDF(
      path,
      (read, total) => {
        if (my === seq) progressText.value = `正在读取… ${Math.round((read / Math.max(total, 1)) * 100)}%`
      },
      abort.signal,
    )
    if (my !== seq) return
    current.value = r.src
    page.value = 1
    lazyPage.value = 10
    scale.value = 1
    src.value = r.data ?? r.url ?? null // 小文件：字节；大文件：/local/<token>（pdf.js 按 Range 加载）
    loadRecent()
  } catch (e) {
    if (my !== seq) return
    const err = toAppError(e)
    if (err.code === 'CANCELED') return
    loading.value = false
    current.value = { id: '', path, name: path.split(/[\\/]/).pop() || path, size: 0, url: '' }
    error.value = docErrorText(err.code, err.message, err.detail)
  }
}

function onLoaded() {
  loading.value = false
  updatePageSize()
}
function onRenderError() {
  loading.value = false
  needPassword.value = false
  error.value = DOC_PDF_RENDER_FAILED_TEXT
}

async function updatePageSize() {
  if (!pdf.value) return
  try {
    const p = await (pdf.value as unknown as { getPage(n: number): Promise<{ getViewport(o: { scale: number }): { width: number; height: number } }> }).getPage(page.value)
    const v = p.getViewport({ scale: 1 })
    pageSizeText.value = `${Math.round((v.width * 25.4) / 72)} × ${Math.round((v.height * 25.4) / 72)} mm`
  } catch {
    pageSizeText.value = ''
  }
}

watch(page, async (n) => {
  if (lazyPage.value - n < 3) lazyPage.value = Math.min(lazyPage.value + 10, pages.value)
  await nextTick()
  scrollEl.value && (scrollEl.value.scrollTop = 0)
  document.querySelector('.thumb.on')?.scrollIntoView({ block: 'nearest' })
  updatePageSize()
})
watch(pages, (n) => {
  if (n > 0) lazyPage.value = Math.min(Math.max(10, page.value + 9), n)
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
    error.value = docErrorText(err.code, err.message, err.detail)
  } finally {
    picking.value = false
  }
}
let offDrop: () => void = () => {}

// ---- 最近打开（doc_recent；OpenPDF 是唯一写入点）----
const recent = ref<PDFFile[]>([])
const recentLoading = ref(true)
async function loadRecent() {
  try {
    recent.value = await listRecentPDFs(MAX_RECENT_LIMIT)
  } catch (e) {
    console.error('读取最近打开失败', e)
  } finally {
    recentLoading.value = false
  }
}
function openRecent(f: PDFFile) {
  if (!f.exists) {
    ElMessage.warning('文件不存在或已被移动，可以从列表中移除')
    return
  }
  open(f.path)
}
async function removeRecent(f: PDFFile) {
  try {
    await removeRecentPDFs([f.id])
    recent.value = recent.value.filter((x) => x.id !== f.id)
    if (current.value?.path === f.path) {
      // 句柄已被撤销：清掉当前预览，避免继续按旧句柄读取
      seq++
      src.value = null
      current.value = null
      error.value = ''
    }
  } catch (e) {
    const err = toAppError(e)
    ElMessage.error(docErrorText(err.code, err.message, err.detail))
  }
}

onMounted(() => {
  offDrop = onFilesDropped((paths) => {
    const p = paths.find((x) => x.toLowerCase().endsWith('.pdf'))
    if (p) open(p)
    else ElMessage.warning('请拖入 PDF 文件')
  })
  loadRecent()
  const q = route.query.path
  if (typeof q === 'string' && q) {
    open(q)
    router.replace({ path: route.path }) // 用完即清，刷新不重复打开
  }
})
onBeforeUnmount(() => {
  offDrop()
  abort?.abort()
  seq++
})
</script>

<style scoped>
.pdfpage {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: var(--ff-space-4);
}
.pdfpage.wails-drop-target-active .viewer-panel {
  border-color: var(--ff-primary);
}
.panel {
  padding: 0;
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
}
.viewer-panel {
  flex: 1;
}
.recent {
  width: 300px;
  flex: none;
}
.phead {
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  padding: var(--ff-space-3) var(--ff-space-4);
  border-bottom: 1px solid var(--ff-border);
}
.phead h2 {
  margin: 0;
  font-size: var(--ff-fs-md);
  font-weight: 600;
  color: var(--ff-text-1);
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pdficon {
  color: var(--ff-danger);
  flex: none;
}
.sub {
  color: var(--ff-text-2);
  font-size: var(--ff-fs-xs);
  white-space: nowrap;
}
.sp {
  flex: 1;
}
.zoomv {
  width: 40px;
  text-align: center;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.demo {
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  padding: var(--ff-space-2) var(--ff-space-4);
  border-bottom: 1px solid var(--ff-border);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.body {
  flex: 1;
  min-height: 0;
  display: flex;
}
.thumbs {
  width: 150px;
  flex: none;
  overflow-y: auto;
  padding: var(--ff-space-3);
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-3);
  align-items: center;
  border-right: 1px solid var(--ff-border);
}
.thumb {
  padding: 0;
  border: 0;
  background: transparent;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--ff-space-1);
  cursor: pointer;
  font: inherit;
}
.thumb .pg {
  width: 112px;
  border-radius: var(--ff-radius-sm);
  background: #fff;
  border: 1px solid var(--ff-border);
  overflow: hidden;
  line-height: 0;
}
.thumb .pg :deep(canvas) {
  width: 100% !important;
  height: auto !important;
}
.thumb.on .pg {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.thumb:focus-visible .pg {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.pgn {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.viewer {
  flex: 1;
  min-width: 0;
  position: relative;
  background: var(--ff-bg-app);
  display: flex;
}
.scroll {
  flex: 1;
  overflow: auto;
  padding: var(--ff-space-5) var(--ff-space-6) 64px;
  display: flex;
  justify-content: center;
  align-items: flex-start;
}
.paper {
  background: #fff; /* PDF 页面本身是白底，暗色主题下也保持白纸 */
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  border-radius: var(--ff-radius-sm);
  line-height: 0;
  max-width: 100%;
}
.paper :deep(canvas) {
  max-width: 100%;
  height: auto !important;
}
.fbar {
  position: absolute;
  bottom: var(--ff-space-4);
  left: 50%;
  transform: translateX(-50%);
  height: 36px;
  border-radius: 18px;
  background: var(--ff-bg-elevated);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.14);
  border: 1px solid var(--ff-border);
  display: flex;
  align-items: center;
  gap: var(--ff-space-1);
  padding: 0 var(--ff-space-2);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.pn {
  padding: 0 6px;
  min-width: 56px;
  text-align: center;
}
.state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--ff-space-2);
  padding: var(--ff-space-6);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  text-align: center;
}
.state b {
  font-size: var(--ff-fs-md);
  color: var(--ff-text-1);
  font-weight: 500;
}
.state .bad {
  color: var(--ff-danger-text);
}
.state.over {
  position: absolute;
  inset: 0;
  background: color-mix(in srgb, var(--ff-bg-app) 92%, transparent);
  z-index: 1;
}
.state.empty .ic {
  width: 56px;
  height: 56px;
  border-radius: var(--ff-radius-xl);
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
  display: grid;
  place-items: center;
}
.state.empty b {
  font-size: var(--ff-fs-lg);
}
.pw form {
  display: flex;
  gap: var(--ff-space-2);
  margin-top: var(--ff-space-2);
}
.pwin {
  height: 28px;
  width: 200px;
  padding: 0 var(--ff-space-3);
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-md);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  font: inherit;
  font-size: var(--ff-fs-sm);
}
.pwin:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.ff-link {
  padding: 0;
  border: none;
  background: transparent;
  font: inherit;
  color: var(--ff-primary-text);
  cursor: pointer;
  border-radius: 2px;
}
.ff-link:hover {
  text-decoration: underline;
}
.rempty {
  padding: var(--ff-space-6) var(--ff-space-4);
  text-align: center;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.rlist {
  list-style: none;
  margin: 0;
  padding: var(--ff-space-3);
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-2);
  overflow-y: auto;
  flex: 1;
  min-height: 0;
}
.ritem {
  display: flex;
  align-items: center;
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-lg);
}
.ritem.on {
  border-color: var(--ff-primary);
  background: var(--ff-primary-soft);
}
.ritem.gone .rname {
  color: var(--ff-text-2);
}
.rmain {
  flex: 1;
  min-width: 0;
  display: grid;
  grid-template-columns: auto 1fr;
  column-gap: var(--ff-space-2);
  align-items: center;
  text-align: left;
  padding: var(--ff-space-2) var(--ff-space-3);
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
  border-radius: var(--ff-radius-lg);
}
.rmain:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: -2px;
}
.rname {
  font-size: var(--ff-fs-sm);
  font-weight: 500;
  color: var(--ff-text-1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.rsub {
  grid-column: 2;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.rdel {
  margin-right: var(--ff-space-1);
}
.iconbtn {
  width: 28px;
  height: 28px;
  flex: none;
  display: inline-grid;
  place-items: center;
  border: 0;
  border-radius: var(--ff-radius-md);
  background: transparent;
  color: var(--ff-text-2);
  cursor: pointer;
}
.iconbtn:hover:not(:disabled) {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.iconbtn:disabled {
  opacity: 0.45;
  cursor: default;
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
  margin-left: var(--ff-space-2);
}
.btn:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.btn:disabled {
  opacity: 0.45;
  cursor: default;
}
.btn.pri {
  background: var(--ff-badge-bg);
  border-color: var(--ff-badge-bg);
  color: var(--ff-on-primary);
  margin-left: 0;
}
@media (max-width: 1100px) {
  .recent {
    width: 240px;
  }
  .thumbs {
    width: 128px;
  }
  .thumb .pg {
    width: 96px;
  }
}
</style>
