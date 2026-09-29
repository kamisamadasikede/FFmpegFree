<template>
  <span class="me" :title="text"><span class="h">{{ parts.head }}</span><span v-if="parts.tail" class="t">{{ parts.tail }}</span></span>
</template>

<script setup lang="ts">
// 长文件名中间省略（设计说明 5.4）：头部收缩并以 … 结尾，尾部（最后 6 个字符 + 扩展名）固定不收缩；title 带全名。
// 不用 text-overflow 直接截尾，也不用 direction: rtl。
import { computed } from 'vue'
import { splitMiddle } from '@/utils/docLogic'

const props = defineProps<{ text: string }>()
const parts = computed(() => splitMiddle(props.text))
</script>

<style scoped>
.me {
  display: flex;
  min-width: 0;
}
.h {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.t {
  flex: none;
  white-space: pre;
}
</style>
