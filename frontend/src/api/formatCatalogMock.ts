/**
 * 格式目录的模拟数据（契约 v0.24 §6.16.2，顺序、displayName、aliases 照抄；GIF 另加别名“图片”，架构师 10-08）。
 * 34 种输出格式、38 个内置预设。encodable 在真实后端按转换组件检测；模拟里 AMR、APE 不可输出（和项目默认安装的转换组件一致），
 * ?cv_fmtoff=<扩展名,…> 可以再置灰别的格式，?cv_fmtoff=all 模拟转换组件没就绪。
 */
import type { FormatEntry, FormatPreset, RecordOptions } from '@/api/convertRecords'
import { simParam } from '@/api/sim'

const opt = (o: Partial<RecordOptions>): RecordOptions => ({ container: '', videoCodec: '', audioCodec: '', width: 0, height: 0, fps: 0, videoBitrate: 0, audioBitrate: 0, crf: 0, targetSizeMb: 0, trimStart: 0, trimEnd: 0, ...o })
type P = [id: string, name: string, options: Partial<RecordOptions>]
type Row = [category: FormatEntry['category'], ext: string, displayName: string, aliases: string[], presets: P[]]
const V = (vc: string, ac: string, more: Partial<RecordOptions> = {}): Partial<RecordOptions> => ({ videoCodec: vc, audioCodec: ac, ...more })
const A = (ac: string, br = 0): Partial<RecordOptions> => ({ audioCodec: ac, ...(br ? { audioBitrate: br } : {}) })

const ROWS: Row[] = [
  ['video', 'mp4', 'MP4', ['通用视频', '手机视频', 'MPEG-4'], [
    ['builtin-mp4-h264', 'MP4（H.264 + AAC，通用）', V('h264', 'aac')],
    ['builtin-mp4-h264-1080p', 'MP4 1080p（H.264 + AAC）', V('h264', 'aac', { width: 1920 })],
    ['builtin-mp4-h264-720p', 'MP4 720p（H.264 + AAC）', V('h264', 'aac', { width: 1280 })],
    ['builtin-mp4-h265', 'MP4（H.265 + AAC，体积更小）', V('h265', 'aac')],
  ]],
  ['video', 'mkv', 'MKV', ['高清', '蓝光', 'Matroska'], [['builtin-mkv-h264', 'MKV（H.264 + AAC）', V('h264', 'aac')], ['builtin-mkv-copy', 'MKV（不重新编码，只换封装）', V('copy', 'copy')]]],
  ['video', 'mov', 'MOV', ['苹果', '苹果视频', 'QuickTime'], [['builtin-mov-h264', 'MOV（H.264 + AAC）', V('h264', 'aac')]]],
  ['video', 'webm', 'WebM', ['网页视频'], [['builtin-webm-vp9', 'WebM（VP9 + Opus）', V('vp9', 'opus')]]],
  ['video', 'avi', 'AVI', ['老式视频', 'Xvid', 'DivX'], [['builtin-avi', 'AVI（Xvid + MP3）', V('mpeg4', 'mp3')]]],
  ['video', 'flv', 'FLV', ['Flash 视频', '网络视频'], [['builtin-flv', 'FLV（H.264 + AAC）', V('h264', 'aac')]]],
  ['video', 'gif', 'GIF', ['动图', '表情包', '动画', '图片'], [['builtin-gif', 'GIF 动图（宽 480，12 帧/秒）', { width: 480, fps: 12 }]]],
  ['video', 'wmv', 'WMV', ['Windows 视频', '微软视频'], [['builtin-wmv', 'WMV（WMV2 + WMA）', V('wmv2', 'wma', { audioBitrate: 192000 })]]],
  ['video', 'mpg', 'MPG', ['MPEG', 'MPEG-2', 'VCD'], [['builtin-mpg', 'MPG（MPEG-2 + MP2）', V('mpeg2', 'mp2', { audioBitrate: 224000 })]]],
  ['video', 'vob', 'VOB', ['DVD'], [['builtin-vob', 'VOB（MPEG-2 + AC-3）', V('mpeg2', 'ac3', { audioBitrate: 192000 })]]],
  ['video', '3gp', '3GP', ['老手机', '手机视频'], [['builtin-3gp', '3GP（H.264 + AAC）', V('h264', 'aac', { audioBitrate: 96000 })]]],
  ['video', 'swf', 'SWF', ['Flash', 'Flash 动画'], [['builtin-swf', 'SWF（Flash 视频）', V('flv1', 'mp3')]]],
  ['video', 'ogv', 'OGV', ['Ogg 视频', 'Theora'], [['builtin-ogv', 'OGV（Theora + Vorbis）', V('theora', 'vorbis')]]],
  ['audio', 'mp3', 'MP3', ['音乐', '歌曲', '通用音频'], [['builtin-mp3', 'MP3（192 kbps）', A('mp3', 192000)]]],
  ['audio', 'm4a', 'M4A', ['苹果', '苹果音频', 'AAC'], [['builtin-m4a', 'M4A（AAC 192 kbps）', A('aac', 192000)]]],
  ['audio', 'aac', 'AAC', ['高级音频编码'], [['builtin-aac', 'AAC（192 kbps）', A('aac', 192000)]]],
  ['audio', 'wav', 'WAV', ['无损', '波形', 'CD 音质'], [['builtin-wav', 'WAV（无损 PCM）', A('pcm')]]],
  ['audio', 'flac', 'FLAC', ['无损', '无损压缩'], [['builtin-flac', 'FLAC（无损压缩）', A('flac')]]],
  ['audio', 'ogg', 'OGG', ['Vorbis', 'Ogg 音频'], [['builtin-ogg', 'OGG（Vorbis）', A('vorbis')]]],
  ['audio', 'opus', 'Opus', ['语音', '网络音频'], [['builtin-opus', 'Opus（128 kbps）', A('opus', 128000)]]],
  ['audio', 'wma', 'WMA', ['Windows 音频', '微软音频'], [['builtin-wma', 'WMA（192 kbps）', A('wma', 192000)]]],
  ['audio', 'amr', 'AMR', ['录音', '手机录音', '语音'], [['builtin-amr', 'AMR（语音 12.2 kbps）', A('amr_nb', 12200)]]],
  ['audio', 'm4r', 'M4R', ['苹果铃声', '铃声', 'iPhone 铃声'], [['builtin-m4r', 'M4R 铃声（AAC 192 kbps）', A('aac', 192000)]]],
  ['audio', 'mp2', 'MP2', ['MPEG 音频', '广播'], [['builtin-mp2', 'MP2（224 kbps）', A('mp2', 224000)]]],
  ['audio', 'ape', 'APE', ['无损', "Monkey's Audio"], [['builtin-ape', 'APE（无损）', A('ape')]]],
  ['audio', 'wv', 'WV', ['无损', 'WavPack'], [['builtin-wv', 'WV（WavPack 无损）', A('wavpack')]]],
  ['audio', 'mmf', 'MMF', ['手机铃声', '彩铃'], [['builtin-mmf', 'MMF（手机铃声）', A('adpcm_yamaha')]]],
  ['image', 'jpg', 'JPG', ['照片', '图片', 'JPEG'], [['builtin-jpg', 'JPG 图片', {}]]],
  ['image', 'png', 'PNG', ['透明图片', '截图', '图片'], [['builtin-png', 'PNG 图片', {}]]],
  ['image', 'webp', 'WebP', ['网页图片', '图片'], [['builtin-webp', 'WebP 图片', {}]]],
  ['image', 'ico', 'ICO', ['图标'], [['builtin-ico', 'ICO 图标（最大 256×256）', {}]]],
  ['image', 'bmp', 'BMP', ['位图', '图片'], [['builtin-bmp', 'BMP 图片', {}]]],
  ['image', 'tif', 'TIF', ['TIFF', '印刷', '图片'], [['builtin-tif', 'TIF 图片', {}]]],
  ['image', 'tga', 'TGA', ['Targa', '游戏贴图'], [['builtin-tga', 'TGA 图片', {}]]],
]
/** 模拟转换组件缺的格式（项目默认安装的版本没有 libopencore_amrnb，也没有 APE 编码器） */
const MOCK_OFF: Record<string, string> = { amr: 'missing_encoder', ape: 'missing_encoder' }
export const FORMAT_OFF_REASON = '当前转换组件不支持输出这个格式'

/** 自定义 summary 函数由调用方给（与 mockParamsSummary 同规则），避免循环依赖 */
export function mockFormatCatalog(summary: (o: Partial<RecordOptions>) => string, userPresets: FormatPreset[] = []): FormatEntry[] {
  const offParam = (simParam('cv_fmtoff') ?? '').split(',').filter(Boolean)
  const allOff = offParam.includes('all')
  return ROWS.map(([category, extension, displayName, aliases, ps]) => {
    const presets: FormatPreset[] = ps.map(([id, name, o]) => {
      const options = opt({ container: extension, ...o })
      return { id, name, builtIn: true, paramsSummary: summary(options), options }
    })
    presets.push(...userPresets.filter((p) => p.options.container === extension).map((p) => ({ ...p, options: { ...p.options } })))
    const code = allOff ? 'converter_not_ready' : offParam.includes(extension) ? 'missing_muxer' : MOCK_OFF[extension]
    return {
      category, extension, displayName, aliases: [...aliases], encodable: !code, ...(code ? { reason: FORMAT_OFF_REASON, reasonCode: code } : {}),
      defaultPresetId: presets[0].id, presets,
    }
  })
}
/** 所有内置预设（38 个，按目录顺序） */
export const mockBuiltinPresetIds = (): string[] => ROWS.flatMap((r) => r[4].map((p) => p[0]))
