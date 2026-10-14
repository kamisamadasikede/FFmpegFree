<template>
  <Teleport to="body">
    <div v-if="open" class="mg-mask" data-testid="cat-merge-dialog" @mousedown.self="tryClose">
      <div class="mg" role="dialog" aria-modal="true" :aria-labelledby="titleId" :aria-busy="busy || undefined" @keydown.esc.stop="tryClose">
        <h3 :id="titleId" class="mg-title">{{ MC.title(current) }}</h3>
        <div class="mg-body">
          <CatModeSelect
            v-model="picked"
            :options="opts"
            :disabled="busy"
            :aria-label="MC.title(current)"
            class="mg-pick"
            testid="cat-merge-pick"
          />
          <div v-if="err" class="mg-err" role="alert" data-testid="cat-merge-err">
            <div class="mg-err-line">
              <span>{{ err.text }}</span>
              <button
                v-if="err.raw"
                type="button"
                class="mg-more"
                :aria-expanded="rawOpen"
                data-testid="cat-merge-err-more"
                @click="rawOpen = !rawOpen"
              ><FIcon :name="rawOpen ? 'down' : 'right'" :size="12" />{{ MC.detail }}</button>
            </div>
            <CatRawDetail v-if="err.raw && rawOpen" :text="err.raw" testid="cat-merge-err" />
          </div>
        </div>
        <footer class="mg-foot">
          <button type="button" class="mg-btn" :disabled="busy" data-testid="cat-merge-cancel" @click="tryClose">{{ MC.cancel }}</button>
          <button type="button" class="mg-btn pri" :disabled="busy || !picked || !!block" data-testid="cat-merge-ok" @click="doMerge">
            <span v-if="busy" class="spin" aria-hidden="true" />{{ MC.ok }}
          </button>
        </footer>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
/** v0.50「合并到当前分支…」弹窗：宽 400、elevated + dialog 阴影、圆角 12；合并中不能关 */
import { computed, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import CatModeSelect from '@/components/cat/panels/CatModeSelect.vue'
import CatRawDetail from './CatRawDetail.vue'
import {
  CAT_BRANCH_COPY,
  CAT_MERGE_COPY as MC,
  catMergeBlock,
  catMergeError,
  defaultMergeOption,
  mergeCatBranch,
  mergeOptions,
  parseBranchOption,
  type CatBranches,
  type CatMergeError,
  type CatMergeResult,
} from '@/api/catBranches'

const props = defineProps<{ open: boolean; convId: string; branches: CatBranches; running?: boolean; mergeState?: string }>()
const emit = defineEmits<{ close: []; done: [r: CatMergeResult, branch: string] }>()

const titleId = `mg-t-${Math.random().toString(36).slice(2, 8)}`
const current = computed(() => props.branches.branch || (props.branches.detached ? CAT_BRANCH_COPY.detached : 'HEAD'))
const opts = computed(() => mergeOptions(props.branches))
const picked = ref('')
const busy = ref(false)
const failed = ref<CatMergeError | null>(null)
const rawOpen = ref(false)
/** 前端已知就先拦（正在回复 / 已在合并中），合并按钮置灰 */
const block = computed<CatMergeError | null>(() => {
  if (props.running) return catMergeBlock('running')
  if (props.mergeState && props.mergeState !== 'none') return catMergeBlock('merging')
  return null
})
const err = computed(() => failed.value ?? block.value)

watch(
  () => props.open,
  (on) => {
    if (!on) return
    picked.value = defaultMergeOption(opts.value)
    failed.value = null
    rawOpen.value = false
    busy.value = false
  },
  { immediate: true },
)
watch(picked, () => { if (!busy.value) failed.value = null })

function tryClose() {
  if (!busy.value) emit('close')
}

async function doMerge() {
  const p = parseBranchOption(picked.value)
  if (!p || busy.value || block.value || !props.convId) return
  busy.value = true
  failed.value = null
  rawOpen.value = false
  try {
    const r = await mergeCatBranch(props.convId, p.branch, p.remote)
    busy.value = false
    emit('done', r, p.branch)
  } catch (e) {
    failed.value = catMergeError(e)
    busy.value = false
  }
}
</script>

<style scoped>
.mg-mask { position: fixed; inset: 0; z-index: 3100; display: grid; place-items: center; background: var(--cat-scrim); }
.mg {
  width: 400px; max-width: calc(100vw - 32px); box-sizing: border-box;
  display: flex; flex-direction: column; gap: 16px; padding: 20px;
  border: 1px solid var(--ff-border); border-radius: 12px;
  background: var(--ff-bg-elevated); box-shadow: var(--ff-shadow-dialog); color: var(--cat-text);
}
.mg-title { margin: 0; font-size: 14px; font-weight: 600; line-height: 20px; word-break: break-all; }
.mg-body { display: flex; flex-direction: column; }
.mg-pick { width: 100%; }
.mg-pick :deep(.cms-btn) { width: 100%; max-width: none; }
.mg-err { margin-top: 4px; font-size: 12px; line-height: 18px; color: var(--ff-danger, var(--cat-danger)); }
.mg-err-line { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.mg-more { display: inline-flex; align-items: center; gap: 2px; padding: 0; border: 0; background: none; color: var(--cat-muted); font: inherit; font-size: 12px; cursor: pointer; }
.mg-more:hover { color: var(--cat-text-2); }
.mg-foot { display: flex; justify-content: flex-end; gap: 8px; }
.mg-btn {
  height: 32px; min-width: 64px; padding: 0 14px; display: inline-flex; align-items: center; justify-content: center; gap: 6px;
  border: 1px solid var(--ff-border); border-radius: 6px; background: var(--ff-bg-elevated); color: var(--cat-text);
  font: inherit; font-size: 13px; cursor: pointer;
}
.mg-btn:hover:not(:disabled) { background: var(--cat-hover); }
.mg-btn:disabled { opacity: 0.5; cursor: default; }
.mg-btn.pri { border-color: transparent; background: var(--cat-accent); color: #fff; }
.mg-btn.pri:hover:not(:disabled) { filter: brightness(1.05); background: var(--cat-accent); }
.mg-btn.pri[disabled]:has(.spin) { opacity: 0.8; }
.spin { width: 12px; height: 12px; border: 1.5px solid currentColor; border-right-color: transparent; border-radius: 50%; animation: mg-spin 0.8s linear infinite; }
@keyframes mg-spin { to { transform: rotate(360deg); } }
</style>
