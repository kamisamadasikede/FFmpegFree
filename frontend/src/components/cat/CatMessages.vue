<template>
  <div class="ct-col">
    <template v-for="row in rows" :key="row.key">
      <template v-if="row.kind === 'block'">
        <div v-if="row.block.kind === 'user'" class="ct-user-row">
          <span v-if="row.block.failed" class="ct-fail" title="这句没有得到回复" aria-label="这句没有得到回复" data-testid="cat-user-fail">!</span>
          <div class="ct-user-stack" :data-cat-user="shownText(row.block) ? undefined : row.key">
            <div v-if="row.block.files?.length" class="ct-user-files">
              <button
                v-for="(f, i) in row.block.files"
                :key="i"
                type="button"
                class="ct-user-card"
                :class="{ thumb: showsThumb(f.kind), on: canPreview(f.kind) }"
                @click="openFile(f)"
              >
                <CatThumb v-if="showsThumb(f.kind)" :file="f" />
                <template v-else>
                  <FIcon name="file" :size="12" />
                  <b :title="f.name">{{ f.name }}</b>
                </template>
              </button>
            </div>
            <div v-if="shownText(row.block)" class="ct-user" :data-cat-user="row.key">{{ shownText(row.block) }}</div>
          </div>
        </div>
        <div v-else-if="row.block.kind === 'status'" class="ct-act think" role="status" data-testid="cat-retry-status">
          <div class="ct-fold static">
            <span class="car" aria-hidden="true" />
            <i v-if="row.block.streaming" class="ct-spin" aria-hidden="true" />
            <FIcon v-else name="retry" :size="14" />
            <span class="sum">{{ row.block.text }}</span>
          </div>
        </div>
        <p v-else-if="row.block.kind === 'p'"><InlineText :text="row.block.text" /></p>
        <ul v-else-if="row.block.kind === 'ul'">
          <li v-for="(it, j) in row.block.items" :key="j"><InlineText :text="it" /></li>
        </ul>
        <div v-else-if="row.block.kind === 'code'" class="ct-code">
          <div class="bar"><span>{{ row.block.lang }}</span><FIcon name="copy" :size="14" /></div>
          <pre><template v-for="(seg, j) in row.block.parts" :key="j"><span v-if="seg[1]" class="kw">{{ seg[0] }}</span><template v-else>{{ seg[0] }}</template></template></pre>
          <div v-if="row.block.more" class="more">{{ row.block.more }}</div>
        </div>
        <div
          v-else-if="row.block.kind === 'a'"
          class="ct-a"
          :class="{ streaming: row.block.streaming, calm: reduced }"
          :aria-busy="row.block.streaming"
        ><CatMarkdown :id="row.block.id" :text="row.block.text" :streaming="row.block.streaming" :reveal="row.block.reveal" @settled="row.block.reveal = false" /></div>
        <div v-else-if="row.block.kind === 'sys'" class="ct-sys" :class="{ err: row.block.tone === 'err' }" role="status">{{ row.block.text }}</div>
      </template>
      <div v-else class="ct-act" :class="row.fold.tone" :aria-busy="row.fold.streaming || undefined">
        <button
          v-if="row.fold.body"
          type="button"
          class="ct-fold"
          :aria-expanded="isOpen(row.fold)"
          @click="toggle(row.fold)"
        >
          <span class="car" aria-hidden="true"><FIcon :name="isOpen(row.fold) ? 'down' : 'right'" :size="12" /></span>
          <i v-if="row.fold.tone === 'think' && row.fold.streaming" class="ct-spin" aria-hidden="true" />
          <FIcon v-else :name="foldIcon(row.fold)" :size="14" />
          <span class="sum">{{ row.fold.title }}</span>
        </button>
        <div v-else class="ct-fold static">
          <span class="car" aria-hidden="true" />
          <i v-if="row.fold.tone === 'think' && row.fold.streaming" class="ct-spin" aria-hidden="true" />
          <FIcon v-else :name="foldIcon(row.fold)" :size="14" />
          <span class="sum">{{ row.fold.title }}</span>
        </div>
        <div v-if="row.fold.body && isOpen(row.fold)" class="ct-body" :class="{ mono: foldBodyMono(row.fold), diff: row.fold.tone === 'change' }">
          <template v-if="row.fold.tone === 'change'">
            <div v-for="(line, i) in diffRows(row.fold.body)" :key="i" class="ct-diff-line" :class="line.kind">{{ line.text }}</div>
          </template>
          <template v-else>{{ row.fold.body }}</template>
        </div>
      </div>
    </template>
    <div v-if="pending" class="ct-act think" role="status">
      <div class="ct-fold static">
        <span class="car" aria-hidden="true" />
        <i class="ct-spin" aria-hidden="true" />
        <span class="sum">正在思考…</span>
      </div>
    </div>
    <CatPreviewDialog v-model:open="previewOn" :file="previewFile" />
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import CatMarkdown from './CatMarkdown.vue'
import CatThumb from './CatThumb.vue'
import CatPreviewDialog from './CatPreviewDialog.vue'
import type { CatBlock } from '@/api/catMock'
import { prefersReducedMotion } from '@/api/catStream'
import { diffLineKind, foldBodyMono, foldIcon, presentCatBlocks, type CatFold } from './catActivity'
import { canPreview, showsThumb, userDisplayText, type CatShownFile } from './catMedia'

const props = defineProps<{ blocks: CatBlock[]; pending?: boolean }>()

const previewOn = ref(false)
const previewFile = ref<CatShownFile | null>(null)

function shownText(block: Extract<CatBlock, { kind: 'user' }>): string {
  return userDisplayText(block.text, block.files)
}

function openFile(file: CatShownFile) {
  if (!canPreview(file.kind)) return
  previewFile.value = file
  previewOn.value = true
}

/** 减少动效：流式文字直接追加，不显示闪烁光标（也不做打字机效果） */
const reduced = prefersReducedMotion()
const rows = computed(() => presentCatBlocks(props.blocks))
/** 用户点过的折叠；没点过的用默认（查找收起，文件改动展开） */
const opened = ref<Record<string, boolean>>({})

function isOpen(fold: CatFold): boolean {
  const v = opened.value[fold.key]
  return v === undefined ? fold.defaultOpen : v
}

function toggle(fold: CatFold) {
  if (!fold.body) return
  opened.value[fold.key] = !isOpen(fold)
}

function diffRows(body: string): { kind: string; text: string }[] {
  return body.split('\n').map((text) => ({ kind: diffLineKind(text), text }))
}

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
  user-select: text;
  -webkit-user-select: text;
}
p,
ul {
  margin: 0;
}
ul {
  padding-left: 18px;
}
.ct-user-row {
  align-self: flex-end;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  max-width: 100%;
}
.ct-fail {
  flex: none;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--ff-danger);
  color: #fff;
  font-size: 13px;
  font-weight: 700;
  line-height: 18px;
  text-align: center;
}
.ct-user-stack {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
  min-width: 0;
  max-width: 100%;
}
.ct-user-files {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
  max-width: 440px;
}
.ct-user-card {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 180px;
  padding: 4px 8px;
  border: none;
  border-radius: 8px;
  background: #f4f5f7;
  color: var(--ff-text-2);
  font: inherit;
  font-size: 12px;
  line-height: 1.3;
  text-align: left;
  cursor: default;
}
.ct-user-card.on {
  cursor: pointer;
}
.ct-user-card.thumb {
  flex-direction: column;
  align-items: stretch;
  width: 108px;
  height: 92px;
  max-width: 108px;
  padding: 0;
  border-radius: 12px;
  overflow: hidden;
}
.ct-user-card.thumb :deep(.pic) {
  flex: 1;
  min-height: 0;
}
.ct-user-card.thumb b {
  flex: none;
  height: 28px;
  padding: 0 8px;
  display: flex;
  align-items: center;
}
.ct-user-card b {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}
html.dark .ct-user-card {
  background: var(--ff-bg-hover);
}
.ct-user {
  max-width: 440px;
  background: var(--ff-bg-hover);
  border-radius: 10px;
  padding: 8px 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
.ct-act {
  font-size: 12px;
  line-height: 1.6;
}
.ct-act.think {
  color: var(--ff-primary-text);
}
.ct-act.tool,
.ct-act.change {
  color: var(--ff-text-2);
}
.ct-fold {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  width: 100%;
  margin: 0;
  padding: 1px 0;
  border: 0;
  background: none;
  color: inherit;
  font: inherit;
  line-height: inherit;
  text-align: left;
  cursor: pointer;
}
.ct-fold.static {
  cursor: default;
}
.ct-fold:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
  border-radius: 4px;
}
.ct-fold .car {
  width: 12px;
  height: 14px;
  display: grid;
  place-items: center;
  flex: none;
  margin-top: 2px;
  color: var(--ff-text-3);
}
.ct-fold .sum {
  flex: 1;
  min-width: 0;
  white-space: pre-wrap;
  word-break: break-word;
}
.ct-act.tool .ct-fold:hover .sum,
.ct-act.change .ct-fold:hover .sum {
  color: var(--ff-text-1);
}
.ct-body {
  margin: 4px 0 0 18px;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--ff-text-2);
}
.ct-body.mono {
  padding: 8px 10px;
  border: 1px solid var(--ff-border);
  border-radius: 8px;
  background: var(--ff-bg-app);
  font-family: var(--ff-font-mono);
  font-size: 12px;
  line-height: 1.6;
}
.ct-body.diff {
  padding: 4px 0;
}
.ct-diff-line {
  padding: 0 10px;
  white-space: pre-wrap;
  word-break: break-word;
}
.ct-diff-line.add {
  color: var(--ff-success-text);
  background: color-mix(in srgb, var(--ff-success) 16%, transparent);
}
.ct-diff-line.del {
  color: var(--ff-danger-text);
  background: color-mix(in srgb, var(--ff-danger) 14%, transparent);
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
.ct-spin {
  width: 12px;
  height: 12px;
  flex: none;
  box-sizing: border-box;
  margin-top: 3px;
  border-radius: 50%;
  border: 2px solid var(--ff-border);
  border-top-color: var(--ff-primary);
  animation: ct-spin 0.9s linear infinite;
}
@keyframes ct-spin {
  to { transform: rotate(360deg); }
}
</style>
