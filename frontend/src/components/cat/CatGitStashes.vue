<template>
  <section class="stash-panel" data-testid="cat-git-stashes">
    <div class="stash-head"><span>{{ t('cat.gitWorkbench.stashHint') }}</span><button type="button" class="icon" :title="t('cat.git.refresh')" @click="load"><FIcon name="refresh" :size="14" /></button></div>
    <div class="save-box">
      <input v-model="message" :placeholder="t('cat.gitWorkbench.stashMessage')" :aria-label="t('cat.gitWorkbench.stashMessage')" @keydown.enter="save" />
      <label class="checkbox"><input v-model="includeUntracked" type="checkbox" />{{ t('cat.gitWorkbench.includeUntracked') }}</label>
      <button type="button" class="btn save" :disabled="busy || blocked || !status?.changes.length" :title="blocked ? t('cat.gitWorkbench.resolveFirst') : undefined" data-testid="cat-git-stash-save" @click="save"><FIcon name="zip" :size="13" />{{ t('cat.gitWorkbench.saveStash') }}</button>
    </div>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p v-if="loading" class="empty" role="status">{{ t('cat.commit.loading') }}</p>
    <div v-else-if="!items.length" class="empty"><FIcon name="zip" :size="22" /><b>{{ t('cat.gitWorkbench.noStashes') }}</b><span>{{ t('cat.gitWorkbench.noStashesHint') }}</span></div>
    <article v-for="st in items" :key="st.sha" class="stash-item" data-testid="cat-git-stash-row">
      <button type="button" class="stash-title" :title="st.message" @click="view(st)"><FIcon name="zip" :size="14" /><span>{{ st.message }}</span></button>
      <div class="stash-meta"><code>{{ st.ref }}</code><span :title="absTime(st.time)">{{ relTime(st.time) }}</span></div>
      <div class="stash-actions">
        <button type="button" :disabled="busy || blocked" :title="t('cat.gitWorkbench.applyHint')" @click="restore(st, 'apply')">{{ t('cat.gitWorkbench.applyStash') }}</button>
        <button type="button" :disabled="busy || blocked" :title="t('cat.gitWorkbench.popHint')" @click="restore(st, 'pop')">{{ t('cat.gitWorkbench.popStash') }}</button>
        <span class="sp" /><button type="button" class="danger" :disabled="busy" :title="t('cat.gitWorkbench.dropStash')" @click="drop(st)"><FIcon name="trash" :size="13" /></button>
      </div>
    </article>
    <CatGitDiffDialog :visible="!!diffTitle" :title="diffTitle" :loading="diffLoading" :error="diffError" :diff="diff" @close="closeDiff" />
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessageBox } from 'element-plus'
import { t } from '@/i18n'
import FIcon from '@/components/icon/FIcon.vue'
import CatGitDiffDialog from './CatGitDiffDialog.vue'
import { absTime, relTime } from '@/api/catCommits'
import { catGitError, isGitConflictError, type GitDiff, type GitStatus } from '@/api/catGit'
import { getCatStashDiff, gitCatStash, listCatStashes, type GitStash } from '@/api/catGitWorkflows'
import { notify } from '@/views/cat/catState'
import { catSim } from '@/views/cat/catDevSim'
const props = defineProps<{ convId: string; live: boolean; status: GitStatus | null; locked?: boolean }>()
const emit = defineEmits<{ changed: []; conflict: []; busy: [value: boolean] }>()
const items = ref<GitStash[]>([]), loading = ref(false), busy = ref(false), error = ref('')
const message = ref(''), includeUntracked = ref(true)
const diffTitle = ref(''), diffLoading = ref(false), diffError = ref(''), diff = ref<GitDiff | null>(null)
const blocked = computed(() => props.locked || props.status?.mergeState !== 'none' || !!props.status?.conflicts.length)
let seq = 0, diffSeq = 0
async function load() {
  const my = ++seq
  loading.value = true
  try {
    const data = props.live ? await listCatStashes(props.convId) : import.meta.env.DEV && catSim.has('gitgraph') ? [{ sha: 'a'.repeat(40), ref: 'stash@{0}', message: 'On main: Sidebar refactor in progress', time: Math.floor(Date.now() / 1000) - 1800 }] : []
    if (my === seq) { items.value = data; error.value = '' }
  } catch (e) { if (my === seq) error.value = catGitError(e) }
  finally { if (my === seq) loading.value = false }
}
async function run(fn: (conv: string) => Promise<void>) {
  if (!props.live) { notify(t('cat.git.preview'), 'warn'); return }
  if (busy.value || props.locked) return
  const conv = props.convId
  busy.value = true; emit('busy', true); error.value = ''
  try { await fn(conv); if (conv === props.convId) await load() }
  catch (e) { if (conv === props.convId) { error.value = catGitError(e); if (isGitConflictError(e)) emit('conflict') } }
  finally { busy.value = false; emit('busy', false); emit('changed') }
}
function save() { if (blocked.value || !props.status?.changes.length) return; const opts = { message: message.value, includeUntracked: includeUntracked.value }; void run(async conv => { await gitCatStash(conv, 'save', opts); if (conv === props.convId) { message.value = ''; notify(t('cat.gitWorkbench.stashed')) } }) }
async function restore(st: GitStash, op: 'apply' | 'pop') {
  const conv = props.convId
  if (props.status?.changes.length) { error.value = t('cat.gitWorkbench.stashDirty'); return }
  if (op === 'pop') {
    try { await ElMessageBox.confirm(t('cat.gitWorkbench.popAsk'), t('cat.gitWorkbench.popStash'), { confirmButtonText: t('cat.gitWorkbench.restore'), cancelButtonText: t('cat.commit.cancel') }) } catch { return }
  }
  if (conv !== props.convId) return
  void run(async id => { await gitCatStash(id, op, { sha: st.sha }); if (id === props.convId) notify(t('cat.gitWorkbench.restored')) })
}
async function drop(st: GitStash) {
  const conv = props.convId
  try { await ElMessageBox.confirm(t('cat.gitWorkbench.dropAsk', { message: st.message }), t('cat.gitWorkbench.dropStash'), { type: 'warning', confirmButtonText: t('cat.gitWorkbench.dropStash'), cancelButtonText: t('cat.commit.cancel'), confirmButtonType: 'danger' }) } catch { return }
  if (conv !== props.convId) return
  void run(id => gitCatStash(id, 'drop', { sha: st.sha }))
}
function closeDiff() { diffTitle.value = ''; diffSeq++ }
async function view(st: GitStash) {
  const my = ++diffSeq
  diffTitle.value = st.message; diffLoading.value = true; diffError.value = ''; diff.value = null
  try { const r = props.live ? await getCatStashDiff(props.convId, st.sha) : { path: '', binary: false, text: 'diff --git a/sidebar.vue b/sidebar.vue\n@@ -1 +1 @@\n-Old sidebar\n+New sidebar handle' }; if (my === diffSeq) diff.value = r }
  catch (e) { if (my === diffSeq) diffError.value = catGitError(e) }
  finally { if (my === diffSeq) diffLoading.value = false }
}
onMounted(load)
watch(() => props.convId, () => { message.value = ''; items.value = []; closeDiff(); void load() })
onBeforeUnmount(() => { seq++; diffSeq++ })
defineExpose({ reload: load })
</script>

<style scoped>
.stash-panel { padding: 10px; color: var(--cat-text); font-size: 12px; }
.stash-head { display: flex; align-items: center; gap: 6px; color: var(--cat-muted); line-height: 1.5; }
.stash-head span { flex: 1; }
.icon { display: grid; place-items: center; width: 24px; height: 24px; flex: none; border: 0; background: transparent; color: var(--cat-text-2); cursor: pointer; }
.save-box { display: flex; flex-direction: column; gap: 10px; margin: 10px 0 14px; padding: 12px; border: 1px solid var(--cat-border); border-radius: 8px; background: var(--cat-hover); }
.save-box > input { width: 100%; min-width: 0; box-sizing: border-box; height: 30px; padding: 0 8px; border: 1px solid var(--cat-border); border-radius: 6px; background: var(--cat-surface); color: var(--cat-text); font: inherit; }
.checkbox { display: flex; align-items: center; gap: 5px; color: var(--cat-text-2); }
.btn { display: flex; align-items: center; justify-content: center; gap: 6px; height: 30px; border: 1px solid var(--cat-border); border-radius: 6px; background: var(--cat-surface); color: var(--cat-text); font: inherit; cursor: pointer; }
.btn:disabled, button:disabled { opacity: .45; cursor: default; }
.stash-item { padding: 10px 0; border-bottom: 1px solid var(--cat-divider); }
.stash-title { display: flex; align-items: flex-start; gap: 6px; width: 100%; border: 0; padding: 0; background: transparent; color: var(--cat-text); text-align: left; font: inherit; cursor: pointer; }
.stash-title :deep(svg) { flex: none; margin-top: 2px; }
.stash-title span { overflow-wrap: anywhere; line-height: 1.5; }
.stash-meta { display: flex; gap: 6px; margin: 6px 0; color: var(--cat-muted); font-size: 10.5px; }
.stash-actions { display: flex; align-items: center; gap: 4px; }
.stash-actions button { display: grid; place-items: center; height: 26px; padding: 0 6px; border: 0; border-radius: 4px; background: transparent; color: var(--cat-text-2); font: inherit; cursor: pointer; }
.stash-actions button:hover:not(:disabled) { background: var(--cat-hover); color: var(--cat-text); }
.stash-actions .danger { color: var(--cat-danger-text); }
.sp { flex: 1; }
.empty { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 30px 10px; color: var(--cat-muted); line-height: 1.5; text-align: center; }
.empty b { color: var(--cat-text-2); font-weight: 500; }
.error { color: var(--cat-danger-text); line-height: 1.5; }
</style>
