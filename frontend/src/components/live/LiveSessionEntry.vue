<template>
  <span ref="wrap" class="wrap">
    <button
      type="button"
      class="sess"
      :class="{ on: dock.open }"
      :aria-label="label"
      :aria-expanded="dock.open"
      aria-haspopup="dialog"
      :title="label"
      @click="dock.open = !dock.open"
    >
      <FIcon name="list" :size="18" />
      <i class="badge" aria-hidden="true">{{ count }}</i>
    </button>
    <div v-if="dock.open" class="pop"><LiveSessionPanel :pull="pull" :count="count" /></div>
  </span>
</template>

<script setup lang="ts">
// 会话入口：标签行最右边的纯图标按钮，面板向下盖住设置栏（设计说明 §2.5，老板 10-08 第二次定稿）。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import LiveSessionPanel from './LiveSessionPanel.vue'
import { useLiveDockStore } from '@/stores/liveDock'
import { useLiveSessionsStore } from '@/stores/liveSessions'
import { lpVisual } from '@/views/live/lpVisual'

const route = useRoute()
const dock = useLiveDockStore()
const store = useLiveSessionsStore()
const wrap = ref<HTMLElement | null>(null)
const pull = computed(() => route.path.startsWith('/live/pull'))
const count = computed(() => (pull.value ? (dock.pull.active ? 1 : 0) : store.rows.length))
const label = computed(() => (pull.value ? `拉流数据，当前 ${count.value} 路` : `推流会话，当前 ${count.value} 路`))

function onDoc(e: MouseEvent) {
  if (!dock.open) return
  const t = e.target as Node
  if (wrap.value?.contains(t)) return
  dock.open = false
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && dock.open) { dock.open = false; e.stopPropagation() }
}
onMounted(() => {
  if (lpVisual?.phase === 'interrupted' && lpVisual.tab === 'push' && !store.rows.length) {
    store.rows.push({ id: 'shot-int', kind: 'file', url: 'rtmp://live.example.com/live/****', status: 'int', archive: false, outputPath: '', startedAt: Date.now() - 60000, endedAt: Date.now(), bitrateKbps: 800, preview: true })
  }
  if (lpVisual?.panel) {
    dock.open = true
    // 截图：面板场景没有真实会话，放三路脱敏地址（只在 ?lpv= 且没有后端时）
    if (lpVisual.phase !== 'empty' && !store.rows.length) {
      const t = Date.now() - 12 * 60 * 1000
      store.rows.push(
        { id: 'shot1', kind: 'file', url: 'rtmp://live.example.com/live/****', status: 'run', archive: false, outputPath: '', startedAt: t, endedAt: 0, bitrateKbps: 2480, preview: true },
        { id: 'shot2', kind: 'screen', url: 'rtmp://push.example.com/app/****', status: 'run', archive: true, outputPath: '', startedAt: t, endedAt: 0, bitrateKbps: null, preview: true },
        { id: 'shot3', kind: 'file', url: 'srt://live.example.com:9000/?streamid=****', status: 'ok', archive: false, outputPath: '', startedAt: t - 3600000, endedAt: t, bitrateKbps: 1200, preview: false },
      )
    }
  }
  document.addEventListener('mousedown', onDoc)
  document.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDoc)
  document.removeEventListener('keydown', onKey)
})
</script>

<style scoped>
.wrap { position: relative; display: flex; flex: none; }
.sess { position: relative; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 50%; background: transparent; box-shadow: none; display: grid; place-items: center; color: var(--ff-text-2); cursor: pointer; }
.sess:hover, .sess.on { color: var(--ff-text-1); background: color-mix(in srgb, var(--ff-text-1) 6%, transparent); }
.sess:focus-visible { outline: 2px solid var(--ff-primary); outline-offset: 2px; }
.badge { position: absolute; top: -5px; right: -5px; min-width: 16px; height: 16px; padding: 0 4px; border-radius: 8px; background: var(--ff-badge-bg); color: var(--ff-on-primary); font-size: 12px; font-style: normal; font-weight: 600; line-height: 16px; text-align: center; box-shadow: 0 0 0 2px var(--ff-bg-app); }
.pop { position: absolute; top: calc(100% + 8px); right: 0; z-index: 6; width: 320px; }
@media (max-width: 1199px) { .pop { width: 304px; } }
</style>
