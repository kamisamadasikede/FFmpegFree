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

/** 字节数：862 * 1024**2 → "862 MB"，1.8 GB → "1.8 GB"（1024 进制，与系统文件管理器一致） */
export function formatBytes(n: number): string {
  if (!isFinite(n) || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 100 || i === 0 ? Math.round(v) : v.toFixed(1).replace(/\.0$/, '')} ${units[i]}`
}

/** 缩略图角标时长：724 → "12:04"，3725 → "1:02:05"，0 → "" */
export function formatShortClock(sec: number): string {
  if (!isFinite(sec) || sec <= 0) return ''
  const s = Math.round(sec)
  const p = (n: number) => String(n).padStart(2, '0')
  return s >= 3600 ? `${Math.floor(s / 3600)}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}` : `${p(Math.floor(s / 60))}:${p(s % 60)}`
}

/** 所在文件夹：/a/b.mp4 → /a；根路径保留分隔符：/a.mp4 → /，C:\a.mp4 → C:\（不是 C:） */
export function dirName(path: string): string {
  const i = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'))
  if (i === 2 && path[1] === ':') return path.slice(0, 3) // 盘符根：C:\ / C:/
  return i <= 0 ? path.slice(0, i + 1) || path : path.slice(0, i)
}
