<template>
  <div v-if="showBanner && first && !state.dismissed" class="cu-bar" role="status" data-testid="cat-cli-update-bar">
    <FIcon name="warn" :size="14" />
    <span class="cu-text">{{ t('cat.cliUpdate.notice', { cli: label(first.agentKind), current: first.currentVersion, latest: first.latestVersion }) }}<span v-if="state.errors[first.agentKind]" class="cu-error"> {{ state.errors[first.agentKind] }}</span></span>
    <div class="cu-bar-actions">
      <button v-for="row in updates" :key="row.agentKind" type="button" class="cu-update" :disabled="state.updating[row.agentKind]" :title="label(row.agentKind) + ' · ' + row.currentVersion + ' → ' + row.latestVersion" :aria-label="label(row.agentKind) + ' · ' + t(row.canUpdate ? 'cat.cliUpdate.update' : 'cat.cliUpdate.manual')" :data-testid="row === first ? 'cat-cli-update-accept' : 'cat-cli-bar-' + row.agentKind" :data-cli="row.agentKind" @click="update(row)">
        <i v-if="state.updating[row.agentKind]" class="cu-spin" aria-hidden="true" /><FIcon v-else name="check" :size="14" /><span v-if="updates.length > 1">{{ label(row.agentKind) }} · </span>{{ state.updating[row.agentKind] ? t('cat.cliUpdate.updating') : row.canUpdate ? t('cat.cliUpdate.update') : t('cat.cliUpdate.manual') }}
      </button>
    </div>
    <button type="button" class="cu-close" :aria-label="t('common.close')" data-testid="cat-cli-update-dismiss" @click="dismissCLIUpdates"><FIcon name="x" :size="14" /></button>
  </div>
  <Teleport to="body">
    <div v-if="state.dialog && updates.length" class="cu-mask" @mousedown.self="closeDialog">
      <section ref="box" class="cu-dialog" role="alertdialog" aria-modal="true" aria-labelledby="cu-title" aria-describedby="cu-body" tabindex="-1" data-testid="cat-cli-update-dialog" @keydown="onKey">
        <div class="cu-heading"><FIcon name="warn" :size="20" /><h3 id="cu-title">{{ t('cat.cliUpdate.title') }}</h3><button type="button" class="cu-close" :aria-label="t('common.close')" @click="dismissThisVisit"><FIcon name="x" :size="16" /></button></div>
        <p id="cu-body">{{ t('cat.cliUpdate.body') }}</p>
        <div v-for="row in updates" :key="row.agentKind" class="cu-row">
          <div><strong>{{ label(row.agentKind) }}</strong><span>{{ row.currentVersion }} → {{ row.latestVersion }}</span><span v-if="state.errors[row.agentKind]" class="cu-error" role="alert">{{ state.errors[row.agentKind] }}</span></div>
          <button type="button" class="cu-update" :disabled="state.updating[row.agentKind]" :data-testid="'cat-cli-update-' + row.agentKind" @click="update(row)">{{ state.updating[row.agentKind] ? t('cat.cliUpdate.updating') : row.canUpdate ? t('cat.cliUpdate.update') : t('cat.cliUpdate.manual') }}</button>
        </div>
        <p v-if="isUpdating" role="status">{{ t('cat.cliUpdate.progress') }}</p>
        <div class="cu-actions"><button ref="later" type="button" @click="closeDialog">{{ t('cat.cliUpdate.later') }}</button></div>
      </section>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { providerLabel as label } from '@/api/catAgents'
import { availableCLIUpdates as updates, catCLIUpdates as state, dismissCLIUpdates, updateCatCLI, type CatCLIUpdate } from '@/views/cat/catCLIUpdates'
import { applyCatStatus } from '@/views/cat/catState'

defineProps<{ showBanner: boolean }>()
const { t } = useI18n()
const first = computed(() => updates.value.find((row) => !state.updating[row.agentKind]) ?? updates.value[0])
const isUpdating = computed(() => Object.values(state.updating).some(Boolean))
const box = ref<HTMLElement>()
const later = ref<HTMLButtonElement>()
let opener: HTMLElement | null = null
watch(() => state.dialog, async (open) => {
  if (open) {
    opener = document.activeElement as HTMLElement | null
    await nextTick()
    later.value?.focus()
  } else if (opener?.isConnected) opener.focus()
}, { immediate: true })
onBeforeUnmount(() => { if (opener?.isConnected) opener.focus() })
function closeDialog() { state.dialog = false }
function dismissThisVisit() { dismissCLIUpdates() }
async function update(row: CatCLIUpdate) {
  if (state.updating[row.agentKind]) return
  if (!row.canUpdate) {
    if (row.updateUrl) BrowserOpenURL(row.updateUrl)
    return
  }
  closeDialog()
  const result = await updateCatCLI(row, applyCatStatus)
  const error = state.errors[row.agentKind]
  if (error) ElMessage.warning(label(row.agentKind) + '：' + error)
  else if (result) ElMessage.success(t('cat.cliUpdate.completed', { cli: label(row.agentKind) }))
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); closeDialog() }
  if (e.key !== 'Tab') return
  const buttons = Array.from(box.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])
  e.preventDefault()
  if (!buttons.length) { box.value?.focus(); return }
  const i = buttons.indexOf(document.activeElement as HTMLButtonElement)
  buttons[(i + (e.shiftKey ? -1 : 1) + buttons.length) % buttons.length]?.focus()
}
</script>

<style scoped>
.cu-bar { flex: none; display: flex; align-items: center; gap: 8px; padding: 8px 14px; background: var(--cat-warn-soft); color: var(--cat-warn-text); border-bottom: 1px solid color-mix(in srgb, var(--cat-warn) 25%, transparent); font-size: 12px; }
.cu-bar { flex-wrap: wrap; }
.cu-bar-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
.cu-bar > .cu-close { margin-left: auto; }
.cu-spin { width: 12px; height: 12px; box-sizing: border-box; border: 2px solid currentColor; border-right-color: transparent; border-radius: 50%; animation: cu-spin .8s linear infinite; }
@keyframes cu-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .cu-spin { animation: none; } }
.cu-text { flex: 1 1 200px; min-width: 0; line-height: 1.5; }
button { font: inherit; cursor: pointer; border: 1px solid var(--cat-border); border-radius: var(--cat-radius-sm); background: var(--cat-elevated); color: var(--cat-text); padding: 6px 10px; }
button:disabled { opacity: .5; cursor: default; }
button:focus-visible { outline: none; box-shadow: var(--cat-focus); }
.cu-update { display: inline-flex; gap: 4px; align-items: center; white-space: nowrap; color: var(--cat-accent); }
.cu-close { display: inline-flex; padding: 4px; border: 0; background: transparent; color: inherit; }
.cu-mask { position: fixed; inset: 0; z-index: 3200; display: grid; place-items: center; background: var(--cat-scrim); }
.cu-dialog { width: 450px; max-width: calc(100vw - 32px); max-height: calc(100vh - 48px); overflow: auto; padding: 22px; border: 1px solid var(--cat-border); border-radius: var(--cat-radius-xl); background: var(--cat-elevated); color: var(--cat-text); box-shadow: var(--cat-shadow-lg); font-size: 13px; }
.cu-heading { display: flex; align-items: center; gap: 10px; color: var(--cat-warn-text); }
.cu-heading h3 { flex: 1; margin: 0; color: var(--cat-text); font-size: 16px; }
.cu-dialog p { line-height: 1.6; color: var(--cat-text-2); }
.cu-row { display: flex; gap: 12px; align-items: center; padding: 12px 0; border-bottom: 1px solid var(--cat-border); }
.cu-row > div { flex: 1; min-width: 0; }
.cu-row strong, .cu-row span { display: block; overflow-wrap: anywhere; }
.cu-row span { margin-top: 4px; color: var(--cat-muted); font-size: 12px; }
.cu-error, .cu-dialog .cu-error { color: var(--cat-danger-text); }
.cu-actions { display: flex; justify-content: flex-end; margin-top: 18px; }
</style>
