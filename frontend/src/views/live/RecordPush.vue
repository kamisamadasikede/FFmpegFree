<template>
  <LiveTabFrame>
    <template #main>
      <LivePushPreview />
      <LiveSessionList :empty-hint="pickerMode === 'dropdown' ? LIVE_RECORD_EMPTY_HINT_WIN : LIVE_RECORD_EMPTY_HINT" />
    </template>
    <template #panel>
      <LivePanel title="推流设置">
        <LiveField :label="pickerMode === 'dropdown' ? LIVE_SOURCE_FIELD_LABEL : LIVE_SOURCE_FIELD_LABEL_SCREEN" :control="false">
          <CaptureSourcePicker
            ref="picker"
            v-model="sourceId"
            :sources="sources"
            :state="srcState"
            :mode="pickerMode"
            :invalid="err?.where === 'source'"
            :gone="goneShown"
            :gone-item="goneItem"
            @update:model-value="onPick"
            @refresh="loadSources(true)"
          />
          <LiveFormError v-if="err?.where === 'source'" :text="err.text">
            <button type="button" class="lk" @click="refreshFromError">{{ LIVE_SOURCE_REFRESH }}</button>
          </LiveFormError>
          <p v-if="sourceId" class="note"><FIcon name="info" :size="14" />{{ LIVE_SCREEN_NO_AUDIO_TEXT }}</p>
        </LiveField>
        <LiveField label="推流地址">
          <LiveInput v-model="baseUrl" mask-key :bad="err?.where === 'addr'" placeholder="rtmp://、rtmps:// 或 srt://" @enter="start" />
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
        <template #action-top>
          <PreviewSwitch v-model="previewOn" :disabled="blocked || starting" :note="starting ? PREVIEW_SWITCH_NOTE_STARTING : undefined" />
        </template>
        <template #action>
          <LiveButton variant="pri" lg icon="play" :disabled="!canStart" :tip-when-disabled="blocked ? '需要先安装转换组件' : undefined" @click="start">开始推流</LiveButton>
        </template>
      </LivePanel>
    </template>
  </LiveTabFrame>
</template>

<script setup lang="ts">
// 录屏推流（设计稿 v0.2 + 直播 v1.1 采集来源选择器，后者设计稿未出）：采集来源（屏幕 / 应用窗口） → 无声音说明 → 推流地址 → 推流码 / 口令 → 保存存档（MP4，目录只读 + 更改）→ 表单级错误 → 开始推流。
// 后端 #47 已支持带存档：archiveDir 非空时任务的 outputPath = 存档路径，终态事件里的 outputPath 决定“打开所在文件夹”。屏幕推流没有声音（audio 恒为 none）。
import { computed, nextTick, onMounted, ref, toRefs, watch } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import LiveTabFrame from '@/components/live/LiveTabFrame.vue'
import LivePanel from '@/components/live/LivePanel.vue'
import LiveField from '@/components/live/LiveField.vue'
import LiveInput from '@/components/live/LiveInput.vue'
import LiveButton from '@/components/live/LiveButton.vue'
import LiveFormError from '@/components/live/LiveFormError.vue'
import CaptureSourcePicker from '@/components/live/CaptureSourcePicker.vue'
import LivePushPreview from '@/components/live/LivePushPreview.vue'
import PreviewSwitch from '@/components/live/PreviewSwitch.vue'
import LiveSessionList from '@/components/live/LiveSessionList.vue'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { restoreSourceId, useLiveFormsStore } from '@/stores/liveForms'
import { useLiveSessionsStore } from '@/stores/liveSessions'
import { recordStartEnabled, sourcePickerMode } from '@/utils/liveSource'
import { LIVE_RECORD_EMPTY_HINT, LIVE_RECORD_EMPTY_HINT_WIN, LIVE_SCREEN_NO_AUDIO_TEXT, LIVE_SOURCE_FIELD_LABEL, LIVE_SOURCE_FIELD_LABEL_SCREEN, LIVE_SOURCE_REFRESH, LIVE_SRT_PASSPHRASE_TEXT, liveSourceGoneText } from '@/errors/errorMessages'
import { PREVIEW_SWITCH_NOTE_STARTING } from '@/errors/livePreviewMessages'
import { composePushUrl, parsePushUrl } from '@/utils/liveUrl'
import * as liveApi from '@/api/live'
import { toAppError, type AppError } from '@/api/call'
import { getOutputDirShown, pickDirectory } from '@/api/system'
import { formPreview } from './pushPreview'
import { pushErrorToForm, type PushFormError } from './pushErrors'

defineOptions({ name: 'LiveRecordPush' })

const ffmpeg = useFFmpegStore()
const store = useLiveSessionsStore()
const blocked = computed(() => ffmpeg.featuresBlocked)
const DEMO_ARCHIVE_DIR = '~/Movies/FFmpegFree/直播存档'

// 表单输入在 stores/liveForms（切换菜单不丢、下次启动恢复）；这里只是引用
const forms = useLiveFormsStore()
const { sourceId, baseUrl, key, archiveOn, archiveDir } = toRefs(forms.screen)
const sources = ref<liveApi.CaptureSource[]>([])
const srcState = ref<'loading' | 'ready' | 'empty' | 'failed'>('loading')
let srcSeq = 0
const picker = ref<InstanceType<typeof CaptureSourcePicker> | null>(null)
/** 平台（GetCaptureCapabilities.platform）：windows → 分组下拉；其他 → 屏幕单选列表。读不到就靠列表里有没有窗口判断，不靠“列表为空” */
const platform = ref('')
const pickerMode = computed(() => sourcePickerMode(platform.value, sources.value))
/** LIVE_SOURCE_GONE：选择器红边 + 名称保留（尺寸位置显示“已不可用”）。重选后 err 清掉即恢复 */
const goneItem = ref<liveApi.CaptureSource | null>(null)
const goneShown = computed(() => err.value?.where === 'source' && !!goneItem.value && !sourceId.value)
/** 预览开关：会话启动参数，默认开；产品经理已定：不记住上次选择，每次打开表单默认开 */
const previewOn = ref(true)
const err = ref<PushFormError | null>(null)
const starting = ref(false)
const canStart = computed(() => recordStartEnabled({ blocked: blocked.value, starting: starting.value, hasUrl: !!baseUrl.value.trim(), sourceId: sourceId.value, state: srcState.value, gone: goneShown.value }))
/** 没选来源（列表加载失败 / 没有可选项）时不传来源，后端默认推主屏：表单里给一句轻提示 */

watch([baseUrl, key], () => (err.value = null))
/** v0.24.3：设置里的输出目录留空 = <base>/output。显示 GetStorageDirs 解析出的真实路径，不显示空文件夹 */
async function fillArchiveDir() {
  if (archiveDir.value) return
  archiveDir.value = (await getOutputDirShown().catch(() => '')) || (liveApi.liveIsReal() ? '' : DEMO_ARCHIVE_DIR)
}
watch(archiveOn, (on) => { if (on) void fillArchiveDir() })
onMounted(() => { if (archiveOn.value) void fillArchiveDir() })

async function changeDir() {
  try {
    const d = await pickDirectory('选择存档文件夹')
    if (d) archiveDir.value = d
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  }
}

/** 拉采集来源列表。已选项（含恢复的）仍在列表里就保留，否则默认选第一个屏幕；keep=false（首次）失败时清空。窗口标题只放在界面里，不打日志 */
async function loadSources(keep = true) {
  const my = ++srcSeq
  srcState.value = 'loading'
  try {
    const list = await liveApi.listCaptureSources()
    if (my !== srcSeq) return
    sources.value = list
    if (!list.length) {
      sourceId.value = ''
      srcState.value = 'empty'
      return
    }
    srcState.value = 'ready'
    // 来源已失效（红边 + 错误行）时刷新不自动改选，等用户重选
    if (goneShown.value) return
    // 刷新（keep）和首次加载都沿用仍在列表里的已选项（首次加载 = 切换菜单回来 / 启动时恢复的来源）；不在了退回第一个屏幕
    sourceId.value = restoreSourceId(list, sourceId.value)
  } catch {
    if (my !== srcSeq) return
    srcState.value = 'failed'
    // 刷新失败且已有旧列表：保留旧列表和已选项（“刷新失败，列表可能已过期”）；首次失败：清空
    if (!sources.value.length || !keep) {
      sources.value = []
      if (!goneShown.value) sourceId.value = ''
    }
  }
}
function pickedSource(id: string) {
  const x = sources.value.find((v) => v.id === id)
  return x ? { title: x.title, kind: x.kind } : undefined
}
/** 换了来源就清掉来源错误 */
function onPick() {
  if (err.value?.where === 'source') err.value = null
  goneItem.value = null
}
/** 错误行的“刷新列表”：展开选择器并立即刷新（Windows）；屏幕单选列表只刷新 */
function refreshFromError() {
  if (pickerMode.value === 'dropdown') picker.value?.openPop()
  else void loadSources(true)
}
/** 表单错误统一入口：LIVE_SOURCE_GONE → 取消已选来源 + 自动刷新列表，错误留在来源选择器下方 */
function showError(e: Pick<AppError, 'code' | 'message' | 'detail' | 'reason' | 'scheme' | 'kind'>, scheme: string) {
  err.value = pushErrorToForm(e, scheme)
  if (e.code === 'LIVE_SOURCE_GONE') {
    goneItem.value = sources.value.find((x) => x.id === sourceId.value) ?? goneItem.value
    sourceId.value = '' // 已不可用的来源要用户重新选，不自动改选
    void loadSources(true)
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
    const task = await liveApi.startScreenPush(liveApi.buildScreenPushRequest({ url: full, sourceId: sourceId.value, archiveDir: dir, preview: previewOn.value }))
    const r = await store.begin(task, { kind: 'screen', redactedUrl: check.info.redacted, archive: !!dir, source: pickedSource(sourceId.value), preview: previewOn.value })
    if (!r.ok) showError(r.error, check.info.scheme)
    else {
      // 包 20：推流码不再在开始后清空（老板要求切换菜单 / 重启后表单原样还在，“重新开始”也要用它）
      previewOn.value = true // 产品经理已定：不记住上次选择，每次开始推流后复位为开（页面被 KeepAlive 保留时也一样）；没开始成功（报错）时保留用户当前选择
    }
  } catch (e) {
    showError(toAppError(e), check.info.scheme)
  } finally {
    starting.value = false
  }
}

onMounted(async () => {
  void store.recover()
  platform.value = (await liveApi.getCaptureCapabilities().catch(() => null))?.platform ?? ''
  await loadSources(false)
  const f = formPreview
  if (f === 'window') sourceId.value = sources.value.find((x) => x.kind === 'window')?.id ?? sourceId.value
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
    srcgone: { where: 'source', text: liveSourceGoneText('window') },
    srcgonescreen: { where: 'source', text: liveSourceGoneText('screen') },
    srcgoneopen: { where: 'source', text: liveSourceGoneText('window') },
    perm: pushErrorToForm({ code: 'SCREEN_PERMISSION_DENIED', message: '' } as never),
    unsupported: pushErrorToForm({ code: 'UNSUPPORTED_PLATFORM', message: '' } as never),
    nosrt: pushErrorToForm({ code: 'UNSUPPORTED', message: '', detail: 'missing=srt' } as never),
    noproto: pushErrorToForm({ code: 'UNSUPPORTED', message: '' } as never),
  }
  if (f === 'srcgone' || f === 'srcgonescreen') {
    goneItem.value = { id: 'window:0', kind: f === 'srcgone' ? 'window' : 'screen', title: f === 'srcgone' ? '会议纪要.txt - 记事本' : '屏幕 2', width: 1280, height: 720 }
    sourceId.value = ''
  }
  if (f === 'srcgoneopen') {
    // 设计稿 213：点“刷新列表”后展开、失效窗口已从列表消失
    goneItem.value = { id: 'window:0', kind: 'window', title: '会议纪要.txt - 记事本', width: 1280, height: 720 }
    sourceId.value = ''
    err.value = { where: 'source', text: liveSourceGoneText('window') }
    await nextTick()
    picker.value?.openPop()
  }
  if (f === 'srtpass' || f === 'connfailsrt' || f === 'nosrt') baseUrl.value = 'srt://srt.example.com:9000'
  if (f === 'same') baseUrl.value = 'rtmp://push.example.com/live'
  await nextTick() // 上面改地址 / 口令会触发“清错误”的 watch，等它跑完再放预览错误
  if (E[f]) err.value = E[f]
})
</script>

<style scoped>
.lk {
  border: 0;
  background: none;
  padding: 0 0 0 8px;
  font: inherit;
  color: var(--ff-primary-text);
  cursor: pointer;
  white-space: nowrap;
}
.lk:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
  border-radius: 2px;
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
