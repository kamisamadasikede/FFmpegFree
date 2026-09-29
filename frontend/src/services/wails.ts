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

// ---- 接口层模拟（api/sim.ts）用的本地事件总线 ----
// 模拟层的 task:created / task:progress / task:status 走这里，形状与后端事件完全一致；任务 store 同时订阅 Wails 事件和这条总线。
const simListeners = new Map<string, Set<(payload: unknown) => void>>()

/** 发一个模拟事件（只给 api 层模拟实现用） */
export function emitSimEvent(event: string, payload: unknown): void {
  for (const cb of [...(simListeners.get(event) ?? [])]) {
    try {
      cb(payload)
    } catch (e) {
      console.error('sim event handler failed', event, e)
    }
  }
}

/** 订阅模拟事件，返回取消订阅函数 */
export function onSimEvent<T = unknown>(event: string, cb: (payload: T) => void): () => void {
  let set = simListeners.get(event)
  if (!set) simListeners.set(event, (set = new Set()))
  const listener = cb as (payload: unknown) => void
  set.add(listener)
  return () => set!.delete(listener)
}

/** 同时订阅后端事件和模拟事件（接口层的 watch 用；真实后端就绪后模拟总线上不会再有事件） */
export function onTaskEvent<T = unknown>(event: string, cb: (payload: T) => void): () => void {
  const a = onEvent<T>(event, cb)
  const b = onSimEvent<T>(event, cb)
  return () => {
    a()
    b()
  }
}

/**
 * 本地预览用：地址里带 ?ff=missing|installing|failed|ready、?tasks=3 可以模拟状态，只影响界面。
 * ?noinstall 模拟"后端还没有 InstallFFmpeg"（配合 ?ff=missing&dlg 查看"安装功能即将上线"）。
 * ?nopicker 模拟没有 PickDirectory（手动指定路径改用文本框）。
 * 仅在 window.go 不存在（浏览器预览）时生效，真实运行不读取这些参数。
 */
export const previewParams = new URLSearchParams(window.location.search)
