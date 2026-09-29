import * as SystemBinding from '../../wailsjs/go/app/SystemService'
import { system } from '../../wailsjs/go/models'
import { call } from '@/api/call'
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
