<template>
  <div v-if="show" class="cap-wrap" data-testid="cat-branch-capsule">
    <button
      v-if="!repo"
      type="button"
      class="cap muted"
      disabled
      :title="CF.notGit"
    >
      <FIcon name="branch" :size="13" />{{ CF.notGit }}
    </button>
    <template v-else>
      <button
        ref="btn"
        type="button"
        class="cap"
        :class="{ open: menuOpen }"
        :aria-expanded="menuOpen"
        aria-haspopup="listbox"
        @click.stop="toggle"
      >
        <FIcon name="branch" :size="13" />
        <span class="nm">{{ branch }}</span>
        <span v-if="dirty" class="dot" /><span v-if="dirty" class="dn">{{ dirty }}</span>
        <span v-if="ahead" class="ahead">↑{{ ahead }}</span>
        <span v-if="behind" class="behind">↓{{ behind }}</span>
        <FIcon name="down" :size="12" class="cv" />
      </button>
      <div v-if="menuOpen" ref="menu" class="menu" role="listbox" data-testid="cat-branch-menu-top" @click.stop>
        <label class="bm-search">
          <FIcon name="search" :size="13" />
          <input ref="filterEl" v-model="q" type="search" :placeholder="CF.branchFilter" @keydown.enter.prevent="onEnter" />
        </label>
        <div class="bm-sec">{{ CF.localSec }}</div>
        <button
          v-for="b in locals"
          :key="'L:' + b.name"
          type="button"
          class="bm-item"
          :class="{ cur: b.current }"
          :disabled="busy || b.current"
          @click="pick(b.name)"
        >
          <FIcon name="branch" :size="13" />
          <span class="nm">{{ b.name }}</span>
          <FIcon v-if="b.current" name="check" :size="13" class="ck" />
        </button>
        <template v-if="remotes.length">
          <hr />
          <div class="bm-sec">{{ CF.remoteSec }}</div>
          <div v-for="b in remotes" :key="'R:' + b.name" class="bm-item remote">
            <FIcon name="branch" :size="13" />
            <span class="nm">{{ b.name }}</span>
            <span class="meta">↓</span>
          </div>
        </template>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessageBox } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import {
  CAT_GIT_CONFLICT_COPY as CF,
  catGitError,
  filterBranchItems,
  gitBranchMenuItems,
  gitCatCheckout,
  isGitDirtyError,
  SIM_GIT_BRANCHES,
} from '@/api/catGit'
import { notify } from '@/views/cat/catState'
import { catGitUi } from './catGitUi'
import { catBranchMenuOpen, closeCatBranchMenu, openCatBranchMenu } from './catBranchMenu'

const props = defineProps<{ visible?: boolean }>()
const busy = ref(false)
const q = ref('')
const btn = ref<HTMLButtonElement>()
const menu = ref<HTMLElement>()
const filterEl = ref<HTMLInputElement>()

const show = computed(() => props.visible !== false)
const st = computed(() => catGitUi.status)
const repo = computed(() => !!st.value?.repo)
const branch = computed(() => st.value?.branch || 'HEAD')
const ahead = computed(() => st.value?.ahead ?? 0)
const behind = computed(() => st.value?.behind ?? 0)
const dirty = computed(() => (st.value?.changes ?? []).length)
const menuOpen = computed(() => catBranchMenuOpen.value === 'top')

const items = computed(() => {
  const s = st.value
  if (!s) return []
  const extra = catGitUi.simBranches
    ? { locals: [...SIM_GIT_BRANCHES.locals], remotes: [...SIM_GIT_BRANCHES.remotes] }
    : {}
  return gitBranchMenuItems({ branch: s.branch, upstream: s.upstream, ...extra })
})
const filtered = computed(() => filterBranchItems(items.value, q.value))
const locals = computed(() => filtered.value.filter((b) => b.kind === 'local'))
const remotes = computed(() => filtered.value.filter((b) => b.kind === 'remote'))

function toggle() {
  if (menuOpen.value) closeCatBranchMenu()
  else {
    q.value = ''
    openCatBranchMenu('top')
    void nextTick(() => filterEl.value?.focus())
  }
}

function onEnter() {
  const hit = locals.value.find((b) => b.name === q.value.trim()) || locals.value.find((b) => !b.current)
  if (hit && !hit.current) void pick(hit.name)
}

async function pick(name: string) {
  closeCatBranchMenu()
  if (!catGitUi.live || !catGitUi.convId) {
    notify('预览里不能改仓库。', 'warn')
    return
  }
  if (busy.value) return
  busy.value = true
  try {
    await gitCatCheckout(catGitUi.convId, name, false)
  } catch (e) {
    if (isGitDirtyError(e)) {
      busy.value = false
      try {
        await ElMessageBox.confirm(CF.dirtyBody(name), CF.dirtyTitle, {
          confirmButtonText: CF.dirtyOk,
          cancelButtonText: CF.dirtyCancel,
          type: 'warning',
        })
      } catch {
        return
      }
      busy.value = true
      try {
        await gitCatCheckout(catGitUi.convId, name, true)
      } catch (e2) {
        notify(catGitError(e2), 'warn')
      }
    } else {
      notify(catGitError(e), 'warn')
    }
  } finally {
    busy.value = false
  }
}

function onPointer(e: PointerEvent) {
  const t = e.target
  if (!(t instanceof Node)) return
  if (btn.value?.contains(t) || menu.value?.contains(t)) return
  if (menuOpen.value) closeCatBranchMenu()
}
onMounted(() => document.addEventListener('pointerdown', onPointer))
onBeforeUnmount(() => document.removeEventListener('pointerdown', onPointer))
</script>

<style scoped>
.cap-wrap { position: relative; flex: none; }
.cap {
  height: 28px;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 0 10px;
  border: 1px solid var(--cat-border);
  border-radius: 999px;
  background: var(--cat-surface);
  color: var(--cat-text);
  font: inherit;
  font-size: 12px;
  font-weight: 650;
  cursor: pointer;
}
.cap:hover, .cap.open { background: var(--cat-hover); }
.cap.muted {
  opacity: 0.55;
  cursor: default;
  font-weight: 500;
}
.cap .nm { max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cap .dot {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--cat-warn, #c9a227);
}
.cap .dn, .cap .ahead, .cap .behind { font-size: 11px; font-weight: 700; }
.cap .ahead { color: var(--cat-success-text); }
.cap .behind, .cap .dn { color: var(--cat-muted); }
.cap .cv { color: var(--cat-muted); }
.menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: 20;
  width: min(280px, 70vw);
  max-height: min(360px, 55vh);
  overflow-y: auto;
  padding: 6px;
  border: 1px solid var(--cat-border);
  border-radius: 10px;
  background: var(--cat-elevated);
  box-shadow: var(--cat-shadow-lg);
}
.bm-search {
  display: flex; align-items: center; gap: 6px; height: 30px;
  padding: 0 8px; margin-bottom: 4px; border-radius: 8px;
  background: var(--cat-hover); color: var(--cat-muted);
}
.bm-search input {
  flex: 1; min-width: 0; border: none; outline: none;
  background: transparent; font: inherit; font-size: 12px; color: var(--cat-text);
}
.bm-sec { padding: 6px 8px 2px; font-size: 11px; font-weight: 650; color: var(--cat-muted); }
.bm-item {
  width: 100%; display: flex; align-items: center; gap: 6px;
  padding: 6px 8px; border: none; border-radius: 6px;
  background: transparent; color: var(--cat-text); font: inherit; font-size: 12.5px;
  text-align: left; cursor: pointer;
}
.bm-item:hover:not(:disabled) { background: var(--cat-hover); }
.bm-item:disabled { cursor: default; }
.bm-item.cur { font-weight: 650; }
.bm-item.remote { opacity: 0.75; cursor: default; }
.bm-item .nm { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.bm-item .ck { color: var(--cat-success-text); }
.bm-item .meta { color: var(--cat-muted); font-size: 11px; }
.menu hr { border: none; border-top: 1px solid var(--cat-border); margin: 4px 2px; }
</style>
