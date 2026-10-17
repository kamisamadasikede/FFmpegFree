<template>
  <ElDialog :model-value="visible" :title="title" width="min(900px, calc(100vw - 40px))" class="git-diff-dialog" destroy-on-close :close-on-click-modal="true" @update:model-value="!$event && emit('close')">
    <p v-if="loading" class="muted" role="status">{{ t('cat.git.diffLoading') }}</p>
    <p v-else-if="error" class="error" role="alert">{{ error }}</p>
    <p v-else-if="diff?.binary" class="muted">{{ t('cat.git.diffBinary') }}</p>
    <p v-else-if="!diff?.text.trim()" class="muted">{{ t('cat.gitWorkbench.noDiff') }}</p>
    <pre v-else class="patch" data-testid="cat-git-diff-content"><span v-for="(line, i) in lines" :key="i" :class="lineClass(line)">{{ line }}</span></pre>
    <template #footer><button type="button" class="close" @click="emit('close')">{{ t('cat.commit.close') }}</button></template>
  </ElDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ElDialog } from 'element-plus'
import { t } from '@/i18n'
import type { GitDiff } from '@/api/catGit'
const props = defineProps<{ visible: boolean; title: string; loading?: boolean; error?: string; diff: GitDiff | null }>()
const emit = defineEmits<{ close: [] }>()
const lines = computed(() => (props.diff?.text || '').replace(/\n$/, '').split('\n'))
const lineClass = (l: string) => l.startsWith('+++') || l.startsWith('---') || l.startsWith('diff ') || l.startsWith('index ') ? 'meta' : l.startsWith('@@') ? 'hunk' : l.startsWith('+') ? 'add' : l.startsWith('-') ? 'del' : ''
</script>

<style scoped>
.patch { margin: 0; max-height: 65vh; overflow: auto; padding: 10px 0; border: 1px solid var(--ff-border); border-radius: 8px; background: var(--ff-bg-body); font: 12px/1.6 var(--ff-font-mono); }
.patch span { display: block; min-height: 1.6em; white-space: pre; padding: 0 12px; }
.add { background: color-mix(in srgb, var(--ff-success) 12%, transparent); }
.del { background: color-mix(in srgb, var(--ff-danger) 12%, transparent); }
.hunk { color: var(--ff-primary-text); background: var(--ff-bg-hover); }
.meta, .muted { color: var(--ff-text-3); }
.error { color: var(--ff-danger); }
.close { height: 30px; padding: 0 14px; border: 1px solid var(--ff-border); border-radius: 6px; background: var(--ff-bg-elevated); color: var(--ff-text-1); font: inherit; cursor: pointer; }
</style>
