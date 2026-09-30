/**
 * ffmpeg 版本号只显示数字版本（“9.0.2”），去掉构建来源的尾巴（如“9.0.2-https://www.martin-riedl.de”“7.1.1-essentials_build-www.gyan.dev”）。
 * 后端也会把版本号规范化成纯数字，前端兼容新旧两种（旧的带尾巴、新的干净）。
 * 以数字开头 → 取开头的“数字.数字…”；不是数字开头（如 git 构建 N-12345-gabcdef）→ 只去掉 URL / 域名尾巴，其余原样；空 → ''。
 */
export function cleanFfmpegVersion(raw: string | null | undefined): string {
  const s = (raw ?? '').trim()
  if (!s) return ''
  const m = /^[vV]?(\d+(?:\.\d+)*)(?![\d.])/.exec(s)
  if (m) return m[1]
  return s.replace(/[-\s]+(?:https?:\/\/|www\.)\S*$/i, '').trim()
}
