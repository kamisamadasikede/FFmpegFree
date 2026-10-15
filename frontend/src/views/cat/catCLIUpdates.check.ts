import { availableCLIUpdates, catCLIUpdates, checkCLIUpdates, dismissCLIUpdates, enterCLIUpdateVisit, leaveCLIUpdateVisit, updateCatCLI } from './catCLIUpdates'

type Eq = (name: string, got: unknown, want: unknown) => void

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((ok, fail) => { resolve = ok; reject = fail })
  return { promise, resolve, reject }
}

export async function catCLIUpdatesChecks(eq: Eq): Promise<void> {
  const win = window as unknown as Record<string, unknown>
  const go = win.go
  const saved = { ...catCLIUpdates, rows: catCLIUpdates.rows.slice(), updating: { ...catCLIUpdates.updating }, errors: { ...catCLIUpdates.errors } }
  const row = { agentKind: 'codex_cli' as const, currentVersion: '0.162.0', latestVersion: '0.162.1', available: true, canUpdate: true }
  const claude = { ...row, agentKind: 'claude_cli' as const, currentVersion: '2.1.159', latestVersion: '2.1.299' }
  const status = (kind: string, version: string) => ({ state: 'missing', agents: [{ agentKind: kind, ready: true, version }] })
  let installed = false
  let fail = false
  let offline = false
  let installs = 0
  let checks = 0
  let installVersion = row.latestVersion
  const service = {
    CheckCatCLIUpdates: async (): Promise<unknown> => {
      checks++
      return offline ? [] : [{ ...row, available: !installed, currentVersion: installed ? installVersion : row.currentVersion }]
    },
    UpdateCatCLI: async (kind: string): Promise<unknown> => {
      installs++
      if (fail) throw new Error('installer failed')
      installed = true
      return status(kind, installVersion)
    },
  }
  win.go = { app: { CatService: service } }
  try {
    let token = enterCLIUpdateVisit()
    await checkCLIUpdates(token)
    eq('CLI 更新：发现新版自动打开弹窗，但不运行安装', [catCLIUpdates.dialog, availableCLIUpdates.value.length, installs], [true, 1, 0])
    dismissCLIUpdates()
    await checkCLIUpdates(token)
    eq('CLI 更新：关闭后本次进入不重复提醒', [catCLIUpdates.dialog, catCLIUpdates.dismissed], [false, true])
    token = enterCLIUpdateVisit()
    await checkCLIUpdates(token)
    eq('CLI 更新：下次进入仍有新版时再提醒', catCLIUpdates.dialog, true)
    let applied = false
    offline = true
    const previousChecks = checks
    const result = await updateCatCLI(row, () => { applied = true })
    eq('CLI 更新：本地确认新版后立即收起，不等待网络检查', [installs, result?.agents[0].ready, availableCLIUpdates.value.length, catCLIUpdates.dialog, catCLIUpdates.updating.codex_cli, applied, checks], [1, true, 0, false, undefined, true, previousChecks])

    offline = installed = false
    token = enterCLIUpdateVisit()
    await checkCLIUpdates(token)
    fail = true
    await updateCatCLI(row)
    eq('CLI 更新：失败保留提醒并解除该 CLI 忙状态', [availableCLIUpdates.value.length, !!catCLIUpdates.errors.codex_cli, catCLIUpdates.updating.codex_cli], [1, true, undefined])
    fail = false
    offline = true
    installVersion = row.currentVersion
    await updateCatCLI(row)
    eq('CLI 更新：无法验证新版本时保留提醒', [availableCLIUpdates.value.length, !!catCLIUpdates.errors.codex_cli], [1, true])

    // A partial/offline lookup is not evidence that a known update disappeared.
    installed = false
    installVersion = row.latestVersion
    offline = false
    token = enterCLIUpdateVisit()
    await checkCLIUpdates(token)
    offline = true
    await checkCLIUpdates(token, false)
    eq('CLI 更新：离线空结果保留已知版本提醒', availableCLIUpdates.value.map((r) => r.agentKind), [row.agentKind])
    const olderCheck = deferred<unknown>()
    const newerCheck = deferred<unknown>()
    let lookup = 0
    service.CheckCatCLIUpdates = () => (++lookup === 1 ? olderCheck.promise : newerCheck.promise)
    const olderPending = checkCLIUpdates(token)
    const newerPending = checkCLIUpdates(token)
    newerCheck.resolve([{ ...row, available: false, currentVersion: row.latestVersion }])
    await newerPending
    olderCheck.resolve([row])
    await olderPending
    eq('CLI 更新：同次访问迟到的旧检查不能覆盖较新的版本结果', [availableCLIUpdates.value.length, catCLIUpdates.rows[0]?.currentVersion], [0, row.latestVersion])

    // Hold two installers and a stale full check to exercise completion order.
    const codexInstall = deferred<unknown>()
    const claudeInstall = deferred<unknown>()
    const lateCheck = deferred<unknown>()
    const requested: string[] = []
    service.CheckCatCLIUpdates = async () => [claude, row]
    token = enterCLIUpdateVisit()
    await checkCLIUpdates(token)
    service.UpdateCatCLI = (kind) => {
      requested.push(kind)
      return kind === row.agentKind ? codexInstall.promise : claudeInstall.promise
    }
    service.CheckCatCLIUpdates = () => lateCheck.promise
    const stale = checkCLIUpdates(token)
    const codexPending = updateCatCLI(row)
    const claudePending = updateCatCLI(claude)
    eq('CLI 更新：不同 CLI 可同时更新', [requested, catCLIUpdates.updating.codex_cli, catCLIUpdates.updating.claude_cli], [[row.agentKind, claude.agentKind], true, true])
    eq('CLI 更新：同一 CLI 不重复安装', await updateCatCLI(row), null)
    dismissCLIUpdates()
    claudeInstall.reject(new Error('offline'))
    await claudePending
    eq('CLI 更新：一个失败不影响另一个后台安装', [catCLIUpdates.updating.codex_cli, catCLIUpdates.updating.claude_cli, !!catCLIUpdates.errors.claude_cli, !!catCLIUpdates.errors.codex_cli], [true, undefined, true, false])
    codexInstall.resolve(status(row.agentKind, 'codex-cli 0.163.0'))
    await codexPending
    lateCheck.resolve([claude, row])
    await stale
    eq('CLI 更新：迟到的旧检查不恢复已更新提醒，也不清除其他错误', [availableCLIUpdates.value.map((r) => r.agentKind), !!catCLIUpdates.errors.claude_cli, catCLIUpdates.dialog, catCLIUpdates.dismissed], [[claude.agentKind], true, false, true])

    // An install survives page changes, and its completion cannot reopen a popup.
    const background = deferred<unknown>()
    service.CheckCatCLIUpdates = async () => [claude]
    service.UpdateCatCLI = () => background.promise
    const pendingInstall = updateCatCLI(claude)
    leaveCLIUpdateVisit()
    token = enterCLIUpdateVisit()
    await checkCLIUpdates(token)
    eq('CLI 更新：重新进入保留独立后台更新状态', catCLIUpdates.updating.claude_cli, true)
    dismissCLIUpdates()
    background.resolve(status(claude.agentKind, claude.latestVersion))
    await pendingInstall
    eq('CLI 更新：后台完成自动收起最后一条提醒，不重开已关闭弹窗', [availableCLIUpdates.value.length, catCLIUpdates.dialog, catCLIUpdates.dismissed, Object.keys(catCLIUpdates.updating).length], [0, false, true, 0])

    token = enterCLIUpdateVisit()
    const pendingCheck = checkCLIUpdates(token)
    leaveCLIUpdateVisit()
    await pendingCheck
    eq('CLI 更新：离开后迟到结果不打开弹窗', [catCLIUpdates.dialog, catCLIUpdates.rows.length], [false, 0])
  } finally {
    leaveCLIUpdateVisit()
    if (go === undefined) delete win.go
    else win.go = go
    Object.assign(catCLIUpdates, saved)
  }
}
