import type { IconName } from '@/components/icon/icons'
import { CAT_UI_ENABLED } from '@/api/flags'
import { AGENT_NAME } from '@/config/brand'

export interface NavItem {
  key: string
  label: string
  path: string
  icon: IconName
  /** 依赖转换组件，缺失时显示警告圆点（设计规范 6） */
  needsFFmpeg?: boolean
  /** 在此项之前画分割线 */
  separatorBefore?: boolean
  /** 名字后面的小角标（如「新」） */
  tag?: string
}

/** 工具入口放在上方；Agent 和设置统一放在侧栏底部。 */
export const mainNav: NavItem[] = [
  { key: 'convert', label: '转换', path: '/', icon: 'convert', needsFFmpeg: true },
  { key: 'voice', label: '语音工具', path: '/voice', icon: 'mic' },
  { key: 'docs', label: '文档', path: '/docs', icon: 'doc' },
  { key: 'live', label: '直播', path: '/live', icon: 'live', needsFFmpeg: true },
  { key: 'tools', label: 'JSON工具', path: '/tools', icon: 'tool' },
  { key: 'tasks', label: '任务中心', path: '/tasks', icon: 'task', separatorBefore: true },
]

export const bottomNav: NavItem[] = [
  ...(CAT_UI_ENABLED ? [{ key: 'cat', label: AGENT_NAME, path: '/cat', icon: 'bot', tag: '新' } as NavItem] : []),
  { key: 'settings', label: '设置', path: '/settings', icon: 'set' },
]

/**
 * 路由守卫用：目标路由的一级入口是否依赖转换组件。与侧栏置灰共用 mainNav 里的 needsFFmpeg，不另存一份清单。
 * topPath 是一级路由的 path（如 '/'、'/live'）。
 */
export function routeNeedsFFmpeg(topPath: string | undefined): boolean {
  return !!topPath && mainNav.some((i) => i.needsFFmpeg && i.path === topPath)
}
