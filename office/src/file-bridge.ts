/** Browser stand-in for the official Electron preloads.
 * The renderer sources stay unmodified. This host only supplies the file they open.
 */
import { DEFAULT_AI_PANEL_PREFS } from '@genoffice/ui/ai-panel-prefs'

export type OfficeCap = { id: string; token: string; locale?: string }
type Listener = (value?: any) => void
type Meta = { name: string; path: string; revision: string; size: number }

const prefs = { ...DEFAULT_AI_PANEL_PREFS, showAiSidebar: false, openInNewDocs: false }

export function connectOffice(): Promise<OfficeCap> {
  return new Promise(resolve => {
    const attach = (event: MessageEvent) => {
      if (event.source !== parent || event.data?.type !== 'ffc-office-connect') return
      if (typeof event.data.id !== 'string' || typeof event.data.token !== 'string') return
      window.removeEventListener('message', attach)
      resolve({ id: event.data.id, token: event.data.token, locale: event.data.locale })
    }
    window.addEventListener('message', attach)
    parent.postMessage({ type: 'ffc-office-ready' }, '*')
  })
}

function auth(cap: OfficeCap) { return { Authorization: 'Bearer ' + cap.token } }
function lang(cap: OfficeCap) {
  return cap.locale?.startsWith('en') ? 'en' : cap.locale?.startsWith('ja') ? 'ja' : 'zh'
}
async function meta(cap: OfficeCap): Promise<Meta> {
  const response = await fetch('/meta?id=' + encodeURIComponent(cap.id), { headers: auth(cap) })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(body.error || '文件读取失败')
  return body
}
async function readBytes(cap: OfficeCap) {
  const response = await fetch('/bytes?id=' + encodeURIComponent(cap.id), { headers: auth(cap) })
  if (!response.ok) throw new Error('文件读取失败')
  return response.arrayBuffer()
}
async function writeBytes(cap: OfficeCap, data: ArrayBuffer | Uint8Array) {
  const body = data instanceof Uint8Array ? data : new Uint8Array(data)
  const response = await fetch('/bytes?id=' + encodeURIComponent(cap.id), {
    method: 'POST', headers: { ...auth(cap), 'Content-Type': 'application/octet-stream' }, body,
  })
  const result = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(result.error || '保存失败')
  return result as { revision: string }
}
function events() {
  const map = new Map<string, Set<Listener>>()
  return {
    on(name: string, fn: Listener) {
      const set = map.get(name) || new Set<Listener>()
      map.set(name, set); set.add(fn)
      return () => { set.delete(fn) }
    },
    emit(name: string, value?: any) { for (const fn of map.get(name) || []) fn(value) },
  }
}
function api(local: Record<string, any>, bus: ReturnType<typeof events>) {
  return new Proxy(local, { get(target, name: string) {
    if (typeof name !== 'string') return undefined
    if (name in target) return target[name]
    if (name.startsWith('on')) return (handler: Listener) => bus.on(name.slice(2), handler)
    if (name.startsWith('list')) return async () => []
    return async () => null
  } })
}
function editing() {
  const node = document.activeElement as HTMLElement | null
  return !!node && (node.isContentEditable || node.tagName === 'INPUT' || node.tagName === 'TEXTAREA')
}
function watchFile(cap: OfficeCap, state: { revision: string; skip: () => boolean }, onChange: () => void) {
  let disposed = false
  const tick = async () => {
    try {
      const info = await meta(cap)
      if (state.revision && info.revision !== state.revision && !state.skip()) onChange()
      else state.revision = info.revision
    } catch { /* keep the page that is already open */ }
    finally { if (!disposed) setTimeout(tick, document.hidden ? 1200 : 400) }
  }
  setTimeout(tick, 400)
  window.addEventListener('pagehide', () => { disposed = true })
}

export function installDocs(cap: OfficeCap) {
  const bus = events()
  const state = { revision: '', skip: () => editing() }
  let consumed = false
  async function opened() {
    const info = await meta(cap)
    const buf = await readBytes(cap)
    state.revision = info.revision
    const digest = await crypto.subtle.digest('SHA-256', buf)
    const hash = [...new Uint8Array(digest)].map(part => part.toString(16).padStart(2, '0')).join('')
    return {
      path: info.path, name: info.name, hash,
      dataUrl: URL.createObjectURL(new Blob([buf], { type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' })),
    }
  }
  const local: Record<string, any> = {
    getLanguage: async () => lang(cap),
    getTheme: async () => 'system',
    getDocumentTheme: async () => 'follow',
    getAiPanelPrefs: async () => prefs,
    setAiPanelPrefs: async () => prefs,
    getAutoSaveDefault: async () => ({ on: true, updatedAt: 1 }),
    getAiSettings: async () => ({ provider: 'openai', providers: { openai: { apiKey: '', model: '' } } }),
    consumeNewBlankDoc: async () => false,
    consumeAiDocContent: async () => null,
    consumeHeadlessExport: async () => null,
    confirmDocumentReplace: async () => true,
    getRecentFiles: async () => [],
    consumePendingOpenDocx: async () => {
      if (consumed) return null
      consumed = true
      return opened()
    },
    saveDocx: async (path: string, data: ArrayBuffer) => {
      const info = await meta(cap)
      if (path !== info.path) return { ok: false, error: '请保存当前打开的文件' }
      const saved = await writeBytes(cap, data)
      state.revision = saved.revision
      return { ok: true }
    },
    saveDocxNew: async (_name: string, data: ArrayBuffer) => {
      const saved = await writeBytes(cap, data)
      state.revision = saved.revision
      return { ok: true, path: (await meta(cap)).path }
    },
    saveDocxAs: async () => ({ ok: false, error: '请直接保存当前文件' }),
  }
  ;(window as any).desktop = api(local, bus)
  watchFile(cap, state, () => { if (consumed) void opened().then(result => bus.emit('OpenDocx', result)).catch(() => undefined) })
}

export function installPdf(cap: OfficeCap) {
  const bus = events()
  let pending = true, dirty = false, reloading = false
  const state = { revision: '', skip: () => dirty || reloading }
  const local: Record<string, any> = {
    getLanguage: async () => lang(cap),
    getTheme: async () => 'system',
    getAiPanelPrefs: async () => prefs,
    consumePending: async () => {
      if (!pending) return null
      pending = false
      const info = await meta(cap)
      state.revision = info.revision
      return info.path
    },
    readFile: async (path: string) => {
      const info = await meta(cap)
      if (path !== info.path) throw new Error('路径不匹配')
      state.revision = info.revision
      return readBytes(cap)
    },
    save: async () => ({ ok: false, error: '这个页面可以查看文件。批注写回还没有接到当前文件。' }),
    setDirty: (value: boolean) => { dirty = !!value },
    isUntitled: async () => false,
  }
  ;(window as any).pdfApi = api(local, bus)
  watchFile(cap, state, () => { reloading = true; location.reload() })
}

function installText(cap: OfficeCap, globalName: 'htmlApi' | 'markdownApi', preview: boolean) {
  const bus = events()
  let pending = true, dirty = false, reloading = false
  const state = { revision: '', skip: () => dirty || reloading }
  const local: Record<string, any> = {
    getLanguage: async () => lang(cap),
    getTheme: async () => 'system',
    getDocumentTheme: async () => 'follow',
    getMarkdownTheme: async () => 'default',
    getAiPanelPrefs: async () => prefs,
    consumePending: async () => {
      if (!pending) return null
      pending = false
      const info = await meta(cap)
      state.revision = info.revision
      return info.path
    },
    readFile: async (path: string) => {
      const info = await meta(cap)
      if (path !== info.path) throw new Error('路径不匹配')
      state.revision = info.revision
      return new TextDecoder().decode(await readBytes(cap))
    },
    save: async (request: { text?: string; mode?: string }) => {
      if (request?.mode === 'saveAs') return { ok: false, error: '请直接保存当前文件' }
      const saved = await writeBytes(cap, new TextEncoder().encode(request?.text || ''))
      state.revision = saved.revision
      return { ok: true, path: (await meta(cap)).path }
    },
    setDirty: (value: boolean) => { dirty = !!value },
    resolveWikiLinks: async () => ({}),
    takeWikiFragment: async () => '',
    rememberWikiFragment: async () => undefined,
    updatePreview: (text: string) => {
      if (!preview) return
      void fetch('/preview?id=' + encodeURIComponent(cap.id), {
        method: 'POST', headers: { ...auth(cap), 'Content-Type': 'text/html; charset=utf-8' }, body: text,
      }).catch(() => undefined)
    },
    getPreviewInfo: async () => ({ url: location.origin + '/preview/' + cap.id }),
  }
  ;(window as any)[globalName] = api(local, bus)
  watchFile(cap, state, () => { reloading = true; location.reload() })
}

export function installHtml(cap: OfficeCap) { installText(cap, 'htmlApi', true) }
export function installMarkdown(cap: OfficeCap) { installText(cap, 'markdownApi', false) }

async function sheetsCall(cap: OfficeCap, body: Record<string, unknown>) {
  const response = await fetch('/sheets?id=' + encodeURIComponent(cap.id), {
    method: 'POST', headers: { ...auth(cap), 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  })
  const result = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(result.error || '表格没有打开')
  return result
}

export function installSheets(cap: OfficeCap) {
  const bus = events()
  let opened = false
  const local: Record<string, any> = {
    getLanguage: async () => lang(cap),
    getTheme: async () => 'system',
    getDocumentTheme: async () => 'follow',
    getAiPanelPrefs: async () => prefs,
    setAiPanelPrefs: async () => prefs,
    getAiSettings: async () => ({ provider: 'openai', providers: { openai: { apiKey: '', model: '' } } }),
    hasQueuedWorkbook: async () => !opened,
    consumeNewBlankWorkbook: async () => false,
    consumeHeadlessExport: async () => null,
    getUserDisplayName: async () => '',
    selectWorkbook: async () => {
      opened = true
      return sheetsCall(cap, { command: 'open' })
    },
    readWorkbookRange: async (request: Record<string, unknown>) => sheetsCall(cap, { command: 'read_range', sheetId: request?.sheetId, range: request?.range }),
    readWorkbookFormulas: async (request: Record<string, unknown>) => sheetsCall(cap, { command: 'read_formula_cells', sheetId: request?.sheetId }),
    readWorkbookRowOutline: async (request: Record<string, unknown>) => sheetsCall(cap, { command: 'read_row_outline', sheetId: request?.sheetId }),
    readWorkbookMedia: async (request: Record<string, unknown>) => sheetsCall(cap, { command: 'read_media', visualId: request?.visualId }),
    closeWorkbook: async () => sheetsCall(cap, { command: 'close' }),
  }
  ;(window as any).desktopApi = api(local, bus)
}
