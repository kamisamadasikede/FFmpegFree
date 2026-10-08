/**
 * 转换组件变为可用（ffmpeg:status → ready）后补取一次源文件列表（配合后端 PR #100）。
 * 冷启动时转换页先列表，后端那时还没检测完转换组件，没 media 的行探测不了；就绪后再列一次，
 * 当次会话就能显示编码、有无画面 / 声音、时长，不用等下次启动。
 *
 * 规则：
 * - 只在“进入 ready”时考虑一次（由调用方的 watch 保证只在 false → true 时调用 onReady）；
 * - 只有组件未就绪期间发过列表请求才补（启动时已经 ready → 不补）；needs() 为 false（行都有 media 了）也不补；
 * - 有列表请求在途时不另发，等它结束后再看一次（只补一次）。
 */
export interface ReadyRelist {
  /** 包住每一次列表请求：记录是否在组件未就绪时发出、是否在途 */
  track<T>(fetch: () => Promise<T>): Promise<T>
  /** 转换组件刚变为可用 */
  onReady(): void
  /** 自检用：当前状态 */
  readonly state: { inflight: number; listedUnready: boolean; pending: boolean }
}

export function createReadyRelist(o: { isReady: () => boolean; needs: () => boolean; relist: () => Promise<void> }): ReadyRelist {
  let inflight = 0
  let listedUnready = false
  let pending = false

  async function track<T>(fetch: () => Promise<T>): Promise<T> {
    if (!o.isReady()) listedUnready = true
    inflight++
    try {
      return await fetch()
    } finally {
      inflight--
      if (!inflight && pending) {
        pending = false
        setTimeout(() => void run(), 0) // 等调用方把这次结果合进列表后再判断 needs()
      }
    }
  }
  async function run() {
    if (!listedUnready || !o.isReady()) return
    listedUnready = false
    if (!o.needs()) return
    try {
      await o.relist()
    } catch (e) {
      console.warn('relist sources after converter ready failed', e)
    }
  }
  function onReady() {
    if (!listedUnready) return
    if (inflight) {
      pending = true // 在途的那次结束后再补，不并发第二个
      return
    }
    void run()
  }
  return {
    track,
    onReady,
    get state() {
      return { inflight, listedUnready, pending }
    },
  }
}
