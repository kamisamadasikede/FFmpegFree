<script setup lang="ts">
// 弹窗里的小提示（已保存等）：从下方 8px 淡入 + 0.96→1 放大，消失时淡出下沉。样式在 styles/motion.css 的 ff-toast-*。
import { guardLeaving } from '@/utils/motion'
</script>

<template>
  <Transition name="ff-toast" @before-leave="guardLeaving"><slot /></Transition>
</template>
