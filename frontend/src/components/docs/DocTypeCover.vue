<script setup lang="ts">
// 文档类型封面（设计 §五）：浅底 + 线性图标 + 左上格式角标；旧格式右下小时钟
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import type { IconName } from '@/components/icon/icons'

const props = defineProps<{ ext: string; sm?: boolean }>()
const TYPE: Record<string, string> = { doc: 'doc', docx: 'doc', odt: 'doc', rtf: 'doc', txt: 'txt', html: 'web', md: 'md', xls: 'sheet', xlsx: 'sheet', ods: 'sheet', csv: 'sheet', ppt: 'slide', pptx: 'slide', odp: 'slide', pdf: 'pdf' }
const ICON: Record<string, IconName> = { doc: 'doc', txt: 'doc', web: 'link', md: 'edit', sheet: 'list', slide: 'monitor', pdf: 'doc' }
const t = computed(() => TYPE[props.ext] ?? 'doc')
const old = computed(() => ['doc', 'xls', 'ppt'].includes(props.ext))
</script>
<template>
  <div class="cv-th cv-cov" :class="[`t-${t}`, { sm }]" aria-hidden="true">
    <FIcon :name="ICON[t]" :size="14" />
    <em class="cv-cf">{{ ext.toUpperCase() }}</em>
    <i v-if="old" class="dc-old" title="旧格式"><FIcon name="refresh" :size="8" /></i>
  </div>
</template>
