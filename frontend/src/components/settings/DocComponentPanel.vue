<template>
  <section class="panel group" :aria-labelledby="headingId">
    <div class="phead">
      <h2 :id="headingId">文档组件</h2>
      <span class="dsub">文档、表格、演示的转换</span>
    </div>
    <!-- 设计 v0.2 场景 13 / 13b / 13c / 13L：不显示路径；版本号为空不显示版本行 -->
    <div class="srow fcomp">
      <div class="l">
        <span class="fok"><i class="fdot" :class="dot" aria-hidden="true" />{{ title }}</span>
        <small v-if="detail">{{ detail }}</small>
        <div v-if="comp.inFlight && comp.compState === 'downloading'" class="bar dbar" role="progressbar" aria-label="下载进度" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="comp.pct"><i :style="{ width: comp.pct + '%' }" /></div>
        <div v-else-if="comp.compState === 'preparing'" class="bar ind dbar" role="progressbar" :aria-label="DOC_PREPARING" aria-busy="true"><i /></div>
      </div>
      <div class="facts">
        <template v-if="!comp.isLinux">
          <button v-if="canDownload" type="button" class="btn pri" :disabled="comp.busy" @click="comp.install()"><FIcon name="download" :size="15" />{{ failed ? '重试' : comp.compState === 'outdated' ? DOC_OUTDATED_BUTTON : '下载' }}</button>
          <button v-if="comp.compState === 'downloading'" type="button" class="btn" :disabled="comp.busy" @click="comp.cancel()">取消下载</button>
        </template>
        <button v-if="comp.compState === 'checking'" type="button" class="btn" disabled>正在检测…</button>
        <!-- 6.12.54：只在用的是应用下载的文档组件且已就绪时显示 -->
        <button v-if="showFolder" type="button" class="btn" @click="openFolder"><FIcon name="folder" :size="15" />打开组件所在文件夹</button>
      </div>
    </div>
    <!-- v0.27（6.12.31，设计场景 21）：本机不止一个引擎时多一行「转换引擎」 -->
    <div v-if="comp.status.engines.length > 1" class="srow feng">
      <div class="l">
        <span class="fok">{{ ENGINE_ROW_TITLE }}</span>
        <small>{{ ENGINE_ROW_HINT }}</small>
      </div>
      <div ref="ddEl" class="fdd">
        <button type="button" class="fdd-btn" aria-haspopup="listbox" :aria-expanded="ddOpen" aria-label="转换引擎" @click="ddOpen = !ddOpen" @keydown.down.prevent="ddOpen = true">
          <span>{{ currentLabel }}</span><FIcon name="down" :size="14" />
        </button>
        <ul v-if="ddOpen" class="fdd-list" role="listbox" aria-label="转换引擎" @keydown.esc.stop="ddOpen = false">
          <li v-for="o in options" :key="o.id" role="option" :aria-selected="o.id === pref" tabindex="0" :class="{ on: o.id === pref }" @click="choose(o.id)" @keydown.enter.prevent="choose(o.id)">
            <FIcon v-if="o.id === pref" name="check" :size="14" class="ck" /><i v-else class="ck" />
            <div><b>{{ o.label }}</b><small v-if="o.hint">{{ o.hint }}</small></div>
          </li>
        </ul>
      </div>
    </div>
    <div v-if="folderErr" class="srow"><small class="ferr" role="alert">{{ folderErr }}</small></div>
    <!-- 选了未下载的文档组件：先确认（场景 21b） -->
    <MotionDialog>
    <div v-if="confirmDl" class="fcf-mask" @mousedown.self="cancelDl">
      <div class="fcf ff-panel" role="alertdialog" aria-modal="true" aria-labelledby="fcf-t" @keydown.esc="cancelDl">
        <h4 id="fcf-t">下载文档组件</h4>
        <p>{{ componentDownloadConfirmText(comp.status.downloadBytes) }}</p>
        <div class="acts">
          <button type="button" class="btn" @click="cancelDl">取消</button>
          <button ref="dlBtn" type="button" class="btn pri" @click="confirmDownload">下载</button>
        </div>
      </div>
    </div>
    </MotionDialog>
  </section>
</template>

<script setup lang="ts">
// 设置页「文档组件」块（契约 6.12.12 / v0.27 6.12.28：读 componentState，不读整体 state；设计 v0.2 §四）。和「转换组件」同一套行样式。
// v0.27：「正在使用本机 WPS」行、引擎下拉（engines 多于一项）、下载确认；v0.27.2：OpenStorageFolder("doc_component")。
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import MotionDialog from '@/components/motion/MotionDialog.vue'
import { useDocComponentStore } from '@/stores/docComponent'
import { DOC_OUTDATED_BUTTON, DOC_PREPARING } from '@/utils/docV26Text'
import {
  COMPONENT_NOT_DOWNLOADED,
  DOC_COMPONENT_FOLDER_NOT_READY,
  ENGINE_AUTO_HINT,
  ENGINE_AUTO_LABEL,
  ENGINE_ROW_HINT,
  ENGINE_ROW_TITLE,
  componentDownloadConfirmText,
  componentOptionHint,
  engineInUseText,
  type DocEnginePref,
} from '@/utils/docV27Text'
import { getDocEngine, openDocComponentFolder, setDocEngine } from '@/api/docV27'
import { toAppError } from '@/api/call'
import { formatBytes } from '@/utils/format'

defineProps<{ headingId?: string }>()
const comp = useDocComponentStore()
const SOURCE: Record<string, string> = { downloaded: '应用下载', system: '系统自带' }
const failed = computed(() => comp.compState === 'failed')
/** 本机 Office / WPS 在用、文档组件没在下载 / 准备 / 失败：整块按「已就绪」显示（设计场景 20），组件要下载从下拉框里选 */
const viaLocal = computed(() => comp.status.state === 'ready' && ['office', 'wps'].includes(comp.status.source) && !['downloading', 'preparing', 'failed', 'ready'].includes(comp.compState))
const shown = computed(() => (viaLocal.value ? 'ready' : comp.compState))
const canDownload = computed(() => !viaLocal.value && ['missing', 'outdated', 'failed'].includes(comp.compState))
const dot = computed(() => {
  const s = shown.value
  if (s === 'ready') return ''
  if (comp.isLinux || s === 'failed' || s === 'outdated') return 'bad'
  return 'off'
})
// ── 引擎 ──
const pref = ref<DocEnginePref>('auto')
const ddOpen = ref(false)
const ddEl = ref<HTMLElement | null>(null)
const confirmDl = ref(false)
const dlBtn = ref<HTMLButtonElement | null>(null)
let before: DocEnginePref = 'auto'
const options = computed(() => {
  const s = comp.status
  const list: { id: DocEnginePref; label: string; hint: string }[] = [{ id: 'auto', label: ENGINE_AUTO_LABEL, hint: ENGINE_AUTO_HINT }]
  for (const e of s.engines) {
    if (e.id === 'office' || e.id === 'wps') list.push({ id: e.id as DocEnginePref, label: e.name || (e.id === 'office' ? 'Microsoft Office' : 'WPS'), hint: e.version ? `版本 ${e.version}` : '' })
    else if (e.id === 'component')
      list.push({ id: 'component', label: e.installed ? e.name || '文档组件' : COMPONENT_NOT_DOWNLOADED, hint: componentOptionHint(e.installed, s.downloadBytes, e.version) })
  }
  return list
})
const currentLabel = computed(() => options.value.find((o) => o.id === pref.value)?.label ?? ENGINE_AUTO_LABEL)
async function choose(id: DocEnginePref) {
  ddOpen.value = false
  if (id === pref.value) return
  const comp0 = comp.status.engines.find((e) => e.id === 'component')
  before = pref.value
  pref.value = id
  if (id === 'component' && comp0 && !comp0.installed) {
    confirmDl.value = true
    await nextTick()
    dlBtn.value?.focus()
    return
  }
  try {
    await setDocEngine(id)
  } catch {
    pref.value = before
  }
}
function cancelDl() {
  confirmDl.value = false
  pref.value = before
}
async function confirmDownload() {
  confirmDl.value = false
  try {
    await setDocEngine('component')
  } catch {
    pref.value = before
    return
  }
  void comp.install()
}
function onDocDown(e: MouseEvent) {
  if (ddOpen.value && ddEl.value && !ddEl.value.contains(e.target as Node)) ddOpen.value = false
}
onMounted(async () => {
  document.addEventListener('mousedown', onDocDown)
  try {
    pref.value = await getDocEngine()
  } catch {
    /* 默认 auto */
  }
})
onBeforeUnmount(() => document.removeEventListener('mousedown', onDocDown))

// ── 打开组件所在文件夹（6.12.54）──
const showFolder = computed(() => comp.compState === 'ready' && comp.status.engines.some((e) => e.id === 'component' && e.source === 'downloaded'))
const folderErr = ref('')
async function openFolder() {
  folderErr.value = ''
  try {
    await openDocComponentFolder()
  } catch (e) {
    folderErr.value = toAppError(e).code === 'NOT_FOUND' ? DOC_COMPONENT_FOLDER_NOT_READY : '没能打开组件所在文件夹，请稍后再试。'
  }
}

const title = computed(() => {
  switch (shown.value) {
    case 'ready':
      return '文档组件已就绪'
    case 'checking':
      return '正在检测文档组件'
    case 'downloading':
      return '正在下载文档组件'
    case 'preparing':
      return DOC_PREPARING
    case 'failed':
      return '文档组件没有下载成功'
    default:
      return '文档组件未就绪'
  }
})
const detail = computed(() => {
  const s = comp.status
  switch (shown.value) {
    case 'ready': {
      // v0.27（设计 §十）：「正在使用本机 WPS / Microsoft Office / 文档组件」；用 Office / WPS 时不写版本
      const using = engineInUseText(s.source)
      if (s.source === 'office' || s.source === 'wps') return using
      const own = s.engines.find((e) => e.id === 'component')
      const from = SOURCE[own?.source ?? (['downloaded', 'system'].includes(s.source) ? s.source : '')] ?? ''
      const ver = own ? own.version : s.version
      const verLine = !ver ? from : from ? `文档组件版本 ${ver} · ${from}` : `文档组件版本 ${ver}`
      return [using, verLine].filter(Boolean).join(' · ')
    }
    case 'downloading':
      return `${comp.pct}% · ${formatBytes(comp.receivedBytes)} / ${formatBytes(comp.totalBytes)}`
    case 'preparing':
      return '大约需要一分钟'
    case 'failed':
      return comp.errorText
    case 'outdated':
      return comp.outdatedText
    case 'missing':
      return comp.isLinux ? comp.linuxMissingText : comp.sizeText
    default:
      return ''
  }
})
</script>

<style scoped>
.dsub { font-size: 12px; color: var(--ff-text-3); }
.fcomp { align-items: flex-start; }
.fcomp .l { min-width: 0; flex: 1; }
.fok { display: flex; align-items: center; gap: 8px; font-size: 13px; line-height: 20px; font-weight: 500; color: var(--ff-text-1); }
.fdot { width: 8px; height: 8px; border-radius: 50%; background: var(--ff-success); flex: none; }
.fdot.off { background: var(--ff-text-3); }
.fdot.bad { background: var(--ff-warning); }
.fcomp .l small { display: block; font-size: 12px; line-height: 18px; color: var(--ff-text-2); margin-top: 2px; }
.dbar { margin-top: 8px; max-width: 360px; }
.facts { display: flex; gap: 8px; flex: none; margin-top: -4px; }
.feng { align-items: center; }
.feng .l { flex: 1; min-width: 0; }
.feng .l small { display: block; font-size: 12px; line-height: 18px; color: var(--ff-text-2); margin-top: 2px; }
.fdd { position: relative; flex: none; }
.fdd-btn { width: 160px; height: 30px; display: flex; align-items: center; justify-content: space-between; gap: 6px; padding: 0 10px; border-radius: 6px; border: 1px solid var(--ff-border); background: var(--ff-bg-surface); color: var(--ff-text-1); font-size: 13px; cursor: pointer; }
.fdd-btn[aria-expanded='true'] { border-color: var(--ff-primary); box-shadow: 0 0 0 2px color-mix(in srgb, var(--ff-primary) 20%, transparent); }
.fdd-btn svg { color: var(--ff-text-3); }
.fdd-list { position: absolute; right: 0; top: calc(100% + 6px); z-index: 30; width: 224px; margin: 0; padding: 4px; list-style: none; border-radius: 8px; background: var(--ff-bg-elevated); border: 1px solid var(--ff-border); box-shadow: 0 8px 24px rgba(0, 0, 0, 0.16); }
.fdd-list li { display: flex; gap: 6px; align-items: flex-start; padding: 7px 8px; border-radius: 6px; cursor: pointer; outline: none; }
.fdd-list li:hover, .fdd-list li:focus-visible, .fdd-list li.on { background: var(--ff-bg-hover); }
.fdd-list .ck { width: 14px; height: 14px; flex: none; margin-top: 3px; color: var(--ff-primary-text); }
.fdd-list b { display: block; font-size: 13px; line-height: 20px; font-weight: 400; color: var(--ff-text-1); }
.fdd-list small { display: block; font-size: 12px; line-height: 16px; color: var(--ff-text-2); }
.ferr { color: var(--ff-danger-text); font-size: 12px; }
.fcf-mask { position: fixed; inset: 0; z-index: 2000; background: rgba(0, 0, 0, 0.45); display: grid; place-items: center; }
.fcf { width: 400px; max-width: calc(100vw - 48px); padding: 20px 20px 16px; border-radius: 10px; background: var(--ff-bg-elevated); border: 1px solid var(--ff-border); box-shadow: var(--ff-shadow-dialog); }
.fcf h4 { margin: 0 0 8px; font-size: 15px; font-weight: 600; color: var(--ff-text-1); }
.fcf p { margin: 0; font-size: 14px; line-height: 22px; color: var(--ff-text-1); }
.fcf .acts { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
</style>
