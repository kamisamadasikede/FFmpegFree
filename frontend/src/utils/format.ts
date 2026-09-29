/** 剩余时间：72 → "1 分 12 秒"，45 → "45 秒"，3700 → "1 小时 1 分" */
export function formatEta(sec: number): string {
  if (!isFinite(sec) || sec <= 0) return ''
  const s = Math.round(sec)
  if (s < 60) return `${s} 秒`
  if (s < 3600) return `${Math.floor(s / 60)} 分 ${s % 60} 秒`
  return `${Math.floor(s / 3600)} 小时 ${Math.floor((s % 3600) / 60)} 分`
}

/** 已推流时长：2538 → "00:42:18" */
export function formatClock(sec: number): string {
  const s = Math.max(0, Math.floor(sec))
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(Math.floor(s / 3600))}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}`
}

/** 开始时间：今天显示 HH:mm，其他天显示 MM-DD HH:mm；0 显示 — */
export function formatStart(ms: number): string {
  if (!ms) return '—'
  const d = new Date(ms)
  const p = (n: number) => String(n).padStart(2, '0')
  const hm = `${p(d.getHours())}:${p(d.getMinutes())}`
  return d.toDateString() === new Date().toDateString() ? hm : `${p(d.getMonth() + 1)}-${p(d.getDate())} ${hm}`
}

export function formatDuration(ms: number): string {
  return formatEta(ms / 1000) || '不到 1 秒'
}

export function fileBaseName(path: string): string {
  return path.split(/[\\/]/).filter(Boolean).pop() ?? path
}
