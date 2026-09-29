<template>
  <LiveHud v-if="session.running.value && hud" :label="hudLabel" :time="formatClock(session.uptimeSec.value)" :lines="hudLines" />
  <ErrorOverlay
    v-if="showOverlay"
    :code="session.errorCode.value"
    :detail="session.errorDetail.value"
    :message="session.errorMessage.value"
    @retry="emit('retry')"
    @view-log="emit('viewLog')"
  />
</template>

<script setup lang="ts">
// PlayerShell #overlay 插槽的内容：左上角“直播中 + 时长”、右上角实时参数、错误遮罩。
import { computed } from 'vue'
import ErrorOverlay from '../common/ErrorOverlay.vue'
import LiveHud from './LiveHud.vue'
import { formatClock, type LiveSession } from '@/composables/useLiveSession'

const props = withDefaults(defineProps<{ session: LiveSession; hudLabel?: string; hudLines?: string[]; hud?: boolean }>(), {
  hudLabel: '直播中',
  hudLines: () => [],
  hud: true,
})
const emit = defineEmits<{ retry: []; viewLog: [] }>()
// LIVE_URL_INVALID 是行内错误，不出遮罩
const showOverlay = computed(() => props.session.phase.value === 'error' && props.session.errorCode.value !== 'LIVE_URL_INVALID')
</script>
