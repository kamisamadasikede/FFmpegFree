<template>
  <div v-if="visible !== false" class="cap-wrap" data-testid="cat-branch-capsule">
    <button ref="btn" type="button" class="cap" :disabled="!status?.repo" :aria-expanded="menuOpen" aria-haspopup="dialog" @click.stop="toggle">
      <FIcon name="branch" :size="13" />
      <span v-if="status?.detached" class="nm" data-testid="cat-branch-detached">{{ BCD.detached }} · {{ status.headShortSha }}</span>
      <span v-else class="nm">{{ status?.branch || CF.notGit }}</span>
      <span v-if="dirty" class="dirty">● {{ dirty }}</span><FIcon name="down" :size="12" />
    </button>
    <button v-if="!status?.detached && status?.ahead" type="button" class="count" :title="t('cat.gitWorkbench.outgoing')" data-testid="cat-branch-outgoing" @click="showHistory('outgoing')">↑ {{ status.ahead }}</button>
    <button v-if="status?.upstream && status.behind" type="button" class="count" :title="t('cat.gitWorkbench.incoming')" data-testid="cat-branch-incoming" @click="showHistory('incoming')">↓ {{ status.behind }}</button>
    <CatBranchTree :open="menuOpen" :conv-id="catGitUi.convId" :anchor="btn || null" @close="closeCatBranchMenu" @changed="catGitUi.gitRefresh++" />
  </div>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import { t } from '@/i18n'
import FIcon from '@/components/icon/FIcon.vue'
import CatBranchTree from './CatBranchTree.vue'
import { CAT_BRANCH_COPY as BCD } from '@/api/catBranches'
import { CAT_GIT_CONFLICT_COPY as CF } from '@/api/catGit'
import { catGitUi, requestCatGitHistory } from './catGitUi'
import { catBranchMenuOpen, closeCatBranchMenu, openCatBranchMenu } from './catBranchMenu'
import type { GitHistoryScope } from '@/api/catGitWorkflows'
withDefaults(defineProps<{ visible?: boolean }>(), { visible: true })
const btn = ref<HTMLButtonElement>()
const status = computed(() => catGitUi.status)
const dirty = computed(() => status.value?.changes.length || 0)
const menuOpen = computed(() => catBranchMenuOpen.value === 'top')
function toggle() { if (menuOpen.value) closeCatBranchMenu(); else openCatBranchMenu('top') }
function showHistory(scope: GitHistoryScope) { closeCatBranchMenu(); requestCatGitHistory(scope) }
</script>
<style scoped>
.cap-wrap { display: inline-flex; align-items: center; gap: 3px; flex: none; }
.cap, .count { display: inline-flex; align-items: center; gap: 5px; height: 28px; border: 1px solid var(--cat-border); border-radius: 7px; padding: 0 8px; background: var(--cat-surface); color: var(--cat-text-2); font: inherit; font-size: 12px; cursor: pointer; }
.cap:hover:not(:disabled), .count:hover { background: var(--cat-hover); color: var(--cat-text); }
.cap:disabled { opacity: .5; cursor: default; }
.nm { max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.dirty { color: var(--cat-orange); font-size: 10px; }
.count { border-color: transparent; font-variant-numeric: tabular-nums; }
button:focus-visible { outline: 2px solid var(--cat-accent); outline-offset: -2px; }
</style>
