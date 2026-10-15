import { computed, reactive } from 'vue'
import { callService, hasServiceBinding } from '@/api/call'
import { mapCatStatus, type CatStatus } from '@/api/cat'
import { CAT_PROVIDER_ORDER, isProviderKind, type CatProviderKind } from '@/api/catAgents'
import { t } from '@/i18n'

export interface CatCLIUpdate {
  agentKind: CatProviderKind
  currentVersion: string
  latestVersion: string
  available: boolean
  canUpdate: boolean
  updateUrl?: string
}

export const catCLIUpdates = reactive({
  rows: [] as CatCLIUpdate[],
  dismissed: false,
  dialog: false,
  updating: {} as Partial<Record<CatProviderKind, boolean>>,
  errors: {} as Partial<Record<CatProviderKind, string>>,
})
export const availableCLIUpdates = computed(() => catCLIUpdates.rows.filter((r) => r.available))
let visit = 0
let checkSequence = 0
const revisions: Partial<Record<CatProviderKind, number>> = {}

export function enterCLIUpdateVisit(): number {
  visit++
  catCLIUpdates.dismissed = false
  catCLIUpdates.dialog = false
  if (!Object.values(catCLIUpdates.updating).some(Boolean)) {
    catCLIUpdates.rows = []
    catCLIUpdates.errors = {}
  }
  return visit
}

export function leaveCLIUpdateVisit(): void {
  visit++
  catCLIUpdates.dialog = false
}

export function dismissCLIUpdates(): void {
  catCLIUpdates.dismissed = true
  catCLIUpdates.dialog = false
}

function mapUpdates(raw: unknown): CatCLIUpdate[] {
  if (!Array.isArray(raw)) return []
  return raw.filter((r) => r && isProviderKind(r.agentKind) && typeof r.currentVersion === 'string' && typeof r.latestVersion === 'string')
    .map((r) => ({ agentKind: r.agentKind, currentVersion: r.currentVersion, latestVersion: r.latestVersion,
      available: r.available === true, canUpdate: r.canUpdate === true,
      updateUrl: typeof r.updateUrl === 'string' && r.updateUrl.startsWith('https://developers.openai.com/') ? r.updateUrl : undefined }))
}

/** A result from a page that has already been left must not reopen a reminder. */
export async function checkCLIUpdates(token: number, prompt = true): Promise<void> {
  if (token !== visit || !hasServiceBinding('CatService', 'CheckCatCLIUpdates')) return
  const sequence = ++checkSequence
  const started = { ...revisions }
  const busy = { ...catCLIUpdates.updating }
  try {
    const rows = mapUpdates(await callService<unknown>('CatService', 'CheckCatCLIUpdates'))
    if (token !== visit || sequence !== checkSequence) return
    // A check started before/during an install must not restore its old version.
    catCLIUpdates.rows = CAT_PROVIDER_ORDER.flatMap((kind) => {
      const protectedRow = busy[kind] || catCLIUpdates.updating[kind] || started[kind] !== revisions[kind]
      // Missing rows mean an offline/failed lookup, not proof of a current version.
      const previous = catCLIUpdates.rows.find((r) => r.agentKind === kind)
      const row = protectedRow ? previous : rows.find((r) => r.agentKind === kind) ?? previous
      return row ? [row] : []
    })
    if (!availableCLIUpdates.value.length) catCLIUpdates.dialog = false
    else if (prompt && !catCLIUpdates.dismissed) catCLIUpdates.dialog = true
  } catch { /* Offline version checks remain silent. */ }
}

function reachedVersion(current: string, target: string): boolean {
  const pattern = /(\d+)\.(\d+)(?:\.(\d+))?(-[\w.-]+)?/
  const a = current.match(pattern)
  const b = target.match(pattern)
  if (!a || !b) return false
  for (let i = 1; i <= 3; i++) {
    const diff = Number(a[i] ?? 0) - Number(b[i] ?? 0)
    if (diff) return diff > 0
  }
  return !a[4] || a[4] === b[4]
}

function replaceRow(row: CatCLIUpdate): void {
  const index = catCLIUpdates.rows.findIndex((r) => r.agentKind === row.agentKind)
  if (index < 0) catCLIUpdates.rows.push(row)
  else catCLIUpdates.rows[index] = row
  if (!availableCLIUpdates.value.length) catCLIUpdates.dialog = false
}

export async function updateCatCLI(row: CatCLIUpdate, applyStatus?: (status: CatStatus) => void): Promise<CatStatus | null> {
  const kind = row.agentKind
  if (catCLIUpdates.updating[kind] || !row.canUpdate) return null
  revisions[kind] = (revisions[kind] ?? 0) + 1
  catCLIUpdates.updating[kind] = true
  delete catCLIUpdates.errors[kind]
  try {
    const status = mapCatStatus(await callService<unknown>('CatService', 'UpdateCatCLI', kind))
    applyStatus?.(status)
    const agent = status.agents.find((a) => a.agentKind === kind)
    // The install already rechecks the executable. Hide its reminder immediately
    // when that local version reaches the offered release, even while offline.
    if (agent?.ready && !agent.checking && !agent.stale && reachedVersion(agent.version, row.latestVersion)) {
      replaceRow({ ...row, currentVersion: agent.version, available: false })
    } else {
      let verified: CatCLIUpdate | undefined
      try {
        verified = mapUpdates(await callService<unknown>('CatService', 'CheckCatCLIUpdates')).find((r) => r.agentKind === kind)
      } catch { /* Keep only this CLI's reminder if verification is unavailable. */ }
      if (verified && agent?.ready && !agent.checking && !agent.stale) {
        replaceRow(verified)
        if (verified.available) catCLIUpdates.errors[kind] = t('cat.cliUpdate.stillOld')
      } else {
        replaceRow(row)
        catCLIUpdates.errors[kind] = t('cat.cliUpdate.verifyFailed')
      }
    }
    return status
  } catch {
    replaceRow(row)
    catCLIUpdates.errors[kind] = t('cat.cliUpdate.failed')
    return null
  } finally {
    delete catCLIUpdates.updating[kind]
    revisions[kind] = (revisions[kind] ?? 0) + 1
  }
}
