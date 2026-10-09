<script setup lang="ts">
// 语音识别组件横幅（契约 6.18.3 / 落地稿 01）。
// canDownload=false（未发布）：只展示定稿句，绝不出现「下载」「安装」按钮。
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import MotionCollapse from '@/components/motion/MotionCollapse.vue'
import { useLangAsrStore } from '@/stores/langAsr'
import { ASR_COMPONENT_NAME, ASR_NOT_PUBLISHED, ASR_NOT_PUBLISHED_TITLE } from '@/utils/langText'

const store = useLangAsrStore()
/** 未发布 / 检测中 / 未就绪：显示横幅；已就绪不显示。有 canDownload 时也不在本卡放下载（设置页另议），一期 URL 未发布。 */
const show = computed(
  () =>
    store.unpublished ||
    store.status.state === 'missing' ||
    store.status.state === 'checking' ||
    store.status.state === 'outdated' ||
    store.status.state === 'failed',
)
const title = computed(() => {
  if (store.unpublished || (store.status.state === 'missing' && !store.canDownload)) return ASR_NOT_PUBLISHED_TITLE
  if (store.status.state === 'checking') return '正在检查语音识别组件…'
  if (store.status.state === 'outdated') return '语音识别组件需要更新'
  if (store.status.state === 'failed') return '语音识别组件未就绪'
  return ASR_NOT_PUBLISHED_TITLE
})
const body = computed(() => {
  if (store.unpublished || (store.status.state === 'missing' && !store.canDownload)) return ASR_NOT_PUBLISHED
  if (store.status.state === 'checking') return '请稍候。'
  // 不把错误码 / reason= 亮给用户；有后端 message 则用，否则定稿句
  const msg = store.status.error?.message?.trim()
  if (msg && !/=/.test(msg) && !/LANG_|INVALID_|NOT_FOUND|INTERNAL/.test(msg)) return msg
  return ASR_NOT_PUBLISHED
})
</script>

<template>
  <MotionCollapse>
    <div v-if="show" class="lg-guide" role="status" :aria-label="ASR_COMPONENT_NAME">
      <div class="gi"><FIcon name="info" /></div>
      <div class="gb">
        <h3>{{ title }}</h3>
        <p>{{ body }}</p>
        <!-- 一期：canDownload=false 时不渲染任何下载/安装按钮（落地稿 01） -->
      </div>
    </div>
  </MotionCollapse>
</template>
