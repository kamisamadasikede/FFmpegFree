<template>
  <div ref="root" class="ct-comp" :class="{ bare: welcome, missing }">
    <div v-if="!welcome" class="ctx-bar" aria-label="项目上下文">
      <span class="cx cx-fwrap" :class="{ miss: missing }">
        <button
          type="button"
          class="cx-folder"
          :aria-label="BC.openFolder + '：' + (folderFull || folderLabel)"
          :aria-describedby="folderFull ? tipId : undefined"
          :disabled="!convId"
          data-testid="cat-ctx-folder"
          @click="openFolder"
        ><FIcon :name="missing ? 'warn' : 'folder'" :size="13" /><b>{{ folderLabel }}</b></button>
        <span v-if="folderFull" :id="tipId" class="cx-tip" role="tooltip" data-testid="cat-ctx-folder-tip">{{ folderFull }}</span>
      </span>
      <template v-if="br.repo">
        <span class="cx cx-up" :class="{ dim: !br.upstream }" :title="br.upstream ? BC.upstreamTitle + '：' + br.upstream : BC.upstreamTitle" data-testid="cat-ctx-upstream"><FIcon name="globe" :size="13" /><i>{{ br.upstream || BC.noUpstream }}</i></span>
        <span class="cx cx-branch" :title="BC.localTitle">
          <FIcon name="branch" :size="13" />
          <CatModeSelect
            compact
            :model-value="br.branch ? 'L:' + br.branch : ''"
            :options="brOpts"
            :placeholder="br.detached ? BC.detached : '—'"
            :disabled="running || switching || missing"
            :busy="switching"
            :aria-label="BC.localTitle"
            testid="cat-ctx-branch"
            @update:model-value="onPickBranch"
          />
        </span>
      </template>
    </div>
    <div v-if="!welcome && brErr" class="ctx-err" role="alert" data-testid="cat-ctx-branch-err">
      <div class="ctx-err-line">
        <FIcon name="warn" :size="13" /><span>{{ brErr.text }}</span>
        <button
          v-if="brErr.raw"
          type="button"
          class="ct-od-more"
          :aria-expanded="brErrOpen"
          data-testid="cat-ctx-branch-err-more"
          @click="brErrOpen = !brErrOpen"
        ><span class="car" aria-hidden="true"><FIcon :name="brErrOpen ? 'down' : 'right'" :size="12" /></span>{{ BC.detail }}</button>
        <button type="button" class="ctx-err-x" aria-label="关闭" data-testid="cat-ctx-branch-err-close" @click="brErr = null"><FIcon name="x" :size="12" /></button>
      </div>
      <pre v-if="brErr.raw && brErrOpen" class="ct-od-raw" data-testid="cat-ctx-branch-err-raw">{{ brErr.raw }}</pre>
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
      <div v-if="picks.length" class="pick-notes" data-testid="cat-pick-notes">
        <div v-for="p in picks" :key="p.nid" class="pick-bubble" data-testid="cat-pick-bubble">
          <button type="button" class="rm" :aria-label="'移除 ' + p.label" @click="removePick(p.nid)"><FIcon name="x" :size="11" /></button>
          <div class="hd"><FIcon :name="p.intent === 'ask' ? 'ask' : 'edit'" :size="13" /><b :title="p.label">{{ p.label }}</b><span>{{ p.intent === 'ask' ? '疑问' : '改变' }}</span></div>
          <p v-if="p.note" class="note" :title="p.note">{{ p.note }}</p>
          <p v-else-if="p.selector" class="note path" :title="p.selector">{{ p.selector }}</p>
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
          v-if="modeOpts.length"
          ref="chipAccess"
          type="button"
          class="chip-access"
          :class="{ on: accessMeta.risky }"
          data-testid="cat-mode-chip"
          :disabled="missing"
          aria-haspopup="menu"
          :aria-expanded="menu === 'access'"
          @click.stop="toggle('access')"
        >
          <template v-if="accessMeta.risky"><svg class="tri" viewBox="0 0 12 12" aria-hidden="true"><path d="M6 1.2 11.2 10.5H.8Z" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" /><path d="M6 4.6v2.6M6 8.7v.1" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg></template><FIcon v-else name="shield" :size="14" /><span>{{ accessMeta.short }}</span><FIcon name="down" :size="11" class="caret up" />
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
        <div v-if="menu === 'access'" class="mn-access" role="menu" aria-label="访问模式" data-testid="cat-mode-menu">
          <div class="mn-ahd"><span>{{ CAT_MODE_MENU_TITLE }}</span></div>
          <div class="mn-list">
          <template v-for="a in modeOpts" :key="a.id">
          <div
            class="mn-acc"
            :class="{ on: accessMeta.id === a.id, danger: a.risky }"
            role="menuitemradio"
            tabindex="0"
            :aria-checked="accessMeta.id === a.id"
            @click="pickAccess(a.id)"
            @keydown.enter.prevent="pickAccess(a.id)"
          >
            <div>
              <b><svg v-if="a.risky" class="tri" viewBox="0 0 12 12" aria-hidden="true"><path d="M6 1.2 11.2 10.5H.8Z" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" /><path d="M6 4.6v2.6M6 8.7v.1" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg>{{ a.name }}<em v-if="a.id === currentCaps.modes.defaultId" class="def">默认</em></b>
              <small v-if="a.desc">{{ a.desc }}</small>
            </div>
            <FIcon v-if="accessMeta.id === a.id" name="check" :size="16" class="ok" />
          </div>
          </template>
          </div>
          <div v-if="confirmId" class="mn-confirm" role="alertdialog" aria-labelledby="cat-danger-t" data-testid="cat-danger-confirm" @click.stop>
            <b id="cat-danger-t"><svg class="tri" viewBox="0 0 12 12" aria-hidden="true"><path d="M6 1.2 11.2 10.5H.8Z" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" /><path d="M6 4.6v2.6M6 8.7v.1" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg>全部放开？</b>
            <p>Cat 助手将不经检查执行命令和修改文件，仅建议在隔离环境使用。</p>
            <div class="bt">
              <button type="button" class="cf-cancel" data-testid="cat-danger-cancel" @click="confirmId = ''">取消</button>
              <button type="button" class="cf-danger" data-testid="cat-danger-ok" @click="confirmDanger">全部放开</button>
            </div>
          </div>
        </div>
        <!-- D3 模型 + 思考强度合成一个浮层：上半模型列表，下半强度分段 -->
        <div v-else class="mn-pop" data-testid="cat-model-pop">
          <template v-if="catState.models.length || customOn">
            <div class="mn-hd">模型</div>
            <!-- v0.46 手动填写：原地变输入框 + 「取消」 -->
            <div v-if="manualOn" class="mn-manual" data-testid="cat-model-manual">
              <input
                ref="manualInp"
                v-model="manualDraft"
                class="mn-minp"
                type="text"
                spellcheck="false"
                autocomplete="off"
                :placeholder="PM.manualPlaceholder"
                aria-label="模型 ID"
                data-testid="cat-model-manual-input"
                @keydown.enter.prevent="commitManual"
                @keydown.esc.prevent.stop="cancelManual"
                @blur="commitManual"
              />
              <button v-if="catState.models.length" type="button" class="mn-mcancel" data-testid="cat-model-manual-cancel" @mousedown.prevent @click="cancelManual">{{ PM.cancel }}</button>
            </div>
            <div v-if="modelHint" class="mn-hint" :class="modelHintTone" role="status" data-testid="cat-model-hint">{{ modelHint }}</div>
            <div v-if="!manualOn" role="menu" aria-label="模型" data-testid="cat-model-list">
              <div
                v-if="followCli"
                class="mn-op two"
                :class="{ on: !catState.model }"
                role="menuitemradio"
                tabindex="0"
                :aria-checked="!catState.model"
                data-testid="cat-model-default"
                @click="pickModel('')"
                @keydown.enter.prevent="pickModel('')"
              >
                <span class="nm">{{ DEFAULT_MODEL_LABEL }}<small>{{ followCliModelDesc(cliName) }}</small></span>
                <FIcon v-if="!catState.model" name="check" :size="15" :stroke="2.2" class="ck" />
              </div>
              <div
                v-for="m in catState.models"
                :key="m.id"
                class="mn-op"
                :class="{ on: catState.model === m.id }"
                role="menuitemradio"
                tabindex="0"
                :aria-checked="catState.model === m.id"
                @click="pickModel(m.id)"
                @keydown.enter.prevent="pickModel(m.id)"
              >
                <span class="nm">{{ m.displayName }}</span>
                <FIcon v-if="catState.model === m.id" name="check" :size="15" :stroke="2.2" class="ck" />
              </div>
              <div v-if="customOn && notInList" class="mn-op on" role="menuitemradio" aria-checked="true" data-testid="cat-model-notinlist">
                <span class="nm">{{ catState.model }}<small class="nil">{{ PM.notInList }}</small></span>
                <FIcon name="check" :size="15" :stroke="2.2" class="ck" />
              </div>
              <template v-if="customOn">
                <div v-if="catState.models.length || notInList" class="mn-sep" role="separator" />
                <div class="mn-op" role="menuitem" tabindex="0" data-testid="cat-model-manual-entry" @click="startManual(true)" @keydown.enter.prevent="startManual(true)">
                  <span class="nm">{{ PM.manualEntry }}</span>
                </div>
              </template>
            </div>
          </template>
          <div v-else class="mn-hd">模型 · Cat 助手</div>
          <template v-if="currentCaps.efforts.options.length">
            <div class="mn-sep" role="separator" />
            <div class="mn-hd">思考强度</div>
            <div role="menu" aria-label="思考强度" data-testid="cat-effort-list">
              <div
                v-for="t in thinkSegs"
                :key="t.id || 'default'"
                class="mn-op"
                :class="{ on: t.on, flash: t.on && effortFlash }"
                role="menuitemradio"
                tabindex="0"
                :aria-checked="t.on"
                @click="pickThink(t.id)"
                @keydown.enter.prevent="pickThink(t.id)"
              >
                <span class="nm">{{ t.name }}<em v-if="t.def" class="def">默认</em></span>
                <FIcon v-if="t.on" name="check" :size="15" :stroke="2.2" class="ck" />
              </div>
            </div>
          </template>
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
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import CatModeSelect from '@/components/cat/panels/CatModeSelect.vue'
import { branchOptions, CAT_BRANCH_COPY as BC, catBranchError, listCatBranches, mapCatBranches, pathEllipsis, parseBranchOption, switchCatBranch, type CatBranches } from '@/api/catBranches'
import { revealCatConversationFolder } from '@/api/catFiles'
import FIcon from '@/components/icon/FIcon.vue'
import CatContextRing from '@/components/cat/CatContextRing.vue'
import { ElMessage } from 'element-plus'
import { CAT_COPY } from '@/api/cat'
import { composePickSend, pickLabel, type PickNote } from '@/components/cat/catPick'
import { CAT_PROJECT_COPY as PC } from '@/api/catProjects'
import { CAT_MODE_MENU_TITLE, DEFAULT_MODEL_LABEL, followCliModelDesc, followsCliModel, modeMeta } from '@/views/cat/catCapabilities'
import { CAT_PROVIDER_LABEL } from '@/api/catAgents'
import { ackDanger, dangerAcked } from '@/views/cat/catDangerAck'
import { pickFiles } from '@/api/system'
import { toAppError } from '@/api/call'
import { CAT_ATTACH_MAX, fileBaseName, splitAttach, type CatOutgoingFile } from '@/components/cat/catAttach'
import { canPreview, previewKind, showsThumb, type CatPreviewKind, type CatShownFile } from '@/components/cat/catMedia'
import CatThumb from '@/components/cat/CatThumb.vue'
import CatPreviewDialog from '@/components/cat/CatPreviewDialog.vue'
import { accessMeta, capsuleLabel, catState, chooseAccess, currentAgentKind, currentCaps, currentProject, chooseModel, chooseThink, customProviderOn, refreshModelsIfStale, THINK_DEFAULT_LABEL } from '@/views/cat/catState'
import { CAT_PROVIDER_MODELS_COPY as PM, normalizeManualModelId, providerModelHint, providerModelHintTone, providerModelNeedsManual } from '@/api/catProviderModels'

/**
 * Cat 输入框：批准方式可选普通模式 / Plan / 自动审查 / 全部批准，没选过是全部批准。
 * 合一胶囊只显示 List API 有的模型/强度（无强度不显示「·」半截）。
 */
const props = withDefaults(
  defineProps<{ welcome?: boolean; placeholder?: string; ctxName?: string; ctxBranch?: string; busy?: boolean; running?: boolean; stopping?: boolean; notReady?: boolean; checking?: boolean; missing?: boolean; projectName?: string; convId?: string; isProject?: boolean }>(),
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
    /** v0.49 底栏：当前对话 id；空 = 欢迎页，不读文件夹 / 分支 */
    convId: '',
    isProject: false,
  },
)
let missSeq = 0
const missId = `ct-miss-${++missSeq}-${Math.random().toString(36).slice(2, 7)}`
const emit = defineEmits<{ send: [payload: { text: string; files: CatOutgoingFile[]; own: string; notes: PickNote[] }]; stop: []; blocked: [] }>()

// ---- v0.49 底栏：文件夹路径 / 远程分支 / 本地分支切换（契约 6.19.24） ----
const br = ref<CatBranches>(mapCatBranches(null))
const switching = ref(false)
let brSeq = 0
const brOpts = computed(() => branchOptions(br.value))
const folderFull = computed(() => br.value.root)
const folderLabel = computed(() => (props.isProject || !br.value.root ? props.ctxName : pathEllipsis(br.value.root)))
const tipId = `ctx-tip-${Math.random().toString(36).slice(2, 8)}`
/** 切换失败行：已知原因只给文案；其它失败带原始报错，收进「查看详情」（同 cli_outdated） */
const brErr = ref<{ text: string; raw: string } | null>(null)
const brErrOpen = ref(false)
async function loadBranches() {
  const id = props.convId
  const seq = ++brSeq
  if (!id) {
    br.value = mapCatBranches(null)
    return
  }
  try {
    const r = await listCatBranches(id)
    if (seq === brSeq) br.value = r
  } catch {
    if (seq === brSeq) br.value = mapCatBranches(null)
  }
}
watch(() => props.convId, () => { brErr.value = null; void loadBranches() }, { immediate: true })
watch(() => props.running, (on) => { if (!on) void loadBranches() })
async function openFolder() {
  if (!props.convId) return
  try {
    await revealCatConversationFolder(props.convId)
  } catch (e) {
    void e
    ElMessage.warning(BC.openFailed)
  }
}
async function onPickBranch(id: string) {
  const p = parseBranchOption(id)
  if (!p || !props.convId || props.running || switching.value) return
  if (!p.remote && p.branch === br.value.branch) return
  switching.value = true
  brErr.value = null
  brErrOpen.value = false
  try {
    await switchCatBranch(props.convId, p.branch, p.remote)
    await loadBranches()
    ElMessage.success(BC.switched(br.value.branch || p.branch))
  } catch (e) {
    brErr.value = catBranchError(e)
    void loadBranches()
  } finally {
    switching.value = false
  }
}


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
interface ShownPick extends PickNote {
  nid: number
  label: string
}
let pickSeq = 0
const picks = ref<ShownPick[]>([])
const canSend = computed(() => (!!draft.value.trim() || atts.value.length > 0 || picks.value.length > 0) && !props.busy && !props.running && !props.notReady && !props.checking && !props.missing)
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

type MenuKind = 'access' | 'casc' | null
const menu = ref<MenuKind>(null)
const menuStyle = ref<Record<string, string>>({})


/** 强度分段：「默认」= 不传强度，其余按后端列表原样 */
const thinkSegs = computed(() => {
  const eff = currentCaps.value.efforts
  const segs = eff.options.map((t) => ({
    id: t.id,
    name: t.label,
    on: catState.think === t.id || (!catState.think && t.id === eff.defaultId),
    def: t.id === eff.defaultId,
  }))
  // 模型有默认档：默认档后面灰字「默认」，不再单独放「默认」段；没有默认档时保留「默认」= 不传强度
  return eff.defaultId ? segs : [{ id: '', name: THINK_DEFAULT_LABEL, on: !catState.think, def: false }, ...segs]
})
/** 模式菜单：按当前 agentKind 的列表渲染（列表空 → 胶囊隐藏） */
const modeOpts = computed(() => currentCaps.value.modes.options.map(modeMeta))
const followCli = computed(() => followsCliModel(currentAgentKind()))
const cliName = computed(() => (CAT_PROVIDER_LABEL as Record<string, string>)[currentAgentKind()] ?? '')
/** 切模型后档位被回落到默认：档位文字主色浅底高亮约 0.9 秒（设计 §4），不弹提示 */
const effortFlash = ref(false)
let flashTimer: ReturnType<typeof setTimeout> | undefined
function pickModel(id: string) {
  const before = catState.think
  chooseModel(id)
  if (before && catState.think !== before) {
    effortFlash.value = true
    clearTimeout(flashTimer)
    flashTimer = setTimeout(() => (effortFlash.value = false), 900)
  }
}
function pickThink(id: string) {
  chooseThink(id)
}

function open(kind: Exclude<MenuKind, null>) {
  const ci = inEl.value?.getBoundingClientRect()
  if (kind === 'access') {
    const a = chipAccess.value?.getBoundingClientRect()
    menuStyle.value = { left: Math.max(0, (a?.left ?? 0) - (ci?.left ?? 0) - 8) + 'px', right: 'auto' }
  } else {
    const a = chipModel.value?.getBoundingClientRect()
    menuStyle.value = { right: Math.max(0, (ci?.right ?? 0) - (a?.right ?? 0)) + 'px', left: 'auto' }
  }
  menu.value = kind
  void nextTick(fitMenu)
}
function confirmDanger() {
  ackDanger(currentProject.value?.path)
  const id = confirmId.value
  confirmId.value = ''
  chooseAccess(id)
  close()
}
/**
 * 弹层朝哪边开（设计复核 #260）：欢迎页（输入框居中）默认向下，对话页默认向上；
 * 默认方向放不下就换到放得下的一边；两边都放不下就选空间大的一边，菜单内滚动。
 * 可用高度写进 --mn-max，各菜单 max-height 跟它走。
 */
function clipRect(from: HTMLElement): { top: number; bottom: number } {
  for (let p = from.parentElement; p; p = p.parentElement) {
    const o = getComputedStyle(p)
    if (/(hidden|auto|scroll|clip)/.test(o.overflowY + o.overflow)) {
      const r = p.getBoundingClientRect()
      return { top: Math.max(0, r.top), bottom: Math.min(window.innerHeight, r.bottom) }
    }
  }
  return { top: 0, bottom: window.innerHeight }
}
/** 菜单打开时停在顶部；只有选中项没完整露出时，才最小幅度滚动把它露出来 */
function revealSelected() {
  const el = root.value?.querySelector('.mn') as HTMLElement | null
  if (!el) return
  const boxes = [el, ...Array.from(el.querySelectorAll<HTMLElement>('*'))].filter((n) => n.scrollHeight > n.clientHeight + 1 && /(auto|scroll)/.test(getComputedStyle(n).overflowY))
  for (const box of boxes) {
    box.scrollTop = 0
    const b = box.getBoundingClientRect()
    for (const on of Array.from(box.querySelectorAll<HTMLElement>('.on'))) {
      const r = on.getBoundingClientRect()
      if (r.bottom > b.bottom) box.scrollTop += r.bottom - b.bottom
      else if (r.top < b.top) box.scrollTop -= b.top - r.top
    }
  }
}
function fitMenu() {
  const el = root.value?.querySelector('.mn') as HTMLElement | null
  const anchor = el?.offsetParent as HTMLElement | null
  if (!el || !anchor || !menu.value) return
  const a = anchor.getBoundingClientRect()
  const c = clipRect(anchor)
  const GAP = 8
  const up = a.top - c.top - GAP * 2
  const down = c.bottom - a.bottom - GAP * 2
  const need = (el.firstElementChild as HTMLElement | null)?.scrollHeight ?? el.offsetHeight
  const prefer: 'up' | 'down' = props.welcome ? 'down' : 'up'
  const other = prefer === 'up' ? 'down' : 'up'
  const room = { up, down }
  const side = room[prefer] >= need ? prefer : room[other] >= need ? other : room.up >= room.down ? 'up' : 'down'
  const max = Math.max(120, Math.floor(room[side])) + 'px'
  void nextTick(revealSelected)
  menuStyle.value =
    side === 'down'
      ? { ...menuStyle.value, top: `calc(100% + ${GAP}px)`, bottom: 'auto', transformOrigin: 'left top', '--mn-max': max }
      : { ...menuStyle.value, top: 'auto', bottom: `calc(100% + ${GAP}px)`, transformOrigin: 'left bottom', '--mn-max': max }
}
function close() {
  confirmId.value = ''
  menu.value = null
}
function toggle(kind: Exclude<MenuKind, null>) {
  if (menu.value === kind) close()
  else {
    if (kind === 'casc') {
      refreshModelsIfStale()
      manualOn.value = false
      // 拉不到列表：输入框直接展开（打开下拉是用户操作，可以聚焦）
      const pm = catState.providerModels
      if (customOn.value && pm && providerModelNeedsManual(pm)) startManual(true)
    }
    open(kind)
  }
}

// ---------- v0.46 自定义服务商：手动填写模型 ID ----------
const customOn = computed(() => customProviderOn(currentAgentKind()))
const notInList = computed(() => !!catState.model && !catState.models.some((m) => m.id === catState.model))
const modelHint = computed(() => (customOn.value && catState.providerModels ? providerModelHint(catState.providerModels) : ''))
const modelHintTone = computed(() => (customOn.value && catState.providerModels ? providerModelHintTone(catState.providerModels) : ''))
const manualOn = ref(false)
const manualDraft = ref('')
const manualInp = ref<HTMLInputElement | null>(null)
function startManual(focus: boolean) {
  manualOn.value = true
  manualDraft.value = notInList.value ? catState.model : ''
  if (focus) void nextTick(() => manualInp.value?.focus())
}
function cancelManual() {
  if (!catState.models.length) return
  manualOn.value = false
}
function commitManual() {
  if (!manualOn.value || !manualDraft.value.trim()) return
  const id = normalizeManualModelId(manualDraft.value)
  if (!id) {
    ElMessage.warning(PM.manualInvalid)
    return
  }
  chooseModel(id)
  manualOn.value = false
  close()
}
const confirmId = ref('')
/** 切到 danger：这个项目第一次要确认；切走不用确认。取消 = 保持原模式 */
function pickAccess(id: string) {
  const opt = modeOpts.value.find((x) => x.id === id)
  if (!opt) {
    ElMessage.info(CAT_COPY.later)
    return
  }
  if (opt.risky && accessMeta.value.id !== id && !dangerAcked(currentProject.value?.path)) {
    confirmId.value = id
    return
  }
  confirmId.value = ''
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
function removePick(id: number) {
  picks.value = picks.value.filter((p) => p.nid !== id)
}
function send() {
  if (!canSend.value) {
    // 未就绪或正在检查时按 Enter，让父组件把横幅抖一下，文字照旧留在框里
    if ((props.notReady || props.checking) && (draft.value.trim() || atts.value.length || picks.value.length)) emit('blocked')
    return
  }
  const own = draft.value
  const notes = picks.value.map((p) => ({ ...p }))
  const files = atts.value.map((f) => ({ name: f.name, path: f.path }))
  emit('send', { text: composePickSend(own, notes), files, own, notes })
  draft.value = ''
  picks.value = []
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
  /** 网页里点选的元素，以气泡放进输入框。正文要等发送时才带上。 */
  addPick: (note: PickNote) => {
    const label = pickLabel(note)
    picks.value = [...picks.value, { ...note, nid: ++pickSeq, label }].slice(-8)
    nextTick(() => ta.value?.focus())
  },
  /** 发送没成功、后端也没存这条（如 CAT_PROJECT_MISSING）：把文字放回输入框；用户已经又输入了就不覆盖，不抢焦点 */
  restoreDraft: (t: string) => { if (draft.value.trim()) return; draft.value = t; nextTick(autosize) },
  restoreFiles: (files: CatOutgoingFile[]) => {
    if (atts.value.length || !files?.length) return
    atts.value = files.filter((f) => f.path).map(toDraft)
  },
  /** 发送没被收下时，把气泡放回来。输入框里已经有新气泡就不覆盖。 */
  restorePicks: (notes: PickNote[]) => {
    if (picks.value.length || !notes?.length) return
    picks.value = notes.slice(-8).map((n) => ({ ...n, nid: ++pickSeq, label: pickLabel(n) }))
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
  width: calc(100% - 96px);
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
  /* 文件夹完整路径 tooltip 要伸出底栏上方 */
  overflow: visible;
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
.ctx-bar .cx.cx-fwrap {
  position: relative;
  overflow: visible;
}
.ctx-bar .cx-folder {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  max-width: 240px;
  border: 0;
  padding: 0;
  background: none;
  color: inherit;
  font: inherit;
  cursor: pointer;
  text-align: left;
}
.ctx-bar .cx-folder:hover:not(:disabled) b {
  color: var(--cat-text);
  text-decoration: underline;
}
.ctx-bar .cx-folder:disabled {
  cursor: default;
}
.ctx-bar .cx-folder b {
  min-width: 0;
}
.ctx-bar .cx-folder:disabled b {
  text-decoration: none;
}
/* 完整路径 tooltip：elevated 样式 + 等宽字，换行不截断 */
.cx-tip {
  position: absolute;
  left: 0;
  bottom: calc(100% + 6px);
  z-index: 20;
  width: max-content;
  max-width: min(480px, 70vw);
  padding: 6px 8px;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  background: var(--ff-bg-elevated);
  box-shadow: var(--ff-shadow-dialog);
  color: var(--cat-text);
  font: 12px/1.5 var(--ff-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  white-space: pre-wrap;
  word-break: break-all;
  pointer-events: none;
  opacity: 0;
  visibility: hidden;
  transition: opacity 120ms var(--ff-ease) 300ms;
}
.cx-fwrap:hover .cx-tip,
.cx-fwrap:focus-within .cx-tip {
  opacity: 1;
  visibility: visible;
}
.ctx-bar .cx-up i {
  font-style: normal;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ctx-err {
  margin: -2px 12px 6px;
  font-size: 12px;
  color: var(--cat-text-2);
}
.ctx-err-line {
  display: flex;
  align-items: center;
  gap: 6px;
}
.ctx-err-line > :deep(svg) {
  color: var(--ff-danger);
  flex: none;
}
.ctx-err-x {
  margin-left: auto;
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border: 0;
  border-radius: 4px;
  background: none;
  color: var(--cat-muted);
  cursor: pointer;
}
.ctx-err-x:hover { background: var(--cat-hover); }
.ctx-err .ct-od-more { display: inline-flex; align-items: center; gap: 4px; border: 0; padding: 0; background: none; color: var(--cat-muted); font-size: 12px; cursor: pointer; }
.ctx-err .ct-od-more:hover { color: var(--cat-text-2); }
.ctx-err .ct-od-more .car { width: 12px; display: grid; place-items: center; }
.ctx-err .ct-od-raw {
  margin: 6px 0 0;
  max-height: 120px;
  overflow: auto;
  padding: 8px 10px;
  border-radius: var(--cat-radius-md);
  border: 1px solid var(--cat-border);
  background: var(--cat-elevated);
  color: var(--cat-text-2);
  font: 12px/1.6 var(--ff-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  white-space: pre-wrap;
  word-break: break-word;
  user-select: text;
}
.ctx-bar .cx.dim {
  color: var(--cat-muted);
  opacity: 0.7;
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
.pick-notes {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 0 0 8px;
}
.pick-bubble {
  position: relative;
  max-width: min(100%, 260px);
  padding: 8px 22px 8px 10px;
  border-radius: 14px 14px 14px 4px;
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.pick-bubble .hd {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: 12px;
}
.pick-bubble .hd b {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 650;
}
.pick-bubble .hd span {
  flex: none;
  color: var(--ff-text-3);
}
.pick-bubble .note {
  margin: 4px 0 0;
  font-size: 12px;
  line-height: 1.45;
  color: var(--ff-text-2);
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.pick-bubble .rm {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 16px;
  height: 16px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 50%;
  background: transparent;
  color: var(--ff-text-3);
  cursor: pointer;
  padding: 0;
}
.pick-bubble .rm:hover {
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
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

.mn-pop {
  width: 260px;
  box-sizing: border-box;
  padding: 6px;
  background: var(--cat-elevated);
  border: 1px solid var(--cat-border);
  border-radius: var(--cat-radius-lg);
  box-shadow: var(--cat-shadow-lg);
}
/* v0.46 手动填写模型 ID：沿用菜单项高度，宽度不变 */
.mn-manual {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px 4px 4px;
}
.mn-minp {
  flex: 1;
  min-width: 0;
  height: 30px;
  padding: 0 8px;
  border: 1px solid var(--cat-border);
  border-radius: var(--cat-radius-sm);
  background: var(--cat-bg);
  color: var(--cat-text);
  font-size: 13px;
  font-family: var(--ff-font-mono, ui-monospace, monospace);
}
.mn-minp:focus {
  outline: none;
  border-color: var(--cat-accent);
  box-shadow: var(--cat-focus);
}
.mn-mcancel {
  flex: none;
  border: 0;
  background: transparent;
  color: var(--cat-text-2);
  font-size: 12.5px;
  padding: 4px 6px;
  border-radius: var(--cat-radius-sm);
  cursor: pointer;
}
.mn-mcancel:hover {
  background: var(--cat-hover);
  color: var(--cat-text);
}
.mn-hint {
  padding: 2px 8px 6px;
  font-size: 12px;
  line-height: 1.5;
  white-space: normal;
}
.mn-hint.danger {
  color: var(--ff-danger);
}
.mn-hint.muted {
  color: var(--cat-muted);
}
.mn-op .nil {
  margin-left: 4px;
  font-size: 12px;
  font-weight: 400;
  color: var(--cat-muted);
}
.mn-hd {
  padding: 6px 8px 4px;
  font-size: 11.5px;
  color: var(--cat-muted);
}
.mn-op {
  height: 34px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px 0 10px;
  border-radius: var(--cat-radius-sm);
  font-size: 13.5px;
  color: var(--cat-text);
  cursor: pointer;
  white-space: nowrap;
  transition: background var(--ff-dur-quick);
}
.mn-op .nm {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.mn-op:hover {
  background: var(--cat-hover);
}
.mn-op.on {
  background: var(--cat-active);
  color: var(--cat-active-text);
  font-weight: 600;
}
.mn-op .ck {
  flex: none;
  color: var(--cat-active-text);
}
.mn-seg {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
  margin: 2px 4px 4px;
  padding: 3px;
  border-radius: var(--cat-radius-md);
  background: var(--cat-sunken);
}
.mn-seg button {
  flex: 1 0 auto;
  min-width: 0;
  padding: 0 6px;
  height: 26px;
  border: none;
  border-radius: var(--cat-radius-sm);
  background: transparent;
  color: var(--cat-text-2);
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
  transition: background var(--ff-dur-quick), color var(--ff-dur-quick);
}
.mn-seg button:hover {
  color: var(--cat-text);
}
.mn-seg button.on {
  background: var(--cat-elevated);
  color: var(--cat-text);
  font-weight: 600;
  box-shadow: var(--cat-shadow-sm);
}
.mn-seg button:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
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
.mn-seg .def {
  margin-left: 3px;
  font-size: 10px;
  color: var(--cat-text-3, #999);
}
/* v0.44 danger：名字前几何三角警示；说明保持灰 */
.mn-acc .tri {
  width: 12px;
  height: 12px;
  margin-right: 5px;
  vertical-align: -1px;
}
.mn-acc.danger small {
  color: var(--cat-muted);
}
.mn-access {
  position: relative;
}
.mn-confirm {
  position: absolute;
  left: calc(100% + 8px);
  bottom: 0;
  width: 260px;
  box-sizing: border-box;
  padding: 14px;
  background: var(--cat-elevated);
  border: 1px solid var(--cat-border);
  border-radius: 12px;
  box-shadow: var(--cat-shadow-lg);
  color: var(--cat-text);
}
.mn-confirm b {
  font-size: 14px;
}
.mn-confirm p {
  margin: 6px 0 12px;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--cat-muted);
}
.mn-confirm .bt {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.mn-confirm button {
  height: 28px;
  padding: 0 12px;
  border-radius: 8px;
  font-size: 12.5px;
  cursor: pointer;
  border: 1px solid var(--cat-border);
  background: transparent;
  color: var(--cat-text);
}
.mn-confirm .cf-danger {
  border-color: transparent;
  background: var(--cat-danger, #d93025);
  color: #fff;
}
/* v0.44 设计 v0.1 对齐 */
.mn-access {
  width: 220px;
  padding: 6px;
  border-radius: 12px;
}
.mn-access .mn-ahd {
  padding: 6px 8px 4px;
  font-size: 11.5px;
  color: var(--cat-muted);
}
.mn-list {
  /* 可用高度减去标题行与内边距；放不下就在列表内滚动 */
  max-height: calc(var(--mn-max, 100vh) - 40px);
  overflow-y: auto;
}
.mn-access .mn-acc {
  padding: 7px 8px;
  border-radius: 8px;
  gap: 8px;
}
.mn-access .mn-acc b {
  font-size: 13px;
  font-weight: 550;
  display: flex;
  align-items: center;
}
.mn-access .mn-acc small {
  /* 说明最多两行自然换行，不截断（设计 v0.1.1） */
  white-space: normal;
  overflow: visible;
  word-break: break-word;
}
.mn-access .mn-acc.on {
  background: transparent;
}
.mn-access .mn-acc.on b {
  color: var(--cat-text);
}
.mn-access .mn-acc:hover {
  background: var(--cat-hover);
}
.mn-access .mn-acc .ok {
  color: var(--cat-accent);
}
.mn-access .mn-acc.danger b {
  color: var(--cat-orange);
}
.mn-access .mn-acc.danger.on {
  background: var(--cat-hover);
}
.def {
  margin-left: 6px;
  font-style: normal;
  font-weight: 400;
  font-size: 11.5px;
  color: var(--cat-muted);
}
.chip-access .tri {
  width: 13px;
  height: 13px;
}
.chip-access .caret.up {
  transform: rotate(180deg);
  opacity: 0.7;
}
.mn-confirm {
  width: 280px;
}
.mn-confirm b {
  /* Chrome 默认 text-spacing-trim 会把行尾全角「？」压成半宽，这里保持全宽 */
  text-spacing-trim: space-all;
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
}
.mn-confirm b .tri {
  width: 14px;
  height: 14px;
  color: var(--cat-orange);
}
.mn-confirm p {
  color: var(--cat-text-2);
}
.mn-confirm button {
  height: 30px;
}
.mn-pop {
  width: 240px;
  max-height: var(--mn-max, 100vh);
  overflow-y: auto;
  border-radius: 12px;
}
.mn-op .ck {
  color: var(--cat-accent);
}
.mn-op.flash .nm {
  background: color-mix(in srgb, var(--cat-accent) 14%, transparent);
  color: var(--cat-accent);
  border-radius: 4px;
  transition: background 0.3s;
}
.mn-op.two {
  height: auto;
  padding-top: 6px;
  padding-bottom: 6px;
}
.mn-op.two .nm small {
  display: block;
  font-size: 11.5px;
  color: var(--cat-muted);
}
</style>
