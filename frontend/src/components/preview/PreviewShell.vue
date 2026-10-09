<script setup lang="ts">
// 统一预览弹窗外壳（设计 v0.3 §九，proto/preview-shell.css 的 pvx-*）：遮罩 + 90vw×90vh + 56px 顶部栏 + 内容区 + 多文件左右切换。
// 文档页、转换页共用；各页只换顶部右侧按钮和内容区。Esc 关闭、← → 切换（焦点在输入框里时不切）、Tab 在弹窗内循环，关闭后焦点回到入口（调用方负责）。
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import './preview-shell.css'

const props = defineProps<{
  title: string
  sub?: string
  /** 类型图标的配色类：t-doc / t-sheet / t-slide / t-pdf / t-txt / t-web / t-md / t-video / t-audio */
  tone?: string
  icon?: string
  /** 多个文件时显示左右箭头 */
  multi?: boolean
  canPrev?: boolean
  canNext?: boolean
  /** 编辑中等情况：←→ 不切换 */
  lockNav?: boolean
  label?: string
}>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'prev'): void; (e: 'next'): void }>()
const root = ref<HTMLElement | null>(null)

const FOCUSABLE = 'button:not([disabled]),[href],input:not([disabled]),select:not([disabled]),textarea:not([disabled]),iframe,[tabindex]:not([tabindex="-1"])'
function onKey(e: KeyboardEvent) {
  if (e.defaultPrevented) return
  const t = e.target as HTMLElement | null
  const typing = !!t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)
  if (e.key === 'Escape') {
    e.preventDefault()
    emit('close')
    return
  }
  if (!typing && !props.lockNav && props.multi && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
    e.preventDefault()
    if (e.key === 'ArrowLeft' && props.canPrev) emit('prev')
    if (e.key === 'ArrowRight' && props.canNext) emit('next')
    return
  }
  if (e.key === 'Tab' && root.value) {
    // 有确认框时只在确认框里循环
    const scope = (root.value.querySelector('.pvx-cf') as HTMLElement | null) ?? root.value
    const els = Array.from(scope.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((x) => x.offsetParent !== null || x === document.activeElement)
    if (!els.length) return
    const first = els[0]
    const last = els[els.length - 1]
    const inside = scope.contains(document.activeElement)
    if (e.shiftKey && (document.activeElement === first || !inside)) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && (document.activeElement === last || !inside)) {
      e.preventDefault()
      first.focus()
    }
  }
}
onMounted(() => {
  document.addEventListener('keydown', onKey)
  void nextTick(() => root.value?.querySelector<HTMLElement>('.pvx-x')?.focus())
})
onBeforeUnmount(() => document.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <div class="pvx-mask ff-in-mask" @mousedown.self="emit('close')">
      <div ref="root" class="pvx ff-in-panel" role="dialog" aria-modal="true" :aria-label="label || title">
        <header class="pvx-h">
          <span class="pvx-ti" :class="tone" aria-hidden="true"><FIcon :name="(icon as any) || 'doc'" /></span>
          <div class="tt">
            <h3 :title="title">{{ title }}</h3>
            <span v-if="sub" class="sub" :title="sub">{{ sub }}</span>
          </div>
          <span class="sp" />
          <slot name="actions" />
          <button type="button" class="pvx-x" aria-label="关闭（Esc）" title="关闭（Esc）" @click="emit('close')"><FIcon name="x" /></button>
        </header>
        <div class="pvx-b">
          <slot />
          <template v-if="multi && !lockNav">
            <button type="button" class="pvx-nav prev" aria-label="上一个" :aria-disabled="!canPrev || undefined" @click="canPrev && emit('prev')"><FIcon name="left" /></button>
            <button type="button" class="pvx-nav next" aria-label="下一个" :aria-disabled="!canNext || undefined" @click="canNext && emit('next')"><FIcon name="right" /></button>
          </template>
        </div>
        <slot name="overlay" />
      </div>
    </div>
  </Teleport>
</template>
