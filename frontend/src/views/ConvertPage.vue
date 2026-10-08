<script setup lang="ts">
// 格式转换页 v2（转换记录）：左栏源文件 + 每次转换的记录（虚拟滚动、按源文件行分页），右栏转换设置。
// 设计：转换页-v2-设计说明-v0.1.md（定稿 + v0.23.1 + 第 2 行规则）；数据：stores/convertRecords.ts（契约 v0.23 §6.14，后端没合入前走模拟）。
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import ErrorLine from '@/components/common/ErrorLine.vue'
import VirtualList from '@/components/common/VirtualList.vue'
import ConvertSourceRow from '@/components/convert/ConvertSourceRow.vue'
import ConvertSettingsPanel from '@/components/convert/ConvertSettingsPanel.vue'
import ConvertDeleteDialog from '@/components/convert/ConvertDeleteDialog.vue'
import ConvertPreviewDialog, { type PreviewTarget } from '@/components/convert/ConvertPreviewDialog.vue'
import { useNarrow } from '@/components/convert/useNarrow'
import '@/components/convert/convert-v2.css'
import { onFilesDropped } from '@/api/fileDrop'
import { toAppError } from '@/api/call'
import { pickDirectory } from '@/api/system'
import { simParam } from '@/api/sim'
import { convertV2IsReal } from '@/api/convertRecords'
import { hasWailsBackend } from '@/services/wails'
import { useConvertRecordsStore, type DeleteAsk, type ParentView } from '@/stores/convertRecords'
import { useTaskStore } from '@/stores/tasks'
import { deleteResultText, roughEta } from '@/utils/convertText'

const cv = useConvertRecordsStore()
const tasks = useTaskStore()
const router = useRouter()
const route = useRoute()
const narrow = useNarrow()

/** Wails 的拖放目标：整个左栏都能拖入（设计 §二 左栏 3） */
const dropStyle = { '--wails-drop-target': 'drop' } as Record<string, string>

// ---- 栏头 ----
const hasRows = computed(() => cv.sourceCount > 0)
const countText = computed(() => (hasRows.value ? `${cv.sourceCount} 个文件 · ${cv.recordCount} 条记录` : ''))
const FILTERS = [
  { key: 'all', label: '全部' },
  { key: 'active', label: '进行中' },
  { key: 'failed', label: '失败' },
] as const
const kw = ref('')
let kwTimer: ReturnType<typeof setTimeout> | undefined
watch(kw, (k) => {
  clearTimeout(kwTimer)
  kwTimer = setTimeout(() => void cv.search(k.slice(0, 100)), k.trim() ? 300 : 0) // 输入停顿 300ms 后查（§四 11）
})
watch(() => cv.keyword, (k) => {
  if (k !== kw.value) kw.value = k
})

// ---- 总进度 / 本轮完成 ----
const total = computed(() => cv.total)
const showTotal = computed(() => total.value.running + total.value.queued > 0)

// ---- 列表（今天 / 更早分组拍平给虚拟列表） ----
type Item = { kind: 'grp'; key: string; label: string; first: boolean } | { kind: 'row'; key: string; p: ParentView }
const items = computed<Item[]>(() => {
  const out: Item[] = []
  cv.groups.forEach((g, i) => {
    out.push({ kind: 'grp', key: `grp:${g.key}`, label: g.label, first: i === 0 })
    for (const p of g.parents) out.push({ kind: 'row', key: `row:${p.src.sourceId}`, p })
  })
  return out
})
const itemKey = (it: Item) => it.key
function estimate(it: Item): number {
  if (it.kind === 'grp') return 32
  const p = it.p
  let h = 56
  if (p.conflict) h += 52
  if (p.open && p.kids.length) h += p.kids.reduce((n, k) => n + (k.status === 'failed' ? 150 : 84), 4) + (p.moreCount ? 30 : 0)
  else if (!p.src.recordCount && !p.conflict) h += 30
  return h
}
/** VirtualList 是泛型组件，InstanceType 取不到；只用到它暴露的这三个 */
const vl = ref<{ scrollToKey: (key: string, align?: 'start' | 'nearest') => Promise<void>; scrollToTop: () => void; el: HTMLElement | null } | null>(null)
const noResult = computed(() => !!cv.searchHits && !cv.searching && !cv.parents.length)
const filterEmpty = computed(() => !cv.searchHits && cv.filter !== 'all' && hasRows.value && !cv.parents.length)
watch(() => cv.addedTick, () => vl.value?.scrollToTop())

// ---- 预览 / 删除 / 日志 ----
const preview = ref<PreviewTarget | null>(null)
function onPreview(kind: 'source' | 'record', id: string) {
  preview.value = { kind, id }
}
const delAsk = ref<DeleteAsk | null>(null)
const delChecked = ref(false)
const deleting = ref(false)
function onRemove(kind: 'source' | 'record', id: string) {
  delChecked.value = false
  delAsk.value = cv.deleteAsk(kind, id)
}
async function onConfirmDelete(withOutput: boolean) {
  const a = delAsk.value
  if (!a || deleting.value) return
  deleting.value = true
  try {
    const r = await cv.confirmDelete(a, withOutput)
    delAsk.value = null
    const text = a.kind === 'source' && a.count === 0 && !r.failures.length ? `已移除“${a.name}”` : deleteResultText(r)
    if (r.failures.length) ElMessage.warning(text)
    else ElMessage.success(text)
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  } finally {
    deleting.value = false
  }
}
const logId = ref('')
const logText = ref('')
const logLoading = ref(false)
async function onLog(id: string) {
  logId.value = id
  logText.value = ''
  logLoading.value = true
  try {
    logText.value = (await tasks.getLog(id, 200)) || '（没有日志）'
  } catch (e) {
    logText.value = toAppError(e).message
  } finally {
    logLoading.value = false
  }
}
function closeLog() {
  logId.value = ''
}
async function onChangeOutput(id: string) {
  const dir = hasWailsBackend() ? await pickDirectory('选择这次转换的输出文件夹').catch(() => '') : 'D:\\Videos\\FFmpegFree'
  if (dir) await cv.resubmitTo(id, dir)
}

// ---- 提示 ----
watch(() => cv.toast, (t) => {
  if (t) ElMessage.info(t.text)
})

// ---- 任务中心“在转换页查看”（?record=&source=）：定位、滚到、高亮 ----
const focusId = ref('')
let focusTimer: ReturnType<typeof setTimeout> | undefined
async function locateFromRoute() {
  const id = typeof route.query.record === 'string' ? route.query.record : ''
  if (!id) return
  const sid = typeof route.query.source === 'string' ? route.query.source : undefined
  void router.replace({ path: '/', query: {} })
  await cv.locate(id, sid)
}
watch(() => cv.focus, async (f) => {
  if (!f) return
  focusId.value = ''
  await nextTick()
  await vl.value?.scrollToKey(`row:${f.sourceId}`)
  await nextTick()
  focusId.value = f.id
  requestAnimationFrame(() => {
    const el = vl.value?.el?.querySelector(`[data-kid="${CSS.escape(f.id)}"]`)
    el?.scrollIntoView({ block: 'nearest' })
  })
  clearTimeout(focusTimer)
  focusTimer = setTimeout(() => (focusId.value = ''), 2600)
})
watch(() => route.query.record, (r) => {
  if (r) void locateFromRoute()
})

// ---- 模拟场景的弹窗（走查 / 截图用：?dlg=video|result|audio|unplayable|delete|delete-kid） ----
function openMockDialog() {
  if (convertV2IsReal()) return
  switch (simParam('dlg')) {
    case 'video': return onPreview('source', 'mock-src-launch')
    case 'result': return onPreview('record', 'simcv-launchMp4')
    case 'audio': return onPreview('record', 'simcv-podMp3')
    case 'unplayable': return onPreview('source', 'mock-src-demo')
    case 'delete': return onRemove('source', 'mock-src-launch')
    case 'delete-kid':
      delAsk.value = cv.deleteAsk('record', 'simcv-ivNew')
      delChecked.value = true
  }
}

// ---- 生命周期 ----
let offDrop: () => void = () => {}
onMounted(async () => {
  offDrop = onFilesDropped((paths) => void cv.addPaths(paths))
  await cv.init()
  if (route.query.record) await locateFromRoute()
  else openMockDialog()
})
onUnmounted(() => {
  offDrop()
  clearTimeout(kwTimer)
  clearTimeout(focusTimer)
})
</script>
<template>
  <div class="cv2" :class="{ w1024: narrow }">
    <div class="cv">
      <section class="panel cv-left" aria-label="转换记录" :style="dropStyle">
        <div class="cv-ph">
          <h2>转换记录</h2>
          <span class="cnt">{{ countText }}</span>
          <span class="sp" />
          <div v-if="hasRows || cv.searchHits" class="cv-filter" role="tablist" aria-label="筛选">
            <button v-for="f in FILTERS" :key="f.key" type="button" role="tab" :class="{ on: cv.filter === f.key }" :aria-selected="cv.filter === f.key" @click="cv.setFilter(f.key)">{{ f.label }}</button>
          </div>
          <label v-if="hasRows || cv.searchHits" class="cv-search">
            <FIcon name="search" />
            <input v-model="kw" type="search" placeholder="搜索文件名" aria-label="搜索文件名" maxlength="100" />
            <button v-if="kw" type="button" class="clr" aria-label="清空搜索" @click="kw = ''"><FIcon name="x" :size="12" /></button>
          </label>
          <button type="button" class="btn" @click="cv.chooseFiles()"><FIcon name="plus" />添加文件</button>
        </div>

        <div v-if="showTotal" class="cv-total" role="status">
          <FIcon name="convert" />
          <b v-if="total.running">正在转换 {{ total.running }} 项</b><b v-else>排队 {{ total.queued }} 项</b>
          <span v-if="total.running && total.queued" class="hide1024">排队 {{ total.queued }} 项</span>
          <div class="bar" role="progressbar" aria-label="总进度" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="total.pct"><i class="run" :style="{ width: total.pct + '%' }" /></div>
          <span class="num">{{ total.pct }}%</span>
          <span v-if="roughEta(total.etaSec)">剩余约 {{ roughEta(total.etaSec) }}</span>
          <button type="button" class="lk" @click="router.push('/tasks')">任务中心</button>
        </div>
        <div v-else-if="cv.roundBanner" class="cv-total t-ok" role="status">
          <FIcon name="check" />
          <b v-if="cv.roundBanner.fail">本轮完成 {{ cv.roundBanner.ok }} 项，失败 {{ cv.roundBanner.fail }} 项</b>
          <b v-else>本轮 {{ cv.roundBanner.ok }} 项全部完成</b>
          <span v-if="cv.roundBanner.ok" class="hide1024">结果已保存在各自的源文件下</span>
          <span class="sp" />
          <button type="button" class="x" aria-label="关闭" title="关闭" @click="cv.closeBanner()"><FIcon name="x" /></button>
        </div>
        <div v-if="cv.notice" class="cv-notice" role="status"><FIcon name="warn" /><span>{{ cv.notice }}</span><button type="button" aria-label="关闭提示" @click="cv.notice = ''"><FIcon name="x" :size="14" /></button></div>

        <div v-if="cv.loadError && !cv.loaded" class="cv-list" style="padding: 16px">
          <ErrorLine :code="cv.loadError.code" :message="cv.loadError.message" :detail="cv.loadError.detail" :show-log="false" fallback-title="没有读到转换记录" show-retry @retry="cv.reload()" />
        </div>
        <div v-else-if="cv.loaded && !hasRows && !cv.searchHits" class="cv-hero">
          <div class="ic"><FIcon name="upload" /></div>
          <h3>拖入视频或音频文件，或点击选择</h3>
          <p>添加后勾选文件，在右侧选择格式，点“转换”。每次转换的结果都会挂在源文件下面，重启后仍在。</p>
          <small>支持常见视频、音频格式，一次最多 50 个</small>
          <div class="acts"><button type="button" class="btn pri" @click="cv.chooseFiles()"><FIcon name="plus" />添加文件</button></div>
        </div>
        <div v-else class="cv-list">
          <VirtualList ref="vl" :items="items" :item-key="itemKey" :estimate="estimate">
            <template #header>
              <button type="button" class="cv-drop" @click="cv.chooseFiles()">
                <span class="ic"><FIcon name="upload" /></span><b>拖入更多文件，或点击选择</b><span class="cv-ds">· 支持常见视频、音频格式</span>
              </button>
            </template>
            <template #default="{ item }">
              <div v-if="item.kind === 'grp'" class="cv-grp" :class="{ first: item.first }">{{ item.label }}<i /></div>
              <ConvertSourceRow
                v-else
                :p="item.p"
                :focus-id="focusId"
                @preview="onPreview"
                @remove="onRemove"
                @log="onLog"
                @change-output="onChangeOutput"
              />
            </template>
            <template #footer>
              <div v-if="noResult" class="cv-noresult">没有找到包含“{{ cv.keyword.trim() }}”的记录</div>
              <div v-else-if="filterEmpty" class="cv-noresult">{{ cv.filter === 'active' ? '已加载的记录里没有进行中的转换' : '已加载的记录里没有失败的转换' }}</div>
              <div v-if="cv.hasMore" class="cv-loadmore">
                <button type="button" class="btn" :disabled="cv.loadingMore" :aria-busy="cv.loadingMore" @click="cv.loadMore()">{{ cv.loadingMore ? '正在加载…' : '加载更早的记录' }}</button>
              </div>
            </template>
          </VirtualList>
        </div>
      </section>

      <ConvertSettingsPanel />
    </div>

    <ConvertPreviewDialog :target="preview" :narrow="narrow" @close="preview = null" />
    <ConvertDeleteDialog :ask="delAsk" :narrow="narrow" :busy="deleting" :checked="delChecked" @close="delAsk = null" @confirm="onConfirmDelete" />
    <Teleport to="body">
      <div v-if="logId" class="cv2 cv-layer" :class="{ w1024: narrow }">
        <div class="cv-mask" @click.self="closeLog" @keydown.esc="closeLog">
          <div class="cv-dlg cv-log" role="dialog" aria-modal="true" aria-label="转换日志">
            <div class="dfoot"><h3 style="flex: 1">转换日志</h3><button type="button" class="btn" @click="closeLog">关闭</button></div>
            <pre>{{ logLoading ? '正在读取日志…' : logText }}</pre>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>
