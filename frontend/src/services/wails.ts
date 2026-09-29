/**
 * Wails 绑定的过渡适配层。
 * 后端 Service 生成 wailsjs 绑定后，这里改为直接 import `wailsjs/go/app/*`，调用方不用改。
 */
type AnyFn = (...args: any[]) => Promise<any>

export function service(name: string): Record<string, AnyFn> | undefined {
  return (window as any).go?.app?.[name]
}

export function onEvent<T = unknown>(event: string, cb: (payload: T) => void): () => void {
  const rt = (window as any).runtime
  if (!rt?.EventsOn) return () => {}
  return rt.EventsOn(event, cb) ?? (() => rt.EventsOff?.(event))
}

/** 本地预览用：地址里带 ?ff=missing|installing|failed|ready、?tasks=3 可以模拟状态，只影响界面 */
export const previewParams = new URLSearchParams(window.location.search)
