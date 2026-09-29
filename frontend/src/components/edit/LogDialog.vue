<template>
  <div class="ed-mask" @pointerdown.self="fl.logOpen.value = false">
    <div ref="box" class="ed-dlg" style="width: 560px" role="dialog" aria-modal="true" aria-labelledby="edl-t">
      <div class="ed-dh"><h3 id="edl-t">查看日志</h3></div>
      <pre class="ed-log" tabindex="0" data-autofocus>{{ fl.logText.value || '正在读取…' }}</pre>
      <div class="ed-dfoot">
        <button type="button" class="ed-btn lg" @click="copy">复制</button>
        <button type="button" class="ed-btn pri lg" @click="fl.logOpen.value = false">关闭</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useEditor } from './editor'
import { useExportFlow } from './exportFlow'
import { useDialog } from './useDialog'

const fl = useExportFlow()
const ed = useEditor()
const box = ref<HTMLElement | null>(null)
async function copy() {
  try {
    await navigator.clipboard.writeText(fl.logText.value)
    ed.say('已复制')
  } catch {
    ed.say('复制失败，请手动选中文本')
  }
}
useDialog(box, () => (fl.logOpen.value = false))
</script>
