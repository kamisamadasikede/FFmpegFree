<script setup lang="ts">
// docx 应用内编辑（EigenPal @docx-editor.dev/vue 2.27.0，Apache-2.0；进入编辑时才按需加载，不进首屏包）。
// 只负责显示和拿到新字节；保存流程（Begin / Append / Commit / Abort）在弹窗里。
import { defineAsyncComponent, onMounted, ref, shallowRef, type Component } from 'vue'
import { useThemeDark } from './useThemeDark'

const props = defineProps<{ bytes: Uint8Array; title?: string }>()
const emit = defineEmits<{ (e: 'dirty'): void; (e: 'failed'): void; (e: 'ready'): void }>()
const dark = useThemeDark()
const i18n = shallowRef<unknown>(null)
const editorRef = ref<{ save(): Promise<ArrayBuffer | null> } | null>(null)
const loaded = ref(false)

const Editor = defineAsyncComponent({
  loader: async () => {
    const [mod, zh] = await Promise.all([import('@docx-editor.dev/vue'), import('@docx-editor.dev/i18n/zh-CN'), import('@docx-editor.dev/vue/styles.css')])
    i18n.value = (zh as { default?: unknown }).default ?? zh
    loaded.value = true
    return mod.DocxEditor as unknown as Component
  },
  onError: (_e, _retry, fail) => {
    emit('failed')
    fail()
  },
})
onMounted(() => emit('ready'))

async function save(): Promise<Uint8Array | null> {
  const buf = await editorRef.value?.save()
  return buf ? new Uint8Array(buf) : null
}
defineExpose({ save })
</script>

<template>
  <div class="pvx-docxed" @input.capture="emit('dirty')" @keydown.capture="($event.key.length === 1 || $event.key === 'Backspace' || $event.key === 'Delete' || $event.key === 'Enter') && emit('dirty')" @paste.capture="emit('dirty')" @cut.capture="emit('dirty')">
    <component
      :is="Editor"
      ref="editorRef"
      :document="bytes"
      :i18n="i18n ?? undefined"
      locale="zh-CN"
      mode="edit"
      :color-mode="dark ? 'dark' : 'light'"
      :menu="false"
      :rulers="false"
      :navigation="false"
      :hyperlink-popup="false"
      :author="''"
      :title="title"
    />
    <div v-if="!loaded" class="pvx-msg"><span class="pvx-spin" /><p>正在打开编辑…</p></div>
  </div>
</template>
