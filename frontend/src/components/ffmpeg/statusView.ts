// 侧栏 ffmpeg 状态的展示映射（设计师定稿）：四种表达，文案 / 可点性都在这里，组件和自检共用。
export type FFmpegState = 'checking' | 'ready' | 'missing' | 'outdated' | 'installing' | 'failed'

export interface FFmpegStatusView {
  tone: 'ok' | 'warn' | 'run' | 'q'
  /** 展开时显示的文字 */
  text: string
  /** 外层 role=status 的 aria-label；折叠气泡文字、title 与它相同 */
  label: string
  /** 可点时按钮自己的 aria-label */
  actionLabel: string
  /** 整行是 button，点开安装对话框（里面有进度，不跳任务中心） */
  clickable: boolean
}

export function ffmpegStatusView(state: FFmpegState): FFmpegStatusView {
  switch (state) {
    case 'ready':
      return { tone: 'ok', text: 'ffmpeg 已就绪', label: 'ffmpeg 已就绪', actionLabel: 'ffmpeg 已就绪', clickable: false }
    case 'installing':
      return { tone: 'run', text: 'ffmpeg 安装中…', label: 'ffmpeg 安装中，点击查看进度', actionLabel: 'ffmpeg 安装中，点击查看进度', clickable: true }
    case 'checking':
      return { tone: 'q', text: 'ffmpeg 检测中…', label: 'ffmpeg 检测中…', actionLabel: 'ffmpeg 检测中…', clickable: false }
    default:
      // 缺失、过旧、安装失败都归为“未就绪”，点开安装对话框后在对话框里看失败原因
      return { tone: 'warn', text: 'ffmpeg 未就绪', label: 'ffmpeg 未就绪', actionLabel: 'ffmpeg 未就绪，点击打开安装对话框', clickable: true }
  }
}
