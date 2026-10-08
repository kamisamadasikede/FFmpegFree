<template>
  <section class="panel group" :aria-labelledby="headingId">
    <div class="phead">
      <h2 :id="headingId">转换组件</h2>
      <span class="tag" :class="view.tone">{{ view.tag }}</span>
    </div>
    <div class="srow">
      <div class="l">
        <b>当前版本</b>
        <small>{{ view.detail }}</small>
      </div>
      <slot name="version-actions" />
    </div>
    <div class="srow">
      <div class="l">
        <b>路径</b>
        <small v-if="!readonly && ffmpeg.status.source === 'custom'">手动指定的位置，恢复后会重新自动查找。</small>
      </div>
      <div class="pathbox" :class="{ empty: !ffmpeg.status.path }" :title="ffmpeg.status.path || undefined">
        <span v-if="!ffmpeg.status.path" class="ph">{{ readonly ? '未设置' : '未找到' }}</span>
        <template v-else><span class="h">{{ pathParts.head }}</span><span class="t">{{ pathParts.tail }}</span></template>
      </div>
      <slot name="path-actions" />
    </div>
  </section>
</template>

<script setup lang="ts">
// ffmpeg 面板：状态标签 + 「当前版本」行 + 「路径」行。设置页与关于页共用。
// 关于页只读：不传 #version-actions / #path-actions 两个插槽即可（不显示重新检测、更换、下载源等操作），并传 readonly。
// 数据取自 ffmpeg store（与顶部提示条同源），状态变化（安装完成、手动指定）自动刷新。
import { computed } from 'vue'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { FFPROBE_MISSING_TEXT } from '@/errors/errorMessages'

defineProps<{ headingId?: string; readonly?: boolean }>()

const ffmpeg = useFFmpegStore()
const SOURCE: Record<string, string> = { bundled: '应用目录', system: '系统环境', custom: '手动指定', legacy: '旧版目录' }

const view = computed(() => {
  const s = ffmpeg.status
  const from = SOURCE[s.source ?? '']
  switch (s.state) {
    case 'ready':
      return { tag: '已就绪', tone: 'ok', detail: s.ffprobeMissing ? FFPROBE_MISSING_TEXT : [s.version && `转换组件版本 ${s.version}`, from && `来自${from}`].filter(Boolean).join(' · ') || '已检测到转换组件' }
    case 'installing':
      return { tag: '安装中', tone: 'run', detail: ffmpeg.install ? `下载中 ${Math.round(ffmpeg.install.progress * 100)}%` : '准备中' }
    case 'failed':
      return { tag: '安装失败', tone: 'fail', detail: s.error?.message || '安装没有成功，可以重试或手动指定位置。' }
    case 'outdated':
      return { tag: '版本过旧', tone: 'warn', detail: `${s.version ? `当前转换组件版本 ${s.version}，` : ''}需要 6.0 或更高版本。` }
    case 'missing':
      return { tag: '未安装', tone: 'warn', detail: '转换和直播暂不可用。' }
    default:
      return { tag: '检测中', tone: 'q', detail: '正在检测转换组件，请稍候。' }
  }
})

const pathParts = computed(() => {
  const p = ffmpeg.status.path ?? ''
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i < 0 ? { head: '', tail: p } : { head: p.slice(0, i + 1), tail: p.slice(i + 1) }
})
</script>
