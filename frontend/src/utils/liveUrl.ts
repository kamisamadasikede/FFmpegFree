/** 推流地址 + 推流码 → 完整地址（推流码为空时原样返回） */
export function joinPushUrl(base: string, key: string): string {
  const b = base.trim()
  const k = key.trim()
  if (!k) return b
  return `${b.replace(/\/+$/, '')}/${k.replace(/^\/+/, '')}`
}

/** 每行一个地址，去空行和首尾空格 */
export function parseTargets(text: string): string[] {
  return text
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
}
