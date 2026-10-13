<template>
  <section class="ct-l" aria-label="会话">
    <div class="ct-brand">
      <div class="lg"><FIcon name="cat" :size="16" /></div>
      <b>Cat</b>
      <button type="button" class="ct-back" title="返回原页面并展开侧栏" @click="emit('back')"><FIcon name="left" :size="14" />返回</button>
    </div>
    <button type="button" class="ct-it new" :class="{ on: catState.sel === NEW_CONV && !catState.newProjectId }" @click="selectConv(NEW_CONV)">
      <FIcon name="plus" :size="16" /><span class="t">新对话</span><span class="r"><FIcon name="list" :size="16" /></span>
    </button>
    <!-- C1：「助手」一期不做，已去掉；「定时任务」等真入口（设置 / 定时任务 PR）接上前先隐藏，改 SHOW_SCHEDULE_ENTRY 即可恢复 -->
    <button v-if="SHOW_SCHEDULE_ENTRY" type="button" class="ct-it disabled" aria-disabled="true" :title="laterTip" @click="onLater">
      <FIcon name="clock" :size="16" /><span class="t">定时任务</span>
    </button>

    <div ref="scrollEl" class="ct-scroll">
      <div class="ct-sec">
        <span>项目</span>
        <span class="acts">
          <!-- 设计「项目状态 v0.1」§01：标题行只有这一个入口 -->
          <button
            ref="newBtn"
            type="button"
            class="ib"
            :title="PC.newProject"
            :aria-label="PC.newProject"
            data-testid="cat-new-project"
            @click="createProject()"
          ><FIcon name="folder-plus" :size="14" /></button>
        </span>
      </div>
      <!-- 没有项目：标题行保留，下面一行淡灰小字（不画插画，一行高） -->
      <div v-if="!catState.projects.length" class="pj-empty" data-testid="cat-no-projects">{{ PC.empty }}</div>
      <div v-else class="pjs">
        <div
          v-for="p in catState.projects"
          :key="p.id"
          class="pj"
          :class="{ open: p.open, missing: p.missing, hl: catState.highlight === p.id, menu: menuFor === p.id }"
          :data-pid="p.id"
        >
          <div v-if="catState.renaming === p.id" class="pj-ren">
            <div class="pj-row editing">
              <FIcon name="right" :size="12" class="car" />
              <FIcon :name="p.open ? 'folder-open' : 'folder'" :size="16" />
              <input
                ref="renameEl"
                v-model="renameDraft"
                class="ren-in"
                :class="{ bad: renameBad }"
                :aria-label="PC.renameLabel"
                :aria-invalid="renameBad"
                :aria-describedby="`ren-hint-${p.id}`"
                data-testid="cat-project-rename"
                @keydown.enter.prevent="commitRename(p.id)"
                @keydown.esc.prevent.stop="cancelRename(p.id)"
                @blur="onRenameBlur(p.id)"
              >
            </div>
            <div
              :id="`ren-hint-${p.id}`"
              class="ren-hint"
              :class="{ bad: renameBad }"
              :role="renameBad ? 'alert' : undefined"
              data-testid="cat-project-rename-hint"
            >{{ renameBad ? PC.badName : PC.renameHint }}</div>
          </div>
          <div
            v-else
            class="pj-row"
            role="button"
            tabindex="0"
            :aria-expanded="p.open"
            :aria-label="p.missing ? `${p.name}，${PC.missing}` : p.name"
            :title="p.missing ? PC.missing : p.path || p.name"
            data-testid="cat-project-row"
            @click="toggle(p, $event)"
            @keydown.enter.self="toggle(p, $event)"
          >
            <FIcon name="right" :size="12" class="car" />
            <FIcon :name="p.open ? 'folder-open' : 'folder'" :size="16" />
            <span class="nm">
              <span class="t">{{ p.name }}</span>
              <span v-if="p.missing" class="miss" data-testid="cat-project-missing"><FIcon name="warn" :size="11" />{{ PC.missing }}</span>
            </span>
            <span class="pj-act">
              <button
                type="button"
                class="ib"
                :aria-label="PC.menu"
                :title="PC.menu"
                aria-haspopup="menu"
                :aria-expanded="menuFor === p.id"
                data-testid="cat-project-more"
                @click.stop="openMenu(p.id, $event)"
              ><FIcon name="more" :size="14" /></button>
              <button
                type="button"
                class="ib"
                :class="{ off: p.missing }"
                :title="p.missing ? PC.missing : PC.newConvInProject"
                :aria-label="PC.newConvInProject"
                :aria-disabled="p.missing"
                data-testid="cat-project-new-conv"
                @click.stop="!p.missing && newConvInProject(p.id)"
              ><FIcon name="plus" :size="14" /></button>
            </span>
          </div>
          <div class="pj-kids">
            <div class="pj-in">
              <div v-if="!p.convs.length" class="pj-none">{{ PC.emptyConvs }}</div>
              <div
                v-for="c in p.convs"
                :key="c.id"
                class="pc"
                :class="{ on: c.id === catState.sel, menu: cmenuFor === c.id }"
                role="button"
                tabindex="0"
                :data-cid="c.id"
                data-testid="cat-conv-row"
                @click="onConvClick(c.id, $event)"
                @keydown.enter.self="selectConv(c.id)"
              >
                <button
                  type="button"
                  class="ib c-more"
                  :aria-label="CC.menu"
                  :title="CC.menu"
                  aria-haspopup="menu"
                  :aria-expanded="cmenuFor === c.id"
                  data-testid="cat-conv-more"
                  @click.stop="openConvMenu(c.id, $event)"
                ><FIcon name="more" :size="14" /></button>
                <div class="l1">
                  <i
                    class="st"
                    :class="rowDot(c.id, c.st)"
                    :role="rowDotLabel(c.id, c.st) ? 'img' : undefined"
                    :aria-label="rowDotLabel(c.id, c.st) || undefined"
                    :aria-hidden="rowDotLabel(c.id, c.st) ? undefined : true"
                  />
                  <i v-if="catState.unseen[c.id]" class="note" aria-label="有新回复" />
                  <span class="t" :title="c.title || '新对话'">{{ c.title || '新对话' }}</span>
                  <span v-if="c.main" class="main-tag">主要</span>
                </div>
                <div v-if="c.sub || c.time" class="l2"><span class="t" :title="c.sub">{{ c.sub }}</span><span class="tm">{{ c.time }}</span></div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div class="ct-sec"><span>对话</span></div>
      <!-- 普通对话行：里面要放「···」按钮，所以行本身不能是 <button> -->
      <div
        v-for="c in catState.plain"
        :key="c.id"
        class="ct-it dc"
        :class="{ on: c.id === catState.sel, menu: cmenuFor === c.id }"
        role="button"
        tabindex="0"
        :data-cid="c.id"
        data-testid="cat-conv-row"
        @click="onConvClick(c.id, $event)"
        @keydown.enter.self="selectConv(c.id)"
      >
        <FIcon name="chat" :size="16" /><i v-if="rowDot(c.id, c.st)" class="st" :class="rowDot(c.id, c.st)" role="img" :aria-label="rowDotLabel(c.id, c.st)" /><i v-if="catState.unseen[c.id]" class="note" aria-label="有新回复" /><span class="t" :title="c.title || '新对话'">{{ c.title || '新对话' }}</span>
        <button
          type="button"
          class="ib c-more"
          :aria-label="CC.menu"
          :title="CC.menu"
          aria-haspopup="menu"
          :aria-expanded="cmenuFor === c.id"
          data-testid="cat-conv-more"
          @click.stop="openConvMenu(c.id, $event)"
        ><FIcon name="more" :size="14" /></button>
      </div>
    </div>
    <div class="ct-it"><FIcon name="set" :size="16" /><span class="t">设置</span></div>

    <!-- 项目菜单（设计 §02）：fixed 定位，可越过侧栏右边界 -->
    <div
      v-if="menuProject"
      ref="menuEl"
      class="pj-menu"
      role="menu"
      :aria-label="PC.menu"
      :style="menuStyle"
      data-testid="cat-project-menu"
      @keydown="onMenuKey"
    >
      <div class="mi" role="menuitem" tabindex="-1" data-testid="cat-project-menu-rename" @click="startRename(menuProject.id)">
        <FIcon name="edit" :size="15" />{{ PC.rename }}
      </div>
      <!-- 设计 v0.2 §09a：灰态菜单 = 重命名 / 重新选择文件夹 / {显示文件夹}（禁用） / 删除项目 -->
      <div
        v-if="menuProject.missing"
        class="mi"
        role="menuitem"
        tabindex="-1"
        data-testid="cat-project-menu-relocate"
        @click="onRelocate(menuProject.id)"
      >
        <FIcon name="refresh" :size="15" />{{ PC.relocate }}
      </div>
      <div
        class="mi"
        :class="{ dis: menuProject.missing }"
        role="menuitem"
        tabindex="-1"
        :aria-disabled="menuProject.missing"
        data-testid="cat-project-menu-reveal"
        @click="onReveal(menuProject)"
      >
        <FIcon name="folder-open" :size="15" />{{ revealText }}
      </div>
      <div class="sep" role="separator" />
      <div class="mi danger" role="menuitem" tabindex="-1" data-testid="cat-project-menu-delete" @click="askDelete(menuProject.id)">
        <FIcon name="trash" :size="15" />{{ PC.del }}
      </div>
    </div>
    <!-- 对话菜单（设计 v0.3 §11/12）：只有红色「删除」；与项目菜单互斥 -->
    <div
      v-if="cmenuFor"
      ref="cmenuEl"
      class="pj-menu c-menu"
      role="menu"
      :aria-label="CC.menu"
      :style="menuStyle"
      data-testid="cat-conv-menu"
      @keydown="onMenuKey"
    >
      <div class="mi danger" role="menuitem" tabindex="-1" data-testid="cat-conv-menu-delete" @click="askDeleteConv(cmenuFor)">
        <FIcon name="trash" :size="15" />{{ CC.del }}
      </div>
    </div>
    <div class="sr-only" role="status" aria-live="polite">{{ catState.announce }}</div>
  </section>
</template>

<script setup lang="ts">
/** 定时任务入口开关：真入口落地前隐藏 */
const SHOW_SCHEDULE_ENTRY = false
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { CAT_CONV_COPY as CC, CAT_COPY } from '@/api/cat'
import { CAT_PROJECT_COPY as PC, CAT_PROJECT_NAME_MAX } from '@/api/catProjects'
import type { CatProject } from '@/api/catMock'
import { convDot, convDotLabel } from '@/views/cat/catConvDot'
import {
  catState,
  createProject,
  findConv,
  newConvInProject,
  NEW_CONV,
  relocateProject,
  renameProject,
  revealProject,
  revealText,
  selectConv,
} from '@/views/cat/catState'

/** 标题左边的点：进行中读 turns，上次结果读 outcome，走查数据才用 c.st */
function rowDot(id: string, stored?: string) {
  return convDot(!!catState.turns[id], catState.outcome[id] ?? '', stored)
}
function rowDotLabel(id: string, stored?: string) {
  return convDotLabel(rowDot(id, stored))
}

const emit = defineEmits<{ back: [] }>()
const laterTip = CAT_COPY.later

const scrollEl = ref<HTMLElement>()
const menuEl = ref<HTMLElement>()
const cmenuEl = ref<HTMLElement>()
const renameEl = ref<HTMLInputElement[] | HTMLInputElement>()

function rowOf(id: string): HTMLElement | null {
  return scrollEl.value?.querySelector<HTMLElement>(`.pj[data-pid="${CSS.escape(id)}"] .pj-row`) ?? null
}

function toggle(p: CatProject, e: Event) {
  if ((e.target as HTMLElement).closest('.pj-act')) return
  p.open = !p.open
}
function onLater() {
  ElMessage.info(CAT_COPY.later)
}

// ---- 菜单 ----
const menuFor = ref('')
const menuStyle = ref<Record<string, string>>({})
const menuProject = computed(() => catState.projects.find((p) => p.id === menuFor.value))
let menuBtn: HTMLElement | null = null

async function openMenu(id: string, e: Event) {
  if (menuFor.value === id) return closeMenu(true)
  cmenuFor.value = ''
  menuBtn = e.currentTarget as HTMLElement
  const r = menuBtn.getBoundingClientRect()
  menuStyle.value = { left: `${r.left}px`, top: `${r.bottom + 4}px` }
  menuFor.value = id
  await nextTick()
  items()[0]?.focus()
}
function closeMenu(refocus = false) {
  menuFor.value = ''
  cmenuFor.value = ''
  if (refocus) menuBtn?.focus()
}
function items(): HTMLElement[] {
  const el = cmenuFor.value ? cmenuEl.value : menuEl.value
  return Array.from(el?.querySelectorAll<HTMLElement>('.mi:not(.dis)') ?? [])
}

// ---- 对话菜单（设计 v0.3 §11/12）：点「···」只开菜单、不切换对话 ----
const cmenuFor = ref('')
async function openConvMenu(id: string, e: Event) {
  if (cmenuFor.value === id) return closeMenu(true)
  menuFor.value = ''
  menuBtn = e.currentTarget as HTMLElement
  const r = menuBtn.getBoundingClientRect()
  menuStyle.value = { left: `${r.left}px`, top: `${r.bottom + 4}px` }
  cmenuFor.value = id
  await nextTick()
  items()[0]?.focus()
}
function onConvClick(id: string, e: Event) {
  if ((e.target as HTMLElement).closest('.c-more')) return
  selectConv(id)
}
function askDeleteConv(id: string) {
  closeMenu()
  catState.deletingConv = id
}
// 对话没了（删除 / 列表刷新）时收起它的菜单
watch(
  () => cmenuFor.value && !findConv(cmenuFor.value),
  (gone) => {
    if (gone) cmenuFor.value = ''
  },
)
function onMenuKey(e: KeyboardEvent) {
  const list = items()
  const i = list.indexOf(document.activeElement as HTMLElement)
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    const n = list.length
    list[(i + (e.key === 'ArrowDown' ? 1 : -1) + n) % n]?.focus()
  } else if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    ;(document.activeElement as HTMLElement)?.click()
  } else if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    closeMenu(true)
  } else if (e.key === 'Tab') closeMenu()
}
function onDocDown(e: MouseEvent) {
  if (!menuFor.value && !cmenuFor.value) return
  const t = e.target as Node
  if (menuEl.value?.contains(t) || cmenuEl.value?.contains(t) || menuBtn?.contains(t)) return
  closeMenu()
}

function onReveal(p: CatProject) {
  if (p.missing) return
  closeMenu()
  void revealProject(p.id)
}
function onRelocate(id: string) {
  closeMenu()
  void relocateProject(id)
}
function askDelete(id: string) {
  closeMenu()
  catState.deleting = id
}

// ---- 改名（设计 §03 / v0.2 §10c：全选原名；回车 / 失焦保存；Esc 取消；清空 = 恢复文件夹名；超过 60 字实时红框，回车不保存、失焦 / Esc 放弃） ----
const renameDraft = ref('')
/** 实时校验：只看上限（清空不是错误，保存时恢复为文件夹名） */
const renameBad = computed(() => [...renameDraft.value.trim()].length > CAT_PROJECT_NAME_MAX)
let saving = false
async function startRename(id: string) {
  closeMenu()
  const p = catState.projects.find((x) => x.id === id)
  if (!p) return
  renameDraft.value = p.name
  catState.renaming = id
  await nextTick()
  const el = Array.isArray(renameEl.value) ? renameEl.value[0] : renameEl.value
  el?.focus()
  el?.select()
}
function onRenameBlur(id: string) {
  if (renameBad.value) cancelRename(id)
  else void commitRename(id)
}
async function commitRename(id: string) {
  if (saving || catState.renaming !== id || renameBad.value) return
  saving = true
  try {
    const done = await renameProject(id, renameDraft.value)
    if (done) finishRename(id)
  } finally {
    saving = false
  }
}
function cancelRename(id: string) {
  finishRename(id)
}
function finishRename(id: string) {
  if (catState.renaming !== id) return
  catState.renaming = ''
  nextTick(() => rowOf(id)?.focus())
}

// ---- 新建 / 重复定位：滚到可见、焦点移到该行 ----
watch(
  () => catState.highlight,
  async (id) => {
    if (!id) return
    await nextTick()
    const row = rowOf(id)
    row?.scrollIntoView({ block: 'nearest' })
    row?.focus({ preventScroll: true })
  },
)

onMounted(() => document.addEventListener('mousedown', onDocDown, true))
onBeforeUnmount(() => document.removeEventListener('mousedown', onDocDown, true))
</script>

<style scoped>
.ct-l {
  width: 248px;
  flex: none;
  display: flex;
  flex-direction: column;
  background: var(--cat-rail);
  border-right: 1px solid var(--cat-border);
  padding: 10px 8px 12px;
  min-height: 0;
}
.ct-brand {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 2px 4px 12px;
  --wails-draggable: drag;
}
.ct-brand .lg {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: var(--cat-text);
  color: var(--cat-surface);
  display: grid;
  place-items: center;
  flex: none;
}
.ct-brand b {
  font-size: 14px;
  font-weight: 600;
  flex: 1;
}
.ct-back {
  height: 28px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 8px;
  border-radius: 6px;
  font: inherit;
  font-size: 12px;
  color: var(--cat-text-2);
  border: 1px solid var(--cat-border);
  background: var(--cat-surface);
  cursor: pointer;
  --wails-draggable: no-drag;
  transition: background var(--ff-dur-quick), color var(--ff-dur-quick), transform var(--ff-dur-quick);
}
.ct-back:active {
  transform: scale(0.94);
}
.ct-back:hover {
  color: var(--cat-text);
  background: var(--cat-hover);
}
.ct-it {
  width: 100%;
  height: 32px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  border-radius: 6px;
  border: none;
  background: transparent;
  font: inherit;
  font-size: 13px;
  color: var(--cat-text);
  text-align: left;
  flex: none;
  position: relative;
  transition: background var(--ff-dur-quick), color var(--ff-dur-quick);
}
.ct-it > :deep(svg) {
  color: var(--cat-text-2);
}
.ct-it .r {
  margin-left: auto;
  color: var(--cat-muted);
  display: flex;
}
button.ct-it,
.ct-it.dc {
  cursor: pointer;
}
button.ct-it:hover,
.ct-it.dc:hover,
.ct-it.dc.menu,
.pc.menu,
.pj-row:hover,
.pc:hover {
  background: var(--cat-hover);
}
/* B2 选中 = 主色 10% 底 + 主色文字 + 左侧 3px 指示条；hover = 中性 5% 底 */
.ct-it.on,
.ct-it.on:hover {
  background: var(--cat-active);
  color: var(--cat-active-text);
  font-weight: 500;
}
.ct-it.on > :deep(svg) {
  color: var(--cat-active-text);
}
.ct-it.on:not(.new)::before,
.pc.on::before {
  content: '';
  position: absolute;
  left: 0;
  top: 7px;
  bottom: 7px;
  width: 3px;
  border-radius: 0 2px 2px 0;
  background: var(--cat-accent);
}
.ct-it .t {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.ct-it.new {
  border: 1px solid var(--cat-border);
  background: var(--cat-surface);
  margin-bottom: 4px;
  font-weight: 500;
}
.ct-it.new.on {
  background: var(--cat-surface);
  border-color: var(--cat-accent);
  color: var(--cat-active-text);
}
.ct-sec {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: var(--cat-text-2);
  padding: 14px 8px 4px;
}
.ct-sec .acts {
  display: flex;
  gap: 2px;
}
.ct-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  margin: 0 -8px;
  padding: 0 8px;
  scrollbar-width: thin;
}
.ib {
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
  border-radius: 5px;
  color: var(--cat-text-2);
  cursor: pointer;
  transition: background var(--ff-dur-quick), color var(--ff-dur-quick), transform var(--ff-dur-quick);
}
.ib:active {
  transform: scale(0.94);
}
.ib:hover {
  background: var(--cat-hover);
  color: var(--cat-text);
}
.pj-empty {
  height: 24px;
  line-height: 24px;
  padding: 0 8px;
  font-size: 12px;
  color: var(--cat-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.pj-row {
  height: 32px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 6px 0 4px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--cat-text);
  cursor: pointer;
  transition: background var(--ff-dur-quick);
}
.pj-row > :deep(svg) {
  color: var(--cat-text-2);
}
.pj-row > :deep(svg.car) {
  color: var(--cat-muted);
  transition: transform var(--ff-dur-base) var(--ff-ease-out);
}
.pj.open .pj-row > :deep(svg.car) {
  transform: rotate(90deg);
}
.pj-row .nm {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.pj-row .t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pj-act {
  display: flex;
  gap: 2px;
  opacity: 0;
  transition: opacity var(--ff-dur-quick);
}
.pj-row:hover .pj-act,
.pj-row:focus-within .pj-act,
.pj-row:focus-visible .pj-act {
  opacity: 1;
}
.pj-kids {
  display: grid;
  grid-template-rows: 0fr;
  transition: grid-template-rows var(--ff-dur-base) var(--ff-ease-out);
}
.pj.open .pj-kids {
  grid-template-rows: 1fr;
}
.pj-in {
  opacity: 0;
  transition: opacity var(--ff-dur-base) var(--ff-ease-out);
}
.pj.open .pj-in {
  opacity: 1;
}
.pj-in {
  overflow: hidden;
  min-height: 0;
  padding-left: 14px;
}
.pc {
  padding: 5px 8px 6px;
  border-radius: 8px;
  margin: 2px 0;
  border: 1px solid transparent;
  cursor: pointer;
  transition: background var(--ff-dur-quick);
}
.pc.on,
.pc.on:hover {
  background: var(--cat-active);
}
.pc.on .l1 {
  color: var(--cat-active-text);
}
.pc .l1 {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--cat-text);
  font-weight: 500;
  height: 20px;
}
.pc .l2 {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  color: var(--cat-muted);
  padding-left: 14px;
  height: 18px;
}
.pc .t {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pc .l1 .t {
  flex: 0 1 auto;
}
.pc .tm {
  flex: none;
  font-variant-numeric: tabular-nums;
}
.st {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
  background: var(--cat-muted);
  box-sizing: border-box;
}
.st.ok {
  background: var(--cat-success);
}
.st.idle {
  background: transparent;
  border: 1.5px solid var(--cat-muted);
  width: 7px;
  height: 7px;
}
.st.run {
  width: 10px;
  height: 10px;
  background: transparent;
  /* C5 回复中 = 主色呼吸点 */
  width: 8px;
  height: 8px;
  background: var(--cat-accent);
  animation: cat-breathe 1.4s ease-in-out infinite;
}
.st.bad {
  width: auto;
  height: auto;
  border: none;
  border-radius: 0;
  background: none;
  color: var(--cat-danger-text);
  font-style: normal;
  font-weight: 800;
  font-size: 15px;
  line-height: 1;
  font-family: inherit;
}
.st.bad::before {
  content: '!';
}
@media (prefers-reduced-motion: reduce) {
  .st.run {
    animation: none;
  }
}
.note {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
  background: var(--cat-success); /* C5 绿点 = 有新回复未读 */
}
@keyframes ct-spin {
  to { transform: rotate(360deg); }
}
.main-tag {
  flex: none;
  font-size: 10.5px;
  line-height: 16px;
  padding: 0 5px;
  border-radius: 4px;
  border: 1px solid var(--cat-border);
  color: var(--cat-text-2);
  background: var(--cat-hover);
  font-weight: 500;
}
button.ct-it:focus-visible,
.ct-it.dc:focus-visible,
.pj-row:focus-visible,
.pc:focus-visible,
.ct-back:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.ct-it.disabled,
.ct-it.disabled:hover {
  color: var(--cat-muted);
  background: transparent;
  cursor: not-allowed;
  opacity: 0.55;
}
.ct-it.disabled > :deep(svg) {
  color: var(--cat-muted);
}
button.ib {
  border: none;
  background: transparent;
  padding: 0;
  font: inherit;
}
.ib:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.ib.off,
.ib.off:hover {
  color: var(--cat-muted);
  background: transparent;
  opacity: 0.45;
  cursor: not-allowed;
}
/* 菜单打开时：该行保持 hover 底色，按钮可见、「···」加深 */
.pj.menu .pj-row {
  background: var(--cat-hover);
}
.pj.menu .pj-act {
  opacity: 1;
}
.pj.menu .pj-act .ib[aria-expanded='true'] {
  background: var(--cat-border);
  color: var(--cat-text);
}
/* 新建 / 重复定位高亮：主色淡底 + 1px 主色描边，1.6s 淡出 */
.pj-row {
  border: 1px solid transparent;
}
.pj.hl .pj-row {
  animation: pj-hl 2.4s ease forwards;
}
@keyframes pj-hl {
  0%, 33% {
    background: color-mix(in srgb, var(--cat-accent) 10%, transparent);
    border-color: var(--cat-accent);
  }
  100% {
    background: transparent;
    border-color: transparent;
  }
}
.pj.hl {
  animation: pj-in var(--ff-dur-base) var(--ff-ease-out) both;
}
@keyframes pj-in {
  from { opacity: 0; transform: translateY(-4px); }
  to { opacity: 1; transform: none; }
}
/* 文件夹不见了：名字 / 图标转灰，第二行提示；对话仍可点开 */
.pj.missing .pj-row {
  height: auto;
  min-height: 32px;
  padding-top: 4px;
  padding-bottom: 4px;
  color: var(--cat-muted);
}
.pj.missing .pj-row > :deep(svg) {
  color: var(--cat-muted);
}
.pj.missing .pc .l1 {
  color: var(--cat-text-2);
}
.miss {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 11.5px;
  font-weight: 400;
  color: var(--cat-muted);
  line-height: 16px;
  white-space: nowrap;
}
.miss :deep(svg) {
  color: var(--cat-warn);
  flex: none;
}
.pj-none {
  height: 24px;
  line-height: 24px;
  padding: 0 8px;
  font-size: 12px;
  color: var(--cat-muted);
}
/* 改名：行内输入框 + 淡灰提示 */
.pj-row.editing {
  cursor: default;
  background: transparent;
}
.ren-in {
  flex: 1;
  min-width: 0;
  height: 26px;
  padding: 0 6px;
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  color: var(--cat-text);
  background: var(--cat-surface);
  border: 1px solid var(--cat-accent);
  border-radius: 5px;
  outline: none;
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cat-accent) 20%, transparent);
}
.ren-hint {
  padding: 2px 0 4px 26px;
  font-size: 11.5px;
  color: var(--cat-muted);
}
.ren-in.bad {
  border-color: var(--cat-danger);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cat-danger) 20%, transparent);
}
.ren-hint.bad {
  color: var(--cat-danger);
}
/* 对话行「···」（设计 v0.3 §11/12）：项目下对话在卡片右上角，普通对话在行右侧垂直居中；hover / 键盘聚焦淡入 120ms */
.pc {
  position: relative;
}
.c-more {
  opacity: 0;
  transition: opacity var(--ff-dur-quick);
  flex: none;
}
.pc .c-more {
  position: absolute;
  top: 4px;
  right: 6px;
  z-index: 1;
  background: var(--cat-hover);
}
.pc.on .c-more {
  background: var(--cat-elevated);
}
.ct-it.dc .c-more {
  margin-left: auto;
}
.pc:hover .c-more,
.pc:focus-within .c-more,
.pc.menu .c-more,
.ct-it.dc:hover .c-more,
.ct-it.dc:focus-within .c-more,
.ct-it.dc.menu .c-more {
  opacity: 1;
}
.pc.menu .c-more,
.ct-it.dc.menu .c-more {
  background: var(--cat-border);
  color: var(--cat-text);
}
.c-menu {
  width: 140px;
}
/* 项目菜单 */
.pj-menu {
  position: fixed;
  z-index: 3000;
  width: 200px;
  padding: 4px;
  border-radius: 10px;
  background: var(--cat-elevated);
  border: 1px solid var(--cat-border);
  box-shadow: var(--cat-shadow-lg);
  animation: pj-menu var(--ff-dur-base) var(--ff-ease-out) both;
  transform-origin: top left;
}
@keyframes pj-menu {
  from { opacity: 0; transform: translateY(4px) scale(0.98); }
  to { opacity: 1; transform: none; }
}
.pj-menu .mi {
  height: 32px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--cat-text);
  cursor: pointer;
  outline: none;
}
.pj-menu .mi :deep(svg) {
  color: var(--cat-text-2);
}
.pj-menu .mi:hover,
.pj-menu .mi:focus-visible,
.pj-menu .mi:focus {
  background: var(--cat-hover);
}
.pj-menu .mi.dis {
  color: var(--cat-muted);
  cursor: not-allowed;
  opacity: 0.55;
}
.pj-menu .mi.dis:hover {
  background: transparent;
}
.pj-menu .mi.danger,
.pj-menu .mi.danger :deep(svg) {
  color: var(--cat-danger);
}
.pj-menu .mi.danger:hover,
.pj-menu .mi.danger:focus {
  background: color-mix(in srgb, var(--cat-danger) 10%, transparent);
}
.pj-menu .sep {
  height: 1px;
  margin: 4px 6px;
  background: var(--cat-divider);
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
@media (prefers-reduced-motion: reduce) {
  .pj.hl,
  .pj-menu {
    animation: none;
  }
  .c-more,
  .pj-kids,
  .pj-in,
  .pj-row > :deep(svg.car) {
    transition: none;
  }
  .pj.hl .pj-row {
    animation: pj-hl 2.4s steps(1, end) forwards;
  }
}
</style>
