<template>
    <el-main>

      <!-- 推流地址输入 -->
      <el-form label-position="top" :model="form" label-width="120px">
        <el-form-item label="推流地址 (RTMP)">
          <el-input v-model="form.streamUrl" placeholder="rtmp://live.example.com/live/streamkey" />
        </el-form-item>
      </el-form>

      <!-- 控制按钮 -->
      <div style="margin-bottom: 20px;">
        <el-button type="primary" @click="startCapture" :disabled="!form.streamUrl || streamStore.isStreaming">
          开始推流
        </el-button>
        <el-button @click="stopCapture" :disabled="!streamStore.isStreaming">停止推流</el-button>
      </div>

      <!-- 状态提示 -->
      <div v-if="streamStore.statusMessage" style="color: green; margin-bottom: 10px;">
        {{ streamStore.statusMessage }}
      </div>

      <!-- 视频预览 -->
      <div class="preview-box">
        <video ref="videoRef" autoplay playsinline style="width: 100%; height: auto;"></video>
      </div>
    </el-main> nhj
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useStreamStore } from '@/stores/useStreamStore'

const streamStore = useStreamStore()

// 表单数据初始化为 Store 中的值
const form = ref({
  streamUrl: streamStore.streamUrl,
})

// 双向绑定表单与 Store
watch(
    () => form.value.streamUrl,
    (newVal) => {
      streamStore.setStreamUrl(newVal)
    }
)

watch(
    () => streamStore.streamUrl,
    (newVal) => {
      form.value.streamUrl = newVal
    }
)

const videoRef = ref<HTMLVideoElement | null>(null)

// 页面加载时恢复视频预览
onMounted(() => {
  if (streamStore.mediaStream && videoRef.value) {
    videoRef.value.srcObject = streamStore.mediaStream
  }
})

// 实时监听 mediaStream 变化
watch(
    () => streamStore.mediaStream,
    (newStream) => {
      if (videoRef.value) {
        videoRef.value.srcObject = newStream || null
      }
    }
)

function startCapture() {
  streamStore.startCapture(form.value.streamUrl)
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