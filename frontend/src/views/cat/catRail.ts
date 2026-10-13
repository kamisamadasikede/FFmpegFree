/** 滑块最短这么高，超长对话里也拖得动。 */
export const RAIL_THUMB_MIN = 28

/** 用户消息记号之间的间距。装得下就按这个排紧，不按正文位置拉开。 */
export const RAIL_TICK_PITCH = 12

/** 左侧记号条最多这么高。再多就在这条里面滚动，不拉满整列对话。 */
export const RAIL_TICK_MAX = 360

/** 可视窗口最上和最下那一截，比中间的短。 */
export const RAIL_TICK_EDGE = 0.45

/**
 * 用户消息记号按发言顺序排。条数少时挨在一起；装不下轨道时再均匀压进可视高度。
 * 返回的是每条记号的 top。
 */
export function railTickTops(count: number, track: number, pitch = RAIL_TICK_PITCH): number[] {
  if (count <= 0 || !(track > 0)) return []
  const usable = Math.max(0, track - pitch)
  if (count === 1) return [0]
  const need = (count - 1) * pitch
  if (need <= usable) return Array.from({ length: count }, (_, i) => i * pitch)
  const step = usable / (count - 1)
  return Array.from({ length: count }, (_, i) => i * step)
}

/** 记号组的高度。装得下就按间距叠紧，装不下就用整段高度。页面再把这组放到左侧中间。 */
export function railTickBox(count: number, track: number, pitch = RAIL_TICK_PITCH): number {
  if (count <= 0 || !(track > 0)) return 0
  return Math.min(track, count * pitch)
}

/**
 * 左侧记号窗口。内容按间距排开，不压扁。
 * 窗口高度不超过对话，也不超过固定上限；多出来的留给里面滚动。
 */
export function railTickWindow(
  count: number,
  track: number,
  pitch = RAIL_TICK_PITCH,
  max = RAIL_TICK_MAX,
): { box: number; content: number; tops: number[] } {
  if (count <= 0 || !(track > 0)) return { box: 0, content: 0, tops: [] }
  const content = count * pitch
  const cap = max > 0 ? max : content
  return {
    box: Math.min(content, track, cap),
    content,
    tops: Array.from({ length: count }, (_, i) => i * pitch),
  }
}

/** 离焦点越远越短，焦点最长。下标 0 是焦点自己。 */
export const RAIL_TICK_STAIR = [36, 24, 16, 11, 8]

/** 每条记号的宽度。焦点处最长，两边逐级变短。 */
export function tickStairWidths(count: number, focus: number, steps = RAIL_TICK_STAIR): number[] {
  const last = steps.length - 1
  const at = Math.min(Math.max(0, focus), Math.max(0, count - 1))
  return Array.from({ length: count }, (_, i) => steps[Math.min(Math.abs(i - at), last)] ?? steps[last])
}

/** 可视窗口中线对着的那一条。 */
export function tickFocusIndex(count: number, scrollTop: number, view: number, pitch = RAIL_TICK_PITCH): number {
  if (count <= 0) return 0
  const mid = Math.max(0, scrollTop) + Math.max(0, view) / 2
  const i = Math.round((mid - pitch / 2) / pitch)
  return Math.min(count - 1, Math.max(0, i))
}

/** 当前能看见的最上、最下两条缩短，中间保持原长。只有一条时不缩。 */
export function tickEdgeScales(count: number, scrollTop: number, view: number, pitch = RAIL_TICK_PITCH, edge = RAIL_TICK_EDGE): number[] {
  const scales = Array.from({ length: count }, () => 1)
  if (count <= 1 || !(view > 0) || !(pitch > 0)) return scales
  const start = Math.max(0, scrollTop)
  const end = start + view
  let lo = -1
  let hi = -1
  for (let i = 0; i < count; i++) {
    const top = i * pitch
    if (top + pitch <= start || top >= end) continue
    if (lo < 0) lo = i
    hi = i
  }
  if (lo >= 0 && hi > lo) {
    scales[lo] = edge
    scales[hi] = edge
  }
  return scales
}

/**
 * 右侧长滑块：高度按视口占全文的比例，位置按当前滚动。
 * 内容没高出视口时没有滑块。
 */
export function railThumb(
  scrollTop: number,
  scrollHeight: number,
  clientHeight: number,
  track: number,
  min = RAIL_THUMB_MIN,
): { top: number; height: number } | null {
  if (!(track > 0) || !(clientHeight > 0) || !(scrollHeight > clientHeight + 1)) return null
  if (!Number.isFinite(scrollTop)) return null
  const height = Math.min(track, Math.max(min, (clientHeight / scrollHeight) * track))
  const maxTop = Math.max(0, track - height)
  const range = scrollHeight - clientHeight
  const top = range <= 0 ? 0 : Math.min(maxTop, Math.max(0, (scrollTop / range) * maxTop))
  return { top, height }
}

/** 拖动滑块时，滑块顶边对应的 scrollTop。 */
export function scrollTopForThumb(
  thumbTop: number,
  scrollHeight: number,
  clientHeight: number,
  track: number,
  min = RAIL_THUMB_MIN,
): number {
  const sample = railThumb(0, scrollHeight, clientHeight, track, min)
  if (!sample) return 0
  const maxTop = Math.max(0, track - sample.height)
  const range = scrollHeight - clientHeight
  if (maxTop <= 0 || range <= 0) return 0
  const t = Math.min(maxTop, Math.max(0, thumbTop)) / maxTop
  return t * range
}
