<template>
  <section class="extra" data-testid="cat-term" :aria-label="t('cat.misc.s_4722bc0c56')" @pointerdown="focusTerm">
    <div class="hd">
      <b>终端</b>
      <span class="sp" />
      <button type="button" class="x" :title="t('cat.misc.s_b15d91274e')" :aria-label="t('cat.misc.s_40dba7a311')" data-testid="cat-term-close" @click="emit('close')">
        <FIcon name="x" :size="14" />
      </button>
    </div>
    <p v-if="hint" class="hint">{{ hint }}</p>
    <div ref="hostEl" class="screen" data-testid="cat-term-screen"></div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import FIcon from '@/components/icon/FIcon.vue'
import { toAppError } from '@/api/call'
import { onCatTerm, resizeCatTerm, startCatTerm, stopCatTerm, writeCatTerm, type CatTermEvent } from '@/api/catTerm'
import { pipeTermKeys } from './catTermInput'

const props = defineProps<{ convId: string; live?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const hint = ref('')
const ready = ref(false)
const hostEl = ref<HTMLElement>()
let term: Terminal | null = null
let fit: FitAddon | null = null
let termId = ''
let generation = 0
let off = () => {}
let queued = ''
let pipeLine = ''
let usePipe = false
let observer: ResizeObserver | null = null

function focusTerm() {
  term?.focus()
}

function apply(ev: CatTermEvent) {
  if (ev.data) term?.write(ev.data)
  if (ev.exit != null) {
    ready.value = false
    hint.value = t('cat.misc.s_5e8c07cd68')
  }
}

async function sendRaw(data: string) {
  if (!termId || !data) return
  try {
    await writeCatTerm(termId, data)
  } catch (e) {
    hint.value = toAppError(e).message || t('cat.misc.s_4d438ab71b')
  }
}

function takeKeys(data: string) {
  const text = data.replace(/\u007f/g, '\b')
  if (!ready.value || !termId) {
    queued += text
    return
  }
  if (!usePipe) {
    void sendRaw(text)
    return
  }
  const next = pipeTermKeys(pipeLine, text)
  pipeLine = next.line
  if (next.screen) term?.write(next.screen)
  if (next.send) void sendRaw(next.send)
}

function openScreen() {
  const host = hostEl.value
  if (!host || term) return
  term = new Terminal({
    cursorBlink: true,
    fontSize: 13,
    fontFamily: 'Consolas, "Cascadia Mono", monospace',
    scrollback: 3000,
    theme: { background: '#1c1c1c', foreground: '#e8e8e8', cursor: '#e8e8e8' },
    windowsPty: { backend: 'conpty' },
  })
  fit = new FitAddon()
  term.loadAddon(fit)
  term.open(host)
  term.onData((data) => {
    takeKeys(data)
  })
  term.onResize(({ cols, rows }) => {
    if (!termId) return
    void resizeCatTerm(termId, cols, rows).catch(() => {})
  })
  fit.fit()
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(() => fit?.fit())
    observer.observe(host)
  }
  term.focus()
}

async function boot() {
  const mine = ++generation
  ready.value = false
  hint.value = ''
  queued = ''
  pipeLine = ''
  usePipe = false
  term?.reset()
  const previous = termId
  termId = ''
  if (previous) void stopCatTerm(previous).catch(() => {})
  off()
  if (!props.live) {
    hint.value = t('cat.misc.s_da12d3240f')
    off = () => {}
    return
  }
  openScreen()
  const early: CatTermEvent[] = []
  let opened = false
  off = onCatTerm((ev) => {
    if (!opened) {
      early.push(ev)
      return
    }
    if (ev.id === termId) apply(ev)
  })
  try {
    const started = await startCatTerm(props.convId)
    if (mine !== generation) {
      if (started.id) void stopCatTerm(started.id).catch(() => {})
      return
    }
    termId = started.id
    usePipe = !started.console
    opened = true
    let ended = false
    for (const ev of early) {
      if (ev.id !== termId) continue
      apply(ev)
      if (ev.exit != null) ended = true
    }
    if (!termId) {
      hint.value = t('cat.misc.s_a156b74cfc')
      return
    }
    fit?.fit()
    if (!ended) {
      ready.value = true
      if (queued) {
        const data = queued
        queued = ''
        takeKeys(data)
      }
    }
    term?.focus()
  } catch (e) {
    if (mine !== generation) return
    const err = toAppError(e)
    hint.value = err.code === 'UNSUPPORTED' ? t('cat.misc.s_c548e98a1a') : (err.message || t('cat.misc.s_a156b74cfc'))
  }
}

onMounted(() => {
  openScreen()
  void boot()
})
watch(() => props.convId, () => {
  void boot()
})
onBeforeUnmount(() => {
  generation++
  off()
  observer?.disconnect()
  term?.dispose()
  term = null
  if (termId) void stopCatTerm(termId).catch(() => {})
})
</script>

<style scoped>
.extra {
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: #1c1c1c;
  color: #e8e8e8;
}
.hd {
  flex: none;
  height: 32px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 8px;
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  border-bottom: 1px solid var(--ff-border);
}
.hd b { font-size: 12px; font-weight: 650; }
.sp { flex: 1; }
.x {
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
.hint {
  margin: 0;
  padding: 4px 8px;
  font-size: 12px;
  color: #f0c674;
}
.screen {
  flex: 1;
  min-height: 0;
  padding: 4px 0 0;
  --wails-draggable: no-drag;
}
.screen :deep(.xterm) {
  height: 100%;
}
</style>
