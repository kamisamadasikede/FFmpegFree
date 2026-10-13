<template>
  <section class="cp" aria-labelledby="cs-h" data-testid="cat-settings">
    <CatPanelNotice />
    <div class="cp-in">
      <header class="cp-head">
        <h2 id="cs-h">{{ C.title }}</h2>
        <p>{{ C.subtitle }}</p>
      </header>
      <div class="cs-tabs" role="tablist" :aria-label="C.title">
        <button
          v-for="t in TABS"
          :key="t"
          type="button"
          role="tab"
          class="cs-tab"
          :class="{ on: catPanels.tab === t }"
          :aria-selected="catPanels.tab === t"
          :data-testid="'cat-settings-tab-' + t"
          @click="catPanels.tab = t"
        >{{ C.tabs[t] }}</button>
      </div>

      <Transition name="cp-sw" mode="out-in">
        <!-- 通用 -->
        <div v-if="catPanels.tab === 'general'" key="g" class="cs-body">
          <div class="card">
            <div class="card-h">{{ C.newConvCard }}</div>
            <div class="row">
              <div class="lab"><b>{{ C.model }}</b><span>{{ C.modelDesc }}</span></div>
              <div class="ctl">
                <SavedMark :on="saved.defaultModel" />
                <select
                  class="sel"
                  :value="modelValue"
                  :disabled="!catState.models.length"
                  :aria-label="C.model"
                  data-testid="cat-set-model"
                  @change="set('defaultModel', ($event.target as HTMLSelectElement).value)"
                >
                  <option value="">{{ C.defaultOpt }}</option>
                  <option v-for="m in catState.models" :key="m.id" :value="m.id">{{ m.displayName }}</option>
                </select>
              </div>
            </div>
            <div class="row">
              <div class="lab"><b>{{ C.effort }}</b><span>{{ C.effortDesc }}</span></div>
              <div class="ctl">
                <SavedMark :on="saved.defaultEffort" />
                <select
                  class="sel"
                  :value="catPanels.settings.defaultEffort"
                  :aria-label="C.effort"
                  data-testid="cat-set-effort"
                  @change="set('defaultEffort', ($event.target as HTMLSelectElement).value)"
                >
                  <option value="">{{ C.defaultOpt }}</option>
                  <option v-for="t in effortOptions" :key="t.id" :value="t.id">{{ t.displayName }}</option>
                </select>
              </div>
            </div>
            <div class="row">
              <div class="lab"><b>{{ C.approval }}</b><span>{{ C.approvalDesc }}</span></div>
              <div class="ctl">
                <SavedMark :on="saved.defaultApprovalMode" />
                <select
                  class="sel"
                  :class="{ warn: approvalValue === 'approve' }"
                  :value="approvalValue"
                  :aria-label="C.approval"
                  data-testid="cat-set-approval"
                  @change="set('defaultApprovalMode', ($event.target as HTMLSelectElement).value)"
                >
                  <option v-for="a in CAT_ACCESS" :key="a.id" :value="a.id">{{ a.name }}</option>
                </select>
              </div>
            </div>
            <div class="row">
              <div class="lab"><b>{{ C.place }}</b><span>{{ C.placeDesc }}</span></div>
              <div class="ctl">
                <SavedMark :on="localSaved.place" />
                <select class="sel" :value="placeValue" :aria-label="C.place" data-testid="cat-set-place" @change="onPlace(($event.target as HTMLSelectElement).value)">
                  <option value="last">{{ C.placeLast }}</option>
                  <option value="none">{{ C.placeNone }}</option>
                  <option v-for="p in catState.projects" :key="p.id" :value="p.id" :disabled="p.missing">{{ p.name }}</option>
                </select>
              </div>
            </div>
          </div>

          <div class="card">
            <div class="card-h">{{ C.looksCard }}</div>
            <div class="row">
              <div class="lab"><b>{{ C.theme }}</b><span>{{ C.themeDesc }}</span></div>
              <div class="ctl ro">{{ isDark ? C.themeDark : C.themeLight }}</div>
            </div>
            <div class="row">
              <div class="lab"><b>{{ C.typewriter }}</b><span>{{ C.typewriterDesc }}</span></div>
              <div class="ctl">
                <SavedMark :on="localSaved.typewriter" />
                <button
                  type="button"
                  role="switch"
                  class="sw"
                  :class="{ on: catLocalPrefs.typewriter }"
                  :aria-checked="catLocalPrefs.typewriter"
                  :aria-label="C.typewriter"
                  data-testid="cat-set-typewriter"
                  @click="onTypewriter"
                ><i /></button>
              </div>
            </div>
          </div>

          <div class="card">
            <div class="card-h">{{ C.convCard }}</div>
            <div class="row">
              <div class="lab"><b>{{ C.notify }}</b><span>{{ C.notifyDesc }}</span></div>
              <div class="ctl">
                <SavedMark :on="saved.notifyOnFinish" />
                <button
                  type="button"
                  role="switch"
                  class="sw"
                  :class="{ on: catPanels.settings.notifyOnFinish }"
                  :aria-checked="catPanels.settings.notifyOnFinish"
                  :aria-label="C.notify"
                  data-testid="cat-set-notify"
                  @click="set('notifyOnFinish', !catPanels.settings.notifyOnFinish)"
                ><i /></button>
              </div>
            </div>
            <div class="row dim">
              <div class="lab"><b>{{ C.history }}</b><span>{{ C.later }}</span></div>
              <div class="ctl"><span class="chip">{{ C.later }}</span></div>
            </div>
            <div class="row">
              <div class="lab"><b>{{ C.clear }}</b><span>{{ C.clearDesc }}</span></div>
              <div class="ctl">
                <button type="button" class="btn danger" data-testid="cat-set-clear" @click="clearOpen = true">{{ C.clearBtn }}</button>
              </div>
            </div>
          </div>

          <div class="card">
            <div class="card-h">{{ C.keysCard }}</div>
            <div class="keys">
              <div v-for="k in C.keys" :key="k[0]" class="key"><span>{{ k[0] }}</span><kbd>{{ mod }} {{ k[1] }}</kbd></div>
              <div class="key"><span>{{ C.stopKey }}</span><kbd>Esc</kbd></div>
            </div>
          </div>
        </div>

        <!-- 自定义指令 -->
        <div v-else-if="catPanels.tab === 'instruction'" key="i" class="cs-body">
          <div class="card pad">
            <p class="tip">{{ C.instrHint }}</p>
            <textarea
              v-model="draft"
              class="ta"
              rows="12"
              :placeholder="C.instrPlaceholder"
              :aria-label="C.tabs.instruction"
              :aria-invalid="over"
              data-testid="cat-set-instruction"
            />
            <div class="ta-foot">
              <span :class="{ bad: over }">
                <template v-if="over">{{ C.instrOver }}</template>
                <template v-else>{{ dirty ? '' : C.saved + ' · ' }}{{ draft.length }} / {{ MAX }} 字</template>
              </span>
              <span class="sp" />
              <button type="button" class="btn" :disabled="!dirty || savingInstr" @click="draft = catPanels.settings.customInstruction">{{ C.instrRevert }}</button>
              <button type="button" class="btn pri" :disabled="!dirty || over || savingInstr" data-testid="cat-set-instruction-save" @click="saveInstr">
                <i v-if="savingInstr" class="spin" aria-hidden="true" />{{ C.instrSave }}
              </button>
            </div>
          </div>
        </div>

        <!-- 技能与扩展（只读） -->
        <div v-else-if="catPanels.tab === 'ext'" key="e" class="cs-body">
          <p class="note"><FIcon name="info" :size="14" />{{ C.extTip }}</p>
          <div v-if="inspect.loading" class="card" aria-busy="true">
            <div v-for="i in 3" :key="i" class="sk"><i /><b /><span /></div>
          </div>
          <div v-else-if="inspect.result?.reason" class="card pad fail" data-testid="cat-ext-failed">
            <span>{{ C.extFailed }}</span>
            <button type="button" class="btn" @click="loadInspect">{{ C.retry }}</button>
          </div>
          <template v-else-if="inspect.result">
            <div v-for="g in groups" :key="g.key" class="card">
              <div class="card-h">{{ g.title }} · {{ g.items.length }}</div>
              <div v-if="!g.items.length" class="empty">{{ C.extEmpty }}</div>
              <div v-for="it in g.items" :key="g.key + it.name" class="row it">
                <FIcon :name="(g.icon as any)" :size="15" class="it-ic" />
                <div class="lab">
                  <b>{{ it.name }}<em v-if="it.tag" class="tag">{{ it.tag }}</em></b>
                  <span v-if="it.description">{{ it.description }}</span>
                </div>
                <div class="ctl st" :class="{ off: !it.enabled }"><i />{{ it.enabled ? C.on : C.off }}</div>
              </div>
            </div>
          </template>
        </div>

        <!-- 关于与状态 -->
        <div v-else key="a" class="cs-body">
          <div class="card">
            <div class="card-h">{{ C.statusCard }}</div>
            <div class="row"><div class="lab"><b>{{ C.detect }}</b></div><div class="ctl st" :class="statusTone"><i />{{ statusText }}</div></div>
            <div class="row"><div class="lab"><b>{{ C.version }}</b></div><div class="ctl ro">{{ version || C.dash }}</div></div>
            <div v-if="catState.status.state === 'ready' || authBad" class="row">
              <div class="lab"><b>{{ C.login }}</b></div>
              <div class="ctl st" :class="authBad ? 'bad' : 'ok'"><i />{{ authBad ? C.loginBad : C.loggedIn }}</div>
            </div>
            <div class="row"><div class="lab"><b>{{ C.lastCheck }}</b></div><div class="ctl ro">{{ lastCheckText }}</div></div>
            <div class="row end">
              <button type="button" class="btn" :disabled="rechecking" data-testid="cat-set-recheck" @click="recheck">
                <i v-if="rechecking" class="spin" aria-hidden="true" />{{ rechecking ? C.rechecking : C.recheck }}
              </button>
            </div>
            <div v-if="catNotReady" class="bar" role="status">{{ authBad ? CAT_COPY.authInvalid : CAT_COPY.notReady }}</div>
          </div>
          <div class="card">
            <div class="card-h">{{ C.aboutCard }}</div>
            <div class="row"><div class="lab"><b>{{ C.aboutLine }}</b></div><div class="ctl ro">{{ appVersion }}</div></div>
          </div>
        </div>
      </Transition>
    </div>

    <CatConfirmDialog
      :open="clearOpen"
      :title="C.clearTitle"
      :body="C.clearBody"
      :ok-text="C.clearOk"
      :cancel-text="C.cancel"
      tone="danger"
      :busy="clearing"
      @cancel="clearOpen = false"
      @ok="doClear"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import CatPanelNotice from './CatPanelNotice.vue'
import CatConfirmDialog from './CatConfirmDialog.vue'
import { CAT_COPY, CAT_THINK_XHIGH_LABEL, getCatStatus, recheckCat } from '@/api/cat'
import { CAT_ACCESS } from '@/api/catMock'
import {
  type CatInspectResult,
  type CatSettings,
  CAT_INSTRUCTION_MAX as MAX,
  CAT_SETTINGS_COPY,
  clearAllCatConversations,
  inspectCatConfig,
} from '@/api/catSettings'
import { getAppVersion } from '@/api/about'
import { catPanels, saveSetting, type CatSettingsTab } from '@/views/cat/catPanels'
import { catLocalPrefs, setNewConvPlace, setTypewriter } from '@/views/cat/catLocalPrefs'
import { catNotReady, catState, afterClearAll, notify } from '@/views/cat/catState'

defineOptions({ name: 'CatSettingsView' })
const C = { ...CAT_SETTINGS_COPY, dash: '—' }
const TABS: CatSettingsTab[] = ['general', 'instruction', 'ext', 'about']

/** 行尾 1.5 秒「已保存」 */
const SavedMark = defineComponent({
  props: { on: Boolean },
  setup: (p) => () => h('span', { class: ['saved', { on: p.on }], 'aria-live': 'polite' }, p.on ? C.saved : ''),
})

const saved = reactive<Record<keyof CatSettings, boolean>>({
  defaultModel: false, defaultEffort: false, defaultApprovalMode: false, customInstruction: false, notifyOnFinish: false,
})
const savedTimers: Partial<Record<keyof CatSettings, ReturnType<typeof setTimeout>>> = {}
function flash(k: keyof CatSettings) {
  saved[k] = true
  clearTimeout(savedTimers[k])
  savedTimers[k] = setTimeout(() => (saved[k] = false), 1500)
}

async function set<K extends keyof CatSettings>(key: K, value: CatSettings[K]) {
  const ok = await saveSetting(key, value)
  if (ok) flash(key)
  else notify(C.saveFailed, 'warn')
}

/** 默认模型已不在列表里时按「默认」显示（契约 6.19.12.1-2） */
const modelValue = computed(() => {
  const id = catPanels.settings.defaultModel
  return catState.models.some((m) => m.id === id) ? id : ''
})
const approvalValue = computed(() => catPanels.settings.defaultApprovalMode || 'approve')
/** 后端还没给强度列表时按契约四档显示 */
const effortOptions = computed(() =>
  catState.thinks.length
    ? catState.thinks
    : [
        { id: 'low', displayName: '低' },
        { id: 'medium', displayName: '中' },
        { id: 'high', displayName: '高' },
        { id: 'xhigh', displayName: CAT_THINK_XHIGH_LABEL },
      ],
)

const isDark = ref(typeof document !== 'undefined' && document.documentElement.classList.contains('dark'))
let mo: MutationObserver | undefined
const mod = typeof navigator !== 'undefined' && /mac/i.test(navigator.platform) ? '⌘' : 'Ctrl'

// ---- 本机偏好（localStorage，不走 SaveCatSettings） ----
const localSaved = reactive({ place: false, typewriter: false })
function flashLocal(k: 'place' | 'typewriter') {
  localSaved[k] = true
  setTimeout(() => (localSaved[k] = false), 1500)
}
/** 选中的项目被删了就按「上次用的项目」显示 */
const placeValue = computed(() => {
  const v = catLocalPrefs.newConvPlace
  return v === 'last' || v === 'none' || catState.projects.some((p) => p.id === v) ? v : 'last'
})
function onPlace(v: string) {
  setNewConvPlace(v)
  flashLocal('place')
}
function onTypewriter() {
  setTypewriter(!catLocalPrefs.typewriter)
  flashLocal('typewriter')
}

// ---- 自定义指令 ----
const draft = ref(catPanels.settings.customInstruction)
watch(
  () => catPanels.settings.customInstruction,
  (v, old) => {
    if (draft.value === old) draft.value = v
  },
)
const dirty = computed(() => draft.value !== catPanels.settings.customInstruction)
const over = computed(() => draft.value.length > MAX)
const savingInstr = ref(false)
async function saveInstr() {
  savingInstr.value = true
  const keep = draft.value
  const ok = await saveSetting('customInstruction', keep)
  savingInstr.value = false
  if (ok) flash('customInstruction')
  else {
    draft.value = keep // 失败保留输入框内容
    notify(C.saveFailed, 'warn')
  }
}

// ---- 技能与扩展 ----
const inspect = reactive({ loading: false, result: null as CatInspectResult | null })
async function loadInspect() {
  inspect.loading = true
  inspect.result = await inspectCatConfig('')
  inspect.loading = false
}
const groups = computed(() => {
  const r = inspect.result
  if (!r) return []
  return [
    { key: 's', title: C.skills, icon: 'spark', items: r.skills.map((x) => ({ ...x, tag: '' })) },
    {
      key: 'x',
      title: C.exts,
      icon: 'magic',
      items: [...r.plugins.map((x) => ({ ...x, tag: '插件' })), ...r.mcpServers.map((x) => ({ ...x, tag: 'MCP' }))],
    },
    { key: 'h', title: C.hooks, icon: 'link', items: r.hooks.map((x) => ({ ...x, tag: '' })) },
  ]
})
watch(
  () => catPanels.tab,
  (t) => {
    if (t === 'ext' && !inspect.result && !inspect.loading) void loadInspect()
  },
  { immediate: true },
)

// ---- 关于与状态 ----
const rechecking = ref(false)
const lastCheck = ref(Date.now())
const appVersion = ref('')
const authBad = computed(() => catState.status.error?.code === 'CAT_NOT_READY' && /reason=auth/.test(catState.status.error?.detail ?? ''))
const version = computed(() => catState.status.version || inspect.result?.version || '')
const statusText = computed(() =>
  catState.status.state === 'ready' ? C.ready : catState.status.state === 'checking' ? C.checking : C.missing,
)
const statusTone = computed(() => (catState.status.state === 'ready' ? 'ok' : catState.status.state === 'checking' ? 'wait' : 'bad'))
const lastCheckText = computed(() => {
  const d = new Date(lastCheck.value)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
})
async function recheck() {
  rechecking.value = true
  try {
    catState.status = await recheckCat()
  } catch {
    /* 保持原状态 */
  }
  lastCheck.value = Date.now()
  rechecking.value = false
}

// ---- 清空 ----
const clearOpen = ref(false)
const clearing = ref(false)
async function doClear() {
  clearing.value = true
  try {
    await clearAllCatConversations()
    clearOpen.value = false
    await afterClearAll()
    notify(C.cleared)
  } catch {
    clearOpen.value = false
    notify(C.clearFailed, 'warn')
  } finally {
    clearing.value = false
  }
}

onMounted(async () => {
  mo = new MutationObserver(() => (isDark.value = document.documentElement.classList.contains('dark')))
  mo.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  try {
    appVersion.value = (await getAppVersion()) || ''
  } catch {
    /* 没有就不显示 */
  }
  if (catState.status.state === 'checking') {
    try {
      catState.status = await getCatStatus()
    } catch {
      /* 保持 */
    }
  }
})
</script>

<style scoped src="./catPanel.css"></style>
<style scoped>
.cs-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--cat-border);
  margin-bottom: 16px;
}
.cs-tab {
  position: relative;
  height: 34px;
  padding: 0 10px;
  border: 0;
  background: none;
  color: var(--cat-text-2);
  font-size: 13px;
  cursor: pointer;
  transition: color 120ms var(--ff-ease);
}
.cs-tab::after {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  bottom: -1px;
  height: 2px;
  border-radius: 2px;
  background: var(--cat-accent);
  transform: scaleX(0);
  transition: transform 200ms var(--ff-ease);
}
.cs-tab:hover {
  color: var(--cat-text);
}
.cs-tab.on {
  color: var(--cat-text);
  font-weight: 600;
}
.cs-tab.on::after {
  transform: scaleX(1);
}
.cs-tab:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
  border-radius: var(--cat-radius-sm);
}
.cs-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.keys {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 24px;
  padding: 4px 14px 10px;
}
.key {
  display: flex;
  justify-content: space-between;
  align-items: center;
  height: 34px;
  font-size: 13px;
  color: var(--cat-text-2);
}
kbd {
  font: 11px var(--ff-font-mono);
  padding: 2px 6px;
  border-radius: 4px;
  border: 1px solid var(--cat-border);
  background: var(--cat-sunken);
  color: var(--cat-text-2);
}
.ta {
  width: 100%;
  box-sizing: border-box;
  resize: vertical;
  min-height: 220px;
  padding: 10px 12px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-bg);
  color: var(--cat-text);
  font: 13px/1.6 var(--ff-font);
  transition: border-color 120ms var(--ff-ease), box-shadow 120ms var(--ff-ease);
}
.ta:focus {
  outline: none;
  border-color: var(--cat-accent);
  box-shadow: var(--cat-focus);
}
.ta[aria-invalid='true'] {
  border-color: var(--cat-danger);
}
.ta-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
  font-size: 12px;
  color: var(--cat-muted);
}
.ta-foot .bad {
  color: var(--cat-danger-text);
}
.sk {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
}
.sk i,
.sk b,
.sk span {
  display: block;
  border-radius: 4px;
  background: var(--cat-sunken);
  animation: cat-breathe 1.2s ease-in-out infinite;
}
.sk i {
  width: 16px;
  height: 16px;
}
.sk b {
  width: 120px;
  height: 12px;
}
.sk span {
  flex: 1;
  height: 10px;
}
.fail {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--cat-danger-text);
  font-size: 13px;
}
.it-ic {
  color: var(--cat-muted);
  flex: none;
}
.tag {
  font-style: normal;
  font-size: 11px;
  font-weight: 500;
  margin-left: 6px;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--cat-sunken);
  color: var(--cat-text-2);
}
.bar {
  margin: 4px 14px 12px;
  padding: 8px 10px;
  border-radius: var(--cat-radius-sm);
  background: var(--cat-danger-soft);
  color: var(--cat-danger-text);
  font-size: 12px;
}
.row.end {
  justify-content: flex-end;
}
</style>
