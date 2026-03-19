<template>
  <el-main>
    <el-form label-position="top" :model="form" label-width="120px">
      <el-form-item label="推流地址 (RTMP)">
        <el-input v-model="form.streamUrl" placeholder="rtmp://live.example.com/live/streamkey" />
      </el-form-item>

      <el-form-item label="自动录制分段">
        <el-switch v-model="form.archiveEnabled" />
      </el-form-item>

      <el-form-item label="分段秒数">
        <el-input-number v-model="form.segmentSeconds" :min="30" :max="3600" :step="30" />
      </el-form-item>

      <el-form-item label="额外转推目标（每行一个）">
        <el-input
          v-model="form.relayTargetsText"
          type="textarea"
          :rows="4"
          placeholder="rtmp://backup.example.com/live/stream1&#10;rtmp://backup2.example.com/live/stream1"
        />
      </el-form-item>
    </el-form>

    <div style="margin-bottom: 20px;">
      <el-button type="primary" @click="startCapture" :disabled="!form.streamUrl || streamStore.isStreaming">
        开始推流
      </el-button>
      <el-button @click="stopCapture" :disabled="!streamStore.isStreaming">停止推流</el-button>
    </div>

    <div v-if="streamStore.statusMessage" style="color: green; margin-bottom: 10px;">
      {{ streamStore.statusMessage }}
    </div>

    <div class="preview-box">
      <video ref="videoRef" autoplay playsinline style="width: 100%; height: auto;"></video>
    </div>
  </el-main>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useStreamStore } from '@/stores/useStreamStore'

const streamStore = useStreamStore()

const form = ref({
  streamUrl: streamStore.streamUrl,
  archiveEnabled: false,
  segmentSeconds: 300,
  relayTargetsText: '',
})

watch(
  () => form.value.streamUrl,
  (newVal) => {
    streamStore.setStreamUrl(newVal)
  },
)

watch(
  () => streamStore.streamUrl,
  (newVal) => {
    form.value.streamUrl = newVal
  },
)

const videoRef = ref<HTMLVideoElement | null>(null)

onMounted(() => {
  if (streamStore.mediaStream && videoRef.value) {
    videoRef.value.srcObject = streamStore.mediaStream
  }
})

watch(
  () => streamStore.mediaStream,
  (newStream) => {
    if (videoRef.value) {
      videoRef.value.srcObject = newStream || null
    }
  },
)

function startCapture() {
  // 每行一个转推目标，和后端 relayTargets 对齐。
  const relayTargets = form.value.relayTargetsText
    .split('\n')
    .map((item) => item.trim())
    .filter((item) => item.length > 0)

  streamStore.startCapture(form.value.streamUrl, {
    archiveEnabled: form.value.archiveEnabled,
    segmentSeconds: form.value.segmentSeconds,
    relayTargets,
  })
}

function stopCapture() {
  streamStore.stopCapture()
}
</script>

<style scoped>
.preview-box {
  border: 1px solid #e4e4e4;
  padding: 10px;
  border-radius: 8px;
  background-color: #f9f9f9;
  max-width: 800px;
  margin-top: 10px;
}
</style>

