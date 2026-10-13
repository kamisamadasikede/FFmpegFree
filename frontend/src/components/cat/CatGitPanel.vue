<template>
  <div class="git" data-testid="cat-git">
    <div class="git-top">
      <button
        type="button"
        class="pr"
        :disabled="prDisabled"
        :title="prTitle"
        data-testid="cat-git-pr"
        @click="createPR"
      >
        <FIcon name="branch" :size="13" />
        <span>{{ COPY.pr }}</span>
      </button>
      <span class="sp" />
      <button type="button" class="ib" :class="{ on: filtering }" :title="COPY.filter" data-testid="cat-git-filter" @click="filtering = !filtering">
        <FIcon name="search" :size="15" />
      </button>
      <button type="button" class="ib" :title="COPY.refresh" :disabled="loading" data-testid="cat-git-refresh" @click="emit('refresh')">
        <FIcon name="refresh" :size="15" />
      </button>
    </div>
    <label v-if="filtering" class="git-filter">
      <FIcon name="search" :size="13" />
      <input v-model="filter" type="search" :placeholder="COPY.filter" />
    </label>
    <p v-if="loadError && status" class="git-err" role="alert">{{ loadError }}</p>
    <div v-if="loading && !status" class="git-empty" role="status">正在加载…</div>
    <div v-else-if="loadError && !status" class="git-empty" role="alert">
      <span>{{ loadError }}</span>
      <button type="button" class="retry" @click="emit('refresh')">重试</button>
    </div>
    <div v-else-if="!status?.repo" class="git-empty">{{ COPY.notRepo }}</div>
    <template v-else>
      <div class="git-branch">
        <div class="bn">
          <div class="name">{{ status.branch || 'HEAD' }}</div>
          <div v-if="status.upstream" class="up">→ {{ status.upstream }}</div>
        </div>
        <div class="counts">
          <span v-if="status.ahead" class="add">+{{ status.ahead }}</span>
          <span v-if="status.behind" class="del">−{{ status.behind }}</span>
          <button v-if="status.web" type="button" class="ib" :title="COPY.openRemote" @click="openRemote">
            <FIcon name="external" :size="14" />
          </button>
        </div>
      </div>
      <textarea v-model="message" class="git-msg" rows="3" :placeholder="COPY.message" data-testid="cat-git-message" />
      <p v-if="actionError" class="git-err" role="alert">{{ actionError }}</p>
      <div class="commit-bar">
        <button type="button" class="stage main" :disabled="commitDisabled" data-testid="cat-git-primary" @click="doCommit()">
          {{ COPY.commit }}
        </button>
        <button
          ref="moreBtn"
          type="button"
          class="stage more"
          :disabled="busy"
          :aria-expanded="menuOpen"
          aria-haspopup="menu"
          title="更多"
          data-testid="cat-git-more"
          @click.stop="menuOpen = !menuOpen"
        >
          <FIcon name="down" :size="14" />
        </button>
        <div v-if="menuOpen" ref="menuEl" class="git-menu" role="menu" @click.stop>
          <template v-for="(e, i) in menuEntries" :key="i">
            <hr v-if="e.kind === 'sep'" />
            <button
              v-else
              type="button"
              role="menuitem"
              :disabled="e.disabled || busy"
              :title="e.title || undefined"
              :class="{ danger: e.danger }"
              @click="onMenu(e.id)"
            >{{ e.label }}</button>
          </template>
        </div>
      </div>
      <div class="groups">
        <div v-if="!sections.length" class="git-empty flat">{{ COPY.empty }}</div>
        <div v-for="g in sections" :key="g.id">
          <div class="sec">
            <button type="button" class="sec-h" @click="toggleOpen(g.id)">
              <FIcon :name="isOpen(g.id) ? 'down' : 'right'" :size="12" />
              <span>{{ g.title }} {{ g.items.length }}</span>
            </button>
            <button
              type="button"
              class="ib"
              :title="g.act === 'unstage' ? COPY.unstage : COPY.stage"
              :disabled="busy"
              @click="g.act === 'unstage' ? unstagePaths(g.items) : stagePaths(g.items)"
            >
              <FIcon :name="g.act === 'unstage' ? 'minus' : 'plus'" :size="14" />
            </button>
          </div>
          <div v-if="isOpen(g.id)" class="list">
            <button
              v-for="c in g.items"
              :key="g.id + ':' + c.path"
              type="button"
              class="row"
              :title="c.path"
              @click="emit('open', c.path, c.name)"
            >
              <FIcon name="file" :size="14" class="fi" />
              <span class="nm" :class="'st-' + c.letter">{{ c.name }}</span>
              <span v-if="c.dir" class="dir">{{ c.dir }}</span>
              <span class="stat">
                <span v-if="c.added" class="add">+{{ c.added }}</span>
                <span v-if="c.deleted" class="del">−{{ c.deleted }}</span>
                <span class="letter" :class="'st-' + c.letter">{{ c.letter }}</span>
              </span>
              <span
                class="act"
                :title="g.act === 'unstage' ? COPY.unstage : COPY.stage"
                @click.stop="g.act === 'unstage' ? unstagePaths([c]) : stagePaths([c])"
              >
                <FIcon :name="g.act === 'unstage' ? 'x' : 'plus'" :size="12" />
              </span>
            </button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessageBox } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import {
  CAT_GIT_COPY as COPY,
  catGitError,
  gitCatBranch,
  gitCatCommit,
  gitCatPullRequest,
  gitCatRemote,
  gitCatStage,
  gitCatUnstage,
  gitChangeGroups,
  gitMenuEntries,
  type GitChange,
  type GitStatus,
} from '@/api/catGit'
import { openExternal } from '@/utils/openExternal'
import { notify } from '@/views/cat/catState'

const props = defineProps<{
  convId: string
  live: boolean
  status: GitStatus | null
  loading?: boolean
  loadError?: string
}>()

const emit = defineEmits<{ refresh: []; open: [rel: string, name: string] }>()

const message = ref('')
const filter = ref('')
const filtering = ref(false)
const opened = ref<Record<string, boolean>>({ work: true, staged: true, untracked: true })
const busy = ref(false)
const actionError = ref('')
const menuOpen = ref(false)
const moreBtn = ref<HTMLButtonElement>()
const menuEl = ref<HTMLElement>()

const shown = computed(() => {
  const list = props.status?.changes ?? []
  const q = filter.value.trim().toLowerCase()
  if (!filtering.value || !q) return list
  return list.filter((c) => c.path.toLowerCase().includes(q) || c.name.toLowerCase().includes(q))
})

const sections = computed(() => {
  const g = gitChangeGroups(shown.value)
  const out: { id: 'work' | 'staged' | 'untracked'; title: string; items: GitChange[]; act: 'stage' | 'unstage' }[] = []
  if (g.work.length) out.push({ id: 'work', title: COPY.changes, items: g.work, act: 'stage' })
  if (g.staged.length) out.push({ id: 'staged', title: COPY.staged, items: g.staged, act: 'unstage' })
  if (g.untracked.length) out.push({ id: 'untracked', title: COPY.untracked, items: g.untracked, act: 'stage' })
  return out
})

function isOpen(id: string) {
  return opened.value[id] !== false
}
function toggleOpen(id: string) {
  opened.value = { ...opened.value, [id]: !isOpen(id) }
}

const prDisabled = computed(() => busy.value || !props.live || !props.status?.repo || !props.status.web)
const prTitle = computed(() => {
  if (!props.status?.repo) return COPY.notRepo
  return props.status.web ? COPY.pr : COPY.notGitHub
})
const commitDisabled = computed(() => busy.value || !props.status?.repo || !message.value.trim() || !props.status.changes.length)
const menuEntries = computed(() => gitMenuEntries({
  message: message.value,
  changeCount: props.status?.changes.length ?? 0,
  upstream: props.status?.upstream ?? '',
  web: props.status?.web ?? '',
}))

async function run(fn: () => Promise<void>) {
  if (!props.live) {
    notify(COPY.preview, 'warn')
    return
  }
  if (busy.value) return
  busy.value = true
  actionError.value = ''
  try {
    await fn()
  } catch (e) {
    actionError.value = catGitError(e)
  } finally {
    busy.value = false
    // 部分文件已经暂存时也要刷新，否则列表还停在失败前
    emit('refresh')
  }
}

function createPR() {
  void run(async () => {
    const url = await gitCatPullRequest(props.convId)
    if (url) openExternal(url)
  })
}

function openRemote() {
  if (props.status?.web) openExternal(props.status.web)
}

function onPointerDown(e: PointerEvent) {
  const t = e.target
  if (!(t instanceof Node)) return
  if (moreBtn.value?.contains(t) || menuEl.value?.contains(t)) return
  menuOpen.value = false
}
onMounted(() => document.addEventListener('pointerdown', onPointerDown))
onBeforeUnmount(() => document.removeEventListener('pointerdown', onPointerDown))

async function commitOnly() {
  const msg = message.value.trim()
  await gitCatCommit(props.convId, msg)
  message.value = ''
}

function doCommit() {
  void run(() => commitOnly())
}

async function afterCommit(op: string) {
  await commitOnly()
  await gitCatRemote(props.convId, op)
}

function remote(op: string) {
  menuOpen.value = false
  void run(() => gitCatRemote(props.convId, op))
}

async function onMenu(id: string) {
  menuOpen.value = false
  if (id === 'commit') {
    doCommit()
    return
  }
  if (id === 'commit-push') {
    void run(() => afterCommit('push'))
    return
  }
  if (id === 'commit-sync') {
    void run(() => afterCommit('sync'))
    return
  }
  if (id === 'force') {
    try {
      await ElMessageBox.confirm(COPY.forceAsk, COPY.force, { confirmButtonText: '继续', cancelButtonText: '取消', type: 'warning' })
    } catch {
      return
    }
    remote('force')
    return
  }
  if (id === 'pr') {
    createPR()
    return
  }
  if (id === 'branch') {
    let name = ''
    try {
      const r = await ElMessageBox.prompt(COPY.branchAsk, COPY.newBranch, { confirmButtonText: '切换', cancelButtonText: '取消' })
      name = r.value.trim()
    } catch {
      return
    }
    if (!name) return
    void run(() => gitCatBranch(props.convId, name))
    return
  }
  if (id === 'push-pr') {
    void run(async () => {
      await gitCatRemote(props.convId, 'push')
      const url = await gitCatPullRequest(props.convId)
      if (url) openExternal(url)
    })
    return
  }
  const op: Record<string, string> = { push: 'push', pull: 'pull', ff: 'ff', sync: 'sync', rebase: 'rebase', fetch: 'fetch', publish: 'publish' }
  if (op[id]) remote(op[id])
}

function stagePaths(items: GitChange[]) {
  if (!items.length) return
  void run(() => gitCatStage(props.convId, items.map((c) => c.path)))
}

function unstagePaths(items: GitChange[]) {
  if (!items.length) return
  void run(() => gitCatUnstage(props.convId, items.map((c) => c.path)))
}
</script>

<style scoped>
.git {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.git-top {
  height: 40px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 8px;
}
.sp { flex: 1; }
.pr {
  height: 26px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 10px;
  border: none;
  border-radius: 999px;
  background: var(--cat-text);
  color: var(--cat-surface);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.pr:disabled {
  opacity: 0.4;
  cursor: default;
}
.ib {
  width: 24px;
  height: 24px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 6px;
  padding: 0;
  background: transparent;
  color: var(--cat-text-2);
  cursor: pointer;
}
.ib:hover:not(:disabled),
.ib.on {
  background: var(--cat-hover);
  color: var(--cat-text);
}
.ib:disabled {
  opacity: 0.4;
  cursor: default;
}
.git-filter {
  margin: 0 10px 8px;
  height: 28px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border-radius: 8px;
  background: var(--cat-hover);
  color: var(--cat-muted);
}
.git-filter input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  font: inherit;
  font-size: 12px;
  color: var(--cat-text);
}
.git-branch {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 2px 12px 8px;
}
.bn { min-width: 0; flex: 1; }
.bn .name {
  font-size: 13px;
  font-weight: 650;
  color: var(--cat-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.bn .up {
  margin-top: 2px;
  font-size: 12px;
  color: var(--cat-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.counts {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: none;
  font-size: 12px;
  font-weight: 650;
}
.add { color: var(--cat-success-text); }
.del { color: var(--cat-danger-text); }
.git-msg {
  margin: 0 10px;
  min-height: 64px;
  resize: none;
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  background: var(--cat-surface);
  color: var(--cat-text);
  font: inherit;
  font-size: 13px;
  line-height: 1.4;
  padding: 8px;
}
.git-msg::placeholder { color: var(--cat-muted); }
.git-err {
  margin: 6px 12px 0;
  font-size: 12px;
  line-height: 1.4;
  color: var(--cat-danger-text);
}
.commit-bar {
  position: relative;
  margin: 8px 10px 0;
  display: flex;
}
.stage {
  height: 30px;
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  background: var(--cat-surface);
  color: var(--cat-text);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.commit-bar .stage { margin: 0; }
.commit-bar .main {
  flex: 1;
  border-radius: 8px 0 0 8px;
  border-right: none;
}
.commit-bar .more {
  width: 32px;
  border-radius: 0 8px 8px 0;
  display: grid;
  place-items: center;
  color: var(--cat-text-2);
}
.git-menu {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  z-index: 6;
  max-height: min(420px, 70vh);
  overflow-y: auto;
  padding: 4px;
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  background: var(--cat-elevated);
  box-shadow: var(--cat-shadow-lg);
  animation: cat-pop-in var(--ff-dur-base) var(--ff-ease-out) both;
  transform-origin: top center;
}
.git-menu button {
  display: block;
  width: 100%;
  text-align: left;
  border: none;
  background: transparent;
  padding: 6px 8px;
  border-radius: 6px;
  font: inherit;
  font-size: 12.5px;
  color: var(--cat-text);
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.git-menu button:hover:not(:disabled) { background: var(--cat-hover); }
.git-menu button:disabled { opacity: 0.4; cursor: default; }
.git-menu button.danger:not(:disabled) { color: var(--cat-danger-text); }
.git-menu hr {
  border: none;
  border-top: 1px solid var(--cat-border);
  margin: 4px 2px;
}
.stage:hover:not(:disabled) { background: var(--cat-hover); }
.stage:disabled { opacity: 0.45; cursor: default; }
.groups {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
.sec {
  margin-top: 10px;
  height: 28px;
  flex: none;
  display: flex;
  align-items: center;
  padding: 0 6px 0 8px;
}
.sec-h {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  color: var(--cat-text-2);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  padding: 0;
}
.list {
  padding: 0 6px 4px;
}
.row {
  width: 100%;
  min-height: 28px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 6px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--cat-text);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.row:hover { background: var(--cat-hover); }
.row :deep(svg.fi) { color: var(--cat-file-icon); flex: none; }
.nm {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dir {
  flex: none;
  max-width: 42%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--cat-muted);
  font-size: 12px;
}
.stat {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 4px;
  flex: none;
  font-size: 11px;
  font-weight: 650;
}
.letter { font-size: 12px; }
.act {
  width: 16px;
  height: 16px;
  display: none;
  place-items: center;
  color: var(--cat-text-2);
  flex: none;
}
.row:hover .act { display: grid; }
.st-M, .st-R { color: var(--cat-git-mod); }
.st-A, .st-U { color: var(--cat-success-text); }
.st-D { color: var(--cat-danger-text); }
.git-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px 16px;
  color: var(--cat-muted);
  font-size: 13px;
  text-align: center;
}
.git-empty.flat {
  flex: none;
  padding: 12px 8px;
  align-items: flex-start;
}
.retry {
  border: 1px solid var(--cat-border);
  background: transparent;
  color: var(--cat-text-2);
  border-radius: 6px;
  padding: 4px 10px;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
</style>
