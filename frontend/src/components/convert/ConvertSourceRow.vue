<script setup lang="ts">
// 转换页源文件父行 + 子记录（设计 §3.1 / §3.3）。行进入可视区域（虚拟列表渲染它）时才取缩略图、补读媒体信息。
import MidEllipsis from '@/components/common/MidEllipsis.vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import ConvertThumb from './ConvertThumb.vue'
import ConvertKid from './ConvertKid.vue'
import { metaInfoOf, useConvertRecordsStore, SOURCE_REMOVE_LABEL, type ParentView } from '@/stores/convertRecords'
import { useTaskStore } from '@/stores/tasks'
import { CONFLICT_TITLE, CONFLICT_TITLE_V24, coverKindOf, extOf, sourceMetaText } from '@/utils/convertText'
import { convertV2IsReal } from '@/api/convertRecords'
import { simParam } from '@/api/sim'
import {
  ORIGINAL_MISSING_OPEN, COPY_CANCEL_LABEL, COPY_CHECK_TIP_CANCELED, COPY_CHECK_TIP_FAILED, COPY_PREVIEW_TIP_CANCELED, COPY_PREVIEW_TIP_FAILED, COPY_PREVIEW_TIP_RUNNING, COPY_RETRY_LABEL, COPY_RUNNING_NOTE,
  OPEN_STORAGE_SETTINGS, copyPct, copyProgressText, copyTag, isNoSpace, sourcePathTip, splitPathTail,
} from '@/utils/convertV24Text'
import { formatBytes, formatShortClock } from '@/utils/format'
import { probeErrorText, userVisibleMessage } from '@/errors/errorMessages'

const props = defineProps<{ p: ParentView; focusId?: string }>()
const emit = defineEmits<{
  preview: [kind: 'source' | 'record', id: string]
  remove: [kind: 'source' | 'record', id: string]
  log: [id: string]
  changeOutput: [id: string]
  reconvert: [id: string]
  openStorageSettings: []
}>()
const cv = useConvertRecordsStore()
const tasks = useTaskStore()

const src = computed(() => props.p.src)
const gone = computed(() => src.value.exists === false)
const info = computed(() => metaInfoOf(src.value))
const ext = computed(() => extOf(src.value.name))
/** 封面类型（包 20）：图片格式 = 图片；有探测结果按有没有画面；否则按扩展名 */
const cover = computed(() => coverKindOf(ext.value, info.value, cv.catalogCategoryOf(ext.value)))
const fmt = computed(() => ext.value.toUpperCase())
const dur = computed(() => formatShortClock(info.value?.duration ?? 0))
// ---- v0.24 副本（§13.2；§八 第 39、40、44、62 条） ----
const v24 = cv.v24
const cs = computed(() => src.value.copyState)
const copying = computed(() => cs.value === 'copying')
const copyBad = computed(() => cs.value === 'failed' || cs.value === 'canceled')
const tag = computed(() => copyTag(cs.value, src.value.copiedBytes ?? 0))
const cpText = computed(() => copyProgressText(src.value.copiedBytes ?? 0, src.value.totalBytes ?? 0))
const cpPct = computed(() => copyPct(src.value.copiedBytes ?? 0, src.value.totalBytes ?? 0))
const noSpace = computed(() => isNoSpace(src.value.copyError))
/** 复制失败的说明：后端 copyError.message（空间不足那句后端拼好） */
const copyMsg = computed(() => userVisibleMessage(src.value.copyError?.message) || '复制文件失败')
const opath = computed(() => splitPathTail(src.value.originalPath || src.value.path))
const ptip = computed(() => sourcePathTip(src.value))
/** 模拟截图：?cv_hover=path 让勾选的行显示路径浮层（截图 25） */
const forcePath = computed(() => !convertV2IsReal() && simParam('cv_hover') === 'path' && props.p.selected)
const pvTip = computed(() => (gone.value ? '源文件已不存在，无法预览' : copying.value ? COPY_PREVIEW_TIP_RUNNING : cs.value === 'failed' ? COPY_PREVIEW_TIP_FAILED : cs.value === 'canceled' ? COPY_PREVIEW_TIP_CANCELED : ''))
const isNew = computed(() => src.value.recordCount === 0 && !props.p.kids.length && !gone.value && !tag.value)
const meta = computed(() => {
  if (gone.value) return { cls: 'gone', text: '原位置找不到这个文件，转换记录仍保留' }
  if (src.value.probe === 'error') return { cls: 'err', text: src.value.probeError ? `读取失败：${probeErrorText(src.value.probeError.code, src.value.probeError.message)}` : '读取失败' }
  if (info.value) {
    // 窄窗口大小优先：最后一段（大小）单独放，前面的参数先省略
    const text = sourceMetaText(info.value, ext.value) // 图片格式不说“没有声音”（走查 D7）
    const i = info.value.size ? text.lastIndexOf(' · ') : -1
    return i > 0 ? { cls: 'cv-mm', text, main: text.slice(0, i), size: text.slice(i) } : { cls: '', text }
  }
  return { cls: 'wait', text: '正在读取…' }
})
const n = computed(() => src.value.recordCount || props.p.kids.length)
const checkTip = computed(() =>
  gone.value ? '源文件已不存在，不能再转换' : cs.value === 'failed' ? COPY_CHECK_TIP_FAILED : cs.value === 'canceled' ? COPY_CHECK_TIP_CANCELED : src.value.probe === 'error' ? '读取失败的文件不能转换' : '',
)
const conflictTitle = v24 ? CONFLICT_TITLE_V24 : CONFLICT_TITLE

function onRow(e: MouseEvent) {
  // 点行空白处 = 切换勾选；按钮、缩略图、链接自己处理
  if ((e.target as HTMLElement).closest('button, a, input')) return
  if (props.p.checkable) cv.toggle(src.value.sourceId)
}
function onCheck() {
  if (props.p.checkable || props.p.selected) cv.toggle(src.value.sourceId)
}
function onFold() {
  if (props.p.kids.length) cv.setOpen(src.value.sourceId, !props.p.open)
}

// 1024 的“更多”菜单
const menuOpen = ref(false)
const moreBtn = ref<HTMLElement | null>(null)
function closeMenu(e?: Event) {
  if (e && moreBtn.value?.parentElement?.contains(e.target as Node)) return
  menuOpen.value = false
}
watch(menuOpen, (o) => (o ? document.addEventListener('pointerdown', closeMenu, true) : document.removeEventListener('pointerdown', closeMenu, true)))
onBeforeUnmount(() => document.removeEventListener('pointerdown', closeMenu, true))
function menu(fn: () => void) {
  menuOpen.value = false
  fn()
}

// 进入可视区域：取缩略图、补读信息；展开的完成记录取缩略图
onMounted(() => {
  cv.ensureThumb(src.value)
  cv.requestMeta(src.value)
})
// 源文件变了（路径 / 大小）或转换组件就绪：重取缩略图（store 里按特征去重；取到过的不重复取，失败的隔 5 秒才重取）
watch(
  () => `${src.value.path}|${(src.value.info ?? src.value.media)?.size ?? ''}|${cv.thumbRetryTick}`,
  () => cv.ensureThumb(src.value),
)
watch(
  // 记录完成（status 变成 succeeded）、重转（version）、转换组件就绪（thumbRetryTick）都会重取；失败的在下次挂载时也会重取
  () => (props.p.open ? props.p.kids.map((k) => `${k.id}:${k.status}:${k.version}:${k.outputGone}`).join(',') + `|${cv.thumbRetryTick}` : ''),
  () => {
    if (props.p.open) for (const k of props.p.kids) cv.ensureRecThumb(k)
  },
  { immediate: true },
)
// 同一路径再次添加：闪一下（§3.1）
const flashing = ref(false)
let flashTimer: ReturnType<typeof setTimeout> | undefined
watch(
  () => src.value.flashAt,
  (at) => {
    if (!at || Date.now() - at > 1500) return
    flashing.value = false
    requestAnimationFrame(() => (flashing.value = true))
    clearTimeout(flashTimer)
    flashTimer = setTimeout(() => (flashing.value = false), 1300)
  },
  { immediate: true },
)
onBeforeUnmount(() => clearTimeout(flashTimer))
/** 打开所在文件夹之前后端会再查一次；不在了就标出来并提示（§四 8） */
async function revealSrc() {
  if (!(await cv.revealSource(src.value.sourceId))) cv.say(ORIGINAL_MISSING_OPEN)
}
async function revealKid(id: string) {
  if (!(await cv.revealOutput(id))) cv.say('文件已被移动或删除')
}
</script>
<template>
  <div class="cv-src" :class="{ sel: p.selected, open: p.open && p.kids.length, flash: flashing }" :data-src="src.sourceId">
    <div class="cv-prow" @click="onRow">
      <button
        type="button"
        class="cv-chk"
        :class="{ on: p.selected, dis: !p.checkable }"
        role="checkbox"
        :aria-checked="p.selected"
        :aria-disabled="!p.checkable || undefined"
        :aria-label="`勾选 ${src.name}`"
        :data-tip="checkTip || undefined"
        @click.stop="onCheck"
      ><FIcon v-if="p.selected" name="check" /></button>
      <button
        type="button"
        class="cv-fold"
        :class="{ closed: !p.open, none: !p.kids.length }"
        :aria-expanded="p.kids.length ? p.open : undefined"
        :aria-label="p.open ? `收起 ${src.name} 的转换记录` : `展开 ${src.name} 的转换记录`"
        :tabindex="p.kids.length ? 0 : -1"
        @click.stop="onFold"
      ><FIcon name="down" /></button>
      <ConvertThumb :state="src.thumb" :gone="gone" :cover="cover" :fmt="fmt" :dur="cover === 'image' ? '' : dur" :clickable="!gone && !pvTip" :label="`预览源文件 ${src.name}`" @click="!pvTip && emit('preview', 'source', src.sourceId)" />
      <div class="cv-pm" :class="{ 'cv-pmtip': v24, hv: forcePath }">
        <div class="cv-nm">
          <MidEllipsis tag="b" :text="src.name" :title="v24 ? src.name : src.path" />
          <span v-if="isNew" class="cv-tag t-new">新添加</span>
          <span v-if="gone" class="cv-tag t-warn">源文件已不存在</span>
          <span v-else-if="tag" class="cv-tag" :class="tag.cls"><FIcon v-if="tag.icon" name="warn" :size="12" />{{ tag.text }}</span>
        </div>
        <div v-if="v24 && copying" class="m cv-cpm">
          <div class="bar" :class="{ q: !cpText }" role="progressbar" :aria-valuenow="cpPct" aria-valuemin="0" aria-valuemax="100" :aria-label="`复制进度 ${src.name}`"><i :style="{ width: (cpText ? cpPct : 0) + '%' }" /></div>
          <template v-if="cpText"><span class="num">{{ cpPct }}%</span><span class="cv-d">·</span><span>{{ formatBytes(src.copiedBytes ?? 0) }} / {{ formatBytes(src.totalBytes ?? 0) }}</span></template>
        </div>
        <div v-else class="m" :class="meta.cls" :title="v24 ? undefined : meta.text"><template v-if="meta.size"><span class="mt">{{ meta.main }}</span><span class="cv-sz">{{ meta.size }}</span></template><template v-else>{{ meta.text }}</template></div>
        <div v-if="v24" class="m cv-op" :class="{ hv: forcePath }"><FIcon name="folder" :size="12" /><span class="pp"><span class="h">{{ opath.head }}</span><span class="t">{{ opath.tail }}</span></span></div>
        <div v-if="v24" class="cv-ptip" role="tooltip">
          <div v-for="r in ptip.rows" :key="r.label"><span>{{ r.label }}</span><b>{{ r.path }}</b></div>
          <small>{{ ptip.note }}</small>
        </div>
      </div>
      <span v-if="n" class="cv-sum" :style="p.failed && !p.running ? { color: 'var(--ff-danger-text)' } : undefined">
        <template v-if="p.running">
          <div class="bar"><i class="run" :style="{ width: p.runPct + '%' }" /></div>
          <span class="only1280">{{ p.running }} 项转换中 · 共 {{ n }} 条</span><span class="only1024">{{ p.running }} 项转换中</span>
        </template>
        <template v-else-if="p.failed">
          <span class="only1280">{{ p.failed }} 项失败 · 共 {{ n }} 条</span><span class="only1024">{{ p.failed }} 项失败</span>
        </template>
        <template v-else>
          <span class="only1280">{{ n }} 条记录</span><span class="only1024">{{ n }} 条</span>
        </template>
      </span>
      <div v-if="v24 && copying" class="cv-ops">
        <button type="button" class="cv-tbtn cv-tbtn-n" :aria-label="COPY_CANCEL_LABEL" :title="COPY_CANCEL_LABEL" @click="cv.cancelCopy(src.sourceId)"><FIcon name="x" :size="14" />取消</button>
        <button ref="moreBtn" type="button" class="cv-ib" :aria-label="`更多：${SOURCE_REMOVE_LABEL}`" title="更多" aria-haspopup="menu" :aria-expanded="menuOpen" @click="menuOpen = !menuOpen"><FIcon name="more" /></button>
        <div v-if="menuOpen" class="cv-more-menu" role="menu">
          <button type="button" role="menuitem" @click="menu(() => emit('remove', 'source', src.sourceId))"><FIcon name="trash" />{{ SOURCE_REMOVE_LABEL }}</button>
        </div>
      </div>
      <div v-else-if="v24 && copyBad" class="cv-ops">
        <button type="button" class="cv-ib" aria-disabled="true" :aria-label="`预览源文件 ${src.name}：${pvTip}`" :data-tip="pvTip"><FIcon name="eye" /></button>
        <button type="button" class="cv-ib only1280" :aria-label="`${COPY_RETRY_LABEL} ${src.name}`" :title="COPY_RETRY_LABEL" @click="cv.retryCopy(src.sourceId)"><FIcon name="retry" /></button>
        <button type="button" class="cv-ib del only1280" :aria-label="`${SOURCE_REMOVE_LABEL} ${src.name}`" :title="SOURCE_REMOVE_LABEL" @click="emit('remove', 'source', src.sourceId)"><FIcon name="trash" /></button>
        <button ref="moreBtn" type="button" class="cv-ib only1024" :aria-label="`更多：${COPY_RETRY_LABEL}、${SOURCE_REMOVE_LABEL}`" title="更多" aria-haspopup="menu" :aria-expanded="menuOpen" @click="menuOpen = !menuOpen"><FIcon name="more" /></button>
        <div v-if="menuOpen" class="cv-more-menu" role="menu">
          <button type="button" role="menuitem" @click="menu(() => cv.retryCopy(src.sourceId))"><FIcon name="retry" />{{ COPY_RETRY_LABEL }}</button>
          <button type="button" role="menuitem" @click="menu(() => emit('remove', 'source', src.sourceId))"><FIcon name="trash" />{{ SOURCE_REMOVE_LABEL }}</button>
        </div>
      </div>
      <div v-else class="cv-ops">
        <button v-if="gone" type="button" class="cv-ib" aria-disabled="true" :aria-label="`预览源文件 ${src.name}：源文件已不存在，无法预览`" data-tip="源文件已不存在，无法预览"><FIcon name="eye" /></button>
        <button v-else type="button" class="cv-ib" :aria-label="`预览源文件 ${src.name}`" title="预览源文件" @click="emit('preview', 'source', src.sourceId)"><FIcon name="eye" /></button>
        <button v-if="gone" type="button" class="cv-ib only1280" aria-disabled="true" :aria-label="`打开所在文件夹 ${src.name}：源文件已不存在`" data-tip="源文件已不存在"><FIcon name="folder" /></button>
        <button v-else type="button" class="cv-ib only1280" :aria-label="`打开所在文件夹 ${src.name}`" title="打开所在文件夹" @click="revealSrc"><FIcon name="folder" /></button>
        <button type="button" class="cv-ib del only1280" :aria-label="`${SOURCE_REMOVE_LABEL} ${src.name}`" :title="SOURCE_REMOVE_LABEL" @click="emit('remove', 'source', src.sourceId)"><FIcon name="trash" /></button>
        <button ref="moreBtn" type="button" class="cv-ib only1024" :aria-label="`更多：打开所在文件夹、${SOURCE_REMOVE_LABEL}`" title="更多" aria-haspopup="menu" :aria-expanded="menuOpen" @click="menuOpen = !menuOpen"><FIcon name="more" /></button>
        <div v-if="menuOpen" class="cv-more-menu" role="menu">
          <button type="button" role="menuitem" :aria-disabled="gone || undefined" :title="gone ? '源文件已不存在' : undefined" @click="!gone && menu(revealSrc)"><FIcon name="folder" />打开所在文件夹</button>
          <button type="button" role="menuitem" :aria-label="SOURCE_REMOVE_LABEL" @click="menu(() => emit('remove', 'source', src.sourceId))"><FIcon name="trash" />{{ SOURCE_REMOVE_LABEL }}</button>
        </div>
      </div>
    </div>
    <div v-if="p.conflict" class="cv-err cv-conf" role="group" :aria-label="conflictTitle">
      <FIcon name="warn" />
      <div class="t"><b>{{ conflictTitle }}</b>{{ p.conflict }}</div>
    </div>
    <div v-if="v24 && copying && !n" class="cv-empty-kid">{{ COPY_RUNNING_NOTE }}</div>
    <div v-if="v24 && cs === 'failed'" class="cv-err cv-cperr" role="group" aria-label="复制失败">
      <FIcon name="warn" />
      <div class="t"><b>复制失败</b>{{ copyMsg }}<div class="acts">
        <button type="button" @click="cv.retryCopy(src.sourceId)">重试</button>
        <button type="button" @click="emit('remove', 'source', src.sourceId)">{{ SOURCE_REMOVE_LABEL }}</button>
        <button v-if="noSpace" type="button" @click="emit('openStorageSettings')">{{ OPEN_STORAGE_SETTINGS }}</button>
      </div></div>
    </div>
    <div v-if="p.open && p.kids.length" class="cv-kids">
      <ConvertKid
        v-for="k in p.kids"
        :key="k.id"
        :kid="k"
        :thumb="cv.recThumbs.get(k.id)"
        :queue-pos="k.status === 'queued' ? tasks.queuePosition(k.id) : 0"
        :busy="tasks.isBusy(k.id)"
        :hit="!!p.hits?.has(k.id)"
        :focused="focusId === k.id"
        @preview="emit('preview', 'record', k.id)"
        @cancel="cv.cancel(k.id)"
        @retry="cv.retry(k.id)"
        @reveal="revealKid(k.id)"
        @remove="emit('remove', 'record', k.id)"
        @log="emit('log', k.id)"
        @change-output="emit('changeOutput', k.id)"
        @reconvert="emit('reconvert', k.id)"
      />
      <button v-if="p.moreCount > 0" type="button" class="ff-link cv-more" :disabled="src.loadingMore" :aria-busy="src.loadingMore" @click="cv.loadMoreRecords(src.sourceId)">{{ src.loadingMore ? '正在加载…' : `展开更多（还有 ${p.moreCount} 条）` }}</button>
    </div>
    <div v-else-if="!n && !p.conflict && !(v24 && (copying || cs === 'failed'))" class="cv-empty-kid">还没有转换记录。勾选后在右侧选择格式，点“转换”。</div>
  </div>
</template>
