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
      return { tone: 'ok', text: '转换组件已就绪', label: '转换组件已就绪', actionLabel: '转换组件已就绪', clickable: false }
    case 'installing':
      return { tone: 'run', text: '转换组件安装中…', label: '转换组件安装中，点击查看进度', actionLabel: '转换组件安装中，点击查看进度', clickable: true }
    case 'checking':
      return { tone: 'q', text: '转换组件检测中…', label: '转换组件检测中…', actionLabel: '转换组件检测中…', clickable: false }
    default:
      // 缺失、过旧、安装失败都归为“未就绪”，点开安装对话框后在对话框里看失败原因
      return { tone: 'warn', text: '转换组件未就绪', label: '转换组件未就绪', actionLabel: '转换组件未就绪，点击打开安装对话框', clickable: true }
  }
}
