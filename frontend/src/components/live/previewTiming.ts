// 切回预览时的分段计时。写进应用日志（前缀 [preview]），用来区分：取地址、播放器建立、首字节、首帧。
import { LogInfo } from '../../../wailsjs/runtime/runtime'

/** 还没收到 FLV 头时预览服务回 503。前几次隔 200ms 再连，不必每次都等满 1 秒（否则一次失败就把切回来顶过 3 秒）。之后仍是 1 秒，总窗口不变。 */
export function previewRetryDelay(attempt: number): number {
  return attempt < 4 ? 200 : 1000
}

export function previewMark(stage: string, extra = ''): void {
  const line = `[preview] ${stage} ${Date.now()}${extra ? ' ' + extra : ''}`
  console.info(line)
  try { LogInfo(line) } catch { /* 浏览器预览没有 runtime */ }
}
