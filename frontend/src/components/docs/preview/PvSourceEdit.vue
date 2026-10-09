<script setup lang="ts">
// 源码编辑（txt / html，md 的左边）：等宽、行号、不自动换行；原样保存（契约 6.12.37：只做换行和编码两步，后端做）
import { computed, ref } from 'vue'

const props = defineProps<{ modelValue: string; label: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()
const lines = computed(() => props.modelValue.split('\n').length)
const gutter = ref<HTMLElement | null>(null)
const onScroll = (e: Event) => {
  if (gutter.value) gutter.value.scrollTop = (e.target as HTMLElement).scrollTop
}
</script>

<template>
  <div class="pvx-srced">
    <div ref="gutter" class="gut" aria-hidden="true"><div v-for="n in lines" :key="n">{{ n }}</div></div>
    <textarea
      :value="modelValue"
      :aria-label="label"
      spellcheck="false"
      wrap="off"
      autocapitalize="off"
      autocomplete="off"
      @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
      @scroll.passive="onScroll"
      @keydown.tab.prevent="($event.target as HTMLTextAreaElement).setRangeText('\t', ($event.target as HTMLTextAreaElement).selectionStart, ($event.target as HTMLTextAreaElement).selectionEnd, 'end'); emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
    />
  </div>
</template>
