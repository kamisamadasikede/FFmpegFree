<script setup lang="ts">
// 下拉 / 「更多」菜单出现与收起（动画 P1）：淡入 + 从按钮方向 4px + 0.98→1，120ms ease-out；收起 100ms ease-in。
// 样式在 styles/motion.css 的 ff-menu-*；transform-origin 由各菜单自己按锚点写（右上角对齐按钮的写 top right）。
// 收起时 inert + 不接收点击（guardLeaving），不会留下看不见的挡板；挂载、焦点、Esc、外部点击都不变。
import { guardLeaving } from '@/utils/motion'
</script>

<template>
  <Transition name="ff-menu" @before-leave="guardLeaving"><slot /></Transition>
</template>
