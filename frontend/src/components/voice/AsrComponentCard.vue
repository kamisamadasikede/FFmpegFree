<script setup lang="ts">
// 语音识别组件横幅（契约 6.18.3 / 落地稿）。未发布：只展示定稿句，无下载按钮。
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import MotionCollapse from '@/components/motion/MotionCollapse.vue'
import { useLangAsrStore } from '@/stores/langAsr'
import { ASR_COMPONENT_NAME, ASR_NOT_PUBLISHED, ASR_NOT_PUBLISHED_TITLE } from '@/utils/langText'

const store = useLangAsrStore()
/** 未发布 / 未下载：显示横幅；已就绪不显示 */
const show = computed(() => store.unpublished || store.status.state === 'missing' || store.status.state === 'checking')
</script>

<template>
  <MotionCollapse>
    <div v-if="show" class="lg-guide" role="status" :aria-label="ASR_COMPONENT_NAME">
      <div class="gi"><FIcon name="info" /></div>
      <div class="gb">
        <h3>{{ ASR_NOT_PUBLISHED_TITLE }}</h3>
        <p>{{ ASR_NOT_PUBLISHED }}</p>
      </div>
    </div>
  </MotionCollapse>
</template>
