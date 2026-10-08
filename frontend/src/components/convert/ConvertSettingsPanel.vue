<script setup lang="ts">
// 转换页右栏“转换设置”（设计 §二 右栏）：已选 / 预设（视频、音频两组）/ 保存到 / “转换 N 个文件”。
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import ErrorLine from '@/components/common/ErrorLine.vue'
import { useConvertRecordsStore } from '@/stores/convertRecords'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { splitPresetName } from '@/utils/convertText'
import type { PresetItem } from '@/api/convert'

const cv = useConvertRecordsStore()
const ffmpeg = useFFmpegStore()

const nSel = computed(() => cv.selectedRows.length)
const n = computed(() => cv.submittableRows.length)
const presetSub = (p: PresetItem) => splitPresetName(p.name).sub || p.options.container.toUpperCase()
const ffmpegText = computed(() => {
  if (ffmpeg.ready) return ''
  const s = ffmpeg.status.state
  return s === 'checking' ? '正在检测转换组件…' : s === 'installing' ? '转换组件正在安装，装好后就可以转换。' : '需要先安装转换组件才能转换。'
})
const ffmpegMissing = computed(() => !ffmpeg.ready && ffmpeg.status.state !== 'checking' && ffmpeg.status.state !== 'installing')
const disabled = computed(() => !!cv.startBlock)
const label = computed(() => (cv.submitting ? '正在提交…' : n.value > 0 && !cv.startBlock ? `转换 ${n.value} 个文件` : '转换'))
/** 按钮下的说明（§二 右栏 4 / §五 按钮说明） */
const hint = computed<{ text: string; warn?: boolean }>(() => {
  switch (cv.startBlock) {
    case 'submitting': return { text: '每次转换都会新增一条记录' }
    case 'ffmpeg': return { text: ffmpegMissing.value ? '需要先安装转换组件' : ffmpegText.value }
    case 'preset': return { text: cv.presetsError ? '没有加载到输出预设' : '正在加载预设…' }
    case 'empty': return { text: '先添加文件' }
    case 'none': return { text: '勾选文件后才能转换' }
    case 'probing': return { text: '正在读取文件信息…' }
    case 'conflict': return { text: '选中的文件都不能用当前预设', warn: true }
  }
  if (cv.blockedCount) return { text: `将跳过 ${cv.blockedCount} 个不兼容的文件`, warn: true }
  if (n.value === 1 && cv.outputName) return { text: `将保存为“${cv.outputName}”` }
  return { text: '每次转换都会新增一条记录' }
})
</script>
<template>
  <section class="panel cv-right" aria-label="转换设置">
    <div class="cv-ph">
      <h2>转换设置</h2>
      <span class="sp" />
      <span class="cv-selh">
        <template v-if="nSel">已选 <b>{{ nSel }}</b> 个<button type="button" class="ff-link" @click="cv.clearSelection()">取消</button></template>
        <span v-else class="none">未选择文件</span>
      </span>
    </div>
    <div class="cv-rp">
      <div class="seg" role="tablist" aria-label="预设分组">
        <button type="button" role="tab" :class="{ on: cv.tab === 'video' }" :aria-selected="cv.tab === 'video'" @click="cv.setTab('video')">视频</button>
        <button type="button" role="tab" :class="{ on: cv.tab === 'audio' }" :aria-selected="cv.tab === 'audio'" @click="cv.setTab('audio')">音频</button>
      </div>
      <div v-if="cv.presetsError" class="cv-gate" role="status"><FIcon name="warn" /><span>没有加载到输出预设。<button type="button" class="ff-link" @click="cv.loadPresets()">重试</button></span></div>
      <div class="cv-presets" :class="{ dim: !nSel }" role="radiogroup" aria-label="输出格式">
        <button
          v-for="p in cv.shownPresets"
          :key="p.id"
          type="button"
          class="preset"
          :class="{ on: p.id === cv.selectedPresetId }"
          role="radio"
          :aria-checked="p.id === cv.selectedPresetId"
          @click="cv.selectedPresetId = p.id"
        >
          <b :title="cv.presetTitle(p)">{{ cv.presetTitle(p) }}</b>
          <small :title="presetSub(p)">{{ presetSub(p) }}</small>
        </button>
      </div>
      <div v-if="ffmpegText" class="cv-gate" :class="{ info: !ffmpegMissing }" role="status">
        <FIcon :name="ffmpegMissing ? 'warn' : 'refresh'" />
        <span>{{ ffmpegText }}<button v-if="ffmpegMissing" type="button" class="ff-link" @click="ffmpeg.dialogOpen = true">安装转换组件</button></span>
      </div>
      <div class="cv-save">
        <label for="cv-outdir">保存到</label>
        <div class="row2">
          <div id="cv-outdir" class="input" :title="cv.effectiveOutputDir || '各自源文件所在的文件夹'">{{ cv.effectiveOutputDir || '各自源文件所在的文件夹' }}</div>
          <button type="button" class="btn" style="padding: 0 7px" aria-label="选择文件夹" title="选择文件夹" @click="cv.chooseOutputDir()"><FIcon name="folder" /></button>
        </div>
        <div class="cv-hint">同名文件自动加序号，不会覆盖。<button v-if="cv.outputOverride" type="button" class="ff-link" @click="cv.outputOverride = ''">恢复默认</button></div>
      </div>
      <ErrorLine v-if="cv.submitError" compact :code="cv.submitError.code" :message="cv.submitError.message" :detail="cv.submitError.detail" :show-log="false" fallback-title="无法开始转换" />
    </div>
    <div class="cv-foot">
      <button
        type="button"
        class="btn pri lg"
        :aria-disabled="disabled"
        aria-describedby="cv-foot-hint"
        :aria-busy="cv.submitting"
        @click="!disabled && cv.submit()"
      ><FIcon name="convert" />{{ label }}</button>
      <small id="cv-foot-hint" :class="{ warn: hint.warn }" :title="hint.text">{{ hint.text }}</small>
    </div>
  </section>
</template>
