/** 每条对话自己的模型、思考强度和批准方式。只记 id，正文仍在 CLI。 */

export const CAT_CONV_PREFS_KEY = 'ffmpegfree.cat.convPrefs'

/** 批准方式。没记过按全部批准。 */
export type CatAccessId = 'normal' | 'plan' | 'auto' | 'approve'

export interface CatConvPref {
  model: string
  think: string
  /** 空或不写 = 全部批准。全部批准不写进记录，避免把没选过和选了全部批准分成两套。 */
  access?: string
}

export type CatConvPrefMap = Record<string, CatConvPref>

export function readConvPrefs(raw: string | null | undefined): CatConvPrefMap {
  if (!raw) return {}
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return {}
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
  const out: CatConvPrefMap = {}
  for (const [id, row] of Object.entries(parsed)) {
    if (!id.trim() || !row || typeof row !== 'object' || Array.isArray(row)) continue
    const rec = row as { model?: unknown; think?: unknown; access?: unknown }
    const model = typeof rec.model === 'string' ? rec.model : ''
    const think = typeof rec.think === 'string' ? rec.think : ''
    const access = typeof rec.access === 'string' ? rec.access : ''
    if (!model && !think && !access) continue
    const pref: CatConvPref = { model, think }
    if (access) pref.access = access
    out[id] = pref
  }
  return out
}

export function writeConvPrefs(map: CatConvPrefMap): string {
  return JSON.stringify(map)
}

export function prefOf(map: CatConvPrefMap, convId: string): CatConvPref | null {
  if (!convId) return null
  return map[convId] ?? null
}

/** 只改这一条，其它对话的记录原样留着 */
export function putPref(map: CatConvPrefMap, convId: string, pref: CatConvPref): CatConvPrefMap {
  if (!convId) return map
  const row: CatConvPref = { model: pref.model, think: pref.think }
  if (pref.access && pref.access !== 'approve') row.access = pref.access
  return { ...map, [convId]: row }
}

/** 界面上用的批准方式。不认识的 id 显示全部批准，不在这里改记录。 */
export function resolveAccess(id: string | undefined | null): CatAccessId {
  const s = (id || '').trim().toLowerCase().replace(/[\s_]/g, '-')
  if (s === 'normal' || s === 'ask' || s === 'default') return 'normal'
  if (s === 'plan') return 'plan'
  if (s === 'auto' || s === 'auto-review' || s === 'autoreview') return 'auto'
  if (s === 'approve' || s === 'always-approve' || s === 'full') return 'approve'
  return 'approve'
}

/** 欢迎页发出去变成正式对话时，把当时选的模型和强度抄到新 id 上。欢迎页自己的记录留着。 */
export function carryPref(map: CatConvPrefMap, fromId: string, toId: string): CatConvPrefMap {
  if (!fromId || !toId || fromId === toId) return map
  const src = map[fromId]
  if (!src) return map
  return putPref(map, toId, src)
}

export function dropPref(map: CatConvPrefMap, convId: string): CatConvPrefMap {
  if (!convId || !Object.prototype.hasOwnProperty.call(map, convId)) return map
  const next = { ...map }
  delete next[convId]
  return next
}

/**
 * 把记下的选择对准当前列表。
 * 模型还在列表里就用记下的，否则用第一个；强度还在就用记下的，否则留空（显示「默认」，发送不带）。
 * 没记过也一样：模型用第一个，强度留空。
 */
export function resolveConvPref(
  pref: CatConvPref | null,
  models: readonly { id: string }[],
  thinks: readonly { id: string }[],
): CatConvPref {
  const modelOk = !!pref?.model && models.some((m) => m.id === pref.model)
  const thinkOk = !!pref?.think && thinks.some((t) => t.id === pref.think)
  return {
    model: modelOk ? pref!.model : (models[0]?.id ?? ''),
    think: thinkOk ? pref!.think : '',
  }
}
