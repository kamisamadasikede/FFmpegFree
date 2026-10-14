/**
 * 右侧文件面板显示规则（#271）：绑定项目 → 显示；未绑定 → 对话自己的文件夹里有文件（ListCatFiles 非空）才显示。
 * 打开未绑定对话时查一次，每轮 cat:turn 结束再查一次（Cat 第一次写文件后面板自动出现）。
 * 切换对话时旧请求的结果按序号丢弃；已知结果按对话缓存，不先清成 false（避免闪烁）。
 */
import { reactive } from 'vue'

export const catScratch = reactive({ has: {} as Record<string, boolean> })

export function shouldShowFilePanel(bound: boolean, scratchHas: boolean | undefined): boolean {
  return bound || scratchHas === true
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
