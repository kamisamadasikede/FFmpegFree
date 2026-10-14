<template>
  <section class="cp" aria-labelledby="sc-h" data-testid="cat-schedules">
    <CatPanelNotice />
    <div class="cp-in wide">
      <header class="cp-head row0">
        <div>
          <h2 id="sc-h">{{ C.title }}</h2>
          <p>{{ C.subtitle }}</p>
        </div>
        <button type="button" class="btn pri" data-testid="cat-sched-new" @click="openNew()"><FIcon name="plus" :size="14" />{{ C.create }}</button>
      </header>

      <div v-if="catNotReady" class="banner warn" role="status">
        <FIcon name="warn" :size="14" /><span>{{ C.notReady }}</span>
        <button type="button" class="btn ghost sm" :disabled="rechecking" @click="recheck"><i v-if="rechecking" class="spin" />{{ C.recheck }}</button>
      </div>
      <p class="note"><FIcon name="info" :size="14" />{{ C.hint }}</p>

      <div v-if="!catPanels.schedulesLoaded" class="card" aria-busy="true"><div v-for="i in 3" :key="i" class="skr" /></div>

      <div v-else-if="!catPanels.schedules.length" class="emp" data-testid="cat-sched-empty">
        <div class="emp-ic"><FIcon name="clock" :size="22" /></div>
        <b>{{ C.emptyTitle }}</b>
        <span>{{ C.emptyBody }}</span>
        <button type="button" class="btn pri" @click="openNew()"><FIcon name="plus" :size="14" />{{ C.create }}</button>
        <div class="tpls">
          <button v-for="t in C.templates" :key="t.name" type="button" class="tpl" @click="openNew(t)">
            <b>{{ t.name }}</b><span>{{ t.prompt }}</span>
          </button>
        </div>
      </div>

      <div v-else class="card tbl" role="table" :aria-label="C.title">
        <div class="tr th" role="row">
          <span role="columnheader">{{ C.colTask }}</span><span role="columnheader">{{ C.colFreq }}</span>
          <span role="columnheader">{{ C.colLast }}</span><span role="columnheader">{{ C.colOn }}</span><span />
        </div>
        <TransitionGroup name="tr">
          <div
            v-for="s in catPanels.schedules"
            :key="s.id"
            class="tr"
            :class="{ off: !s.enabled || s.projectDeleted }"
            role="row"
            tabindex="0"
            :data-testid="'cat-sched-row-' + s.id"
            @click="openRuns(s)"
            @keydown.enter="openRuns(s)"
          >
            <div class="c1" role="cell">
              <b>{{ s.name }}</b>
              <span v-if="s.projectDeleted" class="pd"><FIcon name="warn" :size="12" />{{ C.projectDeleted }}</span>
              <span v-else><FIcon name="folder" :size="12" />{{ projectName(s.projectId) }}</span>
            </div>
            <div class="c2" role="cell">
              <span>{{ fmtFreq(s) }}</span>
              <small>下次 {{ nextText(s) }}</small>
            </div>
            <div class="c3" role="cell">
              <span class="rs" :class="!s.enabled && !isActive(s) ? 'grey' : rv(s).tone"><i />{{ !s.enabled && !isActive(s) ? C.st.paused : rv(s).label }}</span>
              <small v-if="s.enabled || isActive(s)">{{ lastSub(s) }}</small>
            </div>
            <div class="c4" role="cell" @click.stop>
              <button
                type="button"
                role="switch"
                class="sw"
                :class="{ on: s.enabled }"
                :aria-checked="s.enabled"
                :aria-label="C.enabled + ' ' + s.name"
                :disabled="busy[s.id] || (s.projectDeleted && !s.enabled)"
                :data-testid="'cat-sched-toggle-' + s.id"
                @click="toggle(s)"
              ><i /></button>
            </div>
            <div class="c5" role="cell" @click.stop>
              <!-- 自绘提示（不用原生 title）：悬停 300ms 后 / 键盘聚焦立即出现，离开立即消失 -->
              <span v-if="s.lastRun?.status === 'running' && s.lastRun.conversationId" class="tipw">
                <button
                  type="button"
                  class="ib"
                  :aria-label="C.stop"
                  data-testid="cat-sched-stop"
                  @click="stopRun(s)"
                ><FIcon name="stop" :size="14" /></button>
                <span class="tp" role="tooltip" aria-hidden="true">{{ C.stop }}</span>
              </span>
              <button type="button" class="ib" :aria-label="C.more" aria-haspopup="menu" :data-testid="'cat-sched-more-' + s.id" @click="openMenu(s, $event)"><FIcon name="more" :size="14" /></button>
            </div>
          </div>
        </TransitionGroup>
      </div>
    </div>

    <!-- 行菜单 -->
    <Transition name="mn">
      <div v-if="menuFor" class="menu" role="menu" :style="menuStyle" @keydown.esc="menuFor = null">
        <button type="button" role="menuitem" class="mi" @click="openEdit(menuFor)"><FIcon name="edit" :size="14" />{{ C.menuEdit }}</button>
        <button type="button" role="menuitem" class="mi" @click="runNow(menuFor)"><FIcon name="play" :size="14" />{{ C.menuRun }}</button>
        <button type="button" role="menuitem" class="mi" @click="duplicate(menuFor)"><FIcon name="copy" :size="14" />{{ C.menuCopy }}</button>
        <button type="button" role="menuitem" class="mi danger" @click="askDelete(menuFor)"><FIcon name="trash" :size="14" />{{ C.menuDelete }}</button>
      </div>
    </Transition>

    <!-- 运行记录抽屉 -->
    <Transition name="dr">
      <div v-if="runsFor" class="dr-mask" @mousedown.self="runsFor = null">
        <aside class="dr" role="dialog" aria-modal="true" aria-labelledby="dr-h" data-testid="cat-sched-runs" @keydown.esc="runsFor = null">
          <header class="dr-head">
            <div>
              <h3 id="dr-h">{{ runsFor.name }}</h3>
              <p>{{ projectName(runsFor.projectId) }} · {{ fmtFreq(runsFor) }} · 下次 {{ nextText(runsFor) }}</p>
            </div>
            <button type="button" class="ib" :aria-label="C.close" @click="runsFor = null"><FIcon name="x" :size="16" /></button>
          </header>
          <div class="dr-acts">
            <button type="button" class="btn pri" @click="runNow(runsFor)"><FIcon name="play" :size="14" />{{ C.menuRun }}</button>
            <button type="button" class="btn" @click="openEdit(runsFor)">{{ C.menuEdit }}</button>
            <button type="button" class="btn danger" @click="askDelete(runsFor)">{{ C.menuDelete }}</button>
          </div>
          <div class="dr-h2">{{ C.runs }}</div>
          <div class="dr-list">
            <div v-if="runsLoading" class="skr" />
            <div v-else-if="!runs.length" class="empty">{{ C.runsEmpty }}</div>
            <TransitionGroup v-else name="tr">
              <div v-for="r in runs" :key="r.id" class="run" :data-testid="'cat-run-' + r.status">
                <span class="rs" :class="runView(r, now).tone"><i />{{ runView(r, now).label }}</span>
                <span class="rt">{{ fmtWhen(r.scheduledFor, now) }}<template v-if="r.manual"> · {{ C.manualRun }}</template><template v-if="r.finishedAt && r.startedAt"> · 用时 {{ formatDuration(r.finishedAt - r.startedAt) }}</template></span>
                <span v-if="runView(r, now).sub" class="rsub" :class="{ bad: r.status === 'failed' }">{{ runView(r, now).sub }}</span>
                <button v-if="r.conversationId" type="button" class="lnk" @click="openConv(r.conversationId)">{{ C.openConv }}</button>
              </div>
            </TransitionGroup>
          </div>
        </aside>
      </div>
    </Transition>

    <CatScheduleDialog :open="dlgOpen" :schedule="editing" :preset="preset" @close="dlgOpen = false" @saved="onSaved" />
    <CatConfirmDialog
      :open="!!delFor"
      :title="C.delTitle"
      :body="C.delBody"
      :ok-text="C.delOk"
      :cancel-text="C.cancel"
      tone="danger"
      :busy="deleting"
      @cancel="delFor = null"
      @ok="doDelete"
    />
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import CatPanelNotice from './CatPanelNotice.vue'
import CatScheduleDialog from './CatScheduleDialog.vue'
import CatConfirmDialog from './CatConfirmDialog.vue'
import { recheckCat } from '@/api/cat'
import {
  type CatSchedule,
  type CatScheduleInput,
  type CatScheduleRun,
  CAT_SCHED_COPY as C,
  deleteCatSchedule,
  fmtFreq,
  fmtWhen,
  formatDuration,
  listCatScheduleRuns,
  onCatSchedule,
  pauseCatSchedule,
  resumeCatSchedule,
  runNowCatSchedule,
  runView,
} from '@/api/catSchedules'
import { catPanels, refreshSchedules } from '@/views/cat/catPanels'
import { catNotReady, catState, findConv, notify, selectConv, stopTurn } from '@/views/cat/catState'

defineOptions({ name: 'CatSchedulesView' })

const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | undefined
const busy = reactive<Record<string, boolean>>({})

const projectName = (id: string) => catState.projects.find((p) => p.id === id)?.name ?? ''
const rv = (s: CatSchedule) => runView(s.lastRun, now.value)
const isActive = (s: CatSchedule) => s.lastRun?.status === 'running' || s.lastRun?.status === 'queued'
function nextText(s: CatSchedule) {
  if (s.frequency === 'once' && !s.nextRunAt && s.lastRun) return C.ended
  if (!s.enabled || !s.nextRunAt) return C.dash
  return fmtWhen(s.nextRunAt, now.value)
}
function lastSub(s: CatSchedule) {
  const r = s.lastRun
  if (!r) return ''
  if (r.status === 'running') return ''
  const v = runView(r, now.value)
  if (r.status === 'queued') return v.sub
  if (r.status === 'skipped') return `${fmtWhen(r.scheduledFor, now.value)}`
  return fmtWhen(r.finishedAt || r.scheduledFor, now.value)
}

// ---- 启停 / 立即运行 / 停止 ----
async function toggle(s: CatSchedule) {
  busy[s.id] = true
  try {
    const next = s.enabled ? await pauseCatSchedule(s.id) : await resumeCatSchedule(s.id)
    replace(next)
  } catch {
    notify(C.opFailed, 'warn')
  } finally {
    busy[s.id] = false
  }
}
async function runNow(s: CatSchedule) {
  menuFor.value = null
  try {
    const run = await runNowCatSchedule(s.id)
    upsertRun(run)
  } catch {
    notify(C.opFailed, 'warn')
  }
}
function stopRun(s: CatSchedule) {
  const id = s.lastRun?.conversationId
  if (id) void stopTurn(id)
}
function replace(s: CatSchedule) {
  const i = catPanels.schedules.findIndex((x) => x.id === s.id)
  if (i >= 0) catPanels.schedules[i] = { ...s, lastRun: s.lastRun ?? catPanels.schedules[i].lastRun }
}

// ---- 菜单 ----
const menuFor = ref<CatSchedule | null>(null)
const menuStyle = ref<Record<string, string>>({})
async function openMenu(s: CatSchedule, e: MouseEvent) {
  if (menuFor.value?.id === s.id) return (menuFor.value = null)
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  menuStyle.value = { top: `${r.bottom + 4}px`, left: `${Math.max(8, r.right - 160)}px` }
  menuFor.value = s
  await nextTick()
  document.querySelector<HTMLElement>('.menu .mi')?.focus()
}
function onDoc(e: MouseEvent) {
  if (menuFor.value && !(e.target as HTMLElement).closest('.menu, .c5')) menuFor.value = null
}

// ---- 新建 / 编辑 / 复制 ----
const dlgOpen = ref(false)
const editing = ref<CatSchedule | null>(null)
const preset = ref<Partial<CatScheduleInput> | null>(null)
function openNew(t?: Partial<CatScheduleInput>) {
  editing.value = null
  preset.value = t ? { ...t } : null
  dlgOpen.value = true
}
function openEdit(s: CatSchedule) {
  menuFor.value = null
  editing.value = s
  preset.value = null
  dlgOpen.value = true
}
function duplicate(s: CatSchedule) {
  menuFor.value = null
  editing.value = null
  preset.value = {
    name: [...(s.name + C.copySuffix)].slice(0, 40).join(''), prompt: s.prompt, projectId: s.projectDeleted ? '' : s.projectId,
    frequency: s.frequency, time: s.time, weekdays: [...s.weekdays], runAt: s.runAt, model: s.model, effort: s.effort, approvalMode: s.approvalMode,
  }
  dlgOpen.value = true
}
function onSaved(s: CatSchedule) {
  dlgOpen.value = false
  const i = catPanels.schedules.findIndex((x) => x.id === s.id)
  if (i >= 0) catPanels.schedules[i] = s
  else catPanels.schedules.unshift(s)
  if (runsFor.value?.id === s.id) runsFor.value = s
}

// ---- 删除 ----
const delFor = ref<CatSchedule | null>(null)
const deleting = ref(false)
function askDelete(s: CatSchedule) {
  menuFor.value = null
  delFor.value = s
}
async function doDelete() {
  const s = delFor.value
  if (!s) return
  deleting.value = true
  try {
    await deleteCatSchedule(s.id)
    catPanels.schedules = catPanels.schedules.filter((x) => x.id !== s.id)
    if (runsFor.value?.id === s.id) runsFor.value = null
    delFor.value = null
  } catch {
    delFor.value = null
    notify(C.delFailed, 'warn')
  } finally {
    deleting.value = false
  }
}

// ---- 运行记录 ----
const runsFor = ref<CatSchedule | null>(null)
const runs = ref<CatScheduleRun[]>([])
const runsLoading = ref(false)
async function openRuns(s: CatSchedule) {
  runsFor.value = s
  runsLoading.value = true
  runs.value = []
  try {
    runs.value = await listCatScheduleRuns(s.id, 100)
  } catch {
    runs.value = []
  }
  runsLoading.value = false
}
/** cat:schedule 推来的运行（含队列变化时重推的排队位置）：按 id 原地更新 */
function upsertRun(run: CatScheduleRun) {
  if (runsFor.value?.id !== run.scheduleId) return
  const i = runs.value.findIndex((r) => r.id === run.id)
  if (i >= 0) runs.value[i] = run
  else runs.value.unshift(run)
  if (runs.value.length > 100) runs.value.length = 100
}
function openConv(id: string) {
  if (!findConv(id)) {
    notify(C.convGone, 'warn')
    return
  }
  runsFor.value = null
  selectConv(id)
}

const rechecking = ref(false)
async function recheck() {
  rechecking.value = true
  try {
    catState.status = await recheckCat()
  } catch {
    /* 保持 */
  }
  rechecking.value = false
}

let off: (() => void) | undefined
watch(
  () => catPanels.schedules,
  () => {
    if (runsFor.value) runsFor.value = catPanels.schedules.find((s) => s.id === runsFor.value!.id) ?? runsFor.value
  },
  { deep: true },
)
onMounted(() => {
  void refreshSchedules()
  clock = setInterval(() => (now.value = Date.now()), 1000)
  off = onCatSchedule((e) => {
    if (e.run) upsertRun(e.run)
    if (e.deleted && runsFor.value?.id === e.scheduleId) runsFor.value = null
  })
  document.addEventListener('mousedown', onDoc)
  if (typeof location !== 'undefined' && !(window as unknown as { go?: unknown }).go) {
    const q = new URLSearchParams(location.search)
    const open = q.get('cat_sched')
    if (open) {
      const stop = watch(
        () => catPanels.schedulesLoaded,
        (v) => {
          if (!v) return
          const s = catPanels.schedules.find((x) => x.id === q.get('cat_sched_id')) ?? catPanels.schedules[0]
          if (open === 'new') openNew()
          else if (open === 'newbad') {
            openNew({ approvalMode: 'approve', name: '', prompt: '' })
            setTimeout(() => document.querySelector<HTMLButtonElement>('[data-testid="sd-save"]')?.click(), 300)
          } else if (open === 'confirm') {
            openNew({ ...C.templates[0], approvalMode: 'approve' })
            setTimeout(() => document.querySelector<HTMLButtonElement>('[data-testid="sd-save"]')?.click(), 300)
          } else if (open === 'normal') openNew({ ...C.templates[1], approvalMode: 'normal' })
          else if (open === 'edit' && s) openEdit(s)
          else if (open === 'runs' && s) void openRuns(s)
          queueMicrotask(() => stop())
        },
        { immediate: true },
      )
    }
  }
})
onBeforeUnmount(() => {
  clearInterval(clock)
  off?.()
  document.removeEventListener('mousedown', onDoc)
})
</script>

<style scoped src="./catPanel.css"></style>
<style scoped>
.cp-in.wide {
  max-width: 980px;
}
.row0 {
  flex-direction: row;
  align-items: flex-end;
  justify-content: space-between;
}
.row0 > div {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.note {
  margin-bottom: 12px;
}
.banner {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
  padding: 8px 10px 8px 12px;
  border-radius: var(--cat-radius-md);
  font-size: 13px;
}
.banner.warn {
  background: var(--cat-warn-soft);
  color: var(--cat-warn-text);
  border: 1px solid color-mix(in srgb, var(--cat-warn) 35%, transparent);
}
.banner span {
  flex: 1;
}
.btn.sm {
  height: 26px;
  color: inherit;
}
.skr {
  height: 52px;
  margin: 1px 0;
  background: var(--cat-sunken);
  animation: cat-breathe 1.2s ease-in-out infinite;
}
.tbl .tr {
  display: grid;
  grid-template-columns: minmax(0, 2.2fr) minmax(0, 1.4fr) minmax(0, 1.4fr) 56px 64px;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border-top: 1px solid var(--cat-divider);
  cursor: pointer;
  transition: background-color 120ms var(--ff-ease);
}
.tbl .tr:hover:not(.th) {
  background: var(--cat-hover);
}
.tbl .tr:focus-visible {
  outline: none;
  box-shadow: inset var(--cat-focus);
}
.tbl .th {
  border-top: 0;
  cursor: default;
  padding: 8px 14px;
  font-size: 12px;
  color: var(--cat-muted);
  background: var(--cat-sunken);
}
.tr > div {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.c1 b {
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.c1 span {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--cat-muted);
}
.c1 .pd {
  color: var(--cat-warn-text);
}
.tr.off .c1 b,
.tr.off .c2 {
  color: var(--cat-muted);
}
.c3 {
  align-items: flex-start;
}
.c2 span {
  font-size: 13px;
}
small {
  font-size: 12px;
  color: var(--cat-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.c5 {
  flex-direction: row !important;
  justify-content: flex-end;
}
.ib {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: var(--cat-radius-sm);
  background: none;
  color: var(--cat-muted);
  cursor: pointer;
  transition: background-color 120ms var(--ff-ease), color 120ms var(--ff-ease);
}
.ib:hover {
  background: var(--cat-pressed);
  color: var(--cat-text);
}
.ib:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
/* 停止键提示 */
.tipw {
  position: relative;
  display: inline-flex;
}
.tp {
  position: absolute;
  left: 50%;
  bottom: calc(100% + 6px);
  right: auto;
  top: auto;
  transform: translateX(-50%);
  z-index: 5;
  padding: 4px 8px;
  border-radius: var(--cat-radius-sm);
  background: var(--cat-text);
  color: var(--cat-bg);
  box-shadow: var(--cat-shadow-md);
  font-size: 12px;
  line-height: 1.4;
  white-space: nowrap;
  pointer-events: none;
  opacity: 0;
  visibility: hidden;
  transition: none;
}
.tipw:hover .tp {
  opacity: 1;
  visibility: visible;
  transition: opacity 120ms var(--ff-ease) 300ms, visibility 0ms linear 300ms;
}
.tipw:has(:focus-visible) .tp {
  opacity: 1;
  visibility: visible;
  transition: opacity 120ms var(--ff-ease);
}
/* 运行状态 */
.rs {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
}
.rs i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex: none;
  background: var(--cat-muted);
}
.rs.ok {
  color: var(--cat-success-text);
}
.rs.ok i {
  background: var(--cat-success);
}
.rs.bad {
  color: var(--cat-danger-text);
}
.rs.bad i {
  background: var(--cat-danger);
}
.rs.warn {
  color: var(--cat-warn-text);
}
.rs.warn i {
  background: var(--cat-warn);
}
.rs.grey {
  color: var(--cat-text-2);
  font-weight: 500;
}
.rs.skip {
  color: var(--cat-text-2);
  font-weight: 500;
}
.rs.skip i {
  background: none;
  border: 1.5px dashed var(--cat-muted);
  width: 6px;
  height: 6px;
}
.rs.queue {
  color: var(--cat-accent);
}
.rs.queue i {
  background: none;
  border: 1.5px solid var(--cat-accent);
  width: 6px;
  height: 6px;
}
.rs.live {
  color: var(--cat-accent);
}
.rs.live i {
  background: var(--cat-accent);
  animation: cat-breathe 1.6s ease-in-out infinite;
}
/* 空状态 */
.emp {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 48px 16px 24px;
  text-align: center;
  animation: cat-enter 320ms var(--ff-ease) both;
}
.emp-ic {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  background: var(--cat-accent-soft);
  color: var(--cat-accent);
}
.emp b {
  font-size: 15px;
}
.emp > span {
  font-size: 13px;
  color: var(--cat-text-2);
  margin-bottom: 8px;
}
.tpls {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-top: 20px;
  width: 100%;
}
.tpl {
  display: flex;
  flex-direction: column;
  gap: 6px;
  text-align: left;
  padding: 14px;
  border-radius: var(--cat-radius-lg);
  border: 1px solid var(--cat-border);
  background: var(--cat-surface);
  color: var(--cat-text);
  cursor: pointer;
  transition: border-color 120ms var(--ff-ease), box-shadow 200ms var(--ff-ease), transform 200ms var(--ff-ease);
}
.tpl:hover {
  border-color: var(--cat-border-strong);
  box-shadow: var(--cat-shadow-md);
  transform: translateY(-1px);
}
.tpl b {
  font-size: 13px;
}
.tpl span {
  font-size: 12px;
  color: var(--cat-muted);
  line-height: 1.5;
}
/* 菜单 */
.menu {
  position: fixed;
  z-index: 3000;
  width: 160px;
  padding: 4px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-elevated);
  box-shadow: var(--cat-shadow-md);
}
.mi {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 10px;
  border: 0;
  border-radius: var(--cat-radius-sm);
  background: none;
  color: var(--cat-text);
  font-size: 13px;
  cursor: pointer;
  transition: background-color 120ms var(--ff-ease);
}
.mi:hover,
.mi:focus-visible {
  outline: none;
  background: var(--cat-hover);
}
.mi.danger {
  color: var(--cat-danger-text);
}
.mn-enter-active,
.mn-leave-active {
  transition: opacity 120ms var(--ff-ease), transform 120ms var(--ff-ease);
}
.mn-enter-from,
.mn-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
/* 抽屉 */
.dr-mask {
  position: fixed;
  inset: 0;
  z-index: 3000;
  background: var(--cat-scrim);
}
.dr {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: 420px;
  max-width: 100vw;
  display: flex;
  flex-direction: column;
  background: var(--cat-elevated);
  border-left: 1px solid var(--cat-border);
  box-shadow: var(--cat-shadow-lg);
  color: var(--cat-text);
}
.dr-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 18px 18px 8px;
}
.dr-head h3 {
  margin: 0 0 4px;
  font-size: 16px;
}
.dr-head p {
  margin: 0;
  font-size: 12px;
  color: var(--cat-muted);
}
.dr-acts {
  display: flex;
  gap: 8px;
  padding: 8px 18px 14px;
  border-bottom: 1px solid var(--cat-divider);
}
.dr-h2 {
  padding: 12px 18px 4px;
  font-size: 12px;
  color: var(--cat-muted);
}
.dr-list {
  flex: 1;
  overflow-y: auto;
  padding: 0 10px 16px;
}
.run {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 2px 8px;
  padding: 10px 8px;
  border-radius: var(--cat-radius-md);
  transition: background-color 120ms var(--ff-ease);
}
.run:hover {
  background: var(--cat-hover);
}
.run + .run {
  border-top: 1px solid var(--cat-divider);
}
.rt {
  grid-column: 1;
  font-size: 12px;
  color: var(--cat-muted);
}
.rsub {
  grid-column: 1;
  font-size: 12px;
  color: var(--cat-text-2);
}
.rsub.bad {
  color: var(--cat-danger-text);
}
.lnk {
  grid-column: 2;
  grid-row: 1 / span 2;
  align-self: center;
  border: 0;
  background: none;
  color: var(--cat-accent);
  font-size: 12px;
  cursor: pointer;
  padding: 4px 6px;
  border-radius: var(--cat-radius-sm);
}
.lnk:hover {
  background: var(--cat-accent-soft);
}
.dr-enter-active,
.dr-leave-active {
  transition: opacity 320ms var(--ff-ease);
}
.dr-enter-active .dr,
.dr-leave-active .dr {
  transition: transform 320ms var(--ff-ease);
}
.dr-enter-from,
.dr-leave-to {
  opacity: 0;
}
.dr-enter-from .dr,
.dr-leave-to .dr {
  transform: translateX(100%);
}
.tr-enter-active,
.tr-leave-active {
  transition: opacity 200ms var(--ff-ease), transform 200ms var(--ff-ease);
}
.tr-enter-from,
.tr-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
@media (prefers-reduced-motion: reduce) {
  .skr,
  .rs.live i,
  .emp {
    animation: none !important;
  }
  .dr-enter-active,
  .dr-leave-active,
  .dr-enter-active .dr,
  .dr-leave-active .dr,
  .tr-enter-active,
  .tr-leave-active,
  .mn-enter-active,
  .mn-leave-active,
  .tpl {
    transition: none !important;
  }
  .tipw:hover .tp {
    transition: visibility 0ms linear 300ms, opacity 0ms linear 300ms !important;
  }
  .tipw:has(:focus-visible) .tp {
    transition: none !important;
  }
}
</style>
