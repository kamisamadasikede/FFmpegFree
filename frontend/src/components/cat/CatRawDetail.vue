<template>
  <div class="ct-od-box">
    <button type="button" class="ct-od-copy" :data-testid="testid + '-copy'" @click="copyRaw">{{ copied ? CAT_CLI_OUTDATED_COPY.copied : CAT_CLI_OUTDATED_COPY.copy }}</button>
    <pre class="ct-od-raw" :data-testid="testid + '-raw'">{{ text }}</pre>
  </div>
</template>

<script setup lang="ts">
/** 「查看详情」展开后的原始报错框（右上角「复制」，最大高 160px 超出滚动）。cli_outdated 与底栏切换分支失败共用。 */
import { ref } from 'vue'
import { hasWailsBackend } from '@/services/wails'
import { ClipboardSetText } from '../../../wailsjs/runtime/runtime'
import { CAT_CLI_OUTDATED_COPY } from './catCliOutdated'

const props = defineProps<{ text: string; testid: string }>()
const copied = ref(false)
async function copyRaw() {
  try {
    if (hasWailsBackend()) { if (!(await ClipboardSetText(props.text))) return }
    else await navigator.clipboard.writeText(props.text)
  } catch { return }
  copied.value = true
  setTimeout(() => { copied.value = false }, 1500)
}
</script>

<style scoped>
.ct-od-box { position: relative; margin-top: 6px; }
.ct-od-copy {
  position: absolute;
  top: 6px;
  right: 6px;
  z-index: 1;
  height: 22px;
  padding: 0 8px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-elevated);
  color: var(--cat-text-2);
  font-size: 11px;
  cursor: pointer;
}
.ct-od-copy:hover { background: var(--cat-hover); }
.ct-od-raw {
  margin: 0;
  max-height: 160px;
  overflow: auto;
  padding: 8px 64px 8px 10px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-elevated);
  color: var(--cat-text-2);
  font: 12px/1.6 var(--ff-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  white-space: pre-wrap;
  word-break: break-word;
  user-select: text;
  cursor: text;
}
</style>
