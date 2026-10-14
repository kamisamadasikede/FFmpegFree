<template>
  <section class="browser" data-testid="cat-browser" :aria-label="t('cat.misc.s_912302cb02')">
    <div class="tabs">
      <div
        v-for="(tab, i) in tabs"
        :key="tab.id"
        class="tab"
        :class="{ on: i === current }"
        role="tab"
        :aria-selected="i === current"
        @click="current = i"
      >
        <FIcon name="globe" :size="14" />
        <span class="ttl">{{ tab.title || t('cat.misc.s_978eeadf50') }}</span>
        <button type="button" class="x" :title="t('cat.misc.s_b15d91274e')" :aria-label="t('cat.misc.s_57603fcb82')" data-testid="cat-browser-tab-close" @click.stop="closeTab(i)">
          <FIcon name="x" :size="12" />
        </button>
      </div>
      <button type="button" class="add" :title="t('cat.misc.s_978eeadf50')" :aria-label="t('cat.misc.s_978eeadf50')" data-testid="cat-browser-new" @click="addTab">
        <FIcon name="plus" :size="14" />
      </button>
    </div>
    <div class="nav">
      <button type="button" :title="t('cat.misc.s_5094c1a328')" :aria-label="t('cat.misc.s_5094c1a328')" :disabled="!canBack" @click="back">
        <FIcon name="left" :size="16" />
      </button>
      <button type="button" :title="t('cat.misc.s_5811e2837b')" :aria-label="t('cat.misc.s_5811e2837b')" :disabled="!canForward" @click="forward">
        <FIcon name="right" :size="16" />
      </button>
      <button type="button" :title="t('cat.misc.s_64ca9bab92')" :aria-label="t('cat.misc.s_64ca9bab92')" :disabled="!canReload" data-testid="cat-browser-reload" @click="reload">
        <FIcon name="refresh" :size="15" />
      </button>
      <form class="addr" @submit.prevent="go">
        <input
          ref="addrEl"
          v-model="active.draft"
          type="text"
          :placeholder="t('cat.misc.s_113a7f95fd')"
          :aria-label="t('cat.misc.s_113a7f95fd')"
          data-testid="cat-browser-url"
        />
      </form>
      <button
        type="button"
        class="tool"
        :class="{ on: pickOn }"
        :title="t('cat.misc.s_9f31c72ef0')"
        :aria-label="t('cat.misc.s_9f31c72ef0')"
        :aria-pressed="pickOn"
        :disabled="!active.doc"
        data-testid="cat-browser-pick"
        @click="pickOn = !pickOn"
      >
        <FIcon name="chat" :size="16" />
      </button>
      <div class="more" data-cat-browser-more>
        <button type="button" :title="t('cat.misc.s_0ec9eaf9c3')" :aria-label="t('cat.misc.s_0ec9eaf9c3')" :aria-expanded="moreOpen" @click="moreOpen = !moreOpen">
          <FIcon name="more" :size="16" />
        </button>
        <div v-if="moreOpen" class="menu" role="menu">
          <button type="button" role="menuitem" :disabled="!canReload" @click="reload(); moreOpen = false">重新加载</button>
        </div>
      </div>
    </div>
    <div v-if="active.loading" class="empty" role="status">正在打开…</div>
    <div v-else-if="active.error" class="empty fail" role="alert" data-testid="cat-browser-fail">
      <b>{{ active.error }}</b>
      <span class="hint">部分网站可能打不开。</span>
      <button
        type="button"
        class="ext"
        data-testid="cat-browser-open-system"
        :disabled="!systemURL"
        @click="openInSystem"
      >
        在系统浏览器中打开
      </button>
    </div>
    <iframe
      v-else-if="active.doc"
      ref="frameEl"
      :key="active.token"
      class="view"
      :class="{ picking: pickOn }"
      :srcdoc="active.doc"
      sandbox="allow-scripts allow-forms"
      referrerpolicy="no-referrer"
      :title="t('cat.misc.s_912302cb02')"
      @load="onFrameLoad"
    />
    <div v-else class="empty start" data-testid="cat-browser-start">
      <button type="button" class="start-main" @click="focusAddr">
        <FIcon name="globe" :size="36" />
        <b>开始浏览</b>
        <span>搜索或输入网址</span>
      </button>
      <button
        type="button"
        class="ext"
        data-testid="cat-browser-open-system"
        :disabled="!systemURL"
        @click="openInSystem"
      >
        在系统浏览器中打开
      </button>
    </div>
    <p v-if="pickOn" class="pick-bar">{{ COPY.pickHint }}</p>
    <CatPickPop
      v-if="pop"
      :key="pop.key"
      :title="pop.title"
      :selector="pop.selector"
      :anchor="pop.anchor"
      @add="onPopAdd"
      @close="pop = null"
    />
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { toAppError } from '@/api/call'
import { openCatBrowser } from '@/api/catBrowser'
import { CAT_PREVIEW_COPY as COPY } from '@/api/catFilePreview'
import { openExternal } from '@/utils/openExternal'
import CatPickPop from './CatPickPop.vue'
import { browserDocument, browserOpen, readBrowserNav } from './catBrowser'
import { newPickToken, pickLabel, PICK_SOURCE, readPickEvent, type PickBox, type PickNote, type PickSelect } from './catPick'

interface Tab {
  id: string
  title: string
  urls: string[]
  index: number
  draft: string
  doc: string
  token: string
  loading: boolean
  error: string
  gen: number
}

interface Pop {
  key: string
  title: string
  selector: string
  anchor: PickBox
  element: PickSelect
}

const emit = defineEmits<{ close: []; pick: [note: PickNote] }>()
let seq = 0
function fresh(): Tab {
  seq += 1
  return { id: 'b' + seq, title: '', urls: [], index: -1, draft: '', doc: '', token: '', loading: false, error: '', gen: 0 }
}
const tabs = ref<Tab[]>([fresh()])
const current = ref(0)
const active = computed(() => tabs.value[current.value] || tabs.value[0])
const canBack = computed(() => active.value.index > 0)
const canForward = computed(() => active.value.index >= 0 && active.value.index < active.value.urls.length - 1)
const canReload = computed(() => active.value.index >= 0 && !!active.value.urls[active.value.index])
const addrEl = ref<HTMLInputElement>()
const frameEl = ref<HTMLIFrameElement>()
const moreOpen = ref(false)
const pickOn = ref(false)
const pop = ref<Pop | null>(null)
const relBox = ref<PickBox | null>(null)

function focusAddr() {
  addrEl.value?.focus()
}

/** 当前页或地址栏解析出的 http(s)，供「在系统浏览器中打开」 */
const systemURL = computed(() => {
  const tab = active.value
  const cur = tab.index >= 0 ? tab.urls[tab.index] : ''
  if (cur) return cur
  return browserOpen(tab.draft)
})

function openInSystem() {
  const url = systemURL.value
  if (url) openExternal(url)
}

function addTab() {
  if (tabs.value.length >= 6) return
  tabs.value.push(fresh())
  current.value = tabs.value.length - 1
  pop.value = null
  void Promise.resolve().then(focusAddr)
}

function closeTab(i: number) {
  if (tabs.value.length === 1) {
    emit('close')
    return
  }
  tabs.value.splice(i, 1)
  if (current.value >= tabs.value.length) current.value = tabs.value.length - 1
  else if (current.value > i) current.value -= 1
  pop.value = null
}

async function load(tab: Tab, url: string) {
  const mine = ++tab.gen
  tab.loading = true
  tab.error = ''
  tab.doc = ''
  tab.draft = url
  pop.value = null
  try {
    const page = await openCatBrowser(url)
    if (mine !== tab.gen) return
    const final = page.url || url
    tab.token = newPickToken()
    tab.doc = page.html ? browserDocument(page.html, final, tab.token) : ''
    tab.title = page.title || t('cat.misc.s_978eeadf50')
    tab.draft = final
    if (tab.index >= 0) tab.urls[tab.index] = final
    if (!tab.doc) tab.error = t('cat.misc.s_7c41b9d405')
  } catch (e) {
    if (mine !== tab.gen) return
    tab.error = toAppError(e).message || t('cat.misc.s_7c41b9d405')
  } finally {
    if (mine === tab.gen) tab.loading = false
  }
}

function go() {
  const tab = active.value
  const next = browserOpen(tab.draft)
  if (!next) {
    tab.error = t('cat.misc.s_77b9eaebcd')
    tab.doc = ''
    return
  }
  tab.urls = tab.urls.slice(0, tab.index + 1)
  tab.urls.push(next)
  tab.index = tab.urls.length - 1
  void load(tab, next)
}

function back() {
  const tab = active.value
  if (tab.index <= 0) return
  tab.index -= 1
  void load(tab, tab.urls[tab.index])
}

function forward() {
  const tab = active.value
  if (tab.index >= tab.urls.length - 1) return
  tab.index += 1
  void load(tab, tab.urls[tab.index])
}

function reload() {
  const tab = active.value
  const url = tab.urls[tab.index]
  if (!url) return
  void load(tab, url)
}

function openLink(href: string) {
  const tab = active.value
  tab.urls = tab.urls.slice(0, tab.index + 1)
  tab.urls.push(href)
  tab.index = tab.urls.length - 1
  void load(tab, href)
}

function armPick(on: boolean) {
  const frame = frameEl.value?.contentWindow
  if (!frame || !active.value.token) return
  frame.postMessage({ source: PICK_SOURCE, token: active.value.token, kind: 'arm', on }, '*')
}

function clearPickSelection() {
  const frame = frameEl.value?.contentWindow
  if (!frame || !active.value.token) return
  frame.postMessage({ source: PICK_SOURCE, token: active.value.token, kind: 'clear' }, '*')
}

function anchorOf(box: PickBox): PickBox | null {
  const frame = frameEl.value?.getBoundingClientRect()
  if (!frame) return null
  return { left: frame.left + box.left, top: frame.top + box.top, width: box.width, height: box.height }
}

function openPop(el: PickSelect) {
  relBox.value = el.box
  const anchor = anchorOf(el.box)
  if (!anchor) return
  pop.value = {
    key: [el.tag, el.id, el.selector, el.text].join('|'),
    title: pickLabel(el),
    selector: el.selector,
    anchor,
    element: el,
  }
}

function onPopAdd(note: string, intent: 'edit' | 'ask') {
  const cur = pop.value
  if (!cur) return
  const el = cur.element
  const tab = active.value
  emit('pick', {
    file: tab.urls[tab.index] || tab.title || t('cat.misc.s_912302cb02'),
    tag: el.tag,
    id: el.id,
    classes: el.classes,
    selector: el.selector,
    text: el.text,
    styles: el.styles,
    snippet: el.snippet,
    intent,
    note: note.trim().slice(0, 500),
  })
  pop.value = null
  relBox.value = null
}

function onFrameLoad() {
  if (pickOn.value) armPick(true)
}

function onMessage(ev: MessageEvent) {
  const tab = active.value
  const nav = readBrowserNav(ev.data, tab.token)
  const frame = frameEl.value?.contentWindow
  if (nav) {
    if (frame && ev.source !== frame) return
    openLink(nav)
    return
  }
  const parsed = readPickEvent(ev.data, tab.token)
  if (!parsed) return
  if (frame && ev.source !== frame) return
  if (parsed.kind === 'ready') {
    if (pickOn.value) armPick(true)
    return
  }
  if (!pickOn.value) return
  if (parsed.kind === 'esc') {
    if (pop.value) pop.value = null
    else pickOn.value = false
    return
  }
  if (parsed.kind === 'move') {
    if (!pop.value) return
    relBox.value = parsed.box
    const anchor = anchorOf(parsed.box)
    if (anchor) pop.value = { ...pop.value, anchor }
    return
  }
  openPop(parsed)
}

function onOutside(e: PointerEvent) {
  const t = e.target
  if (!(t instanceof Element)) return
  if (moreOpen.value && !t.closest('[data-cat-browser-more]')) moreOpen.value = false
  if (!pop.value) return
  if (t.closest('[data-cat-pick-pop]')) return
  if (t === frameEl.value) return
  pop.value = null
}

function onKey(e: KeyboardEvent) {
  if (!pickOn.value || e.key !== 'Escape') return
  if (pop.value) {
    pop.value = null
    e.preventDefault()
    return
  }
  pickOn.value = false
}

watch(pop, (cur, prev) => {
  if (prev && !cur) clearPickSelection()
})
watch(pickOn, (on) => {
  if (!on) {
    pop.value = null
    armPick(false)
    return
  }
  armPick(true)
})
watch(current, () => {
  pop.value = null
  moreOpen.value = false
})

onMounted(() => {
  window.addEventListener('message', onMessage)
  window.addEventListener('pointerdown', onOutside)
  window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  window.removeEventListener('message', onMessage)
  window.removeEventListener('pointerdown', onOutside)
  window.removeEventListener('keydown', onKey)
})
</script>

<style scoped>
.browser {
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--ff-bg-app);
  position: relative;
}
.tabs {
  flex: none;
  display: flex;
  align-items: flex-end;
  gap: 4px;
  padding: 8px 8px 0;
  background: var(--ff-bg-surface);
  border-bottom: 1px solid var(--ff-border);
}
.tab {
  max-width: 180px;
  height: 30px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 6px 0 8px;
  border: 1px solid transparent;
  border-bottom: none;
  border-radius: 8px 8px 0 0;
  color: var(--ff-text-3);
  cursor: pointer;
}
.tab.on {
  color: var(--ff-text-1);
  background: var(--ff-bg-app);
  border-color: var(--ff-border);
}
.ttl {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
}
.tab .x,
.add,
.nav > button,
.more > button {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--ff-text-2);
  cursor: pointer;
  padding: 0;
}
.tab .x { width: 20px; height: 20px; }
.add { margin: 0 0 2px 2px; }
.nav > button:disabled { opacity: 0.35; cursor: default; }
.nav > button:hover:not(:disabled),
.tab .x:hover,
.add:hover,
.more > button:hover,
.tool.on {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.nav {
  flex: none;
  height: 42px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 0 8px;
  background: var(--ff-bg-app);
  border-bottom: 1px solid var(--ff-border);
}
.addr {
  flex: 1;
  min-width: 0;
}
.addr input {
  width: 100%;
  height: 30px;
  border: none;
  background: transparent;
  color: var(--ff-text-1);
  font: inherit;
  font-size: 13px;
  text-align: center;
  padding: 0 8px;
}
.addr input:focus,
.addr input:not(:placeholder-shown) {
  text-align: left;
}
.addr input:focus { outline: none; }
.more { position: relative; }
.menu {
  position: absolute;
  top: calc(100% + 4px);
  right: 0;
  z-index: 8;
  min-width: 120px;
  padding: 4px;
  border: 1px solid var(--ff-border);
  border-radius: 8px;
  background: var(--ff-bg-surface);
}
.menu button {
  width: 100%;
  height: 32px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--ff-text-1);
  text-align: left;
  padding: 0 8px;
  cursor: pointer;
}
.menu button:hover:not(:disabled) { background: var(--ff-bg-hover); }
.menu button:disabled { opacity: 0.4; }
.empty {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--ff-text-3);
  background: var(--ff-bg-app);
}
.empty.start {
  border: none;
  font: inherit;
}
.start-main {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  border: none;
  background: transparent;
  color: inherit;
  cursor: text;
  font: inherit;
  padding: 0;
}
.empty b {
  font-size: 16px;
  font-weight: 650;
  color: var(--ff-text-1);
  text-align: center;
  max-width: 28em;
}
.empty span,
.empty .hint {
  font-size: 12px;
  color: var(--ff-text-3);
  text-align: center;
  max-width: 28em;
}
.empty.fail {
  gap: 10px;
  padding: 24px;
}
.ext {
  margin-top: 4px;
  height: 32px;
  padding: 0 14px;
  border-radius: 8px;
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.ext:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.ext:disabled {
  opacity: 0.4;
  cursor: default;
}
.view {
  flex: 1;
  min-height: 0;
  width: 100%;
  border: none;
  background: white;
}
.view.picking { cursor: crosshair; }
.pick-bar {
  position: absolute;
  left: 12px;
  bottom: 12px;
  margin: 0;
  padding: 6px 10px;
  border-radius: 8px;
  background: var(--ff-bg-surface);
  color: var(--ff-text-2);
  font-size: 12px;
  border: 1px solid var(--ff-border);
}
</style>
