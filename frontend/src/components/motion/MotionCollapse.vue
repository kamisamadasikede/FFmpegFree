<script setup lang="ts">
// 高度 + 透明度的出现 / 消失（动效规范：出现 200ms ease-out，消失 150ms ease-in；减少动效时瞬间完成）。
// 单个：<MotionCollapse><div v-if>…</div></MotionCollapse>（横幅；v-if / v-else-if 换一种时新旧同时收放，总高度平滑过渡）
// 列表：<MotionCollapse group tag="div" class="…">…v-for…</MotionCollapse>（插入 / 移除行；初次渲染不播）
// 同一帧里进来的行超过 batchLimit（一次添加很多文件、首次读到列表）就不播，直接出现，避免一屏同时动。
import { collapseIn, collapseOut, stopAnim } from '@/utils/motion'

defineOptions({ inheritAttrs: false })
// paused：首次读到列表时由调用方置 true，那一批直接出现
const props = withDefaults(defineProps<{ group?: boolean; tag?: string; batchLimit?: number; paused?: boolean }>(), { group: false, tag: 'div', batchLimit: 6, paused: false })

let pending: { el: Element; done: () => void }[] = []
function onEnter(el: Element, done: () => void) {
  if (props.paused) return done()
  pending.push({ el, done })
  if (pending.length > 1) return
  // 进入钩子在渲染刷新里调用，微任务仍在这一帧绘制之前：先看这一批有多少再决定播不播
  queueMicrotask(() => {
    const batch = pending
    pending = []
    if (batch.length > props.batchLimit) batch.forEach((b) => b.done())
    else batch.forEach((b) => collapseIn(b.el, b.done))
  })
}
const onLeave = (el: Element, done: () => void) => collapseOut(el, done)
const onCancel = (el: Element) => stopAnim(el)
</script>

<template>
  <TransitionGroup v-if="group" :tag="tag" v-bind="$attrs" :css="false" @enter="onEnter" @leave="onLeave" @enter-cancelled="onCancel" @leave-cancelled="onCancel"><slot /></TransitionGroup>
  <Transition v-else :css="false" @enter="onEnter" @leave="onLeave" @enter-cancelled="onCancel" @leave-cancelled="onCancel"><slot /></Transition>
</template>
