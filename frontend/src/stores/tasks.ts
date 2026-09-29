import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { previewParams } from '@/services/wails'

// 任务 store 骨架：下一个 PR 按契约第 5 节接 ListActive 和 task:* 事件（先订阅、缓存、按 version 回放）
export const useTaskStore = defineStore('tasks', () => {
  const previewRunning = Number(previewParams.get('tasks') || 0)
  const runningCount = ref(previewRunning)
  const hasRunning = computed(() => runningCount.value > 0)
  return { runningCount, hasRunning }
})
