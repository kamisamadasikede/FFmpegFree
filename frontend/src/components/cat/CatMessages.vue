<template>
  <div class="ct-col">
    <template v-for="(b, i) in blocks" :key="b.kind === 'a' ? 'a:' + b.id : i">
      <div v-if="b.kind === 'user'" class="ct-user">{{ b.text }}</div>
      <div v-else-if="b.kind === 'think'" class="ct-think"><FIcon name="spark" :size="14" />{{ b.text }}<FIcon name="right" :size="14" /></div>
      <p v-else-if="b.kind === 'p'"><InlineText :text="b.text" /></p>
      <ul v-else-if="b.kind === 'ul'">
        <li v-for="(it, j) in b.items" :key="j"><InlineText :text="it" /></li>
      </ul>
      <div v-else-if="b.kind === 'code'" class="ct-code">
        <div class="bar"><span>{{ b.lang }}</span><FIcon name="copy" :size="14" /></div>
        <pre><template v-for="(seg, j) in b.parts" :key="j"><span v-if="seg[1]" class="kw">{{ seg[0] }}</span><template v-else>{{ seg[0] }}</template></template></pre>
        <div v-if="b.more" class="more">{{ b.more }}</div>
      </div>
      <div v-else-if="b.kind === 'run'" class="ct-run"><i class="ct-spin sm" aria-hidden="true" />{{ b.text }}</div>
      <div
        v-else-if="b.kind === 'a'"
        class="ct-a"
        :class="{ streaming: b.streaming, calm: reduced }"
        :aria-busy="b.streaming"
      ><CatMarkdown :text="b.text" :streaming="b.streaming" /></div>
      <div v-else-if="b.kind === 'sys'" class="ct-sys" :class="{ err: b.tone === 'err' }" role="status">{{ b.text }}</div>
    </template>
    <div v-if="pending" class="ct-run" role="status"><i class="ct-spin sm" aria-hidden="true" />正在思考…</div>
  </div>
</template>

<script setup lang="ts">
import { defineComponent, h } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import CatMarkdown from './CatMarkdown.vue'
import type { CatBlock } from '@/api/catMock'
import { prefersReducedMotion } from '@/api/catStream'

defineProps<{ blocks: CatBlock[]; pending?: boolean }>()

/** 减少动效：流式文字直接追加，不显示闪烁光标（也不做打字机效果） */
const reduced = prefersReducedMotion()

/** `反引号` 包住的片段渲染成行内代码，其余原样文字（不用 v-html） */
const InlineText = defineComponent({
  props: { text: { type: String, required: true } },
  setup(p) {
    return () => p.text.split(/(`[^`]+`)/).map((s) => (s.startsWith('`') && s.endsWith('`') && s.length > 2 ? h('code', s.slice(1, -1)) : s))
  },
})
</script>

<style scoped>
/* 助手回复按 Markdown 显示（CatMarkdown）；用户消息仍是纯文字 pre-wrap */
.ct-a {
  min-width: 0;
}
.ct-sys {
  font-size: 12px;
  line-height: 1.6;
  color: var(--ff-text-3);
}
.ct-sys.err {
  color: var(--ff-text-2);
}
@media (prefers-reduced-motion: reduce) {
  .ct-spin {
    animation: none;
  }
}
.ct-col {
  max-width: 560px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
  font-size: 13px;
  line-height: 1.75;
  color: var(--ff-text-1);
  padding: 0 16px;
}
p,
ul {
  margin: 0;
}
ul {
  padding-left: 18px;
}
.ct-user {
  align-self: flex-end;
  max-width: 440px;
  background: var(--ff-bg-hover);
  border-radius: 10px;
  padding: 8px 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
.ct-think {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--ff-primary-text);
}
.ct-code {
  border: 1px solid var(--ff-border);
  border-radius: 8px;
  background: var(--ff-bg-app);
  overflow: hidden;
}
.ct-code .bar {
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 10px;
  font-size: 11px;
  color: var(--ff-text-3);
}
.ct-code pre {
  margin: 0;
  padding: 4px 14px 10px;
  font-family: var(--ff-font-mono);
  font-size: 12px;
  line-height: 1.7;
  white-space: pre;
  overflow-x: auto;
}
.ct-code .kw {
  color: #2f5bd3;
}
html.dark .ct-code .kw {
  color: #93b4ff;
}
.ct-code .more {
  height: 28px;
  display: grid;
  place-items: center;
  font-size: 12px;
  color: var(--ff-text-2);
  border-top: 1px solid var(--ff-border);
}
.ct-col :deep(code) {
  font-family: var(--ff-font-mono);
  font-size: 12px;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--ff-bg-hover);
  font-weight: 600;
}
.ct-run {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--ff-text-2);
}
.ct-spin {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 2px solid var(--ff-border);
  border-top-color: var(--ff-primary);
  animation: ct-spin 0.9s linear infinite;
}
@keyframes ct-spin {
  to { transform: rotate(360deg); }
}
</style>
