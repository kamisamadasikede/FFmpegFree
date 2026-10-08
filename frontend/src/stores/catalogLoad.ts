/**
 * 转换页格式目录的加载状态（包 24 修“左下角已就绪、右边一直在加载 / 全灰”）。
 *
 * 组件是否就绪只看 ffmpeg store（真实后端推的事件名是 `ffmpeg:status`，见 stores/ffmpeg.ts）。
 * 不能用目录里的 reasonCode=converter_not_ready 自己判断就绪：那是检测还没完成时的旧结果。
 *
 * 三种空态：
 * - loading  骨架。检测中收到目录（哪怕整表都是 converter_not_ready）也停在这里，不报超时。
 * - error    请求失败，或 8 秒没有任何响应。迟到的成功响应仍然生效。
 * - unready  store 明确不是 checking / ready（缺失、过期、安装中、失败）。
 * - shown    可以展示格式块。
 */
export type CatalogPhase = 'loading' | 'error' | 'unready' | 'shown'

/** 前端自己的上限：超过这个时间还没收到响应才进入 error。检测中已经收到响应的不算。 */
export const CATALOG_LOAD_TIMEOUT_MS = 8000

export const CATALOG_NOT_READY_CODE = 'converter_not_ready'

export const catalogNotReady = (list: { reasonCode?: string }[]): boolean => list.some((f) => f.reasonCode === CATALOG_NOT_READY_CODE)

export interface CatalogController {
  readonly phase: CatalogPhase
  /** 开始一次请求。manual=用户点了「重试」，允许再自动补取一次。返回这次的代号。 */
  start(status: string, manual?: boolean): number
  /**
   * 响应到了（含超时之后才到的）。
   * apply=写进界面；hold=检测还没结束，保持骨架；reload=这是检测前的旧结果，立刻再取一次；ignore=丢掉。
   */
  resolve(gen: number, status: string, notReady: boolean): 'apply' | 'hold' | 'reload' | 'ignore'
  /** 请求本身失败：马上进入 error，不等满 8 秒。 */
  reject(gen: number): void
  /** store 状态变了。返回 true 时调用方要再取一次目录。 */
  onStatus(status: string): boolean
}

export function createCatalogController(opts?: {
  timeoutMs?: number
  onPhase?: (phase: CatalogPhase) => void
  schedule?: (fn: () => void, ms: number) => () => void
}): CatalogController {
  const timeoutMs = opts?.timeoutMs ?? CATALOG_LOAD_TIMEOUT_MS
  const schedule = opts?.schedule ?? ((fn: () => void, ms: number) => {
    const id = setTimeout(fn, ms)
    return () => clearTimeout(id)
  })
  let generation = 0
  let phase: CatalogPhase = 'loading'
  /** 检测中已经收到过一次目录，等就绪后再取 */
  let needsReload = false
  /** 这一轮 ready 已经自动再取过，避免旧结果把请求打成死循环 */
  let retried = false
  let cancel: (() => void) | null = null

  function setPhase(next: CatalogPhase) {
    phase = next
    opts?.onPhase?.(next)
  }
  function clearTimer() {
    cancel?.()
    cancel = null
  }
  function arm(gen: number) {
    clearTimer()
    cancel = schedule(() => {
      if (gen !== generation || phase !== 'loading') return
      setPhase('error')
    }, timeoutMs)
  }

  return {
    get phase() {
      return phase
    },
    start(status, manual = false) {
      generation += 1
      if (manual) retried = false
      const gen = generation
      if (status !== 'checking' && status !== 'ready') {
        clearTimer()
        needsReload = false
        setPhase('unready')
        return gen
      }
      setPhase('loading')
      arm(gen)
      return gen
    },
    resolve(gen, status, notReady) {
      if (gen !== generation) return 'ignore'
      clearTimer()
      // 人还在检测：响应不算失败，也不展示（里面的 converter_not_ready 是旧结果）。超时计时已经取消。
      if (status === 'checking') {
        needsReload = true
        setPhase('loading')
        return 'hold'
      }
      if (status !== 'ready') {
        needsReload = false
        setPhase('unready')
        return 'ignore'
      }
      if (notReady) {
        if (retried) {
          needsReload = false
          setPhase('error')
          return 'ignore'
        }
        retried = true
        needsReload = true
        setPhase('loading')
        return 'reload'
      }
      needsReload = false
      retried = false
      setPhase('shown')
      return 'apply'
    },
    reject(gen) {
      if (gen !== generation) return
      clearTimer()
      if (phase === 'unready') return
      needsReload = false
      setPhase('error')
    },
    onStatus(status) {
      if (status === 'checking') {
        if (phase !== 'shown') setPhase('loading')
        return false
      }
      if (status !== 'ready') {
        clearTimer()
        needsReload = false
        setPhase('unready')
        return false
      }
      if (needsReload && !retried) {
        retried = true
        needsReload = false
        return true
      }
      if ((phase === 'error' || phase === 'unready') && !retried) {
        retried = true
        return true
      }
      return false
    },
  }
}
