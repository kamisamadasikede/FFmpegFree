import * as SystemBinding from '../../wailsjs/go/app/SystemService'
import { call } from '@/api/call'
import { hasWailsBackend } from '@/services/wails'

// 契约第 4 节的 SystemService.RevealInFolder 后端还没实现，生成的绑定里没有；
// 用命名空间对象探测，后端补上并重新生成绑定后自动生效。
const optional = SystemBinding as unknown as Record<string, ((...a: any[]) => Promise<any>) | undefined>

/** 绑定里是否已有 RevealInFolder */
export const canRevealInFolder = typeof optional.RevealInFolder === 'function'

/** 在文件管理器里显示文件。返回 false 表示当前没有这个能力（调用方退回复制路径） */
export async function revealInFolder(path: string): Promise<boolean> {
  if (!canRevealInFolder || !hasWailsBackend()) return false
  await call(optional.RevealInFolder!(path))
  return true
}
