<template>
  <div class="ed-root" :class="{ 'ed-dragging': !!ed.sourceDrag.value }">
    <MigrationNotice v-if="!real" text="剪辑功能仍在开发中，当前页面为演示，尚未连接真实导出。" />

    <!-- 工程条 -->
    <div class="ed-pbar">
      <label class="ed-name">
        <input v-model="ed.project.name" type="text" maxlength="80" aria-label="工程名称" autocomplete="off" spellcheck="false" @input="ed.touch()" @blur="fixName" />
        <span v-if="[...ed.project.name].length >= 60" class="ed-cnt2" aria-hidden="true">{{ [...ed.project.name].length }}/80</span>
        <FIcon name="edit" :size="14" />
      </label>
      <span v-if="ed.dirty.value" class="ed-dirty" role="status"><i></i>有未保存的更改</span>
      <span class="sp"></span>
      <button type="button" class="ed-btn" :aria-disabled="ffOff ? 'true' : undefined" :data-tip="ffOff || undefined" @click="onOpenProject"><FIcon name="folder" :size="15" />打开工程</button>
      <button type="button" class="ed-btn" :aria-disabled="saveOff ? 'true' : undefined" :data-tip="saveOff || undefined" @click="onSave"><FIcon name="save" :size="15" />保存工程</button>
      <button id="ed-export-btn" type="button" class="ed-btn pri tp-r" :aria-disabled="exportOff ? 'true' : undefined" :data-tip="exportOff || undefined" @click="onExport"><FIcon name="download" :size="15" />导出视频</button>
    </div>

    <div class="ed-top">
      <MediaBin />
      <section v-if="!ed.sources.value.length" class="ed-panel ed-hero" aria-label="开始剪辑">
        <div class="ic"><FIcon name="cut" :size="28" /></div>
        <h3>导入素材，开始剪辑</h3>
        <p>把音视频文件拖进来，再拖到下面的时间线上。可以叠放多条视频轨做画中画。</p>
        <button type="button" class="ed-btn pri lg" :aria-disabled="ffOff ? 'true' : undefined" :data-tip="ffOff || undefined" @click="onImport"><FIcon name="upload" :size="15" />导入素材</button>
        <span class="fmt">支持 mp4、mov、avi、mkv、flv、webm、m4v、mp3、wav、aac、m4a、flac、ogg</span>
        <span class="lim">{{ LIMIT_TEXT }}</span>
      </section>
      <div v-else class="ed-preview-wrap">
        <PlayerShell
          v-model:playing="ed.playing.value"
          v-model:muted="ed.muted.value"
          v-model:current="ed.previewCurrent.value"
          :duration="ed.previewDuration.value"
          fill
          :fps="30"
          time-format="ms"
          :disabled="ed.previewError.value"
          :labels="{ play: '播放（空格）', pause: '停止播放（空格）', prev: '上一帧（←）', next: '下一帧（→）' }"
          @play="ed.play()"
          @pause="ed.stop()"
          @step="(d) => ed.stepFrame(d)"
          @seek="onSeek"
          @fullscreen="fullscreen"
        >
          <video v-if="real && ed.previewUrl.value && !ed.previewError.value" ref="videoEl" class="ed-video" :src="ed.previewUrl.value" playsinline :muted="ed.muted.value" preload="auto" @error="ed.onMediaError()" @loadedmetadata="syncVideo(true)" />
          <div v-else-if="!real && showScene" class="ed-frame ed-scene"><div class="sun"></div><div class="m1"></div><div class="m2"></div></div>
          <template #overlay>
            <span v-if="ed.previewMode.value === 'source' && ed.previewSource.value" class="ed-chip">素材预览 · {{ ed.previewSource.value.name }}</span>
            <div v-if="ed.previewError.value" class="ed-perr" role="alert">
              <div class="in">
                <div class="eic"><FIcon name="warn" :size="20" /></div>
                <h5>预览加载失败</h5>
                <p>这个片段暂时无法预览，不影响导出。</p>
                <div class="acts">
                  <button type="button" class="ed-btn pri sm" data-autofocus @click="ed.reloadPreview()"><FIcon name="refresh" :size="13" />重新加载</button>
                  <button type="button" class="ed-btn sm" @click="ed.revealPreviewFile()">在文件夹中显示</button>
                </div>
              </div>
            </div>
          </template>
        </PlayerShell>
      </div>
      <Inspector />
    </div>

    <ExportStrip />
    <Timeline />

    <ExportDialog v-if="fl.open.value" />
    <EditDialogs />
    <LogDialog v-if="fl.logOpen.value" />
    <OpenProjectDialog v-if="openList" @close="openList = false" @open="doOpen" />
    <div v-if="openErr" class="ed-toast" style="bottom: 56px" role="alert"><FIcon name="warn" :size="16" /><span>{{ openErr }}</span></div>
  </div>
</template>

<script setup lang="ts">
// 剪辑页（v2 全新实现，不复用 v1 的页面代码）：素材库 / 预览 / 属性区 / 多轨时间线 / 导出。
// 后端调用全部经 src/api（edit / media / system）和 stores/tasks；EDIT_BACKEND_READY=true 且运行在 Wails 里走真实 EditService，纯浏览器走模拟并显示“演示”提示条。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import MigrationNotice from '@/components/common/MigrationNotice.vue'
import PlayerShell from '@/components/common/PlayerShell.vue'
import { editIsReal } from '@/api/edit'
import { hasWailsBackend } from '@/services/wails'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { LIMIT_TEXT, TEXT, clipEnd, round6 } from '@/utils/editLogic'
import MediaBin from '@/components/edit/MediaBin.vue'
import Inspector from '@/components/edit/Inspector.vue'
import Timeline from '@/components/edit/Timeline.vue'
import ExportStrip from '@/components/edit/ExportStrip.vue'
import ExportDialog from '@/components/edit/ExportDialog.vue'
import EditDialogs from '@/components/edit/EditDialogs.vue'
import LogDialog from '@/components/edit/LogDialog.vue'
import OpenProjectDialog from '@/components/edit/OpenProjectDialog.vue'
import '@/components/edit/edit.css'
import { useEditor } from '@/components/edit/editor'
import { useExportFlow } from '@/components/edit/exportFlow'
import { applyPreviewState } from '@/components/edit/previewStates'

const ed = useEditor()
const fl = useExportFlow()
const ffmpeg = useFFmpegStore()
const real = editIsReal()
const openList = ref(false)
const openErr = ref('')
const videoEl = ref<HTMLVideoElement | null>(null)

const ffOff = computed(() => (ffmpeg.featuresBlocked ? TEXT.ffmpegTip : ''))
const saveOff = computed(() => ffOff.value || (ed.dirty.value || ed.allClips.value.length || ed.sources.value.length ? '' : TEXT.saveEmptyTip))
// 导出中禁用（产品定稿）：优先说明“正在导出”，其次没有片段
const exportOff = computed(() => ffOff.value || (fl.busy.value ? TEXT.exportBusyTip : ed.hasVideoClips.value ? '' : TEXT.exportEmptyTip))
const showScene = computed(() => ed.previewMode.value === 'source' || !!ed.clipAtPlayhead.value)

function fixName() {
  if (!ed.project.name.trim()) ed.project.name = '未命名工程'
}
function onImport() {
  if (ffOff.value) return ed.say(ffOff.value)
  ed.importFiles()
}
async function onSave() {
  if (saveOff.value) return ed.say(saveOff.value)
  fixName()
  if (await ed.save()) ed.say('工程已保存')
}
function onExport() {
  if (exportOff.value) return ed.say(exportOff.value)
  fixName()
  fl.openDialog()
}
function guardUnsaved(then: () => void) {
  if (!ed.dirty.value) return then()
  ed.dialog.value = { kind: 'unsaved', then }
}
function onOpenProject() {
  if (ffOff.value) return ed.say(ffOff.value)
  guardUnsaved(() => (openList.value = true))
}
async function doOpen(id: string) {
  openErr.value = ''
  const err = await ed.openProject(id)
  if (err) {
    openErr.value = err
    setTimeout(() => (openErr.value = ''), 4000)
    return
  }
  openList.value = false
}
function onSeek(t: number) {
  ed.stop()
  if (ed.previewMode.value === 'timeline') ed.seek(t)
}
function fullscreen() {
  const el = document.querySelector('.ed-preview-wrap .ff-player') as HTMLElement | null
  if (!el) return
  if (document.fullscreenElement) document.exitFullscreen()
  else el.requestFullscreen?.().catch(() => {})
}

// ───────── 预览：播放头下的片段 → <video> ─────────
function videoTime(): number {
  if (ed.previewMode.value === 'source') return ed.previewTime.value
  const c = ed.selected.value && ed.selected.value.path === ed.previewPath.value ? ed.selected.value : ed.clipAtPlayhead.value
  if (!c) return 0
  const speed = c.speed > 0 ? c.speed : 1
  return c.inSec + (ed.playhead.value - c.startSec) * speed
}
function syncVideo(force = false) {
  const v = videoEl.value
  if (!v) return
  const t = Math.max(0, videoTime())
  if (force || !ed.playing.value || Math.abs(v.currentTime - t) > 0.4) {
    try {
      v.currentTime = t
    } catch {
      /* 元数据未就绪时忽略 */
    }
  }
  const c = ed.clipAtPlayhead.value
  v.playbackRate = ed.previewMode.value === 'timeline' && c && c.speed > 0 ? Math.min(4, Math.max(0.25, c.speed)) : 1
  if (ed.playing.value && v.paused) v.play().catch(() => {})
  if (!ed.playing.value && !v.paused) v.pause()
}
watch(() => ed.previewPath.value, () => ed.loadPreview(), { immediate: true })
watch(() => [ed.playhead.value, ed.previewTime.value, ed.playing.value, ed.previewUrl.value], () => nextTick(() => syncVideo()))

// ───────── 键盘快捷键 ─────────
const mac = /mac/i.test(navigator.platform || '')
function inField(t: EventTarget | null) {
  const el = t as HTMLElement | null
  if (!el) return false
  return el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable
}
function onKey(e: KeyboardEvent) {
  if (document.querySelector('.ed-mask')) return
  const mod = mac ? e.metaKey : e.ctrlKey
  const k = e.key
  if (mod && !e.altKey) {
    const lower = k.toLowerCase()
    if (lower === 's') { e.preventDefault(); onSave(); return }
    if (lower === 'e') { e.preventDefault(); onExport(); return }
    if (lower === 'i') { e.preventDefault(); onImport(); return }
    return
  }
  if (e.metaKey || e.ctrlKey || e.altKey || inField(e.target)) return
  const t = e.target as HTMLElement
  const interactive = !!t.closest?.('button, a, [role="slider"], [role="menuitem"], [role="tab"], select, summary')
  if (k === ' ') {
    if (interactive) return
    e.preventDefault()
    ed.togglePlay()
  } else if (k === 'ArrowLeft' || k === 'ArrowRight') {
    if (t.closest?.('[role="slider"], select')) return
    e.preventDefault()
    ed.stop()
    ed.stepFrame(k === 'ArrowLeft' ? -1 : 1, e.shiftKey)
  } else if (k === 'Home') {
    if (interactive) return
    e.preventDefault(); ed.stop(); ed.previewCurrent.value = 0; ed.revealTime(0)
  } else if (k === 'End') {
    if (interactive) return
    e.preventDefault(); ed.stop(); ed.seek(ed.total.value); ed.revealTime(ed.total.value)
  } else if (k === 's' || k === 'S') {
    if (e.shiftKey || interactive && t.tagName !== 'BUTTON') return
    e.preventDefault(); if (!ffOff.value) ed.splitAtPlayhead(); else ed.say(ffOff.value)
  } else if (k === 'Delete' || k === 'Backspace') {
    if (t.closest?.('.ed-list, .ed-row')) return
    if (ed.selectedId.value) { e.preventDefault(); if (!ffOff.value) ed.deleteSelected() }
  } else if (k === 'n' || k === 'N') {
    ed.snapOn.value = !ed.snapOn.value
  } else if (k === '+' || k === '=') {
    ed.zoomBy(1.25)
  } else if (k === '-' || k === '_') {
    ed.zoomBy(0.8)
  } else if (k === 'Z' && e.shiftKey) {
    ed.fitWindow()
  } else if (k === 'Escape') {
    ed.selectedId.value = null
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKey)
  // 浏览器预览钩子：?edit=<状态>（仅没有 window.go 时读取；真实运行不读）
  if (!hasWailsBackend()) {
    const q = new URLSearchParams(window.location.search)
    const st = q.get('edit')
    if (st) applyPreviewState(st, q)
  }
  nextTick(() => {
    if (ed.allClips.value.length && ed.selectedId.value === null && !ed.dirty.value) ed.fitWindow()
  })
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  ed.stop()
})
void clipEnd
void round6
</script>

<style scoped>
.ed-video {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #000;
}
</style>
