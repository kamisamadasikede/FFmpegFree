<template>
  <Teleport to="body">
    <Transition name="sd">
      <div v-if="open" class="sd-mask" data-testid="cat-schedule-dialog" @mousedown.self="!saving && emit('close')">
        <form class="sd" role="dialog" aria-modal="true" aria-labelledby="sd-h" novalidate @submit.prevent="submit" @keydown.esc.stop="!saving && emit('close')">
          <header class="sd-head">
            <h3 id="sd-h">{{ editing ? C.editTitle : C.newTitle }}</h3>
            <button type="button" class="ib" :aria-label="C.close" @click="emit('close')"><FIcon name="x" :size="16" /></button>
          </header>
          <div class="sd-body">
            <label class="f">
              <span class="fl">{{ C.name }}</span>
              <input ref="nameEl" v-model="form.name" class="in" :class="{ bad: err.name }" :placeholder="C.namePh" :aria-invalid="!!err.name" data-testid="sd-name" />
              <span v-if="err.name" class="fe">{{ err.name }}</span>
            </label>
            <label class="f">
              <span class="fl">{{ C.prompt }}<em>{{ C.promptTip }}</em></span>
              <textarea v-model="form.prompt" class="in ta" rows="4" :class="{ bad: err.prompt }" :aria-invalid="!!err.prompt" data-testid="sd-prompt" />
              <span v-if="err.prompt" class="fe">{{ err.prompt }}</span>
              <span v-else class="fh">{{ C.promptHelp }}</span>
            </label>
            <label class="f">
              <span class="fl">{{ C.project }}</span>
              <select v-model="form.projectId" class="in" :class="{ bad: err.projectId || projMissing }" data-testid="sd-project">
                <option value="" disabled>{{ C.projectPh }}</option>
                <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.name }}</option>
              </select>
              <span v-if="projMissing" class="fe">{{ C.missingProject(projMissing) }}</span>
              <span v-else-if="err.projectId" class="fe">{{ err.projectId }}</span>
            </label>
            <label class="f">
              <span class="fl">{{ C.agentKind }}</span>
              <select v-model="form.agentKind" class="in" data-testid="sd-agent-kind">
                <option v-for="k in PROVIDERS" :key="k" :value="k">{{ providerLabel(k) }}</option>
              </select>
            </label>
            <div class="f">
              <span class="fl">{{ C.approval }}</span>
              <div class="seg" role="radiogroup" :aria-label="C.approval">
                <button
                  v-for="a in modeOpts"
                  :key="a.id"
                  type="button"
                  role="radio"
                  class="seg-b"
                  :class="{ on: form.approvalMode === a.id, warn: !!a.risky && form.approvalMode === a.id }"
                  :aria-checked="form.approvalMode === a.id"
                  :data-testid="'sd-approval-' + a.id"
                  @click="form.approvalMode = a.id"
                >{{ a.label }}</button>
              </div>
              <Transition name="sd-h">
                <p v-if="selRisky" key="w" class="warnbox" data-testid="sd-approve-warn"><FIcon name="alert" :size="14" />{{ C.approveWarn }}</p>
                <span v-else-if="form.approvalMode === 'normal'" key="n" class="fh" data-testid="sd-normal-hint">{{ C.normalHint }}</span>
              </Transition>
            </div>
            <div class="f">
              <span class="fl">{{ C.frequency }}</span>
              <div class="seg" role="radiogroup" :aria-label="C.frequency">
                <button
                  v-for="f in FREQS"
                  :key="f"
                  type="button"
                  role="radio"
                  class="seg-b"
                  :class="{ on: form.frequency === f }"
                  :aria-checked="form.frequency === f"
                  @click="form.frequency = f"
                >{{ C.freq[f] }}</button>
                <button type="button" class="seg-b dis" aria-disabled="true" :title="C.later" @click="notify(C.later)">{{ C.freq.custom }}</button>
              </div>
              <div class="when">
                <input v-if="form.frequency === 'once'" v-model="onceDate" type="date" class="in sm" :aria-label="C.date" />
                <input v-model="form.time" type="time" class="in sm" :aria-label="C.time" />
                <span class="tz">{{ C.timezone }} {{ tz }}</span>
              </div>
              <div v-if="form.frequency === 'weekly'" class="days">
                <button
                  v-for="(d, i) in C.weekdays"
                  :key="d"
                  type="button"
                  class="day"
                  :class="{ on: form.weekdays.includes(i + 1) }"
                  :aria-pressed="form.weekdays.includes(i + 1)"
                  @click="toggleDay(i + 1)"
                >{{ d }}</button>
              </div>
              <span v-if="err.weekdays" class="fe">{{ err.weekdays }}</span>
              <span v-else-if="err.runAt" class="fe">{{ err.runAt }}</span>
              <span v-else-if="preview" class="fh">{{ preview }}</span>
            </div>
            <details class="more">
              <summary>{{ C.more2 }}</summary>
              <div class="two">
                <label class="f">
                  <span class="fl">{{ C.model }}</span>
                  <select v-model="form.model" class="in" :disabled="!catState.models.length">
                    <option value="">{{ C.defaultOpt }}</option>
                    <option v-for="m in catState.models" :key="m.id" :value="m.id">{{ m.displayName }}</option>
                  </select>
                </label>
                <label class="f">
                  <span class="fl">{{ C.effort }}</span>
                  <select v-model="form.effort" class="in">
                    <option value="">{{ C.defaultOpt }}</option>
                    <option v-for="t in efforts" :key="t.id" :value="t.id">{{ t.displayName }}</option>
                  </select>
                </label>
              </div>
            </details>
          </div>
          <footer class="sd-foot">
            <label class="en">
              <button type="button" role="switch" class="sw" :class="{ on: form.enabled }" :aria-checked="form.enabled" :aria-label="C.enabled" @click="form.enabled = !form.enabled"><i /></button>
              {{ C.enabled }}
            </label>
            <span class="sp" />
            <button type="button" class="btn" :disabled="saving" @click="emit('close')">{{ C.cancel }}</button>
            <button type="submit" class="btn pri" :disabled="saving" data-testid="sd-save"><i v-if="saving" class="spin" aria-hidden="true" />{{ editing ? C.saveBtn : C.createBtn }}</button>
          </footer>
        </form>
      </div>
    </Transition>
    <CatConfirmDialog
      :open="confirmOpen"
      :title="C.confirmTitle"
      :body="C.approveWarn"
      :ok-text="C.confirmOk"
      :cancel-text="C.cancel"
      tone="warn"
      :busy="saving"
      @cancel="confirmOpen = false"
      @ok="doSave"
    />
  </Teleport>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { computed, nextTick, reactive, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import CatConfirmDialog from './CatConfirmDialog.vue'
import { useCatAgentCapabilities } from '@/views/cat/catCapabilities'
import { CAT_THINK_XHIGH_LABEL } from '@/api/cat'
import { CAT_PROVIDER_ORDER, normalizeProviderKind, providerLabel } from '@/api/catAgents'
import {
  type CatSchedule,
  type CatScheduleFrequency,
  type CatScheduleInput,
  CAT_SCHED_COPY as C,
  computeNextRun,
  createCatSchedule,
  fmtWhen,
  needsApproveConfirm,
  updateCatSchedule,
  validateSchedule,
} from '@/api/catSchedules'
import { catState, notify } from '@/views/cat/catState'
import { catPanels, scheduleDefaultAgentKind, scheduleDefaultPref } from '@/views/cat/catPanels'

const props = defineProps<{ open: boolean; schedule: CatSchedule | null; preset?: Partial<CatScheduleInput> | null }>()
const emit = defineEmits<{ close: []; saved: [CatSchedule] }>()

const FREQS: CatScheduleFrequency[] = ['once', 'daily', 'weekdays', 'weekly']
const PROVIDERS = CAT_PROVIDER_ORDER
const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || ''
const editing = computed(() => !!props.schedule)
const projects = computed(() => catState.projects)
const formCaps = useCatAgentCapabilities(() => form.agentKind ?? '', () => form.model ?? '')
/** 模式按所选服务商的列表渲染（契约 v0.44 前三家都是写死兜底） */
const modeOpts = computed(() => formCaps.value.modes.options)
const selRisky = computed(() => !!modeOpts.value.find((o) => o.id === form.approvalMode)?.risky)
/** 强度：该家有列表用列表；没有时沿用旧的四档兜底（与改造前行为一致） */
const efforts = computed(() =>
  formCaps.value.efforts.options.length
    ? formCaps.value.efforts.options.map((t) => ({ id: t.id, displayName: t.label }))
    : catState.thinks.length
    ? catState.thinks
    : [
        { id: 'low', displayName: t('cat.misc.s_19ac677ac0') },
        { id: 'medium', displayName: t('cat.misc.s_aed1dfbc31') },
        { id: 'high', displayName: t('cat.misc.s_4296d7d293') },
        { id: 'xhigh', displayName: CAT_THINK_XHIGH_LABEL },
      ],
)

function blank(): CatScheduleInput & { enabled: boolean } {
  // v0.40：默认服务商 = CatSettings.defaultAgentKind；模型/强度跟该家
  const agentKind = scheduleDefaultAgentKind()
  const d = scheduleDefaultPref(agentKind)
  return {
    name: '', prompt: '', projectId: catState.projects.find((p) => !p.missing)?.id ?? '', frequency: 'daily', time: '09:00', weekdays: [],
    runAt: 0, model: d.model, effort: d.effort, approvalMode: 'plan', agentKind, enabled: true,
  }
}
const form = reactive(blank())
const onceDate = ref('')
const tried = ref(false)
const saving = ref(false)
const confirmOpen = ref(false)
const nameEl = ref<HTMLInputElement>()

const pad = (n: number) => String(n).padStart(2, '0')
function dateStr(ts: number) {
  const d = new Date(ts)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

watch(
  () => props.open,
  async (v) => {
    if (!v) return
    tried.value = false
    confirmOpen.value = false
    const s = props.schedule
    Object.assign(form, blank(), props.preset ?? {}, s ? {
      name: s.name, prompt: s.prompt, projectId: s.projectDeleted ? '' : s.projectId, frequency: s.frequency, time: s.time || '09:00',
      weekdays: [...s.weekdays], runAt: s.runAt, model: s.model, effort: s.effort, approvalMode: s.approvalMode,
      agentKind: normalizeProviderKind(s.agentKind), enabled: s.enabled,
    } : {})
    const ref0 = form.runAt || Date.now() + 86_400_000
    onceDate.value = dateStr(ref0)
    if (form.frequency === 'once' && form.runAt) {
      const d = new Date(form.runAt)
      form.time = `${pad(d.getHours())}:${pad(d.getMinutes())}`
    }
    await nextTick()
    nameEl.value?.focus()
  },
  { immediate: true },
)

const input = computed<CatScheduleInput>(() => {
  let runAt = 0
  if (form.frequency === 'once' && onceDate.value && form.time) runAt = new Date(`${onceDate.value}T${form.time}:00`).getTime()
  return {
    name: form.name.trim(), prompt: form.prompt.trim(), projectId: form.projectId, frequency: form.frequency, time: form.time,
    weekdays: form.frequency === 'weekly' ? [...form.weekdays].sort() : [], runAt, model: form.model, effort: form.effort,
    approvalMode: form.approvalMode, agentKind: normalizeProviderKind(form.agentKind), enabled: form.enabled,
  }
})
const allErr = computed(() => validateSchedule(input.value))
const err = computed(() => (tried.value ? allErr.value : {}))
const projMissing = computed(() => {
  const p = catState.projects.find((x) => x.id === form.projectId)
  return p?.missing ? p.name : ''
})
const preview = computed(() => {
  const next = computeNextRun(input.value)
  if (!next) return ''
  const after = form.frequency === 'once' ? '' : t('cat.misc.s_1d14c4d10e', { n: form.frequency === 'weekly' ? t('cat.sched.everyWeek') + form.weekdays.slice().sort().map((w) => C.weekdays[w - 1]).join('、') : C.freq[form.frequency], v1: form.time })
  return t('cat.misc.s_7828179f5c', { n: fmtWhen(next), v1: after })
})

function toggleDay(d: number) {
  const i = form.weekdays.indexOf(d)
  if (i >= 0) form.weekdays.splice(i, 1)
  else form.weekdays.push(d)
}

function submit() {
  tried.value = true
  if (Object.keys(allErr.value).length || projMissing.value) return
  if (needsApproveConfirm(form.approvalMode, props.schedule ? props.schedule.approvalMode : null)) {
    confirmOpen.value = true
    return
  }
  void doSave()
}

async function doSave() {
  saving.value = true
  try {
    const s = props.schedule ? await updateCatSchedule(props.schedule.id, input.value) : await createCatSchedule(input.value)
    confirmOpen.value = false
    emit('saved', s)
  } catch {
    confirmOpen.value = false
    notify(C.saveFailed, 'warn')
  } finally {
    saving.value = false
  }
}
</script>

<style scoped src="./catPanel.css"></style>
<style scoped>
.sd-mask {
  position: fixed;
  inset: 0;
  z-index: 3100;
  display: grid;
  place-items: center;
  background: var(--cat-scrim);
}
.sd {
  width: 560px;
  max-width: calc(100vw - 32px);
  max-height: calc(100vh - 48px);
  display: flex;
  flex-direction: column;
  border-radius: var(--cat-radius-xl);
  background: var(--cat-elevated);
  border: 1px solid var(--cat-border);
  box-shadow: var(--cat-shadow-lg);
  color: var(--cat-text);
}
.sd-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px 8px;
}
.sd-head h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
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
  transition: background-color 120ms var(--ff-ease);
}
.ib:hover {
  background: var(--cat-hover);
  color: var(--cat-text);
}
.sd-body {
  padding: 4px 20px 12px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.f {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.fl {
  font-size: 12px;
  font-weight: 600;
  color: var(--cat-text-2);
}
.fl em {
  font-style: normal;
  font-weight: 400;
  margin-left: 8px;
  color: var(--cat-muted);
}
.in {
  height: 34px;
  padding: 0 10px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-bg);
  color: var(--cat-text);
  font: 13px var(--ff-font);
  transition: border-color 120ms var(--ff-ease), box-shadow 120ms var(--ff-ease);
}
.in.ta {
  height: auto;
  padding: 8px 10px;
  line-height: 1.6;
  resize: vertical;
}
.in.sm {
  width: 150px;
}
.in:focus {
  outline: none;
  border-color: var(--cat-accent);
  box-shadow: var(--cat-focus);
}
.in.bad {
  border-color: var(--cat-danger);
  animation: cat-shake 200ms var(--ff-ease);
}
.in:disabled {
  opacity: 0.55;
}
html.dark .in[type='date'],
html.dark .in[type='time'] {
  color-scheme: dark;
}
.fe {
  font-size: 12px;
  color: var(--cat-danger-text);
}
.fh {
  font-size: 12px;
  color: var(--cat-muted);
}
.seg {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 2px;
  border-radius: var(--cat-radius-md);
  background: var(--cat-sunken);
  border: 1px solid var(--cat-border);
  align-self: flex-start;
}
.seg-b {
  height: 28px;
  padding: 0 12px;
  border: 0;
  border-radius: var(--cat-radius-sm);
  background: none;
  color: var(--cat-text-2);
  font-size: 13px;
  cursor: pointer;
  transition: background-color 120ms var(--ff-ease), color 120ms var(--ff-ease), box-shadow 120ms var(--ff-ease);
}
.seg-b:hover:not(.dis) {
  color: var(--cat-text);
}
.seg-b.on {
  background: var(--cat-surface);
  color: var(--cat-text);
  box-shadow: var(--cat-shadow-sm);
  font-weight: 600;
}
.seg-b.warn {
  color: var(--cat-orange);
}
.seg-b.dis {
  opacity: 0.45;
  cursor: not-allowed;
}
.seg-b:focus-visible,
.day:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.warnbox {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  margin: 0;
  padding: 8px 10px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-orange-line);
  background: var(--cat-orange-soft);
  color: var(--cat-orange);
  font-size: 12px;
  line-height: 1.6;
}
.warnbox > :deep(svg) {
  flex: none;
  margin-top: 2px;
}
.when {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tz {
  font-size: 12px;
  color: var(--cat-muted);
}
.days {
  display: flex;
  gap: 6px;
}
.day {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: 1px solid var(--cat-border);
  background: var(--cat-bg);
  color: var(--cat-text-2);
  font-size: 12px;
  cursor: pointer;
  transition: background-color 120ms var(--ff-ease), color 120ms var(--ff-ease), border-color 120ms var(--ff-ease);
}
.day.on {
  background: var(--cat-accent);
  border-color: var(--cat-accent);
  color: var(--cat-on-accent);
}
.more summary {
  cursor: pointer;
  font-size: 12px;
  color: var(--cat-text-2);
  user-select: none;
}
.two {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 10px;
}
.sd-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 20px 16px;
  border-top: 1px solid var(--cat-divider);
}
.en {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--cat-text-2);
}
.sd-enter-active,
.sd-leave-active {
  transition: opacity 200ms var(--ff-ease);
}
.sd-enter-active .sd,
.sd-leave-active .sd {
  transition: transform 200ms var(--ff-ease), opacity 200ms var(--ff-ease);
}
.sd-enter-from,
.sd-leave-to {
  opacity: 0;
}
.sd-enter-from .sd,
.sd-leave-to .sd {
  opacity: 0;
  transform: translateY(8px) scale(0.98);
}
.sd-h-enter-active {
  transition: opacity 200ms var(--ff-ease), transform 200ms var(--ff-ease);
}
.sd-h-enter-from {
  opacity: 0;
  transform: translateY(-4px);
}
@media (prefers-reduced-motion: reduce) {
  .sd-enter-active,
  .sd-leave-active,
  .sd-enter-active .sd,
  .sd-leave-active .sd,
  .sd-h-enter-active,
  .seg-b,
  .day {
    transition: none !important;
  }
  .in.bad {
    animation: none;
  }
}
</style>
