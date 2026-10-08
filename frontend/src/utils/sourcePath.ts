/**
 * 源文件分组键：同一个文件不同写法（反斜杠 / 重复分隔符 / 末尾分隔符 / Windows 盘符路径大小写）归到一起。
 * 后端给了 sourceId 时以后端为准；这里只是兜底，也用于“重复添加同一路径合并到原来的父行”。
 */
export function normalizeSourcePath(p: string): string {
  let s = (p ?? '').trim().replace(/\\/g, '/').replace(/\/{2,}/g, '/')
  if (s.length > 1) s = s.replace(/\/+$/, '')
  if (/^[a-zA-Z]:\//.test(s)) s = s.toLowerCase() // Windows 路径不区分大小写
  return s
}
