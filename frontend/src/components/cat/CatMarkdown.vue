<template>
  <div class="cat-md" :class="{ streaming, calm: reduced }" @click="onClick" @auxclick="onAux">
    <div v-for="b in blocks" :key="b.key" class="cat-md-blk" v-html="b.html" />
  </div>
</template>

<script setup lang="ts">
// 助手回复的 Markdown 显示（安全规则见 utils/catMarkdown.ts）。
// 流式：一帧最多画一次。小增量当帧就跟上；一次进来的大段按帧拆开，避免整段 Markdown 卡死一帧。
// 按顶层块 v-html，没变的块字符串相同、Vue 不会动它的 DOM。
import { onBeforeUnmount, shallowRef, watch } from 'vue'
import { renderCatMarkdown, CAT_MD_COPIED, CAT_MD_COPY, type CatMdBlock } from '@/utils/catMarkdown'
import { openExternalLink } from '@/utils/openExternal'
import { prefersReducedMotion } from '@/api/catStream'
import { hasWailsBackend } from '@/services/wails'
import { ClipboardSetText } from '../../../wailsjs/runtime/runtime'

const props = defineProps<{ text: string; streaming?: boolean }>()
const reduced = prefersReducedMotion()
const blocks = shallowRef<CatMdBlock[]>([])

let raf = 0
let shown = ''

function paint(text: string, streaming: boolean) {
  blocks.value = renderCatMarkdown(text, streaming)
}
function stopRaf() {
  if (!raf) return
  cancelAnimationFrame(raf)
  raf = 0
}
/** 把还没画出来的尾巴往前赶一截。短尾巴一次画完，长尾巴每帧只吃一段。 */
function step() {
  raf = 0
  const full = props.text
  const streaming = !!props.streaming
  if (!streaming || reduced) {
    shown = full
    paint(shown, streaming)
    return
  }
  if (!full.startsWith(shown)) shown = ''
  const behind = full.length - shown.length
  if (behind <= 32) {
    shown = full
    paint(shown, true)
    return
  }
  const take = Math.min(behind, Math.max(48, Math.ceil(behind / 6)))
  shown = full.slice(0, shown.length + take)
  paint(shown, true)
  if (shown.length < full.length && typeof requestAnimationFrame === 'function') {
    raf = requestAnimationFrame(step)
  }
}
watch(
  () => [props.text, props.streaming] as const,
  ([text, streaming]) => {
    stopRaf()
    if (!streaming) {
      shown = text
      paint(shown, false)
      return
    }
    if (typeof requestAnimationFrame === 'function') raf = requestAnimationFrame(step)
    else step()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  stopRaf()
})

async function copyText(text: string): Promise<boolean> {
  try {
    if (hasWailsBackend()) return await ClipboardSetText(text)
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

const copyTimers = new WeakMap<Element, ReturnType<typeof setTimeout>>()

function onClick(e: MouseEvent) {
  const t = e.target as Element | null
  if (!t || !t.closest) return
  const btn = t.closest('[data-cat-copy]')
  if (btn) {
    e.preventDefault()
    const code = btn.closest('.cat-md-code')?.querySelector('pre code')
    void copyText(code?.textContent ?? '').then((ok) => {
      if (!ok) return
      btn.textContent = CAT_MD_COPIED
      clearTimeout(copyTimers.get(btn))
      copyTimers.set(btn, setTimeout(() => { btn.textContent = CAT_MD_COPY }, 1500))
    })
    return
  }
  const a = t.closest('a')
  if (a) {
    // 绝不在窗口内跳转：一律拦下，只有白名单协议交给系统浏览器 / 邮件程序
    e.preventDefault()
    if (a.hasAttribute('data-cat-ext')) openExternalLink(a.getAttribute('href') ?? '')
  }
}
function onAux(e: MouseEvent) {
  const t = e.target as Element | null
  if (t?.closest?.('a')) e.preventDefault()
}
</script>

<style scoped>
.cat-md {
  word-break: break-word;
  overflow-wrap: anywhere;
  user-select: text;
  -webkit-user-select: text;
}
.cat-md-blk + .cat-md-blk {
  margin-top: 8px;
}
.cat-md :deep(p),
.cat-md :deep(ul),
.cat-md :deep(ol),
.cat-md :deep(blockquote),
.cat-md :deep(pre) {
  margin: 0;
}
.cat-md :deep(li > p + p),
.cat-md :deep(li > ul),
.cat-md :deep(li > ol),
.cat-md :deep(blockquote > * + *) {
  margin-top: 4px;
}
.cat-md :deep(ul),
.cat-md :deep(ol) {
  padding-left: 20px;
}
.cat-md :deep(h1),
.cat-md :deep(h2),
.cat-md :deep(h3),
.cat-md :deep(h4),
.cat-md :deep(h5),
.cat-md :deep(h6) {
  margin: 4px 0 0;
  font-weight: 600;
  line-height: 1.5;
}
.cat-md :deep(h1) { font-size: 17px; }
.cat-md :deep(h2) { font-size: 15px; }
.cat-md :deep(h3) { font-size: 14px; }
.cat-md :deep(h4),
.cat-md :deep(h5),
.cat-md :deep(h6) { font-size: 13px; }
.cat-md :deep(hr) {
  border: 0;
  border-top: 1px solid var(--ff-border);
  margin: 4px 0;
}
.cat-md :deep(blockquote) {
  padding: 2px 0 2px 12px;
  border-left: 3px solid var(--ff-border);
  color: var(--ff-text-2);
}
.cat-md :deep(a) {
  color: var(--ff-primary-text);
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
}
.cat-md :deep(a:not([href])) {
  color: inherit;
  text-decoration: none;
  cursor: text;
}
.cat-md :deep(code) {
  font-family: var(--ff-font-mono);
  font-size: 12px;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--ff-bg-hover);
}
.cat-md :deep(.cat-md-code) {
  border: 1px solid var(--ff-border);
  border-radius: 8px;
  background: var(--ff-bg-app);
  overflow: hidden;
}
.cat-md :deep(.cat-md-bar) {
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 6px 0 12px;
  font-size: 11px;
  color: var(--ff-text-3);
  border-bottom: 1px solid var(--ff-border);
}
.cat-md :deep(.cat-md-copy) {
  border: 0;
  background: transparent;
  color: var(--ff-text-2);
  font: inherit;
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 4px;
  cursor: pointer;
}
.cat-md :deep(.cat-md-copy:hover) {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.cat-md :deep(.cat-md-copy:focus-visible) {
  outline: 2px solid var(--ff-primary);
  outline-offset: 1px;
}
.cat-md :deep(.cat-md-code pre) {
  padding: 8px 14px 10px;
  overflow-x: auto;
  font-family: var(--ff-font-mono);
  font-size: 12px;
  line-height: 1.7;
  white-space: pre;
}
.cat-md :deep(.cat-md-code pre code) {
  padding: 0;
  background: none;
  border-radius: 0;
  font-size: inherit;
  font-weight: 400;
}
.cat-md :deep(.cat-md-table) {
  max-width: 100%;
  overflow-x: auto;
}
.cat-md :deep(table) {
  border-collapse: collapse;
  font-size: 12px;
  line-height: 1.6;
}
.cat-md :deep(th),
.cat-md :deep(td) {
  border: 1px solid var(--ff-border);
  padding: 4px 10px;
  text-align: left;
  white-space: nowrap;
}
.cat-md :deep(th) {
  background: var(--ff-bg-hover);
  font-weight: 600;
}
/* 流式光标：只挂在最后一块的最后一段文字后面（减少动效时不显示） */
.cat-md.streaming:not(.calm) .cat-md-blk:last-child > :deep(p:last-child)::after,
.cat-md.streaming:not(.calm) .cat-md-blk:last-child > :deep(:is(ul, ol):last-child > li:last-child)::after {
  content: '';
  display: inline-block;
  width: 7px;
  height: 1em;
  margin-left: 2px;
  vertical-align: -2px;
  background: var(--ff-text-3);
  border-radius: 1px;
  animation: cat-md-caret 1s steps(2, start) infinite;
}
@keyframes cat-md-caret {
  to { visibility: hidden; }
}
@media (prefers-reduced-motion: reduce) {
  .cat-md.streaming .cat-md-blk:last-child > :deep(p:last-child)::after,
  .cat-md.streaming .cat-md-blk:last-child > :deep(:is(ul, ol):last-child > li:last-child)::after {
    display: none;
    animation: none;
  }
}
</style>
