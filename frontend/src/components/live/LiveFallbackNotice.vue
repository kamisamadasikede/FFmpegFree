<template>
  <EncoderFallbackNotice v-if="shown" variant="live" class="lv-fb" @settings="router.push('/settings/general')" />
</template>

<script setup lang="ts">
// 直播转码回退提示条（设计稿 §2.4 的 live 变体）：任务 store 里有 startedAt>0 且 hwFallback 的直播任务才显示（startedAt 为 0 / 缺失不显示）；
// 文案只说“显卡编码启动失败，已自动改用 CPU 推流”，不出现编码器名。放在推流页左列最上面（预览面板上方），关闭只影响本次会话。
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import EncoderFallbackNotice from '@/components/encoder/EncoderFallbackNotice.vue'
import { liveFallbackShown } from '@/api/encoderTask'
import { useTaskStore } from '@/stores/tasks'

const router = useRouter()
const tasks = useTaskStore()
const shown = computed(() => liveFallbackShown(tasks.active))
</script>

<style scoped>
.lv-fb {
  flex: none;
}
</style>
