<template>
  <LivePanel title="推流会话" flat class="lv-list">
    <template #head><span class="cnt">{{ store.busyCount }}/{{ MAX_LIVE_SESSIONS }}</span></template>
    <div v-if="!store.rows.length" class="lv-empty">
      <div class="ic"><FIcon name="live" :size="24" /></div>
      <b>还没有推流</b>
      <span>{{ emptyHint }}</span>
    </div>
    <template v-else>
      <div class="lv-th"><span>推流地址 / 状态</span><span>用时</span><span>码率</span><span /></div>
      <div class="lv-rows">
        <div v-for="r in store.rows" :key="r.id" class="lv-r" :data-status="r.status">
          <div class="m">
            <div class="ad" :title="r.url"><FIcon :name="r.kind === 'screen' ? 'monitor' : 'film'" :size="14" /><span class="hd">{{ r.url }}</span></div>
            <div v-if="r.source" class="src" :title="r.source.title"><FIcon :name="r.source.kind === 'window' ? 'window' : 'monitor'" :size="12" /><span>{{ r.source.title }}</span></div>
            <div v-if="r.status === 'run'" class="lv-s run"><i class="dot" />运行中</div>
            <div v-else-if="r.status === 'stp'" class="lv-s stp" role="status" aria-live="polite"><i class="lv-spin" />{{ LIVE_STOPPING_TEXT }}</div>
            <div v-else-if="r.status === 'ok'" class="lv-s ok"><FIcon name="check" :size="14" />{{ LIVE_STOP_TEXT.succeeded }}</div>
            <div v-else-if="r.status === 'int'" class="lv-s int"><FIcon name="warn" :size="14" />推流中断</div>
            <div v-else-if="r.outputPath" class="lv-info"><FIcon name="info" :size="14" /><span>{{ LIVE_CANCELED_ARCHIVE_KEPT_TEXT }}</span></div>
            <div v-else class="lv-s cnl"><FIcon name="stop" :size="14" />{{ LIVE_STOP_TEXT.canceled }}</div>
            <LiveButton v-if="showFolder(r)" sm icon="folder" class="fold" :disabled="blocked" :tip-when-disabled="FFMPEG_TIP" @click="store.reveal(r.id)">打开所在文件夹</LiveButton>
          </div>
          <div class="nu">{{ elapsed(r) }}</div>
          <div class="nu">{{ r.archive ? '—' : r.bitrateKbps ? `${r.bitrateKbps} kbps` : '—' }}</div>
          <div class="ac">
            <template v-if="r.status === 'run'">
              <LiveButton sm :disabled="blocked" :tip-when-disabled="FFMPEG_TIP" @click="store.stop(r.id)">停止</LiveButton>
            </template>
            <template v-else-if="r.status === 'stp'">
              <LiveButton sm disabled>停止</LiveButton>
              <LiveButton sm variant="textdanger" :disabled="blocked" :tip-when-disabled="FFMPEG_TIP" @click="store.forceStop(r.id)">强制停止</LiveButton>
            </template>
            <LiveButton v-else variant="rm" :aria-label="`移除 ${r.url}`" @click="store.remove(r.id)">移除</LiveButton>
          </div>
        </div>
      </div>
    </template>
    <template #foot><div class="lv-limit">限制：支持 rtmp、rtmps、srt；同时最多 4 路，同一地址只允许 1 路</div></template>
  </LivePanel>
</template>

<script setup lang="ts">
// 推流会话面板（设计稿 v0.2 §2.2）：文件 / 录屏两个页签共用同一份全局会话（stores/liveSessions.ts）。
import { computed, onBeforeUnmount, ref } from 'vue'
import FIcon from '../icon/FIcon.vue'
import LivePanel from './LivePanel.vue'
import LiveButton from './LiveButton.vue'
import { LIVE_CANCELED_ARCHIVE_KEPT_TEXT, LIVE_STOPPING_TEXT, LIVE_STOP_TEXT } from '@/errors/errorMessages'
import { MAX_LIVE_SESSIONS, useLiveSessionsStore, type LiveRow } from '@/stores/liveSessions'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { formatClock } from '@/composables/useLiveSession'

defineProps<{ emptyHint: string }>()
const store = useLiveSessionsStore()
const ffmpeg = useFFmpegStore()
const FFMPEG_TIP = '需要先安装 ffmpeg'
const blocked = computed(() => ffmpeg.featuresBlocked)
const now = ref(Date.now())
const timer = setInterval(() => (now.value = Date.now()), 1000)
onBeforeUnmount(() => clearInterval(timer))

const elapsed = (r: LiveRow) => formatClock(((r.endedAt || now.value) - r.startedAt) / 1000)
/** 有存档且已结束（已结束推流 / 强杀且存档保留）才有 [打开所在文件夹]；放在状态文案下方 */
const showFolder = (r: LiveRow) => (r.status === 'ok' || r.status === 'cnl') && !!r.outputPath
</script>

<style scoped>
.lv-list {
  flex: 1;
  min-width: 0;
  overflow: hidden;
}
.cnt {
  margin-left: auto;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.lv-th,
.lv-r {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 72px 88px 140px;
  column-gap: 12px;
  align-items: start;
  padding: 0 16px;
}
.lv-th {
  height: 32px;
  align-items: center;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  border-bottom: 1px solid var(--ff-border);
  flex: none;
}
.lv-th span:nth-child(2),
.lv-th span:nth-child(3),
.lv-r .src {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: var(--ff-fs-xs);
  line-height: 18px;
  color: var(--ff-text-2);
}
.src svg {
  flex: none;
}
.src span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.nu {
  text-align: right;
}
.lv-rows {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
.lv-r {
  padding-top: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--ff-border);
}
.m {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ad {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  min-width: 0;
  font-family: var(--ff-font-mono);
  font-size: var(--ff-fs-xs);
  line-height: 20px;
  color: var(--ff-text-1);
}
.ad svg {
  margin-top: 3px;
  color: var(--ff-text-2);
}
.hd {
  min-width: 0;
  overflow-wrap: anywhere;
}
.nu {
  font-family: var(--ff-font-mono);
  font-size: var(--ff-fs-xs);
  line-height: 20px;
  color: var(--ff-text-1);
  white-space: nowrap;
}
.ac {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 4px;
  min-height: 24px;
  margin-top: -2px;
}
.lv-s {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--ff-fs-sm);
  font-weight: 500;
  line-height: 20px;
  white-space: nowrap;
}
.lv-s .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ff-primary);
  display: block;
  margin: 0 3px;
}
.lv-s.run { color: var(--ff-primary-text); }
.lv-s.stp { color: var(--ff-text-2); }
.lv-s.ok { color: var(--ff-success-text); }
.lv-s.cnl { color: var(--ff-text-2); }
.lv-s.int { color: var(--ff-interrupted); }
.lv-spin {
  width: 12px;
  height: 12px;
  margin: 0 1px;
  border-radius: 50%;
  border: 2px solid var(--ff-border);
  border-top-color: var(--ff-text-2);
  animation: lvspin 1s linear infinite;
  display: block;
  flex: none;
}
@keyframes lvspin {
  to { transform: rotate(360deg); }
}
@media (prefers-reduced-motion: reduce) {
  .lv-spin { animation: none; }
}
.lv-info {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
  color: var(--ff-text-2);
}
.lv-info svg {
  margin-top: 1px;
  flex: none;
}
.fold {
  align-self: flex-start;
  margin-top: 0;
}
.lv-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  text-align: center;
  padding: 24px;
}
.lv-empty .ic {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
  display: grid;
  place-items: center;
  margin-bottom: 8px;
}
.lv-empty b {
  font-size: var(--ff-fs-md);
  font-weight: 600;
}
.lv-empty span {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.lv-limit {
  flex: none;
  padding: 8px 16px 12px;
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
  color: var(--ff-text-2);
  border-top: 1px solid var(--ff-border);
}
</style>
