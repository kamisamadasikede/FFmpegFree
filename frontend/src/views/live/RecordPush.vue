<template>
  <LiveTabFrame>
    <template #main><LiveSessionList empty-hint="选择屏幕并填写推流地址，点击“开始推流”" /></template>
    <template #panel>
      <LivePanel title="推流设置">
        <LiveField label="屏幕来源" :control="false">
          <div class="scr" role="radiogroup" aria-label="屏幕来源">
            <button
              v-for="sc in screens"
              :key="sc.id"
              type="button"
              class="so"
              :class="{ on: screenId === sc.id }"
              role="radio"
              :aria-checked="screenId === sc.id"
              @click="screenId = sc.id"
            >
              <i class="rd" /><FIcon name="monitor" :size="14" /><span>{{ sc.name }}{{ sc.primary ? '（主显示器）' : '' }}</span><em>{{ sc.width }}×{{ sc.height }}</em>
            </button>
          </div>
          <p v-if="screenId" class="note"><FIcon name="info" :size="14" />{{ LIVE_SCREEN_NO_AUDIO_TEXT }}</p>
        </LiveField>
        <LiveField label="推流地址">
          <LiveInput v-model="baseUrl" :bad="err?.where === 'addr'" placeholder="rtmp://、rtmps:// 或 srt://" @enter="start" />
          <LiveFormError v-if="err?.where === 'addr'" :text="err.text" />
        </LiveField>
        <LiveField label="推流码 / 口令">
          <LiveInput v-model="key" secret :bad="err?.where === 'key'" placeholder="推流码或口令" @enter="start" />
          <LiveFormError v-if="err?.where === 'key'" :text="err.text" />
        </LiveField>
        <div class="chk"><span>保存存档</span><el-switch v-model="archiveOn" size="small" aria-label="保存存档" /></div>
        <template v-if="archiveOn">
          <div class="chk"><span>存档格式</span><span class="fmt">MP4</span></div>
          <LiveField v-slot="{ id }" label="存档目录">
            <div class="input ro">
              <span :id="id" class="grow" :title="archiveDir">{{ archiveDir || '开始推流时选择存档文件夹' }}</span>
              <a class="lk" role="button" tabindex="0" @click="changeDir" @keyup.enter="changeDir">更改</a>
            </div>
          </LiveField>
        </template>
        <LiveFormError v-if="err?.where === 'form'" :text="err.text" class="form-err" />
        <template #action>
          <LiveButton variant="pri" lg icon="play" :disabled="!canStart" :tip-when-disabled="blocked ? '需要先安装 ffmpeg' : undefined" @click="start">开始推流</LiveButton>
        </template>
      </LivePanel>
    </template>
  </LiveTabFrame>
</template>

<script setup lang="ts">
// 录屏推流（设计稿 v0.2）：屏幕来源单选 → 无声音说明 → 推流地址 → 推流码 / 口令 → 保存存档（MP4，目录只读 + 更改）→ 表单级错误 → 开始推流。
// 后端 #47 已支持带存档：archiveDir 非空时任务的 outputPath = 存档路径，终态事件里的 outputPath 决定“打开所在文件夹”。屏幕推流没有声音（audio 恒为 none）。
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import LiveTabFrame from '@/components/live/LiveTabFrame.vue'
import LivePanel from '@/components/live/LivePanel.vue'
import LiveField from '@/components/live/LiveField.vue'
import LiveInput from '@/components/live/LiveInput.vue'
import LiveButton from '@/components/live/LiveButton.vue'
import LiveFormError from '@/components/live/LiveFormError.vue'
import LiveSessionList from '@/components/live/LiveSessionList.vue'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { useLiveSessionsStore } from '@/stores/liveSessions'
import { LIVE_SCREEN_NO_AUDIO_TEXT, LIVE_SRT_PASSPHRASE_TEXT } from '@/errors/errorMessages'
import { composePushUrl, parsePushUrl } from '@/utils/liveUrl'
import * as liveApi from '@/api/live'
import { toAppError } from '@/api/call'
import { getDefaultOutputDir, pickDirectory } from '@/api/system'
import { formPreview } from './pushPreview'
import { pushErrorToForm, type PushFormError } from './pushErrors'

defineOptions({ name: 'LiveRecordPush' })

const ffmpeg = useFFmpegStore()
const store = useLiveSessionsStore()
const blocked = computed(() => ffmpeg.featuresBlocked)
const DEMO_ARCHIVE_DIR = '~/Movies/FFmpegFree/直播存档'

const screens = ref<liveApi.ScreenInfo[]>([])
const screenId = ref('')
const baseUrl = ref('')
const key = ref('')
const archiveOn = ref(false)
const archiveDir = ref('')
const err = ref<PushFormError | null>(null)
const starting = ref(false)
const canStart = computed(() => !blocked.value && !starting.value && !!screenId.value && !!baseUrl.value.trim())

watch([baseUrl, key], () => (err.value = null))
watch(archiveOn, async (on) => {
  if (!on || archiveDir.value) return
  archiveDir.value = (await getDefaultOutputDir().catch(() => '')) || (liveApi.liveIsReal() ? '' : DEMO_ARCHIVE_DIR)
})

async function changeDir() {
  try {
    const d = await pickDirectory('选择存档文件夹')
    if (d) archiveDir.value = d
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  }
}

async function start() {
  if (!canStart.value) return
  err.value = null
  const head = parsePushUrl(composePushUrl(baseUrl.value, ''))
  if (!head.ok) return void (err.value = { where: 'addr', text: head.message })
  if (head.info.scheme === 'srt' && !liveApi.isValidSrtPassphrase(key.value)) return void (err.value = { where: 'key', text: LIVE_SRT_PASSPHRASE_TEXT })
  const full = composePushUrl(baseUrl.value, key.value)
  const check = parsePushUrl(full)
  if (!check.ok) return void (err.value = { where: 'addr', text: check.message })
  let dir = ''
  if (archiveOn.value) {
    dir = archiveDir.value
    if (!dir) {
      try {
        dir = archiveDir.value = await pickDirectory('选择存档文件夹')
      } catch (e) {
        return void ElMessage.error(toAppError(e).message)
      }
      if (!dir) return // 用户取消
    }
    // 演示目录里的 ~ 不是绝对路径，模拟层要求绝对路径
    if (!liveApi.liveIsReal()) dir = dir.replace(/^~/, '/Users/me')
  }
  starting.value = true
  try {
    const task = await liveApi.startScreenPush({ url: full, screenId: screenId.value, hideCursor: false, audio: 'none', archiveDir: dir, options: liveApi.defaultPushOptions() })
    const r = await store.begin(task, { kind: 'screen', redactedUrl: check.info.redacted, archive: !!dir })
    if (!r.ok) err.value = pushErrorToForm(r.error, check.info.scheme)
    else key.value = ''
  } catch (e) {
    err.value = pushErrorToForm(toAppError(e), check.info.scheme)
  } finally {
    starting.value = false
  }
}

onMounted(async () => {
  void store.recover()
  try {
    screens.value = await liveApi.listScreens()
  } catch {
    screens.value = []
  }
  const f = formPreview
  if (f === 'empty') return
  screenId.value = screens.value.find((s) => s.primary)?.id ?? screens.value[0]?.id ?? ''
  if (!f) return
  baseUrl.value = 'rtmp://live-push.example.com/live'
  key.value = '••••••••••••'
  archiveOn.value = true
  archiveDir.value = DEMO_ARCHIVE_DIR
  const E: Record<string, PushFormError> = {
    scheme: { where: 'addr', text: '暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt' },
    srtpass: { where: 'key', text: LIVE_SRT_PASSPHRASE_TEXT },
    connfail: pushErrorToForm({ code: 'LIVE_CONNECT_FAILED', message: '', detail: 'scheme=rtmp' } as never),
    connfailsrt: pushErrorToForm({ code: 'LIVE_CONNECT_FAILED', message: '', detail: 'scheme=srt', scheme: 'srt' } as never),
    rejected: pushErrorToForm({ code: 'LIVE_PUSH_REJECTED', message: '' } as never),
    same: pushErrorToForm({ code: 'TASK_CONFLICT', message: '', reason: 'duplicate_url' } as never),
    max4: pushErrorToForm({ code: 'TASK_CONFLICT', message: '', reason: 'max_sessions' } as never),
    screen1: pushErrorToForm({ code: 'TASK_CONFLICT', message: '', reason: 'screen_busy' } as never),
    perm: pushErrorToForm({ code: 'SCREEN_PERMISSION_DENIED', message: '' } as never),
    unsupported: pushErrorToForm({ code: 'UNSUPPORTED_PLATFORM', message: '' } as never),
    nosrt: pushErrorToForm({ code: 'UNSUPPORTED', message: '', detail: 'missing=srt' } as never),
    noproto: pushErrorToForm({ code: 'UNSUPPORTED', message: '' } as never),
  }
  if (E[f]) err.value = E[f]
  if (f === 'srtpass' || f === 'connfailsrt' || f === 'nosrt') baseUrl.value = 'srt://srt.example.com:9000'
  if (f === 'same') baseUrl.value = 'rtmp://push.example.com/live'
})
</script>

<style scoped>
.scr {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.so {
  height: 32px;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  font: inherit;
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-1);
  background: var(--ff-bg-surface);
  cursor: pointer;
  text-align: left;
}
.so svg {
  color: var(--ff-text-2);
}
.so em {
  margin-left: auto;
  font-style: normal;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.so .rd {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 1.5px solid var(--ff-text-2);
  flex: none;
  position: relative;
}
.so.on {
  border-color: var(--ff-primary);
  background: var(--ff-primary-soft);
}
.so.on .rd {
  border-color: var(--ff-primary);
}
.so.on .rd::after {
  content: '';
  position: absolute;
  inset: 2px;
  border-radius: 50%;
  background: var(--ff-primary);
}
.so:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.note {
  margin: 8px 0 0;
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
  color: var(--ff-text-2);
}
.note > svg {
  margin-top: 1px;
  flex: none;
}
.chk {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: var(--ff-fs-sm);
  font-weight: 500;
}
.fmt {
  color: var(--ff-text-2);
  font-weight: 400;
}
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
