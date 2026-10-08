/**
 * 转换记录的模拟（CONVERT_V2_BACKEND_READY = false 或纯浏览器）。导出的函数名、参数、返回形状与契约 v0.23（PR #82）§6.14.3 一致，
 * 和 api/convertRecordsBinding.ts 一一对应，store 不需要知道走的是哪一边。
 * 记录本身是 api/sim.ts 的模拟任务（带 sourceId / params 快照 / result / hiddenInTaskCenter），事件走同一条模拟总线，
 * 所以任务中心、侧边栏角标、原地重试、取消、删除的行为和其它模拟任务一致。源文件行放在本文件的表里。
 *
 * 场景：地址加 ?cv=<场景>（缺省 mixed），对应设计稿的 16 张图：
 *   empty | added | running | done | mixed | dup | missing | conflict | conflict-audio | canceled
 * v0.24（设计稿 19–33b）：fmt-search | fmt-empty | fmt-off | fmt-image | fmt-m4r | copying | copyfail | fallback | savepath | settings-storage |
 *   short | reconv | reconv-moved | old-none。其它 v0.24 参数见文件末尾 V024_MOCK_PARAMS。
 *   弹窗场景由页面读 ?dlg=video|result|audio|gif|unplayable|delete|delete-kid。
 *   ?cv_noapp=1：用系统播放器打开时模拟 NOT_FOUND（reason=no_app）。
 * 预置的进行中记录停在固定进度（截图稳定）；新提交的记录会真的推进（约 8 秒）。
 */
import { AppError } from '@/api/call'
import { probeFiles, type ProbeResult } from '@/api/media'
import { adoptSimTask, cancelSimTask, createSimTask, getSimTask, hideSimFinished, isSimTask, listSimAll, listSimFinished, markSimReconverting, reconvertSimTask, removeSimTasks, simParam, SIM_TITLE_PREFIX, unhideSimTasks } from '@/api/sim'
import { mockFormatCatalog } from '@/api/formatCatalogMock'
import type { ApiTask, ApiTaskResult } from '@/api/taskTypes'
import type {
  AddSourceResult, ConvertSearchFilter, ConvertSource, ConvertSourceEntry, ConvertSourceFilter, ConvertSourcePage, ConvertSubmitRequest,
  DeleteFailure, DeleteResult, PreviewURL, RecordOptions, SourcePathCheck, TaskPage, TaskPathCheck, V023Task,
  ConvertSubmitResult, CopyEvent, FormatEntry, ReconvertRequest, SkippedSource, StorageDirs, StorageDirsUpdate,
} from '@/api/convertRecords'
import { emitSimEvent, onSimEvent } from '@/services/wails'
import type { CopyState } from '@/utils/convertV24Text'
import { formatBytes } from '@/utils/format'
import type { TaskError, TaskStatus } from '@/stores/tasks'
import { normalizeSourcePath } from '@/utils/sourcePath'
import { codecName } from '@/utils/mediaText'
import { store as goStore } from '../../wailsjs/go/models'

export type MockScene = 'empty' | 'added' | 'running' | 'done' | 'mixed' | 'dup' | 'missing' | 'conflict' | 'conflict-audio' | 'canceled'
  | 'fmt-search' | 'fmt-empty' | 'fmt-off' | 'fmt-image' | 'fmt-m4r' | 'copying' | 'copyfail' | 'fallback' | 'savepath' | 'settings-storage' | 'short' | 'reconv' | 'reconv-moved' | 'old-none'
export const MOCK_SCENES: readonly MockScene[] = ['empty', 'added', 'running', 'done', 'mixed', 'dup', 'missing', 'conflict', 'conflict-audio', 'canceled',
  'fmt-search', 'fmt-empty', 'fmt-off', 'fmt-image', 'fmt-m4r', 'copying', 'copyfail', 'fallback', 'savepath', 'settings-storage', 'short', 'reconv', 'reconv-moved', 'old-none']

/** 场景给界面的初始状态（勾选 / 折叠 / 预设 / 本轮完成条）；只在模拟时由 store 读一次。都是 sourceId / 任务 id */
export interface MockSceneUi {
  selected: string[]
  closed: string[]
  open: string[]
  presetId: string
  /** 本轮完成条（“本轮 4 项全部完成”）要统计的记录 */
  roundDone: string[]
  /** v0.24：格式区的初始搜索词（设计稿 19 / 20） */
  formatQuery: string
}

/** 源文件行 + 只有模拟才有的东西（文件在不在、探测结果、能不能在应用里播放） */
interface MSource {
  src: ConvertSource
  pathKey: string
  exists: boolean
  probe?: goStore.MediaInfo
  unplayable?: boolean
  /** v0.24：复制计时器（模拟后台复制） */
  copyTimer?: ReturnType<typeof setInterval>
}
const sources = new Map<string, MSource>()
/** 输出已被移动 / 删除的记录 */
const outputGone = new Set<string>()
/** v0.24.1：旧输出的位置上已经是别的文件（output_moved，不能重转） */
const outputReplaced = new Set<string>()
let ui: MockSceneUi = { selected: [], closed: [], open: [], presetId: '', roundDone: [], formatQuery: '' }
let ready = false
let srcSeq = 0

// ---- 场景数据（与 proto/convert-v2.html 的 K / SRC 一致）----
const GB = 1024 ** 3
const MB = 1024 ** 2
/** 模拟后端：videoCodecName / audioCodecName 由后端的编码名表生成（契约 v0.23.4），这里用前端同写法的表代替；编码为空时不给 */
const mi = (o: Partial<goStore.MediaInfo>): goStore.MediaInfo =>
  goStore.MediaInfo.createFrom({
    id: '', path: '', name: '', size: 0, duration: 0, width: 0, height: 0, videoCodec: '', audioCodec: '', bitrate: 0, thumbUrl: '', hasVideo: true, hasAudio: true, probedAt: 0,
    ...(o.videoCodec ? { videoCodecName: codecName(o.videoCodec) } : {}), ...(o.audioCodec ? { audioCodecName: codecName(o.audioCodec) } : {}), ...o,
  })

const SRC = {
  launch: { path: 'D:\\Footage\\2026\\launch-4k.mov', info: mi({ width: 3840, height: 2160, videoCodec: 'hevc', audioCodec: 'aac', duration: 134, size: 3.8 * GB, container: 'mov' }) },
  iv: { path: 'D:\\Footage\\2026\\采访\\采访-机位A.mkv', info: mi({ width: 1920, height: 1080, videoCodec: 'h264', audioCodec: 'aac', duration: 1120, size: 2.1 * GB, container: 'matroska' }) },
  rec: { path: 'D:\\Videos\\屏幕录制 2026-10-08.mp4', info: mi({ width: 2560, height: 1440, videoCodec: 'h264', audioCodec: 'aac', duration: 206, size: 214 * MB, container: 'mp4' }) },
  recMute: { path: 'D:\\Videos\\屏幕录制 2026-10-08.mp4', info: mi({ width: 2560, height: 1440, videoCodec: 'h264', audioCodec: '', hasAudio: false, duration: 206, size: 214 * MB, container: 'mp4' }) },
  demo: { path: 'D:\\Footage\\产品演示-终版.mov', info: mi({ width: 1920, height: 1080, videoCodec: 'prores', audioCodec: 'pcm_s16le', duration: 302, size: 6.4 * GB, container: 'mov' }), unplayable: true },
  pod: { path: 'E:\\播客\\播客-第12期.wav', info: mi({ width: 0, height: 0, videoCodec: '', audioCodec: 'pcm_s16le', hasVideo: false, sampleRate: 48000, channels: 2, duration: 3130, size: 574 * MB, container: 'wav' }) },
  wed: { path: 'D:\\Footage\\婚礼\\婚礼-初剪.mov', info: mi({ width: 1920, height: 1080, videoCodec: 'h264', audioCodec: 'aac', duration: 492, size: 4.6 * GB, container: 'mov' }), gone: true },
} as const
type SrcKey = keyof typeof SRC

const opt = (o: Partial<RecordOptions>): RecordOptions => ({ container: '', videoCodec: '', audioCodec: '', width: 0, height: 0, fps: 0, videoBitrate: 0, audioBitrate: 0, crf: 0, targetSizeMb: 0, trimStart: 0, trimEnd: 0, ...o })
/** 与 api/convert.ts 的预览预设一致（id / 名字），模拟记录里的 presetName 用后端会给的完整名 */
const PRESET = {
  mp4: { id: 'builtin-mp4-h264', name: 'MP4（H.264 + AAC，通用）', options: opt({ container: 'mp4', videoCodec: 'h264', audioCodec: 'aac' }) },
  mp41080: { id: 'builtin-mp4-h264-1080p', name: 'MP4 1080p（H.264 + AAC）', options: opt({ container: 'mp4', videoCodec: 'h264', audioCodec: 'aac', width: 1920 }) },
  mp4720: { id: 'builtin-mp4-h264-720p', name: 'MP4 720p（H.264 + AAC）', options: opt({ container: 'mp4', videoCodec: 'h264', audioCodec: 'aac', width: 1280 }) },
  h265: { id: 'builtin-mp4-h265', name: 'MP4（H.265 + AAC，体积更小）', options: opt({ container: 'mp4', videoCodec: 'h265', audioCodec: 'aac' }) },
  webm: { id: 'builtin-webm-vp9', name: 'WebM（VP9 + Opus）', options: opt({ container: 'webm', videoCodec: 'vp9', audioCodec: 'opus' }) },
  gif: { id: 'builtin-gif', name: 'GIF 动图（宽 480，12 帧/秒）', options: opt({ container: 'gif', width: 480, fps: 12 }) },
  mp3: { id: 'builtin-mp3', name: 'MP3（192 kbps）', options: opt({ container: 'mp3', audioCodec: 'mp3', audioBitrate: 192000 }) },
  m4r: { id: 'builtin-m4r', name: 'M4R 铃声（AAC 192 kbps）', options: opt({ container: 'm4r', audioCodec: 'aac', audioBitrate: 192000 }) },
  jpg: { id: 'builtin-jpg', name: 'JPG 图片', options: opt({ container: 'jpg' }) },
  /** 自定义参数（没有预设快照）：第 2 行“自定义 · H.264 · 1080p · 8.0 Mbps”（设计截图 05、06 的 采访-机位A.mp4） */
  custom: { id: '', name: '', options: opt({ container: 'mp4', videoCodec: 'h264', audioCodec: 'aac', height: 1080, videoBitrate: 8_000_000 }) },
} as const
type PresetKey = keyof typeof PRESET

const GPU = { encoder: 'h264_nvenc', encoderDevice: 'nvidia-0' }
const CPU = { encoder: 'libx264', encoderDevice: 'cpu' }
const FB = { encoder: 'libx264', encoderDevice: 'cpu', hwFallback: true, hwFallbackReason: 'nvenc_init_failed' }

interface KSpec {
  src: SrcKey
  preset: PresetKey
  out: string
  /** [天数偏移（0 今天 / -1 昨天…）, 'HH:mm'] */
  at: [number, string]
  st: TaskStatus
  p?: number
  speed?: string
  eta?: number
  enc?: { encoder: string; encoderDevice: string; hwFallback?: boolean; hwFallbackReason?: string }
  res?: [number, number, number] // [MB, w, h]
  gone?: boolean
  err?: TaskError
  /** v0.24：旧输出的位置上已经是别的文件（output_moved） */
  replaced?: boolean
  /** v0.24：结果警告（short_output） */
  warn?: string[]
  /** v0.24：正在原地重转（status 是 running，旧 result / finishedAt 不变）：[进度, 速度, 剩余秒] */
  rc?: [number, string, number]
  /** v0.24：最近一次重转失败 */
  rcErr?: TaskError
}
const K: Record<string, KSpec> = {
  launchFail: { src: 'launch', preset: 'webm', out: 'launch-4k.webm', at: [0, '11:52'], st: 'failed', p: 0.37, enc: { encoder: 'libvpx-vp9', encoderDevice: 'cpu' }, err: { code: 'PROCESS_FAILED', message: '转换组件异常退出（退出码 -1）', detail: 'Error while filtering: Cannot allocate memory' } },
  launchRun1080: { src: 'launch', preset: 'mp41080', out: 'launch-4k (1).mp4', at: [0, '11:48'], st: 'running', p: 0.62, speed: '2.1x', eta: 52, enc: GPU },
  launchMp4: { src: 'launch', preset: 'mp4', out: 'launch-4k.mp4', at: [0, '11:20'], st: 'succeeded', enc: FB, res: [412, 3840, 2160] },
  launchMp4Ok: { src: 'launch', preset: 'mp4', out: 'launch-4k.mp4', at: [0, '11:20'], st: 'succeeded', enc: GPU, res: [412, 3840, 2160] },
  launch1080Ok: { src: 'launch', preset: 'mp41080', out: 'launch-4k (1).mp4', at: [0, '11:48'], st: 'succeeded', enc: GPU, res: [138, 1920, 1080] },
  launchWebmOk: { src: 'launch', preset: 'webm', out: 'launch-4k.webm', at: [0, '11:52'], st: 'succeeded', enc: { encoder: 'libvpx-vp9', encoderDevice: 'cpu' }, res: [96, 3840, 2160] },
  launchWebmRun: { src: 'launch', preset: 'webm', out: 'launch-4k.webm', at: [0, '11:52'], st: 'running', p: 0.18, speed: '0.6x', eta: 220, enc: { encoder: 'libvpx-vp9', encoderDevice: 'cpu' } },
  launchGifQ: { src: 'launch', preset: 'gif', out: 'launch-4k.gif', at: [0, '11:53'], st: 'queued', enc: { encoder: 'gif', encoderDevice: 'cpu' } },
  launchCx: { src: 'launch', preset: 'h265', out: 'launch-4k (2).mp4', at: [0, '11:56'], st: 'canceled', p: 0.23, enc: { encoder: 'libx265', encoderDevice: 'cpu' } },
  ivNew: { src: 'iv', preset: 'mp4720', out: '采访-机位A (1).mp4', at: [0, '11:30'], st: 'succeeded', enc: CPU, res: [286, 1280, 720] },
  ivOld: { src: 'iv', preset: 'custom', out: '采访-机位A.mp4', at: [0, '10:05'], st: 'succeeded', enc: GPU, res: [1.2 * 1024, 1920, 1080] },
  recRun: { src: 'rec', preset: 'mp4', out: '屏幕录制 2026-10-08.mp4', at: [0, '11:58'], st: 'running', p: 0.34, speed: '3.4x', eta: 21, enc: GPU },
  recOk: { src: 'rec', preset: 'mp4', out: '屏幕录制 2026-10-08 (1).mp4', at: [0, '11:58'], st: 'succeeded', enc: GPU, res: [41, 2560, 1440] },
  recGif: { src: 'rec', preset: 'gif', out: '屏幕录制 2026-10-08.gif', at: [0, '11:59'], st: 'succeeded', enc: { encoder: 'gif', encoderDevice: 'cpu' }, res: [9, 480, 270] },
  podMp3: { src: 'pod', preset: 'mp3', out: '播客-第12期.mp3', at: [-1, '21:14'], st: 'succeeded', res: [72, 0, 0] },
  wedMp4: { src: 'wed', preset: 'mp4720', out: '婚礼-初剪.mp4', at: [-10, '16:40'], st: 'succeeded', enc: GPU, res: [520, 1280, 720] },
  wedGif: { src: 'wed', preset: 'gif', out: '婚礼-初剪.gif', at: [-10, '16:52'], st: 'succeeded', enc: { encoder: 'gif', encoderDevice: 'cpu' }, res: [18, 480, 270], gone: true },
  // v0.24（设计稿 28 / 30 / 31）
  launchShort: { src: 'launch', preset: 'h265', out: 'launch-4k (2).mp4', at: [0, '12:10'], st: 'succeeded', enc: GPU, res: [186, 3840, 2160], warn: ['short_output'] },
  launchRc: { src: 'launch', preset: 'mp4', out: 'launch-4k.mp4', at: [0, '11:20'], st: 'succeeded', enc: GPU, res: [412, 3840, 2160], rc: [0.48, '1.8x', 40] },
  launchGone: { src: 'launch', preset: 'mp4', out: 'launch-4k.mp4', at: [0, '11:20'], st: 'succeeded', enc: GPU, res: [412, 3840, 2160], gone: true },
}
/** 设计稿 28：时长偏短的那条输出只有 01:32 */
const SHORT_DUR: Record<string, number> = { launchShort: 92 }

/** v0.24 源文件行的复制状态：缺省 ready；none = v0.24 之前的旧行；run = 复制中 [百分比]；space = 空间不足；fail = 复制中断 */
type CopySpec = 'ready' | 'none' | 'canceled' | ['run', number] | 'space' | 'fail'
interface SceneDef {
  /** 源文件 → 记录键（新的在前，与设计稿一致）；记录为空 = 刚添加；第 4 项是 v0.24 的复制状态 */
  src: [SrcKey, string[], [number, string]?, CopySpec?][]
  sel?: SrcKey[]
  closed?: SrcKey[]
  open?: SrcKey[]
  preset?: PresetKey | 'mp4720' | 'mp3'
  roundDone?: string[]
  /** v0.24：存储目录（设计稿 24 / 25 / 26） */
  save?: SaveKey
  uploadsCustom?: string
  formatQuery?: string
}
const SCENES: Record<MockScene, SceneDef> = {
  empty: { src: [] },
  added: { src: [['demo', [], [0, '12:01']], ['rec', [], [0, '12:00']], ['pod', ['podMp3']]], sel: ['demo', 'rec'], closed: ['pod'] },
  running: { src: [['rec', ['recRun']], ['launch', ['launchGifQ', 'launchWebmRun', 'launchRun1080', 'launchMp4Ok']], ['iv', ['ivNew', 'ivOld']], ['pod', ['podMp3']]], closed: ['iv', 'pod'] },
  done: { src: [['rec', ['recGif', 'recOk']], ['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['iv', ['ivNew', 'ivOld']], ['pod', ['podMp3']]], closed: ['iv', 'pod'], roundDone: ['recOk', 'recGif', 'launchWebmOk', 'launch1080Ok'] },
  mixed: { src: [['launch', ['launchFail', 'launchRun1080', 'launchMp4']], ['iv', ['ivNew', 'ivOld']], ['rec', [], [0, '09:30']], ['pod', ['podMp3']], ['wed', ['wedGif', 'wedMp4']]], closed: ['pod', 'wed'], open: ['iv'] },
  dup: { src: [['iv', ['ivNew', 'ivOld']], ['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['pod', ['podMp3']]], sel: ['iv'], closed: ['launch', 'pod'], open: ['iv'], preset: 'mp4720' },
  missing: { src: [['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['wed', ['wedGif', 'wedMp4']], ['pod', ['podMp3']]], closed: ['launch', 'pod'], open: ['wed'] },
  conflict: { src: [['rec', [], [0, '12:02']], ['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['pod', ['podMp3']], ['wed', ['wedGif', 'wedMp4']]], sel: ['rec', 'pod'], closed: ['launch', 'wed'], open: ['pod'] },
  'conflict-audio': { src: [['recMute', [], [0, '12:02']], ['iv', ['ivNew', 'ivOld']], ['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['pod', ['podMp3']]], sel: ['recMute', 'iv'], closed: ['iv', 'launch', 'pod'], preset: 'mp3' },
  canceled: { src: [['launch', ['launchCx', 'launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['iv', ['ivNew', 'ivOld']], ['pod', ['podMp3']]], closed: ['iv', 'pod'] },
  // ---- v0.24（proto/convert-v2.html 的同名 state，设计稿 19–33b）----
  'fmt-search': { src: [['demo', [], [0, '12:01']], ['rec', [], [0, '12:00']], ['pod', ['podMp3']]], sel: ['demo', 'rec'], closed: ['pod'], formatQuery: '苹果' },
  'fmt-empty': { src: [['demo', [], [0, '12:01']], ['rec', [], [0, '12:00']], ['pod', ['podMp3']]], sel: ['demo', 'rec'], closed: ['pod'], formatQuery: 'rmvb' },
  'fmt-off': { src: [['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['iv', ['ivNew', 'ivOld']], ['pod', ['podMp3']]], sel: ['pod'], closed: ['launch', 'iv'], open: ['pod'], preset: 'mp3' },
  'fmt-image': { src: [['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['pod', ['podMp3']]], sel: ['launch', 'pod'], open: ['launch', 'pod'], preset: 'jpg' },
  'fmt-m4r': { src: [['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['pod', ['podMp3']]], sel: ['pod'], closed: ['launch'], open: ['pod'], preset: 'm4r' },
  copying: { src: [['demo', [], [0, '12:01'], ['run', 45]], ['rec', [], [0, '12:00']], ['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['pod', ['podMp3']]], sel: ['demo'], closed: ['launch', 'pod'] },
  copyfail: { src: [['demo', [], [0, '12:01'], 'space'], ['rec', [], [0, '12:00'], 'fail'], ['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['pod', ['podMp3']]], closed: ['launch', 'pod'] },
  fallback: { src: [['rec', [], [0, '12:00']], ['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['pod', ['podMp3']]], sel: ['rec'], open: ['launch'], closed: ['pod'], save: 'fb' },
  savepath: { src: [['iv', ['ivNew', 'ivOld']], ['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['pod', ['podMp3']]], sel: ['iv'], open: ['iv'], closed: ['launch', 'pod'], preset: 'mp4720', save: 'long' },
  'settings-storage': { src: [['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['pod', ['podMp3']]], closed: ['pod'], uploadsCustom: 'E:\\FFmpegFree 素材\\uploads' },
  short: { src: [['launch', ['launchShort', 'launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['iv', ['ivNew', 'ivOld']], ['pod', ['podMp3']]], open: ['launch'], closed: ['iv', 'pod'] },
  reconv: { src: [['launch', ['launchWebmOk', 'launch1080Ok', 'launchRc']], ['iv', ['ivNew', 'ivOld']], ['pod', ['podMp3']]], open: ['launch'], closed: ['iv', 'pod'] },
  'reconv-moved': { src: [['launch', ['launchWebmOk', 'launch1080Ok', 'launchGone']], ['iv', ['ivNew', 'ivOld']], ['pod', ['podMp3']]], open: ['launch'], closed: ['iv', 'pod'] },
  // 设计稿 33b 的背景：采访-机位A 是 v0.24 之前添加的旧行（copyState=none），没有记录
  'old-none': { src: [['launch', ['launchWebmOk', 'launch1080Ok', 'launchMp4Ok']], ['iv', [], [-1, '18:40'], 'none'], ['pod', ['podMp3']]], closed: ['launch', 'pod'] },
}

// ---------------- v0.24 存储目录（6.15.1 / 6.15.2） ----------------
type SaveKey = 'exe' | 'long' | 'fb'
const BASES: Record<SaveKey, { base: string; kind: string; fellBack: boolean }> = {
  exe: { base: 'D:\\Tools\\FFmpegFree', kind: 'exe_dir', fellBack: false },
  long: { base: 'D:\\Program Tools\\视频工具\\FFmpegFree', kind: 'exe_dir', fellBack: false },
  // v0.24.1（10-08）：回退位置是 %LocalAppData%\FFmpegFree
  fb: { base: 'C:\\Users\\何\\AppData\\Local\\FFmpegFree', kind: 'user_data', fellBack: true },
}
let storage: StorageDirs = storageFor('exe', '', '')
function storageFor(key: SaveKey, outCustom: string, upCustom: string): StorageDirs {
  const b = BASES[key]
  const dOut = `${b.base}\\output`
  const dUp = `${b.base}\\uploads`
  return {
    outputDir: outCustom || dOut, uploadsDir: upCustom || dUp, outputCustom: !!outCustom, uploadsCustom: !!upCustom, defaultOutputDir: dOut, defaultUploadsDir: dUp,
    baseKind: b.kind, fellBack: b.fellBack, outputAvailable: true, uploadsAvailable: true,
  }
}
const storedPathOf = (sourceId: string, name: string) => `${storage.uploadsDir}\\${sourceId}\\${name}`


/**
 * paramsSummary 里的编码名（v0.23.2，与后端一致）：H.265 / ProRes 的写法；显卡编码器写基础编码（h264_nvenc → H.264、hevc_qsv → H.265）。
 */
function summaryCodec(v: string): string {
  const base = v.toLowerCase().replace(/_(nvenc|qsv|amf|videotoolbox|vaapi|mf)$/, '')
  const M: Record<string, string> = { h264: 'H.264', libx264: 'H.264', h265: 'H.265', hevc: 'H.265', libx265: 'H.265', vp9: 'VP9', 'libvpx-vp9': 'VP9', av1: 'AV1', prores: 'ProRes', prores_ks: 'ProRes', copy: '原画质', '': '无画面', mpeg4: 'MPEG-4', mpeg2: 'MPEG-2', mpeg2video: 'MPEG-2', wmv2: 'WMV', flv1: 'FLV', flv: 'FLV', theora: 'Theora', libtheora: 'Theora' }
  return M[base] ?? codecName(base) // 表里没有的走前端唯一的编码名表，不自己首字母大写（走查 X3）
}
/** paramsSummary：后端提交时生成（6.14.5），这里照规则模拟 */
const AUDIO_CONT = ['mp3', 'aac', 'wav', 'flac', 'm4a', 'ogg', 'opus', 'wma', 'amr', 'm4r', 'mp2', 'ape', 'wv', 'mmf']
/** v0.24 图片格式（6.16.2）：摘要是“单帧”（加尺寸段） */
const IMAGE_CONT = ['jpg', 'png', 'webp', 'ico', 'bmp', 'tif', 'tga']
export function mockParamsSummary(o: Partial<RecordOptions>): string {
  const c = (o.container ?? '').toLowerCase()
  const seg: string[] = [] // v0.23.1：摘要不再带容器名（界面写“预设名 · 摘要”）
  const audio = AUDIO_CONT.includes(c)
  if (IMAGE_CONT.includes(c)) {
    seg.push('单帧')
    if (o.width && !o.height) seg.push(`宽 ${o.width}`)
    else if (o.height && !o.width) seg.push(`高 ${o.height}`)
    else if (o.width && o.height) seg.push(`${o.width}×${o.height}`)
    return seg.join(' · ')
  }
  if (!audio && c !== 'gif') seg.push(summaryCodec(o.videoCodec ?? ''))
  // 只给宽的预设按常见宽度写成 p 值（后端同规则）：3840/2560/1920/1280/854 → 2160p/1440p/1080p/720p/480p，其它写“宽 N”
  const P_OF_WIDTH: Record<number, string> = { 3840: '2160p', 2560: '1440p', 1920: '1080p', 1280: '720p', 854: '480p' }
  if (o.height && !o.width) seg.push(`${o.height}p`)
  else if (o.width && !o.height) seg.push(P_OF_WIDTH[o.width] ?? `宽 ${o.width}`)
  else if (o.width && o.height) seg.push(`${o.width}×${o.height}`)
  if (o.fps && o.fps > 0) seg.push(`${o.fps} fps`)
  if (!audio && o.videoBitrate) seg.push(`${(o.videoBitrate / 1_000_000).toFixed(1)} Mbps`)
  if (audio && o.audioBitrate) seg.push(`${Math.round(o.audioBitrate / 1000)} kbps`)
  if (o.audioCodec === 'none') seg.push('无声')
  if ((o.trimStart ?? 0) > 0 || (o.trimEnd ?? 0) > 0) seg.push('已裁剪')
  return seg.join(' · ').slice(0, 80)
}
/** 契约 6.9：title 形如 `a.mkv → MP4` */
const titleOf = (input: string, container: string) => `${baseOf(input)} → ${container.toUpperCase()}`

function at([day, hm]: [number, string]): number {
  const [h, m] = hm.split(':').map(Number)
  const d = new Date()
  d.setDate(d.getDate() + day)
  d.setHours(h, m, 0, 0)
  return d.getTime()
}
const sidOf = (k: SrcKey) => `mock-src-${k}`
const baseOf = (p: string) => p.slice(Math.max(p.lastIndexOf('\\'), p.lastIndexOf('/')) + 1)

/** media 表缓存的样子：hasVideo / hasAudio 恒为 false（6.14.2），所以页面不能拿它做冲突预检 */
/** 持久化的媒体信息（G3 起整份入库，hasVideo / hasAudio、采样率、声道、编码显示名都在） */
const cachedMedia = (info: goStore.MediaInfo): goStore.MediaInfo => goStore.MediaInfo.createFrom({ ...info })

function seedScene(scene: MockScene) {
  const def = SCENES[scene]
  const save = (simParam('cv_save') as SaveKey | null) ?? def.save ?? 'exe'
  storage = storageFor(BASES[save] ? save : 'exe', simParam('cv_outcustom') ?? '', simParam('cv_upcustom') ?? def.uploadsCustom ?? '')
  const oldRows = new Set((simParam('cv_old') ?? '').split(',').filter(Boolean))
  for (const [key, kids, added, cpSpec] of def.src) {
    const s = SRC[key]
    const kidTimes = kids.map((k) => at(K[k].at))
    const addedAt = added ? at(added) : Math.min(...kidTimes) - 60_000
    const last = Math.max(addedAt, ...kidTimes)
    const probe = goStore.MediaInfo.createFrom({ ...s.info, path: s.path, name: baseOf(s.path) })
    // v0.24：复制状态。缺省 ready；婚礼-初剪（10 天前、源文件已不在）是 v0.24 之前添加的旧行；?cv_old=rec,iv 把指定的行也当成旧行
    const cp: CopySpec = oldRows.has(key) ? 'none' : cpSpec ?? (key === 'wed' ? 'none' : 'ready')
    sources.set(sidOf(key), {
      // 刚添加、还没提交过的行没有 media 缓存（AddSources 不探测）；提交过的行有 media 缓存
      src: { sourceId: sidOf(key), path: s.path, name: baseOf(s.path), addedAt, lastActivityAt: last, ...(kids.length ? { media: cachedMedia(probe) } : {}), ...copyFields(sidOf(key), s.path, s.info.size, cp) },
      pathKey: normalizeSourcePath(s.path), exists: !('gone' in s && s.gone), probe, ...('unplayable' in s ? { unplayable: true } : {}),
    })
    for (const k of kids) seedRecord(k, K[k])
  }
  ui = {
    selected: (def.sel ?? []).map(sidOf),
    closed: (def.closed ?? []).map(sidOf),
    open: (def.open ?? []).map(sidOf),
    presetId: PRESET[(def.preset ?? 'mp4') as PresetKey].id,
    roundDone: (def.roundDone ?? []).map((k) => `simcv-${k}`),
    formatQuery: simParam('cv_fq') ?? def.formatQuery ?? '',
  }
}
/** v0.24 源文件行的副本字段（6.15.3） */
function copyFields(sourceId: string, path: string, size: number, cp: CopySpec): Pick<ConvertSource, 'originalPath' | 'storedPath' | 'copyState' | 'copiedBytes' | 'totalBytes' | 'copyError'> {
  const total = Math.round(size)
  const stored = storedPathOf(sourceId, baseOf(path))
  if (cp === 'none') return { originalPath: path, storedPath: '', copyState: 'none', copiedBytes: 0, totalBytes: 0 }
  if (cp === 'ready') return { originalPath: path, storedPath: stored, copyState: 'ready', copiedBytes: total, totalBytes: total }
  if (cp === 'canceled') return { originalPath: path, storedPath: stored, copyState: 'canceled', copiedBytes: Math.round(total * 0.3), totalBytes: total }
  if (Array.isArray(cp)) return { originalPath: path, storedPath: stored, copyState: 'copying', copiedBytes: Math.round((total * cp[1]) / 100), totalBytes: total }
  return { originalPath: path, storedPath: stored, copyState: 'failed', copiedBytes: cp === 'fail' ? Math.round(total * 0.6) : 0, totalBytes: total, copyError: cp === 'space' ? noSpaceError(total + 0.1 * GB, 2.1 * GB) : interruptedCopyError() }
}
/** 6.15.4：空间不足（需要 = 文件大小 + 余量；设计稿 23 是“需要 6.5 GB，剩余 2.1 GB”） */
const noSpaceError = (need: number, free: number): TaskError => ({ code: 'CONVERT_DISK_FULL', message: `磁盘空间不足，需要 ${formatBytes(need)}，剩余 ${formatBytes(free)}。`, detail: `reason=no_space\nneedBytes=${Math.round(need)}\nfreeBytes=${Math.round(free)}` })
const interruptedCopyError = (): TaskError => ({ code: 'IO_ERROR', message: '复制被中断，请重试', detail: 'reason=interrupted' })

function seedRecord(key: string, k: KSpec) {
  const id = `simcv-${key}`
  const s = SRC[k.src]
  const p = PRESET[k.preset]
  const createdAt = at(k.at)
  // v0.24：输出都在实际输出目录（6.15.1；旧版本是源文件旁边）
  const out = `${storage.outputDir}\\${k.out}`
  const startedAt = k.st === 'queued' ? 0 : createdAt + 2000
  const finished = k.st === 'succeeded' || k.st === 'failed' || k.st === 'canceled' || k.st === 'interrupted'
  const durSec = s.info.duration
  const outDur = SHORT_DUR[key] ?? durSec
  const params = JSON.stringify({ input: s.path, options: p.options, outputDir: '', presetId: p.id, presetName: p.name, paramsSummary: mockParamsSummary(p.options) })
  const task: ApiTask = {
    id, type: 'convert', status: k.st, title: SIM_TITLE_PREFIX + titleOf(s.path, p.options.container), inputPaths: [s.path], outputPath: out,
    progress: k.st === 'succeeded' ? 1 : k.p ?? 0, speed: k.speed ?? '', etaSec: k.eta ?? 0, params,
    // 完成时间和创建时间在同一分钟（成功记录第 2 行写完成时间，和设计稿一致）
    version: 3, error: k.err ? { ...k.err } : null, createdAt, startedAt, finishedAt: finished ? createdAt + Math.min(55_000, Math.max(20_000, durSec * 300)) : 0,
    sourceId: sidOf(k.src), hiddenInTaskCenter: false,
    ...(k.enc ? { encoder: k.enc.encoder, encoderDevice: k.enc.encoderDevice, ...(k.enc.hwFallback ? { hwFallback: true, hwFallbackReason: k.enc.hwFallbackReason } : {}) } : {}),
    ...(k.res ? { result: { ...resultFor(p.options, outDur, k.res), ...(k.warn ? { warnings: [...k.warn] } : {}) } } : {}),
    ...(k.rcErr ? { lastReconvertError: { code: k.rcErr.code, message: k.rcErr.message, ...(k.rcErr.detail ? { detail: k.rcErr.detail } : {}), at: createdAt + 120_000 } } : {}),
  }
  if (k.rc) Object.assign(task, { status: 'running', reconverting: true, progress: k.rc[0], speed: k.rc[1], etaSec: k.rc[2] })
  adoptSimTask(task, { type: 'convert', title: titleOf(s.path, p.options.container), inputPaths: [s.path], outputPath: out, params, simSeconds: 8, mediaSec: durSec, encoder: k.enc ? { ...k.enc } : undefined, resultOf: guessResult })
  if (k.rc) markSimReconverting(id)
  if (k.gone) outputGone.add(id)
  if (k.replaced) outputReplaced.add(id)
  // 进行中的预置记录也放进任务 store（侧边栏角标、任务中心能看到）；停在固定进度，不推进
  if (k.st === 'running' || k.st === 'queued' || k.rc) emitSimEvent('task:created', getSimTask(id))
}
function resultFor(o: RecordOptions, durSec: number, [mb, w, h]: [number, number, number]): ApiTaskResult {
  const audio = AUDIO_CONT.includes(o.container)
  if (IMAGE_CONT.includes(o.container)) return { sizeBytes: mb * MB, width: w, height: h }
  return { sizeBytes: mb * MB, durationSec: durSec, ...(audio ? {} : { width: w, height: h }), ...(o.audioBitrate ? { audioBitrateKbps: Math.round(o.audioBitrate / 1000) } : audio ? {} : { audioBitrateKbps: 128 }) }
}
/** 新提交的模拟任务成功时给一个看起来合理的 result（真实后端是 ffprobe 输出） */
function guessResult(t: ApiTask): ApiTaskResult | undefined {
  const src = t.sourceId ? sources.get(t.sourceId) : undefined
  const info = src?.probe
  let o: Partial<RecordOptions> = {}
  try { o = JSON.parse(t.params).options ?? {} } catch { /* 忽略 */ }
  const audio = AUDIO_CONT.includes(o.container ?? '')
  const image = IMAGE_CONT.includes(o.container ?? '')
  const sw = info?.width ?? 1920
  const sh = info?.height ?? 1080
  const w = o.width || (o.height && sh ? Math.round((sw * o.height) / sh / 2) * 2 : sw)
  const h = o.height || (o.width && sw ? Math.round((sh * o.width) / sw / 2) * 2 : sh)
  if (image) return { sizeBytes: Math.max(1, Math.round(w * h * 0.4)), width: Math.min(w, o.container === 'ico' ? 256 : w), height: Math.min(h, o.container === 'ico' ? 256 : h) }
  // ?cv_short=1：新完成的转换都带 short_output（时长偏短）
  const short = simParam('cv_short') === '1'
  return { sizeBytes: Math.max(1, (info?.size ?? 100 * MB) * (audio ? 0.12 : 0.18)), durationSec: short ? Math.round((info?.duration ?? 60) * 0.6) : info?.duration ?? 0, ...(audio ? {} : { width: w, height: h }), ...(o.audioBitrate ? { audioBitrateKbps: Math.round(o.audioBitrate / 1000) } : {}), ...(short ? { warnings: ['short_output'] } : {}) }
}

/** 首次调用时按 ?cv= 布置场景；自检里可以 resetConvertMock(场景) 重来 */
function ensure() {
  if (ready) return
  ready = true
  const scene = (simParam('cv') ?? 'mixed') as MockScene
  seedScene(MOCK_SCENES.includes(scene) ? scene : 'mixed')
}
export function resetConvertMock(scene: MockScene): void {
  const mine = listSimAll('convert').map((t) => t.id)
  for (const id of mine) {
    const t = getSimTask(id)
    if (t && (t.status === 'queued' || t.status === 'running')) cancelSimTask(id)
  }
  if (mine.length) removeSimTasks(mine)
  for (const m of sources.values()) clearInterval(m.copyTimer)
  sources.clear()
  outputGone.clear()
  outputReplaced.clear()
  copySeq = 0
  ready = true
  seedScene(scene)
}
/** 场景给界面的初始状态（只读一次） */
export function mockSceneUi(): MockSceneUi {
  ensure()
  return ui
}

// ---------------- 组装 ----------------
const toV023 = (t: ApiTask): V023Task => ({ ...t, error: t.error ?? undefined, hiddenInTaskCenter: !!t.hiddenInTaskCenter }) as V023Task
const recordsOf = (sourceId: string): V023Task[] => listSimAll('convert').filter((t) => t.sourceId === sourceId).map(toV023) // createdAt 倒序
const copySource = (m: MSource): ConvertSource => ({ ...m.src, ...(m.src.media ? { media: goStore.MediaInfo.createFrom(m.src.media) } : {}) })
const byActivity = () => [...sources.values()].sort((a, b) => b.src.lastActivityAt - a.src.lastActivityAt || (a.src.sourceId < b.src.sourceId ? 1 : -1))
const clampFilter = (f: ConvertSourceFilter) => {
  const limit = f.limit || 50
  const recordLimit = f.recordLimit || 20
  if (limit < 1 || limit > 200 || recordLimit < 1 || recordLimit > 100 || f.offset < 0) throw new AppError('INVALID_ARGUMENT', '分页参数不正确')
  return { limit, recordLimit, offset: f.offset }
}
function entryOf(m: MSource, recordLimit: number): ConvertSourceEntry {
  const all = recordsOf(m.src.sourceId)
  return { source: copySource(m), records: all.slice(0, recordLimit), recordCount: all.length }
}
const notFoundRecord = (what = '记录不存在') => new AppError('NOT_FOUND', what, 'reason=record')
const notFoundFile = () => new AppError('NOT_FOUND', '文件已被移动或删除', 'reason=file')
const mustSource = (id: string): MSource => {
  ensure()
  const m = sources.get(id)
  if (!m) throw notFoundRecord('源文件不存在')
  return m
}
const touch = (m: MSource) => { m.src.lastActivityAt = Math.max(Date.now(), m.src.lastActivityAt + 1) }

// ---------------- ConvertService ----------------
/** 6.16.4：前端写死的输入扩展名表之外的文件 AddSources 单项 UNSUPPORTED（reason=format）；模拟用同一张表 */
export const MOCK_INPUT_EXTS = ['mp4', 'mkv', 'mov', 'webm', 'avi', 'flv', 'gif', 'wmv', 'mpg', 'mpeg', 'vob', '3gp', 'swf', 'ogv', 'm4v', 'ts', 'mts', 'm2ts', 'rmvb', 'rm', 'asf', 'f4v',
  'mp3', 'm4a', 'aac', 'wav', 'flac', 'ogg', 'opus', 'wma', 'amr', 'm4r', 'mp2', 'ape', 'wv', 'mmf', 'aiff', 'aif', 'ac3', 'jpg', 'jpeg', 'png', 'webp', 'ico', 'bmp', 'tif', 'tiff', 'tga']
export async function AddSources(paths: string[]): Promise<AddSourceResult[]> {
  ensure()
  if (!paths.length || paths.length > 500) throw new AppError('INVALID_ARGUMENT', '一次最多添加 500 个文件')
  return paths.map((path) => {
    const ext = (baseOf(path).split('.').pop() ?? '').toLowerCase()
    if (!MOCK_INPUT_EXTS.includes(ext)) return { path, existed: false, error: { code: 'UNSUPPORTED', message: '不支持这个格式', detail: 'reason=format' } }
    const key = normalizeSourcePath(path)
    const cur = [...sources.values()].find((m) => m.pathKey === key)
    if (cur) {
      touch(cur)
      return { path, source: copySource(cur), existed: true }
    }
    const now = Date.now()
    const sourceId = `mock-src-n${Date.now().toString(36)}${(++srcSeq).toString(36)}`
    const total = mockSizeOf(path)
    const m: MSource = {
      src: { sourceId, path, name: baseOf(path), addedAt: now, lastActivityAt: now, originalPath: path, storedPath: storedPathOf(sourceId, baseOf(path)), copyState: 'copying', copiedBytes: 0, totalBytes: total },
      pathKey: key, exists: true,
    }
    sources.set(sourceId, m)
    startCopy(m)
    return { path, source: copySource(m), existed: false }
  })
}

// ---------------- v0.24 副本（6.15.4）：模拟后台复制，发 convert:copy ----------------
let copySeq = 0
const mockSizeOf = (path: string): number => {
  let h = 0
  for (const ch of path) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return Math.round((80 + (h % 900)) * MB)
}
function emitCopy(m: MSource) {
  const s = m.src
  const ev: CopyEvent = { sourceId: s.sourceId, seq: ++copySeq, copyState: s.copyState ?? 'none', copiedBytes: s.copiedBytes ?? 0, totalBytes: s.totalBytes ?? 0, storedPath: s.storedPath ?? '', ...(s.copyError ? { error: { ...s.copyError } } : {}) }
  emitSimEvent('convert:copy', ev)
}
/**
 * 开始复制：先发 copying（copiedBytes=0），约 3 秒（?cv_copyms=毫秒）复制完。
 * ?cv_copyfail=no_space|interrupted：新复制的都失败；?cv_copyhold=1：停在“等待复制”不动（看排队的样子）。
 */
function startCopy(m: MSource) {
  clearInterval(m.copyTimer)
  const s = m.src
  delete s.copyError
  s.copyState = 'copying'
  s.copiedBytes = 0
  emitCopy(m)
  const fail = simParam('cv_copyfail')
  if (fail === 'no_space') {
    s.copyState = 'failed'
    s.copyError = noSpaceError((s.totalBytes ?? 0) + 0.1 * GB, (s.totalBytes ?? 0) * 0.3)
    queueMicrotask(() => emitCopy(m))
    return
  }
  if (simParam('cv_copyhold') === '1') return
  const ms = Number(simParam('cv_copyms')) || 3000
  const t0 = Date.now()
  m.copyTimer = setInterval(() => {
    const p = Math.min(1, (Date.now() - t0) / ms)
    s.copiedBytes = Math.round((s.totalBytes ?? 0) * p)
    if (fail === 'interrupted' && p >= 0.6) {
      clearInterval(m.copyTimer)
      s.copyState = 'failed'
      s.copyError = interruptedCopyError()
    } else if (p >= 1) {
      clearInterval(m.copyTimer)
      s.copyState = 'ready'
    }
    emitCopy(m)
  }, 250)
}
export async function CancelCopy(sourceId: string): Promise<void> {
  const m = mustSource(sourceId)
  if (m.src.copyState === 'canceled') return
  if (m.src.copyState !== 'copying') throw new AppError('TASK_CONFLICT', '没有正在进行的复制')
  clearInterval(m.copyTimer)
  m.src.copyState = 'canceled'
  emitCopy(m)
}
export async function RetryCopy(sourceId: string): Promise<ConvertSource> {
  const m = mustSource(sourceId)
  const st = m.src.copyState
  if (!st || st === 'none') throw new AppError('INVALID_ARGUMENT', '这个文件不需要复制')
  if (st === 'copying' || st === 'ready') throw new AppError('TASK_CONFLICT', '文件已经在复制或已复制完成')
  if (!m.exists) {
    m.src.copyState = 'failed'
    m.src.copyError = { code: 'NOT_FOUND', message: '原文件已不存在', detail: 'reason=file' }
    emitCopy(m)
    return copySource(m)
  }
  touch(m)
  startCopy(m)
  return copySource(m)
}
/** 副本没就绪（copying / failed / canceled）；none（旧行，读原文件）和 ready 都算就绪 */
const copyNotReady = (m: MSource): boolean => m.src.copyState === 'copying' || m.src.copyState === 'failed' || m.src.copyState === 'canceled'
export const onCopy = (cb: (e: CopyEvent) => void): (() => void) => onSimEvent<CopyEvent>('convert:copy', cb)
/** v0.23.1 status：行里有任一记录满足（EXISTS）；内嵌记录和 recordCount 不过滤 */
const STATUS_OF: Record<string, readonly TaskStatus[] | null> = { '': null, active: ['queued', 'running'], failed: ['failed', 'interrupted'] }
/** v0.24：active 加上正在复制的行，failed 加上复制失败的行（已取消复制不算） */
const COPY_OF: Record<string, CopyState | null> = { '': null, active: 'copying', failed: 'failed' }
function statusMatcher(status: string | undefined): (m: MSource) => boolean {
  const st = status ?? ''
  if (!Object.prototype.hasOwnProperty.call(STATUS_OF, st)) throw new AppError('INVALID_ARGUMENT', '筛选条件不正确', `status=${st}`)
  const want = STATUS_OF[st]
  return (m) => !want || m.src.copyState === COPY_OF[st] || recordsOf(m.src.sourceId).some((t) => want.includes(t.status))
}
export async function ListSources(f: ConvertSourceFilter): Promise<ConvertSourcePage> {
  ensure()
  const { limit, recordLimit, offset } = clampFilter(f)
  const all = byActivity().filter(statusMatcher(f.status))
  return { items: all.slice(offset, offset + limit).map((m) => entryOf(m, recordLimit)), total: all.length }
}
/** v0.23.1：单个源文件行（和 ListSources 的一项同形） */
export async function GetSource(sourceId: string): Promise<ConvertSourceEntry> {
  const m = mustSource(sourceId) // 不存在：NOT_FOUND reason=record
  return entryOf(m, clampFilter({ limit: 1, offset: 0, recordLimit: 0 }).recordLimit)
}
export async function ListSourceRecords(sourceId: string, limit: number, offset: number): Promise<TaskPage> {
  mustSource(sourceId)
  const all = recordsOf(sourceId)
  const n = limit || 50
  return { items: all.slice(offset, offset + n), total: all.length }
}
export async function SearchSources(f: ConvertSearchFilter): Promise<ConvertSourcePage> {
  ensure()
  const k = f.keyword.trim().toLowerCase()
  if (!k || k.length > 100) throw new AppError('INVALID_ARGUMENT', '搜索关键字不正确')
  const { limit, recordLimit, offset } = clampFilter(f)
  const match = statusMatcher(f.status) // v0.23.2：和 ListSources 同样的 status
  const hits: ConvertSourceEntry[] = []
  for (const m of byActivity().filter(match)) {
    const nameMatched = m.src.name.toLowerCase().includes(k)
    const matchedTaskIds = recordsOf(m.src.sourceId).filter((t) => baseOf(t.outputPath).toLowerCase().includes(k)).map((t) => t.id)
    if (nameMatched || matchedTaskIds.length) hits.push({ ...entryOf(m, recordLimit), nameMatched, matchedTaskIds })
  }
  return { items: hits.slice(offset, offset + limit), total: hits.length }
}
export async function CheckSources(ids: string[]): Promise<SourcePathCheck[]> {
  ensure()
  return ids.map((sourceId) => {
    const m = sources.get(sourceId)
    // v0.24：originalExists / storedExists；exists = 读取路径存在（ready 看副本，其余看原文件）
    const stored = !!m && m.src.copyState === 'ready' && !!m.src.storedPath
    return { sourceId, found: !!m, exists: !!m && (stored || m.exists), originalExists: !!m && m.exists, storedExists: stored }
  })
}

/** 6.14.5：期望名、` (1)`、` (2)`…；未结束任务占的名字、磁盘上的（模拟：已成功且输出还在的）、输入路径都算占用 */
function takenKeys(): Set<string> {
  const out = new Set<string>()
  for (const t of listSimAll('convert')) {
    const alive = t.status === 'queued' || t.status === 'running' || (t.status === 'succeeded' && !outputGone.has(t.id))
    if (alive && t.outputPath) out.add(normalizeSourcePath(t.outputPath))
  }
  for (const m of sources.values()) out.add(m.pathKey)
  return out
}
function outputPathFor(input: string, container: string, outputDir: string, taken: Set<string>): string {
  // v0.24（6.15.1）：outputDir 为空 = 实际输出目录（<base>/output 或设置里的自定义目录），不再是源文件旁边
  const dirIn = outputDir || storage.outputDir
  const sep = dirIn.includes('\\') ? '\\' : '/'
  const dir = dirIn.replace(/[\\/]+$/, '') + sep
  const base = baseOf(input).replace(/\.[^.]+$/, '')
  for (let n = 0; n <= 99; n++) {
    const full = `${dir}${base}${n ? ` (${n})` : ''}.${container}`
    if (!taken.has(normalizeSourcePath(full))) return full
  }
  throw new AppError('IO_ERROR', '同名文件太多，换一个输出文件夹试试')
}
export async function PreviewOutputName(sourceId: string, opts: RecordOptions, outputDir: string): Promise<string> {
  const m = mustSource(sourceId)
  return outputPathFor(m.src.path, opts.container, outputDir, takenKeys())
}

let mockSeq = 0
function submitOne(m: MSource, options: RecordOptions, outputDir: string, presetId: string, presetName: string, summary: string, taken: Set<string>): V023Task {
  const output = outputPathFor(m.src.path, options.container, outputDir, taken)
  taken.add(normalizeSourcePath(output))
  const params = JSON.stringify({ input: m.src.path, options, outputDir, presetId, presetName, paramsSummary: summary })
  const t = createSimTask({
    type: 'convert', title: titleOf(m.src.path, options.container), inputPaths: [m.src.path], outputPath: output, params,
    simSeconds: 8 + (mockSeq++ % 3) * 2, mediaSec: m.probe?.duration ?? 60, resultOf: guessResult, sourceId: m.src.sourceId,
  })
  touch(m)
  return toV023(t)
}
const catalog = (): FormatEntry[] => mockFormatCatalog(mockParamsSummary)
const presetById = (id: string) => catalog().flatMap((f) => f.presets).find((p) => p.id === id)
/** 格式不可输出：UNSUPPORTED reason=format（6.16.1） */
function mustEncodable(container: string) {
  const f = catalog().find((x) => x.extension === container)
  if (f && !f.encodable) throw new AppError('UNSUPPORTED', f.reason ?? '当前转换组件不支持输出这个格式', 'reason=format')
}
/**
 * v0.24（6.15.4 第 6、7 条）：没复制好的行跳过，只提交就绪的（none 旧行读原文件，算就绪）；一行都没就绪时 TASK_CONFLICT
 * （有正在复制的 reason=copying，否则 copy_failed；第二行 sourceId=第一个）。
 */
export async function SubmitSources(req: ConvertSubmitRequest): Promise<ConvertSubmitResult> {
  ensure()
  if (!req.sourceIds.length || req.sourceIds.length > 50) throw new AppError('INVALID_ARGUMENT', '一次最多转换 50 个文件')
  const list = req.sourceIds.map(mustSource)
  const skipped: SkippedSource[] = list.filter(copyNotReady).map((m) => ({ sourceId: m.src.sourceId, reason: m.src.copyState === 'copying' ? 'copying' : m.src.copyState === 'failed' ? 'copy_failed' : 'copy_canceled' }))
  const readyList = list.filter((m) => !copyNotReady(m))
  if (!readyList.length) {
    const copying = skipped.find((x) => x.reason === 'copying')
    throw new AppError('TASK_CONFLICT', copying ? '文件还在复制，请等复制完成后再转换' : '文件复制没有完成，请先重试复制', `reason=${copying ? 'copying' : 'copy_failed'}\nsourceId=${(copying ?? skipped[0]).sourceId}`)
  }
  for (const m of readyList) if (!m.exists && m.src.copyState !== 'ready') throw new AppError('NOT_FOUND', '源文件已不存在', m.src.path)
  const preset = req.presetId ? presetById(req.presetId) : undefined
  if (req.presetId && !preset) throw notFoundRecord('预设不存在')
  mustEncodable(req.options.container)
  const taken = takenKeys()
  const summary = mockParamsSummary(req.options)
  return { tasks: readyList.map((m) => submitOne(m, req.options, req.outputDir, req.presetId, preset?.name ?? '', summary, taken)), skipped }
}
/**
 * v0.24 原地重转（6.17.1）的校验和启动。?cv_rcfail=PROCESS_FAILED|in_use：重转跑到一半失败（看失败提示）。
 * 旧输出不在（outputGone）→ 按原来的参数重新生成（v0.24.1 regenerate，不带参数）；旧位置是别的文件（outputReplaced）→ output_moved。
 */
export async function Reconvert(req: ReconvertRequest): Promise<V023Task> {
  ensure()
  const t = getSimTask(req.taskId)
  if (!t) throw notFoundRecord()
  if (t.type !== 'convert') throw new AppError('INVALID_ARGUMENT', '不是转换记录')
  if (t.status !== 'succeeded' || t.reconverting) throw new AppError('TASK_CONFLICT', t.reconverting ? '这条记录正在重转' : '只有完成的记录才能重转', 'reason=invalid_state')
  const m = mustSource(t.sourceId ?? '')
  if (!m.exists && m.src.copyState !== 'ready') throw new AppError('NOT_FOUND', '源文件已不存在，不能重转', `reason=file\n${m.src.originalPath || m.src.path}`)
  if (copyNotReady(m)) throw new AppError('TASK_CONFLICT', '文件复制完成后才能重转', `reason=${m.src.copyState === 'copying' ? 'copying' : 'copy_failed'}\nsourceId=${m.src.sourceId}`)
  if (outputReplaced.has(t.id)) throw new AppError('TASK_CONFLICT', '原来的位置已经有别的文件，不能重转', 'reason=output_moved')
  const old = JSON.parse(t.params)
  const cur = String(old.options?.container ?? '')
  let next = old
  if (req.presetId || req.options) {
    const preset = req.presetId ? presetById(req.presetId) : undefined
    if (req.presetId && !preset) throw notFoundRecord('预设不存在')
    const options = req.options ?? preset!.options
    if (options.container !== cur) throw new AppError('INVALID_ARGUMENT', '重转不能更换格式，要换格式请新转一条', 'reason=format_change')
    next = { ...old, options, presetId: req.presetId ?? '', presetName: preset?.name ?? '', paramsSummary: mockParamsSummary(options) }
  }
  mustEncodable(cur)
  const f = simParam('cv_rcfail')
  const fail = f === 'in_use'
    ? { code: 'IO_ERROR' as const, message: '替换原来的文件失败', detail: `reason=in_use\n${t.outputPath}`, atProgress: 0.99 }
    : f ? { code: 'PROCESS_FAILED' as const, message: '转换组件异常退出（退出码 1）', atProgress: 0.5 } : undefined
  touch(m)
  const task = reconvertSimTask(t.id, JSON.stringify(next), fail)
  // 重新生成成功后旧输出又在了
  if (outputGone.has(t.id)) pendingRegenerate.add(t.id)
  return toV023(task)
}
/** 重新生成中的记录：成功后输出回来了（从 outputGone 里去掉） */
const pendingRegenerate = new Set<string>()
onSimEvent<{ id: string; reconvertOutcome?: string }>('task:status', (p) => {
  if (!p.reconvertOutcome || !pendingRegenerate.has(p.id)) return
  pendingRegenerate.delete(p.id)
  if (p.reconvertOutcome === 'succeeded') outputGone.delete(p.id)
})

/** 6.14.4：进行中的先取消；只删成功记录的输出；文件删不掉不是错误（模拟：?cv_delfail=in_use|permission|… 让每个要删的文件都失败；still_running 让进行中的停不下来；可用逗号组合，如 in_use,still_running） */
function doDelete(ids: string[], deleteOutputs: boolean): DeleteResult {
  const mine = [...new Set(ids)].filter((id) => isSimTask(id) && getSimTask(id)?.type === 'convert')
  const failures: DeleteFailure[] = []
  let deletedFiles = 0
  const kept = new Set<string>()
  const fails = (simParam('cv_delfail') ?? '').split(',').filter(Boolean)
  const stuck = fails.includes('still_running')
  const fileFails = fails.filter((x) => x !== 'still_running') // 多个文件类原因时按文件轮流用（检查 / 截图混合提示用）
  let fi = 0
  const MSG: Record<string, string> = { in_use: '文件正在被使用，没有删除', permission: '没有权限删除这个文件', not_task_output: '文件已被替换或移动，没有删除', io: '删除文件失败', still_running: '任务还没停下来，没有删除这条记录' }
  for (const id of mine) {
    const t = getSimTask(id)!
    const wasDone = t.status === 'succeeded'
    const wasActive = t.status === 'queued' || t.status === 'running'
    if (stuck && wasActive) {
      // ?cv_delfail=still_running：进行中的记录 10 秒内没停下来（路径为空，记录不删）
      failures.push({ taskId: id, path: '', reason: 'still_running', message: MSG.still_running })
      kept.add(id)
      continue
    }
    if (wasActive) cancelSimTask(id)
    if (deleteOutputs && wasDone && !outputGone.has(id)) {
      const fail = fileFails.length ? fileFails[fi++ % fileFails.length] : ''
      if (fail && MSG[fail]) failures.push({ taskId: id, path: t.outputPath, reason: fail, message: MSG[fail] })
      else deletedFiles++
    }
    outputGone.delete(id)
  }
  const gone = mine.filter((id) => !kept.has(id))
  if (gone.length) removeSimTasks(gone)
  return { deletedTaskIds: gone, deletedSourceIds: [], deletedFiles, failures }
}
export async function DeleteRecords(taskIds: string[], deleteOutputs: boolean): Promise<DeleteResult> {
  ensure()
  if (!taskIds.length || taskIds.length > 500) throw new AppError('INVALID_ARGUMENT', '一次最多删除 500 条记录')
  return doDelete(taskIds, deleteOutputs)
}
export async function DeleteSource(sourceId: string, deleteOutputs: boolean): Promise<DeleteResult> {
  const m = mustSource(sourceId)
  const r = doDelete(recordsOf(sourceId).map((t) => t.id), deleteOutputs)
  // 契约 6.14.4：有 still_running（记录没删）时这一行保留，deletedSourceIds 为空
  if (r.failures.some((f) => f.reason === 'still_running')) return r
  // v0.24（6.15.7）：在复制的先停（发 canceled），副本一起删；?cv_delfail=copy 模拟副本删不掉（DeleteFailure 带 sourceId）
  if (m.src.copyState === 'copying') {
    clearInterval(m.copyTimer)
    m.src.copyState = 'canceled'
    emitCopy(m)
  }
  if ((simParam('cv_delfail') ?? '').split(',').includes('copy') && m.src.storedPath) r.failures.push({ taskId: '', sourceId, path: m.src.storedPath, reason: 'in_use', message: '文件正在被使用，没有删除' })
  sources.delete(sourceId)
  return { ...r, deletedSourceIds: [sourceId] }
}

/** 模拟的播放地址：mock:<类型>，播放器显示示例画面并用计时器模拟播放；“播放不了”的源文件给一个真的会报错的地址 */
export const MOCK_URL_PREFIX = 'mock:'
export const MOCK_UNPLAYABLE_URL = 'data:video/quicktime;base64,AAAAAA=='
const kindOf = (container: string) => (container === 'gif' ? 'gif' : AUDIO_CONT.includes(container) ? 'audio' : 'video')
const MIME: Record<string, string> = { video: 'video/mp4', audio: 'audio/mpeg', gif: 'image/gif' }
function urlFor(kind: string, size: number): PreviewURL {
  return { url: MOCK_URL_PREFIX + kind, mime: MIME[kind], size }
}
export async function GetSourcePreviewURL(sourceId: string): Promise<PreviewURL> {
  const m = mustSource(sourceId)
  // v0.24：复制中的行不能预览（TASK_CONFLICT reason=copying，只有一行）
  if (m.src.copyState === 'copying') throw new AppError('TASK_CONFLICT', '文件还在复制，复制完成后才能预览', 'reason=copying')
  if (m.src.copyState === 'failed' || m.src.copyState === 'canceled') throw new AppError('TASK_CONFLICT', '文件没有复制成功，不能预览', 'reason=copy_failed')
  if (!m.exists && m.src.copyState !== 'ready') throw notFoundFile()
  if (m.unplayable) return { url: MOCK_UNPLAYABLE_URL, mime: 'video/quicktime', size: m.probe?.size ?? 0 }
  const ext = (m.src.name.split('.').pop() ?? '').toLowerCase()
  return urlFor(m.probe && !m.probe.width ? 'audio' : kindOf(ext), m.probe?.size ?? 0)
}
const noApp = () => simParam('cv_noapp') === '1'
export async function OpenSourceWithSystem(sourceId: string): Promise<void> {
  const m = mustSource(sourceId)
  if (!m.exists) throw notFoundFile()
  if (noApp()) throw new AppError('NOT_FOUND', '没有找到能打开这个文件的程序', 'reason=no_app')
}
export async function RevealSource(sourceId: string): Promise<void> {
  const m = mustSource(sourceId)
  if (!m.exists) throw notFoundFile()
  console.info('[模拟] 打开所在文件夹', m.src.path)
}
/** §6.14.10：文件不在 NOT_FOUND(reason=file)；纯音频 UNSUPPORTED(reason=format)；否则 data URL（模拟是渐变 SVG） */
const noPicture = () => new AppError('UNSUPPORTED', '这个文件没有画面', 'reason=format')
/** ?cv_thumb=fail：模拟 Windows 上截图失败（INTERNAL）；slow：一直在生成；hold：等 releaseMockThumbs() 再返回（自检用） */
const thumbMode = () => simParam('cv_thumb') ?? ''
const thumbHolds: (() => void)[] = []
export function releaseMockThumbs(): void {
  thumbHolds.splice(0).forEach((r) => r())
}
async function thumbFault(): Promise<void> {
  if (thumbMode() === 'fail') throw new AppError('INTERNAL', '生成缩略图失败', 'reason=thumbnail')
  if (thumbMode() === 'slow') await new Promise(() => undefined)
  if (thumbMode() === 'hold') await new Promise<void>((r) => thumbHolds.push(r))
}
export async function GetSourceThumbnail(sourceId: string): Promise<string> {
  const m = mustSource(sourceId)
  if (!m.exists) throw notFoundFile()
  await thumbFault()
  const audio = m.probe ? !m.probe.width : AUDIO_CONT.includes((m.src.name.split('.').pop() ?? '').toLowerCase())
  if (audio) throw noPicture()
  return mockThumbnail(m.src.path)
}
export async function GetRecordThumbnail(taskId: string): Promise<string> {
  ensure()
  const t = getSimTask(taskId)
  if (!t) throw notFoundRecord()
  if (t.type !== 'convert') throw new AppError('INVALID_ARGUMENT', '不是转换记录')
  if (t.status !== 'succeeded' || outputGone.has(taskId)) throw notFoundFile()
  let c = ''
  try { c = JSON.parse(t.params).options?.container ?? '' } catch { /* 忽略 */ }
  if (AUDIO_CONT.includes(c)) throw noPicture()
  await thumbFault()
  return mockThumbnail(t.inputPaths[0] ?? t.outputPath)
}

// ---------------- TaskService（v0.23 新增 / 改动的部分） ----------------
export async function HideFinishedInTaskCenter(): Promise<number> {
  return hideSimFinished()
}
export async function UnhideInTaskCenter(ids: string[]): Promise<void> {
  if (!ids.length || ids.length > 500) throw new AppError('INVALID_ARGUMENT', '一次最多 500 条')
  for (const id of ids) if (!isSimTask(id)) throw notFoundRecord()
  unhideSimTasks(ids)
}
export async function List(f: { types: string[]; statuses: TaskStatus[]; limit: number; offset: number; includeHidden: boolean }): Promise<TaskPage> {
  const all = listSimFinished(f.includeHidden).filter((t) => (!f.types.length || f.types.includes(t.type)) && (!f.statuses.length || f.statuses.includes(t.status)))
  return { items: all.slice(f.offset, f.offset + f.limit).map(toV023), total: all.length }
}
export async function CheckPaths(taskIds: string[]): Promise<TaskPathCheck[]> {
  ensure()
  return taskIds.map((taskId) => {
    const t = getSimTask(taskId)
    if (!t) return { taskId, found: false, inputExists: false, outputExists: false, reconvertMode: '' as const, reconvertBlock: 'invalid_state' }
    const m = t.sourceId ? sources.get(t.sourceId) : undefined
    const outputExists = t.status === 'succeeded' && !outputGone.has(taskId) && !outputReplaced.has(taskId)
    return { taskId, found: true, inputExists: m ? m.exists : true, outputExists, ...reconvertCheck(t, m) }
  })
}
/**
 * v0.24.1 TaskPathCheck.reconvertMode / reconvertBlock（取代 canReconvert）：源文件不在优先；副本没就绪 copy_not_ready；
 * 旧位置是别的文件 output_moved；旧输出不在 → regenerate；在 → replace。
 */
function reconvertCheck(t: ApiTask, m: MSource | undefined): { reconvertMode: '' | 'replace' | 'regenerate'; reconvertBlock: string } {
  if (t.status !== 'succeeded' || t.reconverting) return { reconvertMode: '', reconvertBlock: 'invalid_state' }
  if (!m || (!m.exists && m.src.copyState !== 'ready')) return { reconvertMode: '', reconvertBlock: 'source_missing' }
  if (copyNotReady(m)) return { reconvertMode: '', reconvertBlock: 'copy_not_ready' }
  if (outputReplaced.has(t.id)) return { reconvertMode: '', reconvertBlock: 'output_moved' }
  return { reconvertMode: outputGone.has(t.id) ? 'regenerate' : 'replace', reconvertBlock: '' }
}
export async function GetPreviewURL(taskId: string, which: 'input' | 'output'): Promise<PreviewURL> {
  ensure()
  const t = getSimTask(taskId)
  if (!t) throw notFoundRecord()
  if (which === 'input') return GetSourcePreviewURL(t.sourceId ?? '')
  if (t.status !== 'succeeded' || outputGone.has(taskId)) throw notFoundFile()
  let c = ''
  try { c = JSON.parse(t.params).options?.container ?? '' } catch { /* 忽略 */ }
  return urlFor(kindOf(c), t.result?.sizeBytes ?? 0)
}
export async function OpenWithSystem(taskId: string, which: 'input' | 'output'): Promise<void> {
  ensure()
  const t = getSimTask(taskId)
  if (!t) throw notFoundRecord()
  if (which === 'output' && (t.status !== 'succeeded' || outputGone.has(taskId))) throw notFoundFile()
  if (which === 'input') {
    const m = t.sourceId ? sources.get(t.sourceId) : undefined
    if (m && !m.exists) throw notFoundFile()
  }
  if (noApp()) throw new AppError('NOT_FOUND', '没有找到能打开这个文件的程序', 'reason=no_app')
}
/** v0.23.1：按记录 id 打开所在文件夹；只有已成功、输出还在的记录可以 */
export async function RevealRecord(taskId: string): Promise<void> {
  ensure()
  const t = getSimTask(taskId)
  if (!t || t.type !== 'convert') throw notFoundRecord()
  if (t.status !== 'succeeded' || outputGone.has(taskId)) throw notFoundFile()
  console.info('[模拟] 打开所在文件夹', t.outputPath)
}

// ---------------- 探测（MediaService.Probe） ----------------
/** 模拟场景里的源文件给场景的探测结果（hasVideo / hasAudio 真实）；其它路径照常探测（浏览器里是 api/media 的预览假数据） */
export async function probeMock(paths: string[], onBatch?: (r: ProbeResult[]) => void): Promise<ProbeResult[]> {
  ensure()
  const known = new Map<string, goStore.MediaInfo>()
  for (const m of sources.values()) if (m.probe) known.set(m.pathKey, m.probe)
  const rest = paths.filter((p) => !known.has(normalizeSourcePath(p)))
  const done: ProbeResult[] = paths.filter((p) => known.has(normalizeSourcePath(p))).map((p) => ({ path: p, info: goStore.MediaInfo.createFrom(known.get(normalizeSourcePath(p))) }))
  if (done.length) onBatch?.(done)
  const more = rest.length ? await probeFiles(rest, onBatch) : []
  const all = new Map([...done, ...more].map((r) => [r.path, r]))
  return paths.map((p) => all.get(p) ?? { path: p, error: { code: 'INTERNAL', message: '没有拿到这个文件的信息' } })
}

/** 自检用：把某条记录的输出标成“已被移动或删除” */
/** 模拟 RevealInFolder（删除失败的路径）：?cv_revealfail=NOT_FOUND（文件被移走）| INVALID_ARGUMENT（超过 10 分钟 / 重启后） */
export async function revealDeleteFailureMock(path: string): Promise<void> {
  const f = simParam('cv_revealfail')
  if (f === 'NOT_FOUND') throw new AppError('NOT_FOUND', '找不到文件', path)
  if (f === 'INVALID_ARGUMENT') throw new AppError('INVALID_ARGUMENT', '不允许打开这个位置', path)
  console.info('[模拟] 打开所在文件夹', path)
}
export function mockMarkOutputGone(id: string): void {
  outputGone.add(id)
}
/** 自检用：旧输出的位置上换成了别的文件（output_moved） */
export function mockMarkOutputReplaced(id: string): void {
  outputReplaced.add(id)
}
/** 自检用：读当前的模拟存储目录 */
export const mockStorage = (): StorageDirs => ({ ...storage })

// ---------------- v0.24：格式目录、存储目录、重转中断（6.15.2 / 6.16 / v0.24.1） ----------------
export async function GetFormatCatalog(): Promise<FormatEntry[]> {
  ensure()
  return catalog()
}
export async function GetStorageDirs(): Promise<StorageDirs> {
  ensure()
  return { ...storage }
}
/** 6.15.2 第 2 条：两个都先校验，任一失败整体不生效。模拟：?cv_dirfail=output|uploads 让对应目录“无法写入” */
export async function SetStorageDirs(req: StorageDirsUpdate): Promise<StorageDirs> {
  ensure()
  const bad = simParam('cv_dirfail')
  const check = (v: string, what: '保存位置' | '上传位置', kind: string) => {
    if (!v) return
    if (!/^([A-Za-z]:[\\/]|\/)/.test(v)) throw new AppError('INVALID_ARGUMENT', `${what}必须是绝对路径`, v)
    if (bad === kind) throw new AppError('INVALID_ARGUMENT', `${what}无法写入`, v)
  }
  check(req.outputDir, '保存位置', 'output')
  check(req.uploadsDir, '上传位置', 'uploads')
  const norm = (v: string, dft: string) => (v && normalizeSourcePath(v) !== normalizeSourcePath(dft) ? v.replace(/[\\/]+$/, '') : '')
  const out = norm(req.outputDir, storage.defaultOutputDir)
  const up = norm(req.uploadsDir, storage.defaultUploadsDir)
  storage = { ...storage, outputDir: out || storage.defaultOutputDir, uploadsDir: up || storage.defaultUploadsDir, outputCustom: !!out, uploadsCustom: !!up }
  return { ...storage }
}
export async function OpenStorageFolder(kind: 'output' | 'uploads'): Promise<void> {
  ensure()
  if (kind !== 'output' && kind !== 'uploads') throw new AppError('INVALID_ARGUMENT', '目录类型不正确')
  if (simParam('cv_openfail') === '1') throw new AppError('NOT_FOUND', '保存位置不存在，请在设置里重新选择', 'reason=file')
  console.info('[模拟] 打开文件夹', kind === 'output' ? storage.outputDir : storage.uploadsDir)
}
/** v0.24.1：启动时取一次上次退出时被中断的重转条数（取完清零）。?cv_rcint=<n> */
let interruptedTaken = false
export async function TakeInterruptedReconverts(): Promise<number> {
  if (interruptedTaken) return 0
  interruptedTaken = true
  return Math.max(0, Number(simParam('cv_rcint')) || 0)
}
/** 自检用：重置“已取过” */
export function mockResetInterrupted(): void {
  interruptedTaken = false
}

/** 自检用：模拟源文件被移走 */
export function mockMarkSourceGone(sourceId: string): void {
  const m = sources.get(sourceId)
  if (m) m.exists = false
}

/** 模拟缩略图：与设计稿同色系的渐变（launch 紫橙、采访 青绿、录屏 灰蓝、婚礼 粉紫、演示 橙红），其余按文件名取色 */
const GRADS: [string, string, string?][] = [['#1E3A8A', '#7C3AED', '#F97316'], ['#10B981', '#0EA5E9'], ['#64748B', '#1E293B'], ['#EC4899', '#8B5CF6'], ['#F59E0B', '#EF4444']]
export function mockThumbnail(path: string): string {
  const name = baseOf(path)
  const fixed = [/launch/i, /采访/, /屏幕录制/, /婚礼/, /产品演示/].findIndex((re) => re.test(name))
  let h = 0
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  const g = GRADS[fixed >= 0 ? fixed : h % GRADS.length]
  const stops = g.filter(Boolean).map((c, i, a) => `<stop offset="${a.length === 1 ? 0 : i / (a.length - 1)}" stop-color="${c}"/>`).join('')
  const vertical = g.length === 3
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="160" height="90"><defs><linearGradient id="g" x1="0" y1="0" x2="${vertical ? 0 : 1}" y2="1">${stops}</linearGradient></defs><rect width="160" height="90" fill="url(#g)"/></svg>`
  return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg)
}
