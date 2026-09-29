<template>
  <LiveTabFrame>
    <template #main>
      <LiveFallbackNotice />
      <LivePushPreview />
      <LiveSessionList empty-hint="选择文件并填写推流地址，点击“开始推流”" />
    </template>
    <template #panel>
      <LivePanel title="推流设置">
        <LiveField v-slot="{ id }" label="推流文件">
          <div class="input ro" :class="{ ph: !material }">
            <span :id="id" class="grow" :title="material?.name">{{ material ? material.name : '选择要推流的文件' }}</span>
            <a class="lk" role="button" tabindex="0" @click="pick" @keyup.enter="pick">{{ material ? '更换' : '选择文件' }}</a>
          </div>
        </LiveField>
        <LiveField label="推流地址">
          <LiveInput v-model="baseUrl" :bad="err?.where === 'addr'" placeholder="rtmp://、rtmps:// 或 srt://" @enter="start" />
          <LiveFormError v-if="err?.where === 'addr'" :text="err.text" />
        </LiveField>
        <LiveField label="推流码 / 口令">
          <LiveInput v-model="key" secret :bad="err?.where === 'key'" placeholder="推流码或口令" @enter="start" />
          <LiveFormError v-if="err?.where === 'key'" :text="err.text" />
        </LiveField>
        <LiveFormError v-if="err?.where === 'form'" :text="err.text" class="form-err" />
        <template #action-top>
          <PreviewSwitch v-model="previewOn" :disabled="blocked || starting" :note="starting ? PREVIEW_SWITCH_NOTE_STARTING : undefined" />
        </template>
        <template #action>
          <LiveButton variant="pri" lg icon="play" :disabled="!canStart" :tip-when-disabled="blocked ? '需要先安装 ffmpeg' : undefined" @click="start">开始推流</LiveButton>
        </template>
      </LivePanel>
    </template>
  </LiveTabFrame>
</template>

<script setup lang="ts">
// 文件推流（设计稿 v0.2）：推流文件 → 推流地址 → 推流码 / 口令 → 表单级错误 → 开始推流。会话列表是全局的（stores/liveSessions.ts）。
// 后端调用走 @/api/live（契约 v0.10）：StartFilePush 返回 Task，第一条 task:progress 之后列表里才出现“运行中”。完整地址 / 口令只存在于输入框和调用参数里，不写日志。
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import LiveTabFrame from '@/components/live/LiveTabFrame.vue'
import LivePanel from '@/components/live/LivePanel.vue'
import LiveField from '@/components/live/LiveField.vue'
import LiveInput from '@/components/live/LiveInput.vue'
import LiveButton from '@/components/live/LiveButton.vue'
import LiveFormError from '@/components/live/LiveFormError.vue'
import LiveFallbackNotice from '@/components/live/LiveFallbackNotice.vue'
import LivePushPreview from '@/components/live/LivePushPreview.vue'
import PreviewSwitch from '@/components/live/PreviewSwitch.vue'
import LiveSessionList from '@/components/live/LiveSessionList.vue'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { useLiveSessionsStore } from '@/stores/liveSessions'
import { LIVE_SRT_PASSPHRASE_TEXT } from '@/errors/errorMessages'
import { PREVIEW_SWITCH_NOTE_STARTING } from '@/errors/livePreviewMessages'
import { composePushUrl, parsePushUrl } from '@/utils/liveUrl'
import * as liveApi from '@/api/live'
import { toAppError } from '@/api/call'
import { formPreview } from './pushPreview'
import { pushErrorToForm, type PushFormError } from './pushErrors'

defineOptions({ name: 'LiveFilePush' })

const ffmpeg = useFFmpegStore()
const store = useLiveSessionsStore()
const blocked = computed(() => ffmpeg.featuresBlocked)

const material = ref<liveApi.LiveMaterial | null>(null)
/** 预览开关：会话启动参数，默认开；产品经理已定：不记住上次选择，每次打开表单默认开 */
const previewOn = ref(true)
const baseUrl = ref('')
const key = ref('')
const err = ref<PushFormError | null>(null)
const starting = ref(false)
const canStart = computed(() => !blocked.value && !starting.value && !!material.value && !!baseUrl.value.trim())

watch([baseUrl, key], () => (err.value = null))

async function pick() {
  if (blocked.value) return
  try {
    const picked = liveApi.liveIsReal() ? await liveApi.pickMaterial() : liveApi.demoMaterials().slice(0, 1)
    if (picked[0]) material.value = picked[0]
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  }
}

async function start() {
  if (!canStart.value || !material.value) return
  err.value = null // 新一次点击先清旧错误
  const base = baseUrl.value
  // 前端先校验：SRT 口令长度（不发给后端）→ 地址
  const head = parsePushUrl(composePushUrl(base, ''))
  if (!head.ok) return void (err.value = { where: 'addr', text: head.message })
  if (head.info.scheme === 'srt' && !liveApi.isValidSrtPassphrase(key.value)) return void (err.value = { where: 'key', text: LIVE_SRT_PASSPHRASE_TEXT })
  const full = composePushUrl(base, key.value)
  const check = parsePushUrl(full)
  if (!check.ok) return void (err.value = { where: 'addr', text: check.message })
  starting.value = true
  try {
    const task = await liveApi.startFilePush({ inputPath: material.value.path, url: full, loop: true, options: liveApi.defaultPushOptions(), preview: previewOn.value })
    const r = await store.begin(task, { kind: 'file', redactedUrl: check.info.redacted, archive: false, preview: previewOn.value })
    if (!r.ok) err.value = pushErrorToForm(r.error, check.info.scheme)
    else key.value = ''
  } catch (e) {
    err.value = pushErrorToForm(toAppError(e), check.info.scheme)
  } finally {
    starting.value = false
  }
}

onMounted(() => {
  void store.recover()
  // 浏览器预览：?form=… 预置表单状态（真实运行不读）
  const f = formPreview
  if (!f || f === 'empty') return
  material.value = { name: '概念片_终版.mp4', path: '/Users/me/Movies/概念片_终版.mp4', duration: '00:42:18' }
  baseUrl.value = 'rtmp://live-push.example.com/live'
  key.value = '••••••••••••'
  const E: Record<string, PushFormError> = {
    scheme: { where: 'addr', text: '暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt' },
    malformed: { where: 'addr', text: '推流地址格式不正确，请检查后重新输入' },
    host: { where: 'addr', text: '推流地址里缺少服务器地址，请检查后重新输入' },
    param: { where: 'addr', text: '推流地址里有不支持的参数，请去掉后重试' },
    unknown: { where: 'addr', text: '推流地址不可用，请检查后重新输入' },
    srtpass: { where: 'key', text: LIVE_SRT_PASSPHRASE_TEXT },
    connfail: pushErrorToForm({ code: 'LIVE_CONNECT_FAILED', message: '', detail: 'scheme=rtmp' } as never),
    connfailsrt: pushErrorToForm({ code: 'LIVE_CONNECT_FAILED', message: '', detail: 'scheme=srt', scheme: 'srt' } as never),
    rejected: pushErrorToForm({ code: 'LIVE_PUSH_REJECTED', message: '' } as never),
    same: pushErrorToForm({ code: 'TASK_CONFLICT', message: '', reason: 'duplicate_url' } as never),
    max4: pushErrorToForm({ code: 'TASK_CONFLICT', message: '', reason: 'max_sessions' } as never),
    conflictunk: pushErrorToForm({ code: 'TASK_CONFLICT', message: '' } as never),
    nosrt: pushErrorToForm({ code: 'UNSUPPORTED', message: '', detail: 'missing=srt' } as never),
    noproto: pushErrorToForm({ code: 'UNSUPPORTED', message: '' } as never),
  }
  if (E[f]) err.value = E[f]
  if (f === 'srtpass' || f === 'connfailsrt' || f === 'nosrt') baseUrl.value = 'srt://srt.example.com:9000'
  if (f === 'same') baseUrl.value = 'rtmp://push.example.com/live'
})
</script>

<style scoped>
.input.ro {
  height: 28px;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  background: var(--ff-bg-hover);
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-2);
  overflow: hidden;
}
.input.ph {
  color: var(--ff-text-3);
}
.grow {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.lk {
  flex: none;
  color: var(--ff-primary-text);
  font-size: var(--ff-fs-sm);
  cursor: pointer;
}
.lk:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
  border-radius: 2px;
}
.form-err {
  margin-top: 0;
}
</style>
