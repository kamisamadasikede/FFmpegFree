import { makeLatestGuard, refreshScratch, shouldShowFilePanel } from './catScratchFiles'

type Eq = (name: string, got: unknown, want: unknown) => void

export async function catScratchFilesChecks(eq: Eq): Promise<void> {
  eq('绑定项目 → 显示', shouldShowFilePanel(true, undefined), true)
  eq('未绑定 + 有文件 → 显示', shouldShowFilePanel(false, true), true)
  eq('未绑定 + 空 → 隐藏', shouldShowFilePanel(false, false), false)
  eq('未绑定 + 未知 → 隐藏', shouldShowFilePanel(false, undefined), false)

  const g = makeLatestGuard()
  const a = g.begin('c'), b = g.begin('c')
  eq('旧序号作废', [g.isLatest('c', a), g.isLatest('c', b)], [false, true])

  // 两次请求乱序返回：先发的慢、后发的快 → 只认后发的
  const store: Record<string, boolean> = {}
  let release!: () => void
  const slow = refreshScratch('x', false, () => new Promise<boolean>((r) => { release = () => r(false) }), store)
  await refreshScratch('x', false, async () => true, store)
  release()
  await slow
  eq('乱序：旧响应被忽略', store.x, true)

  await refreshScratch('y', true, async () => true, store)
  eq('绑定项目不查', 'y' in store, false)
  store.z = true
  await refreshScratch('z', false, async () => { throw new Error('x') }, store)
  eq('出错保留旧值', store.z, true)
  await refreshScratch('z', false, async () => false, store)
  eq('变空 → 隐藏', store.z, false)
}
