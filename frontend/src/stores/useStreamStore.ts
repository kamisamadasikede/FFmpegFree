// stores/useStreamStore.ts

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { ElMessage, ElNotification } from 'element-plus'

export const useStreamStore = defineStore('stream', () => {
    // 状态
    const streamUrl = ref('')
    const isStreaming = ref(false)
    const statusMessage = ref('')
    const mediaStream = ref<MediaStream | null>(null)
    const mediaRecorder = ref<MediaRecorder | null>(null)
    const ws = ref<WebSocket | null>(null)

    // 设置推流地址（用于表单双向绑定）
    function setStreamUrl(url: string) {
        streamUrl.value = url
    }

    // 获取屏幕流并开始推流
    async function startCapture(url: string) {
        if (!url) {
            ElMessage.warning('请输入有效的 RTMP 推流地址')
            return
        }

        try {
            // 获取屏幕流，设置分辨率和帧率
            const stream = await navigator.mediaDevices.getDisplayMedia({
                video: { frameRate: 20, width: 1280, height: 720 },
                audio: true
            })
            mediaStream.value = stream

            // 创建 WebSocket 连接
            const socket = new WebSocket('ws://localhost:19200/ws')
            ws.value = socket

            socket.onopen = () => {
                // 发送推流地址给后端
                socket.send(JSON.stringify({ type: 'stream_url', url }))
                streamUrl.value = url

                // 初始化 MediaRecorder
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

                recorder.start(80) // 80ms一包，低延迟
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
                    message: '与服务器的连接异常，请检查服务是否运行。',
                })
                stopCapture()
            }
        } catch (err: any) {
            console.error('屏幕捕获失败:', err)
            ElMessage.error(`屏幕捕获失败：${err.message}`)
            stopCapture()
        }
    }

    // 停止推流并清理资源
    function stopCapture() {
        if (mediaRecorder.value) {
            mediaRecorder.value.stop()
            mediaRecorder.value = null
        }

        if (mediaStream.value) {
            mediaStream.value.getTracks().forEach(track => track.stop())
            mediaStream.value = null
        }

        if (ws.value) {
            ws.value.close()
            ws.value = null
        }

        isStreaming.value = false
        statusMessage.value = ''
    }

    // 是否可以停止推流
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