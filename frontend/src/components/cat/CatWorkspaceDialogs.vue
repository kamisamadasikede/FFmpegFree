<template>
  <el-dialog v-model="renameOpen" :title="t('cat.workspace.rename')" width="420px" append-to-body :close-on-click-modal="false" @opened="renameInput?.focus()">
    <el-input ref="renameInput" v-model="renameTitle" maxlength="120" show-word-limit :aria-label="t('cat.workspace.rename')" data-testid="cat-rename-title" @keydown.enter.prevent="rename" />
    <template #footer><el-button @click="renameOpen = false">{{ t('cat.workspace.cancel') }}</el-button><el-button type="primary" :disabled="!renameTitle.trim() || metadataBusy(workspaceUI.renameId, 'rename')" @click="rename">{{ t('cat.workspace.save') }}</el-button></template>
  </el-dialog>
  <el-dialog v-model="workspaceUI.editor" :title="t('cat.workspace.title')" width="min(720px, 90vw)" append-to-body :close-on-click-modal="false" data-testid="cat-workspace-dialog">
    <div class="ws-tabs"><button :class="{on: workspaceUI.tab === 'prompts'}" @click="workspaceUI.tab = 'prompts'">{{ t('cat.workspace.prompts') }}</button><button :class="{on: workspaceUI.tab === 'rules'}" @click="workspaceUI.tab = 'rules'">{{ t('cat.workspace.rules') }}</button></div>
    <template v-if="workspaceUI.tab === 'rules'">
      <p class="hint">{{ t('cat.workspace.rulesHint') }}</p>
      <el-select v-model="workspaceUI.projectId" :placeholder="t('cat.workspace.selectProject')" class="project" data-testid="cat-rules-project"><el-option v-for="p in catState.projects" :key="p.id" :label="p.name" :value="p.id" /></el-select>
      <template v-if="workspaceUI.projectId">
        <p v-if="loading" role="status" class="hint">{{ t('cat.workspace.loading') }}</p>
        <p v-else-if="rulesError" role="alert" class="hint">{{ rulesError }} <button @click="reloadRules">{{ t('cat.workspace.retry') }}</button></p>
        <el-input v-else v-model="rulesDraft" type="textarea" :rows="10" maxlength="8000" show-word-limit :aria-label="t('cat.workspace.rules')" data-testid="cat-project-rules" />
        <div class="footer"><span class="hint">{{ t('cat.workspace.nextTurn') }}</span><el-button type="primary" :disabled="loading || !!rulesError || savingRules.has(workspaceUI.projectId)" @click="saveRules">{{ t('cat.workspace.save') }}</el-button></div>
      </template>
    </template>
    <template v-else>
      <p class="hint">{{ t('cat.workspace.promptsHint') }}</p>
      <div v-if="editing" class="prompt-form">
        <el-input v-model="promptDraft.name" maxlength="60" :placeholder="t('cat.workspace.promptName')" :aria-label="t('cat.workspace.promptName')" data-testid="cat-prompt-name" />
        <el-input v-model="promptDraft.content" type="textarea" :rows="7" maxlength="16000" :placeholder="t('cat.workspace.promptContent')" :aria-label="t('cat.workspace.promptContent')" data-testid="cat-prompt-content" />
        <div class="footer"><el-button @click="editing = false">{{ t('cat.workspace.cancel') }}</el-button><el-button type="primary" :disabled="!promptDraft.name.trim() || !promptDraft.content.trim() || savingPrompts.has(promptDraft.id)" @click="putPrompt">{{ t('cat.workspace.save') }}</el-button></div>
      </div>
      <template v-else>
        <button class="add" data-testid="cat-prompt-add" @click="editPrompt()"><FIcon name="plus" :size="14" />{{ t('cat.workspace.addPrompt') }}</button>
        <p v-if="!workspaceUI.prompts.length" class="hint">{{ t('cat.workspace.emptyPrompts') }}</p>
        <div class="prompt-list"><div v-for="p in workspaceUI.prompts" :key="p.id" class="prompt-row"><div><b>{{ p.name }}</b><p>{{ p.content }}</p></div><button :aria-label="t('cat.workspace.edit')" @click="editPrompt(p)"><FIcon name="edit" :size="15" /></button><button :disabled="savingPrompts.has(p.id)" :aria-label="t('cat.workspace.remove')" @click="removePrompt(p.id)"><FIcon name="trash" :size="15" /></button></div></div>
      </template>
    </template>
  </el-dialog>
</template>
<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { catState, findConv, notify } from '@/views/cat/catState'
import { workspaceUI, changeConversation, metadataBusy, loadPromptLibrary } from '@/views/cat/catWorkspace'
import { getWorkspace, saveProjectRules, savePrompt, deletePrompt, type CatPrompt } from '@/api/catWorkspace'
import { toAppError } from '@/api/call'
const { t } = useI18n()
const renameOpen = computed({ get: () => !!workspaceUI.renameId, set: v => { if (!v) workspaceUI.renameId = '' } })
const renameTitle = ref(''), renameInput = ref<{focus:()=>void}>()
const renameDrafts = new Map<string, string>()
watch(() => workspaceUI.renameId, id => { renameTitle.value = renameDrafts.get(id) ?? findConv(id)?.conv.title ?? '' })
async function rename() {
  const id = workspaceUI.renameId, title = renameTitle.value.trim()
  if (!id || !title) return
  workspaceUI.renameId = ''
  if (await changeConversation(id, 'rename', title)) renameDrafts.delete(id)
  else renameDrafts.set(id, title)
}
const ruleDrafts = reactive<Record<string, string>>({})
const rulesDraft = computed({get: () => ruleDrafts[workspaceUI.projectId] ?? '', set: v => { ruleDrafts[workspaceUI.projectId] = v }})
const loading = ref(false), rulesError = ref(''), savingRules = reactive(new Set<string>()), savingPrompts = reactive(new Set<string>())
let loadSeq = 0
async function reloadRules() {
  const open = workspaceUI.editor, id = workspaceUI.projectId
  const seq = ++loadSeq
  rulesError.value = ''
  if (!open) { loading.value = false; return }
  void loadPromptLibrary()
  if (!id || ruleDrafts[id] !== undefined) { loading.value = false; return }
  loading.value = true
  try { const w = await getWorkspace(id); if (seq === loadSeq) ruleDrafts[id] = w.rules ?? '' }
  catch (e) { if (seq === loadSeq) rulesError.value = toAppError(e).message }
  finally { if (seq === loadSeq) loading.value = false }
}
watch(() => [workspaceUI.editor, workspaceUI.projectId] as const, reloadRules)
async function saveRules() {
  const id = workspaceUI.projectId, rules = rulesDraft.value
  if (!id || savingRules.has(id)) return
  savingRules.add(id); workspaceUI.editor = false
  try { await saveProjectRules(id, rules); notify(t('cat.workspace.saved'), 'ok') }
  catch (e) { ElMessage.warning(toAppError(e).message) }
  finally { savingRules.delete(id) }
}
const editing = ref(false), promptDraft = reactive<CatPrompt>({id: '', name: '', content: ''})
let promptEditTicket = 0
function editPrompt(p?: CatPrompt) { ++promptEditTicket; Object.assign(promptDraft, p ?? {id: '', name: '', content: ''}); editing.value = true }
async function putPrompt() {
  const p = {...promptDraft}
  const ticket = promptEditTicket
  if (savingPrompts.has(p.id)) return
  savingPrompts.add(p.id); editing.value = false
  try { await savePrompt(p); await loadPromptLibrary(); notify(t('cat.workspace.saved'), 'ok') }
  catch (e) { ElMessage.warning(toAppError(e).message); if (!editing.value && ticket === promptEditTicket) { Object.assign(promptDraft, p); editing.value = true } }
  finally { savingPrompts.delete(p.id) }
}
async function removePrompt(id: string) {
  if (savingPrompts.has(id)) return
  savingPrompts.add(id)
  try { await deletePrompt(id); await loadPromptLibrary() }
  catch (e) { ElMessage.warning(toAppError(e).message) }
  finally { savingPrompts.delete(id) }
}
</script>
<style scoped>
.ws-tabs { display:flex; gap:6px; padding:4px; background:var(--ff-bg-hover); border-radius:10px; margin-bottom:16px }
.ws-tabs button { flex:1; border:0; padding:9px; border-radius:7px; background:transparent; color:var(--ff-text-2); cursor:pointer }
.ws-tabs button.on { background:var(--ff-bg-surface); color:var(--ff-text-1); box-shadow:0 1px 5px #0001 }
.hint { color:var(--ff-text-3); font-size:12px; line-height:1.65 }.project { width:100%; margin-bottom:12px }.footer { display:flex; justify-content:flex-end; align-items:center; gap:10px; margin-top:16px }.footer .hint { margin-right:auto }
.prompt-form { display:grid; gap:12px }.add { display:flex; gap:6px; align-items:center; padding:8px 12px; border:1px solid var(--ff-border); border-radius:8px; background:var(--ff-bg-surface); color:var(--ff-text-1); cursor:pointer }
.prompt-list { max-height:350px; overflow:auto; margin-top:12px }.prompt-row { display:flex; align-items:center; gap:8px; padding:12px 0; border-bottom:1px solid var(--ff-border) }.prompt-row>div { flex:1; min-width:0 }.prompt-row b { font-size:13px }.prompt-row p { color:var(--ff-text-3); white-space:pre-wrap; margin:5px 0 0; display:-webkit-box; -webkit-line-clamp:2; -webkit-box-orient:vertical; overflow:hidden }.prompt-row button { background:transparent; border:0; color:var(--ff-text-2); cursor:pointer; padding:6px }
</style>
