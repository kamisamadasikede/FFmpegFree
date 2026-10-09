<script setup lang="ts">
// 弹窗出现 / 关闭：遮罩淡入淡出，面板（加了 ff-panel 类的那层）轻微上移 + 放大。样式在 styles/motion.css 的 ff-dlg-*。
// 退场时元素设 inert：那 150ms 里回车 / 点击不会再触发一次按钮。挂载、焦点、Esc 处理都不变（仍由弹窗自己负责）。
import { guardLeaving } from '@/utils/motion'
</script>

<template>
  <Transition name="ff-dlg" @before-leave="guardLeaving"><slot /></Transition>
</template>
