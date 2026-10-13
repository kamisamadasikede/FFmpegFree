/** 滑块最短这么高，超长对话里也拖得动。 */
export const RAIL_THUMB_MIN = 28

/** 用户消息记号之间的间距。装得下就按这个排紧，不按正文位置拉开。 */
export const RAIL_TICK_PITCH = 8

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
