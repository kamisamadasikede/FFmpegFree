<template>
  <div
    class="ff-error-line"
    :class="[`tone-${shownTone}`, { compact, arow: actionsRow }]"
    :role="announce ? 'alert' : 'group'"
    :aria-label="announce ? undefined : `${shownTitle} ${shownDescription}`.trim()"
  >
    <FIcon name="warn" :size="16" />
    <div class="body">
      <b v-if="shownTitle">{{ shownTitle }}</b>{{ shownDescription }}
      <div v-if="actionsRow && !compact && hasActions" class="arow-line">
        <span class="acts">
          <button v-if="retryVisible" type="button" class="ff-link" :class="{ busy }" :disabled="busy" :aria-busy="busy" @click="onRetry">重试</button>
          <button v-if="showChange" type="button" class="ff-link" :class="{ busy }" :disabled="busy" :aria-busy="busy" @click="onChange">更换输出位置</button>
          <button v-if="showLog && !canceled" type="button" class="ff-link" @click="emit('viewLog')">查看日志</button>
        </span>
      </div>
      <template v-else-if="!compact">
        <button v-if="retryVisible" type="button" class="ff-link" :class="{ busy }" :disabled="busy" :aria-busy="busy" @click="onRetry">重试</button>
        <button v-if="showChange" type="button" class="ff-link" :class="{ busy }" :disabled="busy" :aria-busy="busy" @click="onChange">更换输出位置</button>
        <button v-if="showLog && !canceled" type="button" class="ff-link" @click="emit('viewLog')">查看日志</button>
      </template>
    </div>
    <div v-if="compact && (retryVisible || showChange || (showLog && !canceled))" class="actions">
      <button v-if="retryVisible" type="button" class="ff-link" :class="{ busy }" :disabled="busy" :aria-busy="busy" @click="onRetry">重试</button>
      <button v-if="showChange" type="button" class="ff-link" :class="{ busy }" :disabled="busy" :aria-busy="busy" @click="onChange">更换输出位置</button>
      <button v-if="showLog && !canceled" type="button" class="ff-link" @click="emit('viewLog')">查看日志</button>
    </div>
  </div>
</template>

<script setup lang="ts">
// 任务中心失败行下方的错误说明，样式来自 proto/errors.html 的 .errline（compact 为 index.html 任务中心的横排版本）。
// 平时是 role="group"，不会让读屏软件把每一行都当成紧急提醒；只有新出现的错误（announce）才是 role="alert"。
import { computed } from 'vue'
import FIcon from '../icon/FIcon.vue'
import { renderTaskError, userVisibleMessage, UNMAPPED_ERROR_TEXT } from '../../errors/errorMessages'

const props = withDefaults(
  defineProps<{
    code: string
    /** 后端 AppError.message。已知错误码用冻结文案；未知错误码把它作为说明文字 */
    message?: string
    /** 后端 AppError.detail。未知错误码且没有 message 时，取最后一行非空内容当说明 */
    detail?: string
    showLog?: boolean
    showRetry?: boolean
    /** 不显示重试链接（已中断的行：重试按钮在行内，这里只保留说明和查看日志） */
    hideRetry?: boolean
    /** 新出现的错误：role="alert"，读屏软件会立即播报 */
    announce?: boolean
    /** 横排紧凑版：标题、说明、错误码在一行，操作靠右 */
    compact?: boolean
    /** 卡片版（转换页记录卡，设计截图 05）：标题和说明一行；下一行左边“重试 · 查看日志”，右边错误码 */
    actionsRow?: boolean
    /** danger 红色（失败）；interrupted 灰橙色（已中断） */
    tone?: 'danger' | 'interrupted' | 'neutral'
    /** 没有专属文案的错误码的标题，默认「转换失败」；非任务错误（如列表加载失败）可改成别的，但不会是「出错了」 */
    fallbackTitle?: string
    /** 覆盖标题 / 说明（interrupted 且后端没给 error 时用）；title 传空串 = 不显示标题，只有一行说明 */
    title?: string
    description?: string
    /** 任务类型（任务中心传）：没有专属文案时按类型取标题，直播不会是「转换失败」 */
    taskType?: string
    /** 保留给旧调用。错误码现在任何情况下都不显示（契约 v0.25.3）。 */
    hideCode?: boolean
    /** 重试 / 更换输出位置正在处理：这两个链接禁用（aria-busy），忽略点击直到调用返回 */
    busy?: boolean
  }>(),
  { busy: false, showLog: true, showRetry: false, announce: false, compact: false, actionsRow: false, tone: 'danger', hideCode: false },
)
const emit = defineEmits<{ viewLog: []; retry: []; changeOutput: [] }>()

function onRetry() {
  if (!props.busy) emit('retry')
}
function onChange() {
  if (!props.busy) emit('changeOutput')
}

const resolved = computed(() => renderTaskError(props.code, props.message, props.detail, props.taskType))
/** 该错误码是否带「更换输出位置」（磁盘空间不足） */
const showChange = computed(() => !canceled.value && !props.title && resolved.value.actions.includes('changeOutput'))
/** CANCELED 不是失败：中性样式，不提供重试 / 更换输出位置 / 查看日志 */
const canceled = computed(() => props.code === 'CANCELED')
const shownTone = computed(() => (canceled.value ? 'neutral' : props.tone))
const retryVisible = computed(() => props.showRetry && !props.hideRetry && !canceled.value)
const shownTitle = computed(() => props.title ?? (!resolved.value.known && props.fallbackTitle ? props.fallbackTitle : resolved.value.title))
const shownDescription = computed(() => userVisibleMessage(props.description ?? resolved.value.description) || UNMAPPED_ERROR_TEXT)
const hasActions = computed(() => retryVisible.value || showChange.value || (props.showLog && !canceled.value))
</script>

<style scoped>
.ff-error-line {
  --tone: var(--ff-danger);
  display: flex;
  gap: 12px;
  align-items: flex-start;
  padding: 8px var(--ff-space-3);
  border-radius: 8px;
  background: color-mix(in srgb, var(--tone) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--tone) 28%, transparent);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  line-height: 1.6;
}
.ff-error-line.tone-interrupted {
  --tone: var(--ff-interrupted);
}
.ff-error-line.tone-neutral {
  --tone: var(--ff-text-2);
}
.ff-error-line > svg {
  color: var(--tone);
  margin-top: 1px;
}
.body {
  flex: 1;
  min-width: 0;
}
b {
  display: block;
  color: var(--ff-text-1);
  font-weight: 500;
  font-size: var(--ff-fs-sm);
}
.ff-link {
  margin-left: 12px;
  padding: 0;
  border: none;
  background: transparent;
  font: inherit;
  color: var(--ff-primary-text);
  cursor: pointer;
  border-radius: 2px;
}
.ff-link:hover {
  text-decoration: underline;
}
.ff-link:disabled {
  cursor: progress;
  opacity: 0.55;
  text-decoration: none;
}
.code {
  font-family: var(--ff-font-mono);
  font-size: 12px;
  color: var(--ff-text-2);
}
/* 卡片版：标题和说明同一行，操作左、错误码右 */
.arow {
  gap: 8px;
  line-height: 1.5;
}
.arow b {
  display: inline;
  margin-right: 6px;
}
.arow-line {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 2px;
  white-space: nowrap;
}
.arow-line .acts {
  display: flex;
  gap: 12px;
}
.arow-line .ff-link {
  margin-left: 0;
}
.arow-line .code {
  margin-left: auto;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
/* 横排紧凑版 */
.compact {
  gap: 8px;
  padding: 8px var(--ff-space-3);
  line-height: 1.5;
}
.compact b {
  display: inline;
  margin-right: 8px;
}
.compact .code {
  margin-left: 8px;
}
.compact .actions {
  display: flex;
  gap: 0; /* 链接间距只由 .ff-link 的 margin-left 12px 提供 */
  flex: none;
}
</style>
