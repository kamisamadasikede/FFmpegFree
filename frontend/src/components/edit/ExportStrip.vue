<template>
  <div v-if="s" class="ed-strip" :class="cls" :role="s.kind === 'err' ? 'alert' : 'status'" :aria-live="s.kind === 'run' ? 'off' : undefined">
    <template v-if="s.kind === 'run'">
      <FIcon name="download" :size="16" />
      <b>正在导出</b>
      <span class="nm" :title="s.name">{{ s.name }}</span>
      <span class="sp"></span>
      <div class="bar" role="progressbar" aria-label="导出进度" :aria-valuenow="s.pct" aria-valuemin="0" aria-valuemax="100"><i :style="{ width: s.pct + '%' }"></i></div>
      <span class="num">{{ s.pct }}%</span>
      <span v-if="s.speedText || s.etaText">{{ [s.speedText, s.etaText].filter(Boolean).join(' · ') }}</span>
      <div class="acts">
        <button type="button" class="ed-btn sm" @click="fl.cancelExport()">取消导出</button>
        <button type="button" class="ed-lk" @click="router.push('/tasks')">在任务中心查看</button>
      </div>
      <span class="sr-only" aria-live="polite">{{ spoken }}</span>
    </template>

    <template v-else-if="s.kind === 'ok'">
      <FIcon name="check" :size="16" />
      <b>导出完成</b>
      <span class="nm" :title="s.name">已保存为 {{ s.name }}</span>
      <span class="sp"></span>
      <span v-if="s.meta">{{ s.meta }}</span>
      <div class="acts">
        <button type="button" class="ed-btn sm" @click="fl.reveal(s.outputPath)">在文件夹中显示</button>
        <button type="button" class="x" aria-label="关闭提示" @click="fl.dismiss()"><FIcon name="x" :size="14" /></button>
      </div>
      <ul v-if="s.ignoredTransitions" class="ed-wl" role="list">
        <li role="listitem"><FIcon name="warn" :size="14" />{{ TEXT.transitionIgnored }}</li>
      </ul>
    </template>

    <template v-else-if="s.kind === 'cx'">
      <FIcon name="stop" :size="16" />
      <b>已取消导出</b>
      <span class="nm">没有生成文件</span>
      <span class="sp"></span>
      <div class="acts"><button type="button" class="x" aria-label="关闭提示" @click="fl.dismiss()"><FIcon name="x" :size="14" /></button></div>
    </template>

    <template v-else>
      <FIcon name="warn" :size="16" />
      <div class="m"><b>{{ s.view.title }}</b>{{ s.view.text }}<span class="ed-code">{{ s.view.code }}</span></div>
      <div class="acts">
        <button v-if="s.view.actions.includes('locate')" type="button" class="ed-lk" @click="fl.locate()">定位片段</button>
        <button v-if="s.view.actions.includes('retry')" type="button" class="ed-lk" @click="fl.retry()">重试</button>
        <button v-if="s.view.actions.includes('changeOutput')" type="button" class="ed-lk" @click="fl.changeOutput()">更换输出位置</button>
        <button v-if="s.view.actions.includes('log')" type="button" class="ed-lk" @click="fl.viewLog()">查看日志</button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import { TEXT } from '@/utils/editLogic'
import { useExportFlow } from './exportFlow'

const fl = useExportFlow()
const router = useRouter()
const s = computed(() => fl.strip.value)
const cls = computed(() => {
  const k = s.value?.kind
  return k === 'run' ? 'st-run' : k === 'ok' ? 'st-ok' : k === 'cx' ? 'st-cx' : 'st-err'
})
/** 读屏只在每 10% 和结束时朗读 */
const spoken = computed(() => (s.value && s.value.kind === 'run' ? `正在导出，${Math.floor(s.value.pct / 10) * 10}%` : ''))
</script>
