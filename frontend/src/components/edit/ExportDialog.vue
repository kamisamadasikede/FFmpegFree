<template>
  <div class="ed-mask" @pointerdown.self="fl.closeDialog()">
    <div ref="box" class="ed-dlg" style="width: 480px; padding: 24px" role="dialog" aria-modal="true" aria-labelledby="edDT">
      <div class="ed-dh">
        <h3 id="edDT">导出视频</h3>
        <button type="button" class="ed-x" aria-label="关闭" style="border: 0; background: none; width: 28px; height: 28px; border-radius: 6px; display: grid; place-items: center; color: var(--ff-text-2); cursor: pointer" @click="fl.closeDialog()">
          <FIcon name="x" :size="16" />
        </button>
      </div>

      <ErrorLine v-if="fl.validateError.value" :code="fl.validateError.value.code" :title="fl.validateError.value.title" :description="fl.validateError.value.text" :show-log="false" announce compact />
      <ErrorLine v-if="fl.submitError.value" :code="fl.submitError.value.code" :title="fl.submitError.value.title" :description="fl.submitError.value.text" :show-log="false" announce compact />

      <div class="ed-df">
        <label for="ed-name-in"><span>文件名</span><span class="ed-cnt" :class="{ bad: tooLong }">{{ len }}/100</span></label>
        <div class="ed-in" :class="{ bad: !!fl.nameErr.value }">
          <input id="ed-name-in" v-model="fl.form.name" data-autofocus class="ed-plain" type="text" aria-label="文件名" :aria-invalid="fl.nameErr.value ? 'true' : undefined" :aria-describedby="fl.nameErr.value ? 'edErr' : undefined" autocomplete="off" spellcheck="false" />
          <span class="suf">.{{ fl.form.format }}</span>
        </div>
        <div v-if="fl.nameErr.value" id="edErr" class="ed-ferr" role="alert"><FIcon name="warn" :size="14" /><span>{{ fl.nameErr.value.text }}</span></div>
        <div v-else-if="illegal" class="ed-hint2" role="status" style="margin-top: 4px">导出时会去掉不能用的字符（\ / : * ? " &lt; &gt; |）。</div>
        <div v-else class="help">可以用中文、日文、韩文。不能含 \ / : * ? " &lt; &gt; |，最多 100 个字符。</div>
      </div>

      <div class="ed-df">
        <label><span>保存位置</span></label>
        <div class="ed-in pathbox" :class="{ bad: !!fl.nameErr.value?.path }">
          <FIcon name="folder" :size="14" />
          <span class="p" :title="fl.form.dir" :style="{ direction: fl.nameErr.value?.path ? 'rtl' : 'ltr', textAlign: 'left' }">{{ fl.form.dir || '与第一个视频素材相同的文件夹' }}</span>
          <button type="button" class="ed-lk" @click="fl.pickDir()">更改</button>
        </div>
        <div class="help">同名文件不会被覆盖，会自动加 (1)。</div>
      </div>

      <div class="ed-row3">
        <div class="ed-df">
          <label for="ed-fmt"><span>格式</span></label>
          <select id="ed-fmt" v-model="fl.form.format" class="ed-in">
            <option v-for="f in FORMATS" :key="f.v" :value="f.v">{{ f.t }}</option>
          </select>
        </div>
        <div class="ed-df">
          <label><span>分辨率（宽 × 高）</span></label>
          <div class="ed-wh">
            <input v-model.number="fl.form.width" class="ed-in mono" type="number" min="16" max="7680" aria-label="宽" :class="{ bad: badW }" @change="clampRes" />×
            <input v-model.number="fl.form.height" class="ed-in mono" type="number" min="16" max="4320" aria-label="高" :class="{ bad: badH }" @change="clampRes" />
          </div>
        </div>
        <div class="ed-df">
          <label for="ed-fps"><span>帧率</span></label>
          <div class="ed-in mono"><input id="ed-fps" v-model.number="fl.form.fps" class="ed-plain" type="number" min="1" max="120" aria-label="帧率" :class="{ bad: badF }" @change="clampRes" /><span class="suf">fps</span></div>
        </div>
      </div>
      <div class="ed-hint2" style="margin-top: -8px">
        宽 16~7680，高 16~4320，帧率不超过 120。<template v-if="evenNote"> 实际输出 {{ fl.actualSize.value.w }}×{{ fl.actualSize.value.h }}（向下取偶数）。</template>
      </div>

      <div v-if="fl.plan.value" class="ed-sum">
        <span><b>{{ fl.plan.value.clipCount }}</b> 个片段</span>
        <span>时长 <b>{{ formatClock(fl.plan.value.durationSec) }}</b></span>
        <span>{{ fl.form.format === 'webm' ? 'VP9 + Opus' : 'H.264 + AAC' }}</span>
      </div>
      <div v-else-if="fl.validating.value" class="ed-sum" role="status"><span>正在检查工程…</span></div>

      <div v-if="fl.plan.value && fl.noAudio.value" class="ed-note warn" role="status"><FIcon name="warn" :size="16" /><span>{{ TEXT.noAudio }}</span></div>
      <div v-if="fl.plan.value && fl.shownWarnings.value.length" class="ed-note info" role="status">
        <FIcon name="info" :size="16" />
        <span>
          有 {{ fl.shownWarnings.value.length }} 个提示：
          <template v-for="(w, i) in fl.shownWarnings.value.slice(0, 3)" :key="i">{{ i ? ' ' : '' }}{{ text(w) }}</template>
          <template v-if="fl.shownWarnings.value.length > 3"> 另有 {{ fl.shownWarnings.value.length - 3 }} 个提示。</template>
        </span>
      </div>

      <div class="ed-dfoot">
        <button type="button" class="ed-btn lg" @click="fl.closeDialog()">取消</button>
        <button
          type="button"
          class="ed-btn pri lg tp-r tp-up"
          :aria-disabled="fl.canStart.value ? undefined : 'true'"
          :data-tip="startTip"
          @click="fl.start()"
        >开始导出</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import ErrorLine from '@/components/common/ErrorLine.vue'
import { TEXT, formatClock, nameLength, warningText, clipNumberLabel, type EditWarning } from '@/utils/editLogic'
import { useEditor } from './editor'
import { useExportFlow } from './exportFlow'
import { useDialog } from './useDialog'

const ed = useEditor()
const fl = useExportFlow()
const box = ref<HTMLElement | null>(null)
const FORMATS = [
  { v: 'mp4', t: 'MP4' },
  { v: 'mov', t: 'MOV' },
  { v: 'mkv', t: 'MKV' },
  { v: 'webm', t: 'WebM' },
] as const

const len = computed(() => nameLength(fl.form.name))
const tooLong = computed(() => len.value > 100)
const illegal = computed(() => /[\\/:*?"<>|]/.test(fl.form.name))
const num = (v: unknown) => Number(v)
const badW = computed(() => !(num(fl.form.width) >= 16 && num(fl.form.width) <= 7680))
const badH = computed(() => !(num(fl.form.height) >= 16 && num(fl.form.height) <= 4320))
const badF = computed(() => !(num(fl.form.fps) > 0 && num(fl.form.fps) <= 120))
const evenNote = computed(() => !badW.value && !badH.value && (num(fl.form.width) % 2 !== 0 || num(fl.form.height) % 2 !== 0))
const startTip = computed(() => (fl.nameErr.value ? TEXT.nameTooLongTip : undefined))
function clampRes() {
  fl.form.width = Math.min(7680, Math.max(16, Math.round(num(fl.form.width)) || 1920))
  fl.form.height = Math.min(4320, Math.max(16, Math.round(num(fl.form.height)) || 1080))
  fl.form.fps = Math.min(120, Math.max(1, num(fl.form.fps) || 30))
}
function text(w: EditWarning) {
  return warningText(w, w.clipId ? clipNumberLabel(ed.project, w.clipId) : '')
}
useDialog(box, () => fl.closeDialog(), () => document.getElementById('ed-export-btn'))
</script>

<style scoped>
.ed-plain {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: none;
  background: none;
  font: inherit;
  color: inherit;
  padding: 0;
  text-overflow: ellipsis;
}
.ed-in:focus-within {
  border-color: var(--ff-primary);
}
.ed-in.bad:focus-within {
  border-color: var(--ff-danger);
}
input.ed-in.bad,
.ed-in .ed-plain.bad {
  color: var(--ff-danger-text);
}
input[type='number'] {
  -moz-appearance: textfield;
  appearance: textfield;
}
input[type='number']::-webkit-outer-spin-button,
input[type='number']::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
.ed-df {
  min-width: 0;
}
.ed-dh {
  margin-bottom: 0;
}
</style>
