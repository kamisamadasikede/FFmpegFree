<template>
  <div class="shared-ext" data-testid="cat-shared">
    <div class="se-heading">
      <div><h3>{{ t('cat.shared.title') }}</h3><p>{{ t('cat.shared.subtitle') }}</p></div>
      <button type="button" :disabled="isBusy('sync')" @click="sync"><FIcon name="refresh" :size="14" />{{ t(isBusy('sync') ? 'cat.shared.syncing' : 'cat.shared.sync') }}</button>
    </div>
    <div class="se-scopes" role="group" :aria-label="t('cat.shared.scope')">
      <button type="button" :class="{ selected: scope === 'global' }" :aria-pressed="scope === 'global'" @click="scope = 'global'">{{ t('cat.shared.global') }}</button>
      <button type="button" :class="{ selected: scope === 'project' }" :aria-pressed="scope === 'project'" :disabled="!currentProject" @click="scope = 'project'">{{ t('cat.shared.project') }}<span v-if="currentProject"> · {{ currentProject.name }}</span></button>
    </div>
    <p class="se-scope-note">{{ t(scope === 'global' ? 'cat.shared.globalHint' : 'cat.shared.projectHint') }}</p>
    <div v-if="error" class="se-error" role="alert">{{ error }}<button type="button" @click="load">{{ t('cat.shared.retry') }}</button><button type="button" @click="editMCP">{{ t('cat.shared.editMCP') }}</button><button type="button" @click="openFolder('skills')">{{ t('cat.shared.openFolder') }}</button></div>
    <p v-if="loading" class="se-muted" role="status">{{ t('cat.shared.loading') }}</p>
    <div v-if="data" class="se-cards">
      <section class="se-card">
        <header><span><FIcon name="spark" :size="16" /><b>{{ t('cat.shared.skills') }}</b><small>{{ data.skills.length }}</small></span><div><button type="button" @click="openFolder('skills')">{{ t('cat.shared.openFolder') }}</button><button type="button" :disabled="isBusy('import')" @click="importSkill">{{ t(isBusy('import') ? 'cat.shared.importing' : 'cat.shared.import') }}</button></div></header>
        <code v-if="data.skillsRoot" class="se-path">{{ data.skillsRoot }}</code>
        <p v-if="!data.skills.length" class="se-empty">{{ t('cat.shared.noSkills') }}</p>
        <div v-for="item in data.skills" :key="item.name" class="se-item">
          <div><b>{{ item.name }}</b><p>{{ item.description }}</p></div>
          <button type="button" role="switch" class="se-switch" :class="{ on: item.enabled }" :aria-checked="item.enabled" :aria-label="item.name" :disabled="isBusy('skill:' + item.name)" @click="toggle('skill', item)"><i /></button>
        </div>
      </section>
      <section class="se-card">
        <header><span><FIcon name="link" :size="16" /><b>MCP</b><small>{{ data.mcpServers.length }}</small></span><div><button type="button" @click="openFolder('mcp')">{{ t('cat.shared.openFolder') }}</button><button type="button" :disabled="isBusy('editor')" @click="editMCP">{{ t('cat.shared.editMCP') }}</button></div></header>
        <code v-if="data.mcpConfigPath" class="se-path">{{ data.mcpConfigPath }}</code>
        <p v-if="!data.mcpServers.length" class="se-empty">{{ t('cat.shared.noMCP') }}</p>
        <div v-for="item in data.mcpServers" :key="item.name" class="se-item">
          <div><b>{{ item.name }}</b><small class="se-transport">{{ item.description }}</small></div>
          <button type="button" role="switch" class="se-switch" :class="{ on: item.enabled }" :aria-checked="item.enabled" :aria-label="item.name" :disabled="isBusy('mcp:' + item.name)" @click="toggle('mcp', item)"><i /></button>
        </div>
      </section>
      <section class="se-card se-native">
        <header><b>{{ t('cat.shared.targets') }}</b><button type="button" @click="load">{{ t('cat.shared.refresh') }}</button></header>
        <p v-if="!data.targets.length" class="se-empty">{{ t('cat.shared.desktop') }}</p>
        <div v-for="target in data.targets" :key="target.agentKind" class="se-target">
          <div class="se-target-head"><b>{{ label(target.agentKind) }}</b><span :class="['se-status', target.state]">{{ t(statusKey(target.state)) }}</span><small>{{ t('cat.shared.counts', { skills: target.skillCount, mcp: target.mcpCount }) }}</small></div>
          <details><summary>{{ t('cat.shared.paths') }}</summary><code>{{ target.skillsPath }}</code><code>{{ target.configPath }}</code></details>
          <ul v-if="target.warnings.length" class="se-warnings"><li v-for="w in target.warnings" :key="w">{{ w }}</li></ul>
        </div>
      </section>
      <ul v-if="data.warnings.length" class="se-warnings" role="alert"><li v-for="w in data.warnings" :key="w">{{ w }}</li></ul>
    </div>
    <details v-if="currentProject" class="se-existing" @toggle="loadExisting">
      <summary>{{ t('cat.shared.existing') }}</summary>
      <p>{{ t('cat.shared.existingHint') }}</p>
      <p v-if="existingLoading">{{ t('cat.shared.loading') }}</p>
      <p v-else-if="existing?.reason">{{ t('cat.shared.existingUnavailable') }}</p>
      <template v-else-if="existing">
        <div v-for="group in existingGroups" :key="group.key"><b>{{ group.label }} · {{ group.items.length }}</b><p v-for="item in group.items" :key="item.name"><strong>{{ item.name }}</strong><span v-if="item.description"> · {{ item.description }}</span></p></div>
      </template>
      <p v-else>{{ t('cat.shared.existingUnavailable') }}</p>
    </details>
    <p class="se-foot">{{ t('cat.shared.runtimeHint') }}</p>
    <el-dialog v-model="editor" append-to-body :title="t('cat.shared.editMCP')" width="680px" class="se-mcp-dialog">
      <p class="se-editor-hint">{{ t('cat.shared.editorHint') }}</p>
      <textarea v-model="json" spellcheck="false" :aria-label="t('cat.shared.editMCP')" class="se-json" />
      <details class="se-example"><summary>{{ t('cat.shared.example') }}</summary><pre>{{ example }}</pre></details>
      <p v-if="editorError" class="se-error" role="alert">{{ editorError }}</p>
      <template #footer><button type="button" class="se-dialog-btn" @click="editor = false">{{ t('cat.shared.close') }}</button><button type="button" class="se-dialog-btn se-save" :disabled="isBusy('save')" @click="saveMCP">{{ t('cat.shared.save') }}</button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import FIcon from '@/components/icon/FIcon.vue'
import { currentProject, notify } from '@/views/cat/catState'
import { CAT_PROVIDER_LABEL } from '@/api/catAgents'
import { inspectCatConfig, type CatInspectResult } from '@/api/catSettings'
import { pickDirectory } from '@/api/system'
import { toAppError } from '@/api/call'
import { getShared, syncShared, importSharedSkill, toggleShared, readSharedMCP, saveSharedMCP, revealShared, type SharedExtensions, type SharedItem } from '@/api/catSharedExtensions'

const { t } = useI18n()
const scope = ref<'global' | 'project'>('global')
const projectId = computed(() => scope.value === 'project' ? currentProject.value?.id ?? '' : '')
const data = ref<SharedExtensions>()
const loading = ref(false)
const error = ref('')
const busy = reactive<Record<string, boolean>>({})
const editor = ref(false), json = ref(''), editorError = ref('')
const existing = ref<CatInspectResult>(), existingLoading = ref(false)
let existingGeneration = 0
const existingGroups = computed(() => [
  { key: 'plugins', label: t('cat.shared.plugins'), items: existing.value?.plugins ?? [] },
  { key: 'hooks', label: t('cat.shared.hooks'), items: existing.value?.hooks ?? [] },
  { key: 'agents', label: t('cat.shared.agents'), items: existing.value?.agents ?? [] },
])
async function loadExisting(event: Event) {
  if (!(event.target as HTMLDetailsElement).open || !currentProject.value) return
  const id = currentProject.value.id, gen = ++existingGeneration
  existingLoading.value = true
  try {
    const result = await inspectCatConfig(id, 'cat_build')
    if (gen === existingGeneration && id === currentProject.value?.id) existing.value = result
  } catch {
    if (gen === existingGeneration) existing.value = undefined
  } finally { if (gen === existingGeneration) existingLoading.value = false }
}
let generation = 0, editorProject = ''
const drafts = new Map<string, string>()
const pendingMutations = new Map<string, number>()
watch(json, value => { if (editor.value) drafts.set(editorProject, value) }, { flush: 'sync' })
const example = JSON.stringify({ mcpServers: { local: { command: 'npx', args: ['-y', 'your-mcp-package'], env: {} }, remote: { type: 'http', url: 'https://your-server.example/mcp', headers: {} } } }, null, 2)
function statusKey(state: string) { return ({ synced: 'cat.shared.synced', pending: 'cat.shared.pending', error: 'cat.shared.error' } as Record<string, string>)[state] ?? 'cat.shared.pending' }
function label(kind: string) { return CAT_PROVIDER_LABEL[kind as keyof typeof CAT_PROVIDER_LABEL] ?? kind }
function message(e: unknown) { return toAppError(e).code === 'UNSUPPORTED' ? t('cat.shared.desktop') : toAppError(e).message || t('cat.shared.failed') }
function isBusy(key: string) { return !!busy[projectId.value + ':' + key] }
async function load() {
  const gen = ++generation, id = projectId.value
  loading.value = true; error.value = ''
  try { const r = await getShared(id); if (gen === generation) data.value = r }
  catch (e) { if (gen === generation) error.value = message(e) }
  finally { if (gen === generation) loading.value = false }
}
// Only the item being changed is busy; switching scope or closing stays available.
async function mutate(key: string, fn: (id: string) => Promise<SharedExtensions>, id = projectId.value) {
  const busyKey = id + ':' + key
  if (busy[busyKey]) return
  busy[busyKey] = true
  const active = id === projectId.value
  const gen = active ? ++generation : -1
  if (active) loading.value = false
  pendingMutations.set(id, (pendingMutations.get(id) ?? 0) + 1)
  let applied = false
  try {
    const r = await fn(id)
    if (gen === generation && id === projectId.value) { data.value = r; error.value = ''; applied = true }
  } catch (e) { notify(message(e), 'warn') }
  finally {
    busy[busyKey] = false
    const remaining = (pendingMutations.get(id) ?? 1) - 1
    pendingMutations.set(id, remaining)
    // A later-started operation may finish first or fail. Re-read once the
    // whole batch settles so every row reflects the final backend state.
    if (!remaining && id === projectId.value && (!applied || gen !== generation)) void load()
  }
}
function sync() { return mutate('sync', syncShared) }
function toggle(category: string, item: SharedItem) {
  return mutate(category + ':' + item.name, id => toggleShared(id, category, item.name, !item.enabled))
}
async function importSkill() {
  const id = projectId.value
  try {
    const path = await pickDirectory(t('cat.shared.pickSkill'))
    if (path) await mutate('import', pid => importSharedSkill(pid, path), id)
  } catch (e) { notify(message(e), 'warn') }
}
async function openFolder(kind: 'skills' | 'mcp') {
  try { await revealShared(projectId.value, kind) }
  catch (e) { notify(message(e), 'warn') }
}
async function editMCP() {
  const id = projectId.value, gen = generation
  if (drafts.has(id)) { editorProject = id; json.value = drafts.get(id)!; editorError.value = ''; editor.value = true; return }
  const busyKey = id + ':editor'
  busy[busyKey] = true
  try {
    const raw = await readSharedMCP(id)
    if (gen !== generation || id !== projectId.value) return
    json.value = raw; editorProject = id; editorError.value = ''; editor.value = true
  } catch (e) { notify(message(e), 'warn') }
  finally { busy[busyKey] = false }
}
async function saveMCP() {
  let parsed: unknown
  try { parsed = JSON.parse(json.value) } catch { editorError.value = t('cat.shared.invalidJSON'); return }
  if (!parsed || typeof parsed !== 'object' || !('mcpServers' in parsed)) { editorError.value = t('cat.shared.invalidJSON'); return }
  const id = editorProject, raw = json.value
  editor.value = false
  await mutate('save', async () => {
    const result = await saveSharedMCP(id, raw)
    if (drafts.get(id) === raw) drafts.delete(id)
    return result
  }, id)
}
watch([scope, () => currentProject.value?.id], () => {
  if (scope.value === 'project' && !currentProject.value) scope.value = 'global'
  editor.value = false; data.value = undefined
  existing.value = undefined; existingGeneration++; existingLoading.value = false
  void load()
}, { immediate: true })
</script>

<style scoped>
.shared-ext { color: var(--cat-text); font-size: 13px; }
.se-heading, header, header > span, header > div, .se-target-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.se-heading { justify-content: space-between; margin-bottom: 18px; }
h3 { margin: 0 0 6px; font-size: 17px; font-weight: 650; }
.se-heading p, .se-scope-note, .se-foot, .se-muted { color: var(--cat-muted); line-height: 1.6; margin: 0; }
button { display: inline-flex; align-items: center; justify-content: center; gap: 6px; padding: 7px 10px; font: inherit; color: var(--cat-text); background: var(--cat-elevated); border: 1px solid var(--cat-border); border-radius: 7px; cursor: pointer; }
button:hover { border-color: var(--cat-accent); }
button:disabled { opacity: .5; cursor: default; }
button:focus-visible, textarea:focus-visible { outline: 2px solid var(--cat-accent); outline-offset: 2px; }
.se-scopes { display: flex; gap: 4px; padding: 4px; width: fit-content; max-width: 100%; background: var(--cat-surface); border: 1px solid var(--cat-border); border-radius: 10px; }
.se-scopes button { background: transparent; border-color: transparent; }
.se-scopes .selected { background: var(--cat-elevated); border-color: var(--cat-border); color: var(--cat-accent); }
.se-scopes span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 180px; }
.se-scope-note { margin: 10px 0 18px; }
.se-cards { display: flex; flex-direction: column; gap: 16px; }
.se-card { border: 1px solid var(--cat-border); border-radius: 12px; overflow: hidden; background: var(--cat-elevated); }
header { padding: 14px 16px; justify-content: space-between; }
header button { padding: 5px 9px; font-size: 12px; }
header > span small { color: var(--cat-muted); background: var(--cat-surface); border-radius: 5px; padding: 2px 6px; }
.se-path { display: block; font-size: 11px; color: var(--cat-muted); margin: 0 16px 12px; overflow-wrap: anywhere; }
.se-empty { margin: 0; padding: 14px 16px 22px; color: var(--cat-muted); line-height: 1.65; }
.se-item { display: flex; align-items: center; gap: 16px; border-top: 1px solid var(--cat-border); padding: 14px 16px; }
.se-item > div { flex: 1; min-width: 0; }
.se-item b { overflow-wrap: anywhere; }
.se-item p { color: var(--cat-muted); font-size: 12px; margin: 5px 0 0; line-height: 1.5; overflow-wrap: anywhere; }
.se-transport { display: inline-block; margin-left: 10px; color: var(--cat-muted); font: 11px monospace; }
.se-switch { flex: none; width: 32px; height: 19px; padding: 2px; border-radius: 12px; background: var(--cat-border); justify-content: flex-start; border: 0; }
.se-switch i { width: 15px; height: 15px; border-radius: 50%; background: #fff; box-shadow: 0 1px 3px #0002; transition: transform .15s; }
.se-switch.on { background: var(--cat-accent); }
.se-switch.on i { transform: translateX(13px); }
.se-target { padding: 14px 16px; border-top: 1px solid var(--cat-border); }
.se-target-head small { color: var(--cat-muted); font-size: 11px; margin-left: auto; }
.se-status { font-size: 11px; padding: 3px 6px; border-radius: 5px; background: var(--cat-surface); color: var(--cat-muted); }
.se-status.synced { color: var(--cat-success-text); background: var(--cat-success-soft); }
.se-status.error, .se-warnings { color: var(--cat-warn-text); }
.se-status.error { background: var(--cat-warn-soft); }
details { color: var(--cat-muted); margin-top: 10px; font-size: 11px; }
summary { cursor: pointer; width: fit-content; }
details code { display: block; margin: 6px 0; overflow-wrap: anywhere; }
.se-warnings { font-size: 12px; padding-left: 18px; line-height: 1.8; overflow-wrap: anywhere; }
.se-foot { margin-top: 16px; font-size: 12px; }
.se-existing { border: 1px solid var(--cat-border); border-radius: 10px; padding: 14px 16px; margin-top: 16px; font-size: 12px; line-height: 1.6; }
.se-existing p { overflow-wrap: anywhere; }
.se-error { color: var(--cat-danger-text); font-size: 12px; margin: 12px 0; display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
.se-json { width: 100%; height: 300px; min-height: 180px; box-sizing: border-box; resize: vertical; background: var(--cat-surface); color: var(--cat-text); border: 1px solid var(--cat-border); border-radius: 8px; padding: 12px; font: 12px/1.7 Consolas, monospace; tab-size: 2; }
.se-editor-hint { color: var(--cat-muted); font-size: 12px; line-height: 1.6; margin-top: 0; }
.se-example pre { white-space: pre-wrap; overflow-wrap: anywhere; }
.se-dialog-btn { padding: 8px 14px; border: 1px solid var(--cat-border); background: var(--cat-elevated); color: var(--cat-text); border-radius: 7px; cursor: pointer; margin-left: 8px; }
.se-save { background: var(--cat-accent); color: #fff; }
@media (max-width: 560px) { .se-heading { align-items: flex-start; } .se-scopes { width: 100%; box-sizing: border-box; } .se-scopes button { flex: 1; } .se-scopes span { max-width: 100px; } .se-target-head small { width: 100%; margin-left: 0; } }
@media (prefers-reduced-motion: reduce) { .se-switch i { transition: none; } }
</style>
