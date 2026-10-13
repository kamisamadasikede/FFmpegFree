import { railThumb, railTickTops, scrollTopForThumb } from './catRail'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catRailChecks(eq: Eq): void {
  eq('轨道：用户消息按顺序排紧', railTickTops(3, 200), [0, 8, 16])
  eq('轨道：装不下时均匀排进轨道', railTickTops(3, 10), [0, 1, 2])
  eq('轨道：没有消息或没有高度就没有记号', [railTickTops(0, 200), railTickTops(2, 0)], [[], []])
  eq('轨道：没高出视口就没有长滑块', railThumb(0, 100, 200, 180), null)
  const mid = railThumb(500, 2000, 400, 300)
  eq('轨道：拖回滑块顶边能回到原来的滚动', mid && Math.round(scrollTopForThumb(mid.top, 2000, 400, 300)), 500)
  const tiny = railThumb(0, 10000, 400, 300)
  eq('轨道：超长时滑块不会缩成一条缝', tiny && tiny.height >= 28 && tiny.top === 0, true)
  const end = railThumb(9600, 10000, 400, 300)
  eq('轨道：滚到末尾滑块贴底', end ? end.top + end.height : -1, 300)
}
