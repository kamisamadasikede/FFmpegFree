import { reactive } from 'vue'
import { ElMessage } from 'element-plus'
import { t } from '@/i18n'
import { toAppError } from '@/api/call'
import { updateConversation, exportConversation, getWorkspace, type CatPrompt, type ConvAction } from '@/api/catWorkspace'
import { catState, findConv, notify, NEW_CONV, markConvMetadataChanged } from './catState'

export const workspaceUI = reactive({ editor: false, tab: 'prompts' as 'prompts' | 'rules', projectId: '', renameId: '', prompts: [] as CatPrompt[] })
export function openWorkspace(tab: 'prompts' | 'rules', projectId = '') { workspaceUI.tab = tab; workspaceUI.projectId = projectId; workspaceUI.editor = true }
let promptSeq = 0
export async function loadPromptLibrary() {
  const seq = ++promptSeq
  try { const w = await getWorkspace(''); if (seq === promptSeq) workspaceUI.prompts = w.prompts ?? [] }
  catch (e) { if (seq === promptSeq) ElMessage.warning(toAppError(e).message) }
}
const pending = reactive(new Set<string>())
export const metadataBusy = (id: string, action: string) => pending.has(id + ':' + action)
export async function changeConversation(id: string, action: ConvAction, title = '', enabled = false) {
  const key = id + ':' + action
  if (pending.has(key)) return false
  pending.add(key)
  try {
    const c = await updateConversation(id, action, title, enabled)
    const found = findConv(id)
    if (!found) return true
    // Apply only the field changed by this request; independent late replies do not overwrite one another.
    if (action === 'rename' || action === 'reset_title') { found.conv.title = c.title; found.conv.customTitle = c.customTitle }
    if (action === 'pin') found.conv.pinned = c.pinned
    if (action === 'archive') {
      found.conv.archived = c.archived
      if (c.archived && catState.sel === id) catState.sel = NEW_CONV
    }
    markConvMetadataChanged()
    notify(t('cat.workspace.saved'), 'ok')
    return true
  } catch (e) { ElMessage.warning(toAppError(e).message); return false }
  finally { pending.delete(key) }
}
export async function exportChat(id: string, format: 'md' | 'json') {
  const key = id + ':export'
  if (pending.has(key)) return
  pending.add(key)
  try { if (await exportConversation(id, format)) notify(t('cat.workspace.exported'), 'ok') }
  catch (e) { ElMessage.warning(toAppError(e).message) }
  finally { pending.delete(key) }
}
