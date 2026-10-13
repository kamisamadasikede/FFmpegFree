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
            <p class="card-tip">{{ C.newConvHint }}</p>

            <div
              v-for="(block, bi) in providerBlocks"
              :key="block.kind"
              class="agent"
              :class="{ dim: !block.ready, sep: bi > 0 }"
              :data-testid="'cat-set-agent-' + block.kind"
            >
              <div class="agent-hd">
                <CatAgentMark :kind="block.kind" :size="18" />
                <div class="lab">
                  <b>{{ block.label }}</b>
                  <span :class="block.ready ? 'okt' : 'badt'">{{ block.ready ? C.installed : C.notDetected }}</span>
                </div>
              </div>
              <div class="row">
                <div class="lab"><b>{{ C.model }}</b><span>{{ C.modelDesc }}</span></div>
                <div class="ctl">
                  <SavedMark :on="!!savedAgent[block.kind + ':model']" />
                  <select
                    class="sel"
                    :value="modelValue(block.kind)"
                    :disabled="!block.ready || !block.models.length"
                    :aria-label="block.label + ' ' + C.model"
                    :data-testid="'cat-set-model-' + block.kind"
                    @change="setAgent(block.kind, 'model', ($event.target as HTMLSelectElement).value)"
                  >
                    <option value="">{{ C.defaultOpt }}</option>
                    <option v-for="m in block.models" :key="m.id" :value="m.id">{{ m.displayName }}</option>
                  </select>
                </div>
              </div>
              <div v-if="block.thinks.length" class="row">
                <div class="lab"><b>{{ C.effort }}</b><span>{{ C.effortDesc }}</span></div>
                <div class="ctl">
                  <SavedMark :on="!!savedAgent[block.kind + ':effort']" />
                  <select
                    class="sel"
                    :value="effortValue(block.kind)"
                    :disabled="!block.ready"
                    :aria-label="block.label + ' ' + C.effort"
                    :data-testid="'cat-set-effort-' + block.kind"
                    @change="setAgent(block.kind, 'effort', ($event.target as HTMLSelectElement).value)"
                  >
                    <option value="">{{ C.defaultOpt }}</option>
                    <option v-for="t in block.thinks" :key="t.id" :value="t.id">{{ t.displayName }}</option>
                  </select>
                </div>
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

        <!-- 关于与状态（按 agents[] 拆行） -->
        <div v-else key="a" class="cs-body">
          <div class="card">
            <div class="card-h">{{ C.statusCard }}</div>
            <div
              v-for="row in agentRows"
              :key="row.kind"
              class="agent-row"
              :class="{ dim: !row.ready && catState.status.state !== 'checking' }"
              :data-testid="'cat-status-' + row.kind"
            >
              <div class="row">
                <CatAgentMark :kind="row.kind" :size="16" />
                <div class="lab"><b>{{ row.label }}</b></div>
                <div class="ctl st" :class="row.tone"><i />{{ row.statusText }}</div>
              </div>
              <div v-if="row.version" class="row sub"><div class="lab"><b>{{ C.version }}</b></div><div class="ctl ro">{{ row.version }}</div></div>
              <div v-if="row.loginText" class="row sub">
                <div class="lab"><b>{{ C.login }}</b></div>
                <div class="ctl st" :class="row.loginTone"><i />{{ row.loginText }}</div>
              </div>
              <div v-if="row.loginBad" class="bar" role="status">{{ C.loginBanner(row.label) }}</div>
            </div>
            <div class="row end">
              <button type="button" class="btn" :disabled="rechecking" data-testid="cat-set-recheck" @click="recheck">
                <i v-if="rechecking" class="spin" aria-hidden="true" />{{ rechecking ? C.rechecking : C.recheck }}
              </button>
            </div>
            <div v-if="noneReady" class="bar" role="status">{{ C.noneReady }}</div>
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
import CatAgentMark from '@/components/cat/CatAgentMark.vue'
import CatPanelNotice from './CatPanelNotice.vue'
import CatConfirmDialog from './CatConfirmDialog.vue'
import {
  CAT_COPY,
  getCatStatus,
  listCatModels,
  listCatThinkLevels,
  recheckCat,
  type CatModel,
  type CatThinkLevel,
} from '@/api/cat'
import {
  CAT_PROVIDER_LABEL,
  CAT_PROVIDER_ORDER,
  type CatProviderKind,
} from '@/api/catAgents'
import { CAT_ACCESS } from '@/api/catMock'
import {
  type CatInspectResult,
  type CatSettings,
  CAT_INSTRUCTION_MAX as MAX,
  CAT_SETTINGS_COPY,
  clearAllCatConversations,
  emptyDefaults,
  inspectCatConfig,
  loginBanner,
} from '@/api/catSettings'
import { getAppVersion } from '@/api/about'
import { catPanels, saveSetting, type CatSettingsTab } from '@/views/cat/catPanels'
import { catLocalPrefs, setNewConvPlace, setTypewriter } from '@/views/cat/catLocalPrefs'
import { catState, afterClearAll, notify } from '@/views/cat/catState'
import { catSim, simModels, simThinks } from '@/views/cat/catDevSim'

defineOptions({ name: 'CatSettingsView' })
const C = { ...CAT_SETTINGS_COPY, dash: '—', loginBanner }
const TABS: CatSettingsTab[] = ['general', 'instruction', 'ext', 'about']

const SavedMark = defineComponent({
  props: { on: Boolean },
  setup: (p) => () => h('span', { class: ['saved', { on: p.on }], 'aria-live': 'polite' }, p.on ? C.saved : ''),
})

type TopKey = 'defaultApprovalMode' | 'customInstruction' | 'notifyOnFinish' | 'defaultAgentKind' | 'defaultsByAgent'
const saved = reactive<Record<TopKey, boolean>>({
  defaultApprovalMode: false, customInstruction: false, notifyOnFinish: false, defaultAgentKind: false, defaultsByAgent: false,
})
const savedTimers: Partial<Record<TopKey, ReturnType<typeof setTimeout>>> = {}
function flash(k: TopKey) {
  saved[k] = true
  clearTimeout(savedTimers[k])
  savedTimers[k] = setTimeout(() => (saved[k] = false), 1500)
}

const savedAgent = reactive<Record<string, boolean>>({})
const savedAgentTimers: Record<string, ReturnType<typeof setTimeout>> = {}
function flashAgent(key: string) {
  savedAgent[key] = true
  clearTimeout(savedAgentTimers[key])
  savedAgentTimers[key] = setTimeout(() => (savedAgent[key] = false), 1500)
}

async function set<K extends TopKey>(key: K, value: CatSettings[K]) {
  const ok = await saveSetting(key, value)
  if (ok) flash(key)
  else notify(C.saveFailed, 'warn')
}

async function setAgent(kind: CatProviderKind, field: 'model' | 'effort', value: string) {
  const prev = { ...catPanels.settings.defaultsByAgent }
  const cur = { ...(prev[kind] ?? emptyDefaults()), [field]: value }
  const next = { ...prev, [kind]: cur }
  const ok = await saveSetting('defaultsByAgent', next)
  if (ok) flashAgent(`${kind}:${field}`)
  else notify(C.saveFailed, 'warn')
}

const caps = reactive({
  models: {} as Record<CatProviderKind, CatModel[]>,
  thinks: {} as Record<CatProviderKind, CatThinkLevel[]>,
  loaded: false,
})

async function loadCaps() {
  const useSim = import.meta.env.DEV && catSim.has('caps')
  await Promise.all(
    CAT_PROVIDER_ORDER.map(async (kind) => {
      if (useSim) {
        // 走查：三家都给列表；Codex 故意空强度 → 测「整行隐藏」
        caps.models[kind] = simModels().map((m) => ({ ...m, id: `${kind}:${m.id}`, displayName: m.displayName }))
        caps.thinks[kind] = kind === 'codex_cli' ? [] : simThinks()
      } else {
        const [models, thinks] = await Promise.all([listCatModels(kind), listCatThinkLevels(kind)])
        caps.models[kind] = models
        caps.thinks[kind] = thinks
      }
    }),
  )
  caps.loaded = true
}

function agentReady(kind: CatProviderKind): boolean {
  const a = catState.status.agents?.find((x) => x.agentKind === kind)
  if (a) return a.ready
  // 旧后端没 agents[]：顶层 ready 只算 cat_build
  return kind === 'cat_build' && catState.status.state === 'ready'
}

const providerBlocks = computed(() =>
  CAT_PROVIDER_ORDER.map((kind) => ({
    kind,
    label: CAT_PROVIDER_LABEL[kind],
    ready: agentReady(kind),
    models: caps.models[kind] ?? [],
    thinks: caps.thinks[kind] ?? [],
  })),
)

function modelValue(kind: CatProviderKind) {
  const id = catPanels.settings.defaultsByAgent[kind]?.model ?? ''
  const list = caps.models[kind] ?? []
  return list.some((m) => m.id === id) ? id : ''
}
function effortValue(kind: CatProviderKind) {
  const id = catPanels.settings.defaultsByAgent[kind]?.effort ?? ''
  const list = caps.thinks[kind] ?? []
  return list.some((t) => t.id === id) ? id : ''
}

const approvalValue = computed(() => catPanels.settings.defaultApprovalMode || 'approve')

const isDark = ref(typeof document !== 'undefined' && document.documentElement.classList.contains('dark'))
let mo: MutationObserver | undefined
const mod = typeof navigator !== 'undefined' && /mac/i.test(navigator.platform) ? '⌘' : 'Ctrl'

const localSaved = reactive({ place: false, typewriter: false })
function flashLocal(k: 'place' | 'typewriter') {
  localSaved[k] = true
  setTimeout(() => (localSaved[k] = false), 1500)
}
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
    draft.value = keep
    notify(C.saveFailed, 'warn')
  }
}

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
    if (t === 'general' && !caps.loaded) void loadCaps()
  },
  { immediate: true },
)

const rechecking = ref(false)
const appVersion = ref('')

const agentRows = computed(() => {
  const checking = catState.status.state === 'checking' || rechecking.value
  return CAT_PROVIDER_ORDER.map((kind) => {
    const a = catState.status.agents?.find((x) => x.agentKind === kind)
    const ready = a?.ready ?? (kind === 'cat_build' && catState.status.state === 'ready')
    const version = a?.version || (kind === 'cat_build' ? catState.status.version : '') || ''
    const loginBad = a?.loggedIn === false
    let loginText = ''
    let loginTone: 'ok' | 'bad' | 'wait' = 'ok'
    if (a && a.loggedIn === true) {
      loginText = C.loggedIn
      loginTone = 'ok'
    } else if (loginBad) {
      loginText = C.loginBad
      loginTone = 'bad'
    } else if (a && a.loggedIn == null && ready) {
      loginText = C.loginNa
      loginTone = 'wait'
    }
    const statusText = checking ? C.checking : ready ? C.ready : C.missing
    const tone: 'ok' | 'bad' | 'wait' = checking ? 'wait' : ready ? 'ok' : 'bad'
    return {
      kind,
      label: CAT_PROVIDER_LABEL[kind],
      ready,
      version,
      statusText,
      tone,
      loginText,
      loginTone,
      loginBad,
    }
  })
})
const noneReady = computed(
  () =>
    !rechecking.value &&
    catState.status.state !== 'checking' &&
    agentRows.value.every((r) => !r.ready),
)

async function recheck() {
  rechecking.value = true
  try {
    catState.status = await recheckCat()
  } catch {
    /* 保持原状态 */
  }
  rechecking.value = false
}

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
  void loadCaps()
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
.card-tip {
  margin: 0;
  padding: 4px 14px 10px;
  font-size: 12px;
  color: var(--cat-muted);
  line-height: 1.5;
}
.agent {
  padding-bottom: 4px;
}
.agent.sep {
  border-top: 1px solid var(--cat-divider);
  margin-top: 4px;
  padding-top: 4px;
}
.agent.dim {
  opacity: 0.55;
}
.agent-hd {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px 2px;
}
.agent-hd .lab {
  flex: 1;
}
.okt {
  color: var(--cat-success-text, #1a7f4b);
}
.badt {
  color: var(--cat-muted);
}
.agent-row + .agent-row {
  border-top: 1px solid var(--cat-divider);
}
.agent-row.dim {
  opacity: 0.65;
}
.agent-row .row {
  gap: 10px;
}
.agent-row .row.sub {
  min-height: 36px;
  padding-top: 0;
  padding-bottom: 4px;
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
@media (prefers-reduced-motion: reduce) {
  .cs-tab,
  .cs-tab::after,
  .ta {
    transition: none;
  }
  .sk i,
  .sk b,
  .sk span {
    animation: none;
  }
}
</style>
