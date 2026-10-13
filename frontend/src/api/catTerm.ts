import { callService } from '@/api/call'
import { onEvent } from '@/services/wails'

export const CAT_TERM_EVENT = 'cat:term'

export interface CatTermEvent {
  id: string
  data: string
  exit?: number
}

export function mapTermEvent(raw: unknown): CatTermEvent | null {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Record<string, unknown>
  const id = typeof r.id === 'string' ? r.id : ''
  if (!id) return null
  const exit = typeof r.exit === 'number' && Number.isFinite(r.exit) ? r.exit : undefined
  return { id, data: typeof r.data === 'string' ? r.data : '', exit }
}

export async function startCatTerm(convId: string): Promise<{ id: string; console: boolean }> {
  const raw = await callService<unknown>('CatService', 'StartCatTerm', { convId })
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return {
    id: typeof r.id === 'string' ? r.id : '',
    console: r.console === true,
  }
}

export async function writeCatTerm(id: string, data: string): Promise<void> {
  await callService('CatService', 'WriteCatTerm', { id, data })
}

export async function stopCatTerm(id: string): Promise<void> {
  await callService('CatService', 'StopCatTerm', { id })
}

export async function resizeCatTerm(id: string, cols: number, rows: number): Promise<void> {
  await callService('CatService', 'ResizeCatTerm', { id, cols, rows })
}

export function onCatTerm(cb: (ev: CatTermEvent) => void): () => void {
  return onEvent(CAT_TERM_EVENT, (raw) => {
    const ev = mapTermEvent(raw)
    if (ev) cb(ev)
  })
}
