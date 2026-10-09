<script setup lang="ts">
// 重转确认框（设计说明 §八 第 58、59、64 条；截图 29 / 31）。样式同删除确认框，图标圈中性色 + 重试图标，默认焦点在“取消”。
// replace：带“参数”下拉（同一格式的预设，默认“沿用原来的参数（…）”）；regenerate（v0.24.1，原来的文件已经不在了）：没有下拉，不传参数。
import MidEllipsis from '@/components/common/MidEllipsis.vue'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import MotionDialog from '@/components/motion/MotionDialog.vue'
import type { ReconvertAsk } from '@/stores/convertRecords'
import { isAudioContainer } from '@/utils/convertText'
import { RECONVERT_FORMAT_NOTE, RECONVERT_TITLE, reconvertBody, reconvertKeepLabel } from '@/utils/convertV24Text'

const props = defineProps<{ ask: ReconvertAsk | null; narrow: boolean; busy: boolean }>()
const emit = defineEmits<{ close: []; confirm: [presetId: string] }>()

const presetId = ref('')
const cancelBtn = ref<HTMLButtonElement | null>(null)
const box = ref<HTMLElement | null>(null)
let returnTo: HTMLElement | null = null
watch(
  () => props.ask,
  async (a, old) => {
    if (a && !old) {
      returnTo = document.activeElement as HTMLElement | null
      presetId.value = ''
      await nextTick()
      cancelBtn.value?.focus()
    } else if (!a && old) {
      const el = returnTo
      returnTo = null
      if (el?.isConnected) el.focus()
    }
  },
)
const a = computed(() => props.ask)
const audio = computed(() => isAudioContainer(a.value?.container ?? ''))
const chosen = computed(() => a.value?.presets.find((p) => p.id === presetId.value))
const selText = computed(() => (chosen.value ? chosen.value.title : reconvertKeepLabel(a.value?.current ?? '')))
const selTip = computed(() => (chosen.value ? chosen.value.tip : a.value?.currentTip ?? ''))

function onKey(e: KeyboardEvent) {
  if (!props.ask) return
  if (e.key === 'Escape') {
    e.preventDefault()
    if (!props.busy) emit('close')
  } else if (e.key === 'Tab' && box.value) {
    const f = Array.from(box.value.querySelectorAll<HTMLElement>('button:not([disabled]), select:not([disabled])'))
    if (!f.length) return
    const i = f.indexOf(document.activeElement as HTMLElement)
    if (e.shiftKey && i <= 0) {
      e.preventDefault()
      f[f.length - 1].focus()
    } else if (!e.shiftKey && i === f.length - 1) {
      e.preventDefault()
      f[0].focus()
    }
  }
}
watch(() => !!props.ask, (o) => (o ? window.addEventListener('keydown', onKey, true) : window.removeEventListener('keydown', onKey, true)))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey, true))
</script>
<template>
  <Teleport to="body">
    <MotionDialog>
    <div v-if="a" class="cv2 cv-layer" :class="{ w1024: narrow }">
      <div class="cv-mask" @click.self="!busy && emit('close')">
        <div ref="box" class="cv-dlg ff-panel" role="alertdialog" aria-modal="true" aria-labelledby="cv-rc-t" aria-describedby="cv-rc-d">
          <div class="big neutral"><FIcon name="retry" /></div>
          <h3 id="cv-rc-t">{{ RECONVERT_TITLE }}</h3>
          <div class="cv-delfile"><FIcon :name="audio ? 'music' : 'film'" :size="14" /><MidEllipsis :text="a.name" /><template v-if="a.line"><i>·</i><em>{{ a.line }}</em></template></div>
          <p id="cv-rc-d">{{ reconvertBody(a.mode, a.name) }}</p>
          <div v-if="a.mode === 'replace'" class="cv-rcpar">
            <label for="cv-rc-sel">参数</label>
            <div class="select" :title="selTip">
              <span>{{ selText }}</span><FIcon name="down" :size="14" />
              <select id="cv-rc-sel" v-model="presetId" :disabled="busy">
                <option value="" :title="a.currentTip">{{ reconvertKeepLabel(a.current) }}</option>
                <option v-for="p in a.presets" :key="p.id" :value="p.id" :title="p.tip">{{ p.title }}</option>
              </select>
            </div>
            <small>{{ RECONVERT_FORMAT_NOTE }}</small>
          </div>
          <div class="dfoot" style="justify-content: flex-end">
            <button ref="cancelBtn" type="button" class="btn lg" :disabled="busy" @click="emit('close')">取消</button>
            <button type="button" class="btn lg pri" :disabled="busy" :aria-busy="busy" @click="emit('confirm', a.mode === 'replace' ? presetId : '')">重转</button>
          </div>
        </div>
      </div>
    </div>
    </MotionDialog>
  </Teleport>
</template>
