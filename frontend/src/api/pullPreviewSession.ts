// 拉流预览会话的生命周期控制（不依赖 Vue，check:api 里自检）。保证：
//   · 只要 start 成功过，就一定会调 stop（点停止、播放出错、离开页面 / 组件卸载、start 期间已经被取消）；
//   · 重复 begin 不重复 start（同一时刻只有一个会话 / 一个在途的 start）；
//   · stop 幂等：同一个会话 id 只调一次 stopPullPreview，stop 自己出错也不抛给界面（只记下，会话在后端会随应用退出 / 远端流结束清理）。
import type { PullPreviewRequest, PullSession } from '@/api/live'

export interface PullPreviewApi {
  start(req: PullPreviewRequest): Promise<PullSession>
  stop(id: string): Promise<void>
}
export type PullPreviewState = 'idle' | 'starting' | 'active' | 'off' | 'failed'

export class PullPreviewController {
  state: PullPreviewState = 'idle'
  session: PullSession | null = null
  /** stop 调用次数（自检用） */
  stopCalls = 0
  private token = 0
  private stopped = new Set<string>()

  constructor(
    private api: PullPreviewApi,
    private onChange?: (s: PullPreviewState, session: PullSession | null) => void,
  ) {}

  private set(s: PullPreviewState, session: PullSession | null = this.session) {
    this.state = s
    this.session = session
    this.onChange?.(s, session)
  }

  /** 开始一个预览会话。enabled=false（用户开始前关了开关）→ 不调后端，状态 off */
  async begin(url: string, enabled: boolean): Promise<PullSession | null> {
    if (this.state === 'starting' || this.state === 'active') return this.session // 重复点击不重复 Start
    if (!enabled) {
      this.set('off', null)
      return null
    }
    const my = ++this.token
    this.set('starting', null)
    try {
      const s = await this.api.start({ url, preview: true })
      if (my !== this.token) {
        // start 期间已经点了停止 / 离开页面：拿到的会话必须停掉
        await this.stopSession(s.id)
        return null
      }
      if (!s.preview) {
        // 后端说不出预览（例如纯音频流）：会话仍要 Stop
        await this.stopSession(s.id)
        this.set('off', null)
        return null
      }
      this.set('active', s)
      return s
    } catch {
      if (my === this.token) this.set('failed', null)
      return null
    }
  }

  /** 停止当前会话（点停止 / 播放出错 / 离开页面 / 卸载都走这里）。start 还在途中也会在它返回后补停 */
  async end(): Promise<void> {
    this.token++
    const s = this.session
    this.set('idle', null)
    if (s) await this.stopSession(s.id)
  }

  private async stopSession(id: string): Promise<void> {
    if (this.stopped.has(id)) return
    this.stopped.add(id)
    this.stopCalls++
    try {
      await this.api.stop(id)
    } catch {
      /* 停止失败不打断界面；后端会话随远端流结束 / 应用退出清理 */
    }
  }
}
