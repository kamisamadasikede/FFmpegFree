import { CAT_GROUP_POSITIONS_KEY } from '@/api/catStorageKeys'

export interface CatGroupPosition {
  atBottom: boolean
  seq: number
  offset: number
}

function positions(): Array<[string, CatGroupPosition]> {
  try {
    const raw: unknown = JSON.parse(globalThis.localStorage?.getItem(CAT_GROUP_POSITIONS_KEY) || '[]')
    if (!Array.isArray(raw)) return []
    return raw.filter((row): row is [string, CatGroupPosition] => {
      if (!Array.isArray(row) || typeof row[0] !== 'string' || !row[0] || !row[1]) return false
      const p = row[1]
      return typeof p.atBottom === 'boolean' && Number.isSafeInteger(p.seq) && p.seq >= 0 && Number.isFinite(p.offset)
    })
  } catch { return [] }
}

export function readGroupPosition(id: string): CatGroupPosition | undefined {
  return positions().find(([key]) => key === id)?.[1]
}

export function saveGroupPosition(id: string, position: CatGroupPosition | null) {
  if (!id) return
  try {
    const saved = positions().filter(([key]) => key !== id)
    if (position) saved.push([id, position])
    globalThis.localStorage?.setItem(CAT_GROUP_POSITIONS_KEY, JSON.stringify(saved.slice(-50)))
  } catch { /* Keep chat usable when local storage is full or unavailable. */ }
}
