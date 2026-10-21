<template>
  <details class="workflow" :data-testid="history ? 'cat-group-workflow-record' : 'cat-group-workflow'">
    <summary>
      <span class="indicator" :class="history ? 'settled' : ''" aria-hidden="true" />
      <strong>{{ phase }}</strong><span class="request">{{ workflow.request }}</span>
      <span class="more">{{ t('cat.groupChat.workflowDetails') }}</span>
      <button v-if="history" type="button" class="remove" :disabled="busy" @click.stop.prevent="dismiss">{{ t('cat.groupChat.workflowRemove') }}</button>
    </summary>
    <div class="tasks">
      <p v-if="history && workflow.status !== 'completed'" class="help">{{ t('cat.groupChat.workflowHistoryHelp') }}</p>
      <ol>
        <li v-for="task in workflow.tasks" :key="task.id">
          <div class="row">
            <b>{{ task.title }}</b><span class="owner">{{ store.nameOf(task.memberId) }}</span>
            <span class="state" :class="task.status">{{ task.retryAt && task.status === 'queued' ? t('cat.groupChat.workflowRetrying') : label(task.status) }}</span>
          </div>
          <p v-if="task.result && (history || ['blocked', 'failed', 'stopped'].includes(task.status) || task.retryAt)" class="reason">{{ task.result }}</p>
          <div class="actions">
            <button v-if="['blocked', 'failed', 'stopped'].includes(task.status)" type="button" :disabled="busy" @click="retry(task.memberId, task.id)">{{ t('cat.groupChat.workflowContinue') }}</button>
            <button v-if="task.turnId && task.status === 'running'" type="button" @click="store.openProcess(task.memberId)">{{ t('cat.groupChat.viewProcess') }}</button>
            <select v-if="!history && !['done', 'stopped'].includes(task.status) && store.members.length > 1" :aria-label="t('cat.groupChat.reassignTo')" :disabled="busy" @change="reassign(task.id, $event)">
              <option value="">{{ t('cat.groupChat.reassign') }}</option>
              <option v-for="m in store.membersSorted.filter((x) => x.id !== task.memberId)" :key="m.id" :value="m.id">{{ m.name }}</option>
            </select>
          </div>
        </li>
      </ol>
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useCatGroupChat } from '@/stores/catGroupChat'
import type { CatGroupWorkflow } from '@/api/catGroupChat'
const props = defineProps<{ workflow: CatGroupWorkflow; history?: boolean }>()
const { t } = useI18n()
const store = useCatGroupChat()
const busy = ref(false)
const label = (status: string) => t(`cat.groupChat.workflow_${status}`)
const phase = computed(() => {
  if (props.history) return label(props.workflow.status)
  const running = props.workflow.tasks.filter((task) => task.status === 'running')
  if (running.some((task) => task.kind === 'verify')) return t('cat.groupChat.workflowVerifying')
  if (running.some((task) => task.kind === 'integrate')) return t('cat.groupChat.workflowIntegrating')
  if (!running.length && props.workflow.tasks.some((task) => task.status === 'queued' && task.retryAt)) return t('cat.groupChat.workflowRetrying')
  return t('cat.groupChat.workflowDeveloping')
})
async function act(fn: () => Promise<unknown>) {
  if (busy.value) return
  busy.value = true
  try { await fn() }
  catch (e) { ElMessage.error(String((e as Error)?.message || e)) }
  finally { busy.value = false }
}
const retry = (memberId: string, taskId: string) => act(() => store.retryTurn(memberId, props.workflow.chainId, taskId))
const dismiss = () => act(() => store.dismissWorkflow(props.workflow.chainId))
async function reassign(taskId: string, event: Event) {
  const select = event.target as HTMLSelectElement
  const memberId = select.value
  select.value = ''
  if (memberId) await act(() => store.reassign(props.workflow.chainId, memberId, taskId))
}
</script>

<style scoped>
.workflow + .workflow { border-top: 1px solid var(--cat-divider); }
summary { display: flex; align-items: center; gap: 10px; padding: 10px 20px; cursor: pointer; font-size: 12px; list-style: none; }
summary::-webkit-details-marker { display: none; }
strong { font-weight: 500; white-space: nowrap; }
.indicator { width: 6px; height: 6px; border-radius: 50%; background: var(--cat-accent); flex: none; }
.indicator.settled { background: var(--cat-muted); }
.request { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--cat-muted); }
.more { flex: none; color: var(--cat-accent); }
.tasks { padding: 0 20px 14px; font-size: 12px; }
.help { color: var(--cat-muted); margin-bottom: 12px; }
ol { list-style: none; margin: 0; padding: 0; }
li { padding: 10px 0; border-top: 1px solid var(--cat-divider); }
.row { display: flex; flex-wrap: wrap; align-items: baseline; gap: 8px; }
.row b { font-weight: 500; }
.owner, .state { color: var(--cat-muted); }
.state { margin-left: auto; }
.state.running { color: var(--cat-accent); }
.state.done { color: var(--cat-success-text); }
.state.failed, .state.blocked { color: var(--cat-orange); }
p { margin: 5px 0; white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.6; }
.actions { display: flex; gap: 12px; margin-top: 6px; }
.actions:empty { display: none; }
button, select { border: 0; background: transparent; color: var(--cat-accent); cursor: pointer; font: inherit; padding: 0; }
select { max-width: 150px; }
option { background: var(--cat-bg); color: var(--cat-text); }
button:disabled, select:disabled { opacity: .5; cursor: default; }
.remove { color: var(--cat-muted); flex: none; }
</style>
