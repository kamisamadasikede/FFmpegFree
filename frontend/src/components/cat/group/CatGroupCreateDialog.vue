<template>
  <div class="cg-modal-bg" @click.self="emit('close')">
    <div class="cg-modal wide" role="dialog" aria-modal="true" :aria-label="mode === 'group' ? t('cat.groupChat.newGroup') : t('cat.groupChat.addMember')" data-testid="cat-group-create">
      <h3>{{ mode === 'group' ? t('cat.groupChat.newGroup') : t('cat.groupChat.addMember') }}</h3>
      <div class="body">{{ t('cat.groupChat.createHint') }}</div>

      <template v-if="mode === 'group'">
        <div class="step"><i>1</i>{{ t('cat.groupChat.createStep1') }}</div>
        <input v-model="name" class="cg-in" :class="{ bad: nameBad }" maxlength="40" style="height: 34px" :placeholder="t('cat.groupChat.groupNamePh')" :aria-label="t('cat.groupChat.groupName')" data-testid="cat-group-create-name">
        <div v-if="nameBad" class="cg-err">{{ t('cat.groupChat.groupNameBad') }}</div>
        <div class="step"><i>2</i>{{ t('cat.groupChat.createStep2') }}</div>
      </template>
      <div v-else style="height: 12px" />

      <div class="cg-mrow cg-hint" style="margin-bottom: 2px">
        <span /><span>{{ t('cat.groupChat.memberTitle') }}</span><span>{{ t('cat.groupChat.memberName') }} · {{ t('cat.groupChat.optional') }}</span><span>{{ t('cat.groupChat.memberCli') }}</span><span>{{ t('cat.groupChat.memberModel') }}</span><span />
      </div>
      <div v-for="(r, i) in rows" :key="r.key" class="cg-mrow">
        <span class="av sm" :style="{ '--c': memberColorVar(baseOrder + i) }">{{ Array.from(autoName(i))[0]?.toUpperCase() || i + 1 }}</span>
        <input v-model="r.title" class="cg-in" :placeholder="t('cat.groupChat.memberTitlePh')" :aria-label="t('cat.groupChat.memberTitle')">
        <input v-model="r.name" class="cg-in" :class="{ bad: dupOf(i) }" :placeholder="autoName(i) || t('cat.groupChat.memberNamePh')" :aria-label="t('cat.groupChat.memberName')">
        <select v-model="r.agentKind" class="cg-sel" :aria-label="t('cat.groupChat.memberCli')" @change="r.modelId = MODELS[r.agentKind]?.[0] ?? ''">
          <option v-for="a in AGENTS" :key="a.kind" :value="a.kind">{{ a.label }}</option>
        </select>
        <select v-model="r.modelId" class="cg-sel" :aria-label="t('cat.groupChat.memberModel')">
          <option v-for="mm in MODELS[r.agentKind] ?? []" :key="mm" :value="mm">{{ mm }}</option>
        </select>
        <button v-if="mode === 'group' && rows.length > 1" type="button" class="cg-ib" :aria-label="t('cat.groupChat.removeMember')" @click="rows.splice(i, 1)"><FIcon name="x" :size="13" /></button>
        <span v-else />
      </div>
      <div v-if="rows.some((_, i) => dupOf(i))" class="cg-err">{{ t('cat.groupChat.memberNameTaken') }}</div>
      <button v-if="mode === 'group'" type="button" class="cg-add" style="width: 100%; margin-top: 4px" @click="rows.push(row())"><FIcon name="plus" :size="13" />{{ t('cat.groupChat.addMember') }}</button>
      <div v-if="err" class="cg-err">{{ err }}</div>

      <div class="ft">
        <button type="button" class="cg-btn" @click="emit('close')">{{ t('cat.groupChat.cancel') }}</button>
        <button type="button" class="cg-btn pri" :disabled="!canSave || busy" data-testid="cat-group-create-ok" @click="save()">{{ mode === 'group' ? t('cat.groupChat.create') : t('cat.groupChat.save') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import FIcon from '@/components/icon/FIcon.vue'
import { memberColorVar } from '@/api/catGroupChat'
import { groupScene } from '@/api/catGroupChatMock'
import { useCatGroupChat } from '@/stores/catGroupChat'
import { openGroup } from '@/views/cat/catPanels'
import './catGroup.css'

const props = defineProps<{ mode: 'group' | 'member'; projectId?: string }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const store = useCatGroupChat()

const AGENTS = [
  { kind: 'cat_build', label: 'Grok Build' },
  { kind: 'claude_cli', label: 'Claude' },
  { kind: 'codex_cli', label: 'Codex' },
]
// 设计猜测：先给每家一份固定模型列表；真实可选模型应接 getCatAgentCapabilities（TODO）
const MODELS: Record<string, string[]> = {
  cat_build: ['grok-code-fast-1', 'grok-4'],
  claude_cli: ['claude-sonnet-4.5', 'claude-opus-4.1', 'claude-haiku-4.5'],
  codex_cli: ['gpt-5.3-codex', 'gpt-5'],
}
let key = 0
function row(title = '', agentKind = 'cat_build', nm = '') {
  return reactive({ key: ++key, title, name: nm, agentKind, modelId: MODELS[agentKind][0] })
}
const demo = props.mode === 'group' && groupScene() === 'create'
const name = ref(demo ? 'Release checklist' : '')
const rows = reactive(demo ? [row('PM', 'claude_cli'), row('Frontend', 'cat_build'), row('Frontend', 'codex_cli'), row('', 'cat_build')] : [row()])
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
const canSave = computed(() => {
  const filled = rows.filter((r) => r.title.trim())
  const okRows = filled.length > 0 && filled.every((r) => r.modelId) && !rows.some((_, i) => dupOf(i))
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
