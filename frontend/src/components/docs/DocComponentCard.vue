<script setup lang="ts">
// 文档组件引导 / 下载 / 准备 / 失败 / 过旧 / Linux / 收起细条（契约 6.12.12，设计 v0.2 §四）
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import MotionCollapse from '@/components/motion/MotionCollapse.vue'
import { useDocComponentStore } from '@/stores/docComponent'
import { docHintSimpleBar, DOC_OUTDATED_BUTTON, DOC_PREPARING } from '@/utils/docV26Text'
import { docV28On } from '@/api/docV26'
import { formatBytes, formatEta } from '@/utils/format'

const comp = useDocComponentStore()
const view = computed(() => comp.guideView)
// v0.28：DOC_V28_BACKEND_READY 打开后横条加上 PDF 那半句；没打开保持原句
const DOC_HINT_SIMPLE_BAR = docHintSimpleBar(docV28On())
const speedText = computed(() => (comp.speedBps > 0 ? `${formatBytes(comp.speedBps)}/s` : ''))
const etaText = computed(() => {
  if (!comp.speedBps) return ''
  const left = Math.max(0, comp.totalBytes - comp.receivedBytes)
  const e = formatEta(left / comp.speedBps)
  return e ? `剩余约 ${e}` : ''
})
const failRetryNote = computed(() => {
  const code = comp.actionError?.code ?? comp.status.error?.code
  const got = comp.status.receivedBytes ?? 0
  if (code === 'DOC_CHECKSUM_FAILED') return `点“重试”会从头重新下载（${comp.downloadSizeShort}）。`
  if (code === 'DOC_COMPONENT_INSTALL_FAILED') return '点“重试”会重新准备，不用重新下载。'
  if (code === 'CONVERT_DISK_FULL') return ''
  if (got > 0 && comp.status.downloadBytes > 0) return `已下载 ${formatBytes(got)} / ${formatBytes(comp.status.downloadBytes)}，点“重试”会接着下。`
  return ''
})
const failTitle = computed(() => {
  const code = comp.actionError?.code ?? comp.status.error?.code
  return code === 'DOC_COMPONENT_INSTALL_FAILED' ? '文档组件没有准备好' : '文档组件没有下载成功'
})
</script>

<template>
  <MotionCollapse>
  <div v-if="view === 'guide'" class="dc-guide" role="region" aria-label="文档组件">
    <div class="gi"><FIcon name="download" /></div>
    <div class="gb">
      <h3>下载文档组件，转换时保留图片和排版</h3>
      <p>转换 Word、Excel、PowerPoint 等文件需要文档组件。<template v-if="comp.sizeText">{{ comp.sizeText }}</template></p>
      <small>在这之前，{{ DOC_HINT_SIMPLE_BAR }}。</small>
      <div class="acts">
        <button type="button" class="btn pri" :disabled="comp.busy" @click="comp.install()"><FIcon name="download" />下载文档组件</button>
        <button type="button" class="btn" @click="comp.dismissGuide()">先用简易转换</button>
      </div>
    </div>
  </div>

  <div v-else-if="view === 'outdated'" class="dc-guide" :class="{ lnx: comp.isLinux }" role="region" aria-label="文档组件">
    <div class="gi"><FIcon name="warn" /></div>
    <div class="gb">
      <h3>文档组件版本太旧</h3>
      <p>{{ comp.outdatedText }}</p>
      <span v-if="!comp.isLinux && comp.sizeText" class="dc-size">{{ comp.sizeText }}</span>
      <small>在这之前，{{ DOC_HINT_SIMPLE_BAR }}。</small>
      <div class="acts">
        <button v-if="!comp.isLinux" type="button" class="btn pri" :disabled="comp.busy" @click="comp.install()"><FIcon name="download" />{{ DOC_OUTDATED_BUTTON }}</button>
        <button v-else type="button" class="btn" :disabled="comp.busy" @click="comp.recheck()"><FIcon name="refresh" />重新检测</button>
        <button type="button" class="btn" @click="comp.dismissGuide()">先用简易转换</button>
      </div>
    </div>
  </div>

  <div v-else-if="view === 'linux'" class="dc-guide lnx" role="region" aria-label="文档组件">
    <div class="gi"><FIcon name="warn" /></div>
    <div class="gb">
      <h3>需要文档组件</h3>
      <p>{{ comp.linuxMissingText }}</p>
      <small>在这之前，{{ DOC_HINT_SIMPLE_BAR }}。</small>
      <div class="acts">
        <button type="button" class="btn" :disabled="comp.busy" @click="comp.recheck()"><FIcon name="refresh" />重新检测</button>
        <button type="button" class="btn" @click="comp.dismissGuide()">先用简易转换</button>
      </div>
    </div>
  </div>

  <div v-else-if="view === 'downloading'" class="dc-guide" role="region" aria-label="文档组件">
    <div class="gi"><FIcon name="download" /></div>
    <div class="gb">
      <h3>正在下载文档组件</h3>
      <div class="dpg">
        <div class="top"><span>下载中</span><b>{{ comp.pct }}%</b></div>
        <div class="bar" role="progressbar" aria-label="下载进度" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="comp.pct"><i :style="{ width: comp.pct + '%' }" /></div>
        <div class="mt">
          <span>{{ formatBytes(comp.receivedBytes) }} / {{ formatBytes(comp.totalBytes) }}</span>
          <span v-if="speedText">{{ speedText }}<template v-if="etaText"> · {{ etaText }}</template></span>
        </div>
      </div>
      <small>下载完成后自动启用，不用重启。期间可以继续用简易转换。</small>
      <div class="acts"><button type="button" class="btn" :disabled="comp.busy" @click="comp.cancel()">取消下载</button></div>
    </div>
  </div>

  <div v-else-if="view === 'preparing'" class="dc-guide" role="region" aria-label="文档组件">
    <div class="gi"><FIcon name="download" /></div>
    <div class="gb">
      <h3>{{ DOC_PREPARING }}</h3>
      <div class="dpg">
        <div class="bar ind" role="progressbar" :aria-label="DOC_PREPARING" aria-busy="true"><i /></div>
        <div class="mt"><span>大约需要一分钟</span></div>
      </div>
      <!-- 设计 v0.2：准备这一步不给取消（契约允许 CancelDocComponentInstall 在 preparing 时调用，界面不暴露） -->
      <small>准备好后自动启用。这一步不能取消，期间可以继续用简易转换。</small>
    </div>
  </div>

  <div v-else-if="view === 'failed'" class="dc-guide fail" role="region" aria-label="文档组件">
    <div class="gi"><FIcon name="warn" /></div>
    <div class="gb">
      <h3>{{ failTitle }}</h3>
      <div class="dfail" role="alert"><FIcon name="warn" /><span>{{ comp.errorText }}</span></div>
      <div v-if="failRetryNote" class="dc-done">{{ failRetryNote }}</div>
      <small>在这之前，{{ DOC_HINT_SIMPLE_BAR }}。</small>
      <div class="acts">
        <button v-if="!comp.isLinux" type="button" class="btn pri" :disabled="comp.busy" @click="comp.install()"><FIcon name="retry" />重试</button>
        <button type="button" class="btn" @click="comp.dismissGuide()">先用简易转换</button>
      </div>
    </div>
  </div>

  <div v-else-if="view === 'slim'" class="dc-slim" role="status">
    <FIcon name="warn" />
    <span class="tx" :title="`文档组件未就绪。${DOC_HINT_SIMPLE_BAR}。`">文档组件未就绪。<span class="only1280">{{ DOC_HINT_SIMPLE_BAR }}。</span></span>
    <span class="sp" />
    <button type="button" class="lk" @click="comp.isLinux ? comp.reopenGuide() : comp.install()">{{ comp.isLinux ? '查看说明' : '下载文档组件' }}</button>
  </div>
  </MotionCollapse>
</template>
