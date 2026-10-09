/**
 * 语音工具接口层（首期只做转字幕 speech_to_subtitle）。
 * LANG_BACKEND_READY=false：本地模拟（组件未发布 missing/canDownload=false；假 cues 演示时间轴）。
 * 真绑定合入后在此接 LangService，页面 / store 不用大改。
 */
import { LANG_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend } from '@/services/wails'
import { CUE_MAX_CHARS } from '@/utils/langText'

export { LANG_BACKEND_READY }

export type AsrTier = 'standard' | 'hd'
export type LangComponentState =
  | 'checking'
  | 'ready'
  | 'missing'
  | 'outdated'
  | 'downloading'
  | 'preparing'
  | 'failed'

export interface LangAsrStatus {
  state: LangComponentState
  version: string
  source: string
  tier: AsrTier
  canDownload: boolean
  downloadBytes: number
  installBytes: number
  phase?: string
  receivedBytes?: number
  error?: { code: string; message: string; detail?: string } | null
}

export interface SubtitleCue {
  id: string
  text: string
  startMs: number
  endMs: number
}

export type AsrTierSetting = AsrTier

const TIER_KEY = 'ff-asr-tier'
const live = () => LANG_BACKEND_READY && hasWailsBackend()
export const isLangSim = (): boolean => !live()

/** 未发布：标准约 400MB / 高清约 1.2GB（引导数字；canDownload=false 时不真正下载） */
const BYTES_STANDARD = 400 * 1024 * 1024
const BYTES_HD = Math.round(1.2 * 1024 * 1024 * 1024)

function readTier(): AsrTier {
  try {
    const v = localStorage.getItem(TIER_KEY)
    if (v === 'hd' || v === 'standard') return v
  } catch {
    /* ignore */
  }
  return 'standard'
}

export async function getAsrTier(): Promise<AsrTier> {
  if (live()) {
    // 真绑定后读 Settings.asrTier
    return readTier()
  }
  return readTier()
}

/** 切换档位只改引导体积，不自动下载 */
export async function setAsrTier(tier: AsrTier): Promise<void> {
  if (tier !== 'standard' && tier !== 'hd') return
  try {
    localStorage.setItem(TIER_KEY, tier)
  } catch {
    /* ignore */
  }
}

export async function getLangAsrStatus(): Promise<LangAsrStatus> {
  const tier = await getAsrTier()
  if (live()) {
    // 真绑定后调 GetLangAsrStatus
    return unpublishedStatus(tier)
  }
  return unpublishedStatus(tier)
}

function unpublishedStatus(tier: AsrTier): LangAsrStatus {
  return {
    state: 'missing',
    version: '',
    source: '',
    tier,
    canDownload: false,
    downloadBytes: tier === 'hd' ? BYTES_HD : BYTES_STANDARD,
    installBytes: 0,
    error: null,
  }
}

/** 演示用假字幕（组件未发布时走这条） */
export const MOCK_CUES: SubtitleCue[] = [
  { id: 'c1', text: '欢迎收看本期产品介绍。', startMs: 1200, endMs: 4800 },
  { id: 'c2', text: '今天我们来看语音工具里的转字幕。', startMs: 5100, endMs: 9400 },
  { id: 'c3', text: '生成后可以在时间轴上改字和起止时间。', startMs: 9900, endMs: 14200 },
  { id: 'c4', text: '改完再导出成字幕文件。', startMs: 14600, endMs: 18000 },
]

export type MockAsrOutcome = 'ok' | 'empty' | 'fail'

/** 按文件名启发式决定模拟结果（演示用） */
export function mockAsrOutcome(path: string): MockAsrOutcome {
  const name = path.replace(/\\/g, '/').split('/').pop() ?? path
  if (/静音|silent|empty/i.test(name)) return 'empty'
  if (/损坏|fail|broken/i.test(name)) return 'fail'
  return 'ok'
}

export function validateCues(cues: SubtitleCue[]): { ok: true } | { ok: false; reasons: string[] } {
  const reasons: string[] = []
  const sorted = [...cues].sort((a, b) => a.startMs - b.startMs)
  for (const c of cues) {
    if (!(c.endMs > c.startMs)) reasons.push('end')
    const n = [...c.text].length
    if (n > CUE_MAX_CHARS) reasons.push('chars')
  }
  for (let i = 0; i < sorted.length; i++) {
    for (let j = i + 1; j < sorted.length; j++) {
      const a = sorted[i]
      const b = sorted[j]
      if (!(a.endMs <= b.startMs || b.endMs <= a.startMs)) reasons.push('overlap')
    }
  }
  const uniq = [...new Set(reasons)]
  return uniq.length ? { ok: false, reasons: uniq } : { ok: true }
}

export function formatCueTime(ms: number): string {
  const n = Math.max(0, Math.round(ms))
  const h = Math.floor(n / 3600000)
  const m = Math.floor((n % 3600000) / 60000)
  const s = Math.floor((n % 60000) / 1000)
  const frac = n % 1000
  const pad = (x: number, w = 2) => String(x).padStart(w, '0')
  return `${pad(h)}:${pad(m)}:${pad(s)},${pad(frac, 3)}`
}

/** 解析 SRT 风格时间；失败返回 null */
export function parseCueTime(raw: string): number | null {
  const t = raw.trim().replace('.', ',')
  const m = /^(\d{1,2}):(\d{2}):(\d{2}),(\d{1,3})$/.exec(t)
  if (!m) return null
  const h = Number(m[1])
  const min = Number(m[2])
  const sec = Number(m[3])
  let ms = Number(m[4].padEnd(3, '0').slice(0, 3))
  if (![h, min, sec, ms].every((x) => Number.isFinite(x)) || min > 59 || sec > 59) return null
  return h * 3600000 + min * 60000 + sec * 1000 + ms
}

export function cuesToSrt(cues: SubtitleCue[]): string {
  return cues
    .map((c, i) => `${i + 1}\n${formatCueTime(c.startMs)} --> ${formatCueTime(c.endMs)}\n${c.text.trim()}\n`)
    .join('\n')
}

export function cuesToVtt(cues: SubtitleCue[]): string {
  const toVtt = (ms: number) => formatCueTime(ms).replace(',', '.')
  return (
    'WEBVTT\n\n' +
    cues.map((c, i) => `${i + 1}\n${toVtt(c.startMs)} --> ${toVtt(c.endMs)}\n${c.text.trim()}\n`).join('\n')
  )
}

/** 浏览器模拟导出：触发下载；Wails 真机接 ExportSubtitleCues */
export async function exportSubtitleCues(
  cues: SubtitleCue[],
  format: 'srt' | 'vtt',
  baseName: string,
): Promise<{ fileName: string }> {
  const v = validateCues(cues)
  if (!v.ok) {
    const err = new Error('INVALID_ARGUMENT') as Error & { code: string }
    err.code = 'INVALID_ARGUMENT'
    throw err
  }
  const body = format === 'vtt' ? cuesToVtt(cues) : cuesToSrt(cues)
  const fileName = `${baseName || '字幕'}.${format}`
  if (live()) {
    // 真绑定后调 ExportSubtitleCues
    return { fileName }
  }
  if (typeof document !== 'undefined') {
    const blob = new Blob([body], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = fileName
    a.click()
    URL.revokeObjectURL(url)
  }
  return { fileName }
}
