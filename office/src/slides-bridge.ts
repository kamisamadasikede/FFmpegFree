/** Browser equivalent of GenOffice Slides' Electron preload.
 * Document authority and mutations stay in the ffc Node host, shared with Agent tools.
 */
import { DEFAULT_AI_PANEL_PREFS } from '@genoffice/ui/ai-panel-prefs'
import { showToast } from '../vendor/slides/renderer/components/toast-bus'

type State = { revision: number; savedRevision: number; sequence: number; state: string; error: string; slides?: any[]; canUndo: boolean; canRedo: boolean }
const listeners = new Map<string, Set<(value: any) => void>>()
let capability: { id: string; token: string; locale?: string }, state: State
let tail: Promise<unknown> = Promise.resolve(), disposed = false
let textRevision: number | undefined, gestureRevision: number | undefined
function emit(name: string, value?: any) { for (const fn of listeners.get(name) || []) fn(value) }
function on(name: string, fn: (value: any) => void) {
  const set = listeners.get(name) || new Set(); listeners.set(name, set); set.add(fn)
  return () => { set.delete(fn) }
}
async function request(action: string, args: any = {}) {
  const response = await fetch('/api', { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + capability.token },
    body: JSON.stringify({ id: capability.id, action, ...args }), signal: AbortSignal.timeout(60000) })
  const result = await response.json()
  if (!response.ok || result.error?.message) throw new Error(result.error?.message || '办公操作失败')
  return result
}
function publish(next: State) {
  const previous = state; state = next
  if (next.slides && previous?.revision !== next.revision) emit('DeckChanged', { slides: next.slides })
  emit('HistoryChanged', { canUndo: next.canUndo, canRedo: next.canRedo })
  if (next.error && next.error !== previous?.error) showToast(next.error, 'error')
}
function serialized<T>(work: () => Promise<T>) {
  const result = tail.catch(() => undefined).then(work)
  tail = result.catch(() => undefined); return result
}
async function call(method: string, args: unknown[] = []) {
  // Capture the edit revision when the action is requested, before it waits in
  // the queue. Cancelling an overlay must not rebase an already queued commit.
  const op = args[0] as any
  const revision = method === 'editText' || method === 'editTableCell' ? textRevision ?? state.revision
    : ['editTransform', 'editTransformMulti', 'batchEditTransform'].includes(method) ? gestureRevision ?? state.revision : state.revision
  return serialized(async () => {
    try {
      const result = await request('editor', { method, args, expectedRevision: revision, requestId: crypto.randomUUID() })
      if (result.state) publish(result.state)
      if (['editTransform', 'editTransformMulti', 'batchEditTransform'].includes(method) && result.state) gestureRevision = op?.preview ? result.state.revision : undefined
      if (method === 'editText' || method === 'editTableCell') textRevision = undefined
      return result.value
    } catch (error: any) {
      // Many official actions are fire-and-forget; always show an actionable error.
      showToast(error.message, 'error')
      if (method === 'editText' || method === 'editTableCell') {
        emit('EditConflict', { method, args })
        const overlay = document.querySelector<HTMLElement>('.slide-text-editor')
        if (overlay) { overlay.dataset.ffcConflict = 'true'; overlay.focus() }
      }
      return null
    }
  })
}
const prefs = { ...DEFAULT_AI_PANEL_PREFS, showAiSidebar: false, openInNewDocs: false }
const local: Record<string, (...args: any[]) => any> = {
  getLanguage: async () => capability.locale?.startsWith('en') ? 'en' : capability.locale?.startsWith('ja') ? 'ja' : 'zh',
  getTheme: async () => 'system', getAiPanelPrefs: async () => prefs,
  setAiPanelPrefs: async () => prefs, getAutoSaveDefault: async () => ({ on: true, updatedAt: 1 }),
  getAiSettings: async () => ({ provider: 'openai', providers: { openai: { apiKey: '', model: '' } } }),
  aiGskStatus: async () => ({ loggedIn: false }),
  privateFontFaces: async () => [], privateFontData: async () => null, fontCatalog: async () => [], fontMissing: async () => [],
  consumeHeadlessExport: async () => null, headlessExportDone: () => undefined,
  getRecentFiles: async () => [], clipboardExternal: async () => ({ kind: 'none' }),
  consumePendingOpen: () => call('open'),
  isDirty: async () => state?.revision !== state?.savedRevision,
  cloudGenStatus: async () => ({ available: false }),
  saveAs: async () => ({ ok: false, error: '请在 FFC Agent 中另存文档副本；原文件未更改。' }),
  nativeClipboard: async (command: string) => document.execCommand(command),
  setShowFullScreen: async (value: boolean) => { if (value) await document.documentElement.requestFullscreen?.(); else if (document.fullscreenElement) await document.exitFullscreen() },
  getAiUiPrefs: async () => ({}),
}
function api() {
  return new Proxy(local, { get(target, name: string) {
    if (name in target) return target[name]
    if (name.startsWith('on')) return (handler: (data: any) => void) => on(name.slice(2), handler)
    return (...args: unknown[]) => call(name, args)
  } })
}
export async function connectSlidesBridge() {
  await new Promise<void>(resolve => {
    const attach = (event: MessageEvent) => {
      if (event.source !== parent || event.data?.type !== 'ffc-office-connect' || capability) return
      if (typeof event.data.id !== 'string' || typeof event.data.token !== 'string') return
      capability = event.data; window.removeEventListener('message', attach); resolve()
    }
    window.addEventListener('message', attach)
    parent.postMessage({ type: 'ffc-office-ready' }, '*')
  })
  state = await request('state', { since: -1 })
  while (!state.slides?.length && !disposed) {
    document.getElementById('root')!.textContent = state.error || '等待文档写入…'
    await new Promise(resolve => setTimeout(resolve, 200))
    state = await request('state', { since: -1 })
  }
  ;(window as any).slidesApi = api()
  // Preserve the revision at the start of a native text edit or geometry gesture.
  document.addEventListener('focusin', event => {
    if ((event.target as HTMLElement)?.isContentEditable) textRevision ??= state.revision
  })
  const editLifecycle = new MutationObserver(() => {
    if (!document.querySelector('.slide-text-editor')) textRevision = undefined
  })
  editLifecycle.observe(document.getElementById('root')!, { childList: true, subtree: true })
  document.addEventListener('pointerdown', event => {
    if ((event.target as HTMLElement)?.tagName === 'CANVAS') gestureRevision = state.revision
  })
  window.addEventListener('pagehide', () => { disposed = true; editLifecycle.disconnect() })
  async function poll() {
    try { await serialized(async () => publish(await request('state', { since: state.sequence }))) }
    catch (error: any) { showToast(error.message, 'error') }
    finally { if (!disposed) setTimeout(poll, document.hidden ? 1200 : 200) }
  }
  setTimeout(poll, 200)
}
