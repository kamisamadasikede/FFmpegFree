<template>
  <Teleport to="body">
    <Transition name="ap">
      <div
        v-if="catState.addingProject"
        class="ap-mask"
        data-testid="cat-add-project"
        @mousedown.self="!busy && close()"
      >
        <div
          class="ap"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="apTitleId"
          @keydown.esc.stop="!busy && close()"
        >
          <header class="ap-head">
            <h3 :id="apTitleId">{{ title }}</h3>
            <button type="button" class="ib" :aria-label="PC.cancel" :disabled="busy" @click="close()"><FIcon name="x" :size="16" /></button>
          </header>

          <!-- A · 双入口 -->
          <template v-if="step === 'entry'">
            <div class="ap-body">
              <p class="lead">{{ PC.addLead }}</p>
              <div class="entry-grid" role="radiogroup" :aria-label="PC.addTitle">
                <button
                  type="button"
                  class="entry-card"
                  role="radio"
                  :class="{ on: entry === 'local' }"
                  :aria-checked="entry === 'local'"
                  data-testid="cat-add-local"
                  @click="entry = 'local'"
                >
                  <span class="ico"><FIcon name="folder" :size="20" /></span>
                  <b>{{ PC.entryLocal }}</b>
                  <small>{{ PC.entryLocalHint }}</small>
                </button>
                <button
                  type="button"
                  class="entry-card"
                  role="radio"
                  :class="{ on: entry === 'git' }"
                  :aria-checked="entry === 'git'"
                  data-testid="cat-add-git"
                  @click="entry = 'git'"
                >
                  <span class="ico"><FIcon name="branch" :size="20" /></span>
                  <b>{{ PC.entryGit }}</b>
                  <small>{{ PC.entryGitHint }}</small>
                </button>
              </div>
            </div>
            <footer class="ap-foot">
              <button type="button" class="btn" @click="close()">{{ PC.cancel }}</button>
              <button type="button" class="btn pri" data-testid="cat-add-continue" @click="onContinue">{{ PC.continue }}</button>
            </footer>
          </template>

          <!-- B / C / D / E · 克隆表单 / 进度 / 失败 -->
          <template v-else>
            <div class="ap-body">
              <div v-if="banner" class="banner" :class="banner.tone" data-testid="cat-clone-banner">
                <FIcon :name="banner.tone === 'warn' ? 'alert' : 'warn'" :size="16" />
                <div>
                  {{ banner.text }}
                  <button v-if="banner.hint" type="button" class="hint-link" @click="openGitHint">{{ PC.noGitHint }}</button>
                </div>
              </div>

              <div v-if="busy" class="prog" data-testid="cat-clone-progress">
                <div class="ph">
                  <b>{{ PC.cloningTitle }}</b>
                  <span v-if="progressPct != null">{{ progressPct }}%</span>
                </div>
                <div class="bar-track"><div class="bar-fill" :class="{ ind: progressPct == null }" :style="progressPct != null ? { width: progressPct + '%' } : undefined" /></div>
                <div class="prog-meta"><FIcon name="branch" :size="14" /><span>{{ progMeta }}</span></div>
              </div>

              <label class="fld" :class="{ dim: busy }">
                <span class="fl">{{ PC.cloneUrl }}</span>
                <input
                  ref="urlEl"
                  v-model="url"
                  class="inp"
                  :class="{ err: !!urlErr }"
                  :placeholder="PC.cloneUrlPh"
                  :disabled="busy"
                  :aria-invalid="!!urlErr"
                  data-testid="cat-clone-url"
                  @input="urlErr = ''"
                >
                <span v-if="urlErr" class="errt" data-testid="cat-clone-url-err">{{ urlErr }}</span>
              </label>

              <div class="fld" :class="{ dim: busy }">
                <span class="fl">{{ PC.cloneParent }} <small>{{ PC.cloneParentOpt }}</small></span>
                <div class="path-row">
                  <input
                    v-model="parentDir"
                    class="inp"
                    :class="{ err: !!parentErr }"
                    :disabled="busy"
                    :aria-invalid="!!parentErr"
                    data-testid="cat-clone-parent"
                    @input="parentErr = ''"
                  >
                  <button type="button" class="btn" :disabled="busy" data-testid="cat-clone-pick-parent" @click="pickParent">{{ PC.clonePickParent }}</button>
                </div>
                <span v-if="parentErr" class="errt" data-testid="cat-clone-parent-err">{{ parentErr }}</span>
              </div>

              <label class="fld" :class="{ dim: busy }">
                <span class="fl">{{ PC.cloneName }} <small>{{ PC.cloneNameHint }}</small></span>
                <input v-model="name" class="inp" :disabled="busy" :placeholder="defaultName" data-testid="cat-clone-name">
              </label>
            </div>
            <footer class="ap-foot">
              <button type="button" class="btn" :disabled="canceling" data-testid="cat-clone-cancel" @click="onCancel">{{ PC.cancel }}</button>
              <button type="button" class="btn pri" :disabled="busy || !url.trim()" data-testid="cat-clone-submit" @click="startClone">
                <FIcon v-if="!busy" name="download" :size="14" />{{ PC.cloneSubmit }}
              </button>
            </footer>
          </template>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { computed, nextTick, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import {
  CAT_PROJECT_COPY as PC,
  cancelCloneCatProject,
  cloneCatProject,
  cloneErrorPlacement,
  looksLikeGitUrl,
  onCatClone,
  pickCloneParentDir,
  repoNameFromUrl,
  type CatCloneEvent,
} from '@/api/catProjects'
import { AppError, toAppError } from '@/api/call'
import { catState, applyAddedProject, closeAddProject, createProjectFromFolder, notify } from '@/views/cat/catState'

const apTitleId = 'cat-add-project-title'
const step = ref<'entry' | 'clone'>('entry')
const entry = ref<'local' | 'git'>('git')
const url = ref('')
const parentDir = ref('')
const name = ref('')
const urlErr = ref('')
const parentErr = ref('')
const banner = ref<{ text: string; tone: 'err' | 'warn'; hint?: boolean } | null>(null)
const busy = ref(false)
const canceling = ref(false)
const cloneId = ref('')
const progress = ref<number | undefined>(undefined)
const urlEl = ref<HTMLInputElement>()
let unsub: (() => void) | undefined

const title = computed(() => (step.value === 'entry' ? PC.addTitle : busy.value ? PC.cloningTitle : PC.cloneTitle))
const defaultName = computed(() => (url.value.trim() ? repoNameFromUrl(url.value) : ''))
const progressPct = computed(() => (typeof progress.value === 'number' && Number.isFinite(progress.value) ? Math.round(Math.min(1, Math.max(0, progress.value)) * 100) : null))
const progMeta = computed(() => {
  const n = name.value.trim() || defaultName.value || '…'
  return t('cat.misc.s_a5c5a25f3f', { n: n })
})

watch(
  () => catState.addingProject,
  async (open) => {
    unsub?.()
    unsub = undefined
    if (!open) {
      busy.value = false
      canceling.value = false
      cloneId.value = ''
      return
    }
    step.value = 'entry'
    entry.value = 'git'
    url.value = ''
    parentDir.value = ''
    name.value = ''
    urlErr.value = ''
    parentErr.value = ''
    banner.value = null
    progress.value = undefined
    unsub = onCatClone(onCloneEvent)
  },
)

function close() {
  if (busy.value && cloneId.value) void cancelCloneCatProject({ cloneId: cloneId.value }).catch(() => {})
  closeAddProject()
}

async function onContinue() {
  if (entry.value === 'local') {
    closeAddProject()
    await createProjectFromFolder()
    return
  }
  step.value = 'clone'
  await nextTick()
  urlEl.value?.focus()
}

async function pickParent() {
  try {
    const p = await pickCloneParentDir()
    if (p) {
      parentDir.value = p
      parentErr.value = ''
    }
  } catch (e) {
    notify(cloneErrorPlacement(e).text, 'warn')
  }
}

function clearFieldErrors() {
  urlErr.value = ''
  parentErr.value = ''
  banner.value = null
}

function applyFail(e: unknown) {
  const { place, text } = cloneErrorPlacement(e)
  if (place === 'url') urlErr.value = text
  else if (place === 'parent') parentErr.value = text
  else banner.value = { text, tone: text === PC.noGit ? 'warn' : 'err', hint: text === PC.noGit }
}

function onCloneEvent(e: CatCloneEvent) {
  if (!cloneId.value || e.cloneId !== cloneId.value) return
  if (typeof e.progress === 'number') progress.value = e.progress
  if (e.status === 'cloning' || e.status === 'creating' || e.status === 'starting') {
    busy.value = true
    return
  }
  busy.value = false
  canceling.value = false
  if (e.status === 'succeeded' && e.project) {
    const existed = catState.projects.some((p) => p.id === e.project!.id)
    applyAddedProject(e.project, existed)
    closeAddProject()
    return
  }
  if (e.status === 'canceled') {
    cloneId.value = ''
    progress.value = undefined
    notify(PC.cloneCanceled)
    return
  }
  if (e.status === 'failed') {
    cloneId.value = ''
    progress.value = undefined
    const msg = (e.error?.message || '').trim() || t('cat.misc.s_e482af9575')
    applyFail(new AppError('PROCESS_FAILED', msg, e.error?.detail))
  }
}

async function startClone() {
  clearFieldErrors()
  const u = url.value.trim()
  if (!u) {
    urlErr.value = PC.badGitUrl
    return
  }
  if (!looksLikeGitUrl(u)) {
    urlErr.value = PC.badGitUrl
    return
  }
  if (busy.value || cloneId.value) return
  busy.value = true
  progress.value = undefined
  try {
    const r = await cloneCatProject({
      url: u,
      parentDir: parentDir.value.trim() || undefined,
      name: name.value.trim() || undefined,
    })
    cloneId.value = r.cloneId
    if (r.status === 'succeeded' && r.project) {
      applyAddedProject(r.project, false)
      closeAddProject()
      return
    }
    if (r.status === 'failed') {
      busy.value = false
      cloneId.value = ''
    }
  } catch (e) {
    busy.value = false
    cloneId.value = ''
    applyFail(e)
  }
}

async function onCancel() {
  if (!busy.value || !cloneId.value) {
    if (step.value === 'clone' && !busy.value) {
      step.value = 'entry'
      return
    }
    close()
    return
  }
  canceling.value = true
  try {
    await cancelCloneCatProject({ cloneId: cloneId.value })
  } catch {
    canceling.value = false
  }
}

function openGitHint() {
  // 一期不做安装器；点链接仅作提示（产品允许附简短说明）
  notify(t('cat.misc.s_0abef96e4f'))
}
</script>

<style scoped>
.ap-mask {
  position: fixed;
  inset: 0;
  z-index: 3100;
  display: grid;
  place-items: center;
  background: var(--cat-scrim);
}
.ap {
  width: 520px;
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
.ap-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px 8px;
}
.ap-head h3 {
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
}
.ib:hover:not(:disabled) {
  background: var(--cat-hover);
  color: var(--cat-text);
}
.ib:disabled { opacity: 0.5; cursor: default; }
.ap-body {
  padding: 4px 20px 12px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.lead {
  margin: 0;
  font-size: 13px;
  color: var(--cat-text-2);
}
.entry-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.entry-card {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  padding: 16px;
  text-align: left;
  border-radius: var(--cat-radius-lg);
  border: 1px solid var(--cat-border);
  background: var(--cat-surface, var(--cat-elevated));
  color: var(--cat-text);
  cursor: pointer;
  transition: border-color 120ms var(--ff-ease), box-shadow 120ms var(--ff-ease), background 120ms var(--ff-ease);
}
.entry-card:hover {
  border-color: var(--cat-border-strong, var(--cat-border));
  background: var(--cat-hover);
}
.entry-card.on {
  border-color: var(--cat-accent);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cat-accent) 22%, transparent), var(--cat-shadow-sm, none);
}
.entry-card .ico {
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  border-radius: var(--cat-radius-md);
  background: color-mix(in srgb, var(--cat-accent) 14%, transparent);
  color: var(--cat-accent);
  margin-bottom: 4px;
}
.entry-card b { font-size: 14px; font-weight: 600; }
.entry-card small { font-size: 12px; color: var(--cat-muted); line-height: 1.4; }
.fld {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.fld.dim { opacity: 0.55; pointer-events: none; }
.fl {
  font-size: 12px;
  font-weight: 600;
  color: var(--cat-text-2);
}
.fl small {
  font-weight: 400;
  color: var(--cat-muted);
  margin-left: 6px;
}
.inp {
  height: 36px;
  padding: 0 12px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-input, var(--cat-elevated));
  color: var(--cat-text);
  font-size: 13px;
}
.inp:focus {
  outline: none;
  border-color: var(--cat-accent);
  box-shadow: var(--cat-focus);
}
.inp.err {
  border-color: var(--cat-danger);
}
.errt {
  font-size: 12px;
  color: var(--cat-danger);
}
.path-row {
  display: flex;
  gap: 8px;
}
.path-row .inp { flex: 1; min-width: 0; }
.ap-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 20px 16px;
  border-top: 1px solid var(--cat-border);
}
.btn {
  height: 34px;
  padding: 0 14px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-elevated);
  color: var(--cat-text);
  font-size: 13px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.btn:hover:not(:disabled) { background: var(--cat-hover); }
.btn:disabled { opacity: 0.5; cursor: default; }
.btn.pri {
  border-color: transparent;
  background: var(--cat-accent);
  color: #fff;
}
.btn.pri:hover:not(:disabled) { filter: brightness(1.05); }
.banner {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 10px 12px;
  border-radius: var(--cat-radius-md);
  font-size: 13px;
  line-height: 1.45;
}
.banner.err {
  background: color-mix(in srgb, var(--cat-danger) 12%, transparent);
  color: var(--cat-danger);
}
.banner.warn {
  background: color-mix(in srgb, var(--cat-warn, #c47a00) 14%, transparent);
  color: var(--cat-text);
}
.hint-link {
  margin-left: 6px;
  border: 0;
  background: none;
  color: var(--cat-accent);
  cursor: pointer;
  font-size: inherit;
  text-decoration: underline;
  padding: 0;
}
.prog {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-hover);
}
.ph {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
}
.ph b { font-weight: 600; }
.ph span { color: var(--cat-muted); }
.bar-track {
  height: 6px;
  border-radius: 99px;
  background: var(--cat-border);
  overflow: hidden;
}
.bar-fill {
  height: 100%;
  border-radius: 99px;
  background: var(--cat-accent);
  transition: width 200ms var(--ff-ease);
}
.bar-fill.ind {
  width: 40%;
  animation: ap-ind 1.1s ease-in-out infinite;
}
@keyframes ap-ind {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(280%); }
}
.prog-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--cat-muted);
}
.ap-enter-active, .ap-leave-active { transition: opacity 160ms var(--ff-ease); }
.ap-enter-from, .ap-leave-to { opacity: 0; }
@media (prefers-reduced-motion: reduce) {
  .ap-enter-active, .ap-leave-active, .bar-fill.ind { transition: none; animation: none; }
}
</style>
