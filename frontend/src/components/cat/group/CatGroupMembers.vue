<template>
  <aside class="cg-mem" :aria-label="t('cat.groupChat.members')" data-testid="cat-group-members">
    <div class="cg-mem-hd">
      {{ t('cat.groupChat.membersCount', { n: store.members.length }) }}<span class="sp" />
      <button type="button" class="cg-ib" :title="t('cat.groupChat.addMember')" :aria-label="t('cat.groupChat.addMember')" @click="emit('add')"><FIcon name="plus" :size="15" /></button>
      <button v-if="closable" type="button" class="cg-ib" :aria-label="t('cat.groupChat.close')" @click="emit('close')"><FIcon name="x" :size="14" /></button>
    </div>
    <div class="cg-mem-list" role="list">
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
          <span class="av" :style="{ '--c': memberColorVar(m.order) }">{{ initial(m.name) }}</span>
          <span class="nm">
            <span class="a">
              {{ m.name }}
              <span v-if="i === 0" class="cg-badge acc" :title="t('cat.groupChat.firstTip')">{{ t('cat.groupChat.first') }}</span>
              <span v-if="m.charter.status === 'generating'" class="cg-spin" style="width: 10px; height: 10px; border-width: 1.5px" :title="t('cat.groupChat.generating')" />
              <span v-else-if="m.charter.status === 'failed'" :title="t('cat.groupChat.charterFailed')" style="display: inline-flex; color: var(--cat-warn-text)"><FIcon name="warn" :size="11" /></span>
            </span>
            <span class="b"><CatAgentMark :kind="normalizeProviderKind(m.agentKind)" :size="11" />{{ m.title }} · {{ m.modelId }}</span>
          </span>
          <span class="cg-state" :class="{ busy: !!store.live[m.id] }" :title="store.live[m.id] ? t('cat.groupChat.busy') : t('cat.groupChat.idle')" />
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
const toggle = (id: string) => (open.has(id) ? open.delete(id) : open.add(id))

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
