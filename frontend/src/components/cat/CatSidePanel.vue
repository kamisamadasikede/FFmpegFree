<template>
  <section ref="rootEl" class="ct-l" :class="{ mini: railCollapsed, dragging: sideDragging }" :aria-label="t('cat.misc.s_9a834e9a3e')">
    <button
      type="button"
      class="ct-rail-toggle"
      :title="t(railCollapsed ? 'cat.misc.s_c209364b39' : 'cat.misc.s_9401deaaf6')"
      :aria-label="t(railCollapsed ? 'cat.misc.s_e4517cb153' : 'cat.misc.s_843e43c606')"
      :aria-expanded="!railCollapsed"
      :data-testid="railCollapsed ? 'cat-rail-expand' : 'cat-rail-collapse'"
      @pointerdown.stop
      @click="railCollapsed = !railCollapsed"
    ><FIcon :name="railCollapsed ? 'right' : 'left'" :size="14" /></button>
    <!-- 收起后保留品牌、新对话、工具箱和设置入口。 -->
    <div v-if="railCollapsed" class="ct-mini">
      <div class="ct-mini-brand lg" :title="AGENT_NAME"><FIcon name="bot" :size="16" /></div>
      <button type="button" class="ib big" :class="{ on: catState.sel === NEW_CONV }" :title="t('cat.misc.s_1ac07a4bb2')" :aria-label="t('cat.misc.s_1ac07a4bb2')" @click="startNewConv()"><FIcon name="edit" :size="16" /></button>
      <span class="sp" />
      <RouterLink to="/cat/team" class="ib big" title="协作群 · 数字员工" aria-label="协作群 · 数字员工"><FIcon name="users" :size="16" /></RouterLink>
      <button type="button" class="ib big" :title="t('cat.misc.s_11599d066f')" :aria-label="t('cat.misc.s_11599d066f')" data-testid="cat-rail-toolbox" @click="emit('back')"><FIcon name="toolbox" :size="16" /></button>
      <button type="button" class="ib big" :class="{ on: catPanels.view === 'settings' }" :title="t('cat.misc.s_e366ccf155')" :aria-label="t('cat.misc.s_e366ccf155')" data-testid="cat-rail-settings" @click="openView('settings')"><FIcon name="set" :size="16" /></button>
    </div>
    <div v-show="!railCollapsed" class="ct-full">
    <div class="ct-brand">
      <div class="lg"><FIcon name="bot" :size="16" /></div>
      <b>{{ AGENT_NAME }}</b>
    </div>
    <button type="button" class="ct-it new" :class="{ on: catPanels.view === 'chat' && catState.sel === NEW_CONV && !catState.newProjectId }" @click="startNewConv()">
      <FIcon name="plus" :size="16" /><span class="t">新对话</span><span class="r"><FIcon name="list" :size="16" /></span>
    </button>
    <!-- C1：「助手」一期不做。v0.35：定时任务正式入口（带启用数），替代 SHOW_SCHEDULE_ENTRY；CAT_SCHEDULE_BACKEND_READY=false 时打包后的应用里不出现 -->
    <button
      v-if="schedulesOn"
      type="button"
      class="ct-it"
      :class="{ on: catPanels.view === 'schedules' }"
      :aria-current="catPanels.view === 'schedules' ? 'page' : undefined"
      data-testid="cat-nav-schedules"
      @click="openView('schedules')"
    >
      <FIcon name="clock" :size="16" /><span class="t">定时任务</span><span v-if="enabledScheduleCount()" class="r cnt">{{ enabledScheduleCount() }}</span>
    </button>

    <div ref="scrollEl" class="ct-scroll">
      <RouterLink to="/cat/team" class="ct-it" data-testid="cat-nav-team"><FIcon name="users" :size="16" /><span class="t">协作群</span></RouterLink>
      <div class="ct-sec">
        <input v-if="projectSearchOn" ref="projectSearchEl" v-model="projectQuery" class="sec-search" :placeholder="t('cat.workspace.filterProjects')" :aria-label="t('cat.workspace.filterProjects')" data-testid="cat-project-search" @keydown.esc.prevent="closeProjectSearch" />
        <span v-else>项目</span>
        <span class="acts">
          <button type="button" class="ib" :class="{ on: projectSearchOn }" :title="t('cat.workspace.filterProjects')" :aria-label="t('cat.workspace.filterProjects')" :aria-expanded="projectSearchOn" data-testid="cat-project-search-toggle" @click="toggleProjectSearch"><FIcon name="search" :size="14" /></button>
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
      <div v-if="!catState.projects.length" class="pj-empty-box" data-testid="cat-no-projects">
        <FIcon name="folder-plus" :size="22" /><span>{{ PC.empty }}</span>
        <button type="button" class="pj-empty-act" data-testid="cat-no-projects-new" @click="createProject()">{{ PC.newProject }}</button>
      </div>
      <div v-else class="pjs">
        <div
          v-for="p in shownProjects"
          :key="p.id"
          class="pj"
          :class="{ open: p.open || !!projectQuery.trim(), missing: p.missing, hl: catState.highlight === p.id, menu: menuFor === p.id }"
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
                v-for="c in visibleConversations(p.convs, false, projectQuery)"
                :key="c.id"
                class="pc"
                :class="{ on: catPanels.view === 'chat' && c.id === catState.sel, menu: cmenuFor === c.id }"
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
                  <span class="t" :title="c.title || t('cat.misc.s_1ac07a4bb2')"><FIcon v-if="c.pinned" name="pin" :size="11" class="conv-pin" />{{ c.title || t('cat.misc.s_1ac07a4bb2') }}</span>
                  <span v-if="c.main" class="main-tag">主要</span>
                </div>
                <div v-if="c.sub || c.time" class="l2"><span class="t" :title="c.sub">{{ c.sub }}</span><span class="tm">{{ c.time }}</span></div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div class="ct-sec">
        <input v-if="convSearchOn" ref="convSearchEl" v-model="convQuery" class="sec-search" :placeholder="t('cat.workspace.filter')" :aria-label="t('cat.workspace.filter')" data-testid="cat-sidebar-filter" @keydown.esc.prevent="closeConvSearch" />
        <span v-else>对话</span>
        <span class="acts">
          <button type="button" class="ib" :class="{ on: convSearchOn }" :title="t('cat.workspace.filter')" :aria-label="t('cat.workspace.filter')" :aria-expanded="convSearchOn" data-testid="cat-conv-search-toggle" @click="toggleConvSearch"><FIcon name="search" :size="14" /></button>
        </span>
      </div>
      <!-- 普通对话行：里面要放「···」按钮，所以行本身不能是 <button> -->
      <div
        v-for="c in visibleConversations(catState.plain, false, convQuery)"
        :key="c.id"
        class="ct-it dc"
        :class="{ on: catPanels.view === 'chat' && c.id === catState.sel, menu: cmenuFor === c.id }"
        role="button"
        tabindex="0"
        :data-cid="c.id"
        data-testid="cat-conv-row"
        @click="onConvClick(c.id, $event)"
        @keydown.enter.self="selectConv(c.id)"
      >
        <FIcon name="chat" :size="16" /><i v-if="rowDot(c.id, c.st)" class="st" :class="rowDot(c.id, c.st)" role="img" :aria-label="rowDotLabel(c.id, c.st)" /><span class="t" :title="c.title || t('cat.misc.s_1ac07a4bb2')">{{ c.title || t('cat.misc.s_1ac07a4bb2') }}</span>
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
    <div class="ct-footer">
    <button type="button" class="ct-it" :title="t('cat.misc.s_11599d066f')" data-testid="cat-nav-toolbox" @click="emit('back')">
      <FIcon name="toolbox" :size="16" /><span class="t">{{ t('cat.misc.s_11599d066f') }}</span>
    </button>
    <button
      type="button"
      class="ct-it"
      :class="{ on: catPanels.view === 'settings' }"
      :aria-current="catPanels.view === 'settings' ? 'page' : undefined"
      data-testid="cat-nav-settings"
      @click="openView('settings')"
    ><FIcon name="set" :size="16" /><span class="t">设置</span></button>
    </div>
    </div>

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
      <div class="mi" role="menuitem" tabindex="-1" data-testid="cat-conv-menu-rename" @click="renameConv(cmenuFor)"><FIcon name="edit" :size="15" />{{ t('cat.workspace.rename') }}</div>
      <div class="mi" role="menuitem" tabindex="-1" data-testid="cat-conv-menu-pin" @click="convAction('pin')"><FIcon name="pin" :size="15" />{{ t(menuConv?.pinned ? 'cat.workspace.unpin' : 'cat.workspace.pin') }}</div>
      <div class="mi" role="menuitem" tabindex="-1" data-testid="cat-conv-menu-archive" @click="convAction('archive')"><FIcon name="folder" :size="15" />{{ t(menuConv?.archived ? 'cat.workspace.restore' : 'cat.workspace.archive') }}</div>
      <div class="mi" role="menuitem" tabindex="-1" data-testid="cat-conv-menu-export-md" @click="exportConv('md')"><FIcon name="download" :size="15" />{{ t('cat.workspace.exportMarkdown') }}</div>
      <div class="mi" role="menuitem" tabindex="-1" @click="exportConv('json')"><FIcon name="download" :size="15" />{{ t('cat.workspace.exportJson') }}</div>
      <div class="sep" role="separator" />
      <div class="mi danger" role="menuitem" tabindex="-1" data-testid="cat-conv-menu-delete" @click="askDeleteConv(cmenuFor)">
        <FIcon name="trash" :size="15" />{{ CC.del }}
      </div>
    </div>
    <div class="sr-only" role="status" aria-live="polite">{{ catState.announce }}</div>
    <div
      v-show="!railCollapsed"
      class="ct-side-resize"
      role="separator"
      tabindex="0"
      aria-orientation="vertical"
      :aria-label="t('cat.misc.s_e935cf1619')"
      :aria-valuenow="catPane.side"
      :aria-valuemin="160"
      data-testid="cat-side-resize"
      @pointerdown="onSideResizeDown"
      @keydown="onSideResizeKey"
    />
  </section>
</template>

<script setup lang="ts">
import { visibleConversations } from '@/views/cat/catCommands'
import { workspaceUI, changeConversation, exportChat } from '@/views/cat/catWorkspace'
import { AGENT_NAME } from '@/config/brand'
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { railCollapsed } from '@/views/cat/catLayout'
/** 定时任务入口开关：真入口落地前隐藏 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { catPanels, catSchedulesVisible, enabledScheduleCount, openView } from '@/views/cat/catPanels'
import { CAT_CONV_COPY as CC, CAT_COPY } from '@/api/cat'
import { CAT_PROJECT_COPY as PC, CAT_PROJECT_NAME_MAX } from '@/api/catProjects'
import type { CatProject } from '@/api/catMock'
import { convDot, convDotLabel } from '@/views/cat/catConvDot'
import { CAT_CHAT_MIN, CAT_SIDE_DEFAULT, CAT_SIDE_MIN, CAT_SIDE_WIDTH_KEY, catPane, clampToLimits, dragPaneWidth, paneLimits, rememberPaneWidth } from '@/components/cat/catPreviewWidth'
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
  startNewConv,
} from '@/views/cat/catState'

/** 标题左边的点：进行中读 turns，上次结果读 outcome，走查数据才用 c.st */
function rowDot(id: string, stored?: string) {
  return convDot(!!catState.turns[id], catState.outcome[id] ?? '', stored, !!catState.unseen[id])
}
function rowDotLabel(id: string, stored?: string) {
  return convDotLabel(rowDot(id, stored))
}

const convQuery = ref('')
const projectQuery = ref('')
const projectSearchOn = ref(false), convSearchOn = ref(false)
const projectSearchEl = ref<HTMLInputElement>()
const convSearchEl = ref<HTMLInputElement>()
const shownProjects = computed(() => {
  const q = projectQuery.value.trim().toLocaleLowerCase()
  if (!q) return catState.projects
  return catState.projects.filter((p) => p.convs.some((c) => !c.archived && c.title.toLocaleLowerCase().includes(q)))
})
function toggleProjectSearch() {
  projectSearchOn.value = !projectSearchOn.value
  if (!projectSearchOn.value) projectQuery.value = ''
  else nextTick(() => projectSearchEl.value?.focus())
}
function closeProjectSearch() {
  projectQuery.value = ''
  projectSearchOn.value = false
}
function toggleConvSearch() {
  convSearchOn.value = !convSearchOn.value
  if (!convSearchOn.value) convQuery.value = ''
  else nextTick(() => convSearchEl.value?.focus())
}
function closeConvSearch() {
  convQuery.value = ''
  convSearchOn.value = false
}
const menuConv = computed(() => findConv(cmenuFor.value)?.conv)
function renameConv(id: string) { closeMenu(); workspaceUI.renameId = id }
function convAction(action: 'pin' | 'archive') { const id = cmenuFor.value; const conv = menuConv.value; closeMenu(); if (conv) void changeConversation(id, action, '', !(action === 'pin' ? conv.pinned : conv.archived)) }
function exportConv(format: 'md' | 'json') { const id = cmenuFor.value; closeMenu(); void exportChat(id, format) }
const emit = defineEmits<{ back: [] }>()
const laterTip = CAT_COPY.later
const schedulesOn = catSchedulesVisible()

const rootEl = ref<HTMLElement>()
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
  const menu = (cmenuFor.value ? cmenuEl : menuEl).value
  if (menu) { const rect = menu.getBoundingClientRect(); menuStyle.value = { left: `${Math.max(8, Math.min(rect.left, window.innerWidth - rect.width - 8))}px`, top: `${Math.max(8, Math.min(rect.top, window.innerHeight - rect.height - 8))}px` } }
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

const sideDragging = ref(false)

function paintSide() {
  const el = rootEl.value
  if (!el) return
  el.style.width = (railCollapsed.value ? 56 : catPane.side) + 'px'
}

watch(() => catPane.side, paintSide)
watch(railCollapsed, paintSide)

function sideLimits(): { lo: number; hi: number } {
  const page = rootEl.value?.closest('.ct') as HTMLElement | null
  const total = page?.clientWidth ?? window.innerWidth
  const right = (page?.querySelector('.ct-r') as HTMLElement | null)?.offsetWidth ?? 0
  return paneLimits(total, right + CAT_CHAT_MIN, CAT_SIDE_MIN)
}

function fitSide(want: number): number {
  return clampToLimits(want, sideLimits(), CAT_SIDE_DEFAULT)
}

function onSideResizeDown(e: PointerEvent) {
  const el = rootEl.value
  if (!el || railCollapsed.value) return
  const startW = el.offsetWidth || catPane.side
  const limits = sideLimits()
  let next = startW
  sideDragging.value = true
  dragPaneWidth(e.currentTarget as HTMLElement, e, 1, (dx) => {
    next = clampToLimits(startW + dx, limits, startW)
    el.style.width = next + 'px'
  }, () => {
    sideDragging.value = false
    catPane.side = next
    rememberPaneWidth(CAT_SIDE_WIDTH_KEY, next)
  })
}

function onSideResizeKey(e: KeyboardEvent) {
  const step = e.shiftKey ? 80 : 24
  if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
  e.preventDefault()
  catPane.side = fitSide(catPane.side + (e.key === 'ArrowRight' ? step : -step))
  rememberPaneWidth(CAT_SIDE_WIDTH_KEY, catPane.side)
}

function onSideWindow() {
  const page = rootEl.value?.closest('.ct')
  if (page?.querySelector('.ct-r')) return
  catPane.side = fitSide(catPane.side)
}

onMounted(() => {
  paintSide()
  document.addEventListener('mousedown', onDocDown, true)
  window.addEventListener('resize', onSideWindow)
  nextTick(onSideWindow)
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocDown, true)
  window.removeEventListener('resize', onSideWindow)
})
</script>

<style scoped>
.conv-pin { margin-right:4px; vertical-align:middle }
.sec-search { flex:1; min-width:0; height:22px; border:0; outline:none; background:transparent; color:var(--cat-text); font:inherit; font-size:12px }
.ct-sec .acts .ib.on { background:var(--cat-hover); color:var(--cat-text) }
.ct-l {
  position: relative;
  flex: none;
  box-sizing: border-box;
  overflow: visible;
  transition: width var(--ff-dur-slow) var(--ff-ease-out), padding var(--ff-dur-slow) var(--ff-ease-out);
}
.ct-l.mini {
  width: 56px;
  padding-left: 0;
  padding-right: 0;
}
.ct-l.dragging {
  transition: none;
}
.ct-full {
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.ct-mini {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  animation: cat-fade-in var(--ff-dur-base) var(--ff-ease-out) both;
}
.ct-mini .sp {
  flex: 1;
}
.ct-mini-brand {
  margin: 2px 0 8px;
}
.ct-footer {
  flex: none;
  border-top: 1px solid var(--cat-border);
  margin-top: 8px;
  padding-top: 6px;
}
.ib.big {
  width: 32px;
  height: 32px;
  border-radius: var(--cat-radius-md);
}
.ib.big.on {
  background: var(--cat-surface);
  box-shadow: var(--cat-shadow-sm);
  color: var(--cat-text);
}
.ct-l {
  display: flex;
  flex-direction: column;
  background: var(--cat-bg);
  border-right: 1px solid var(--cat-border);
  padding: 10px 8px 12px;
  min-width: 0;
  min-height: 0;
  overflow: visible;
}
.ct-rail-toggle {
  position: absolute;
  top: 50%;
  right: -9px;
  z-index: 7;
  width: 18px;
  height: 44px;
  display: grid;
  place-items: center;
  padding: 0;
  border: 1px solid var(--cat-border);
  border-radius: 9px;
  background: var(--cat-bg);
  color: var(--cat-muted);
  cursor: pointer;
  transform: translateY(-50%);
  --wails-draggable: no-drag;
  transition: background var(--ff-dur-quick), border-color var(--ff-dur-quick), color var(--ff-dur-quick);
}
.ct-rail-toggle::before {
  content: '';
  position: absolute;
  inset: -5px -6px;
}
.ct-rail-toggle:hover {
  background: var(--cat-surface);
  border-color: var(--cat-muted);
  color: var(--cat-text);
}
.ct-rail-toggle:active {
  background: var(--cat-active);
  color: var(--cat-active-text);
}
.ct-rail-toggle:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.ct-side-resize {
  position: absolute;
  top: 0;
  bottom: 0;
  right: 0;
  width: 6px;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: col-resize;
  z-index: 6;
  touch-action: none;
  --wails-draggable: no-drag;
}
.ct-side-resize::after {
  content: '';
  position: absolute;
  right: 0;
  top: 0;
  bottom: 0;
  width: 2px;
  background: transparent;
}
.ct-side-resize:hover::after,
.ct-side-resize:focus-visible::after {
  background: var(--ff-primary);
}
.ct-side-resize:focus-visible {
  outline: none;
}
.ct-brand {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 2px 4px 12px;
  --wails-draggable: drag;
}
.ct-brand .lg,
.ct-mini-brand {
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
/* 选中和悬浮用同一块灰色，不再用主色底和左侧蓝条 */
.ct-it.on,
.ct-it.on:hover {
  background: var(--cat-hover);
  color: var(--cat-text);
  font-weight: 500;
}
.ct-it.on > :deep(svg) {
  color: var(--cat-text-2);
}
.ct-it .t {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.ct-it.new {
  margin-bottom: 4px;
  font-weight: 500;
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
  opacity: 0;
  transition: opacity var(--ff-dur-quick);
}
.ct-sec:hover .acts,
.ct-sec:focus-within .acts,
.ct-sec:has(.sec-search) .acts {
  opacity: 1;
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
  background: var(--cat-hover);
}
.pc.on .l1 {
  color: var(--cat-text);
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
.st.ok,
.st.new {
  background: var(--cat-success); /* C5 绿点 = 已完成 / 有新回复，同一行只出一个 */
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
.pc:focus-visible {
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
.pj-empty-box {
  margin: 4px 0 8px;
  padding: 14px 10px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  border: 1px dashed var(--cat-border-strong);
  border-radius: var(--cat-radius-md);
  color: var(--cat-muted);
  font-size: 12px;
  text-align: center;
  animation: cat-fade-in var(--ff-dur-base) var(--ff-ease-out) both;
}
.pj-empty-act {
  height: 26px;
  padding: 0 12px;
  border: none;
  border-radius: var(--cat-radius-sm);
  background: var(--cat-accent);
  color: var(--cat-on-accent);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  transition: background var(--ff-dur-quick), transform var(--ff-dur-quick);
}
.pj-empty-act:hover {
  background: var(--cat-accent-hover);
}
.pj-empty-act:active {
  transform: scale(0.94);
}
.pj-empty-act:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
</style>
