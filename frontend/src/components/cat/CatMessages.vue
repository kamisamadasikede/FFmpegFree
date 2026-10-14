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
        <template v-else-if="row.block.kind === 'a'">
          <div
            class="ct-a"
            :class="{ streaming: row.block.streaming, calm: reduced }"
            :aria-busy="row.block.streaming"
          ><CatMarkdown :id="row.block.id" :text="row.block.text" :streaming="row.block.streaming" :reveal="row.block.reveal" @settled="row.block.reveal = false" /></div>
          <CatMsgFooter v-if="footerFields(row.block)" :fields="footerFields(row.block)!" :force-tip="simTip" />
        </template>
        <div v-else-if="row.block.kind === 'sys' && (row.block.outdated || row.block.providerErr)" class="ct-sys err ct-outdated" role="status" :data-testid="row.block.outdated ? 'cat-cli-outdated' : 'cat-provider-err'">
          <div class="ct-od-line">
            <span>{{ row.block.text }}</span>
            <span v-if="row.block.outdated?.version" class="ct-od-ver" data-testid="cat-cli-outdated-version">{{ CAT_CLI_OUTDATED_COPY.version(row.block.outdated?.version) }}</span>
          </div>
          <div class="ct-od-acts">
            <button v-if="retryAt(row.key) === 'can'" type="button" class="ct-od-btn" :disabled="pending" data-testid="cat-cli-outdated-retry" @click="emit('retry')">{{ CAT_CLI_OUTDATED_COPY.retry }}</button>
            <span v-else-if="retryAt(row.key) === 'gone'" class="ct-retry-gone" data-testid="cat-retry-gone">{{ CAT_RETRY_GONE_TEXT }}</span>
            <button
              v-if="row.block.outdated?.raw"
              type="button"
              class="ct-od-more"
              :aria-expanded="!!rawOpen[row.key]"
              data-testid="cat-cli-outdated-toggle"
              @click="rawOpen[row.key] = !rawOpen[row.key]"
            ><span class="car" aria-hidden="true"><FIcon :name="rawOpen[row.key] ? 'down' : 'right'" :size="12" /></span>{{ CAT_CLI_OUTDATED_COPY.detail }}</button>
          </div>
          <div v-if="row.block.outdated?.raw && rawOpen[row.key]" class="ct-od-box">
            <button type="button" class="ct-od-copy" data-testid="cat-cli-outdated-copy" @click="copyRaw(row.key, row.block.outdated?.raw ?? '')">{{ copied[row.key] ? CAT_CLI_OUTDATED_COPY.copied : CAT_CLI_OUTDATED_COPY.copy }}</button>
            <pre class="ct-od-raw" data-testid="cat-cli-outdated-raw">{{ row.block.outdated?.raw }}</pre>
          </div>
        </div>
        <div v-else-if="row.block.kind === 'sys' && row.block.tone === 'err' && retryAt(row.key)" class="ct-sys err" role="status">
          <div>{{ errText(row.key, row.block.text) }}</div>
          <div class="ct-od-acts">
            <button v-if="retryAt(row.key) === 'can'" type="button" class="ct-od-btn" :disabled="pending" data-testid="cat-retry" @click="emit('retry')">{{ CAT_CLI_OUTDATED_COPY.retry }}</button>
            <span v-else class="ct-retry-gone" data-testid="cat-retry-gone">{{ CAT_RETRY_GONE_TEXT }}</span>
          </div>
        </div>
        <div v-else-if="row.block.kind === 'sys'" class="ct-sys" :class="{ err: row.block.tone === 'err' }" role="status">{{ row.block.tone === 'err' ? errText(row.key, row.block.text) : row.block.text }}</div>
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
    <div v-if="pending" class="ct-act think pend" role="status">
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
import type { CatMessageFooterFields } from './catMessageFooter'
import CatMsgFooter from './CatMsgFooter.vue'
import { catSim } from '@/views/cat/catDevSim'
import { canPreview, showsThumb, userDisplayText, type CatShownFile } from './catMedia'
import { CAT_CLI_OUTDATED_COPY } from './catCliOutdated'
import { CAT_REPLY_FAILED_FINAL, CAT_RETRY_GONE_TEXT } from '@/api/catV0445'
import { CAT_COPY } from '@/api/cat'
import { hasWailsBackend } from '@/services/wails'
import { ClipboardSetText } from '../../../wailsjs/runtime/runtime'

/** retry：v0.44.5 最后一轮失败的「重试」状态（can = 显示按钮；gone = 原地灰字；空 = 都不显示） */
const props = defineProps<{ blocks: CatBlock[]; pending?: boolean; retry?: 'can' | 'gone' }>()
const emit = defineEmits<{ retry: [] }>()
/** 「查看详情」默认收起 */
const rawOpen = ref<Record<string, boolean>>({})
const copied = ref<Record<string, boolean>>({})
async function copyRaw(key: string, text: string) {
  try {
    if (hasWailsBackend()) { if (!(await ClipboardSetText(text))) return }
    else await navigator.clipboard.writeText(text)
  } catch { return }
  copied.value[key] = true
  setTimeout(() => { copied.value[key] = false }, 1500)
}

const previewOn = ref(false)
const previewFile = ref<CatShownFile | null>(null)

function shownText(block: Extract<CatBlock, { kind: 'user' }>): string {
  return userDisplayText(block.text, block.files)
}

/** v0.37.1 / v0.45：仅完成态且有 finishedAt；进行中（streaming）不出尾巴 */
function footerFields(block: Extract<CatBlock, { kind: 'a' }>): CatMessageFooterFields | null {
  if (block.streaming || !block.finishedAt) return null
  return {
    finishedAt: block.finishedAt,
    tokenTotal: block.tokenTotal,
    tokensPerSec: block.tokensPerSec,
    firstTokenMs: block.firstTokenMs,
    totalMs: block.totalMs,
  }
}
/** 走查截图：?cat_sim=footertip 常显悬停提示 */
const simTip = import.meta.env.DEV && catSim.has('footertip')

function openFile(file: CatShownFile) {
  if (!canPreview(file.kind)) return
  previewFile.value = file
  previewOn.value = true
}

/** 减少动效：流式文字直接追加，不显示闪烁光标（也不做打字机效果） */
const reduced = prefersReducedMotion()
const rows = computed(() => presentCatBlocks(props.blocks))
/** 只挂在最后一行、且是失败行（6.19.18.7 第 6 条：只能重试最后一轮） */
const lastErrKey = computed(() => {
  const last = rows.value[rows.value.length - 1]
  return last?.kind === 'block' && last.block.kind === 'sys' && last.block.tone === 'err' ? last.key : ''
})
/** 不能重试时去掉「请重试」：通用失败文案换成「回复没生成出来。」（可重试时保持原文） */
function errText(key: string, text: string): string {
  return text === CAT_COPY.replyFailed && retryAt(key) !== 'can' ? CAT_REPLY_FAILED_FINAL : text
}
function retryAt(key: string): 'can' | 'gone' | undefined {
  return key && key === lastErrKey.value ? props.retry : undefined
}
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
/* 消息尾：气泡下 12px 灰字；200ms 淡入（减少动效则直接显示） */
.ct-meta {
  margin: 2px 0 2px;
  font-size: 12px;
  line-height: 1.4;
  color: var(--cat-muted);
  font-variant-numeric: tabular-nums;
  animation: cat-meta-in var(--ff-dur-base) var(--ff-ease-out);
}
@keyframes cat-meta-in {
  from { opacity: 0; }
  to { opacity: 1; }
}
@media (prefers-reduced-motion: reduce) {
  .ct-meta {
    animation: none;
  }
}
.ct-sys {
  font-size: 12px;
  line-height: 1.6;
  color: var(--cat-muted);
}
.ct-sys.err {
  color: var(--cat-text-2);
}
/* v0.43 CLI 版本过旧：同行内错误样式，下挂次按钮 + 折叠原始报错 */
.ct-od-line { display: flex; flex-wrap: wrap; align-items: baseline; gap: 0 8px; }
.ct-retry-gone { font-size: 11px; color: var(--cat-muted); }
.ct-od-ver { font-size: 11px; color: var(--cat-muted); font-variant-numeric: tabular-nums; }
.ct-od-acts { display: flex; align-items: center; gap: 10px; margin-top: 6px; }
.ct-od-btn {
  height: 26px;
  padding: 0 10px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-elevated);
  color: var(--cat-text);
  font-size: 12px;
  cursor: pointer;
}
.ct-od-btn:hover:not(:disabled) { background: var(--cat-hover); }
.ct-od-btn:disabled { opacity: 0.5; cursor: default; }
.ct-od-more {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  padding: 0;
  background: none;
  color: var(--cat-muted);
  font-size: 12px;
  cursor: pointer;
}
.ct-od-more:hover { color: var(--cat-text-2); }
.ct-od-more .car { width: 12px; display: grid; place-items: center; }
.ct-od-box { position: relative; margin-top: 6px; }
.ct-od-copy {
  position: absolute;
  top: 6px;
  right: 6px;
  z-index: 1;
  height: 22px;
  padding: 0 8px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-elevated);
  color: var(--cat-text-2);
  font-size: 11px;
  cursor: pointer;
}
.ct-od-copy:hover { background: var(--cat-hover); }
.ct-od-raw {
  margin: 0;
  max-height: 160px;
  overflow: auto;
  padding: 8px 64px 8px 10px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-elevated);
  color: var(--cat-text-2);
  font: 12px/1.6 var(--ff-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  white-space: pre-wrap;
  word-break: break-word;
  user-select: text;
  cursor: text;
}
@media (prefers-reduced-motion: reduce) {
  .ct-spin {
    animation: none;
  }
}
.ct-col {
  width: calc(100% - 96px);
  max-width: none;
  box-sizing: border-box;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
  font-size: 13px;
  line-height: 1.75;
  color: var(--cat-text);
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
  background: var(--cat-danger);
  color: var(--cat-on-danger);
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
  max-width: 100%;
}
.ct-user-card {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 180px;
  padding: 4px 8px;
  border: none;
  border-radius: 8px;
  background: var(--cat-user-bubble);
  color: var(--cat-text-2);
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
.ct-user {
  max-width: 100%;
  background: var(--cat-user-bubble);
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
  color: var(--cat-active-text);
}
/* F1 「正在思考…」文字闪光；减少动效下静态次要色 */
/* 只闪文字；同屏只留一个循环动画，等待时不再转圈 */
.ct-act.pend .ct-spin {
  display: none;
}
.ct-act.pend .sum {
  background: linear-gradient(90deg, var(--cat-text-2) 0%, var(--cat-text-2) 40%, var(--cat-accent) 50%, var(--cat-text-2) 60%, var(--cat-text-2) 100%);
  background-size: 300% 100%;
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  animation: cat-shimmer 1.6s linear infinite;
}
@keyframes cat-shimmer {
  from { background-position: 100% 0; }
  to { background-position: 0 0; }
}
@media (prefers-reduced-motion: reduce) {
  .ct-act.pend .sum {
    animation: none;
    background: none;
    color: var(--cat-text-2);
  }
}
.ct-act.tool,
.ct-act.change {
  color: var(--cat-text-2);
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
  outline: 2px solid var(--cat-accent);
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
  color: var(--cat-muted);
}
.ct-fold .sum {
  flex: 1;
  min-width: 0;
  white-space: pre-wrap;
  word-break: break-word;
}
.ct-act.tool .ct-fold:hover .sum,
.ct-act.change .ct-fold:hover .sum {
  color: var(--cat-text);
}
.ct-body {
  margin: 4px 0 0 18px;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--cat-text-2);
}
.ct-body.mono {
  padding: 8px 10px;
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  background: var(--cat-code-bg);
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
  color: var(--cat-success-text);
  background: color-mix(in srgb, var(--cat-success) 16%, transparent);
}
.ct-diff-line.del {
  color: var(--cat-danger-text);
  background: color-mix(in srgb, var(--cat-danger) 14%, transparent);
}
.ct-code {
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  background: var(--cat-code-bg);
  overflow: hidden;
}
.ct-code .bar {
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 10px;
  font-size: 11px;
  color: var(--cat-muted);
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
  color: var(--cat-code-kw);
}
.ct-code .more {
  height: 28px;
  display: grid;
  place-items: center;
  font-size: 12px;
  color: var(--cat-text-2);
  border-top: 1px solid var(--cat-border);
}
.ct-col :deep(code) {
  font-family: var(--ff-font-mono);
  font-size: 12px;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--cat-hover);
  font-weight: 600;
}
.ct-spin {
  width: 12px;
  height: 12px;
  flex: none;
  box-sizing: border-box;
  margin-top: 3px;
  border-radius: 50%;
  border: 2px solid var(--cat-border);
  border-top-color: var(--cat-accent);
  animation: ct-spin 0.9s linear infinite;
}
@keyframes ct-spin {
  to { transform: rotate(360deg); }
}
</style>
