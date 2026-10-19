<template>
  <section v-if="visible.length" class="cg-workflows" :aria-label="t('cat.groupChat.workflowTitle')" data-testid="cat-group-workflows">
    <details v-for="w in visible" :key="w.chainId" class="workflow" data-testid="cat-group-workflow">
      <summary>
        <span class="indicator" :class="w.status" aria-hidden="true" />
        <strong>{{ phase(w) }}</strong>
        <span class="request">{{ w.request }}</span>
        <span class="more">{{ t('cat.groupChat.workflowDetails') }}</span>
      </summary>
      <div class="tasks">
        <div v-if="w.status === 'blocked'" class="help">{{ t('cat.groupChat.workflowHelp') }}</div>
        <ol>
          <li v-for="task in w.tasks" :key="task.id">
            <div class="row">
              <b>{{ task.title }}</b><span class="owner">{{ store.nameOf(task.memberId) }}</span>
              <span class="state" :class="task.status">{{ label(task.status) }}</span>
            </div>
            <p v-if="['blocked', 'failed', 'stopped'].includes(task.status) && task.result" class="reason">{{ task.result }}</p>
            <div class="actions">
              <button v-if="['blocked', 'failed', 'stopped'].includes(task.status)" type="button" :disabled="retrying.has(w.chainId + task.id)" @click="retry(w.chainId, task.memberId, task.id)">{{ t('cat.groupChat.retry') }}</button>
              <button v-if="task.turnId" type="button" @click="store.openProcess(task.memberId)">{{ t('cat.groupChat.viewProcess') }}</button>
              <select v-if="task.status !== 'done' && w.status !== 'stopped' && store.members.length > 1" :aria-label="t('cat.groupChat.reassignTo')" :disabled="retrying.has(w.chainId + task.id)" @change="reassign(w.chainId, task.id, $event)">
                <option value="">{{ t('cat.groupChat.reassign') }}</option>
                <option v-for="m in store.membersSorted.filter((x) => x.id !== task.memberId)" :key="m.id" :value="m.id">{{ m.name }}</option>
              </select>
            </div>
          </li>
        </ol>
        <button v-if="w.status !== 'stopped'" type="button" class="stop" @click="store.stopAll()">{{ t('cat.groupChat.stopAll') }}</button>
      </div>
    </details>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useCatGroupChat } from '@/stores/catGroupChat'
import type { CatGroupWorkflow as Workflow } from '@/api/catGroupChat'

const { t } = useI18n()
const store = useCatGroupChat()
const retrying = reactive(new Set<string>())
const visible = computed(() => store.workflows.filter((w, i) => ['running', 'blocked'].includes(w.status) || (w.status === 'stopped' && i === 0)))
const label = (status: string) => t(`cat.groupChat.workflow_${status}`)
function phase(w: Workflow) {
  if (w.status !== 'running') return label(w.status)
  const running = w.tasks.filter((task) => task.status === 'running')
  if (running.some((task) => task.kind === 'verify')) return t('cat.groupChat.workflowVerifying')
  if (running.some((task) => task.kind === 'integrate')) return t('cat.groupChat.workflowIntegrating')
  return t('cat.groupChat.workflowDeveloping')
}
async function retry(chainId: string, memberId: string, taskId: string) {
  const key = chainId + taskId
  retrying.add(key)
  try { await store.retryTurn(memberId, chainId, taskId) }
  catch (e) { ElMessage.error(String((e as Error)?.message || e)) }
  finally { retrying.delete(key) }
}
async function reassign(chainId: string, taskId: string, event: Event) {
  const select = event.target as HTMLSelectElement
  const memberId = select.value
  select.value = ''
  if (!memberId) return
  const key = chainId + taskId
  retrying.add(key)
  try { await store.reassign(chainId, memberId, taskId) }
  catch (e) { ElMessage.error(String((e as Error)?.message || e)) }
  finally { retrying.delete(key) }
}
</script>

<style scoped>
.cg-workflows { flex: none; border-bottom: 1px solid var(--cat-divider); background: var(--cat-bg); max-height: 38%; overflow-y: auto; }
.workflow + .workflow { border-top: 1px solid var(--cat-divider); }
summary { display: flex; align-items: center; gap: 10px; padding: 12px 20px; cursor: pointer; font-size: 12px; list-style: none; }
summary::-webkit-details-marker { display: none; }
strong { font-size: 12px; font-weight: 500; white-space: nowrap; }
.indicator { width: 6px; height: 6px; border-radius: 50%; background: var(--cat-accent); flex: none; }
.indicator.blocked { background: var(--cat-orange); }
.indicator.stopped { background: var(--cat-muted); }
.request { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--cat-muted); }
.more { flex: none; color: var(--cat-accent); }
.tasks { padding: 0 20px 14px; font-size: 12px; }
.help { margin-bottom: 12px; color: var(--cat-muted); }
ol { list-style: none; margin: 0; padding: 0; }
li { padding: 10px 0; border-top: 1px solid var(--cat-divider); }
.row { display: flex; flex-wrap: wrap; align-items: baseline; gap: 8px; }
.row b { font-weight: 500; }
.owner { color: var(--cat-muted); }
.state { margin-left: auto; color: var(--cat-muted); }
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
.stop { color: var(--cat-muted); margin-top: 6px; }
</style>
