<template>
  <section v-if="active.length || history.length" class="cg-workflows" :aria-label="t('cat.groupChat.workflowTitle')" data-testid="cat-group-workflows">
    <div v-if="active.length" class="toolbar">
      <strong>{{ t('cat.groupChat.workflowActive', { count: active.length }) }}</strong>
      <button type="button" class="muted" :disabled="stopping" @click="stop">{{ stopping ? t('cat.groupChat.workflowStopping') : t('cat.groupChat.stopAll') }}</button>
    </div>
    <WorkflowCard v-for="w in active" :key="w.chainId" :workflow="w" />
    <details v-if="history.length" :key="store.activeId" class="history" data-testid="cat-group-workflow-history">
      <summary><span>{{ t('cat.groupChat.workflowHistory', { count: history.length }) }}</span></summary>
      <div class="history-help">
        <span>{{ t('cat.groupChat.workflowHistoryHint') }}</span>
        <button type="button" :disabled="clearing" @click="clearHistory">{{ t('cat.groupChat.workflowClearHistory') }}</button>
      </div>
      <WorkflowCard v-for="w in history" :key="w.chainId" :workflow="w" history />
    </details>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useCatGroupChat } from '@/stores/catGroupChat'
import { partitionWorkflows } from './groupWorkflowState'
import WorkflowCard from './CatGroupWorkflowCard.vue'

const { t } = useI18n()
const store = useCatGroupChat()
const stopping = ref(false)
const clearing = ref(false)
const partition = computed(() => partitionWorkflows(store.workflows))
const active = computed(() => partition.value.active)
const history = computed(() => partition.value.history)
async function stop() {
  stopping.value = true
  try { await store.stopAll() }
  catch (e) { ElMessage.error(String((e as Error)?.message || e)) }
  finally { stopping.value = false }
}
async function clearHistory() {
  clearing.value = true
  try { await store.clearWorkflowHistory() }
  catch (e) { ElMessage.error(String((e as Error)?.message || e)) }
  finally { clearing.value = false }
}
</script>

<style scoped>
.cg-workflows { flex: none; border-bottom: 1px solid var(--cat-divider); background: var(--cat-bg); max-height: 38%; overflow-y: auto; }
.toolbar { display: flex; justify-content: space-between; align-items: center; padding: 10px 20px; gap: 12px; font-size: 12px; }
strong { font-weight: 500; }
.history { border-bottom: 1px solid var(--cat-divider); color: var(--cat-muted); font-size: 12px; }
.history > summary { cursor: pointer; padding: 8px 20px; }
.history-help { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 10px; padding: 4px 20px 12px; }
button { border: 0; background: transparent; color: var(--cat-accent); cursor: pointer; font: inherit; padding: 0; }
button:disabled { opacity: .5; cursor: default; }
.muted { color: var(--cat-muted); }
</style>
