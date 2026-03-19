import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { ElMessage, ElNotification } from 'element-plus'

interface CaptureOptions {
  archiveEnabled?: boolean
  segmentSeconds?: number
  relayTargets?: string[]
}

export const useStreamStore = defineStore('stream', () => {
  const streamUrl = ref('')
  const isStreaming = ref(false)
  const statusMessage = ref('')
  const mediaStream = ref<MediaStream | null>(null)
  const mediaRecorder = ref<MediaRecorder | null>(null)
  const ws = ref<WebSocket | null>(null)

  function setStreamUrl(url: string) {
    streamUrl.value = url
  }

  async function startCapture(url: string, options: CaptureOptions = {}) {
    if (!url) {
      ElMessage.warning('请输入有效的 RTMP 推流地址')
      return
    }

    try {
      const stream = await navigator.mediaDevices.getDisplayMedia({
        video: { frameRate: 20, width: 1280, height: 720 },
        audio: true,
      })
      mediaStream.value = stream

      const socket = new WebSocket('ws://localhost:19200/ws')
      ws.value = socket

      socket.onopen = () => {
        // 推流启动元数据：主地址 + 自动归档参数 + 额外转推目标。
        socket.send(JSON.stringify({
          type: 'stream_url',
          url,
          archiveEnabled: !!options.archiveEnabled,
          segmentSeconds: options.segmentSeconds ?? 300,
          relayTargets: options.relayTargets ?? [],
        }))
        streamUrl.value = url

        const recorder = new MediaRecorder(stream, {
          mimeType: 'video/webm;codecs=vp8',
        })
        mediaRecorder.value = recorder

        recorder.ondataavailable = (event) => {
          if (event.data.size > 0 && socket.readyState === WebSocket.OPEN) {
            const reader = new FileReader()
            reader.readAsArrayBuffer(event.data)
            reader.onloadend = () => {
              const arrayBuffer = reader.result as ArrayBuffer
              socket.send(arrayBuffer)
            }
          }
        }

        recorder.start(80)
        isStreaming.value = true
        statusMessage.value = '正在推流中...'
      }

      socket.onclose = () => {
        stopCapture()
      }

      socket.onerror = (err) => {
        console.error('WebSocket error:', err)
        ElNotification.error({
          title: '错误',
          message: '与服务器连接异常，请检查后端服务是否运行',
        })
        stopCapture()
      }
    } catch (err: any) {
      console.error('屏幕捕获失败:', err)
      ElMessage.error(`屏幕捕获失败: ${err.message}`)
      stopCapture()
    }
  }

  function stopCapture() {
    if (mediaRecorder.value) {
      mediaRecorder.value.stop()
      mediaRecorder.value = null
    }

    if (mediaStream.value) {
      mediaStream.value.getTracks().forEach((track) => track.stop())
      mediaStream.value = null
    }

    if (ws.value) {
      ws.value.close()
      ws.value = null
    }

    isStreaming.value = false
    statusMessage.value = ''
  }

  const canStop = computed(() => isStreaming.value)

  return {
    streamUrl,
    isStreaming,
    statusMessage,
    mediaStream,
    mediaRecorder,
    ws,
    setStreamUrl,
    startCapture,
    stopCapture,
    canStop,
  }
})

