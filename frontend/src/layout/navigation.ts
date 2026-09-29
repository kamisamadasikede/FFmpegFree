import type { Component } from 'vue'
import { Switch, Scissor, VideoCamera, Document, Tools, List, Setting } from '@element-plus/icons-vue'

export interface NavItem {
  key: string
  label: string
  path: string
  icon: Component
  /** 依赖 ffmpeg，缺失时显示警告圆点（设计规范 6） */
  needsFFmpeg?: boolean
}

export const mainNav: NavItem[] = [
  { key: 'convert', label: '转换', path: '/', icon: Switch, needsFFmpeg: true },
  { key: 'edit', label: '剪辑', path: '/edit', icon: Scissor, needsFFmpeg: true },
  { key: 'live', label: '直播', path: '/live', icon: VideoCamera, needsFFmpeg: true },
  { key: 'docs', label: '文档', path: '/docs', icon: Document },
  { key: 'tools', label: '工具', path: '/tools', icon: Tools },
  { key: 'tasks', label: '任务中心', path: '/tasks', icon: List },
]

export const bottomNav: NavItem[] = [{ key: 'settings', label: '设置', path: '/settings', icon: Setting }]
