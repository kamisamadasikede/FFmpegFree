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
        <div v-if="comp.inFlight && comp.status.state === 'downloading'" class="bar dbar" role="progressbar" aria-label="下载进度" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="comp.pct"><i :style="{ width: comp.pct + '%' }" /></div>
        <div v-else-if="comp.status.state === 'preparing'" class="bar ind dbar" role="progressbar" :aria-label="DOC_PREPARING" aria-busy="true"><i /></div>
      </div>
      <div class="facts">
        <template v-if="!comp.isLinux">
          <button v-if="canDownload" type="button" class="btn pri" :disabled="comp.busy" @click="comp.install()"><FIcon name="download" :size="15" />{{ failed ? '重试' : '下载' }}</button>
          <button v-if="comp.status.state === 'downloading'" type="button" class="btn" :disabled="comp.busy" @click="comp.cancel()">取消下载</button>
        </template>
        <button v-if="comp.status.state === 'checking'" type="button" class="btn" disabled>正在检测…</button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
// 设置页「文档组件」块（契约 6.12.12；设计 v0.2 §四）。和「转换组件」同一套行样式。
// 契约没有「打开文档组件所在文件夹」的方法（OpenStorageFolder 只有 component=转换组件），就绪时不放这个按钮。
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { useDocComponentStore } from '@/stores/docComponent'
import { DOC_PREPARING } from '@/utils/docV26Text'
import { formatBytes } from '@/utils/format'

defineProps<{ headingId?: string }>()
const comp = useDocComponentStore()
const SOURCE: Record<string, string> = { downloaded: '应用下载', system: '系统安装' }
const failed = computed(() => comp.status.state === 'failed')
const canDownload = computed(() => ['missing', 'outdated', 'failed'].includes(comp.status.state))
const dot = computed(() => {
  const s = comp.status.state
  if (s === 'ready') return ''
  if (comp.isLinux || s === 'failed' || s === 'outdated') return 'bad'
  return 'off'
})
const title = computed(() => {
  switch (comp.status.state) {
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
  switch (s.state) {
    case 'ready': {
      const from = SOURCE[s.source] ?? ''
      if (!s.version) return from
      return from ? `文档组件版本 ${s.version} · ${from}` : `文档组件版本 ${s.version}`
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
</style>
