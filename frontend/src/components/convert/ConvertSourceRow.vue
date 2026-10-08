<script setup lang="ts">
// 转换页源文件父行 + 子记录（设计 §3.1 / §3.3）。行进入可视区域（虚拟列表渲染它）时才取缩略图、补读媒体信息。
import MidEllipsis from '@/components/common/MidEllipsis.vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import ConvertThumb from './ConvertThumb.vue'
import ConvertKid from './ConvertKid.vue'
import { metaInfoOf, useConvertRecordsStore, SOURCE_REMOVE_LABEL, type ParentView } from '@/stores/convertRecords'
import { useTaskStore } from '@/stores/tasks'
import { CONFLICT_TITLE, isAudioContainer, isAudioOnly, sourceMetaText } from '@/utils/convertText'
import { formatShortClock } from '@/utils/format'

const props = defineProps<{ p: ParentView; focusId?: string }>()
const emit = defineEmits<{
  preview: [kind: 'source' | 'record', id: string]
  remove: [kind: 'source' | 'record', id: string]
  log: [id: string]
  changeOutput: [id: string]
}>()
const cv = useConvertRecordsStore()
const tasks = useTaskStore()

const src = computed(() => props.p.src)
const gone = computed(() => src.value.exists === false)
const info = computed(() => metaInfoOf(src.value))
const audio = computed(() => (info.value ? isAudioOnly(info.value) : isAudioContainer(src.value.name.split('.').pop() ?? '')))
const dur = computed(() => formatShortClock(info.value?.duration ?? 0))
const isNew = computed(() => src.value.recordCount === 0 && !props.p.kids.length && !gone.value)
const meta = computed(() => {
  if (gone.value) return { cls: 'gone', text: '原位置找不到这个文件，转换记录仍保留' }
  if (src.value.probe === 'error') return { cls: 'err', text: src.value.probeError?.message ? `读取失败：${src.value.probeError.message}` : '读取失败' }
  if (info.value) return { cls: '', text: sourceMetaText(info.value) }
  return { cls: 'wait', text: '正在读取…' }
})
const n = computed(() => src.value.recordCount || props.p.kids.length)
const checkTip = computed(() => (gone.value ? '源文件已不存在，不能再转换' : src.value.probe === 'error' ? '读取失败的文件不能转换' : ''))

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
watch(
  () => (props.p.open ? props.p.kids.map((k) => `${k.id}:${k.status}:${k.outputGone}`).join(',') : ''),
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
  if (!(await cv.revealSource(src.value.sourceId))) cv.say('文件已被移动或删除')
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
      <ConvertThumb :state="src.thumb" :gone="gone" :audio="audio" :dur="dur" :clickable="!gone" :label="`预览源文件 ${src.name}`" @click="emit('preview', 'source', src.sourceId)" />
      <div class="cv-pm">
        <div class="cv-nm">
          <MidEllipsis tag="b" :text="src.name" :title="src.path" />
          <span v-if="isNew" class="cv-tag t-new">新添加</span>
          <span v-if="gone" class="cv-tag t-warn">源文件已不存在</span>
        </div>
        <div class="m" :class="meta.cls" :title="meta.text">{{ meta.text }}</div>
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
      <div class="cv-ops">
        <button v-if="gone" type="button" class="cv-ib" aria-disabled="true" :aria-label="`预览源文件 ${src.name}：源文件已不存在，无法预览`" data-tip="源文件已不存在，无法预览"><FIcon name="eye" /></button>
        <button v-else type="button" class="cv-ib" :aria-label="`预览源文件 ${src.name}`" title="预览源文件" @click="emit('preview', 'source', src.sourceId)"><FIcon name="eye" /></button>
        <button v-if="gone" type="button" class="cv-ib only1280" aria-disabled="true" :aria-label="`打开所在文件夹 ${src.name}：源文件已不存在`" data-tip="源文件已不存在"><FIcon name="folder" /></button>
        <button v-else type="button" class="cv-ib only1280" :aria-label="`打开所在文件夹 ${src.name}`" title="打开所在文件夹" @click="revealSrc"><FIcon name="folder" /></button>
        <button type="button" class="cv-ib del only1280" :aria-label="SOURCE_REMOVE_LABEL" :title="SOURCE_REMOVE_LABEL" @click="emit('remove', 'source', src.sourceId)"><FIcon name="trash" /></button>
        <button ref="moreBtn" type="button" class="cv-ib only1024" :aria-label="`更多：打开所在文件夹、${SOURCE_REMOVE_LABEL}`" title="更多" aria-haspopup="menu" :aria-expanded="menuOpen" @click="menuOpen = !menuOpen"><FIcon name="more" /></button>
        <div v-if="menuOpen" class="cv-more-menu" role="menu">
          <button type="button" role="menuitem" :aria-disabled="gone || undefined" :title="gone ? '源文件已不存在' : undefined" @click="!gone && menu(revealSrc)"><FIcon name="folder" />打开所在文件夹</button>
          <button type="button" role="menuitem" :aria-label="SOURCE_REMOVE_LABEL" @click="menu(() => emit('remove', 'source', src.sourceId))"><FIcon name="trash" />{{ SOURCE_REMOVE_LABEL }}</button>
        </div>
      </div>
    </div>
    <div v-if="p.conflict" class="cv-err cv-conf" role="group" :aria-label="CONFLICT_TITLE">
      <FIcon name="warn" />
      <div class="t"><b>{{ CONFLICT_TITLE }}</b>{{ p.conflict }}</div>
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
      />
      <button v-if="p.moreCount > 0" type="button" class="ff-link cv-more" :disabled="src.loadingMore" :aria-busy="src.loadingMore" @click="cv.loadMoreRecords(src.sourceId)">{{ src.loadingMore ? '正在加载…' : `展开更多（还有 ${p.moreCount} 条）` }}</button>
    </div>
    <div v-else-if="!n && !p.conflict" class="cv-empty-kid">还没有转换记录。勾选后在右侧选择格式，点“转换”。</div>
  </div>
</template>
