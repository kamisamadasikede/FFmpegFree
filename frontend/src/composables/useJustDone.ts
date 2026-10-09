// 「刚完成」标记：某条记录 / 任务的状态从别的状态变成 succeeded 的那一刻，标记 ms 毫秒（给完成对勾播一次动画）。
// 打开页面时已经是完成的、虚拟列表滚回来重新挂载的，都不算刚完成，不播。
import { onScopeDispose, reactive, watch } from 'vue'

type Item = { id: string; status: string }

export function useJustDone(list: () => readonly Item[] | undefined, ms = 1200) {
  const just = reactive(new Set<string>())
  const timers = new Set<ReturnType<typeof setTimeout>>()
  let prev: Map<string, string> | null = null
  watch(
    () => (list() ?? []).map((x) => `${x.id}\u0000${x.status}`).join('\u0001'),
    () => {
      const next = new Map((list() ?? []).map((x) => [x.id, x.status] as const))
      if (prev) {
        for (const [id, s] of next) {
          const p = prev.get(id)
          if (s === 'succeeded' && p && p !== 'succeeded') {
            just.add(id)
            const t = setTimeout(() => {
              timers.delete(t)
              just.delete(id)
            }, ms)
            timers.add(t)
          }
        }
      }
      prev = next
    },
    { immediate: true },
  )
  onScopeDispose(() => timers.forEach(clearTimeout))
  return (id: string) => just.has(id)
}
