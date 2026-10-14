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
              :class="{ sep: bi > 0 }"
              :data-testid="'cat-set-agent-' + block.kind"
            >
              <div class="agent-hd" :class="{ dim: !block.ready }">
                <CatAgentMark :kind="block.kind" :size="18" />
                <div class="lab">
                  <b>{{ block.label }}</b>
                  <span :class="block.ready ? 'okt' : 'badt'">{{ block.ready ? C.installed : C.notDetected }}</span>
                </div>
              </div>
              <!-- v0.46 自定义服务商排在默认模型上面；开着时自动展开（v0.38：未检测到 CLI 时仍可展开预填） -->
              <div
                class="cust-fold"
                :class="{ open: isCustomOpen(block.kind) }"
                :data-testid="'cat-set-custom-' + block.kind"
              >
                <button
                  type="button"
                  class="cust-tog"
                  :aria-expanded="isCustomOpen(block.kind)"
                  :data-testid="'cat-set-custom-tog-' + block.kind"
                  @click="toggleCustom(block.kind)"
                >
                  <FIcon name="right" :size="14" class="chev" />
                  <span class="cust-lab">
                    <b>{{ C.customFold }}</b>
                    <small>{{ providerOf(block.kind).enabledCustom ? C.customFoldHintOn : C.customFoldHintOff }}</small>
                  </span>
                </button>
                <div v-if="isCustomOpen(block.kind)" class="cust-body">
                  <div class="row">
                    <div class="lab"><b>{{ C.enableCustom }}</b><span>{{ customToggleHint(block.kind) }}</span></div>
                    <div class="ctl">
                      <SavedMark :on="!!savedAgent[block.kind + ':custom']" />
                      <button
                        type="button"
                        role="switch"
                        class="sw"
                        :class="{ on: providerOf(block.kind).enabledCustom }"
                        :aria-checked="providerOf(block.kind).enabledCustom"
                        :aria-label="C.enableCustom"
                        :data-testid="'cat-set-custom-enable-' + block.kind"
                        @click="setProviderField(block.kind, 'enabledCustom', !providerOf(block.kind).enabledCustom)"
                      ><i /></button>
                    </div>
                  </div>
                  <div v-if="provErr[block.kind]?.field === 'enabledCustom'" class="prov-err" role="alert" :data-testid="'cat-set-custom-err-' + block.kind">{{ provErr[block.kind]?.msg }}</div>
                  <div
                    v-if="customKnownIssue(block.kind, providerOf(block.kind).enabledCustom)"
                    class="pm-known"
                    :data-testid="'cat-set-custom-known-' + block.kind"
                  >{{ customKnownIssue(block.kind, providerOf(block.kind).enabledCustom) }}</div>
                  <div class="row">
                    <div class="lab">
                      <b>{{ C.protocol }}</b>
                      <span>{{ capOf(block.kind).allowProtocolPick ? '' : C.protocolLocked }}</span>
                    </div>
                    <div class="ctl">
                      <SavedMark :on="!!savedAgent[block.kind + ':protocol']" />
                      <CatModeSelect
                        v-if="capOf(block.kind).allowProtocolPick"
                        :model-value="effectiveProtocol(block.kind)"
                        :options="capOf(block.kind).allowedProtocols.map((pr) => ({ id: pr, label: protocolLabel(pr) }))"
                        :aria-label="C.protocol"
                        :testid="'cat-set-custom-protocol-' + block.kind"
                        @update:model-value="setProviderField(block.kind, 'protocol', $event)"
                      />
                      <span v-else class="ro locked" :data-testid="'cat-set-custom-protocol-' + block.kind">{{ protocolLabel(effectiveProtocol(block.kind)) }}</span>
                    </div>
                  </div>
                  <div v-if="provErr[block.kind]?.field === 'protocol'" class="prov-err" role="alert">{{ provErr[block.kind]?.msg }}</div>
                  <div class="row">
                    <div class="lab"><b>{{ C.apiUrl }}</b></div>
                    <div class="ctl">
                      <SavedMark :on="!!savedAgent[block.kind + ':apiUrl']" />
                      <input
                        class="inp url"
                        :class="{ 'prov-bad': provErr[block.kind]?.field === 'apiUrl' }"
                        type="url"
                        :value="providerOf(block.kind).apiUrl"
                        :placeholder="protocolUrlPlaceholder(effectiveProtocol(block.kind))"
                        :aria-label="C.apiUrl"
                        :data-testid="'cat-set-custom-url-' + block.kind"
                        @input="clearProvErr(block.kind, 'apiUrl')"
                        @change="setProviderField(block.kind, 'apiUrl', ($event.target as HTMLInputElement).value.trim())"
                      />
                    </div>
                  </div>
                  <div v-if="provErr[block.kind]?.field === 'apiUrl'" class="prov-err" role="alert" :data-testid="'cat-set-custom-url-err-' + block.kind">{{ provErr[block.kind]?.msg }}</div>
                  <div class="row">
                    <div class="lab"><b>{{ C.apiKey }}</b><span>{{ C.apiKeyDesc }}</span></div>
                    <div class="ctl key-ctl">
                      <template v-if="providerOf(block.kind).apiKeySet">
                        <span class="key-saved" :data-testid="'cat-set-custom-key-saved-' + block.kind">{{ C.apiKeySaved }}</span>
                        <button
                          type="button"
                          class="btn text key-clear"
                          :data-testid="'cat-set-custom-key-clear-' + block.kind"
                          @click="askClearKey(block.kind)"
                        >{{ C.apiKeyClear }}</button>
                      </template>
                      <input
                        v-else
                        class="inp key"
                        type="password"
                        autocomplete="off"
                        :value="keyDraft[block.kind] || ''"
                        :placeholder="C.apiKeyPlaceholder"
                        :aria-label="C.apiKey"
                        :data-testid="'cat-set-custom-key-' + block.kind"
                        @input="keyDraft[block.kind] = ($event.target as HTMLInputElement).value"
                        @keydown.enter.prevent="saveKey(block.kind)"
                        @blur="saveKey(block.kind)"
                      />
                    </div>
                  </div>
                </div>
              </div>
              <div class="row" :class="{ dim: !block.ready && !customOn(block.kind) }">
                <div class="lab"><b>{{ C.model }}</b><span>{{ C.modelDesc }}</span></div>
                <div class="ctl">
                  <SavedMark :on="!!savedAgent[block.kind + ':model']" />
                  <!-- v0.46：自定义开 → 服务商列表（ListCatProviderModels）+ 刷新 + 手动填写 -->
                  <template v-if="customOn(block.kind)">
                    <template v-if="pmOf(block.kind).manual">
                      <input
                        :ref="(el) => setPmInput(block.kind, el)"
                        class="inp pm-inp"
                        type="text"
                        spellcheck="false"
                        autocomplete="off"
                        :value="pmOf(block.kind).draft"
                        :placeholder="PM.manualPlaceholder"
                        :aria-label="block.label + ' ' + C.model"
                        :data-testid="'cat-set-model-manual-' + block.kind"
                        @input="pmOf(block.kind).draft = ($event.target as HTMLInputElement).value"
                        @keydown.enter.prevent="commitManual(block.kind)"
                        @keydown.esc.prevent="cancelManual(block.kind)"
                        @blur="commitManual(block.kind)"
                      />
                      <button
                        v-if="pmHasList(block.kind)"
                        type="button"
                        class="btn text pm-cancel"
                        :data-testid="'cat-set-model-manual-cancel-' + block.kind"
                        @mousedown.prevent
                        @click="cancelManual(block.kind)"
                      >{{ PM.cancel }}</button>
                    </template>
                    <CatModeSelect
                      v-else
                      :model-value="pmOf(block.kind).loading ? '__loading' : modelValueCustom(block.kind)"
                      :options="pmOf(block.kind).loading ? [{ id: '__loading', label: PM.loading, disabled: true }] : pmSelectOptions(block.kind)"
                      :busy="pmOf(block.kind).loading"
                      :aria-label="block.label + ' ' + C.model"
                      :testid="'cat-set-model-' + block.kind"
                      @update:model-value="onPmSelect(block.kind, $event)"
                    />
                    <button
                      type="button"
                      class="pm-refresh"
                      :class="{ spin: pmOf(block.kind).loading }"
                      :disabled="pmOf(block.kind).loading"
                      :title="PM.refreshTip"
                      :aria-label="PM.refreshTip"
                      :data-testid="'cat-set-model-refresh-' + block.kind"
                      @click="loadPm(block.kind, true)"
                    ><FIcon name="refresh" :size="15" /></button>
                  </template>
                  <CatModeSelect
                    v-else
                    :model-value="modelValue(block.kind)"
                    :options="[{ id: '', label: C.defaultOpt }, ...block.models.map((m) => ({ id: m.id, label: m.displayName }))]"
                    :disabled="!block.ready || !block.models.length"
                    :aria-label="block.label + ' ' + C.model"
                    :testid="'cat-set-model-' + block.kind"
                    @update:model-value="setAgent(block.kind, 'model', $event)"
                  />
                </div>
              </div>
              <div v-if="customOn(block.kind) && pmLine(block.kind)" :class="['pm-err', pmTone(block.kind)]" role="status" :data-testid="'cat-set-model-hint-' + block.kind">{{ pmLine(block.kind) }}</div>
              <div v-if="block.ready && block.thinks.length" class="row">
                <div class="lab"><b>{{ C.effort }}</b><span>{{ C.effortDesc }}</span></div>
                <div class="ctl">
                  <SavedMark :on="!!savedAgent[block.kind + ':effort']" />
                  <CatModeSelect
                    :model-value="effortValue(block.kind)"
                    :options="[{ id: '', label: C.defaultOpt }, ...block.thinks.map((t) => ({ id: t.id, label: t.label }))]"
                    :disabled="!block.ready"
                    :aria-label="block.label + ' ' + C.effort"
                    :testid="'cat-set-effort-' + block.kind"
                    @update:model-value="setAgent(block.kind, 'effort', $event)"
                  />
                </div>
              </div>

              <div v-if="block.modes.options.length" class="row mode-row" :class="{ dim: !block.ready }">
                <div class="lab"><b>默认模式</b><span>新对话默认使用的模式</span></div>
                <div class="ctl">
                  <SavedMark :on="!!savedAgent[block.kind + ':mode']" />
                  <CatModeSelect
                    :model-value="modeValue(block.kind)"
                    :options="block.modes.options"
                    :default-id="block.modes.defaultId"
                    :aria-label="block.label + ' 默认模式'"
                    :testid="'cat-set-mode-' + block.kind"
                    @update:model-value="setDefaultMode(block.kind, $event)"
                  />
                </div>
              </div>
              <div v-if="modeIsDanger(block.kind)" class="mode-danger-note" :data-testid="'cat-set-mode-danger-' + block.kind">{{ DANGER_NOTE }}</div>

            </div>

            <div class="row">
              <div class="lab"><b>{{ C.place }}</b><span>{{ C.placeDesc }}</span></div>
              <div class="ctl">
                <SavedMark :on="localSaved.place" />
                <CatModeSelect
                  :model-value="placeValue"
                  :options="[{ id: 'last', label: C.placeLast }, { id: 'none', label: C.placeNone }, ...catState.projects.map((p) => ({ id: p.id, label: p.name, disabled: p.missing }))]"
                  :aria-label="C.place"
                  testid="cat-set-place"
                  @update:model-value="onPlace($event)"
                />
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

        <!-- 技能与扩展（v0.39 可管理） -->
        <div v-else-if="catPanels.tab === 'ext'" key="e" class="cs-body" data-testid="cat-ext">
          <template v-if="!extProject">
            <div class="card pad ext-empty" data-testid="cat-ext-no-project">
              <p>{{ C.noProjectExt }}</p>
              <button type="button" class="btn pri" data-testid="cat-ext-pick-project" @click="pickExtProject">{{ C.pickProject }}</button>
            </div>
          </template>
          <template v-else>
            <div class="note tip-row" data-testid="cat-ext-tip">
              <FIcon name="info" :size="14" />
              <p>
                {{ C.extTipBefore }}<strong>{{ C.extTipProject }}</strong>{{ C.extTipMid }}<code>{{ extPathLabel }}</code>{{ C.extTipAfter }}
              </p>
              <button type="button" class="btn" data-testid="cat-ext-open-dir" :disabled="openingDir" @click="openSkillsDir">
                <FIcon name="folder" :size="14" /><i v-if="openingDir" class="spin" aria-hidden="true" />{{ C.openDir }}
              </button>
            </div>
            <div v-if="inspect.loading" class="card" aria-busy="true" data-testid="cat-ext-loading">
              <div v-for="i in 3" :key="i" class="sk"><i /><b /><span /></div>
            </div>
            <div v-else-if="inspectFail" class="card pad fail" data-testid="cat-ext-failed">
              <span>{{ C.extFailed }}</span>
              <button type="button" class="btn" @click="loadInspect">{{ C.retry }}</button>
            </div>
            <template v-else-if="inspect.result">
              <div v-for="g in groups" :key="g.key" class="card" :data-testid="'cat-ext-card-' + g.key">
                <div class="card-h">{{ g.title }} · {{ g.items.length }}</div>
                <div v-if="!g.items.length" class="empty">
                  {{ C.extEmpty }}
                  <button type="button" class="btn" data-testid="cat-ext-empty-open" @click="openSkillsDir">{{ C.openDir }}</button>
                </div>
                <div v-for="it in g.items" :key="g.key + it.name" class="row it" :data-testid="'cat-ext-item-' + g.key + '-' + it.name">
                  <FIcon :name="(g.icon as any)" :size="15" class="it-ic" />
                  <div class="lab">
                    <b>{{ it.name }}<em v-if="it.tag" class="tag">{{ it.tag }}</em></b>
                    <span v-if="it.description">{{ it.description }}</span>
                  </div>
                  <div class="ctl">
                    <button
                      type="button"
                      role="switch"
                      class="sw"
                      :class="{ on: it.enabled }"
                      :aria-checked="it.enabled"
                      :aria-label="it.enabled ? C.on : C.off"
                      :disabled="!!toggling[g.key + ':' + it.name]"
                      :data-testid="'cat-ext-sw-' + g.key + '-' + it.name"
                      @click="toggleItem(g, it)"
                    ><i /></button>
                  </div>
                </div>
              </div>
            </template>
          </template>
        </div>

        <!-- 关于与状态（按 agents[] 拆行） -->
        <div v-else key="a" class="cs-body">
          <div class="card">
            <div class="card-h">{{ C.statusCard }}</div>
            <el-tooltip
              v-for="row in agentRows"
              :key="row.kind"
              :disabled="!row.pathTip"
              effect="light"
              placement="top-start"
              :show-after="400"
              :hide-after="0"
              popper-class="cat-path-tip"
            >
              <template #content><span class="cat-path-tip-text">{{ row.pathTip }}</span></template>
              <div
                class="agent-row"
                :class="{ dim: !row.ready && !row.checking && !row.pending }"
                :data-testid="'cat-status-' + row.kind"
              >
                <div class="row">
                  <CatAgentMark :kind="row.kind" :size="16" />
                  <div class="lab"><b>{{ row.label }}</b></div>
                  <div class="ctl detect-meta" :data-testid="'cat-detect-meta-' + row.kind">
                    <span>{{ row.meta }}</span>
                    <button
                      v-if="row.showRecheck"
                      type="button"
                      class="btn text detect-recheck"
                      :data-testid="'cat-detect-recheck-' + row.kind"
                      @click="recheckRow(row.kind)"
                    >{{ D.recheck }}</button>
                  </div>
                </div>
                <div v-if="row.version && !row.pending" class="row sub"><div class="lab"><b>{{ C.version }}</b></div><div class="ctl ro">{{ row.version }}</div></div>
              </div>
            </el-tooltip>
            <div class="row end">
              <button type="button" class="btn" :disabled="recheckDisabled" data-testid="cat-set-recheck" @click="recheck">
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
    <CatConfirmDialog
      :open="!!clearKeyKind"
      :title="C.clearKeyTitle"
      :body="C.clearKeyBody"
      :ok-text="C.clearKeyOk"
      :cancel-text="C.cancel"
      tone="danger"
      :busy="clearingKey"
      @cancel="clearKeyKind = null"
      @ok="doClearKey"
    />
    <CatConfirmDialog
      :open="skillsDirOpen"
      :title="C.noSkillsDir"
      :body="C.noSkillsDirBody"
      :ok-text="C.createAndOpen"
      :cancel-text="C.cancel"
      tone="warn"
      :busy="creatingDir"
      @cancel="skillsDirOpen = false"
      @ok="doCreateSkillsDir"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, nextTick, onMounted, reactive, ref, watch } from 'vue'
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
import { FALLBACK_MODES, getCatAgentCapabilities, mapAgentModes, mapBackendCaps, resolveCapabilities, resolveModeId, setBackendCaps } from '@/views/cat/catCapabilities'
import CatModeSelect, { type CatSelectOption } from './CatModeSelect.vue'
import { CAT_PROVIDER_MODELS_COPY as PM_COPY_ALL } from '@/api/catProviderModels'
const PM_COPY_DESC = PM_COPY_ALL.followDefaultDesc
import { CAT_DEFAULT_DANGER_NOTE as DANGER_NOTE, emptyModeByAgent } from '@/api/catV0445'
import {
  type CatInspectCategory,
  type CatInspectItem,
  type CatInspectResult,
  type CatProviderCapability,
  type CatProviderProtocol,
  type CatProviderSettings,
  type CatSettings,
  CAT_INSTRUCTION_MAX as MAX,
  CAT_SETTINGS_COPY,
  clearAllCatConversations,
  clearCatProviderKey,
  emptyDefaults,
  emptyProvider,
  ensureCreateCatSkillsDir,
  inspectCatConfig,
  listCatProviderCapabilities,
  protocolLabel,
  protocolUrlPlaceholder,
  revealCatSkillsDir,
  saveCatProviderKey,
  setCatInspectItemEnabled,
  skillsPathLabel,
} from '@/api/catSettings'
import { getAppVersion } from '@/api/about'
import { catPanels, saveSetting, takeSaveError, type CatSettingsTab } from '@/views/cat/catPanels'
import { catLocalPrefs, setNewConvPlace, setTypewriter } from '@/views/cat/catLocalPrefs'
import { catState, afterClearAll, currentProject, notify, createProject, applyCatStatus, recheckCatAgents } from '@/views/cat/catState'
import { CAT_DETECT_COPY, allDetectRowsChecking, detectRowView } from '@/api/catDetect'
import { catSim, simAgentRaw, simModels, simThinks } from '@/views/cat/catDevSim'
import {
  CAT_PROVIDER_MODELS_COPY as PM,
  customToggleHint,
  customKnownIssue,
  listCatProviderModels,
  normalizeManualModelId,
  providerModelHint,
  providerModelHintTone,
  providerModelNeedsManual,
  providerModelOptions,
  shouldFocusManual,
  type CatProviderModelList,
  type ProviderModelFetchCause,
} from '@/api/catProviderModels'

defineOptions({ name: 'CatSettingsView' })
const C = { ...CAT_SETTINGS_COPY, dash: '—' }
const TABS: CatSettingsTab[] = ['general', 'instruction', 'ext', 'about']

const SavedMark = defineComponent({
  props: { on: Boolean },
  setup: (p) => () => h('span', { class: ['saved', { on: p.on }], 'aria-live': 'polite' }, p.on ? C.saved : ''),
})

type TopKey = 'defaultModeByAgent' | 'customInstruction' | 'notifyOnFinish' | 'defaultAgentKind' | 'defaultsByAgent'
const saved = reactive<Record<TopKey, boolean>>({
  defaultModeByAgent: false, customInstruction: false, notifyOnFinish: false, defaultAgentKind: false, defaultsByAgent: false,
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
  else notify(takeSaveError() || C.saveFailed, 'warn')
}

async function setAgent(kind: CatProviderKind, field: 'model' | 'effort', value: string) {
  const prev = { ...catPanels.settings.defaultsByAgent }
  const cur = { ...(prev[kind] ?? emptyDefaults()), [field]: value }
  const next = { ...prev, [kind]: cur }
  const ok = await saveSetting('defaultsByAgent', next)
  if (ok) flashAgent(`${kind}:${field}`)
  else notify(takeSaveError() || C.saveFailed, 'warn')
}

const customOpen = reactive<Record<string, boolean>>({})
// 服务商区行内错误（字段下方红字，不弹提示条）；用户一改该字段就清掉
const provErr = reactive<Record<string, { field: 'enabledCustom' | 'protocol' | 'apiUrl'; msg: string }>>({})
function clearProvErr(kind: string, field: 'enabledCustom' | 'protocol' | 'apiUrl') {
  if (provErr[kind]?.field === field) delete provErr[kind]
}
const keyDraft = reactive<Record<string, string>>({})
const providerCaps = ref<CatProviderCapability[]>([])
const clearKeyKind = ref<CatProviderKind | null>(null)
const clearingKey = ref(false)

function toggleCustom(kind: CatProviderKind) {
  customOpen[kind] = !isCustomOpen(kind)
}
/** v0.46：开着时默认展开，关着时默认折叠；手动点过就按手动的 */
function isCustomOpen(kind: CatProviderKind): boolean {
  return customOpen[kind] ?? providerOf(kind).enabledCustom
}
function customOn(kind: CatProviderKind): boolean {
  return providerOf(kind).enabledCustom
}

// ---------- v0.46 服务商模型列表 ----------
type PmState = { loading: boolean; list: CatProviderModelList | null; manual: boolean; draft: string; ticket: number }
const pm = reactive<Record<string, PmState>>({})
const pmInputs: Record<string, HTMLInputElement | null> = {}
function pmOf(kind: CatProviderKind): PmState {
  if (!pm[kind]) pm[kind] = { loading: false, list: null, manual: false, draft: '', ticket: 0 }
  return pm[kind]
}
function setPmInput(kind: CatProviderKind, el: unknown) {
  pmInputs[kind] = el instanceof HTMLInputElement ? el : null
}
function pmHasList(kind: CatProviderKind): boolean {
  const l = pmOf(kind).list
  return !!l && l.state === 'ok' && l.models.length > 0
}
function pmHint(kind: CatProviderKind): string {
  const p = pmOf(kind)
  return p.loading || !p.list ? '' : providerModelHint(p.list)
}
function pmTone(kind: CatProviderKind): string {
  const p = pmOf(kind)
  if (p.loading) return ''
  const t = p.list ? providerModelHintTone(p.list) : ''
  return t || 'muted'
}
/** 下拉下面一行：有状态提示就显示提示；否则选的是「默认模型」时灰字说明 */
function pmLine(kind: CatProviderKind): string {
  const p = pmOf(kind)
  if (p.loading) return ''
  return pmHint(kind) || (!p.manual && !curModel(kind) ? PM.followDefaultDesc : '')
}
function curModel(kind: CatProviderKind): string {
  return catPanels.settings.defaultsByAgent[kind]?.model ?? ''
}
function customNotInList(kind: CatProviderKind): boolean {
  const id = curModel(kind)
  return !!id && !(pmOf(kind).list?.models ?? []).some((m) => m.id === id)
}
function modelValueCustom(kind: CatProviderKind): string {
  return curModel(kind)
}
async function loadPm(kind: CatProviderKind, refresh: boolean, cause: ProviderModelFetchCause = 'refresh') {
  const p = pmOf(kind)
  const t = ++p.ticket
  p.loading = true
  const r = await listCatProviderModels(kind, refresh)
  if (t !== p.ticket) return
  p.loading = false
  p.list = r
  if (providerModelNeedsManual(r)) {
    // 拉不到 → 输入框自动展开；只有用户触发的拉取才抢焦点
    p.manual = true
    p.draft = curModel(kind)
    if (shouldFocusManual(cause, r)) await nextTick().then(() => pmInputs[kind]?.focus())
  } else p.manual = false // unauthorized / unreachable / not_configured / 有列表：保留下拉，靠刷新恢复
}
function pmSelectOptions(kind: CatProviderKind): CatSelectOption[] {
  const out: CatSelectOption[] = []
  let divide = false
  for (const o of providerModelOptions(pmOf(kind).list, curModel(kind))) {
    if (o.value === '__sep') { divide = true; continue }
    out.push({
      id: o.value,
      label: o.label,
      disabled: o.disabled,
      desc: o.value === '' ? PM_COPY_DESC : undefined,
      divided: divide,
    })
    divide = false
  }
  return out
}
async function onPmSelect(kind: CatProviderKind, v: string) {
  if (v === '__manual') {
    const p = pmOf(kind)
    p.manual = true
    p.draft = customNotInList(kind) ? curModel(kind) : ''
    await nextTick()
    pmInputs[kind]?.focus()
    return
  }
  if (v === '__sep' || v === '__loading') return
  await setAgent(kind, 'model', v)
}
async function commitManual(kind: CatProviderKind) {
  const p = pmOf(kind)
  if (!p.manual) return
  const raw = p.draft
  if (!raw.trim()) return // 空：不保存，保持输入框（失焦也不动）
  const id = normalizeManualModelId(raw)
  if (!id) {
    notify(PM.manualInvalid, 'warn')
    return
  }
  if (pmHasList(kind)) p.manual = false
  if (id !== curModel(kind)) {
    // 不在列表里的模型只接受空强度（6.19.20.2 ⑧）
    const inList = (p.list?.models ?? []).some((m) => m.id === id)
    if (!inList && catPanels.settings.defaultsByAgent[kind]?.effort) await setAgent(kind, 'effort', '')
    await setAgent(kind, 'model', id)
  }
}
function cancelManual(kind: CatProviderKind) {
  const p = pmOf(kind)
  if (!pmHasList(kind)) return
  p.manual = false
  p.draft = ''
}

function providerOf(kind: CatProviderKind): CatProviderSettings {
  return catPanels.settings.providersByAgent[kind] ?? emptyProvider()
}

function capOf(kind: CatProviderKind): CatProviderCapability {
  return (
    providerCaps.value.find((c) => c.agentKind === kind) ?? {
      agentKind: kind,
      defaultProtocol: kind === 'claude_cli' ? 'anthropic' : 'openai',
      allowProtocolPick: false,
      allowedProtocols: [kind === 'claude_cli' ? 'anthropic' : 'openai'],
    }
  )
}

function effectiveProtocol(kind: CatProviderKind): CatProviderProtocol {
  const p = providerOf(kind).protocol
  if (p === 'openai' || p === 'anthropic') return p
  return capOf(kind).defaultProtocol
}

async function setProviderField(
  kind: CatProviderKind,
  field: 'enabledCustom' | 'protocol' | 'apiUrl',
  value: boolean | string,
) {
  const prev = { ...catPanels.settings.providersByAgent }
  const cur = { ...(prev[kind] ?? emptyProvider()) }
  const before = { ...cur }
  delete provErr[kind]
  // 打开自定义前必须有地址：地址框下红字、框变红，开关保持关
  if (field === 'enabledCustom' && value && !(cur.apiUrl || '').trim()) {
    provErr[kind] = { field: 'apiUrl', msg: C.apiUrlRequired }
    return
  }
  if (field === 'enabledCustom') cur.enabledCustom = !!value
  else if (field === 'protocol') cur.protocol = (value === 'openai' || value === 'anthropic' ? value : '') as CatProviderProtocol | ''
  else cur.apiUrl = String(value || '')
  // 保留已有 apiKeySet；Save 不会清 Key
  const next = { ...prev, [kind]: cur }
  const ok = await saveSetting('providersByAgent', next)
  if (ok) flashAgent(`${kind}:${field === 'enabledCustom' ? 'custom' : field}`)
  else {
    // 不清掉用户填的地址：只把失败的开关退回原值，其余输入保留在表单里
    const keep = { ...cur, enabledCustom: field === 'enabledCustom' ? before.enabledCustom : cur.enabledCustom }
    catPanels.settings.providersByAgent = { ...catPanels.settings.providersByAgent, [kind]: keep }
    const m = takeSaveError() || C.saveFailed
    const errField = m === C.apiUrlRequired ? 'apiUrl' : field
    provErr[kind] = { field: errField, msg: m }
  }
  // v0.46：打开开关 / 改地址或协议后自动重拉（用户触发，可抢焦点）
  if (ok && cur.enabledCustom) void loadPm(kind, true, field === 'enabledCustom' ? 'toggle' : 'edit')
}

async function saveKey(kind: CatProviderKind) {
  const draft = (keyDraft[kind] || '').trim()
  if (!draft) return
  try {
    const view = await saveCatProviderKey(kind, draft)
    keyDraft[kind] = ''
    const prev = { ...catPanels.settings.providersByAgent }
    catPanels.settings.providersByAgent = { ...prev, [kind]: { ...(prev[kind] ?? emptyProvider()), ...view, apiKeySet: true } }
    if (customOn(kind)) void loadPm(kind, true, 'edit')
  } catch {
    notify(takeSaveError() || C.saveFailed, 'warn')
  }
}

function askClearKey(kind: CatProviderKind) {
  clearKeyKind.value = kind
}

async function doClearKey() {
  const kind = clearKeyKind.value
  if (!kind) return
  clearingKey.value = true
  try {
    const view = await clearCatProviderKey(kind)
    const prev = { ...catPanels.settings.providersByAgent }
    // Clear 不关 enabledCustom、不清 apiUrl
    catPanels.settings.providersByAgent = {
      ...prev,
      [kind]: {
        ...(prev[kind] ?? emptyProvider()),
        ...view,
        enabledCustom: (prev[kind] ?? emptyProvider()).enabledCustom,
        apiUrl: (prev[kind] ?? emptyProvider()).apiUrl,
        apiKeySet: false,
      },
    }
    clearKeyKind.value = null
    if (customOn(kind)) void loadPm(kind, true, 'edit')
  } catch {
    clearKeyKind.value = null
    notify(takeSaveError() || C.saveFailed, 'warn')
  } finally {
    clearingKey.value = false
  }
}

const caps = reactive({
  models: {} as Record<CatProviderKind, CatModel[]>,
  thinks: {} as Record<CatProviderKind, CatThinkLevel[]>,
  loaded: false,
})

async function loadCaps() {
  const useSim = import.meta.env.DEV && catSim.has('caps')
  const [capList] = await Promise.all([
    listCatProviderCapabilities().catch(() => []),
    Promise.all(
      CAT_PROVIDER_ORDER.map(async (kind) => {
        if (useSim) {
          // 走查：三家都给模型列表；Codex 故意空强度。未检测到时模板还会因 !ready 整行隐藏强度。
          caps.models[kind] = simModels().map((m) => ({ ...m, id: `${kind}:${m.id}`, displayName: m.displayName }))
          caps.thinks[kind] = kind === 'codex_cli' ? [] : simThinks()
          setBackendCaps(kind, { thinks: caps.thinks[kind], modes: mapBackendCaps(simAgentRaw(kind)).modes })
        } else {
          const [models, thinks] = await Promise.all([listCatModels(kind), listCatThinkLevels(kind)])
          caps.models[kind] = models
          caps.thinks[kind] = thinks
          const st = catState.status.agents?.find((a) => a.agentKind === kind)
          setBackendCaps(kind, { thinks, modes: mapAgentModes({ modes: st?.rawModes }) })
        }
      }),
    ),
  ])
  if (capList.length) providerCaps.value = capList
  caps.loaded = true
  // v0.46：进入设置页，自定义开着的家自动拉一次（不抢焦点）
  for (const kind of CAT_PROVIDER_ORDER) if (customOn(kind)) void loadPm(kind, true, 'enter')
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
    thinks: resolveCapabilities({ thinks: caps.thinks[kind] }).efforts.options,
    modes: useModes(kind),
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

/** 设置页显示值：按该家 modes[] 解析（旧单值按「旧值」列映射；认不出 → 该家最保守默认） */
const useModes = (kind: CatProviderKind) => getCatAgentCapabilities(kind).modes

function modeValue(kind: CatProviderKind): string {
  const caps = getCatAgentCapabilities(kind)
  const want = catPanels.settings.defaultModeByAgent?.[kind] ?? ''
  if (caps.modes === FALLBACK_MODES) return caps.modes.options.some((o) => o.id === want) ? want : caps.modes.defaultId
  return resolveModeId(kind, want)
}
function modeIsDanger(kind: CatProviderKind): boolean {
  const id = modeValue(kind)
  return !!getCatAgentCapabilities(kind).modes.options.find((o) => o.id === id)?.risky
}
/** 设置页选 danger 不弹确认（6.19.18.9 第 5 条） */
async function setDefaultMode(kind: CatProviderKind, id: string) {
  const next = { ...emptyModeByAgent(), ...catPanels.settings.defaultModeByAgent }
  // 把另两家「旧值」一起落成解析后的 id，Save 写新形状
  for (const k of CAT_PROVIDER_ORDER) next[k] = k === kind ? id : (next[k] ? modeValue(k) : '')
  const ok = await saveSetting('defaultModeByAgent', next)
  if (ok) flashAgent(kind + ':mode')
  else notify(takeSaveError() || C.saveFailed, 'warn')
}

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
    notify(takeSaveError() || C.saveFailed, 'warn')
  }
}

const inspect = reactive({ loading: false, result: null as CatInspectResult | null })
const toggling = reactive<Record<string, boolean>>({})
const openingDir = ref(false)
const skillsDirOpen = ref(false)
const creatingDir = ref(false)

const extProject = computed(() => currentProject.value)
const inspectFail = computed(() => {
  const r = inspect.result?.reason
  return !!r && r !== 'no_project'
})
const extPathLabel = computed(() =>
  skillsPathLabel(inspect.result?.skillsRoot, extProject.value?.name || ''),
)

async function loadInspect() {
  const proj = currentProject.value
  if (!proj) {
    inspect.loading = false
    inspect.result = {
      skills: [],
      plugins: [],
      hooks: [],
      mcpServers: [],
      agents: [],
      reason: 'no_project',
      skillsRoot: '',
      mcpConfigPath: '',
      skillsRootExists: false,
    }
    return
  }
  inspect.loading = true
  const agentKind = findAgentKind()
  inspect.result = await inspectCatConfig(proj.id, agentKind)
  inspect.loading = false
}

function findAgentKind(): string {
  const sel = catState.sel
  for (const p of catState.projects) {
    const c = p.convs.find((x) => x.id === sel)
    if (c?.agentKind) return c.agentKind
  }
  const plain = catState.plain.find((x) => x.id === sel)
  if (plain?.agentKind) return plain.agentKind
  return catPanels.settings.defaultAgentKind || 'cat_build'
}

type ExtGroup = {
  key: string
  title: string
  icon: string
  category: CatInspectCategory
  items: Array<CatInspectItem & { tag: string; category: CatInspectCategory }>
}

const groups = computed((): ExtGroup[] => {
  const r = inspect.result
  if (!r) return []
  return [
    {
      key: 's',
      title: C.skills,
      icon: 'spark',
      category: 'skill',
      items: r.skills.map((x) => ({ ...x, tag: '', category: 'skill' as const })),
    },
    {
      key: 'x',
      title: C.extsFull,
      icon: 'magic',
      category: 'mcp',
      items: [
        ...r.mcpServers.map((x) => ({ ...x, tag: 'MCP', category: 'mcp' as const })),
        ...r.plugins.map((x) => ({ ...x, tag: '插件', category: 'plugin' as const })),
      ],
    },
    {
      key: 'h',
      title: C.hooks,
      icon: 'link',
      category: 'hook',
      items: r.hooks.map((x) => ({ ...x, tag: '', category: 'hook' as const })),
    },
  ]
})

async function toggleItem(g: ExtGroup, it: ExtGroup['items'][number]) {
  const proj = currentProject.value
  if (!proj) return
  const key = g.key + ':' + it.name
  const prev = it.enabled
  const next = !prev
  it.enabled = next
  // 同步到底层 result，便于失败回滚
  const list =
    it.category === 'skill'
      ? inspect.result?.skills
      : it.category === 'hook'
        ? inspect.result?.hooks
        : it.category === 'plugin'
          ? inspect.result?.plugins
          : inspect.result?.mcpServers
  const raw = list?.find((x) => x.name === it.name)
  if (raw) raw.enabled = next
  toggling[key] = true
  try {
    await setCatInspectItemEnabled({
      projectId: proj.id,
      agentKind: findAgentKind(),
      category: it.category,
      name: it.name,
      enabled: next,
    })
  } catch {
    it.enabled = prev
    if (raw) raw.enabled = prev
    notify(takeSaveError() || C.saveFailed, 'warn')
  } finally {
    toggling[key] = false
  }
}

async function openSkillsDir() {
  const proj = currentProject.value
  if (!proj || openingDir.value) return
  openingDir.value = true
  try {
    const r = await revealCatSkillsDir(proj.id)
    if (inspect.result) {
      inspect.result.skillsRoot = r.skillsRoot || inspect.result.skillsRoot
      inspect.result.mcpConfigPath = r.mcpConfigPath || inspect.result.mcpConfigPath
      inspect.result.skillsRootExists = r.skillsRootExists
    }
    if (!r.skillsRootExists) skillsDirOpen.value = true
  } catch {
    notify(takeSaveError() || C.saveFailed, 'warn')
  } finally {
    openingDir.value = false
  }
}

async function doCreateSkillsDir() {
  const proj = currentProject.value
  if (!proj) return
  creatingDir.value = true
  try {
    const r = await ensureCreateCatSkillsDir(proj.id)
    if (inspect.result) {
      inspect.result.skillsRoot = r.skillsRoot || inspect.result.skillsRoot
      inspect.result.mcpConfigPath = r.mcpConfigPath || inspect.result.mcpConfigPath
      inspect.result.skillsRootExists = true
    }
    skillsDirOpen.value = false
  } catch {
    skillsDirOpen.value = false
    notify(takeSaveError() || C.saveFailed, 'warn')
  } finally {
    creatingDir.value = false
  }
}

function pickExtProject() {
  catPanels.view = 'chat'
  void createProject()
}

watch(
  () => catPanels.tab,
  (t) => {
    if (t === 'ext') void loadInspect()
    if (t === 'general' && !caps.loaded) void loadCaps()
  },
  { immediate: true },
)
watch(
  () => currentProject.value?.id,
  () => {
    if (catPanels.tab === 'ext') void loadInspect()
  },
)

const rechecking = ref(false)
const appVersion = ref('')

const D = CAT_DETECT_COPY
const agentRows = computed(() => {
  return CAT_PROVIDER_ORDER.map((kind) => {
    const a = catState.status.agents?.find((x) => x.agentKind === kind)
    const v = detectRowView(a, { forceChecking: rechecking.value })
    const ready = v.ready
    const version = a?.version || ''
    return { kind, label: CAT_PROVIDER_LABEL[kind], ...v, version }
  })
})
/** 全部行都在检测中（含首次未到 / 全量重检）时底部「重新检测」置灰 */
const recheckDisabled = computed(
  () => rechecking.value || allDetectRowsChecking(CAT_PROVIDER_ORDER, catState.status.agents, { forceChecking: rechecking.value }),
)
/** 黄条只在三家都检测完且确实都没找到时出现（检测中 / 结果未到不出） */
const noneReady = computed(() => !rechecking.value && agentRows.value.every((r) => r.failed))

async function recheck() {
  rechecking.value = true
  await recheckCatAgents()
  rechecking.value = false
}
function recheckRow(kind: CatProviderKind) {
  void recheckCatAgents(kind)
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
      applyCatStatus(await getCatStatus())
    } catch {
      /* 保持 */
    }
  }
  void loadCaps()
})
</script>

<style scoped src="./catPanel.css"></style>
<style scoped>
/* v0.46 服务商模型列表 */
.pm-refresh {
  width: 28px;
  height: 28px;
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--cat-border);
  border-radius: var(--cat-radius-md);
  background: var(--cat-bg);
  color: var(--cat-text-2);
  cursor: pointer;
  transition: border-color 120ms var(--ff-ease), background 120ms var(--ff-ease);
}
.pm-refresh:hover:not(:disabled) {
  border-color: var(--cat-border-strong);
  background: var(--cat-hover);
}
.pm-refresh:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.pm-refresh:disabled {
  cursor: default;
  opacity: 0.7;
}
.pm-refresh.spin :deep(svg) {
  animation: pm-spin 0.9s linear infinite;
}
@keyframes pm-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .pm-refresh.spin :deep(svg) {
    animation-duration: 2.4s;
  }
}
.pm-inp {
  width: 180px;
}
.pm-err {
  margin: -4px 0 6px;
  text-align: right;
  font-size: 12px;
  line-height: 1.5;
}
.pm-known {
  margin: -4px 0 6px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--cat-muted);
}
.pm-err.danger {
  color: var(--ff-danger);
}
.pm-err.muted {
  color: var(--cat-muted);
}
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
.agent-hd {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px 2px;
}
.agent-hd.dim,
.row.dim {
  opacity: 0.55;
}
/* v0.44.4：默认模式选「全部放开」时常驻的橙色小字 */
.mode-danger-note {
  margin: -4px 14px 8px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--cat-orange);
}
.agent-hd .lab {
  flex: 1;
}
.cust-fold {
  margin: 4px 14px 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--cat-divider);
}
.cust-tog {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  width: 100%;
  text-align: left;
  padding: 6px 0;
  border: 0;
  background: none;
  color: var(--cat-text-2);
  cursor: pointer;
  border-radius: var(--cat-radius-sm);
}
.cust-tog:hover {
  color: var(--cat-text);
}
.cust-tog:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.cust-tog .chev {
  margin-top: 3px;
  color: var(--cat-muted);
  transition: transform 200ms var(--ff-ease);
  flex: none;
}
.cust-fold.open .cust-tog .chev {
  transform: rotate(90deg);
}
.cust-lab {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.cust-lab b {
  font-weight: 600;
  color: var(--cat-text);
  font-size: 13px;
}
.cust-lab small {
  font-size: 12px;
  color: var(--cat-muted);
  line-height: 1.4;
}
.cust-body {
  padding: 4px 0 2px 22px;
}
.cust-body .row {
  padding-left: 0;
  padding-right: 0;
}
.inp {
  height: 32px;
  min-width: 180px;
  max-width: 280px;
  padding: 0 10px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-bg);
  color: var(--cat-text);
  font-size: 12.5px;
  font-family: var(--ff-font-mono, ui-monospace, monospace);
  transition: border-color 120ms var(--ff-ease), box-shadow 120ms var(--ff-ease);
}
.inp:focus {
  outline: none;
  border-color: var(--cat-accent);
  box-shadow: var(--cat-focus);
}
.inp.url {
  width: min(280px, 100%);
}
.inp.key {
  width: min(220px, 100%);
}
.ro.locked {
  font-size: 12.5px;
  color: var(--cat-text-2);
  background: var(--cat-sunken);
  border: 1px solid var(--cat-divider);
  border-radius: var(--cat-radius-sm);
  padding: 4px 10px;
}
.key-saved {
  font-size: 12.5px;
  color: var(--cat-text-2);
}
.key-ctl {
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}
.cust-body .btn.key-clear {
  border: none;
  background: none;
  color: var(--cat-text-2);
  height: 28px;
  padding: 0 4px;
  font-size: 12.5px;
}
.cust-body .btn.key-clear:hover:not(:disabled) {
  color: var(--cat-text);
}
@media (prefers-reduced-motion: reduce) {
  .cust-tog .chev {
    transition: none;
  }
}
.okt {
  color: var(--cat-success-text, #1a7f4b);
}
.badt {
  color: var(--cat-muted);
}
.detect-meta {
  color: var(--cat-muted);
  font-size: 12.5px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-left: auto;
}
.btn.detect-recheck {
  border: none;
  background: none;
  height: auto;
  padding: 0 2px;
  font-size: 12px;
  color: var(--ff-primary-text);
}
.btn.detect-recheck:hover {
  text-decoration: underline;
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
.tip-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}
.tip-row > :deep(svg) {
  flex: none;
  margin-top: 2px;
  color: var(--cat-muted);
}
.tip-row p {
  flex: 1;
  margin: 0;
  font-size: 12.5px;
  line-height: 1.55;
  color: var(--cat-text-2);
}
.tip-row code {
  font-family: var(--ff-font-mono, ui-monospace, monospace);
  font-size: 12px;
  color: var(--cat-text);
}
.tip-row .btn {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.ext-empty {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 12px;
  font-size: 13px;
  color: var(--cat-text-2);
}
.ext-empty p {
  margin: 0;
}
.empty {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
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
.prov-err {
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--ff-danger);
}
.inp.prov-bad {
  border-color: var(--ff-danger) !important;
}
</style>

<style>
/* v0.47.1 路径 tooltip：elevated 气泡，等宽、可换行、不截断 */
.el-popper.cat-path-tip {
  max-width: 420px;
}
.cat-path-tip-text {
  font-family: var(--ff-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  white-space: pre-wrap;
  word-break: break-all;
  overflow-wrap: anywhere;
}
</style>
