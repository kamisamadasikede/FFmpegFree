<template>
  <div class="row-wrap" :class="{ haserr: !!errorLine }">
    <div ref="el" class="row" :class="[`s-${state}`, { selectable, sel: selected }]" @click="onRowClick">
      <div class="thumb" :class="{ audio: isAudio }" aria-hidden="true">
        <img v-if="row.thumb" :src="row.thumb" alt="" />
        <FIcon v-else :name="isAudio ? 'music' : 'play'" :size="18" />
        <span v-if="durationText" class="dur">{{ durationText }}</span>
      </div>

      <div class="fmeta">
        <button v-if="selectable" type="button" class="fname fbtn" :title="row.path" :aria-pressed="selected" :aria-label="`${row.name}，查看文件信息`" @click.stop="emit('select')">{{ row.name }}</button>
        <div v-else class="fname" :title="row.path">{{ row.name }}</div>
        <div class="finfo">
          <span class="ellip" :title="state === 'invalid' ? row.path : undefined">{{ infoLine }}</span>
          <span v-if="toText" class="to">{{ toText }}</span>
        </div>
      </div>

      <div class="prog">
        <div class="pline">
          <span class="tag" :class="tag.cls"><FIcon v-if="tag.icon" :name="tag.icon" :size="12" />{{ tag.label }}</span>
          <span class="ptxt ellip" :title="progressText">{{ progressText }}</span>
        </div>
        <div v-if="showBar" class="bar" role="progressbar" :aria-label="`${row.name} 进度`" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="percent">
          <i :class="barClass" :style="{ width: percent + '%' }" />
        </div>
      </div>

      <div class="ops">
        <button v-if="state === 'succeeded'" type="button" class="iconbtn" :title="`打开输出位置 ${row.name}`" :aria-label="`打开输出位置 ${row.name}`" @click="emit('reveal')"><FIcon name="folder" :size="16" /></button>
        <button v-if="state === 'queued' || state === 'running'" type="button" class="iconbtn" :title="`取消 ${row.name}`" :aria-label="`取消 ${row.name}`" @click="emit('cancel')"><FIcon name="x" :size="16" /></button>
        <button v-if="state === 'canceled'" type="button" class="btn sm" @click="emit('readd')"><FIcon name="retry" />重新加入</button>
        <button v-if="canRemove" type="button" class="iconbtn" :title="`移出列表 ${row.name}`" :aria-label="`移出列表 ${row.name}`" @click="emit('remove')"><FIcon name="x" :size="16" /></button>
      </div>
    </div>

    <div v-if="errorLine" class="errwrap">
      <ErrorLine
        compact
        :tone="state === 'interrupted' ? 'interrupted' : 'danger'"
        :code="errorLine.code"
        :message="errorLine.message"
        :detail="errorLine.detail"
        :title="errorLine.title"
        :description="errorLine.description"
        :show-retry="errorLine.retry"
        :busy="busy"
        announce
        :show-log="errorLine.log"
        @retry="emit('retry')"
        @change-output="emit('changeOutput')"
        @view-log="emit('viewLog')"
      />
    </div>
    <div v-if="logText !== null" class="logwrap">
      <div class="loghead">
        <span>日志 · {{ row.name }}</span>
        <span class="grow" />
        <button type="button" class="iconbtn sm" title="关闭日志" aria-label="关闭日志" @click="emit('closeLog')"><FIcon name="x" :size="14" /></button>
      </div>
      <pre class="log selectable" tabindex="0" aria-label="任务日志">{{ logText || '（暂无日志）' }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import ErrorLine from '@/components/common/ErrorLine.vue'
import type { IconName } from '@/components/icon/icons'
import { dirName, fileBaseName, formatBytes, formatEta, formatShortClock } from '@/utils/format'
import { isAudioInfo, rowInfoText } from '@/utils/mediaText'
import { probeErrorText, PROBE_ERROR_TITLE, SUBMIT_ERROR_TITLE } from '@/errors/errorMessages'
import type { ConvertRow, RowState, RowTask } from '@/stores/convert'

const props = defineProps<{
  row: ConvertRow
  state: RowState
  task?: RowTask
  /** 与当前所选预设不兼容的原因 */
  conflict: string | null
  /** 当前所选预设的短名（MP4 / GIF …），未提交的行用它显示「转为 X」 */
  presetShort: string
  /** 展开日志时的文本；null = 未展开 */
  logText: string | null
  /** ffmpeg 是否就绪：没就绪时“还没读取”的行显示“等待 ffmpeg” */
  ffmpegReady?: boolean
  /** 多个文件时可点选这一行查看文件信息卡 */
  selectable?: boolean
  selected?: boolean
  /** 该行的重试 / 更换输出位置正在处理：对应链接禁用（aria-busy），点击被忽略 */
  busy?: boolean
}>()
const emit = defineEmits<{
  remove: []
  cancel: []
  readd: []
  retry: []
  changeOutput: []
  viewLog: []
  closeLog: []
  reveal: []
  visible: []
  select: []
}>()

const el = ref<HTMLElement | null>(null)
let io: IntersectionObserver | null = null
onMounted(() => {
  if (typeof IntersectionObserver === 'undefined' || !el.value) return emit('visible')
  io = new IntersectionObserver((es) => {
    if (es.some((e) => e.isIntersecting)) {
      emit('visible')
      io?.disconnect()
    }
  })
  io.observe(el.value)
})
onUnmounted(() => io?.disconnect())

const isAudio = computed(() => props.row.probe === 'ok' && isAudioInfo(props.row.info))
const durationText = computed(() => formatShortClock(props.row.info?.duration ?? 0))

const infoText = computed(() => {
  const i = props.row.info
  return props.row.probe === 'ok' && i ? rowInfoText(i) : ''
})
/**
 * 第二行：读取成功 = 分辨率 · 编码 · 大小；读取失败 = 文件所在文件夹（原因只在标签和下面的说明里写一遍，
 * 文件名在上一行；读取失败拿不到大小）；还没读取 = 等待 ffmpeg / 正在读取。
 */
const infoLine = computed(() => {
  if (infoText.value) return infoText.value
  if (props.state === 'invalid') return dirName(props.row.path)
  if (props.state === 'waiting' && props.ffmpegReady === false) return '装好 ffmpeg 后读取文件信息'
  return '正在读取文件信息…'
})

function onRowClick(e: MouseEvent) {
  if (!props.selectable) return
  if ((e.target as HTMLElement).closest('button, a, pre')) return
  emit('select')
}

const toText = computed(() => {
  if (props.state === 'invalid' || props.state === 'probing' || props.state === 'waiting') return ''
  const label = props.row.label || props.presetShort
  return label ? `转为 ${label}` : ''
})

const percent = computed(() => Math.round(Math.min(1, Math.max(0, props.task?.progress ?? 0)) * 100))
const showBar = computed(() => ['running', 'failed', 'interrupted'].includes(props.state) && (props.state === 'running' || percent.value > 0))
const barClass = computed(() => ({ run: props.state === 'running', fail: props.state === 'failed', int: props.state === 'interrupted' }))

const TAGS: Record<RowState, { label: string; cls: string; icon?: IconName }> = {
  waiting: { label: '读取中', cls: 'q' }, // ffmpeg 未就绪时改成“等待 ffmpeg”，见 tag
  probing: { label: '读取中', cls: 'q' },
  invalid: { label: '无法读取', cls: 'fail', icon: 'warn' },
  conflict: { label: '不兼容', cls: 'fail', icon: 'warn' },
  ready: { label: '待转换', cls: 'q' },
  queued: { label: '排队中', cls: 'q' },
  running: { label: '转换中', cls: 'run' },
  succeeded: { label: '完成', cls: 'ok', icon: 'check' },
  failed: { label: '失败', cls: 'fail' },
  interrupted: { label: '已中断', cls: 'int', icon: 'warn' },
  canceled: { label: '已取消', cls: 'cx' },
}
const tag = computed(() => (props.state === 'waiting' && props.ffmpegReady === false ? { label: '等待 ffmpeg', cls: 'q' } : TAGS[props.state]))

const progressText = computed(() => {
  switch (props.state) {
    case 'running': {
      const eta = formatEta(props.task?.etaSec ?? 0)
      return `${percent.value}%${eta ? ` · 剩余 ${eta}` : ''}`
    }
    case 'succeeded': return props.task?.outputPath ? fileBaseName(props.task.outputPath) : '已完成'
    case 'failed':
    case 'interrupted': return percent.value > 0 ? `停在 ${percent.value}%` : ''
    default: return ''
  }
})

const canRemove = computed(() => ['waiting', 'probing', 'invalid', 'conflict', 'ready', 'succeeded', 'failed', 'interrupted', 'canceled'].includes(props.state))

interface ErrLine { code: string; message?: string; detail?: string; title?: string; description?: string; retry: boolean; log: boolean }
const errorLine = computed<ErrLine | null>(() => {
  const r = props.row
  if (props.state === 'invalid' && r.probeError) {
    const e = r.probeError
    return { code: e.code, detail: undefined, title: PROBE_ERROR_TITLE, description: probeErrorText(e.code, e.message), retry: false, log: false }
  }
  if (props.state === 'conflict' && props.conflict) {
    return { code: 'INVALID_ARGUMENT', title: '这个文件不能用当前预设', description: props.conflict, retry: false, log: false }
  }
  if (r.submitError && !r.taskId) {
    const e = r.submitError
    const detailRest = (e.detail ?? '').split(/\r?\n/).slice(1).join(' ').trim()
    if (e.code === 'CANCELED') return { code: e.code, message: e.message, retry: false, log: false }
    return { code: e.code, title: SUBMIT_ERROR_TITLE, description: [e.message, detailRest].filter(Boolean).join('：'), retry: false, log: false }
  }
  if ((props.state === 'failed' || props.state === 'interrupted') && props.task) {
    const e = props.task.error
    if (e) return { code: e.code, message: e.message, detail: e.detail, retry: true, log: true }
    return { code: 'INTERRUPTED', title: '', description: '应用退出时这个任务被中断，可以重试。', retry: true, log: true }
  }
  return null
})
</script>

<style scoped>
.row-wrap {
  border-radius: var(--ff-radius-lg);
  container-type: inline-size;
}
.row {
  display: flex;
  align-items: center;
  gap: var(--ff-space-3);
  padding: var(--ff-space-3) var(--ff-space-2);
  border-radius: 8px;
}
.row:hover {
  background: var(--ff-bg-hover);
}
.row.selectable {
  cursor: pointer;
}
.row.sel {
  background: var(--ff-primary-soft);
}
.row-wrap.haserr .row {
  padding-bottom: var(--ff-space-2);
}
.thumb {
  width: 56px;
  height: 36px;
  border-radius: var(--ff-radius-md);
  flex: none;
  position: relative;
  overflow: hidden;
  background: var(--ff-bg-hover);
  color: var(--ff-text-2);
  display: grid;
  place-items: center;
}
.thumb.audio {
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
}
.thumb img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.dur {
  position: absolute;
  right: 2px;
  bottom: 2px;
  font-size: var(--ff-fs-xs);
  line-height: 16px;
  padding: 0 4px;
  border-radius: var(--ff-radius-sm);
  color: var(--ff-text-1);
  background: color-mix(in srgb, var(--ff-bg-surface) 82%, transparent);
}
.fmeta {
  flex: 1;
  min-width: 0;
}
.fname {
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.fbtn {
  display: block;
  max-width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  font: inherit;
  font-weight: 500;
  color: var(--ff-text-1);
  text-align: left;
  cursor: pointer;
  border-radius: var(--ff-radius-sm);
}
.fbtn:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.finfo {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  display: flex;
  gap: var(--ff-space-2);
  align-items: center;
  min-width: 0;
}
.finfo .to {
  color: var(--ff-text-2);
  flex: none;
}
.ellip {
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.prog {
  width: 160px;
  order: 0;
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.pline {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--ff-space-2);
}
.ptxt {
  text-align: right;
}
.tag {
  height: 20px;
  padding: 0 8px;
  border-radius: var(--ff-radius-sm);
  font-size: var(--ff-fs-xs);
  display: inline-flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
  flex: none;
}
.tag.run { background: var(--ff-primary-soft); color: var(--ff-primary-text); }
.tag.ok { background: color-mix(in srgb, var(--ff-success) 14%, transparent); color: var(--ff-success-text); }
.tag.fail { background: color-mix(in srgb, var(--ff-danger) 14%, transparent); color: var(--ff-danger-text); }
.tag.int { background: color-mix(in srgb, var(--ff-interrupted) 14%, transparent); color: var(--ff-interrupted); }
.tag.q { background: var(--ff-bg-hover); color: var(--ff-text-2); }
.tag.cx { background: transparent; border: 1px solid var(--ff-border); color: var(--ff-text-2); }
.bar {
  width: 100%;
  height: 4px;
  border-radius: 2px;
  background: var(--ff-border);
  overflow: hidden;
}
.bar i {
  display: block;
  position: relative;
  height: 100%;
  background: var(--ff-primary);
  border-radius: 2px;
  overflow: hidden;
  transition: width var(--ff-dur-base) var(--ff-ease);
}
.bar i.fail { background: var(--ff-danger); }
.bar i.int { background: var(--ff-interrupted); }
.ops {
  display: flex;
  gap: 2px;
  color: var(--ff-text-2);
  min-width: 28px;
  justify-content: flex-end;
  flex: none;
}
.iconbtn {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  border-radius: var(--ff-radius-md);
  display: grid;
  place-items: center;
  color: var(--ff-text-2);
  cursor: pointer;
}
.iconbtn:hover {
  background: var(--ff-bg-hover);
}
.iconbtn.sm {
  width: 22px;
  height: 22px;
}
.btn.sm {
  height: 24px;
  padding: 0 8px;
  font: inherit;
  font-size: var(--ff-fs-xs);
  border-radius: var(--ff-radius-md);
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  display: inline-flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
  cursor: pointer;
}
.btn.sm:hover {
  background: var(--ff-bg-hover);
}
/* 列表较窄时（窗口最小 1024 宽：列表约 440px）进度区换到第二行，文件名不再被挤成几个字 */
@container (max-width: 520px) {
  .row {
    flex-wrap: wrap;
    row-gap: var(--ff-space-2);
  }
  .fmeta {
    flex: 1 1 0;
  }
  .ops {
    order: 2;
  }
  .prog {
    order: 3;
    width: auto;
    flex: 1 1 100%;
    margin-left: 68px; /* 缩略图 56 + 间距 12，与文件名对齐 */
  }
  .finfo {
    flex-wrap: wrap;
    column-gap: var(--ff-space-2);
  }
  .prog .pline {
    justify-content: flex-start;
  }
  .prog .ptxt {
    text-align: left;
    flex: 1;
  }
}
.errwrap {
  padding: 0 var(--ff-space-2) var(--ff-space-3);
}
.logwrap {
  margin: 0 var(--ff-space-2) var(--ff-space-3);
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-md);
  overflow: hidden;
}
.loghead {
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  padding: var(--ff-space-2) var(--ff-space-3);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  border-bottom: 1px solid var(--ff-border);
}
.grow {
  flex: 1;
}
.log {
  margin: 0;
  font-family: var(--ff-font-mono);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  background: var(--ff-bg-app);
  padding: var(--ff-space-3);
  line-height: 1.7;
  white-space: pre;
  max-height: 150px;
  min-height: 44px;
  overflow: auto;
}
</style>
