<template>
  <section class="pvp" aria-label="推流预览">
    <div class="phead">
      <h2>{{ PREVIEW_PANEL_TITLE }}</h2>
      <span v-if="cur" class="src" :title="cur.url"><FIcon :name="cur.kind === 'screen' ? 'monitor' : 'film'" :size="14" /><span>{{ cur.url }}</span></span>
    </div>
    <LivePlayer
      v-model:muted="muted"
      kind="push"
      :phase="phase"
      :url="playUrl"
      :mime="mime"
      :has-audio="hasAudio"
      :clock="clock"
      :aspect="16 / 9"
      :fake="fake"
      :force-idle="!!vis?.idle"
      :force-vol="!!vis?.vol"
      :force-hint="!!vis?.hint"
      :force-full="!!vis?.full"
      :reason="reason"
      :empty-text="cur && cur.preview === false && !vis ? PREVIEW_OFF_TITLE : undefined"
      @restart="onRestart"
      @media-unsupported="fail = 'codec'"
      @media-broken="broken = true"
      @media-ended="ended = true"
      @playing="connected = true"
    />
  </section>
</template>

<script setup lang="ts">
// 推流预览：铺满左栏。地址来自 GetPreviewStream。开关只连接 / 断开这里，不重启推流（契约 6.10.3.2a）。
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import LivePlayer from './LivePlayer.vue'
import { PREVIEW_OFF_TITLE, PREVIEW_PANEL_TITLE } from '@/errors/livePreviewMessages'
import { useLiveSessionsStore, type LiveRow } from '@/stores/liveSessions'
import { useLiveDockStore } from '@/stores/liveDock'
import { classifyPreviewError, getPreviewStream } from '@/api/livePreviewStream'
import { toAppError } from '@/api/call'
import { lpVisual } from '@/views/live/lpVisual'
import { formatClock } from '@/composables/useLiveSession'

const store = useLiveSessionsStore()
const dock = useLiveDockStore()
const vis = lpVisual && lpVisual.kind === 'push' ? lpVisual : null
const muted = ref(true)
const playUrl = ref('')
const mime = ref('video/x-flv')
const hasAudio = ref(true)
const connected = ref(false)
const broken = ref(false)
const ended = ref(false)
const fail = ref<'' | 'codec' | 'unavailable'>('')
const now = ref(Date.now())
setInterval(() => (now.value = Date.now()), 1000)

const cur = computed<LiveRow | undefined>(() => store.current)
const clock = computed(() => {
  if (vis) return vis.clock
  const r = cur.value
  if (!r) return '00:00:00'
  return formatClock(((r.endedAt || now.value) - r.startedAt) / 1000)
})
const fake = computed(() => vis?.fake ?? '')
const reason = computed(() => vis?.reason || fail.value)

const phase = computed(() => {
  if (vis) return vis.phase
  const r = cur.value
  if (!r) return 'empty' as const
  if (r.status === 'int' || broken.value) return 'interrupted' as const
  if (r.status === 'ok' || r.status === 'cnl' || ended.value) return 'ended' as const
  if (r.preview === false) return 'empty' as const
  if (fail.value) return 'unsupported' as const
  if (!connected.value) return 'connecting' as const
  return 'playing' as const
})

let seq = 0
watch(
  () => [cur.value?.id, cur.value?.status, cur.value?.preview !== false] as const,
  async ([id, status, on]) => {
    const mine = ++seq
    playUrl.value = ''
    connected.value = false
    broken.value = false
    ended.value = false
    fail.value = ''
    if (vis || !id || !on || (status !== 'run' && status !== 'stp')) return
    try {
      const s = await getPreviewStream(id)
      if (mine !== seq) return
      mime.value = s.mime
      hasAudio.value = s.hasAudio
      playUrl.value = s.url
    } catch (e) {
      if (mine !== seq) return
      const k = classifyPreviewError(e)
      fail.value = k === 'unsupported' ? 'codec' : 'unavailable'
    }
  },
  { immediate: true },
)

async function onRestart() {
  const r = cur.value
  if (!r) return
  try {
    const res = await store.restart(r.id, dock.previewOn)
    if (res && !res.ok) ElMessage.error(res.error.message)
    else if (!res) ElMessage.info('请在右侧再点一次「重新推流」')
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  }
}
</script>

<style scoped>
.pvp { flex: 1; min-height: 0; width: 100%; background: var(--ff-bg-surface); border: 1px solid var(--ff-border); border-radius: 10px; overflow: hidden; display: flex; flex-direction: column; }
.phead { height: 48px; flex: none; display: flex; align-items: center; gap: 8px; padding: 0 16px; border-bottom: 1px solid var(--ff-border); min-width: 0; }
h2 { margin: 0; font-size: 14px; font-weight: 600; flex: none; }
.src { display: flex; align-items: center; gap: 6px; min-width: 0; font-family: var(--ff-font-mono); font-size: 12px; color: var(--ff-text-2); }
.src span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
