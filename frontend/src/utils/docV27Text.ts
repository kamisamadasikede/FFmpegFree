// 文档页 v0.27 / v0.27.1 / v0.27.2 文案（契约 6.12.28、6.12.31、6.12.32.6、6.12.45、6.12.52；设计 v0.3 §九 §十）。
// 只按 code + reason 映射，界面不出现错误码、reason=、editBlock 取值；LibreOffice 不出现（Linux 两句在 docV26Text）。
import { DOC_LINUX_MISSING, DOC_OUTDATED_BUTTON, DOC_OUTDATED_DOWNLOAD } from '@/utils/docV26Text'
import { formatBytes } from '@/utils/format'

export const UNMAPPED = '出了点问题，请重试。'

// ─── 引擎（6.12.28 / 6.12.31，产品定） ───
export type DocEnginePref = 'auto' | 'office' | 'wps' | 'component'
export const DOC_ENGINE_PREFS: readonly DocEnginePref[] = ['auto', 'office', 'wps', 'component']
export const ENGINE_AUTO_LABEL = '自动（推荐）'
export const ENGINE_AUTO_HINT = '按 Office、WPS、文档组件的顺序选第一个能用的'
export const ENGINE_ROW_TITLE = '转换引擎'
export const ENGINE_ROW_HINT = '本机装了多个能转换文档的软件，可以指定用哪个。'
export const COMPONENT_NOT_DOWNLOADED = '文档组件（未下载）'

/** 设置页「文档组件已就绪」下面那行（按顶层 source） */
export function engineInUseText(source?: string | null): string {
  if (source === 'office') return '正在使用本机 Microsoft Office'
  if (source === 'wps') return '正在使用本机 WPS'
  if (source === 'system' || source === 'downloaded') return '正在使用文档组件'
  return ''
}
/** 记录详情（6.12.31）：go / simple / 旧记录不显示 */
export function engineRecordText(engine?: string | null): string {
  if (engine === 'office') return '由本机 Microsoft Office 转换'
  if (engine === 'wps') return '由本机 WPS 转换'
  if (engine === 'component') return '由文档组件转换'
  return ''
}
/** 下载确认（选中未下载的文档组件时） */
export function componentDownloadConfirmText(downloadBytes: number): string {
  return downloadBytes > 0 ? `需要下载文档组件（约 ${formatBytes(downloadBytes)}），现在下载吗？` : '需要下载文档组件，现在下载吗？'
}
export function componentOptionHint(installed: boolean, downloadBytes: number, version: string): string {
  if (!installed) return downloadBytes > 0 ? `选中后需要先下载，约 ${formatBytes(downloadBytes)}` : '选中后需要先下载'
  return version ? `版本 ${version}` : ''
}

// ─── 预览（6.12.32.6，产品定） ───
export const PV_GENERATING = '正在生成预览…'
export const PV_GENERATING_SUB = '关掉这个窗口会取消生成。'
export const PV_SIMPLE = '简易预览，排版可能和原文件不一样。'
export const PV_TRUNCATED = '文件太长，只显示了前面一部分。'
export const PV_NEEDS_COMPONENT = '需要文档组件才能预览。'
export const PV_TOO_LARGE_SIMPLE = '文件太大，没法简易预览。下载文档组件后可以完整预览。'
export const PV_FAILED = '这个文件暂时无法预览。'
export const PV_ENCRYPTED = '这个文件有密码保护，不能预览。'
export const PV_PRESENTATION_BUSY = '请先关闭正在打开的演示文稿，再预览。'
export const PV_ENGINE_BUSY = '请先关闭正在打开的文档，再预览。'
export const PV_DOWNLOAD_BUTTON = '下载文档组件'
export const pvCsvFooter = (total: number): string => `只显示前 1000 行，共 ${total} 行。`
export const pvIndexText = (i: number, n: number): string => `第 ${i} 个，共 ${n} 个`

export interface PreviewNotice {
  text: string
  /** 「重试」= 重新 GetDocPreview */
  retry?: boolean
  /** 「下载文档组件」/「更新文档组件」 */
  download?: string
}
/** state=failed 时：只认三个码，其余一律「这个文件暂时无法预览。」；不用后端 message */
export function previewFailedNotice(code?: string | null): PreviewNotice {
  if (code === 'DOC_ENCRYPTED') return { text: PV_ENCRYPTED }
  if (code === 'DOC_PRESENTATION_BUSY') return { text: PV_PRESENTATION_BUSY, retry: true }
  if (code === 'DOC_ENGINE_BUSY') return { text: PV_ENGINE_BUSY, retry: true }
  return { text: PV_FAILED }
}
/** kind=unavailable：reason 只给程序用；Linux 不给按钮；组件太旧（6.12.55）换成「文档组件版本太旧，请重新下载。」+「更新文档组件」 */
export function previewUnavailableNotice(reason: string | undefined, opts: { linux: boolean; outdated: boolean; engineReady?: boolean }): PreviewNotice {
  // 已经有能用的引擎（文档组件 / 本机 Office / WPS）却还是 unavailable（例如后端这条预览路线还没接上）：不让用户去下载，只说暂时无法预览
  if (opts.engineReady) return { text: PV_FAILED }
  // Linux 没有下载源：不给按钮（6.12.32.5），太大的只说前半句
  if (opts.linux) return { text: reason === 'too_large_for_simple' ? '文件太大，没法简易预览。' : DOC_LINUX_MISSING }
  if (opts.outdated) return { text: DOC_OUTDATED_DOWNLOAD, download: DOC_OUTDATED_BUTTON }
  if (reason === 'too_large_for_simple') return { text: PV_TOO_LARGE_SIMPLE, download: PV_DOWNLOAD_BUTTON }
  if (reason === 'needs_component') return { text: PV_NEEDS_COMPONENT, download: PV_DOWNLOAD_BUTTON }
  return { text: PV_FAILED }
}

// ─── 编辑（6.12.38 / 6.12.45） ───
export const EDIT_FORMAT_TIP = '这类文件请用默认程序打开编辑'
export const EDIT_TOO_LARGE_TIP = '文件太大，只能查看，不能在这里编辑。'
export const EDIT_MACRO_TIP = '这个文件带宏，请用默认程序打开编辑。'
export const EDIT_CONVERTING_TIP = '文件正在转换，转完再保存。'
export const EDIT_ENCODING_TIP = '这个网页的编码不支持编辑，只能查看。'
export const EDIT_MALFORMED_CSV_TIP = '这个 CSV 有格式问题，只能查看，不能编辑。'
export const EDIT_MALFORMED_DOCX_TIP = '这个文件打不开编辑，请用默认程序打开编辑。'
export const EDIT_MISSING_BANNER = '原文件已经不在了，改完只能另存为。'
export const UNSAVED_CONFIRM = '有改动还没保存，要保存吗？'
export const RECONVERT_EDITED_CONFIRM = '这个文件在应用里改过。重转会按原文件重新生成一份，你改的内容不会带过去。'
export const SAVED_TOAST = '已保存。'
export const SAVED_BACKUP_TOAST = '已保存，原文件备份在同一个文件夹。'
export const savedAsToast = (name: string): string => `已另存为“${name}”。`
export const DOCX_SAVE_AS = '另存为'
export const DOCX_OVERWRITE = '保存（覆盖原文件）'

/** editBlock → 「编辑」按钮悬停说明（missing 不禁用编辑，只禁用「保存」） */
export function editBlockTip(block: string | undefined, ext: string): string {
  switch (block) {
    case 'format':
      return EDIT_FORMAT_TIP
    case 'too_large':
      return EDIT_TOO_LARGE_TIP
    case 'converting':
      return EDIT_CONVERTING_TIP
    case 'encoding':
      return EDIT_ENCODING_TIP
    case 'malformed':
      return ext === 'csv' ? EDIT_MALFORMED_CSV_TIP : EDIT_MALFORMED_DOCX_TIP
    case 'macro':
      return EDIT_MACRO_TIP
    case undefined:
    case '':
      return ''
    default:
      return EDIT_FORMAT_TIP
  }
}

export type SaveAction = 'retry' | 'saveAs' | 'saveAsUtf8' | 'reopen'
export interface SaveErrorView {
  text: string
  actions: SaveAction[]
}
/**
 * 保存出错的文案（6.12.45 定稿 + 6.12.52）。mode：text（SaveDocText）/ textAs（SaveDocTextAs）/ docx / docxAs。
 * 只看 code + reason；后端 message 不直接显示。所有出错都保留编辑内容（调用方负责）。
 */
export const SAVE_APP_DIR_TEXT = '不能保存到应用自己的文件夹里，请换一个位置。'
/**
 * message：后端 message，只用来认「另存为到应用目录」这一种（契约里它是 INVALID_ARGUMENT 且没有 reason，
 * 别的没有 reason 的 INVALID_ARGUMENT 都是参数问题，一律「出了点问题，请重试。」）；不会直接显示。
 */
export function saveErrorView(code: string | undefined, reason: string | undefined, mode: 'text' | 'textAs' | 'docx' | 'docxAs', message?: string): SaveErrorView {
  const asMode = mode === 'textAs' || mode === 'docxAs'
  const v = (text: string, ...actions: SaveAction[]): SaveErrorView => ({ text, actions: asMode ? actions.filter((a) => a === 'retry') : actions })
  switch (code) {
    case 'TASK_CONFLICT':
      if (reason === 'file_changed') return v('文件在别处被改过了，请重新打开，或另存为。', 'reopen', 'saveAs')
      if (reason === 'converting' || reason === 'in_use') return v('文件正在转换，转完再保存。', 'saveAs')
      if (reason === 'copying') return v('文件还在准备中，准备好后再保存。')
      return v(UNMAPPED) // saving 等前端内部错误
    case 'IO_ERROR':
      if (reason === 'in_use') return v('文件正被其他程序占用，请关闭后再保存。', 'retry', 'saveAs')
      if (reason === 'permission') return v('没有权限保存到这里，请另存到其他位置。', 'saveAs')
      if (reason === 'backup') return v('没法在这个文件夹留备份，文件没有保存。请另存为。', 'saveAs')
      return v('保存失败，请重试或另存为。', 'retry', 'saveAs')
    case 'INVALID_ARGUMENT':
      if (reason === 'encoding') return { text: '有些字符没法按原编码保存，请另存为 UTF-8。', actions: ['saveAsUtf8'] }
      if (reason === 'too_large') return v('内容太多，没法在这里保存。请用默认程序打开编辑。')
      if (reason === 'malformed') return v('文件没能保存，原文件没有改动。请另存为再试。', 'saveAs')
      if (reason === 'format') return mode === 'docx' || mode === 'docxAs' ? v('文件没能保存，原文件没有改动。请另存为再试。', 'saveAs') : v('只能保存成同一种格式。')
      if (reason === 'chunk_order' || reason === 'checksum') return v(UNMAPPED)
      if (!reason && asMode && message === SAVE_APP_DIR_TEXT) return v(SAVE_APP_DIR_TEXT)
      return v(UNMAPPED)
    case 'NOT_FOUND':
      if (reason === 'file' || reason === 'record') return v(EDIT_MISSING_BANNER, 'saveAs')
      return v(UNMAPPED) // save_session
    case 'CONVERT_DISK_FULL':
      return v('磁盘空间不足，没有保存。')
    default:
      return v(UNMAPPED)
  }
}

/** 设置页：文档组件所在文件夹打不开（状态刚好变了） */
export const DOC_COMPONENT_FOLDER_NOT_READY = '文档组件还没有就绪。'

/** 场景 35「重新打开」：自己的确认（PM 10-09 更正：不走「有改动还没保存」确认，因为这时保存不了） */
export const REOPEN_DISCARD_CONFIRM = '重新打开会丢掉你改的内容，确定吗？'
/** docx 覆盖确认（设计 §十一，场景 40） */
export const OVERWRITE_CONFIRM_TITLE = '覆盖原文件'
export const OVERWRITE_CONFIRM_BODY = '会先在同一文件夹留一份备份，再覆盖原文件。'
export const DOCX_UNKNOWN_HINT = '部分内容无法在这里编辑，保存后会保留。'
export const CSV_EDIT_HINT = '双击单元格编辑，Enter 确认，Esc 放弃这一格'
export const EDITED_IN_APP = '在应用里改过'
export const SAVE_AS_UTF8_LABEL = '另存为 UTF-8'
