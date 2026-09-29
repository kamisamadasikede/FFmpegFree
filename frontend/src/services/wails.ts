/**
 * Wails 运行时适配层。
 * Service 方法直接 import 生成的 `wailsjs/go/app/*`（经 `api/call.ts` 解析 AppError）；
 * 这里只保留三样：事件订阅（走生成的 runtime）、后端是否存在的判断、浏览器预览参数。
 */
import { EventsOn } from '../../wailsjs/runtime/runtime'

/** 是否运行在 Wails 壳里（浏览器里 `npm run dev` / `vite preview` 时为 false） */
export function hasWailsBackend(): boolean {
  return typeof window !== 'undefined' && !!(window as any).go && !!(window as any).runtime
}

/** 订阅后端事件，返回取消订阅函数；浏览器预览下什么也不做。 */
export function onEvent<T = unknown>(event: string, cb: (payload: T) => void): () => void {
  if (!hasWailsBackend()) return () => {}
  return EventsOn(event, cb)
}

/**
 * 本地预览用：地址里带 ?ff=missing|installing|failed|ready、?tasks=3 可以模拟状态，只影响界面。
 * ?noinstall 模拟"后端还没有 InstallFFmpeg"（配合 ?ff=missing&dlg 查看"安装功能即将上线"）。
 * ?nopicker 模拟没有 PickDirectory（手动指定路径改用文本框）。
 * 仅在 window.go 不存在（浏览器预览）时生效，真实运行不读取这些参数。
 */
export const previewParams = new URLSearchParams(window.location.search)
