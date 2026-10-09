import type { IconName } from '@/components/icon/icons'

export interface NavItem {
  key: string
  label: string
  path: string
  icon: IconName
  /** 依赖 ffmpeg，缺失时显示警告圆点（设计规范 6） */
  needsFFmpeg?: boolean
  /** 在此项之前画分割线 */
  separatorBefore?: boolean
}

export const mainNav: NavItem[] = [
  { key: 'convert', label: '转换', path: '/', icon: 'convert', needsFFmpeg: true },
  { key: 'live', label: '直播', path: '/live', icon: 'live', needsFFmpeg: true },
  { key: 'docs', label: '文档', path: '/docs', icon: 'doc' },
  { key: 'tools', label: 'JSON工具', path: '/tools', icon: 'tool' },
  { key: 'tasks', label: '任务中心', path: '/tasks', icon: 'task', separatorBefore: true },
]

export const bottomNav: NavItem[] = [{ key: 'settings', label: '设置', path: '/settings', icon: 'set' }]

/**
 * 路由守卫用：目标路由的一级入口是否依赖 ffmpeg。与侧栏置灰共用 mainNav 里的 needsFFmpeg，不另存一份清单。
 * topPath 是一级路由的 path（如 '/'、'/live'）。
 */
export function routeNeedsFFmpeg(topPath: string | undefined): boolean {
  return !!topPath && mainNav.some((i) => i.needsFFmpeg && i.path === topPath)
}
