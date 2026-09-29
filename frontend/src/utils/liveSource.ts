// 采集来源标题的中间省略（设计说明 §4.1）：头部可收缩（CSS 省略号），尾部固定保留最后 10 个字符（一般是“ - 应用名”），title 放全名。
// 标题不超过 20 个字符时不拆（用不着省略）。按码点数，不把 emoji 劈开。
export const SOURCE_TAIL_CHARS = 10
export const SOURCE_SPLIT_MIN = 20
export function splitSourceTitle(title: string): { head: string; tail: string } {
  const chars = Array.from(title)
  if (chars.length <= SOURCE_SPLIT_MIN) return { head: title, tail: '' }
  return { head: chars.slice(0, -SOURCE_TAIL_CHARS).join(''), tail: chars.slice(-SOURCE_TAIL_CHARS).join('') }
}
/** 选择器形态：Windows（平台是 windows，或列表里有窗口）→ 分组下拉；macOS / Linux → 屏幕单选列表。不靠“列表为空”判断（设计说明 §4.7） */
export function sourcePickerMode(platform: string, sources: readonly { kind: string }[]): 'dropdown' | 'list' {
  return platform === 'windows' || sources.some((s) => s.kind === 'window') ? 'dropdown' : 'list'
}

export type SourceLoadState = 'loading' | 'ready' | 'empty' | 'failed'
/**
 * 录屏推流“开始推流”可用性（走查 G5，架构师已定）：
 *  - ffmpeg 未就绪 / 正在开始 / 地址为空 → 置灰
 *  - 已有选中项 → 可用（旧列表里的已选项、刷新中、刷新失败保留旧列表都不置灰；开始时后端会重新校验，失效走 LIVE_SOURCE_GONE）
 *  - 没有选中项：来源失效（gone，等用户重选）→ 置灰；首次加载中 → 置灰；首次失败 / 空列表 → 可用，不传来源，后端默认推主屏；正常列表却没选（不会出现）→ 置灰
 */
export function recordStartEnabled(p: { blocked: boolean; starting: boolean; hasUrl: boolean; sourceId: string; state: SourceLoadState; gone: boolean }): boolean {
  if (p.blocked || p.starting || !p.hasUrl) return false
  if (p.sourceId) return true
  if (p.gone) return false
  return p.state === 'failed' || p.state === 'empty'
}
/** 没选来源、将默认推主屏时，表单里显示一句轻提示（true 时才显示） */
export function defaultMainScreenHint(p: { sourceId: string; state: SourceLoadState; gone: boolean }): boolean {
  return !p.sourceId && !p.gone && (p.state === 'failed' || p.state === 'empty')
}
