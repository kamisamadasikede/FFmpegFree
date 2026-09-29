// 不引入测试框架的自检：npm run check:ffmpeg（scripts/check-ffmpeg.mjs 用 esbuild 打包后在 node 里跑）。
// 用假的 window.go / window.runtime 让 ffmpeg store 走"Wails 里"的真实分支，验证安装完成后的状态与对话框。
// 调用前必须已经把 globalThis.window 设成 installFakeWails() 返回的对象（store 在模块加载时读一次 hasWailsBackend）。
import { createPinia, setActivePinia } from 'pinia'

type Handler = (payload: any) => void

export interface FakeWails {
  location: { search: string }
  go: any
  runtime: any
  /** 模拟后端推 ffmpeg:status */
  emit(event: string, payload: unknown): void
  calls: string[]
  /** 可覆盖的绑定行为 */
  impl: {
    getStatus: () => Promise<any>
    install: (mirror: string) => Promise<any>
    setPath: (dir: string) => Promise<any>
    recheck: () => Promise<any>
  }
}

export function installFakeWails(): FakeWails {
  const handlers = new Map<string, Set<Handler>>()
  const calls: string[] = []
  const impl: FakeWails['impl'] = {
    getStatus: async () => ({ state: 'missing', error: { code: 'FFMPEG_NOT_FOUND', message: '未找到可用的 ffmpeg' } }),
    install: async () => ({ id: 't1', progress: 0, speed: '', etaSec: 0 }),
    setPath: async () => ({ state: 'ready', path: '/x/ffmpeg', version: '7.1', source: 'custom' }),
    recheck: async () => ({ state: 'ready', path: '/x/ffmpeg', version: '7.1', source: 'bundled' }),
  }
  const rec = (n: string, f: (...a: any[]) => any) => (...a: any[]) => (calls.push(n), f(...a))
  const w: FakeWails = {
    location: { search: '' },
    calls,
    impl,
    emit(event, payload) {
      for (const h of [...(handlers.get(event) ?? [])]) h(payload)
    },
    runtime: {
      EventsOnMultiple(name: string, cb: Handler) {
        let s = handlers.get(name)
        if (!s) handlers.set(name, (s = new Set()))
        s.add(cb)
        return () => s!.delete(cb)
      },
      EventsOff() {},
    },
    go: {
      app: {
        SystemService: {
          GetFFmpegStatus: rec('GetFFmpegStatus', () => impl.getStatus()),
          GetInstallOptions: rec('GetInstallOptions', async () => ({ platform: 'windows-amd64', supported: true, mirrors: ['cn'] })),
          GetSettings: rec('GetSettings', async () => ({ ffmpegPath: '', ffmpegPromptDismissed: false })),
          UpdateSettings: rec('UpdateSettings', async () => {}),
          InstallFFmpeg: rec('InstallFFmpeg', (m: string) => impl.install(m)),
          CancelFFmpegInstall: rec('CancelFFmpegInstall', async () => {}),
          PickDirectory: rec('PickDirectory', async () => 'C:\\ffmpeg'),
          SetFFmpegPath: rec('SetFFmpegPath', (d: string) => impl.setPath(d)),
          RecheckFFmpeg: rec('RecheckFFmpeg', () => impl.recheck()),
        },
      },
    },
  }
  return w
}

const tick = () => new Promise<void>((r) => setTimeout(r, 0))
const READY = { state: 'ready', path: 'C:\\data\\bin\\ffmpeg.exe', version: '9.0.2', source: 'bundled', ffprobeMissing: false }

export async function runFFmpegChecks(w: FakeWails): Promise<string[]> {
  const fails: string[] = []
  const eq = (name: string, got: unknown, want: unknown) => {
    const a = JSON.stringify(got)
    const b = JSON.stringify(want)
    if (a !== b) fails.push(`✗ ${name}: 得到 ${a}，期望 ${b}`)
  }
  const { useFFmpegStore } = await import('./ffmpeg')
  const fresh = async () => {
    setActivePinia(createPinia())
    w.calls.length = 0
    const s = useFFmpegStore()
    await s.init()
    return s
  }

  // ---- 1. 主流程（老板遇到的 bug）：缺失 → 自动弹框 → 点下载 → 安装中 → 后端推 ready → 对话框必须消失 ----
  {
    const s = await fresh()
    eq('启动缺失：状态', s.status.state, 'missing')
    eq('启动缺失：首次自动弹框', s.dialogVisible, true)
    await s.startInstall('')
    eq('点下载后：installing', s.status.state, 'installing')
    eq('点下载后：对话框仍开着（进度视图）', s.dialogVisible, true)
    w.emit('ffmpeg:status', { state: 'installing', taskId: 't1' })
    w.emit('ffmpeg:status', READY)
    eq('安装完成：状态 ready', s.status.state, 'ready')
    eq('安装完成：dialogOpen 已复位', s.dialogOpen, false)
    eq('安装完成：对话框不再显示', s.dialogVisible, false)
    eq('安装完成：功能不再置灰', s.featuresBlocked, false)
    // 旧行为的回归：即使有别处把 dialogOpen 又置成 true（侧栏/守卫），ready 时也不显示
    s.dialogOpen = true
    eq('ready 时 dialogOpen=true 也不显示', s.dialogVisible, false)
    eq('没有多余的 InstallFFmpeg 调用', w.calls.filter((c) => c === 'InstallFFmpeg').length, 1)
  }

  // ---- 2. 「后台安装」关掉对话框后完成：保持关闭 ----
  {
    const s = await fresh()
    await s.startInstall('')
    s.dialogOpen = false
    w.emit('ffmpeg:status', READY)
    eq('后台安装完成：不显示', s.dialogVisible, false)
    eq('后台安装完成：ready', s.ready, true)
  }

  // ---- 3. 竞态：InstallFFmpeg 返回前后端已经推了 ready，返回值不能把状态改回 installing ----
  {
    const s = await fresh()
    w.impl.install = async () => {
      w.emit('ffmpeg:status', READY) // 事件先到
      return { id: 't1', progress: 0.99, speed: '', etaSec: 0 }
    }
    await s.startInstall('')
    eq('事件先于 InstallFFmpeg 返回：仍是 ready', s.status.state, 'ready')
    eq('事件先于 InstallFFmpeg 返回：对话框不显示', s.dialogVisible, false)
    w.impl.install = async () => ({ id: 't1', progress: 0, speed: '', etaSec: 0 })
  }

  // ---- 4. 竞态：启动时的 GetFFmpegStatus（missing）晚于 ready 事件返回，不能覆盖 ----
  {
    setActivePinia(createPinia())
    let release!: (v: any) => void
    w.impl.getStatus = () => new Promise((r) => (release = r))
    const s = useFFmpegStore()
    const p = s.init()
    await tick()
    w.emit('ffmpeg:status', READY)
    release({ state: 'missing' })
    await p
    eq('过期的启动检测不覆盖 ready 事件', s.status.state, 'ready')
    eq('过期的启动检测不弹框', s.dialogVisible, false)
    w.impl.getStatus = async () => ({ state: 'missing' })
  }

  // ---- 5. 竞态：重新检测（安装前发出）晚于 ready 事件返回 ----
  {
    const s = await fresh()
    let release!: (v: any) => void
    w.impl.recheck = () => new Promise((r) => (release = r))
    const p = s.recheck()
    await tick()
    w.emit('ffmpeg:status', READY)
    release({ state: 'missing' })
    await p
    eq('过期的 RecheckFFmpeg 不覆盖 ready 事件', s.status.state, 'ready')
    w.impl.recheck = async () => READY
  }

  // ---- 6. 手动指定位置成功：对话框关闭；返回值不覆盖已到的事件 ----
  {
    const s = await fresh()
    eq('手动指定前：对话框开着', s.dialogVisible, true)
    await s.pickPath('C:\\ffmpeg')
    eq('手动指定成功：ready', s.status.state, 'ready')
    eq('手动指定成功：对话框关闭', s.dialogVisible, false)
  }

  // ---- 7. 安装失败：对话框继续显示并带错误；之后重试成功后关闭 ----
  {
    const s = await fresh()
    await s.startInstall('')
    w.emit('ffmpeg:status', { state: 'failed', taskId: 't1', error: { code: 'INTERNAL', message: '下载超时' } })
    eq('失败：对话框保持显示', s.dialogVisible, true)
    await s.startInstall()
    w.emit('ffmpeg:status', READY)
    eq('失败后重试成功：对话框关闭', s.dialogVisible, false)
  }

  // ---- 8. 用户点过「稍后」后安装完成又变缺失（比如被删）：重新走提示逻辑 ----
  {
    const s = await fresh()
    await s.dismissPrompt()
    eq('稍后：关闭', s.dialogVisible, false)
    w.emit('ffmpeg:status', READY)
    eq('ready 后 promptDismissed 复位', s.promptDismissed, false)
  }
  return fails
}
