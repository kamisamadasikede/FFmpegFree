// WebKitGTK：<video> 或 MediaSource 已经卸掉之后，mpegts.js 仍会调用 endOfStream / appendBuffer，
// 抛出 InvalidStateError（没被它自己接住）。预览就停在「正在连接…」。这里把这类调用接住。

const STATE_ERRORS = new Set(['InvalidStateError', 'InvalidAccessError'])

export function isMseStateError(e: unknown): boolean {
  return STATE_ERRORS.has((e as { name?: string } | null)?.name ?? '')
}

/** 调用 fn；InvalidStateError / InvalidAccessError 当成这次操作没做成，其它照旧抛。 */
export function guardCall<T>(fn: () => T): T | undefined {
  try {
    return fn()
  } catch (e) {
    if (isMseStateError(e)) return undefined
    throw e
  }
}

function wrap(proto: object, name: string) {
  const orig = (proto as Record<string, unknown>)[name]
  if (typeof orig !== 'function' || (orig as { __ffMseGuard?: boolean }).__ffMseGuard) return
  const guarded = function (this: unknown, ...args: unknown[]) {
    return guardCall(() => (orig as (...a: unknown[]) => unknown).apply(this, args))
  }
  ;(guarded as { __ffMseGuard?: boolean }).__ffMseGuard = true
  ;(proto as Record<string, unknown>)[name] = guarded
}

let installed = false

export function installMseGuard(): void {
  if (installed || typeof MediaSource === 'undefined') return
  installed = true
  wrap(MediaSource.prototype, 'endOfStream')
  wrap(MediaSource.prototype, 'addSourceBuffer')
  wrap(MediaSource.prototype, 'removeSourceBuffer')
  const desc = Object.getOwnPropertyDescriptor(MediaSource.prototype, 'duration')
  if (desc?.set && desc.get) {
    const set = desc.set
    const get = desc.get
    Object.defineProperty(MediaSource.prototype, 'duration', {
      configurable: true,
      enumerable: desc.enumerable,
      get() { return get.call(this) },
      set(v: number) { guardCall(() => set.call(this, v)) },
    })
  }
  // appendBuffer 不包：mpegts 自己接住并上报。remove / abort 在拆的时候没有 try，会直接把更新回调打断。
  if (typeof SourceBuffer !== 'undefined') {
    wrap(SourceBuffer.prototype, 'remove')
    wrap(SourceBuffer.prototype, 'abort')
  }
}
