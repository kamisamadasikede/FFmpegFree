<script setup lang="ts">
// 删除确认（设计 §四 9，截图 13 / 14）：不做回收站；“同时删除输出文件”默认不勾、不记住；默认焦点在“取消”。源文件本身永远不删。
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import type { DeleteAsk } from '@/stores/convertRecords'
import { formatBytes } from '@/utils/format'

const props = defineProps<{ ask: DeleteAsk | null; narrow: boolean; busy: boolean; /** 只给模拟场景（?dlg=delete-kid）用：打开时已勾选 */ checked?: boolean }>()
const emit = defineEmits<{ close: []; confirm: [deleteOutput: boolean] }>()

const withOutput = ref(false)
const cancelBtn = ref<HTMLButtonElement | null>(null)
const box = ref<HTMLElement | null>(null)
let returnTo: HTMLElement | null = null

watch(
  () => props.ask,
  async (a, old) => {
    if (a && !old) {
      returnTo = document.activeElement as HTMLElement | null
      withOutput.value = !!props.checked && a.outputs > 0
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
const sizeText = computed(() => (a.value && a.value.outputBytes > 0 ? formatBytes(a.value.outputBytes) : ''))
const optSmall = computed(() => {
  if (!a.value) return ''
  if (a.value.kind === 'record') return sizeText.value
  return `${a.value.outputs} 个文件${sizeText.value ? `，共 ${sizeText.value}` : ''}`
})
const activeLine = computed(() => (a.value && a.value.activeCount > 0 ? `其中 ${a.value.activeCount} 项正在转换，删除时会先取消它。` : ''))

function onKey(e: KeyboardEvent) {
  if (!props.ask) return
  if (e.key === 'Escape') {
    e.preventDefault()
    if (!props.busy) emit('close')
  } else if (e.key === 'Tab' && box.value) {
    // 焦点留在弹窗里
    const f = Array.from(box.value.querySelectorAll<HTMLElement>('button:not([disabled])'))
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
    <div v-if="a" class="cv2 cv-layer" :class="{ w1024: narrow }">
      <div class="cv-mask" @click.self="!busy && emit('close')">
        <div ref="box" class="cv-dlg" role="alertdialog" aria-modal="true" aria-labelledby="cv-del-t" aria-describedby="cv-del-d">
          <div class="big"><FIcon name="trash" /></div>
          <h3 id="cv-del-t">{{ a.title }}</h3>
          <p v-if="a.kind === 'record'" id="cv-del-d">
            <template v-if="withOutput">“{{ a.name }}”的记录会从列表里移除，<b>磁盘上的这个文件也会被删除</b>。</template>
            <template v-else>“{{ a.name }}”的记录会从列表里移除。只删除记录，<b>不删除磁盘上的文件</b>。</template>
          </p>
          <p v-else-if="a.count === 0" id="cv-del-d">只从列表里移除，<b>不删除磁盘上的文件</b>。</p>
          <p v-else id="cv-del-d">
            <template v-if="withOutput">记录和 {{ a.outputs }} 个输出文件都会被删除，<b>源文件不会被删除</b>。</template>
            <template v-else>只删除记录，<b>不删除磁盘上的文件</b>，源文件也不会被删除。</template>{{ activeLine }}
          </p>
          <button v-if="a.outputs > 0" type="button" class="cv-opt" :class="{ on: withOutput }" role="checkbox" :aria-checked="withOutput" @click="withOutput = !withOutput">
            <span class="cv-chk" :class="{ on: withOutput }"><FIcon v-if="withOutput" name="check" /></span>
            <div>同时删除输出文件<small v-if="optSmall">{{ optSmall }}</small><div v-if="withOutput" class="cv-irrev"><FIcon name="warn" />删除后无法恢复。</div></div>
          </button>
          <div class="dfoot" style="justify-content: flex-end">
            <button ref="cancelBtn" type="button" class="btn lg" :disabled="busy" @click="emit('close')">取消</button>
            <button type="button" class="btn lg danger" :disabled="busy" :aria-busy="busy" @click="emit('confirm', withOutput)">{{ a.kind === 'source' && a.count === 0 ? '移除' : '删除' }}</button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
