import * as SystemBinding from '../../wailsjs/go/app/SystemService'
import { system } from '../../wailsjs/go/models'
import { AppError, call } from '@/api/call'
import { hasWailsBackend } from '@/services/wails'

/** 在文件管理器里显示文件（文件夹则直接打开）。path 必须是绝对路径且存在，否则 INVALID_ARGUMENT / NOT_FOUND */
export async function revealInFolder(path: string): Promise<void> {
  await call(SystemBinding.RevealInFolder(path))
}

/** 系统选择文件夹对话框。用户取消返回 ""（不是错误） */
export async function pickDirectory(title = ''): Promise<string> {
  return await call(SystemBinding.PickDirectory(title))
}

/** 默认输出位置；"" = 保存到源文件所在文件夹 */
export async function getDefaultOutputDir(): Promise<string> {
  if (!hasWailsBackend()) return ''
  const s = await call(SystemBinding.GetSettings())
  return s?.defaultOutputDir ?? ''
}

/**
 * 保存默认输出位置。先读最新 Settings 再整体写回（UpdateSettings 是整体更新），只改 defaultOutputDir。
 * 目录不存在 / 不可写 / 不是绝对路径时后端返回 INVALID_ARGUMENT，整个更新不生效。
 */
export async function setDefaultOutputDir(dir: string): Promise<void> {
  const s = await call(SystemBinding.GetSettings())
  await call(SystemBinding.UpdateSettings(system.Settings.createFrom({ ...s, defaultOutputDir: dir })))
}

/** 同时转换数量的合法范围（契约 v0.7.6）：0 = 自动（CPU 核数的一半，限制在 1~3），1~8 = 固定值 */
export const MAX_CONCURRENT_AUTO = 0
export const MAX_CONCURRENT_MAX = 8

/** 读取同时转换数量；不在合法范围的值（旧数据 / 异常）按 0（自动）处理；浏览器预览返回 0 */
export async function getMaxConcurrent(): Promise<number> {
  if (!hasWailsBackend()) return MAX_CONCURRENT_AUTO
  const s = await call(SystemBinding.GetSettings())
  const n = s?.maxConcurrent
  return Number.isInteger(n) && n >= MAX_CONCURRENT_AUTO && n <= MAX_CONCURRENT_MAX ? n : MAX_CONCURRENT_AUTO
}

/**
 * 保存同时转换数量。先读最新 Settings 再整体写回（UpdateSettings 是整体更新），只改 maxConcurrent。
 * 范围外的值后端返回 INVALID_ARGUMENT；这里先在前端拦一次，不发请求。
 */
export async function setMaxConcurrent(n: number): Promise<void> {
  if (!Number.isInteger(n) || n < MAX_CONCURRENT_AUTO || n > MAX_CONCURRENT_MAX) {
    throw new AppError('INVALID_ARGUMENT', '同时转换数量只能是 0（自动）或 1 到 8')
  }
  const s = await call(SystemBinding.GetSettings())
  await call(SystemBinding.UpdateSettings(system.Settings.createFrom({ ...s, maxConcurrent: n })))
}

// ---- 选择文件（SystemService.PickFiles，后端 PR #12）----

/** 文件选择对话框的过滤器；patterns 形如 ["*.mp4", "*.mkv"]，空 = 不过滤 */
export interface PickFilter {
  name: string
  patterns: string[]
}

/** 转换页的「音视频文件」过滤器（宽松：扩展名不全时用户可在对话框里改成"所有文件"由后端 Probe 兜底） */
export const MEDIA_FILE_FILTER: PickFilter = {
  name: '音视频文件',
  patterns: [
    '*.mp4', '*.m4v', '*.mov', '*.mkv', '*.avi', '*.flv', '*.wmv', '*.webm', '*.ts', '*.mts', '*.m2ts', '*.mpg', '*.mpeg', '*.3gp', '*.ogv', '*.vob',
    '*.mp3', '*.wav', '*.flac', '*.aac', '*.m4a', '*.ogg', '*.opus', '*.wma', '*.ac3', '*.aiff', '*.gif',
  ],
}

/** v0.24（契约 6.16.4：输入扩展名表由前端写）：多了图片和新加的音视频格式 */
export const CONVERT_V24_FILE_FILTER: PickFilter = {
  name: '音视频和图片文件',
  patterns: [
    ...MEDIA_FILE_FILTER.patterns, '*.swf', '*.rm', '*.rmvb', '*.asf', '*.f4v', '*.amr', '*.m4r', '*.mp2', '*.ape', '*.wv', '*.mmf', '*.aif',
    '*.jpg', '*.jpeg', '*.png', '*.webp', '*.ico', '*.bmp', '*.tif', '*.tiff', '*.tga',
  ],
}

const PICK_FILES_SOON = '选择文件功能即将上线，请先把文件拖到这里。'

/**
 * 特性检测：当前构建的绑定里有没有 PickFiles（后端 PR #12 合入前没有）。
 * 浏览器预览（没有 window.go）视为可用，返回假路径。绑定落地后只需要改这个包装，调用方不用动。
 */
export function canPickFiles(): boolean {
  if (!hasWailsBackend()) return true
  const fn = (SystemBinding as unknown as Record<string, unknown>).PickFiles
  const live = (window as any).go?.app?.SystemService?.PickFiles
  return typeof fn === 'function' && typeof live === 'function'
}

/** 预览模式的假路径（?convert=... 之外的浏览器预览点「选择文件」时用） */
const PREVIEW_PICKED = ['/Users/me/Movies/发布会素材/开场动画_v3.mp4', '/Users/me/Movies/发布会素材/旁白_第一段.wav']

/**
 * 系统选择文件对话框。用户取消返回 []（不是错误）；返回的是清理过的绝对路径。
 * 没有 PickFiles 绑定时抛 AppError('UNSUPPORTED', …)，调用方显示 e.message。
 */
export async function pickFiles(filter: PickFilter = MEDIA_FILE_FILTER, multiple = true): Promise<string[]> {
  if (!hasWailsBackend()) return multiple ? [...PREVIEW_PICKED] : PREVIEW_PICKED.slice(0, 1)
  if (!canPickFiles()) throw new AppError('UNSUPPORTED', PICK_FILES_SOON)
  const fn = (SystemBinding as unknown as { PickFiles: (f: unknown, m: boolean) => Promise<string[]> }).PickFiles
  const paths = await call(fn(system.FileFilter.createFrom(filter), multiple))
  return paths ?? []
}
