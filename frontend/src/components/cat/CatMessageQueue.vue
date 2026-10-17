<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import FIcon from '@/components/icon/FIcon.vue'
import { updateQueue, type QueueAction, type QueuedMessage } from '@/api/catWorkflow'
import { acceptQueue, catWorkflow, liveSteerReady } from '@/views/cat/catWorkflow'
import { catState, notify, openQueuedMessageInNewChat } from '@/views/cat/catState'
import { catDraftFor } from '@/views/cat/catDrafts'
import { toAppError } from '@/api/call'
const props = defineProps<{ convId: string }>()
const { t } = useI18n()
const q = computed(() => catWorkflow.queues[props.convId])
const root = ref<HTMLElement>(), menu = ref<HTMLElement>()
const running = computed(() => !!catState.turns[props.convId])
const canSteer = computed(() => liveSteerReady(props.convId, catState.turns[props.convId]?.turnId))
const submitLabel = computed(() => t(canSteer.value ? 'cat.workflow.steer' : running.value ? 'cat.workflow.guide' : 'cat.workflow.sendNow'))
const editing = ref(''), menuId = ref(''), busy = ref(new Set<string>()), dragged = ref(''), dropId = ref('')
const menuStyle = ref({ top: '0px', left: '0px' })
let menuAnchor: HTMLElement | undefined
const menuItem = computed(() => q.value?.items.find(item => item.id === menuId.value))
const editKey = (id: string) => JSON.stringify(['queued-message', props.convId, id])
const editDraft = (id: string) => catDraftFor(editKey(id))
async function act(action: QueueAction, id = '', content = '', beforeId = ''): Promise<boolean> {
  const convId = props.convId, key = action + id
  if (busy.value.has(key)) return false
  busy.value.add(key)
  try { acceptQueue(await updateQueue(convId, action, id, content, beforeId)); return true }
  catch(e) { notify(toAppError(e).message, 'warn'); return false }
  finally { busy.value.delete(key) }
}
async function beginEdit(item: QueuedMessage) {
  closeMenu(false)
  const convId = props.convId
  if (!item.held && !await act('edit-start', item.id)) return
  if (props.convId !== convId || !q.value?.items.some(v => v.id === item.id)) return
  const draft = editDraft(item.id)
  if (!item.held || !draft.text) draft.text = item.request.content
  editing.value = item.id
  await nextTick()
  const textarea = root.value?.querySelector<HTMLTextAreaElement>('textarea')
  if (textarea) { resize(textarea); textarea.focus(); textarea.setSelectionRange(textarea.value.length, textarea.value.length) }
}
async function finishEdit(item: QueuedMessage, submit = false) {
  const key = editKey(item.id), convId = props.convId
  const action = submit ? (canSteer.value ? 'edit-steer' : running.value ? 'edit' : 'edit-send') : 'edit'
  if (await act(action, item.id, editDraft(item.id).text)) {
    if (submit && running.value && !canSteer.value) await act('guide', item.id)
    catDraftFor(key).text = ''
    if (props.convId === convId) editing.value = ''
  }
}
async function cancelEdit(item: QueuedMessage) {
  const key = editKey(item.id), convId = props.convId
  if (await act('edit-cancel', item.id)) { catDraftFor(key).text = ''; if (props.convId === convId) editing.value = '' }
}
function editKeys(event: KeyboardEvent, item: QueuedMessage) {
  if (event.isComposing || event.keyCode === 229 || event.repeat) return
  if (event.key === 'Escape') { event.preventDefault(); void cancelEdit(item) }
  if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void finishEdit(item, event.ctrlKey || event.metaKey) }
}
function resize(el: HTMLTextAreaElement) { el.style.height = 'auto'; el.style.height = `${Math.min(144, Math.max(40, el.scrollHeight))}px` }
async function submit(item: QueuedMessage) { closeMenu(false); await act(canSteer.value ? 'steer' : running.value ? 'guide' : 'send', item.id) }
async function remove(item: QueuedMessage) { closeMenu(false); const key = editKey(item.id); if (await act('delete', item.id)) catDraftFor(key).text = '' }
async function newChat(item: QueuedMessage) { closeMenu(false); try { await openQueuedMessageInNewChat(item.request) } catch(e) { notify(toAppError(e).message, 'warn') } }
async function toggleMenu(event: MouseEvent, item: QueuedMessage) {
  if (menuId.value === item.id) { closeMenu(); return }
  menuAnchor = event.currentTarget as HTMLElement; menuId.value = item.id
  const rect = menuAnchor.getBoundingClientRect()
  menuStyle.value = { top: `${rect.bottom + 5}px`, left: `${Math.max(8, rect.right - 214)}px` }
  await nextTick()
  const menuRect = menu.value?.getBoundingClientRect()
  if (menuRect && menuRect.bottom > window.innerHeight - 8) menuStyle.value.top = `${Math.max(8, rect.top - menuRect.height - 5)}px`
  menu.value?.querySelector<HTMLButtonElement>('button')?.focus()
}
function closeMenu(focus = true) { menuId.value = ''; if (focus) menuAnchor?.focus() }
function outside(event: PointerEvent) { if (!menu.value?.contains(event.target as Node) && !menuAnchor?.contains(event.target as Node)) closeMenu(false) }
function menuKeys(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); closeMenu(); return }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const buttons = Array.from(menu.value!.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')), index = buttons.indexOf(document.activeElement as HTMLButtonElement)
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length
  buttons[next]?.focus()
}
watch(menuId, value => { if (value) document.addEventListener('pointerdown', outside); else document.removeEventListener('pointerdown', outside) })
watch(() => props.convId, () => { closeMenu(false); editing.value = ''; dragged.value = dropId.value = '' })
watch(() => q.value?.items.map(item => item.id).join(','), () => {
  if (editing.value && !q.value?.items.some(v => v.id === editing.value)) editing.value = ''
  if (menuId.value && !menuItem.value) closeMenu(false)
})
onBeforeUnmount(() => document.removeEventListener('pointerdown', outside))
function dragStart(event: DragEvent, id: string) { dragged.value = id; event.dataTransfer?.setData('text/plain', id); if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'; closeMenu(false) }
async function drop(beforeId: string) { const id = dragged.value; dragged.value = dropId.value = ''; if (id && id !== beforeId) await act('move', id, '', beforeId) }
async function move(item: QueuedMessage, offset: number) {
  const items = q.value?.items || [], at = items.findIndex(v => v.id === item.id)
  if (at < 0 || at + offset < 0 || at + offset >= items.length) return
  await act('move', item.id, '', offset < 0 ? items[at - 1].id : items[at + 2]?.id || '')
}
</script>
<template>
  <section v-if="q?.items.length || q?.queueingDisabled" ref="root" class="mq" data-testid="cat-message-queue" :aria-label="t('cat.workflow.queue')">
    <div v-if="q.paused || q.queueingDisabled || q.error" class="queue-status" role="status"><FIcon :name="q.queueingDisabled ? 'list' : 'pause'" :size="12" /><span>{{ q.error || t(q.queueingDisabled ? 'cat.workflow.queueOff' : 'cat.workflow.paused') }}</span><button type="button" @click="act(q.queueingDisabled ? 'queue-on' : 'resume')">{{ t(q.queueingDisabled ? 'cat.workflow.enableQueue' : 'cat.workflow.resume') }}</button></div>
    <div class="queue-items" @dragover.prevent @drop.stop.prevent="drop('')">
      <div v-for="(item, i) in q.items" :key="item.id" class="item" :class="{ editing: editing === item.id, dragging: dragged === item.id, 'drop-before': dropId === item.id }" data-testid="cat-queued-message" @dragover.stop.prevent="dropId = item.id" @drop.stop.prevent="drop(item.id)">
        <button class="handle icon" type="button" :draggable="editing !== item.id" :aria-label="t('cat.workflow.reorder')" :title="t('cat.workflow.reorderHelp')" @dragstart="dragStart($event, item.id)" @dragend="dragged = dropId = ''" @keydown.alt.up.prevent="move(item, -1)" @keydown.alt.down.prevent="move(item, 1)"><FIcon :name="i ? 'queued' : 'grip'" :size="14" /></button>
        <div class="body">
          <template v-if="editing === item.id"><textarea v-model="editDraft(item.id).text" rows="1" :aria-label="t('cat.workflow.edit')" @input="resize($event.target as HTMLTextAreaElement)" @keydown="editKeys($event, item)" /><div class="edit-actions"><small>{{ t('cat.workflow.editKeys') }}</small><button type="button" @click="cancelEdit(item)">{{ t('cat.workflow.cancel') }}</button><button type="button" @click="finishEdit(item)">{{ t('cat.workflow.save') }}</button><button type="button" class="submit" @click="finishEdit(item, true)"><FIcon name="send" :size="12" />{{ submitLabel }}</button></div></template>
          <template v-else><button type="button" class="message" :title="item.request.content" @dblclick="beginEdit(item)">{{ item.request.content || t('cat.workflow.attachments') }}</button><small v-if="item.request.attachments?.length" class="attachments">{{ item.request.attachments.map(f => f.name).join(' · ') }}</small><small v-if="item.held" class="held">{{ t('cat.workflow.editingHeld') }}</small></template>
        </div>
        <div v-if="editing !== item.id" class="actions"><button type="button" class="quick" :aria-label="submitLabel" :title="t(canSteer ? 'cat.workflow.steerHelp' : running ? 'cat.workflow.guideHelp' : 'cat.workflow.sendHelp')" data-testid="cat-queue-submit" @click="submit(item)"><FIcon :name="canSteer || running ? 'steer' : 'send'" :size="13" /><span>{{ submitLabel }}</span></button><button type="button" class="icon" :aria-label="t('cat.workflow.remove')" :title="t('cat.workflow.remove')" @click="remove(item)"><FIcon name="trash" :size="14" /></button><button type="button" class="icon" aria-haspopup="menu" :aria-expanded="menuId === item.id" :aria-label="t('cat.workflow.more')" :title="t('cat.workflow.more')" data-testid="cat-queue-more" @click="toggleMenu($event, item)"><FIcon name="more" :size="16" /></button></div>
      </div>
    </div>
    <span class="sr-only">{{ t('cat.workflow.waiting', { n: q.items.length }) }} · {{ t(q.paused ? 'cat.workflow.paused' : 'cat.workflow.autoSend') }}</span>
    <Teleport to="body"><div v-if="menuItem" ref="menu" class="mq-menu cat-root" :style="menuStyle" role="menu" :aria-label="t('cat.workflow.more')" data-testid="cat-queue-menu" @keydown="menuKeys"><button type="button" role="menuitem" @click="beginEdit(menuItem)"><FIcon name="edit" :size="14" />{{ t('cat.workflow.edit') }}</button><button type="button" role="menuitem" @click="newChat(menuItem)"><FIcon name="chat" :size="14" />{{ t('cat.workflow.newChat') }}</button><button type="button" role="menuitem" @click="move(menuItem, -1)" :disabled="q.items[0]?.id === menuItem.id"><FIcon name="up" :size="14" />{{ t('cat.workflow.moveUp') }}</button><button type="button" role="menuitem" @click="act(q.paused ? 'resume' : 'pause'); closeMenu()"><FIcon :name="q.paused ? 'play' : 'pause'" :size="14" />{{ t(q.paused ? 'cat.workflow.resume' : 'cat.workflow.pause') }}</button><div class="separator" role="separator" /><button type="button" role="menuitem" @click="act(q.queueingDisabled ? 'queue-on' : 'queue-off'); closeMenu()"><FIcon name="queued" :size="14" />{{ t(q.queueingDisabled ? 'cat.workflow.enableQueue' : 'cat.workflow.disableQueue') }}</button></div></Teleport>
  </section>
</template>
<style scoped>
.mq { flex: none; width: calc(100% - 96px); box-sizing: border-box; min-width: 0; margin: 8px auto 6px; border: 1px solid var(--cat-border); border-radius: 16px; background: var(--cat-surface); font-size: 12px; }
.queue-items { max-height: 220px; overflow: auto; border-radius: inherit; padding: 4px 0; }
.item { display: flex; align-items: flex-start; gap: 5px; padding: 3px 9px; position: relative; min-height: 30px; box-sizing: border-box; }
.item.dragging { opacity: .35; }.item.drop-before::before { content: ''; position: absolute; top: 0; left: 12px; right: 12px; border-top: 2px solid var(--cat-accent); }
.mq button, .mq-menu button { border: 0; background: transparent; color: var(--cat-muted); cursor: pointer; font: inherit; border-radius: 6px; padding: 4px 6px; display: inline-flex; gap: 5px; align-items: center; }
.mq button:hover, .mq-menu button:hover:not(:disabled) { background: var(--cat-hover); color: var(--cat-text); }
.mq button:focus-visible, .mq-menu button:focus-visible, textarea:focus-visible { outline: 2px solid var(--cat-accent); outline-offset: -2px; }
.mq .handle { padding: 5px 0; flex: none; width: 16px; justify-content: center; cursor: grab; color: var(--cat-muted); opacity: .7; }
.body { flex: 1; min-width: 0; padding: 0 2px; }
.mq .message { display: block; text-align: left; width: 100%; color: var(--cat-text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; line-height: 22px; padding: 0 2px; min-height: 24px; cursor: text; }
.body small { color: var(--cat-muted); font-size: 11px; }.attachments { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.held { padding-left: 2px; }
.actions { flex: none; display: flex; align-items: center; gap: 2px; opacity: .65; }.item:hover .actions, .item:focus-within .actions { opacity: 1; }
.mq .icon { justify-content: center; width: 25px; height: 25px; padding: 0; }.quick { min-height: 25px; }
.body textarea { display: block; box-sizing: border-box; width: 100%; min-height: 40px; max-height: 144px; resize: none; background: var(--cat-bg); color: var(--cat-text); border: 1px solid var(--cat-border); padding: 7px 9px; border-radius: 8px; font: inherit; line-height: 1.6; }
.item.editing { padding: 7px 9px; }.edit-actions { display: flex; align-items: center; gap: 4px; padding-top: 4px; flex-wrap: wrap; }.edit-actions small { flex: 1; }.mq .submit { color: var(--cat-text); }
.queue-status { display: flex; align-items: center; gap: 6px; padding: 6px 12px 2px; color: var(--cat-muted); font-size: 11px; }.queue-status span { flex: 1; min-width: 0; }.queue-status button { flex: none; }
.mq-menu { position: fixed; z-index: 3100; width: 214px; padding: 5px; box-sizing: border-box; background: var(--cat-surface); border: 1px solid var(--cat-border); border-radius: 12px; box-shadow: 0 5px 22px #00000018; font-size: 12px; }
.mq-menu button { width: 100%; padding: 8px; text-align: left; color: var(--cat-text-2); }.mq-menu button:disabled { opacity: .4; cursor: default; }.separator { border-top: 1px solid var(--cat-border); margin: 4px; }
@media (max-width: 700px) { .quick span { display: none; }.edit-actions small { flex-basis: 100%; } }
</style>
