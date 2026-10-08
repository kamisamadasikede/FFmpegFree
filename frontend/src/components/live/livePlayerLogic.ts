// 直播播放器里不依赖 DOM 的小规则（check:api 自检）：画面比例（G4）、读屏播报（G5）、「回到最新」胶囊的出现 / 收起（§4.2）。
export type LpPhaseLike = 'empty' | 'connecting' | 'playing' | 'buffering' | 'unsupported' | 'ended' | 'interrupted'

/** G4：父组件指定了比例（截图）就用它；否则用视频自己的宽高比；都没有时 16:9 */
export function stageAspectOf(forced: number | undefined, natural: number | null): number {
  if (forced && forced > 0) return forced
  if (natural && Number.isFinite(natural) && natural > 0) return natural
  return 16 / 9
}

/** 「回到最新」：落后超过 3 秒出现，降到 1.5 秒以下收起（中间保持原样，免得来回闪）；显示的秒数取整 */
export const LAG_SHOW_SEC = 3
export const LAG_HIDE_SEC = 1.5
export function nextLagShown(cur: number | null, lagSec: number): number | null {
  if (!Number.isFinite(lagSec)) return cur
  if (cur == null) return lagSec > LAG_SHOW_SEC ? Math.round(lagSec) : null
  return lagSec < LAG_HIDE_SEC ? null : Math.max(1, Math.round(lagSec))
}

export interface AnnounceInput {
  phase: LpPhaseLike
  kind: 'push' | 'pull'
  /** 进入播放中那一刻是不是静音、有没有声音（之后切换静音不再重播） */
  startMuted: boolean
  hasAudio: boolean
  /** 缓冲已经超过 2 秒 */
  bufferingLong: boolean
  /** 状态层正文（结束 / 被中断 / 不支持） */
  overlayText: string
  endedNote: string
  text: { connecting: string; startedPush: string; startedPull: string; mutedSuffix: string; buffering: string }
}

/** G5：播报区现在该是什么；null = 保持原样（未开始） */
export function liveAnnouncement(a: AnnounceInput): string | null {
  switch (a.phase) {
    case 'connecting':
      return a.text.connecting
    case 'playing':
    case 'buffering':
      if (a.bufferingLong) return a.text.buffering
      return (a.kind === 'pull' ? a.text.startedPull : a.text.startedPush) + (a.startMuted && a.hasAudio ? a.text.mutedSuffix : '')
    case 'ended':
    case 'interrupted':
    case 'unsupported': {
      const t = a.overlayText
      if (a.phase !== 'ended' || !a.endedNote) return t
      return /[。！？]$/.test(t) ? t + a.endedNote : `${t}。${a.endedNote}`
    }
    default:
      return null
  }
}
