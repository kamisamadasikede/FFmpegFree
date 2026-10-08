// 浏览器预览专用的假数据（只在 window.go 不存在且地址带 ?tasks= 时使用），不代表真实任务。
import type { TaskItem } from './tasks'
import { simEncoderScenario } from '@/api/sim'

/** ?enc=<场景> 时给预览任务带上编码器字段（回退类场景直接是回退后的样子）；没有场景 = 不带 */
function encFields(): Partial<TaskItem> {
  const sc = simEncoderScenario()
  if (!sc) return {}
  return { encoder: sc.encoder, encoderDevice: sc.encoderDevice, ...(sc.hwFallback ? { hwFallback: true, hwFallbackReason: sc.hwFallbackReason } : {}) }
}

const now = () => Date.now()
const min = 60_000

function base(over: Partial<TaskItem> & Pick<TaskItem, 'id' | 'type' | 'status' | 'title'>): TaskItem {
  return {
    inputPaths: [], outputPath: '', progress: 0, speed: '', etaSec: 0, outTimeSec: 0, params: '', version: 1,
    error: null, createdAt: now(), startedAt: 0, finishedAt: 0, ...over,
  }
}

/** ?tasks=N：第 1 个转换（运行中），第 2 个直播，其余排队；N>3 时再补几个运行中的转换 */
export function buildPreviewActive(n: number): TaskItem[] {
  const t = now()
  const all: TaskItem[] = [
    base({
      id: 'p1', type: 'convert', status: 'running', title: '产品发布会_完整版.mov',
      inputPaths: ['/Users/me/Movies/产品发布会_完整版.mov'], outputPath: '/Users/me/Movies/FFmpegFree/产品发布会_完整版.mp4',
      progress: 0.68, speed: '5.47x', etaSec: 72, outTimeSec: 493, params: '{"container":"mp4","targetSizeMb":200}',
      createdAt: t - 40 * min, startedAt: t - 38 * min, ...encFields(),
    }),
    base({
      id: 'p2', type: 'live_screen_push', status: 'running', title: 'B站直播间推流',
      progress: -1, outTimeSec: 2538, createdAt: t - 100 * min, startedAt: t - 99 * min,
    }),
    base({ id: 'p3', type: 'convert', status: 'queued', title: 'vlog_杭州西湖.mkv', ...encFields(), createdAt: t - 30 * min, inputPaths: ['/Users/me/Movies/vlog_杭州西湖.mkv'] }),
    base({ id: 'p4', type: 'office_pdf', status: 'running', title: '用户调研报告.docx', progress: 0.67, speed: '', createdAt: t - 5 * min, startedAt: t - 4 * min }),
    base({ id: 'p5', type: 'convert', status: 'queued', title: '会议录音_0928.wav', createdAt: t - 3 * min }),
  ]
  return all.slice(0, n)
}

export function buildPreviewHistory(n: number): TaskItem[] {
  const t = now()
  const mk = (i: number): TaskItem => {
    const kinds: Array<Partial<TaskItem>> = [
      { type: 'convert', status: 'succeeded', title: '旅行记录_东京.mov', ...encFields(), outputPath: '/Users/me/Movies/FFmpegFree/旅行记录_东京.mp4' },
      {
        type: 'live_screen_push', status: 'failed', title: 'B站直播间推流',
        error: { code: 'LIVE_PUSH_INTERRUPTED', message: '推流被服务器中断', detail: 'Connection reset by peer' },
      },
      // 包 24 N4：推流进程被杀，后端目前落成 failed + INTERNAL → 任务中心按「已中断 / 推流被中断」显示
      {
        type: 'live_file_push', status: 'failed', title: '概念片_终版.mp4',
        error: { code: 'INTERNAL', message: '推流异常退出', detail: 'signal: killed' },
      },
      { type: 'convert', status: 'succeeded', title: '课程录像_第3讲.mkv', outputPath: '/Users/me/Movies/FFmpegFree/课程录像_第3讲.mp4' },
      { type: 'office_pdf', status: 'succeeded', title: '2026 Q3 产品回顾.pptx', outputPath: '/Users/me/Documents/2026 Q3 产品回顾.pdf' },
      {
        type: 'convert', status: 'failed', title: '课程录屏_第五讲.mkv', params: '{"container":"mp4"}', progress: 0.31,
        error: { code: 'CONVERT_DISK_FULL', message: '输出磁盘空间不足，请清理后重试。', detail: 'No space left on device' },
      },
      {
        type: 'convert', status: 'failed', title: 'broken_sample.avi',
        error: { code: 'PROCESS_FAILED', message: '转换组件退出码 1', detail: 'Invalid data found when processing input' },
      },
      { type: 'convert', status: 'canceled', title: 'vlog_杭州西湖.mkv' },
      { type: 'convert', status: 'interrupted', title: '婚礼现场_全程4K.mp4', params: '{"container":"mp4","targetSizeMb":500}', progress: 0.42, error: null },
      { type: 'edit_export', status: 'succeeded', title: '周报.fproj', outputPath: '/Users/me/Movies/FFmpegFree/周报.mp4' },
      // 已下线功能的旧记录（类型显示「旧版导出」）：失败 / 已中断的剪辑导出不显示“重试”，只能移除
      {
        type: 'edit_export', status: 'failed', title: '旅行短片.fproj', progress: 0.55,
        error: { code: 'PROCESS_FAILED', message: '导出没有成功', detail: 'Conversion failed!' },
      },
      { type: 'edit_export', status: 'interrupted', title: '年会.fproj', progress: 0.2, error: null },
    ]
    const k = kinds[i % kinds.length]
    const started = t - (i + 1) * 47 * min
    return base({
      id: 'h' + i, type: 'convert', status: 'succeeded', title: '', ...k,
      progress: k.progress ?? (k.status === 'succeeded' ? 1 : 0.3), createdAt: started - min, startedAt: started, finishedAt: started + (3 + i) * min, version: 10 + i,
    } as any)
  }
  return Array.from({ length: n }, (_, i) => mk(i))
}

export const PREVIEW_LOG = `frame= 12834 fps=142 q=28.0 size=  131072kB time=00:08:13.40 bitrate=2176.4kbits/s speed=5.47x
frame= 13402 fps=142 q=28.0 size=  136960kB time=00:08:35.12 bitrate=2177.1kbits/s speed=5.46x`
