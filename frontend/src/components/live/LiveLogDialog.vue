<template>
  <el-dialog v-model="open" title="查看日志" width="560px" append-to-body>
    <pre class="log selectable">{{ lines.length ? lines.join('\n') : '暂无日志' }}</pre>
    <p class="hint">目前只显示本页记录的事件；接入 Wails 后改为 TaskService.GetLog 返回的 ffmpeg 输出。</p>
    <template #footer>
      <el-button @click="copy">复制</el-button>
      <el-button type="primary" @click="open = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ElMessage } from 'element-plus'

const open = defineModel<boolean>({ default: false })
const props = defineProps<{ lines: string[] }>()

async function copy() {
  try {
    await navigator.clipboard.writeText(props.lines.join('\n'))
    ElMessage.success('已复制')
  } catch {
    ElMessage.warning('复制失败，请手动选中文本')
  }
}
</script>

<style scoped>
.log {
  margin: 0;
  max-height: 320px;
  overflow: auto;
  font-family: var(--ff-font-mono);
  font-size: 12px;
  line-height: 1.7;
  color: var(--ff-text-2);
  background: var(--ff-bg-app);
  border-radius: 6px;
  padding: 10px 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
.hint {
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--ff-text-2);
}
</style>
