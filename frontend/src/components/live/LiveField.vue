<template>
  <div class="field">
    <label :id="labelId" :for="control ? inputId : undefined">{{ label }}</label>
    <slot :id="inputId" />
  </div>
</template>

<script setup lang="ts">
// 面板里的一个表单项：12px 灰色标签 + 控件，标签与控件间距 6px。
// label 的 for 绑定到控件 id；同时把 id 和错误行 id 提供给里面的 LiveInput / InlineError（aria-describedby）。
import { provide, useId } from 'vue'

// control=false：里面是滑块 / 单选组这类非单个表单控件，label 不写 for，改由控件用 aria-labelledby 指向 labelId。
withDefaults(defineProps<{ label: string; control?: boolean }>(), { control: true })
const uid = useId()
const inputId = `ff-f-${uid}`
const errorId = `ff-e-${uid}`
const labelId = `ff-l-${uid}`
provide('ff-field-label-id', labelId)
provide('ff-field-input-id', inputId)
provide('ff-field-error-id', errorId)
</script>

<style scoped>
label {
  display: block;
  font-size: 12px;
  color: var(--ff-text-2);
  margin-bottom: 6px;
  line-height: 18px;
}
</style>
