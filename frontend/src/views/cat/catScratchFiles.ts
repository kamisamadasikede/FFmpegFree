/**
 * 右侧文件面板显示规则（#271）：绑定项目 → 显示；未绑定 → 对话文件夹里有内容才显示。
 * 这一轮正在写临时文件夹时也显示。打开过的对话在这一轮结束后仍可再展开。
 * 打开未绑定对话时查一次，每轮结束再查一次。切换对话时旧请求按序号丢弃。
 */
import { reactive } from 'vue'
import { t } from '@/i18n'

export const catScratch = reactive({ has: {} as Record<string, boolean>, keep: {} as Record<string, boolean> })

export function keepFilePanel(convId: string): void {
  if (convId) catScratch.keep[convId] = true
}

export function shouldShowFilePanel(bound: boolean, scratchHas: boolean | undefined, turnRunning = false, kept = false): boolean {
  return bound || scratchHas === true || turnRunning || kept
}

/** 每个对话一个递增序号；只有最新一次请求的结果生效 */
export function makeLatestGuard() {
  const seq = new Map<string, number>()
  return {
    begin(key: string): number {
      const n = (seq.get(key) ?? 0) + 1
      seq.set(key, n)
      return n
    },
    isLatest(key: string, n: number): boolean {
      return seq.get(key) === n
    },
  }
}

const guard = makeLatestGuard()

/** probe：返回该对话文件夹是否有文件；抛错 → 保留旧值 */
export async function refreshScratch(
  convId: string,
  bound: boolean,
  probe: (convId: string) => Promise<boolean>,
  store: Record<string, boolean> = catScratch.has,
): Promise<void> {
  if (!convId || bound) return
  const n = guard.begin(convId)
  let v: boolean
  try {
    v = await probe(convId)
  } catch {
    return
  }
  if (!guard.isLatest(convId, n)) return
  store[convId] = v
}
