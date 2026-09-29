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
