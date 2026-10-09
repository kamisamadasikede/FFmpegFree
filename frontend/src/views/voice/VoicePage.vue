<script setup lang="ts">
// 语音工具 · 转字幕（契约 v0.29.1 §6.18；落地稿 speech-sub-v1）。一期只有这一页，不画另外三个标签。
import { computed, onMounted, onUnmounted } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { useNarrow } from '@/components/convert/useNarrow'
import AsrComponentCard from '@/components/voice/AsrComponentCard.vue'
import CueTimeline from '@/components/voice/CueTimeline.vue'
import '@/components/voice/voice.css'
import { onFilesDropped } from '@/api/fileDrop'
import { useLangAsrStore } from '@/stores/langAsr'
import {
  ASR_EMPTY,
  ASR_EMPTY_TITLE,
  ASR_FAIL,
  ASR_FAIL_TITLE,
  tierSizeText,
} from '@/utils/langText'

defineOptions({ name: 'VoicePage' })

const store = useLangAsrStore()
const narrow = useNarrow()
const dropStyle = { '--wails-drop-target': 'drop' } as Record<string, string>
let offDrop: (() => void) | undefined

const tierLabel = computed(() => (store.tier === 'hd' ? '高清' : '标准'))
const sizeText = computed(() => tierSizeText(store.tier))
const primaryDisabled = computed(() => !store.canGenerate)
const primaryHint = computed(() => {
  if (store.unpublished && !store.demoTimeline) return '组件发布后即可使用'
  if (store.phase === 'empty') return '请重新选择文件'
  if (store.hasFile) return '将生成 1 个任务'
  return '选择文件后开始识别'
})
const dropHint = computed(() =>
  store.unpublished && !store.demoTimeline
    ? '组件发布并下载后，即可在这里生成字幕。'
    : '支持常见视频和音频。生成后可以在下方改时间轴上的字幕。',
)

onMounted(() => {
  void store.init()
  offDrop = onFilesDropped((paths) => {
    if (store.canPick) store.addPaths(paths)
  })
})
onUnmounted(() => {
  offDrop?.()
  offDrop = undefined
})

function onPick() {
  if (!store.canPick) return
  void store.chooseFile()
}
</script>

<template>
  <div class="lg" :class="{ w1024: narrow }" :style="dropStyle">
    <div class="lg-main">
      <AsrComponentCard />

      <div class="lg-panel grow">
        <div class="lg-ph">
          <h2>文件</h2>
          <span class="sp" />
          <button type="button" class="btn" :disabled="!store.canPick" @click="onPick">
            <FIcon name="plus" />{{ store.hasFile ? '重新选择' : '添加文件' }}
          </button>
        </div>

        <div v-if="!store.hasFile" class="lg-drop">
          <div class="ic"><FIcon name="film" /></div>
          <h4>拖入视频或音频</h4>
          <p>{{ dropHint }}</p>
          <button type="button" class="btn" :disabled="!store.canPick" @click="onPick">
            <FIcon name="upload" />选择文件
          </button>
        </div>

        <template v-else>
          <div class="lg-file">
            <div class="th"><FIcon name="film" /></div>
            <div class="meta">
              <b :title="store.fileName">{{ store.fileName }}</b>
              <small>{{ store.phase === 'done' ? '已生成字幕' : store.phase === 'running' ? '识别中' : '待识别' }}</small>
            </div>
            <span v-if="store.phase === 'done'" class="lg-chip">已生成字幕</span>
            <span v-else-if="store.phase === 'running'" class="lg-chip">识别中</span>
          </div>

          <div v-if="store.phase === 'running'" class="lg-prog" role="status">
            <div class="top"><b>正在识别字幕…</b><span>{{ store.progress }}%</span></div>
            <div class="bar"><i :style="{ width: store.progress + '%' }" /></div>
            <div class="mt">也可在任务中心查看</div>
          </div>

          <div v-else-if="store.phase === 'empty'" class="lg-err" role="status">
            <FIcon name="info" />
            <div class="t">
              <b>{{ ASR_EMPTY_TITLE }}</b>
              <p>{{ ASR_EMPTY }}</p>
              <div class="acts">
                <button type="button" class="btn pri" @click="onPick"><FIcon name="upload" />重新选择文件</button>
              </div>
            </div>
          </div>

          <div v-else-if="store.phase === 'fail'" class="lg-err" role="alert">
            <FIcon name="warn" />
            <div class="t">
              <b>{{ ASR_FAIL_TITLE }}</b>
              <p>{{ ASR_FAIL }}</p>
              <div class="acts">
                <button type="button" class="btn pri" @click="store.retryGenerate()"><FIcon name="retry" />重试</button>
              </div>
            </div>
          </div>
        </template>
      </div>

      <div v-if="store.exportError" class="lg-err" role="alert">
        <FIcon name="warn" />
        <div class="t">
          <b>还不能导出</b>
          <p>{{ store.exportError.replace(/^还不能导出。?/, '') || store.exportError }}</p>
        </div>
      </div>

      <CueTimeline
        :cues="store.cues"
        :editing="store.phase === 'done'"
        @update="(id, patch) => store.updateCue(id, patch)"
      />
    </div>

    <aside class="lg-side">
      <div class="lg-panel">
        <div class="lg-ph"><h2>转字幕</h2></div>
        <div class="lg-row">
          <b>识别语言</b>
          <select v-model="store.language" class="lg-select" aria-label="识别语言">
            <option value="auto">自动检测</option>
            <option value="zh">中文</option>
            <option value="en">英文</option>
          </select>
          <small>也可以指定中文、英文等。</small>
        </div>
        <div class="lg-row">
          <b>输出格式</b>
          <div class="lg-seg" role="group" aria-label="输出格式">
            <button type="button" :class="{ on: store.exportFormat === 'srt' }" @click="store.exportFormat = 'srt'">SRT</button>
            <button type="button" :class="{ on: store.exportFormat === 'vtt' }" @click="store.exportFormat = 'vtt'">VTT</button>
          </div>
        </div>
        <div class="lg-row">
          <b>识别档位</b>
          <div class="lg-st">{{ tierLabel }}（{{ sizeText }}）</div>
          <small>默认「标准」。设置里可换「高清」（约 1–1.5 GB）。</small>
        </div>
        <div class="lg-row">
          <b>语音识别组件</b>
          <div class="lg-st">
            <i :class="store.status.state === 'ready' ? 'ok' : 'miss'" />
            <template v-if="store.status.state === 'ready'">已就绪 · {{ tierLabel }}<span class="lg-chip">{{ sizeText }}</span></template>
            <template v-else-if="store.unpublished">尚未发布</template>
            <template v-else-if="store.status.state === 'checking'">检查中…</template>
            <template v-else>未就绪 · {{ tierLabel }}<span class="lg-chip">{{ sizeText }}</span></template>
          </div>
        </div>
        <div class="lg-foot">
          <template v-if="store.phase === 'done'">
            <button type="button" class="btn" :disabled="store.busy || primaryDisabled" @click="store.generate()">
              <FIcon name="caption" />重新生成
            </button>
            <button type="button" class="btn pri" :disabled="!store.canExport" @click="store.exportCues()">
              <FIcon name="download" />导出字幕
            </button>
            <span class="hint">改完字和起止时间后再导出。</span>
          </template>
          <template v-else-if="store.phase === 'running'">
            <button type="button" class="btn" disabled>识别中…</button>
            <span class="hint">任务已加入任务中心</span>
          </template>
          <template v-else>
            <button type="button" class="btn pri" :disabled="primaryDisabled" @click="store.generate()">
              <FIcon name="caption" />生成字幕
            </button>
            <span class="hint">{{ primaryHint }}</span>
          </template>
        </div>
      </div>
    </aside>

    <div v-if="store.exportOk" class="lg-toast ok" role="status">
      <FIcon name="check" />
      <span>{{ store.exportOk }}</span>
    </div>
  </div>
</template>
