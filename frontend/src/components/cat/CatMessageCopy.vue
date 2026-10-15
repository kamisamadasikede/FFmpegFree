<template>
  <button type="button" class="message-copy" :disabled="busy" :title="label" :aria-label="label" :data-testid="testid" @click="copy">
    <FIcon :name="copied ? 'check' : 'copy'" :size="13" /><span>{{ label }}</span>
  </button>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { hasWailsBackend } from '@/services/wails'
import { ClipboardSetText } from '../../../wailsjs/runtime/runtime'

const props = defineProps<{ text: string; testid?: string }>()
const { t } = useI18n()
const copied = ref(false)
const busy = ref(false)
const label = computed(() => t(copied.value ? 'cat.chatUX.copied' : 'cat.chatUX.copy'))
let timer: ReturnType<typeof setTimeout> | undefined
let alive = true
async function copy() {
  if (busy.value) return
  busy.value = true
  try {
    if (hasWailsBackend()) {
      if (!await ClipboardSetText(props.text)) throw new Error('clipboard unavailable')
    } else await navigator.clipboard.writeText(props.text)
    if (!alive) return
    copied.value = true
    clearTimeout(timer)
    timer = setTimeout(() => { copied.value = false }, 1600)
  } catch {
    if (alive) ElMessage.warning(t('cat.chatUX.copyFailed'))
  } finally { busy.value = false }
}
onBeforeUnmount(() => { alive = false; clearTimeout(timer) })
</script>

<style scoped>
.message-copy { display: inline-flex; align-items: center; gap: 5px; min-height: 26px; padding: 3px 7px; border: 0; border-radius: var(--cat-radius-sm); background: transparent; color: var(--cat-muted); font: inherit; font-size: 11px; cursor: pointer; }
.message-copy:hover { color: var(--cat-text); background: var(--cat-hover); }
.message-copy:focus-visible { outline: none; box-shadow: var(--cat-focus); }
.message-copy:disabled { opacity: .6; }
</style>
