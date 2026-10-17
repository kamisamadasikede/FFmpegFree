<template>
  <aside class="cg-mem" :aria-label="t('cat.groupChat.members')" data-testid="cat-group-members">
    <div class="cg-mem-hd">
      <div class="cg-tabs" role="tablist">
        <button type="button" role="tab" :aria-selected="tab === 'm'" :class="{ on: tab === 'm' }" @click="tab = 'm'">{{ t('cat.groupChat.members') }} · {{ store.members.length }}</button>
        <button type="button" role="tab" :aria-selected="tab === 'f'" :class="{ on: tab === 'f' }" @click="tab = 'f'">{{ t('cat.groupChat.tabFiles') }}</button>
      </div>
      <span class="sp" />
      <button type="button" class="cg-ib" :title="t('cat.groupChat.addMember')" :aria-label="t('cat.groupChat.addMember')" @click="emit('add')"><FIcon name="plus" :size="15" /></button>
      <button v-if="closable" type="button" class="cg-ib" :aria-label="t('cat.groupChat.close')" @click="emit('close')"><FIcon name="x" :size="14" /></button>
    </div>
    <div v-if="tab === 'f'" class="cg-mem-list"><div class="cg-hint" style="padding: 12px">{{ t('cat.groupChat.filesTabEmpty') }}</div></div>
    <div v-else class="cg-mem-list" role="list">
      <div
        v-for="(m, i) in store.membersSorted"
        :key="m.id"
        class="cg-mc"
        :class="{ open: open.has(m.id), drag: dragId === m.id, dropb: overId === m.id && dragId !== m.id }"
        role="listitem"
        :data-mid="m.id"
        @dragover.prevent="overId = m.id"
        @drop.prevent="drop(m.id)"
      >
        <div class="cg-mc-row" role="button" tabindex="0" :aria-expanded="open.has(m.id)" @click="toggle(m.id)" @keydown.enter.self="toggle(m.id)">
          <span class="grip" draggable="true" :title="t('cat.groupChat.dragTip')" @dragstart="dragId = m.id" @dragend="dragId = overId = ''" @click.stop><FIcon name="grip" :size="14" /></span>
          <span class="av md" :style="{ '--c': memberColorVar(m.order) }">{{ initial(m.name) }}<i v-if="stateOf(m.id) === 'over'" class="odot" /></span>
          <span class="nm">
            <span class="a">
              {{ m.name }}
              <span v-if="i === 0" class="cg-badge acc" :title="t('cat.groupChat.firstTip')">{{ t('cat.groupChat.firstBadge') }}</span>
              <span v-if="m.charter.status === 'generating'" class="cg-spin" style="width: 10px; height: 10px; border-width: 1.5px" :title="t('cat.groupChat.generating')" />
              <span v-else-if="m.charter.status === 'failed'" :title="t('cat.groupChat.charterFailed')" style="display: inline-flex; color: var(--cat-warn-text)"><FIcon name="warn" :size="11" /></span>
            </span>
            <span class="b">{{ m.title }} · <CatAgentMark :kind="normalizeProviderKind(m.agentKind)" :size="11" />{{ cliName(m.agentKind) }}</span>
          </span>
          <span class="cg-st" :class="stateOf(m.id)">
            <template v-if="stateOf(m.id) === 'busy'">● {{ t('cat.groupChat.busy') }}<button type="button" class="cg-ib sm" :aria-label="t('cat.groupChat.stop')" @click.stop="store.stop(m.id)"><FIcon name="stop" :size="10" /></button></template>
            <template v-else-if="stateOf(m.id) === 'over'">{{ t('cat.groupChat.overPending') }}</template>
            <template v-else-if="stateOf(m.id) === 'stopped'">{{ t('cat.groupChat.stoppedState') }}</template>
            <template v-else-if="stateOf(m.id) === 'failed'">{{ t('cat.groupChat.runFailed') }}</template>
            <template v-else>{{ t('cat.groupChat.idle') }}</template>
          </span>
        </div>
        <template v-if="open.has(m.id)">
          <CatGroupCharter :member="m" :start-editing="editIds.has(m.id)" />
          <div class="cg-mc-acts">
            <button type="button" class="cg-btn ghost" :disabled="i === 0" :aria-label="t('cat.groupChat.moveUp')" :title="t('cat.groupChat.moveUp')" @click="move(m.id, -1)"><FIcon name="up" :size="12" /></button>
            <button type="button" class="cg-btn ghost" :disabled="i === store.members.length - 1" :aria-label="t('cat.groupChat.moveDown')" :title="t('cat.groupChat.moveDown')" @click="move(m.id, 1)"><FIcon name="down" :size="12" /></button>
            <span class="sp" />
            <button type="button" class="cg-btn ghost red" @click="remove(m.id)">{{ t('cat.groupChat.removeMember') }}</button>
          </div>
        </template>
      </div>
      <button type="button" class="cg-add" data-testid="cat-group-add-member" @click="emit('add')"><FIcon name="plus" :size="13" />{{ t('cat.groupChat.addMember') }}</button>
    </div>
    <div v-if="tab === 'm'" class="cg-mem-ft">{{ t('cat.groupChat.membersFoot') }}</div>
  </aside>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import CatAgentMark from '@/components/cat/CatAgentMark.vue'
import { normalizeProviderKind } from '@/api/catAgents'
import { memberColorVar } from '@/api/catGroupChat'
import { useCatGroupChat } from '@/stores/catGroupChat'
import CatGroupCharter from './CatGroupCharter.vue'

const props = defineProps<{ closable?: boolean; initialOpen?: string }>()
const emit = defineEmits<{ close: []; add: [] }>()
const { t } = useI18n()
const store = useCatGroupChat()
const initial = (n: string) => Array.from(n.trim())[0]?.toUpperCase() ?? ''

const open = reactive(new Set<string>())
const editIds = reactive(new Set<string>())
/** initialOpen：逗号分隔的成员 id；前缀 edit: 表示直接进编辑态（走查场景用） */
watch(
  () => props.initialOpen,
  (v) => {
    for (const raw of (v ?? '').split(',').filter(Boolean)) {
      const id = raw.replace(/^edit:/, '')
      open.add(id)
      if (raw.startsWith('edit:')) editIds.add(id)
    }
  },
  { immediate: true },
)
const toggle = (id: string) => {
  const was = open.has(id)
  open.clear()
  if (!was) open.add(id)
}
const tab = ref<'m' | 'f'>('m')
const CLI: Record<string, string> = { claude_cli: 'Claude', codex_cli: 'Codex', cat_build: 'Grok Build', gemini_cli: 'Gemini' }
const cliName = (k: string) => CLI[k] ?? k
function stateOf(id: string) {
  if (store.live[id]) return 'busy'
  const last = [...store.messages].reverse().find((x) => x.memberId === id)
  if (!last) return 'idle'
  if (last.systemKind === 'failed') return 'failed'
  if (last.systemKind === 'stopped') return 'stopped'
  if (last.report?.files.some((f) => f.tag === 'overreach') && !store.revertedSeqs?.includes(last.seq)) return 'over'
  return 'idle'
}

const dragId = ref('')
const overId = ref('')
async function reorderTo(ids: string[]) {
  try {
    await store.reorder(ids)
  } catch {
    ElMessage.error(t('cat.groupChat.saveFailed'))
  }
}
function drop(target: string) {
  const from = dragId.value
  dragId.value = overId.value = ''
  if (!from || from === target) return
  const ids = store.membersSorted.map((m) => m.id).filter((id) => id !== from)
  ids.splice(ids.indexOf(target), 0, from)
  void reorderTo(ids)
}
function move(id: string, d: number) {
  const ids = store.membersSorted.map((m) => m.id)
  const i = ids.indexOf(id)
  const j = i + d
  if (j < 0 || j >= ids.length) return
  ;[ids[i], ids[j]] = [ids[j], ids[i]]
  void reorderTo(ids)
}
async function remove(id: string) {
  try {
    await store.removeMember(id)
    open.delete(id)
  } catch {
    ElMessage.error(t('cat.groupChat.saveFailed'))
  }
}
</script>
