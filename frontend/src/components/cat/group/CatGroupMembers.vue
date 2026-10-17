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
    <div v-if="tab === 'f'" class="cg-mem-list cg-files-tab" role="list" data-testid="cat-group-files">
      <div v-if="!store.files.length" class="cg-hint" style="padding: 12px">{{ t('cat.groupChat.filesTabEmpty') }}</div>
      <button
        v-for="f in store.files"
        :key="f.path"
        type="button"
        class="cg-gf"
        :class="{ gone: !f.exists }"
        role="listitem"
        :disabled="!f.exists"
        :title="f.path"
        data-testid="cat-group-file"
        @click="openFile(f.path)"
      >
        <FIcon name="file" :size="14" class="fi" />
        <span class="tx">
          <span class="a"><span class="dir">{{ dirMid(dirOf(f.path)) }}</span><span class="base">{{ baseOf(f.path) }}</span></span>
          <span class="b">{{ t('cat.groupChat.fileBy', { name: store.nameOf(f.lastMemberId), n: f.changeCount }) }}</span>
        </span>
        <span v-if="!f.exists" class="cg-tag unk">{{ t('cat.groupChat.fileMissing') }}</span>
        <span v-else-if="f.overreach" class="cg-tag over">{{ t('cat.groupChat.tagOverreach') }}</span>
      </button>
    </div>
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
            <!-- 产品 10-17：正在干活时只出一行「后面还有 {queueLength} 个在排队」；没在干活、自己在排队才出「排队中 · …」 -->
            <span v-if="stateOf(m.id) === 'busy'" class="s busy"><i class="cg-breath" />{{ t('cat.groupChat.busy') }}<template v-if="queueCount(m.id)"> · {{ t('cat.groupChat.queuedBehind', { n: queueCount(m.id) }) }}</template></span>
            <span v-else-if="stateOf(m.id) === 'queued' && store.queued[m.id]?.length" class="q">{{ queuedText(Math.min(...store.queued[m.id].map((q) => q.queuePosition))) }}</span>
          </span>
          <span class="cg-st" :class="stateOf(m.id)">
            <template v-if="stateOf(m.id) === 'busy'"><button type="button" class="cg-ib sm" :aria-label="t('cat.groupChat.stop')" @click.stop="store.stop(m.id)"><FIcon name="stop" :size="10" /></button></template>
            <template v-else-if="stateOf(m.id) === 'queued'" />
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
    <CatPreviewDialog v-model:open="previewOn" :file="previewFile" />
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
import CatPreviewDialog from '@/components/cat/CatPreviewDialog.vue'
import { previewKind, type CatShownFile } from '@/components/cat/catMedia'
import { catState } from '@/views/cat/catState'

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
// 走查截图用：?cat_group_tab=f 直接停在文件页（真实环境无此参数）
const tab = ref<'m' | 'f'>(new URLSearchParams(location.search).get('cat_group_tab') === 'f' ? 'f' : 'm')
const CLI: Record<string, string> = { claude_cli: 'Claude', codex_cli: 'Codex', cat_build: 'Grok Build', gemini_cli: 'Gemini' }
const cliName = (k: string) => CLI[k] ?? k
/** 排队文案：n=0 与 n≥1 用两个 key，产品改口径时只换 key（6.21.17 ④-4） */
const queuedText = (n: number) => (n === 0 ? t('cat.groupChat.queuedNext') : t('cat.groupChat.queued', { n }))
/** 排队总数：优先 queueLength（6.21.17），没有就数 queue[] */
const queueCount = (id: string) => store.queued[id]?.length || store.members.find((x) => x.id === id)?.queueLength || 0
function stateOf(id: string) {
  const st = store.memberStatus[id]
  if (store.live[id] || st === 'running') return 'busy'
  if (st === 'queued') return 'queued'
  const last = [...store.messages].reverse().find((x) => x.memberId === id)
  if (!last) return 'idle'
  if (last.systemKind === 'failed') return 'failed'
  if (last.systemKind === 'stopped') return 'stopped'
  if (last.report?.files.some((f) => f.tag === 'overreach') && !store.revertedSeqs?.includes(last.seq)) return 'over'
  return 'idle'
}

// 文件页签：项目根 + path 交给现有的预览弹窗（6.21.17 ①；exists=false 不可点）
const previewOn = ref(false)
const previewFile = ref<CatShownFile | null>(null)
const dirOf = (p: string) => p.slice(0, p.lastIndexOf('/') + 1)
/** 设计 走查 #335-3：长目录中间省略（保留首段和末段），文件名永远完整 */
function dirMid(d: string) {
  if (d.length <= 22) return d
  const seg = d.split('/').filter(Boolean)
  return seg.length > 2 ? `${seg[0]}/…/${seg[seg.length - 1]}/` : d
}
const baseOf = (p: string) => p.slice(p.lastIndexOf('/') + 1)
function openFile(rel: string) {
  const root = catState.projects.find((p) => p.id === store.group?.projectId)?.path ?? ''
  const sep = root.includes('\\') ? '\\' : '/'
  const abs = root ? `${root.replace(/[\\/]+$/, '')}${sep}${rel.split('/').join(sep)}` : rel
  const name = baseOf(rel)
  previewFile.value = { name, path: abs, kind: previewKind(name) }
  previewOn.value = true
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
