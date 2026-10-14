<template>
  <div class="ct-col">
    <template v-for="row in rows" :key="row.key">
      <template v-if="row.kind === 'block'">
        <div v-if="row.block.kind === 'user'" class="ct-user-row">
          <span v-if="row.block.failed" class="ct-fail" title=t('cat.misc.s_f218738f66') aria-label=t('cat.misc.s_f218738f66') data-testid="cat-user-fail">!</span>
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
          <CatRawDetail v-if="row.block.outdated?.raw && rawOpen[row.key]" :text="row.block.outdated?.raw ?? ''" testid="cat-cli-outdated" />
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
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

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
import CatRawDetail from './CatRawDetail.vue'

/** retry：v0.44.5 最后一轮失败的「重试」状态（can = 显示按钮；gone = 原地灰字；空 = 都不显示） */
const props = defineProps<{ blocks: CatBlock[]; pending?: boolean; retry?: 'can' | 'gone' }>()
const emit = defineEmits<{ retry: [] }>()
/** 「查看详情」默认收起 */
const rawOpen = ref<Record<string, boolean>>({})

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
    outputTokens: block.outputTokens,
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
    return () => p.text.split(/(`[^`]+`)/).map((s) => (s.startsWith('`') && s.endsWith('t('cat.misc.s_8894471112')