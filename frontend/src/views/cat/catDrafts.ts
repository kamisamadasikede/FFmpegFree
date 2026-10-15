import { reactive, watch } from 'vue'
import type { CatOutgoingFile } from '@/components/cat/catAttach'
import type { PickNote } from '@/components/cat/catPick'

export interface CatDraft {
  text: string
  files: CatOutgoingFile[]
  notes: PickNote[]
}

const STORAGE_KEY = 'ffc.agent.drafts.v1'
const drafts = reactive(new Map<string, CatDraft>())
const noteFields = ['file', 'tag', 'id', 'classes', 'selector', 'text', 'styles', 'snippet', 'note'] as const

export function catDraftKey(convId: string, projectId = ''): string {
  return JSON.stringify(convId && convId !== 'new' ? ['conversation', convId] : ['new', projectId])
}

function copyDraft(value: CatDraft): CatDraft {
  return {
    text: value.text,
    files: value.files.map(({ name, path }) => ({ name, path })),
    notes: value.notes.map((n) => ({ ...Object.fromEntries(noteFields.map((field) => [field, n[field]])), intent: n.intent }) as PickNote),
  }
}

// Session storage keeps drafts across page reloads without writing conversation
// content into the long-lived application preferences.
try {
  const saved: unknown = JSON.parse(sessionStorage.getItem(STORAGE_KEY) || '[]')
  if (Array.isArray(saved)) for (const entry of saved.slice(-50)) {
    if (!Array.isArray(entry) || typeof entry[0] !== 'string') continue
    const d = entry[1]
    if (!d || typeof d.text !== 'string' || !Array.isArray(d.files) || !Array.isArray(d.notes)) continue
    if (!d.files.every((f: CatOutgoingFile) => f && typeof f.path === 'string' && typeof f.name === 'string')) continue
    if (!d.notes.every((n: PickNote) => n && ['edit', 'ask'].includes(n.intent) && noteFields.every((field) => typeof n[field] === 'string'))) continue
    drafts.set(entry[0], copyDraft(d))
  }
} catch { /* Drafts still work in memory when storage is unavailable. */ }

export function catDraftFor(key: string): CatDraft {
  if (!drafts.has(key)) drafts.set(key, { text: '', files: [], notes: [] })
  return drafts.get(key)!
}

/** Rejected sends return to their original conversation even after navigation. */
export function restoreCatDraft(key: string, failed: CatDraft): void {
  failed = copyDraft(failed)
  const current = catDraftFor(key)
  if (failed.text && failed.text !== current.text) current.text = current.text.trim() ? `${failed.text}\n\n${current.text}` : failed.text
  const pathKey = (path: string) => path.replace(/\\/g, '/').toLowerCase()
  const paths = new Set(current.files.map((f) => pathKey(f.path)))
  current.files = [...failed.files.filter((f) => !paths.has(pathKey(f.path))), ...current.files].map((f) => ({ ...f }))
  const noteKey = (n: PickNote) => JSON.stringify([n.intent, ...noteFields.map((field) => n[field])])
  const existing = new Set(current.notes.map(noteKey))
  current.notes = [...failed.notes.filter((n) => !existing.has(noteKey(n))), ...current.notes].map((n) => ({ ...n }))
}

let persistTimer: ReturnType<typeof setTimeout> | undefined
function persistDrafts() {
  clearTimeout(persistTimer)
  try {
    const nonempty = [...drafts].filter(([, d]) => d.text || d.files.length || d.notes.length)
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(nonempty.slice(-50).map(([key, d]) => [key, copyDraft(d)])))
  } catch { /* A full or disabled storage must not prevent typing. */ }
}
watch(drafts, () => {
  clearTimeout(persistTimer)
  persistTimer = setTimeout(persistDrafts, 250)
}, { deep: true, flush: 'sync' })
if (typeof window !== 'undefined' && window.addEventListener) window.addEventListener('pagehide', persistDrafts)
