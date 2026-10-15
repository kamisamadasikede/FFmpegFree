<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import FIcon from '@/components/icon/FIcon.vue'
import { updateQueue } from '@/api/catWorkflow'
import { acceptQueue, catWorkflow } from '@/views/cat/catWorkflow'
import { notify } from '@/views/cat/catState'
import { toAppError } from '@/api/call'
const props = defineProps<{ convId: string }>()
const { t } = useI18n()
const q = computed(() => catWorkflow.queues[props.convId])
const editing = ref(''), text = ref(''), busy = ref(new Set<string>())
async function act(action: 'pause'|'resume'|'edit'|'delete'|'guide', id = '', content = '') {
  const convId = props.convId, key = action + id
  if (busy.value.has(key)) return
  busy.value.add(key)
  try { acceptQueue(await updateQueue(convId, action, id, content)); if (props.convId === convId && action === 'edit') editing.value = '' }
  catch(e) { notify(toAppError(e).message, 'warn') }
  finally { busy.value.delete(key) }
}
</script>
<template>
  <section v-if="q?.items.length" class="mq" data-testid="cat-message-queue" :aria-label="t('cat.workflow.queue')">
    <header><FIcon name="clock" :size="14" /><b>{{ t('cat.workflow.waiting', { n: q.items.length }) }}</b><span>{{ t(q.paused ? 'cat.workflow.paused' : 'cat.workflow.autoSend') }}</span><button type="button" @click="act(q.paused ? 'resume' : 'pause')">{{ t(q.paused ? 'cat.workflow.resume' : 'cat.workflow.pause') }}</button></header>
    <p v-if="q.error" class="error" role="status">{{ q.error }}</p>
    <div v-for="(item, i) in q.items" :key="item.id" class="item" data-testid="cat-queued-message">
      <span class="num">{{ i + 1 }}</span>
      <div class="body"><template v-if="editing === item.id"><textarea v-model="text" :aria-label="t('cat.workflow.edit')" /><div class="edit-actions"><button type="button" @click="act('edit', item.id, text)">{{ t('cat.workflow.save') }}</button><button type="button" @click="editing = ''">{{ t('cat.workflow.cancel') }}</button></div></template><template v-else><p>{{ item.request.content || t('cat.workflow.attachments') }}</p><small v-if="item.request.attachments?.length">{{ item.request.attachments.map(f => f.name).join(' · ') }}</small></template></div>
      <div v-if="editing !== item.id" class="actions"><button type="button" :title="t('cat.workflow.guideHelp')" @click="act('guide', item.id)">{{ t('cat.workflow.guide') }}</button><button type="button" :aria-label="t('cat.workflow.edit')" @click="editing = item.id; text = item.request.content"><FIcon name="edit" :size="13" /></button><button type="button" :aria-label="t('cat.workflow.remove')" @click="act('delete', item.id)"><FIcon name="x" :size="13" /></button></div>
    </div>
  </section>
</template>
<style scoped>
.mq{margin:8px 18px 0;border:1px solid var(--cat-border);border-radius:12px;background:var(--cat-surface);font-size:12px;max-height:240px;overflow:auto}.mq header{display:flex;align-items:center;gap:8px;padding:9px 12px;position:sticky;top:0;background:var(--cat-surface);z-index:1}.mq header b{font-weight:550}.mq header span{color:var(--cat-muted);flex:1}.mq button{border:0;background:transparent;color:var(--cat-text-2);cursor:pointer;font:inherit;border-radius:6px;padding:4px 6px}.mq button:hover{background:var(--cat-hover);color:var(--cat-text)}.item{display:flex;gap:10px;padding:9px 12px;border-top:1px solid var(--cat-border)}.num{color:var(--cat-muted);font-variant-numeric:tabular-nums}.body{flex:1;min-width:0}.body p{margin:0;white-space:pre-wrap;max-height:60px;overflow:auto;line-height:1.55;overflow-wrap:anywhere}.body small{color:var(--cat-muted)}.actions,.edit-actions{display:flex;align-items:flex-start;gap:4px}.body textarea{width:100%;min-height:70px;resize:vertical;background:var(--cat-bg);color:var(--cat-text);border:1px solid var(--cat-border);padding:8px;border-radius:6px;font:inherit}.error{margin:0;padding:0 12px 9px;color:var(--cat-muted)}
</style>
