<script setup lang="ts">
/**
 * Cat 对话内办公画布下载卡（契约 6.19.16.1 / 细视觉 v0.1.1）。
 * 出现演示文稿能力且组件未 ready 时展示；「稍后」可再开。
 */
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { useOfficeCanvasStore } from '@/stores/officeCanvas'
import { formatBytes } from '@/utils/format'

const store = useOfficeCanvasStore()
const C = store.copy

const view = computed(() => {
  if (store.downloading) return 'downloading' as const
  if (store.failed) return 'failed' as const
  return 'need' as const
})

const failMsg = computed(() => {
  const msg = store.status.error?.message?.trim()
  if (msg && !/=/.test(msg) && !/LANG_|INVALID_|NOT_FOUND|INTERNAL/.test(msg)) return msg
  return C.failedUnconfigured
})

const sizeHint = computed(() => {
  const n = store.status.downloadBytes
  return n > 0 ? `约 ${formatBytes(n)}` : ''
})
</script>

<template>
  <div class="oc-wrap" data-testid="cat-office-artifact">
    <div class="tool-deck" data-testid="cat-office-tool-deck">
      <FIcon name="slide" :size="14" />
      <b>{{ C.toolLabel }}</b>
      <span class="sp" />
      <span class="meta">{{ store.ready ? C.toolPreview : C.toolPending }}</span>
    </div>

    <div
      v-if="store.showDownloadCard"
      class="dl-card"
      :class="{ fail: view === 'failed' }"
      role="region"
      aria-label="办公画布组件"
      data-testid="cat-office-download-card"
    >
      <div class="dh">
        <span class="ico"><FIcon name="slide" :size="19" /></span>
        <div>
          <b>{{ view === 'downloading' ? C.downloading : view === 'failed' ? '办公画布组件未就绪' : C.needTitle }}</b>
          <small v-if="view === 'need'">{{ C.needBody }}</small>
          <small v-else-if="view === 'downloading'">{{ C.needBody.replace('下载后即可使用。', '') }}</small>
          <div v-else class="err" role="alert" data-testid="cat-office-fail-msg">{{ failMsg }}</div>
        </div>
      </div>

      <div v-if="view === 'downloading'" class="prog-mini" data-testid="cat-office-progress">
        <div class="ph">
          <span>{{ sizeHint || '下载中' }}</span>
          <span>{{ store.pct }}%</span>
        </div>
        <div class="bar-track" role="progressbar" :aria-valuenow="store.pct" aria-valuemin="0" aria-valuemax="100">
          <div class="bar-fill" :style="{ width: store.pct + '%' }" />
        </div>
      </div>

      <div class="acts">
        <template v-if="view === 'need'">
          <button type="button" class="btn pri" :disabled="store.busy" data-testid="cat-office-download" @click="store.install()">
            <FIcon name="download" :size="14" />{{ C.download }}
          </button>
          <button type="button" class="btn" data-testid="cat-office-later" @click="store.dismissLater()">{{ C.later }}</button>
        </template>
        <template v-else-if="view === 'downloading'">
          <button type="button" class="btn" :disabled="store.busy" data-testid="cat-office-cancel" @click="store.cancel()">{{ C.cancel }}</button>
        </template>
        <template v-else>
          <button type="button" class="btn pri" :disabled="store.busy" data-testid="cat-office-retry" @click="store.install()">
            <FIcon name="retry" :size="14" />重试
          </button>
          <button type="button" class="btn" data-testid="cat-office-later" @click="store.dismissLater()">{{ C.later }}</button>
        </template>
      </div>
    </div>

    <div v-else-if="store.ready && store.slideCount" class="md-hint">
      <p>已在右侧打开画布，边生成边预览。完成后可导出 PPTX。</p>
    </div>
  </div>
</template>

<style scoped>
.oc-wrap {
  display: flex;
  flex-direction: column;
  gap: 0;
  margin: 4px 0 8px;
}
.tool-deck {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 12px;
  border: 1px solid var(--cat-divider);
  border-radius: var(--r-md, 10px);
  background: var(--cat-panel);
  margin: 8px 0;
  font-size: 12.5px;
  color: var(--cat-text-2);
  box-shadow: var(--cat-shadow-sm);
}
.tool-deck b {
  color: var(--cat-text);
  font-weight: 600;
}
.tool-deck .sp {
  flex: 1;
}
.tool-deck .meta {
  color: var(--cat-muted);
}
.dl-card {
  border: 1px solid var(--cat-border);
  border-radius: var(--r-lg, 12px);
  background: var(--cat-surface);
  padding: 16px;
  margin: 2px 0 10px;
  box-shadow: var(--cat-shadow-md);
}
.dl-card.fail {
  border-color: color-mix(in srgb, var(--cat-danger, #e11) 35%, var(--cat-border));
}
.dh {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 12px;
}
.dh .ico {
  width: 40px;
  height: 40px;
  border-radius: 11px;
  background: var(--cat-active);
  color: var(--cat-active-text);
  display: grid;
  place-items: center;
  flex: none;
  box-shadow: 0 1px 2px color-mix(in srgb, var(--cat-accent) 25%, transparent);
}
.dh b {
  display: block;
  font-size: 14px;
  font-weight: 650;
}
.dh small {
  display: block;
  font-size: 12.5px;
  color: var(--cat-muted);
  line-height: 1.5;
  margin-top: 3px;
}
.err {
  margin-top: 4px;
  font-size: 12.5px;
  color: var(--cat-danger-text, #c00);
  line-height: 1.5;
}
.acts {
  display: flex;
  gap: 8px;
  margin-top: 4px;
}
.btn {
  height: 32px;
  padding: 0 14px;
  border-radius: 8px;
  border: 1px solid var(--cat-border);
  background: var(--cat-elevated, var(--cat-surface));
  color: var(--cat-text);
  font-size: 13px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
}
.btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.btn.pri {
  background: var(--cat-accent);
  border-color: transparent;
  color: #fff;
  box-shadow: 0 1px 2px color-mix(in srgb, var(--cat-accent) 30%, transparent);
}
.prog-mini {
  margin: 0 0 10px;
}
.prog-mini .ph {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  margin-bottom: 7px;
  font-weight: 550;
}
.prog-mini .ph span {
  color: var(--cat-muted);
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}
.bar-track {
  height: 7px;
  border-radius: 999px;
  background: var(--cat-border);
  overflow: hidden;
}
.bar-fill {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, var(--cat-accent), color-mix(in srgb, var(--cat-accent) 70%, #7aa2ff));
}
.md-hint {
  font-size: 13px;
  color: var(--cat-text-2);
  line-height: 1.6;
  margin: 4px 0 8px;
}
.md-hint p {
  margin: 0;
}
</style>
