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
      <i v-if="badge" class="badge" aria-hidden="true">{{ badge }}</i>
    </button>
    <!-- X4：外层不能也叫 .pop——scoped 样式会同时落到面板根元素（也是 .pop）上，再往下错 8px，实际变成 16px -->
    <div v-if="dock.open" class="drop"><LiveSessionPanel :pull="pull" :count="count" /></div>
  </span>
</template>

<script setup lang="ts">
// 会话入口：标签行最右边的纯图标按钮，面板向下盖住设置栏（设计说明 §2.5，老板 10-08 第二次定稿）。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import LiveSessionPanel from './LiveSessionPanel.vue'
import { sessionBadge, useLiveDockStore } from '@/stores/liveDock'
import { useLiveSessionsStore } from '@/stores/liveSessions'
import { lpVisual } from '@/views/live/lpVisual'

const route = useRoute()
const dock = useLiveDockStore()
const store = useLiveSessionsStore()
const wrap = ref<HTMLElement | null>(null)
const pull = computed(() => route.path.startsWith('/live/pull'))
// 只数进行中的：推流 = 运行中 + 正在停止；拉流 = 连接中 / 播放中 / 缓冲中
const count = computed(() => (pull.value ? (dock.pull.active ? 1 : 0) : store.busyCount))
const badge = computed(() => sessionBadge(count.value))
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
  // 截图（?lpv=，没有后端）：和设计稿一样放一路脱敏地址；已结束 / 被中断的场景这一路是对应终态
  if (lpVisual && lpVisual.tab === 'push' && lpVisual.phase !== 'empty' && !store.rows.length) {
    const t = Date.now() - (12 * 60 + 36) * 1000
    const st = lpVisual.phase === 'ended' ? 'ok' : lpVisual.phase === 'interrupted' ? 'int' : 'run'
    store.rows.push({ id: 'shot1', kind: 'file', url: 'rtmp://push.example.com/live/****', status: st, archive: false, outputPath: '', startedAt: t, endedAt: st === 'run' ? 0 : Date.now(), bitrateKbps: 4820, preview: true })
  }
  if (lpVisual?.panel) dock.open = true
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
.drop { position: absolute; top: calc(100% + 8px); right: 0; z-index: 6; width: 320px; }
@media (max-width: 1199px) { .drop { width: 304px; } }
</style>
