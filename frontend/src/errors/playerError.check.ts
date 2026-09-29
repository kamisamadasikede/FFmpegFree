// 不引入测试框架的自检：在 /dev/components 页面运行，也可以用 esbuild 打包后在 node 里跑。
import { mapPlayerError, isValidStreamUrl, isValidPullUrl, isPersistentPlayerError, isCrossOrigin, type PlayerErrorInput } from './playerError'
import { resolveError, errorMessages, resolveTaskError, taskErrorMessages } from './errorMessages'

const PAGE = 'http://localhost:5173'
const cases: Array<[string, PlayerErrorInput, string]> = [
  ['地址缺少第二个斜杠', { kind: 'mpegts', type: 'NetworkError', detail: 'Exception', url: 'http:/live.example' }, 'LIVE_URL_INVALID'],
  ['未知协议', { kind: 'video', code: 4, url: 'ftp://a.b/c' }, 'LIVE_URL_INVALID'],
  ['连接超时', { kind: 'mpegts', type: 'NetworkError', detail: 'ConnectingTimeout', url: 'https://a.b/x.flv', pageOrigin: PAGE }, 'LIVE_CONNECT_FAILED'],
  ['跨域 + fetch TypeError', { kind: 'mpegts', type: 'NetworkError', detail: 'Exception', info: { code: -1, msg: 'Failed to fetch' }, url: 'https://a.b/x.flv', pageOrigin: PAGE }, 'LIVE_CORS_BLOCKED'],
  ['跨域 + 状态码 0', { kind: 'mpegts', type: 'NetworkError', detail: 'Exception', info: { code: 0 }, url: 'https://a.b/x.flv', pageOrigin: PAGE }, 'LIVE_CORS_BLOCKED'],
  ['同源的 fetch 失败不算跨域', { kind: 'mpegts', type: 'NetworkError', detail: 'Exception', info: { code: -1, msg: 'Failed to fetch' }, url: PAGE + '/x.flv', pageOrigin: PAGE }, 'LIVE_PLAY_FAILED'],
  ['跨域但有 HTTP 状态码 403', { kind: 'mpegts', type: 'NetworkError', detail: 'HttpStatusCodeInvalid', info: { code: 403, msg: 'Forbidden' }, url: 'https://a.b/x.flv', pageOrigin: PAGE }, 'LIVE_PLAY_FAILED'],
  ['提前结束', { kind: 'mpegts', type: 'NetworkError', detail: 'UnrecoverableEarlyEof', url: 'https://a.b/x.flv', pageOrigin: PAGE }, 'LIVE_PLAY_FAILED'],
  ['媒体格式错误', { kind: 'mpegts', type: 'MediaError', detail: 'FormatError', url: 'https://a.b/x.flv', pageOrigin: PAGE }, 'LIVE_PLAY_FAILED'],
  ['其他错误', { kind: 'mpegts', type: 'OtherError', detail: 'x', url: 'https://a.b/x.flv' }, 'LIVE_PLAY_FAILED'],
  ['video 网络错误（无状态码，不猜跨域）', { kind: 'video', code: 2, url: 'https://a.b/x.mp4', pageOrigin: PAGE }, 'LIVE_PLAY_FAILED'],
  ['video 解码错误', { kind: 'video', code: 3, url: 'https://a.b/x.mp4', pageOrigin: PAGE }, 'LIVE_PLAY_FAILED'],
  ['video 探测到 status 0 且跨域', { kind: 'video', code: 2, status: 0, url: 'https://a.b/x.mp4', pageOrigin: PAGE }, 'LIVE_CORS_BLOCKED'],
]

/** 返回失败项描述，空数组表示全部通过 */
export function runErrorChecks(): string[] {
  const fails: string[] = []
  const eq = (name: string, got: unknown, want: unknown) => {
    if (got !== want) fails.push(`${name}: 期望 ${String(want)}，实际 ${String(got)}`)
  }
  for (const [name, input, want] of cases) eq(name, mapPlayerError(input), want)

  eq('rtmp 地址合法', isValidStreamUrl('rtmp://live.example/app/key'), true)
  eq('srt 地址合法', isValidStreamUrl('srt://1.2.3.4:9000?mode=caller'), true)
  eq('拉流：http 合法', isValidPullUrl('http://a.b/x.flv'), true)
  eq('拉流：wss 合法', isValidPullUrl('wss://a.b/x'), true)
  eq('拉流：rtmp 不合法', isValidPullUrl('rtmp://a.b/x'), false)
  eq('拉流：srt 不合法', isValidPullUrl('srt://1.2.3.4:9000'), false)
  eq('持久：跨域', isPersistentPlayerError('LIVE_CORS_BLOCKED'), true)
  eq('持久：连接失败', isPersistentPlayerError('LIVE_CONNECT_FAILED'), true)
  eq('持久：403', isPersistentPlayerError('LIVE_PLAY_FAILED', 403), true)
  eq('持久：404', isPersistentPlayerError('LIVE_PLAY_FAILED', 404), true)
  eq('暂时：无状态码', isPersistentPlayerError('LIVE_PLAY_FAILED'), false)
  eq('暂时：503', isPersistentPlayerError('LIVE_PLAY_FAILED', 503), false)
  eq('空串不合法', isValidStreamUrl(''), false)
  eq('同源', isCrossOrigin(PAGE + '/a', PAGE), false)
  eq('端口不同即跨域', isCrossOrigin('http://localhost:8080/a', PAGE), true)
  eq('rtmp 不受跨域限制', isCrossOrigin('rtmp://a.b/x', PAGE), false)

  eq('已知码取表文案', resolveError('LIVE_PLAY_FAILED', 'ignored').title, '拉流失败')
  eq('FFMPEG_NOT_FOUND 主按钮', resolveError('FFMPEG_NOT_FOUND').primary?.label, '去设置')
  eq('未知码标题', resolveError('INTERNAL', 'boom').title, '出错了')
  eq('未知码描述取 message', resolveError('INTERNAL', 'boom').description, 'boom')
  eq('未传码为 INTERNAL', resolveError(undefined).code, 'INTERNAL')
  eq('错误码数量', Object.keys(errorMessages).length, 8)
  // 任务中心失败行文案
  const disk = resolveTaskError('CONVERT_DISK_FULL', 'ignored')
  eq('磁盘满标题', disk.title, '磁盘空间不足')
  eq('磁盘满描述', disk.description, '输出位置的可用空间不够，请清理空间或换一个输出文件夹。')
  eq('磁盘满操作', disk.actions.join(','), 'retry,changeOutput,viewLog')
  eq('任务未知码标题', resolveTaskError('PROCESS_FAILED', 'ffmpeg 退出码 1').title, '转换失败')
  eq('任务未知码描述取 message', resolveTaskError('PROCESS_FAILED', 'ffmpeg 退出码 1').description, 'ffmpeg 退出码 1')
  eq('任务无码标题不是出错了', resolveTaskError(undefined).title, '转换失败')
  eq('直播码沿用冻结文案', resolveTaskError('LIVE_PUSH_INTERRUPTED').title, '推流已中断')
  eq('任务专属码数量', Object.keys(taskErrorMessages).length, 1)
  for (const [code, m] of Object.entries(errorMessages)) {
    if (m.description.length > 40) fails.push(`${code} 描述过长，可能超过两行`)
  }
  return fails
}
