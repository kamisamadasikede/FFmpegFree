import { railThumb, railTickBox, railTickTops, railTickWindow, scrollTopForThumb, tickEdgeScales, tickFocusIndex, tickStairWidths } from './catRail'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catRailChecks(eq: Eq): void {
  eq('轨道：用户消息按顺序排紧', railTickTops(3, 200), [0, 12, 24])
  eq('轨道：装不下时均匀排进轨道', railTickTops(3, 20), [0, 4, 8])
  eq('轨道：没有消息或没有高度就没有记号', [railTickTops(0, 200), railTickTops(2, 0)], [[], []])
  eq('轨道：记号组少时收成一竖条', railTickBox(3, 200), 36)
  eq('轨道：记号组装不下就用整段高度', railTickBox(30, 40), 40)
  eq('轨道：没有消息就没有记号组', [railTickBox(0, 200), railTickBox(2, 0)], [0, 0])
  const win = railTickWindow(80, 800)
  eq('轨道：记号超高时窗口固定且不挤扁', [win.box, win.content, win.tops[1]], [360, 960, 12])
  eq('轨道：记号窗口不超过对话高度', railTickWindow(80, 200).box, 200)
  eq('轨道：记号少时窗口跟内容一样高', railTickWindow(3, 800).box, 36)
  eq('轨道：上下断点更短', tickEdgeScales(5, 0, 60), [0.45, 1, 1, 1, 0.45])
  eq('轨道：滚进窗口后仍是两端更短', tickEdgeScales(10, 24, 36), [1, 1, 0.45, 1, 0.45, 1, 1, 1, 1, 1])
  eq('轨道：只有一条时不缩短', tickEdgeScales(1, 0, 40), [1])
  eq('轨道：悬浮处阶梯变长', tickStairWidths(7, 3), [11, 16, 24, 36, 24, 16, 11])
  eq('轨道：窗口中线是滚动焦点', tickFocusIndex(10, 24, 36), 3)
  eq('轨道：没高出视口就没有长滑块', railThumb(0, 100, 200, 180), null)
  const mid = railThumb(500, 2000, 400, 300)
  eq('轨道：拖回滑块顶边能回到原来的滚动', mid && Math.round(scrollTopForThumb(mid.top, 2000, 400, 300)), 500)
  const tiny = railThumb(0, 10000, 400, 300)
  eq('轨道：超长时滑块不会缩成一条缝', tiny && tiny.height >= 28 && tiny.top === 0, true)
  const end = railThumb(9600, 10000, 400, 300)
  eq('轨道：滚到末尾滑块贴底', end ? end.top + end.height : -1, 300)
}
