<template>
  <el-dialog v-model="visible" fullscreen :close-on-click-modal="false">
    <div class="preview-shell">
      <video
        ref="videoRef"
        class="fullscreen-video"
        autoplay
        controls
        playsinline
      ></video>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import flvjs from 'flv.js'

const props = defineProps<{
  modelValue: boolean
  url: string
  name: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value)
})

const videoRef = ref<HTMLVideoElement | null>(null)
let flvPlayer: ReturnType<typeof flvjs.createPlayer> | null = null

const isFlv = computed(() => props.name.toLowerCase().endsWith('.flv'))

const destroyPlayer = () => {
  if (flvPlayer) {
    flvPlayer.destroy()
    flvPlayer = null
  }
  if (videoRef.value) {
    videoRef.value.pause()
    videoRef.value.removeAttribute('src')
    videoRef.value.load()
  }
}

const initPlayer = () => {
  if (!videoRef.value || !props.url) return

  if (isFlv.value && flvjs.isSupported()) {
    destroyPlayer()
    flvPlayer = flvjs.createPlayer(
      { type: 'flv', url: props.url },
      { enableWorker: true, isLive: false }
    )
    flvPlayer.attachMediaElement(videoRef.value)
    flvPlayer.load()
    flvPlayer.play()
    return
  }

  destroyPlayer()
  videoRef.value.src = props.url
  videoRef.value.play().catch(() => undefined)
}

watch(
  () => [visible.value, props.url, props.name],
  ([open]) => {
    if (open) {
      initPlayer()
    } else {
      destroyPlayer()
    }
  }
)

onUnmounted(() => {
  destroyPlayer()
})
</script>

<style scoped>
.preview-shell {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #000;
}

.fullscreen-video {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
</style>
