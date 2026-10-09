/**
 * 文档页 v0.26 文案与格式化（契约 6.12.9~6.12.22 + 产品/设计定稿）。
 * 数字一律从后端字段格式化，不写死 MB/GB。
 */
import { formatBytes } from '@/utils/format'

/** Linux 未安装：界面唯一允许出现 LibreOffice 的两处之一（check:copy 放行） */
export const DOC_LINUX_MISSING =
  '请先在系统里安装 LibreOffice，然后重启应用。' // check:copy 白名单 1/2（LIBRE_ALLOWED）
/** Linux 过旧：白名单第二句 */
export const DOC_LINUX_OUTDATED =
  '系统里的 LibreOffice 版本太旧，请升级到 7.2 或更高版本，然后重启应用。' // check:copy 白名单 2/2（LIBRE_ALLOWED）
/** Win/mac 过旧 */
// 产品 10-09 定稿（Win/mac）：取代原来的「系统里的文档组件版本太旧，请下载新的文档组件。」
export const DOC_OUTDATED_DOWNLOAD = '文档组件版本太旧，请重新下载。'
export const DOC_OUTDATED_BUTTON = '更新文档组件'

export const DOC_HINT_SIMPLE_BAR = 'Word、ODT、TXT 可以简易转 PDF，md 和网页可以互转'
/** v0.28 打开 PDF 输入后（产品 10-09 定稿）组件未就绪横条的整句：PDF 转 txt / md / 简易网页不需要组件 */
export const DOC_HINT_SIMPLE_BAR_PDF = 'Word、ODT、TXT 可以简易转 PDF，md 和网页可以互转，PDF 可以提取文字转成 TXT、md 和简易网页'
/** 组件未就绪横条用的句子（不带句号）；pdfOn = docV28On() */
export const docHintSimpleBar = (pdfOn: boolean): string => (pdfOn ? DOC_HINT_SIMPLE_BAR_PDF : DOC_HINT_SIMPLE_BAR)
export const DOC_NEED_COMPONENT = '需要文档组件'
export const DOC_SIMPLE_PDF_LABEL = '简易转换（只保留文字）'
export const DOC_SIMPLE_HINT = '下载文档组件后可保留图片和排版'
export const DOC_MD_HINT = '转成 Markdown 只保留文字和基本格式，图片和复杂表格会丢失。'
// 产品 10-09 定稿（契约 / 设计统一；格式说明行和 csv_first_sheet_only 结果警告共用）
export const DOC_CSV_HINT = '转成 CSV 只会保留第一个工作表。'
// ── v0.28 PDF 转其他格式（契约 6.12.58~6.12.65；产品 10-09 定稿，改文案只改这里） ──
/** pdf → doc / docx / odt / rtf（hintKey=pdf_layout） */
export const DOC_PDF_LAYOUT_HINT = 'PDF 转 Word 会尽量保留排版，复杂版式和扫描件可能会走样。'
/** pdf → txt / md（hintKey=pdf_text）；没有组件时的 pdf → html 仍用 simple_mode，但文案同这句。旧后端若给 md_lossy / simple_mode，PDF 源上也兜底用这句，永不显示 md_lossy 那句。 */
export const DOC_PDF_TEXT_ONLY_HINT = '只提取文字，不保留排版和图片。'
/** DOC_PDF_NO_TEXT（不可重试，只能移除） */
export const DOC_PDF_NO_TEXT_TEXT = '这个 PDF 里没有能提取的文字，可能是扫描件。'
/** 添加 PDF：INVALID_ARGUMENT reason=too_large（200 MiB） */
export const DOC_PDF_TOO_LARGE_TEXT = 'PDF 太大了，最多支持 200 MB。'
/** 添加 PDF：INVALID_ARGUMENT reason=too_many_pages（500 页） */
export const DOC_PDF_TOO_MANY_PAGES_TEXT = 'PDF 页数太多，最多支持 500 页。'
/** PDF 能转成的 Word 类目标（带 pdf_layout 提示） */
export const DOC_PDF_WORD_TARGETS = ['doc', 'docx', 'odt', 'rtf'] as const
/** PDF 只提取文字的目标 */
export const DOC_PDF_TEXT_TARGETS = ['txt', 'md'] as const
export const DOC_PREPARING = '正在准备文档组件…'
export const DOC_QUEUE_NEXT = '排队中 · 下一个'
/** 子记录进度条右侧：0 → 下一个（设计 v0.2 §二.8） */
export const DOC_QUEUE_LINE = (n: number): string => (n <= 0 ? '下一个' : `前面还有 ${n} 项`)
export const DOC_QUEUE_AHEAD = (n: number) => (n <= 0 ? DOC_QUEUE_NEXT : `排队中 · 前面还有 ${n} 项`)

/** 契约 6.12.20 文案；retryable 与契约一致 */
export const DOC_ERROR_COPY: Record<string, { text: string; retryable: boolean }> = {
  DOC_ENCRYPTED: { text: '这个文件有密码保护，不能转换。请先去掉密码再添加。', retryable: false },
  DOC_CORRUPT: { text: '文件打不开，可能已损坏或不是有效的文档。', retryable: false },
  DOC_TIMEOUT: { text: '文件处理太久没完成，可能已损坏，请检查后重试。', retryable: true },
  DOC_COMPONENT_CRASHED: { text: '文档组件意外退出，请重试。', retryable: true },
  DOC_COMPONENT_NOT_READY: { text: '需要先下载文档组件。', retryable: true },
  DOC_DOWNLOAD_FAILED: { text: '文档组件下载失败，请检查网络后重试。', retryable: true },
  DOC_CHECKSUM_FAILED: { text: '下载的文档组件校验失败，请重试。', retryable: true },
  DOC_COMPONENT_INSTALL_FAILED: { text: '文档组件准备失败，请重试。', retryable: true },
  DOC_FORMAT_UNSUPPORTED: { text: '不支持这种文件。', retryable: false },
  // v0.28 起后端不再产生；旧后端 / 旧记录仍可能见到，按“格式不支持”显示（旧句「PDF 暂时不能…」已删，产品 10-09）
  DOC_PDF_INPUT_UNSUPPORTED: { text: '不支持这种文件。', retryable: false },
  // v0.28（6.12.63）
  DOC_PDF_NO_TEXT: { text: DOC_PDF_NO_TEXT_TEXT, retryable: false },
  // v0.27（6.12.33，产品 10-09 定稿；转换用这两句，预览的「再预览。」版本属于下一包）
  DOC_PRESENTATION_BUSY: { text: '请先关闭正在打开的演示文稿，再转换。', retryable: true },
  DOC_ENGINE_BUSY: { text: '请先关闭正在打开的文档，再转换。', retryable: true },
}

/** v0.27 成功记录的结果警告（result.warnings 机器码 → 文案）；未知码不显示 */
export const DOC_RESULT_WARNINGS: Record<string, string> = {
  simple_fallback: '这次是简易转换，只保留了文字。可以稍后重转。',
  csv_first_sheet_only: DOC_CSV_HINT,
}
export function docResultWarnings(warnings?: string[] | null): string[] {
  return (warnings ?? []).map((w) => DOC_RESULT_WARNINGS[w]).filter((x): x is string => !!x)
}

/** DOC_ENCRYPTED reason=owner_only（PDF 只设了权限 / 所有者密码，后端后续小 PR 发；不可重试，只能移除） */
export const DOC_PDF_OWNER_ONLY_TEXT = '这个 PDF 设置了权限保护，暂时不能转换。'

const reasonOfDetail = (detail?: string | null): string | undefined =>
  /^reason=([A-Za-z0-9_-]+)/.exec((detail ?? '').split(/\r?\n/, 1)[0].trim())?.[1]

export function docErrorText(code?: string | null, message?: string | null, linux = false, detail?: string | null): string {
  if (code === 'DOC_COMPONENT_NOT_READY' && linux) return DOC_LINUX_MISSING
  if (code === 'DOC_ENCRYPTED' && reasonOfDetail(detail) === 'owner_only') return DOC_PDF_OWNER_ONLY_TEXT
  if (code && DOC_ERROR_COPY[code]) return DOC_ERROR_COPY[code].text
  const m = (message ?? '').trim()
  return m || '出了点问题，请重试。'
}

/**
 * 添加文件（AddDocSources 的 error）的文案：PDF 的大小 / 页数上限按 reason 给定稿句子（界面不出现 reason），其他同 docErrorText。
 * 非 PDF 的 too_large 仍用后端 message。
 */
export function docAddErrorText(err: { code?: string | null; message?: string | null; detail?: string | null }, path = '', linux = false): string {
  const reason = reasonOfDetail(err.detail)
  const isPdf = /\.pdf$/i.test(path)
  if (err.code === 'INVALID_ARGUMENT' && reason === 'too_many_pages') return DOC_PDF_TOO_MANY_PAGES_TEXT
  if (err.code === 'INVALID_ARGUMENT' && reason === 'too_large' && isPdf) return DOC_PDF_TOO_LARGE_TEXT
  return docErrorText(err.code, err.message, linux, err.detail)
}

/** hintKey → 定稿文案。pdf_text 固定「只提取文字…」；md_lossy 只给非 PDF → md（PDF 源请走 docPdfNote，永不显示 md_lossy 那句）。 */
export function docHintText(hintKey?: string | null): string | undefined {
  switch (hintKey) {
    case 'pdf_text':
      return DOC_PDF_TEXT_ONLY_HINT
    case 'pdf_layout':
      return DOC_PDF_LAYOUT_HINT
    case 'md_lossy':
      return DOC_MD_HINT
    case 'csv_first_sheet':
      return DOC_CSV_HINT
    case 'simple_mode':
      return DOC_SIMPLE_HINT
    default:
      return undefined
  }
}

/**
 * 格式区下面的说明行（v0.28 PDF 源）。selectedFamilies 里有 pdf 时才返回；没有返回 null，走原来的逻辑。
 * Word 类目标 → 排版提示；txt / md（hintKey=pdf_text，旧后端 md_lossy 也兜底）→ 只提取文字；html 简易 → 只提取文字 + 可下载组件（Linux 不给按钮）。
 */
export function docPdfNote(
  families: readonly string[],
  target: { ext: string; available: boolean; simple?: boolean; hintKey?: string } | null,
  linux = false,
): { text: string; tone: 'info' | 'warn'; download?: boolean } | null {
  if (!target || !target.available || !families.includes('pdf')) return null
  if ((DOC_PDF_WORD_TARGETS as readonly string[]).includes(target.ext) || target.hintKey === 'pdf_layout') return { text: DOC_PDF_LAYOUT_HINT, tone: 'info' }
  // 新后端 pdf_text；旧后端可能仍给 md_lossy / simple_mode；按扩展名兜底。PDF 源上永不显示 md_lossy 那句。
  if (
    target.hintKey === 'pdf_text' ||
    (DOC_PDF_TEXT_TARGETS as readonly string[]).includes(target.ext) ||
    target.hintKey === 'md_lossy'
  ) {
    return { text: DOC_PDF_TEXT_ONLY_HINT, tone: 'info' }
  }
  if (target.ext === 'html' && target.simple) return { text: DOC_PDF_TEXT_ONLY_HINT, tone: 'warn', download: !linux }
  return null
}

export function docErrorRetryable(code?: string | null, detail?: string | null): boolean {
  if (!code) return true
  // v0.28：PDF 太大 / 页数太多不可重试，只能移除
  if (code === 'INVALID_ARGUMENT') {
    const r = reasonOfDetail(detail)
    if (r === 'too_large' || r === 'too_many_pages') return false
  }
  return DOC_ERROR_COPY[code]?.retryable ?? true
}

/** detail 里的 needBytes / freeBytes（契约 2.2） */
export function parseSpaceDetail(detail?: string | null): { needBytes?: number; freeBytes?: number } {
  if (!detail) return {}
  let needBytes: number | undefined
  let freeBytes: number | undefined
  for (const line of detail.split(/\r?\n/)) {
    const m = /^(needBytes|freeBytes)=([0-9]+)$/.exec(line.trim())
    if (!m) continue
    const n = Number(m[2])
    if (m[1] === 'needBytes') needBytes = n
    else freeBytes = n
  }
  return { needBytes, freeBytes }
}

/** 磁盘满：至少需要 x GB（按 needBytes；没有时退回后端 message） */
export function docDiskFullText(detail?: string | null, fallback?: string | null): string {
  const { needBytes } = parseSpaceDetail(detail)
  if (needBytes && needBytes > 0) {
    const gb = needBytes / (1024 * 1024 * 1024)
    // 「至少需要」向上取整（设计 v0.2：包 + 2 GiB ≈ 2.35 GiB → 约 2.4 GB），不会少报
    const x = gb >= 10 ? String(Math.ceil(gb)) : (Math.ceil(gb * 10) / 10).toFixed(1).replace(/\.0$/, '')
    return `磁盘空间不够，至少需要 ${x} GB 可用空间。`
  }
  return (fallback ?? '').trim() || '磁盘空间不够，请清理后重试。'
}

/** 引导卡尺寸文案：downloadBytes / installBytes（v0.26.1）都来自后端；installBytes 为 0 时省略“安装后约占用”半句 */
export function docSizeGuideText(downloadBytes: number, installBytes = 0): string {
  if (!downloadBytes && !installBytes) return ''
  const dl = downloadBytes > 0 ? `约 ${formatBytes(downloadBytes)}` : ''
  if (downloadBytes > 0 && installBytes && installBytes > 0) {
    return `需要下载${dl}，安装后约占用 ${formatBytes(installBytes)} 磁盘空间。`
  }
  if (downloadBytes > 0) return `需要下载${dl}，只需下载一次。`
  return `安装后约占用 ${formatBytes(installBytes)} 磁盘空间。`
}

export function intersectionWhy(families: string[]): string {
  const map: Record<string, string> = { pdf: 'PDF', text: '文档', sheet: '表格', slide: '演示' }
  const names = [...new Set(families)].map((f) => map[f] ?? f).filter(Boolean)
  if (names.length <= 1) return ''
  if (names.length === 2) return `选中的文件有${names[0]}和${names[1]}两类，只显示它们都能转的格式。`
  const cn = ['', '', '两', '三', '四'][names.length] ?? String(names.length)
  return `选中的文件有${names.join('、')}${cn}类，只显示它们都能转的格式。`
}

export function sameFamilyWhy(exts: string[], labels: Record<string, string>): string {
  const uniq = [...new Set(exts)]
  if (uniq.length <= 1) return ''
  return `文件不能转成自己的格式，所以 ${uniq.map((e) => labels[e] ?? e.toUpperCase()).join(' 和 ')} 都没有列出。`
}
