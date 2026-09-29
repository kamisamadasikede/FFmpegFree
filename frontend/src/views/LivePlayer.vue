<template>
  <div class="live-player">
    <div class="controls">
      <el-input
        v-model="videoUrl"
        placeholder="输入 FLV 拉流地址，例如 http://127.0.0.1:8080/live/test.flv"
        clearable
        @keyup.enter="initPlayer"
      />
      <el-button type="primary" @click="initPlayer">播放</el-button>
      <el-button @click="stopPlayer">停止</el-button>
    </div>
    <div class="video-stage">
      <video ref="videoRef" class="video" controls muted playsinline></video>
      <div v-if="errorText" class="error">{{ errorText }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onUnmounted } from 'vue'
import mpegts from 'mpegts.js'
import { useFlvPlayerStore } from '@/stores/useFlvPlayerStore'

defineOptions({ name: 'LivePlayer' })

const videoUrl = ref('http://localhost:8080/live/livestream.flv')
const videoRef = ref<HTMLVideoElement | null>(null)
const errorText = ref('')
const flvPlayerStore = useFlvPlayerStore()
let player: mpegts.Player | null = null

const stopPlayer = () => {
  if (player) {
    player.pause()
    player.unload()
    player.detachMediaElement()
    player.destroy()
    player = null
  }
}

const initPlayer = () => {
  stopPlayer()
  errorText.value = ''
  if (!videoUrl.value || !videoRef.value) return
  if (!mpegts.getFeatureList().mseLivePlayback) {
    errorText.value = '当前环境不支持 FLV 直播播放'
    return
  }
  flvPlayerStore.setFlvUrl(videoUrl.value)
  player = mpegts.createPlayer(
    { type: 'flv', isLive: true, url: videoUrl.value },
    {
      enableWorker: true,
      enableStashBuffer: false,
      stashInitialSize: 128,
      lazyLoad: true,
      lazyLoadMaxDuration: 3,
      autoCleanupSourceBuffer: true,
      liveBufferLatencyChasing: true,
      liveSync: true,
      liveSyncTargetLatency: 1,
    }
  )
  player.on(mpegts.Events.ERROR, (type: string, detail: string) => {
    errorText.value = `播放失败：${type} ${detail}`
  })
  player.attachMediaElement(videoRef.value)
  player.load()
  const p = player.play()
  if (p && typeof (p as Promise<void>).catch === 'function') (p as Promise<void>).catch(() => undefined)
}

onUnmounted(stopPlayer)
</script>

<style scoped>
.live-player {
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-3);
  height: 100%;
  min-height: 0;
}
.controls {
  display: flex;
  gap: var(--ff-space-2);
}
.video-stage {
  position: relative;
  flex: 1;
  min-height: 0;
  background: #000;
  border-radius: var(--ff-radius-lg);
  overflow: hidden;
}
.video {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.error {
  position: absolute;
  left: var(--ff-space-3);
  bottom: var(--ff-space-3);
  padding: var(--ff-space-1) var(--ff-space-2);
  border-radius: var(--ff-radius-sm);
  background: var(--ff-danger);
  color: #fff;
  font-size: 12px;
}
</style>
