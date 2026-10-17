<template>
  <div class="cg-modal-bg" @click.self="emit('close')">
    <div class="cg-modal wide" role="dialog" aria-modal="true" :aria-label="mode === 'group' ? t('cat.groupChat.newGroup') : t('cat.groupChat.addMember')" data-testid="cat-group-create">
      <h3>{{ mode === 'group' ? t('cat.groupChat.newGroup') : t('cat.groupChat.addMember') }}</h3>
      <div class="body">{{ mode === 'group' ? t('cat.groupChat.createSub', { project: projectName }) : t('cat.groupChat.createHint') }}</div>

      <div v-if="mode === 'group'" class="cg-fld1">
        <label for="cg-new-name">{{ t('cat.groupChat.groupName') }}</label>
        <input id="cg-new-name" v-model="name" class="cg-in" :class="{ bad: nameBad }" maxlength="40" :placeholder="t('cat.groupChat.groupNamePh')" data-testid="cat-group-create-name">
        <div v-if="nameBad" class="cg-err">{{ t('cat.groupChat.groupNameBad') }}</div>
      </div>

      <div class="cg-hint" style="margin-bottom: 6px">{{ t('cat.groupChat.members') }} · {{ t('cat.groupChat.createOrderHint') }}</div>
      <div class="cg-mrow head">
        <span /><span /><span>{{ t('cat.groupChat.memberCli') }}</span><span>{{ t('cat.groupChat.memberTitle') }}</span><span>{{ t('cat.groupChat.memberModel') }}（{{ t('cat.groupChat.optional') }}）</span><span>{{ t('cat.groupChat.colAlias') }}</span><span />
      </div>
      <template v-for="(r, i) in rows" :key="r.key">
        <div class="cg-mrow">
          <span class="grip"><FIcon name="grip" :size="13" /></span>
          <span class="av sm" :style="{ '--c': memberColorVar(baseOrder + i) }">{{ Array.from(autoName(i))[0]?.toUpperCase() || i + 1 }}</span>
          <select v-model="r.agentKind" class="cg-sel" :aria-label="t('cat.groupChat.memberCli')" data-testid="cat-group-member-cli" @change="onCliChange(r)">
            <option v-for="a in AGENTS" :key="a.kind" :value="a.kind">{{ a.label }}</option>
          </select>
          <input v-model="r.title" class="cg-in" :placeholder="t('cat.groupChat.memberTitlePh')" :aria-label="t('cat.groupChat.memberTitle')">
          <select v-model="r.modelId" class="cg-sel" :aria-label="t('cat.groupChat.memberModel')" data-testid="cat-group-member-model">
            <option value="">{{ t('cat.groupChat.defaultModel') }}</option>
            <option v-for="mm in modelsOf(r.agentKind)" :key="mm.id" :value="mm.id">{{ mm.displayName }}</option>
          </select>
          <span class="cg-alias" :class="{ bad: dupOf(i) }"><i>{{ AT }}</i><input v-model="r.name" :placeholder="autoName(i) || t('cat.groupChat.memberNamePh')" :aria-label="t('cat.groupChat.colAlias')"></span>
          <button v-if="mode === 'group' && rows.length > 1" type="button" class="cg-ib" :aria-label="t('cat.groupChat.removeMember')" @click="rows.splice(i, 1)"><FIcon name="trash" :size="13" /></button>
          <span v-else />
        </div>
        <div v-if="dupOf(i)" class="cg-err cg-mrow-err">{{ t('cat.groupChat.memberNameTaken') }} · <button type="button" class="lkb" @click="r.name = suggest(i)">{{ t('cat.groupChat.nameSuggest', { name: suggest(i) }) }}</button></div>
      </template>
      <button v-if="mode === 'group'" type="button" class="cg-add" style="width: 100%; margin-top: 4px" @click="addRow()"><FIcon name="plus" :size="13" />{{ t('cat.groupChat.addMember') }}</button>
      <div v-if="mode === 'group'" class="cg-info"><FIcon name="info" :size="12" />{{ t('cat.groupChat.createInfo') }}</div>
      <div v-if="err" class="cg-err">{{ err }}</div>

      <div class="ft">
        <span class="cg-hint" style="margin-right: auto">{{ t('cat.groupChat.nMembers', { n: rows.filter((r) => r.title.trim()).length }) }}</span>
        <button type="button" class="cg-btn" @click="emit('close')">{{ t('cat.groupChat.cancel') }}</button>
        <button type="button" class="cg-btn pri" :disabled="!canSave || busy" data-testid="cat-group-create-ok" @click="save()">{{ mode === 'group' ? t('cat.groupChat.create') : t('cat.groupChat.save') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import FIcon from '@/components/icon/FIcon.vue'
import { CAT_PROVIDER_LABEL, CAT_PROVIDER_ORDER } from '@/api/catAgents'
import { memberColorVar } from '@/api/catGroupChat'
import { groupScene } from '@/api/catGroupChatMock'
import { useCatGroupChat } from '@/stores/catGroupChat'
import { openGroup } from '@/views/cat/catPanels'
import { catProjectName } from './catProjectName'
import { defaultModelId, loadCliModels, modelsOf, preloadAllCliModels, refreshCliModels } from './catGroupCliModels'
import './catGroup.css'

const props = defineProps<{ mode: 'group' | 'member'; projectId?: string }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const store = useCatGroupChat()

/** 与设置页 / 底栏同一套三家顺序与文案 */
const AGENTS = CAT_PROVIDER_ORDER.map((kind) => ({ kind, label: CAT_PROVIDER_LABEL[kind] }))

type MemberRow = { key: number; title: string; name: string; agentKind: string; modelId: string }

/** CLI 一变：刷新该家模型列表；默认模型 = defaultsByAgent（在列表里）否则空=跟随 CLI */
function onCliChange(r: MemberRow) {
  r.modelId = ''
  void refreshCliModels(r.agentKind).then((list) => {
    r.modelId = defaultModelId(r.agentKind, list)
  })
}

onMounted(() => {
  preloadAllCliModels(CAT_PROVIDER_ORDER)
})

let key = 0
const AT = '@'
const DEMO_ROWS: [string, string, string][] = [
  ['\u67b6\u6784\u5e08', 'claude_cli', ''],
  ['\u540e\u7aef\u5de5\u7a0b\u5e08', 'codex_cli', ''],
  ['\u524d\u7aef\u5de5\u7a0b\u5e08', 'cat_build', ''],
  ['\u540e\u7aef\u5de5\u7a0b\u5e08', 'claude_cli', '\u540e\u7aef\u5de5\u7a0b\u5e08'],
]
function row(title = '', agentKind = 'cat_build', nm = '', model = ''): MemberRow {
  return reactive({ key: ++key, title, name: nm, agentKind, modelId: model || '' })
}
function addRow() {
  const r = row()
  rows.push(r)
  void loadCliModels(r.agentKind)
}
const demo = props.mode === 'group' && groupScene() === 'create'
// 走查（create 场景）：与设计原型同一组成员，第 4 行故意重名以展示提示
const name = ref(demo ? 'v0.55 \u7fa4\u804a\u5f00\u53d1' : '')
const rows = reactive(demo ? DEMO_ROWS.map(([title, kind, nm]) => row(title, kind, nm)) : [row()])
const baseOrder = computed(() => (props.mode === 'member' ? store.members.length : 0))
const busy = ref(false)
const err = ref('')
const nameBad = computed(() => props.mode === 'group' && name.value.length > 0 && !name.value.trim())
const taken = computed(() => (props.mode === 'member' ? store.members.map((m) => m.name.trim().toLowerCase()) : []))

/** 名称没填 = 用职位；同职位第二个起加「 2」「 3」（契约 6.21.2） */
function autoName(i: number): string {
  const r = rows[i]
  if (r.name.trim()) return r.name.trim()
  const base = r.title.trim()
  if (!base) return ''
  const before = rows.slice(0, i).map((_, j) => autoName(j).toLowerCase())
  let n = 1
  let cand = base
  while (taken.value.includes(cand.toLowerCase()) || before.includes(cand.toLowerCase())) cand = `${base} ${++n}`
  return cand
}
function dupOf(i: number) {
  const n = rows[i].name.trim().toLowerCase()
  return !!n && (taken.value.includes(n) || rows.some((_, j) => j !== i && autoName(j).toLowerCase() === n))
}
function suggest(i: number) {
  const base = rows[i].name.trim() || rows[i].title.trim()
  const used = [...taken.value, ...rows.map((_, j) => (j === i ? '' : autoName(j).toLowerCase()))]
  let n = 2
  while (used.includes(`${base} ${n}`.toLowerCase())) n++
  return `${base} ${n}`
}
const projectName = computed(() => catProjectName(props.projectId ?? ''))
const canSave = computed(() => {
  const filled = rows.filter((r) => r.title.trim())
  const okRows = filled.length > 0 && !rows.some((_, i) => dupOf(i))
  return props.mode === 'group' ? !!name.value.trim() && name.value.trim().length <= 40 && okRows : okRows
})

async function save() {
  if (!canSave.value) return
  busy.value = true
  err.value = ''
  try {
    if (props.mode === 'group') {
      const g = await store.createGroup(props.projectId ?? '', name.value.trim())
      await store.open(g.id)
      openGroup(g.id)
    }
    for (let i = 0; i < rows.length; i++) {
      const r = rows[i]
      if (!r.title.trim()) continue
      await store.addMember({ name: autoName(i), title: r.title.trim(), agentKind: r.agentKind, modelId: r.modelId })
    }
    emit('close')
  } catch (e) {
    const d = String((e as { detail?: string })?.detail ?? (e as Error)?.message ?? '')
    err.value = d.includes('member_name_taken') ? t('cat.groupChat.memberNameTaken') : t('cat.groupChat.saveFailed')
  } finally {
    busy.value = false
  }
}
</script>
