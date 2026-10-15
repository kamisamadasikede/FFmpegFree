<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import FIcon from '@/components/icon/FIcon.vue'
import { getChangeDetail, type TurnChanges } from '@/api/catWorkflow'
import { toAppError } from '@/api/call'
import { requestWorkspaceFile } from '@/views/cat/catWorkflow'
const props = defineProps<{ change: TurnChanges; timestamp?: boolean }>()
const { t } = useI18n()
const opened = ref(false), diffPath = ref(''), detail = ref<TurnChanges | null>(null), loading = ref(false), error = ref('')
const files = computed(() => detail.value?.files || props.change.files)
async function toggle() {
  opened.value = !opened.value
  if (!opened.value) return
  if (props.change.files.length) { detail.value = props.change; return }
  if (detail.value || !props.change.fileCount || !props.change.finished || loading.value) return
  loading.value = true; error.value = ''
  try { detail.value = await getChangeDetail(props.change.convId, props.change.turnId) }
  catch(e) { error.value = toAppError(e).message }
  finally { loading.value = false }
}
</script>
<template>
  <div class="turn-change" data-testid="cat-turn-summary">
    <button type="button" class="summary" :aria-expanded="opened" @click="toggle"><FIcon :name="opened ? 'down' : 'right'" :size="12" /><FIcon name="file" :size="13" /><span>{{ t('cat.workflow.changed', { n: change.fileCount }) }}</span><b class="add">+{{ change.added }}</b><b class="del">−{{ change.deleted }}</b><small v-if="!change.finished">{{ t('cat.workflow.tracking') }}</small><small v-if="timestamp">{{ new Date(change.createdAt).toLocaleTimeString() }}</small></button>
    <div v-if="opened" class="details"><p class="hint">{{ t('cat.workflow.changeHelp') }}</p><p v-if="change.truncated" class="hint">{{ t('cat.workflow.limited') }}</p><p v-if="loading" class="hint">{{ t('cat.preview.loading') }}</p><p v-if="error" class="hint" role="status">{{ error }}</p><div v-for="file in files" :key="file.path" class="file"><div class="file-row"><button type="button" class="path" :disabled="file.status === 'deleted'" @click="requestWorkspaceFile(change.convId, file.path)"><span class="status">{{ file.status === 'added' ? 'A' : file.status === 'deleted' ? 'D' : 'M' }}</span>{{ file.path }}</button><span v-if="file.binary" class="hint">{{ t('cat.workflow.binary') }}</span><template v-else><b class="add">+{{ file.added }}</b><b class="del">−{{ file.deleted }}</b><button v-if="file.diff" type="button" :aria-expanded="diffPath === file.path" @click="diffPath = diffPath === file.path ? '' : file.path">{{ t('cat.workflow.diff') }}</button></template></div><div v-if="diffPath === file.path" class="diff"><pre v-for="(line, i) in (file.diff || '').split('\n')" :key="i" :class="{ add: line.startsWith('+'), del: line.startsWith('-') }">{{ line }}</pre><p v-if="file.truncated" class="hint">{{ t('cat.workflow.limited') }}</p></div></div></div>
  </div>
</template>
<style scoped>
.turn-change{font-size:12px;margin:6px 0 12px;min-width:0}.summary{display:flex;gap:8px;align-items:center;width:fit-content;border:1px solid var(--cat-border);border-radius:8px;background:transparent;color:var(--cat-text-2);padding:7px 10px;cursor:pointer;font:inherit}.summary:hover{background:var(--cat-hover)}b{font-weight:500;font-variant-numeric:tabular-nums}.add{color:#23855a}.del{color:#ca5560}.summary small,.hint{color:var(--cat-muted);font-size:11px}.details{border:1px solid var(--cat-border);border-radius:8px;padding:8px 12px;margin-top:6px}.hint{margin:3px 0 8px}.file-row{display:flex;gap:10px;align-items:center;padding:5px 0}.file-row button{background:transparent;border:0;color:var(--cat-text-2);cursor:pointer;font:inherit}.file-row button:disabled{cursor:default}.path{flex:1;text-align:left;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.status{color:var(--cat-muted);margin-right:8px}.diff{overflow:auto;max-height:300px;background:var(--cat-bg);border-radius:6px;padding:8px}.diff pre{margin:0;font:11px/1.6 var(--font-mono,monospace);white-space:pre}.diff .add{background:#23855a0c}.diff .del{background:#ca55600c}
</style>
