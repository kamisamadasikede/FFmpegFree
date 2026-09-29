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
  { key: 'edit', label: '剪辑', path: '/edit', icon: 'cut', needsFFmpeg: true },
  { key: 'live', label: '直播', path: '/live', icon: 'live', needsFFmpeg: true },
  { key: 'docs', label: '文档', path: '/docs', icon: 'doc' },
  { key: 'tools', label: '工具', path: '/tools', icon: 'tool' },
  { key: 'tasks', label: '任务中心', path: '/tasks', icon: 'task', separatorBefore: true },
]

export const bottomNav: NavItem[] = [{ key: 'settings', label: '设置', path: '/settings', icon: 'set' }]
