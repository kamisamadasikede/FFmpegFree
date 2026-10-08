// 契约 v0.25.1（架构师 10-08）：后端的事件可能比调用返回得早，也可能乱序。前端的三条规则都在这里，不依赖 Vue，check:api 里自检：
//   ① 暂存：收到列表里还没有的那一行 / 会话的事件，先按 id 暂存（只留 seq 最大的一条），等这一行加进来再补上，不丢；
//   ② 对齐：AddSources 返回后用这一批的 id 再查一次当前状态（见 stores/convertRecords.ts addPaths → reconcileBatch）；
//      查询在途时又收到了这一行的事件，就以事件为准，不用查询结果覆盖（canApplySnapshot）；
//   ③ 只往终态走：拉流会话一旦是已结束 / 被中断 / 不支持，后到的事件都不再改它（先到先定，PullOutcomeGate）。

/** ① 早到事件的暂存区：按 id 只留 seq 最大的一条；超过 cap 条时丢最早暂存的（那一行多半已经不会再出现） */
export function createEarlyEvents<E extends { seq: number }>(cap = 500) {
  const held = new Map<string, E>()
  return {
    hold(id: string, e: E): void {
      const cur = held.get(id)
      if (cur && cur.seq >= e.seq) return
      held.delete(id) // 重新插入，刷新淘汰顺序
      held.set(id, e)
      if (held.size > cap) {
        const oldest = held.keys().next().value
        if (oldest !== undefined) held.delete(oldest)
      }
    },
    /** 取出并删除这一行暂存的事件 */
    take(id: string): E | undefined {
      const e = held.get(id)
      held.delete(id)
      return e
    },
    has: (id: string): boolean => held.has(id),
    get size(): number {
      return held.size
    },
    clear: (): void => held.clear(),
  }
}

/** ② 对齐用的快照没有 seq：只有发出查询以后这一行没再收到事件（copySeq 没变）时，快照才可以覆盖本地状态 */
export const canApplySnapshot = (seqAtRequest: number, seqNow: number): boolean => seqNow === seqAtRequest

// ---------------- ③ 拉流会话只往终态走 ----------------
export type PullEnd = 'ended' | 'interrupted' | 'unsupported'
export type PullEndSource = 'user' | 'event' | 'player' | 'start'
export interface PullOutcome {
  phase: PullEnd
  /** 用户自己点了「停止播放」 */
  byUser: boolean
  source: PullEndSource
  /** failed 事件 / 开始失败时的后端 message（只用于被中断的正文） */
  message?: string
}

/**
 * 拉流会话的终态闸门（先到先定）：
 * - 用户点停止 → 已结束（只有标题）；
 * - 后端 live:pull 的 ended / interrupted / failed / unsupported → 先到的那个定终态，后到的都忽略；
 * - 播放器自己读到流结尾或网络出错：有后端会话时不马上定，等 graceMs 看后端事件（走查 G2：本机 HTTP-FLV 正常收尾，
 *   播放器的 LOADING_COMPLETE 比后端的 interrupted 早到）；等不到再按播放器的结果定。没有后端会话（ws / wss 直连）时马上定。
 * reset() 之后（重新开始拉流）才能再定一次。
 */
export class PullOutcomeGate {
  outcome: PullOutcome | null = null
  private timer: ReturnType<typeof setTimeout> | undefined
  private pendingPhase: PullEnd | null = null

  constructor(
    private onSettle: (o: PullOutcome) => void,
    private hasSession: () => boolean,
    private graceMs = 3000,
  ) {}

  get settled(): boolean {
    return this.outcome !== null
  }
  /** 播放器报了结果、正在等后端事件 */
  get waiting(): boolean {
    return this.pendingPhase !== null
  }

  reset(): void {
    clearTimeout(this.timer)
    this.timer = undefined
    this.pendingPhase = null
    this.outcome = null
  }

  private settle(o: PullOutcome): boolean {
    if (this.outcome) return false // 已经是终态：后到的不覆盖
    clearTimeout(this.timer)
    this.timer = undefined
    this.pendingPhase = null
    this.outcome = o
    this.onSettle(o)
    return true
  }

  user(): boolean {
    return this.settle({ phase: 'ended', byUser: true, source: 'user' })
  }
  /** live:pull 事件（playing 不归这里管） */
  event(state: 'ended' | 'interrupted' | 'failed' | 'unsupported', message?: string): boolean {
    if (state === 'failed') return this.settle({ phase: 'interrupted', byUser: false, source: 'event', ...(message ? { message } : {}) })
    return this.settle({ phase: state, byUser: false, source: 'event' })
  }
  /** StartPullPreview 自己就失败了（还没有会话） */
  startFailed(phase: PullEnd, message?: string): boolean {
    return this.settle({ phase, byUser: false, source: 'start', ...(message ? { message } : {}) })
  }
  /** 播放器的结果：不支持的编码马上定；结尾 / 出错在有后端会话时先等后端事件 */
  player(phase: PullEnd): boolean {
    if (this.outcome) return false
    if (phase === 'unsupported' || !this.hasSession()) return this.settle({ phase, byUser: false, source: 'player' })
    if (this.pendingPhase) return false // 已经在等了：以第一次播放器报的为准
    this.pendingPhase = phase
    this.timer = setTimeout(() => {
      const p = this.pendingPhase
      if (p) this.settle({ phase: p, byUser: false, source: 'player' })
    }, this.graceMs)
    return false
  }
  /** 离开页面 / 卸载：之后到的事件、播放器结果都不再处理（不回调界面） */
  close(): void {
    this.dispose()
    if (!this.outcome) this.outcome = { phase: 'ended', byUser: true, source: 'user' }
  }
  dispose(): void {
    clearTimeout(this.timer)
    this.timer = undefined
    this.pendingPhase = null
  }
}
