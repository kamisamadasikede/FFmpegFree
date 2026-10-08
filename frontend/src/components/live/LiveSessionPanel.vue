<template>
  <div class="pop" role="dialog" :aria-label="pull ? '拉流数据' : '推流会话'">
    <div class="ph"><h2>{{ pull ? '拉流数据' : '推流会话' }}</h2><span class="cnt">{{ pull ? count + ' 路' : count + '/' + MAX_LIVE_SESSIONS }}</span></div>
    <template v-if="pull">
      <div v-if="!count" class="none"><p>{{ LP_EMPTY_PULL }}</p></div>
      <div class="cards">
        <div v-for="c in cards" :key="c.label" class="mini"><small>{{ c.label }}</small><b>{{ c.value }}<em v-if="c.unit">{{ c.unit }}</em></b></div>
      </div>
    </template>
    <template v-else>
      <div v-if="!store.activeRows.length" class="none"><p>{{ LP_EMPTY_SESS }}</p><small>{{ LP_EMPTY_SESS_HINT }}</small></div>
      <div v-for="r in store.activeRows" :key="r.id" class="si">
        <div class="ad" :title="r.url"><FIcon :name="r.kind === 'screen' ? 'monitor' : 'film'" :size="14" /><span>{{ r.url }}</span></div>
        <div class="meta">
          <span class="st" :class="r.status"><i v-if="r.status === 'run'" /><span>{{ statusText(r) }}</span></span>
          <span>用时 <b>{{ elapsed(r) }}</b></span>
          <span>码率 <b>{{ r.archive || !r.bitrateKbps ? '—' : r.bitrateKbps + ' kbps' }}</b></span>
        </div>
        <div class="ops">
          <span>预览</span>
          <button type="button" class="sw" :class="{ on: r.preview !== false }" role="switch" :aria-checked="r.preview !== false" aria-label="这一路的预览" title="推流中也可以开关，不影响推流" @click="store.setPreview(r.id, r.preview === false)"></button>
          <span class="sp" />
          <button v-if="r.status === 'run'" type="button" class="btn" @click="store.stop(r.id)">停止</button>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
// 会话面板（设计说明 §2.5）：推流是每一路三行，只列进行中的（已结束 / 已中断的离开面板）；拉流是 2×2 数据卡。开始按钮不在这里。
// 每一行的预览开关只连接 / 断开这一路的播放器，不重启推流（契约 v0.25 ⑤）；是当前会话时和设置栏的开关同步（stores/liveDock.ts）。
import { computed, onBeforeUnmount, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { LP_EMPTY_PULL, LP_EMPTY_SESS, LP_EMPTY_SESS_HINT } from '@/errors/livePreviewMessages'
import { MAX_LIVE_SESSIONS, useLiveSessionsStore, type LiveRow } from '@/stores/liveSessions'
import { useLiveDockStore } from '@/stores/liveDock'
import { formatClock } from '@/composables/useLiveSession'

defineProps<{ pull: boolean; count: number }>()
const store = useLiveSessionsStore()
const dock = useLiveDockStore()
const now = ref(Date.now())
const timer = setInterval(() => (now.value = Date.now()), 1000)
onBeforeUnmount(() => clearInterval(timer))
const elapsed = (r: LiveRow) => formatClock(((r.endedAt || now.value) - r.startedAt) / 1000)
function statusText(r: LiveRow) {
  return r.status === 'stp' ? '正在停止…' : '运行中'
}
const cards = computed(() => {
  const p = dock.pull
  const dash = !p.active
  return [
    { label: '实时码率', value: dash ? '—' : p.bitrate, unit: dash ? '' : 'kbps' },
    { label: '帧率', value: dash ? '—' : p.fps, unit: '' },
    { label: '丢帧', value: dash ? '—' : p.dropped, unit: '' },
    { label: '已接收', value: dash ? '—' : p.bytes, unit: dash ? '' : p.unit },
  ]
})
</script>

<style scoped>
.pop { background: var(--ff-bg-surface); border: 1px solid var(--ff-border); border-radius: 10px; box-shadow: var(--ff-shadow-dialog); overflow: auto; max-height: calc(100vh - 140px); }
.ph { height: 48px; display: flex; align-items: center; gap: 8px; padding: 0 16px; border-bottom: 1px solid var(--ff-border); }
h2 { margin: 0; font-size: 14px; font-weight: 600; }
.cnt { font-size: 12px; color: var(--ff-text-2); }
.none { padding: 28px 16px; text-align: center; display: flex; flex-direction: column; gap: 4px; }
.none p { margin: 0; font-size: 13px; color: var(--ff-text-1); }
.none small { font-size: 12px; color: var(--ff-text-2); }
.si { padding: 12px 16px; display: flex; flex-direction: column; gap: 8px; border-bottom: 1px solid var(--ff-border); font-size: 12px; }
.si:last-child { border-bottom: 0; }
.ad { display: flex; align-items: center; gap: 6px; min-width: 0; font-family: var(--ff-font-mono); }
.ad span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.meta { display: flex; align-items: center; gap: 12px; color: var(--ff-text-2); white-space: nowrap; }
.meta b { font-weight: 400; font-family: var(--ff-font-mono); color: var(--ff-text-1); }
.st { display: flex; align-items: center; gap: 6px; }
.st i { width: 6px; height: 6px; border-radius: 50%; background: var(--ff-primary); }
.st.run { color: var(--ff-primary-text); }
.st.int { color: var(--ff-warning-text); }
.ops { display: flex; align-items: center; gap: 8px; color: var(--ff-text-2); }
.ops .sp { flex: 1; }
.sw { width: 28px; height: 16px; border-radius: 8px; border: 0; background: var(--ff-border); position: relative; padding: 0; }
.sw.on { background: var(--ff-primary); }
.sw::after { content: ''; position: absolute; top: 2px; left: 2px; width: 12px; height: 12px; border-radius: 50%; background: #fff; }
.sw.on::after { transform: translateX(12px); }
.sw { cursor: pointer; transition: background .15s; }
.sw::after { transition: transform .15s; }
.sw:focus-visible { outline: 2px solid var(--ff-primary); outline-offset: 2px; }
.btn { height: 28px; padding: 0 12px; border-radius: 6px; border: 1px solid var(--ff-border); background: var(--ff-bg-surface); color: var(--ff-text-1); font: inherit; font-size: 13px; cursor: pointer; }
.cards { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; padding: 12px 16px 16px; }
.mini { padding: 12px 14px; border: 1px solid var(--ff-border); border-radius: 10px; min-width: 0; }
.mini small { display: block; font-size: 12px; color: var(--ff-text-2); }
.mini b { display: block; font-size: 20px; font-weight: 600; font-family: var(--ff-font-mono); margin-top: 2px; white-space: nowrap; }
.mini em { font-style: normal; font-size: 12px; font-weight: 400; color: var(--ff-text-2); margin-left: 4px; }
@media (max-width: 1199px) {
  .mini { padding: 10px 12px; }
  .mini b { font-size: 16px; }
}
</style>
