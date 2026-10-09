<script setup lang="ts">
// 文档转换页 v2（契约 v0.26 §6.12.9~6.12.22；设计 文档页-v2-设计说明 v0.2）。
// 左栏：文档组件引导 + 源文件和记录；右栏：按交集分组的格式。
import { computed, defineAsyncComponent, onActivated, onDeactivated, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import DocComponentCard from '@/components/docs/DocComponentCard.vue'
import DocFormatPanel from '@/components/docs/DocFormatPanel.vue'
import DocSourceRow from '@/components/docs/DocSourceRow.vue'
import { useNarrow } from '@/components/convert/useNarrow'
import '@/components/convert/convert-v2.css'
import '@/components/docs/doc-v2.css'
import { dropHandlers } from '@/stores/docs'
import { useDocConvertStore } from '@/stores/docConvert'
import { usePreviewStore } from '@/stores/docPreview'

defineOptions({ name: 'DocConvertPage' })

const DocPreviewModal = defineAsyncComponent(() => import('@/components/docs/preview/DocPreviewModal.vue'))
const dc = useDocConvertStore()
const pst = usePreviewStore()
const router = useRouter()
const narrow = useNarrow()
const dropStyle = { '--wails-drop-target': 'drop' } as Record<string, string>
import { docV28On } from '@/api/docV26'
// v0.28：PDF 输入打开后（DOC_V28_BACKEND_READY 或纯浏览器模拟）提示里加 PDF
const SUP = docV28On() ? '支持 Word、Excel、PowerPoint、PDF、文本、网页、Markdown' : '支持 Word、Excel、PowerPoint、文本、网页、Markdown'
const DROP_TITLE = docV28On() ? '拖入文档、表格、演示或 PDF 文件，或点击选择' : '拖入文档、表格或演示文件，或点击选择'

const countText = computed(() => {
  const n = dc.rows.length
  if (!n) return ''
  const rec = dc.rows.reduce((a, r) => a + r.records.length, 0)
  return `${n} 个文件 · ${rec} 条记录`
})
const showTotal = computed(() => dc.total.running + dc.total.queued > 0)
const totalPct = computed(() => (dc.total.inRound ? Math.round((dc.total.roundDone / dc.total.inRound) * 100) : 0))

watch(() => dc.toast, (t) => {
  if (t) ElMessage({ message: t.text, type: t.warn ? 'warning' : 'info', duration: t.warn ? 8000 : 4000 })
})
// 组件就绪 / 格式表换了以后：保持当前目标可用，否则回到 PDF
watch(() => dc.tiles, () => dc.ensureTarget())

const onDrop = (paths: string[]) => void dc.addPaths(paths)
onMounted(() => void dc.init())
onActivated(() => (dropHandlers.office = onDrop))
onDeactivated(() => {
  if (pst.open) void pst.close()
  if (dropHandlers.office === onDrop) dropHandlers.office = undefined
})
dropHandlers.office = onDrop
</script>

<template>
  <div class="cv2 dc-page" :class="{ w1024: narrow }">
    <div class="cv">
      <section class="panel cv-left" aria-label="转换记录" :style="dropStyle">
        <div class="cv-ph">
          <h2>转换记录</h2>
          <span class="cnt">{{ countText }}</span>
          <span class="sp" />
          <button type="button" class="btn" @click="dc.chooseFiles()"><FIcon name="plus" />添加文件</button>
        </div>

        <DocComponentCard />

        <!-- 设计 v0.2 §二.8：只写「正在转换 n 项 · 排队 k 项」和已完成计数，不写并发上限 -->
        <div v-if="showTotal" class="cv-total" role="status">
          <FIcon name="convert" />
          <b v-if="dc.total.running">正在转换 {{ dc.total.running }} 项</b><b v-else>排队中</b>
          <span v-if="dc.total.queued">排队 {{ dc.total.queued }} 项</span>
          <div class="bar dc-tbar"><i :style="{ width: totalPct + '%' }" /></div>
          <span class="sp" />
          <span class="dc-tdone">已完成 <b>{{ dc.total.roundDone }}</b> / {{ dc.total.inRound }}</span>
          <button type="button" class="lk" @click="router.push('/tasks')">任务中心</button>
        </div>
        <div v-else-if="dc.roundBanner" class="cv-total" :class="{ 't-ok': !dc.roundBanner.fail }" role="status">
          <FIcon :name="dc.roundBanner.fail ? 'warn' : 'check'" />
          <b v-if="dc.roundBanner.fail">本轮完成 {{ dc.roundBanner.ok }} 项，失败 {{ dc.roundBanner.fail }} 项</b>
          <b v-else>本轮 {{ dc.roundBanner.ok }} 项全部完成</b>
          <span v-if="!dc.roundBanner.fail" class="hide1024">结果已保存到输出文件夹</span>
          <span class="sp" />
          <button type="button" class="x" aria-label="关闭" title="关闭" @click="dc.roundBanner = null"><FIcon name="x" /></button>
        </div>

        <div v-if="dc.loaded && !dc.rows.length" class="cv-hero">
          <div class="ic"><FIcon name="upload" /></div>
          <h3>{{ DROP_TITLE }}</h3>
          <p>添加后勾选文件，在右侧选择格式，点“转换”。每次转换的结果都会挂在源文件下面，重启后仍在。</p>
          <small>{{ SUP }}，一次最多 50 个</small>
          <div class="acts"><button type="button" class="btn pri" @click="dc.chooseFiles()"><FIcon name="plus" />添加文件</button></div>
        </div>
        <div v-else class="cv-list" style="overflow: auto">
          <button type="button" class="cv-drop" @click="dc.chooseFiles()">
            <span class="ic"><FIcon name="upload" /></span><b>拖入更多文件，或点击选择</b><span class="cv-ds" :title="SUP">{{ SUP }}</span>
          </button>
          <DocSourceRow v-for="r in dc.rows" :key="r.src.sourceId" :row="r" />
        </div>
      </section>

      <DocFormatPanel />
    </div>
    <DocPreviewModal v-if="pst.open" />
  </div>
</template>
