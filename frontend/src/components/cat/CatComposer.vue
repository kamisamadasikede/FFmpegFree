<template>
  <div ref="root" class="ct-comp" :class="{ bare: welcome, missing }">
    <div v-if="!welcome" class="ctx-bar" aria-label="项目上下文">
      <span class="cx" :class="{ miss: missing }"><FIcon :name="missing ? 'warn' : 'folder'" :size="13" /><b :title="ctxName">{{ ctxName }}</b></span>
      <span class="cx"><FIcon name="monitor" :size="13" />本地</span>
      <span class="cx"><FIcon name="branch" :size="13" />{{ ctxBranch }}</span>
    </div>
    <div ref="inEl" class="ct-in" :aria-disabled="missing || undefined" :aria-describedby="missing ? missId : undefined">
      <div v-if="shownFiles.length" class="ct-files" data-testid="cat-attach-row">
        <div v-for="f in shownFiles" :key="f.id" class="ct-card" :class="{ open: canPreview(f.kind), media: showsThumb(f.kind) }" data-testid="cat-attach-card">
          <button type="button" class="rm" :aria-label="'移除 ' + f.name" :title="'移除 ' + f.name" @click="removeFile(f.id)"><FIcon name="x" :size="11" /></button>
          <div
            class="face"
            :role="canPreview(f.kind) ? 'button' : undefined"
            :tabindex="canPreview(f.kind) ? 0 : undefined"
            :aria-label="canPreview(f.kind) ? '预览 ' + f.name : undefined"
            @click="openPreview(shownOf(f))"
            @keydown.enter.prevent="openPreview(shownOf(f))"
          >
            <CatThumb v-if="showsThumb(f.kind)" :file="shownOf(f)" />
            <template v-else>
              <span class="pic"><FIcon name="file" :size="22" /></span>
              <span class="nm"><FIcon name="file" :size="12" /><b :title="f.name">{{ f.name }}</b></span>
            </template>
          </div>
        </div>
      </div>
      <div v-if="missing" :id="missId" class="miss-ph" data-testid="cat-composer-missing"><FIcon name="warn" :size="14" />{{ PC.missing }}</div>
      <textarea
        ref="ta"
        v-model="draft"
        class="ta"
        rows="1"
        :placeholder="missing ? '' : placeholder"
        :disabled="missing"
        aria-label="输入消息"
        @input="autosize"
        @keydown.enter="onEnter"
      />
      <div class="row">
        <button type="button" class="ct-plus" title="添加" aria-label="添加" :disabled="missing" data-testid="cat-attach-add" @click="addFiles"><FIcon name="plus" :size="15" :stroke="2" /></button>
        <button
          v-if="extraFiles.length"
          type="button"
          class="ct-more"
          :aria-expanded="extraOpen"
          :aria-label="'还有 ' + extraFiles.length + ' 个文件'"
          title="其余文件"
          data-testid="cat-attach-more"
          @click.stop="extraOpen = !extraOpen"
        >
          <FIcon name="file" :size="16" />
          <span class="badge">{{ extraFiles.length }}</span>
        </button>
        <div v-if="extraOpen" class="ct-extra" data-testid="cat-attach-extra" @click.stop>
          <div v-for="f in extraFiles" :key="f.id" class="ex">
            <button type="button" class="ex-open" :class="{ on: canPreview(f.kind), media: showsThumb(f.kind) }" :aria-label="showsThumb(f.kind) ? '预览 ' + f.name : undefined" @click="openPreview(shownOf(f))">
              <CatThumb v-if="showsThumb(f.kind)" :file="shownOf(f)" tile />
              <template v-else>
                <FIcon name="file" :size="14" />
                <b :title="f.name">{{ f.name }}</b>
              </template>
            </button>
            <button type="button" class="rx" :aria-label="'移除 ' + f.name" @click="removeFile(f.id)"><FIcon name="x" :size="12" /></button>
          </div>
        </div>
        <CatPreviewDialog v-model:open="previewOn" :file="previewFile" />
        <button
          ref="chipAccess"
          type="button"
          class="chip-access"
          :class="{ on: catState.access === 'approve' }"
          :disabled="missing"
          aria-haspopup="menu"
          :aria-expanded="menu === 'access'"
          @click.stop="toggle('access')"
        >
          <FIcon :name="accessMeta.icon" :size="14" /><span>{{ accessMeta.short }}</span>
        </button>
        <span class="sp" />
        <CatContextRing v-if="contextUsage" :used="contextUsage.used" :size="contextUsage.window" />
        <button
          ref="chipModel"
          type="button"
          class="chip-model"
          :disabled="missing"
          aria-haspopup="menu"
          :aria-expanded="menu === 'casc'"
          @click.stop="toggle('casc')"
        >
          <FIcon name="gauge" :size="15" /><span>{{ capsuleLabel }}</span><FIcon name="down" :size="12" class="caret" />
        </button>
        <button
          v-if="running"
          type="button"
          class="ct-send ct-stop"
          :aria-label="stopping ? '正在停止' : '停止生成'"
          :title="stopping ? '正在停止' : '停止生成'"
          :disabled="stopping"
          data-testid="cat-stop"
          @click="onStop"
        ><i class="sq" aria-hidden="true" /></button>
        <button
          v-else
          type="button"
          class="ct-send"
          :class="{ blocked: notReady || checking || missing }"
          aria-label="发送"
          :aria-disabled="!canSend"
          :disabled="notReady || checking || missing"
          :title="missing ? PC.missing : notReady ? CAT_COPY.notReady : undefined"
          data-testid="cat-send"
          @click="send"
        ><FIcon name="up" :size="15" /></button>
      </div>

      <div v-if="menu" class="mn" :style="menuStyle" @click.stop>
        <!-- 访问模式 -->
        <div v-if="menu === 'access'" class="mn-access" role="menu" aria-label="访问模式">
          <div class="mn-ahd"><span>应如何批准 Cat 操作？</span><a class="more" href="#" @click.prevent>了解更多</a></div>
          <template v-for="a in CAT_ACCESS" :key="a.id">
          <div v-if="a.id === 'approve'" class="mn-sep" role="separator" />
          <div
            class="mn-acc"
            :class="{ on: catState.access === a.id, disabled: !a.enabled, danger: a.id === 'approve' }"
            role="menuitemradio"
            tabindex="0"
            :aria-checked="catState.access === a.id"
            :aria-disabled="!a.enabled"
            :title="a.enabled ? undefined : laterTip"
            @click="pickAccess(a.id)"
            @keydown.enter.prevent="pickAccess(a.id)"
          >
            <span class="ic"><FIcon :name="a.icon" :size="18" /></span>
            <div><b>{{ a.name }}</b><small>{{ a.desc }}</small></div>
            <FIcon v-if="catState.access === a.id" name="check" :size="16" class="ok" />
          </div>
          </template>
        </div>
        <!-- 模型 / 思考强度：一级菜单 + 左侧子菜单 -->
        <div v-else class="mn-casc">
          <div class="mn-main" role="menu" aria-label="模型设置">
            <div
              v-for="r in rows"
              :key="r.key"
              :ref="(el) => setRowRef(r.key, el)"
              class="mn-row"
              :class="{ on: sub === r.key }"
              role="menuitem"
              tabindex="0"
              aria-haspopup="menu"
              :aria-expanded="sub === r.key"
              @mouseenter="showSub(r.key)"
              @click.stop="showSub(r.key)"
              @keydown.enter.prevent="showSub(r.key)"
            >
              <span class="lb">{{ r.label }}</span><span class="v">{{ r.value }}</span><FIcon name="right" :size="14" />
            </div>
          </div>
          <div v-if="sub" :key="sub" ref="subEl" class="mn-sub" role="menu" :aria-label="sub === 'model' ? '模型' : '思考强度'" :style="{ top: subTop + 'px' }">
            <div
              v-for="o in subItems"
              :key="o.id"
              class="mn-op"
              :class="{ on: o.on }"
              role="menuitemradio"
              tabindex="0"
              :aria-checked="o.on"
              @click="pickSub(o.id)"
              @keydown.enter.prevent="pickSub(o.id)"
            >
              <span class="ck"><FIcon v-if="o.on" name="check" :size="15" :stroke="2.2" /></span><span class="nm">{{ o.name }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
    <div v-if="welcome" class="wl-tray">
      <span v-if="projectName" class="wl-proj on static" data-testid="cat-work-in-project">
        <FIcon name="folder" :size="14" /><span>{{ PC.workIn(projectName) }}</span>
      </span>
      <button
        v-else
        type="button"
        class="wl-proj"
        :class="{ on: catState.workInProject }"
        :aria-pressed="catState.workInProject"
        @click="catState.workInProject = !catState.workInProject"
      >
        <FIcon name="folder" :size="14" /><span>在项目中工作</span><FIcon name="down" :size="11" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, type ComponentPublicInstance } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import CatContextRing from '@/components/cat/CatContextRing.vue'
import { ElMessage } from 'element-plus'
import { CAT_COPY } from '@/api/cat'
import { CAT_PROJECT_COPY as PC } from '@/api/catProjects'
import { CAT_ACCESS } from '@/api/catMock'
import { pickFiles } from '@/api/system'
import { toAppError } from '@/api/call'
import { CAT_ATTACH_MAX, fileBaseName, splitAttach, type CatOutgoingFile } from '@/components/cat/catAttach'
import { canPreview, previewKind, showsThumb, type CatPreviewKind, type CatShownFile } from '@/components/cat/catMedia'
import CatThumb from '@/components/cat/CatThumb.vue'
import CatPreviewDialog from '@/components/cat/CatPreviewDialog.vue'
import { accessMeta, capsuleLabel, catState, chooseAccess, chooseModel, chooseThink, modelName, thinkName } from '@/views/cat/catState'

/**
 * Cat 输入框：批准方式可选普通模式 / Plan / 自动审查 / 全部批准，没选过是全部批准。
 * 合一胶囊只显示 List API 有的模型/强度（无强度不显示「·」半截）。
 */
const props = withDefaults(
  defineProps<{ welcome?: boolean; placeholder?: string; ctxName?: string; ctxBranch?: string; busy?: boolean; running?: boolean; stopping?: boolean; notReady?: boolean; checking?: boolean; missing?: boolean; projectName?: string }>(),
  {
    welcome: false,
    placeholder: '随心输入',
    ctxName: 'Cat',
    ctxBranch: 'main',
    busy: false,
    running: false,
    stopping: false,
    notReady: false,
    /** 组件检查中：发送同样置灰（样式与未就绪一致），不让用户点了才被拒 */
    checking: false,
    /** 项目文件夹不见了：输入框禁用，占位换成警示图标 +「项目文件夹不见了。」，胶囊 45% 不可点（设计 §05） */
    missing: false,
    /** 欢迎页属于某个项目（项目行「+」进来）：托盘显示「在项目中工作 · {项目}」 */
    projectName: '',
  },
)
let missSeq = 0
const missId = `ct-miss-${++missSeq}-${Math.random().toString(36).slice(2, 7)}`
const emit = defineEmits<{ send: [payload: { text: string; files: CatOutgoingFile[] }]; stop: []; blocked: [] }>()

/** 停止：父组件把 stopping 置真后按钮立即置灰，重复点击无效 */
function onStop() {
  if (props.stopping) return
  emit('stop')
}

const draft = ref('')
interface DraftFile extends CatOutgoingFile {
  id: number
  kind: CatPreviewKind
}
let fileSeq = 0
const atts = ref<DraftFile[]>([])
const extraOpen = ref(false)
const shownFiles = computed(() => splitAttach(atts.value).shown)
const extraFiles = computed(() => splitAttach(atts.value).extra)
const canSend = computed(() => (!!draft.value.trim() || atts.value.length > 0) && !props.busy && !props.running && !props.notReady && !props.checking && !props.missing)
const contextUsage = computed(() => {
  const u = catState.context[catState.sel]
  if (!u || u.used <= 0 || u.window <= 0) return null
  return u
})
const root = ref<HTMLElement>()
const inEl = ref<HTMLElement>()
const ta = ref<HTMLTextAreaElement>()
const chipAccess = ref<HTMLElement>()
const chipModel = ref<HTMLElement>()
const subEl = ref<HTMLElement>()

type MenuKind = 'access' | 'casc' | null
type SubKind = 'model' | 'think'
const menu = ref<MenuKind>(null)
const sub = ref<SubKind | null>(null)
const subTop = ref(0)
const menuStyle = ref<Record<string, string>>({})
const rowEls: Partial<Record<SubKind, HTMLElement>> = {}
function setRowRef(k: SubKind, el: Element | ComponentPublicInstance | null) {
  if (el instanceof HTMLElement) rowEls[k] = el
}

const laterTip = CAT_COPY.later

const rows = computed(() => {
  const list: Array<{ key: 'model' | 'think'; label: string; value: string }> = [
    { key: 'model', label: '模型', value: modelName.value || 'Cat 助手' },
  ]
  // 无强度列表时不展示「思考强度」行（设计 v0.4 §5.2）
  if (catState.thinks.length) {
    list.push({ key: 'think', label: '思考强度', value: thinkName.value })
  }
  return list
})
const subItems = computed(() =>
  sub.value === 'model'
    ? catState.models.map((m) => ({ id: m.id, name: m.displayName, on: catState.model === m.id }))
    : catState.thinks.map((t) => ({ id: t.id, name: t.displayName, on: catState.think === t.id })),
)

function open(kind: Exclude<MenuKind, null>) {
  const ci = inEl.value?.getBoundingClientRect()
  if (kind === 'access') {
    const a = chipAccess.value?.getBoundingClientRect()
    menuStyle.value = { left: Math.max(0, (a?.left ?? 0) - (ci?.left ?? 0) - 8) + 'px', right: 'auto' }
  } else {
    const a = chipModel.value?.getBoundingClientRect()
    menuStyle.value = { right: Math.max(0, (ci?.right ?? 0) - (a?.right ?? 0)) + 'px', left: 'auto' }
  }
  sub.value = null
  menu.value = kind
}
function close() {
  menu.value = null
  sub.value = null
}
function toggle(kind: Exclude<MenuKind, null>) {
  if (menu.value === kind) close()
  else open(kind)
}
/** 子菜单顶部与触发行顶部对齐（减去自身 4px 内边距 + 1px 边框），向下展开；靠近窗口边缘时挪回窗口内 */
async function showSub(k: SubKind) {
  if (sub.value === k) return
  sub.value = k
  const row = rowEls[k]
  const top = (row?.offsetTop ?? 0) - 5
  subTop.value = top
  await nextTick()
  const r = subEl.value?.getBoundingClientRect()
  if (!r) return
  if (r.bottom > innerHeight - 8) subTop.value = top - (r.bottom - innerHeight + 8)
  else if (r.top < 8) subTop.value = top + 8 - r.top
}
function pickSub(id: string) {
  if (sub.value === 'model') chooseModel(id)
  else chooseThink(id)
  close()
}
function pickAccess(id: 'normal' | 'plan' | 'auto' | 'approve') {
  const a = CAT_ACCESS.find((x) => x.id === id)
  if (!a?.enabled) {
    ElMessage.info(CAT_COPY.later)
    return
  }
  chooseAccess(id)
  close()
}

function autosize() {
  const el = ta.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 160) + 'px'
}
function toDraft(f: CatOutgoingFile): DraftFile {
  const name = f.name || fileBaseName(f.path)
  return { id: ++fileSeq, name, path: f.path, kind: previewKind(name || f.path) }
}
function shownOf(f: DraftFile): CatShownFile {
  return { name: f.name, path: f.path, kind: f.kind }
}
const previewOn = ref(false)
const previewFile = ref<CatShownFile | null>(null)
function openPreview(f: CatShownFile) {
  if (!canPreview(f.kind)) return
  previewFile.value = f
  previewOn.value = true
}
async function addFiles() {
  if (props.missing || props.busy) return
  let paths: string[] = []
  try {
    paths = await pickFiles({ name: '所有文件', patterns: ['*'] }, true)
  } catch (e) {
    ElMessage.warning(toAppError(e).message || '没能打开文件')
    return
  }
  if (!paths?.length) return
  const next = atts.value.slice()
  const same = (a: string, b: string) => a.replace(/\//g, '\\').toLowerCase() === b.replace(/\//g, '\\').toLowerCase()
  let capped = false
  for (const p of paths) {
    if (!p || next.some((f) => same(f.path, p))) continue
    if (next.length >= CAT_ATTACH_MAX) {
      capped = true
      break
    }
    next.push(toDraft({ name: fileBaseName(p), path: p }))
  }
  atts.value = next
  if (capped) ElMessage.warning('一次最多 20 个文件')
}
function removeFile(id: number) {
  atts.value = atts.value.filter((f) => f.id !== id)
  if (!extraFiles.value.length) extraOpen.value = false
}
function send() {
  if (!canSend.value) {
    // G2：未就绪 / 检查中按 Enter，让父组件把横幅抖一下，文字照旧留在框里
    if ((props.notReady || props.checking) && (draft.value.trim() || atts.value.length)) emit('blocked')
    return
  }
  const files = atts.value.map((f) => ({ name: f.name, path: f.path }))
  emit('send', { text: draft.value, files })
  draft.value = ''
  atts.value = []
  extraOpen.value = false
  nextTick(autosize)
}
function onEnter(e: KeyboardEvent) {
  if (e.shiftKey || e.isComposing) return
  e.preventDefault()
  send()
}

function onDocClick(e: MouseEvent) {
  const t = e.target
  if (!(t instanceof Node) || !root.value) return
  if (menu.value && !root.value.querySelector('.mn')?.contains(t)) close()
  if (extraOpen.value && !root.value.querySelector('.ct-more')?.contains(t) && !root.value.querySelector('.ct-extra')?.contains(t)) extraOpen.value = false
}
function onKey(e: KeyboardEvent) {
  if (!menu.value) return
  if (e.key === 'Escape') {
    // D4：关闭后焦点回到触发按钮
    const trigger = menu.value === 'access' ? chipAccess.value : chipModel.value
    close()
    e.stopPropagation()
    trigger?.focus()
    return
  }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    // D4：方向键在选项间移动
    const box = root.value?.querySelector('.mn')
    const items = Array.from(box?.querySelectorAll<HTMLElement>('[role="menuitemradio"], [role="menuitem"]') ?? [])
    if (!items.length) return
    e.preventDefault()
    const i = items.indexOf(document.activeElement as HTMLElement)
    const n = e.key === 'ArrowDown' ? (i + 1) % items.length : (i <= 0 ? items.length - 1 : i - 1)
    items[n]?.focus()
  }
}
onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKey)
})
defineExpose({
  focus: () => ta.value?.focus(),
  setDraft: (t: string) => { draft.value = t; nextTick(autosize); ta.value?.focus() },
  /** 发送没成功、后端也没存这条（如 CAT_PROJECT_MISSING）：把文字放回输入框；用户已经又输入了就不覆盖，不抢焦点 */
  restoreDraft: (t: string) => { if (draft.value.trim()) return; draft.value = t; nextTick(autosize) },
  restoreFiles: (files: CatOutgoingFile[]) => {
    if (atts.value.length || !files?.length) return
    atts.value = files.filter((f) => f.path).map(toDraft)
  },
})
</script>

<style scoped>
.ct-comp.missing .ct-in {
  background: var(--cat-hover);
  box-shadow: none;
}
.ct-comp.missing .ct-plus,
.ct-comp.missing .ct-more,
.ct-comp.missing .chip-access,
.ct-comp.missing .chip-model {
  opacity: 0.45;
  pointer-events: none;
}
.ct-comp.missing .ta {
  cursor: not-allowed;
  background: transparent;
}
.miss-ph {
  position: absolute;
  top: 23px;
  left: 16px;
  height: 22px;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13.5px;
  color: var(--cat-muted);
  pointer-events: none;
}
.miss-ph :deep(svg),
.ctx-bar .cx.miss :deep(svg) {
  color: var(--cat-warn);
}
.ctx-bar .cx.miss b {
  color: var(--cat-muted);
}
.wl-proj.static {
  cursor: default;
}
.ct-comp {
  position: relative;
  flex: none;
  margin: 0 auto 16px;
  width: min(560px, calc(100% - 32px));
  min-width: 0;
  box-sizing: border-box;
}
.ct-comp.bare {
  padding-top: 0;
}
.ctx-bar {
  /* G1 上下文条独立一行放在输入框上方，留 6px，不再被输入框压住 */
  height: 24px;
  margin: 0 8px 6px;
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 0 4px;
  font-size: 12px;
  color: var(--cat-text-2);
  overflow: hidden;
}
.ctx-bar .cx {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  white-space: nowrap;
}
.ctx-bar .cx:first-child {
  flex: 1 1 auto;
  overflow: hidden;
}
.ctx-bar .cx:not(:first-child) {
  flex: none;
}
.ctx-bar .cx b {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ctx-bar .cx :deep(svg) {
  color: var(--cat-muted);
}
.ct-in {
  position: relative;
  z-index: 1;
  min-width: 0;
  border: 1px solid var(--cat-border);
  border-radius: 22px;
  padding: 14px 14px 10px;
  background: var(--cat-surface);
  box-shadow: var(--cat-shadow-md);
  transition: border-color var(--ff-dur-base), box-shadow var(--ff-dur-base);
}
.ct-in:hover {
  border-color: var(--cat-border-strong);
}
/* G3 聚焦：边框主色 50% + 3px 主色 14% 外环 */
.ct-in:focus-within {
  border-color: color-mix(in srgb, var(--cat-accent) 50%, transparent);
  box-shadow: var(--cat-shadow-md), 0 0 0 3px color-mix(in srgb, var(--cat-accent) 14%, transparent);
}
.ta {
  display: block;
  width: 100%;
  min-height: 40px;
  max-height: 160px;
  resize: none;
  border: none;
  outline: none;
  background: transparent;
  font: inherit;
  font-size: 14px;
  line-height: 22px;
  padding: 9px 2px;
  color: var(--cat-text);
}
.ta::placeholder {
  color: var(--cat-muted);
}
.bare .ct-in {
  min-height: 113px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}
.bare .ta {
  padding: 1px 2px 0;
  min-height: 0;
}
.ct-files {
  display: flex;
  gap: 8px;
  margin: 0 0 8px;
  min-width: 0;
  overflow: hidden;
}
.ct-card {
  position: relative;
  flex: 0 0 108px;
  width: 108px;
  height: 92px;
  border-radius: 12px;
  background: var(--cat-sunken);
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
}
.ct-card .face {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-width: 0;
  border-radius: inherit;
}
.ct-card.open .face {
  cursor: pointer;
}
.ct-card .pic {
  flex: 1;
  display: grid;
  place-items: center;
  color: var(--cat-muted);
  min-height: 0;
}
.ct-card .nm {
  flex: none;
  display: flex;
  align-items: center;
  gap: 4px;
  height: 28px;
  padding: 0 8px;
  color: var(--cat-text-2);
  min-width: 0;
}
.ct-card .nm b {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  font-weight: 500;
}
.ct-card .rm {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 16px;
  height: 16px;
  border: none;
  border-radius: 50%;
  padding: 0;
  display: grid;
  place-items: center;
  background: color-mix(in srgb, var(--cat-elevated) 90%, transparent);
  box-shadow: var(--cat-shadow-sm);
  color: var(--cat-muted);
  cursor: pointer;
}
.ct-card .rm:hover {
  color: var(--cat-text);
}
.row {
  position: relative;
  min-height: 52px; /* 与原型实测一致（输入框里控件行 52 高） */
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}
.row .sp {
  flex: 1;
}
button {
  font: inherit;
}
.ct-plus {
  width: 30px;
  height: 30px;
  border-radius: 50%;
  border: none;
  background: var(--cat-sunken);
  color: var(--cat-text-2);
  display: grid;
  transition: background var(--ff-dur-quick), transform var(--ff-dur-quick);
  place-items: center;
  cursor: pointer;
  flex: none;
}
.ct-plus:hover {
  background: var(--cat-pressed);
}
.ct-more {
  position: relative;
  width: 30px;
  height: 30px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--cat-text-2);
  display: grid;
  place-items: center;
  cursor: pointer;
  flex: none;
}
.ct-more:hover,
.ct-more[aria-expanded='true'] {
  background: var(--cat-hover);
}
.ct-more .badge {
  position: absolute;
  top: -1px;
  right: -2px;
  min-width: 14px;
  height: 14px;
  padding: 0 3px;
  border-radius: 8px;
  background: var(--cat-send);
  color: var(--cat-on-send);
  font-size: 10px;
  line-height: 14px;
  text-align: center;
}
.ct-extra {
  position: absolute;
  left: 38px;
  bottom: calc(100% + 6px);
  z-index: 5;
  width: min(240px, 70vw);
  max-height: 220px;
  overflow: auto;
  padding: 6px;
  border-radius: 12px;
  background: var(--cat-elevated);
  border: 1px solid var(--cat-border);
  box-shadow: var(--cat-shadow-lg);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.ct-extra .ex {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-width: 0;
  color: var(--cat-text);
  border-radius: 8px;
  padding: 4px 6px 4px 4px;
}
.ct-extra .ex-open {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  border: none;
  background: transparent;
  color: inherit;
  font: inherit;
  padding: 2px 4px;
  text-align: left;
  cursor: default;
}
.ct-extra .ex-open.on {
  cursor: pointer;
}
.ct-extra .ex-open.media {
  flex: none;
  padding: 0;
}
.ct-extra .ex:hover {
  background: var(--cat-hover);
}
.ct-extra .rx {
  flex: none;
  width: 18px;
  height: 18px;
  border: none;
  border-radius: 50%;
  padding: 0;
  display: grid;
  place-items: center;
  background: transparent;
  color: var(--cat-muted);
  cursor: pointer;
}
.ct-extra .rx:hover {
  color: var(--cat-text);
}
.ct-extra .ex b {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  font-weight: 500;
}
.ct-send {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: none;
  display: grid;
  place-items: center;
  background: var(--cat-send);
  color: var(--cat-on-send);
  cursor: pointer;
  flex: none;
  transition: background var(--ff-dur-quick), opacity var(--ff-dur-quick), transform var(--ff-dur-quick);
}
/* G4 发送键：hover 放大 1.06，按下 .94 */
.ct-send:not(:disabled):not([aria-disabled='true']):hover {
  transform: scale(1.06);
}
.ct-send:not(:disabled):not([aria-disabled='true']):active,
.ct-plus:active,
.ct-more:active,
.chip-access:active,
.chip-model:active {
  transform: scale(0.94);
}
/* 没内容时次要灰 */
.ct-send[aria-disabled='true']:not(.blocked) {
  opacity: 0.35;
}
.ct-send[aria-disabled='true'] {
  cursor: default;
}
/* 未就绪 / 检查中：发送置灰（两种状态同一样式） */
.ct-send.blocked,
.ct-send.blocked:hover {
  cursor: default;
  opacity: 0.4;
}
.ct-stop .sq {
  width: 10px;
  height: 10px;
  border-radius: 2px;
  background: currentColor;
}
.ct-stop:disabled,
.ct-stop:disabled:hover {
  cursor: default;
  opacity: 0.4;
}

/* 批准方式胶囊（全部批准时橙色） */
.chip-access {
  height: 28px;
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 0 10px 0 8px;
  border-radius: 14px;
  border: 1px solid transparent;
  background: transparent;
  font-size: 12.5px;
  font-weight: 500;
  color: var(--cat-text-2);
  cursor: pointer;
  white-space: nowrap;
  transition: background var(--ff-dur-quick), border-color var(--ff-dur-quick), color var(--ff-dur-quick), transform var(--ff-dur-quick);
}
.chip-access:hover {
  background: var(--cat-hover);
}
.chip-access[aria-expanded='true'] {
  background: var(--cat-hover);
}
/* 全部批准：橙色只用于这一项和胶囊 */
.chip-access.on {
  color: var(--cat-orange);
  background: var(--cat-orange-soft);
  border-color: var(--cat-orange-line);
}
.chip-access.on:hover,
.chip-access.on[aria-expanded='true'] {
  background: color-mix(in srgb, var(--cat-orange) 16%, transparent);
}

/* 合一模型胶囊 */
.chip-model {
  height: 30px;
  min-width: 0;
  flex: 0 1 auto;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 8px 0 9px;
  border-radius: 15px;
  border: none;
  background: transparent;
  font-size: 14px;
  color: var(--cat-text);
  cursor: pointer;
  white-space: nowrap;
  transition: background var(--ff-dur-quick), transform var(--ff-dur-quick);
}
.chip-model > :deep(svg) {
  flex: none;
  color: var(--cat-text-2);
}
.chip-model span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.chip-model > :deep(svg.caret) {
  color: var(--cat-muted);
}
.chip-model:hover,
.chip-model[aria-expanded='true'] {
  background: var(--cat-hover);
}
/* H1 统一双层焦点环，只在键盘聚焦时 */
.chip-model:focus-visible,
.chip-access:focus-visible,
.ct-plus:focus-visible,
.ct-more:focus-visible,
.ct-send:focus-visible,
.mn-acc:focus-visible,
.mn-row:focus-visible,
.mn-op:focus-visible,
.wl-proj:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}

/* 菜单：外层只定位，在输入框上方 8px */
.mn {
  position: absolute;
  bottom: calc(100% + 8px);
  z-index: 30;
  transform-origin: left bottom;
  animation: mnUp var(--ff-dur-base) var(--ff-ease-out) both;
}
@keyframes mnUp {
  from { opacity: 0; transform: translateY(4px) scale(0.98); }
  to { opacity: 1; transform: none; }
}
@keyframes subIn {
  from { opacity: 0; transform: translateX(4px); }
  to { opacity: 1; transform: none; }
}
.mn-access {
  width: min(340px, 100%);
  box-sizing: border-box;
  padding: 10px;
  background: var(--cat-elevated);
  border: 1px solid var(--cat-border);
  border-radius: 14px;
  box-shadow: var(--cat-shadow-lg);
}
.mn-ahd {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 2px 6px 10px;
  font-size: 12px;
  color: var(--cat-muted);
}
.mn-ahd .more {
  color: var(--cat-text-2);
  text-decoration: underline;
  font-size: 12px;
}
.mn-ahd .more:hover {
  color: var(--cat-text);
}
.mn-acc {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px;
  border-radius: 10px;
  cursor: pointer;
  color: var(--cat-text);
  transition: background var(--ff-dur-quick);
}
.mn-acc:hover {
  background: var(--cat-hover);
}
.mn-acc .ic {
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
  flex: none;
  margin-top: 1px;
}
.mn-acc div {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.mn-acc b {
  font-size: 13.5px;
  font-weight: 600;
}
.mn-acc small {
  font-size: 12px;
  color: var(--cat-muted);
  line-height: 1.45;
}
.mn-acc .ok {
  margin-top: 2px;
  color: var(--cat-active-text);
}
.mn-acc.on {
  background: var(--cat-active);
}
.mn-acc.on b {
  color: var(--cat-active-text);
}
/* D2 危险项：分隔线下面，唯一的橙色项 */
.mn-sep {
  height: 1px;
  margin: 6px 4px;
  background: var(--cat-divider);
}
.mn-acc.danger .ic,
.mn-acc.danger b,
.mn-acc.danger .ok {
  color: var(--cat-orange);
}
.mn-acc.danger:hover,
.mn-acc.danger.on {
  background: var(--cat-orange-soft);
}
.mn-acc.disabled,
.mn-acc.disabled:hover {
  opacity: 0.45;
  cursor: not-allowed;
  background: transparent;
  color: var(--cat-muted);
}
.mn-acc.disabled b,
.mn-acc.disabled small {
  color: var(--cat-muted);
}

.mn-casc {
  position: relative;
}
.mn-main,
.mn-sub {
  background: var(--cat-elevated);
  border: 1px solid var(--cat-border);
  border-radius: var(--cat-radius-md);
  box-shadow: var(--cat-shadow-lg);
  padding: 4px;
}
.mn-main {
  width: 182px;
}
.mn-row {
  height: 38px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px 0 10px;
  border-radius: 4px;
  font-size: 14px;
  color: var(--cat-text);
  cursor: pointer;
  white-space: nowrap;
}
.mn-row .v {
  margin-left: auto;
  font-size: 13.5px;
  color: var(--cat-muted);
}
.mn-row:hover,
.mn-row.on,
.mn-op:hover,
.mn-op.on {
  background: var(--cat-hover);
}
.mn-sub {
  position: absolute;
  right: calc(100% + 3px);
  width: 180px;
  animation: subIn var(--ff-dur-quick) var(--ff-ease-out) both;
}
.mn-op {
  height: 36px;
  display: flex;
  align-items: center;
  padding: 0 10px 0 8px;
  border-radius: 4px;
  font-size: 14px;
  color: var(--cat-text);
  cursor: pointer;
  white-space: nowrap;
}
.mn-op .ck {
  width: 28px;
  flex: none;
  display: flex;
  align-items: center;
  color: var(--cat-active-text);
}

/* 欢迎页：输入框下方的灰色底条，顶边被输入框压住 */
.wl-tray {
  position: relative;
  z-index: 0;
  margin-top: -20px;
  padding: 26px 18px 10px;
  background: var(--cat-sunken);
  border-radius: 0 0 22px 22px;
  display: flex;
  align-items: center;
}
.wl-proj {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: none;
  background: none;
  font-size: 13.5px;
  color: var(--cat-text-2);
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 4px;
}
.wl-proj:not(.on) {
  color: var(--cat-muted);
}
.wl-proj:hover {
  color: var(--cat-text);
}
.wl-proj :deep(svg) {
  color: var(--cat-muted);
}
</style>
