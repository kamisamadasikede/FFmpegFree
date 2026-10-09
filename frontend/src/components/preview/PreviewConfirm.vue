<script setup lang="ts">
// 弹窗里的小确认框（未保存、重新打开、覆盖原文件）。放在 PreviewShell 的 overlay 槽里，焦点在它里面循环。
import { nextTick, onMounted, ref } from 'vue'

export interface ConfirmButton {
  label: string
  value: string
  kind?: 'pri' | 'danger' | ''
}
defineProps<{ title?: string; text: string; sub?: string; buttons: ConfirmButton[] }>()
const emit = defineEmits<{ (e: 'pick', v: string): void }>()
const el = ref<HTMLElement | null>(null)
onMounted(() => void nextTick(() => el.value?.querySelector<HTMLElement>('.btn.pri,.btn.danger')?.focus()))
</script>

<template>
  <div ref="el" class="pvx-cf ff-in-mask" role="alertdialog" aria-modal="true" :aria-label="title || text" @keydown.esc.stop.prevent="emit('pick', 'cancel')">
    <div class="box ff-in-pop">
      <h4 v-if="title">{{ title }}</h4>
      <p class="t">{{ text }}</p>
      <p v-if="sub" class="s" :title="sub">{{ sub }}</p>
      <div class="acts">
        <button v-for="(b, i) in buttons" :key="b.value" type="button" class="btn" :class="[b.kind, { left: i === 0 && buttons.length > 2 }]" @click="emit('pick', b.value)">{{ b.label }}</button>
      </div>
    </div>
  </div>
</template>
