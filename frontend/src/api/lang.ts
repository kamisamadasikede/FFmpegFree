/**
 * 语音工具接口层（首期只做转字幕 speech_to_subtitle，契约 v0.29.1 §6.18）。
 * LANG_BACKEND_READY=true 且有 Wails：调 LangService / Settings.asrTier。
 * 纯浏览器（无 window.go）仍走模拟；?voice=demo 可演示时间轴。
 * 未发布（missing + canDownload=false）：界面不出现下载/安装按钮。
 */
import * as LangBinding from '../../wailsjs/go/app/LangService'
import * as SystemBinding from '../../wailsjs/go/app/SystemService'
import { lang, langasr, system } from '../../wailsjs/go/models'
import { AppError, call, toAppError } from '@/api/call'
import { LANG_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend, onTaskEvent, previewParams } from '@/services/wails'
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

export interface LangAsrProgress {
  phase: string
  receivedBytes: number
  totalBytes: number
  progress?: number
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

/** 浏览器走查：?voice=demo（仅无 Wails 时生效） */
export const isVoiceDemo = (): boolean => isLangSim() && previewParams.get('voice') === 'demo'

/** 未发布引导体积：标准约 400MB / 高清约 1.2GB（canDownload=false 时不真正下载） */
const BYTES_STANDARD = 400 * 1024 * 1024
const BYTES_HD = Math.round(1.2 * 1024 * 1024 * 1024)

function readTierLocal(): AsrTier {
  try {
    const v = localStorage.getItem(TIER_KEY)
    if (v === 'hd' || v === 'standard') return v
  } catch {
    /* ignore */
  }
  return 'standard'
}

function writeTierLocal(tier: AsrTier) {
  try {
    localStorage.setItem(TIER_KEY, tier)
  } catch {
    /* ignore */
  }
}

function normTier(v: unknown): AsrTier {
  return v === 'hd' ? 'hd' : 'standard'
}

function mapStatus(raw: langasr.Status | LangAsrStatus | null | undefined, fallbackTier?: AsrTier): LangAsrStatus {
  const tier = normTier(raw?.tier || fallbackTier)
  if (!raw) return unpublishedStatus(tier)
  const err = raw.error
  return {
    state: (raw.state as LangComponentState) || 'missing',
    version: raw.version ?? '',
    source: raw.source ?? '',
    tier,
    canDownload: raw.canDownload === true,
    downloadBytes: typeof raw.downloadBytes === 'number' ? raw.downloadBytes : tier === 'hd' ? BYTES_HD : BYTES_STANDARD,
    installBytes: typeof raw.installBytes === 'number' ? raw.installBytes : 0,
    ...(raw.phase ? { phase: raw.phase } : {}),
    ...(typeof raw.receivedBytes === 'number' ? { receivedBytes: raw.receivedBytes } : {}),
    error: err && typeof (err as { code?: string }).code === 'string'
      ? {
          code: (err as { code: string }).code,
          message: (err as { message?: string }).message ?? '',
          detail: (err as { detail?: string }).detail,
        }
      : null,
  }
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

export async function getAsrTier(): Promise<AsrTier> {
  if (live()) {
    const s = await call(SystemBinding.GetSettings())
    return normTier(s?.asrTier)
  }
  return readTierLocal()
}

/** 切换档位只改引导体积，不自动下载 */
export async function setAsrTier(tier: AsrTier): Promise<void> {
  if (tier !== 'standard' && tier !== 'hd') return
  if (live()) {
    const s = await call(SystemBinding.GetSettings())
    await call(SystemBinding.UpdateSettings(system.Settings.createFrom({ ...s, asrTier: tier })))
    return
  }
  writeTierLocal(tier)
}

export async function getLangAsrStatus(): Promise<LangAsrStatus> {
  const tier = await getAsrTier()
  if (live()) {
    const st = await call(LangBinding.GetLangAsrStatus())
    return mapStatus(st, tier)
  }
  return unpublishedStatus(tier)
}

export async function recheckLangAsr(): Promise<LangAsrStatus> {
  if (live()) {
    const st = await call(LangBinding.RecheckLangAsr())
    return mapStatus(st)
  }
  return getLangAsrStatus()
}

/**
 * 安装/下载语音识别组件。仅 canDownload=true 时由 UI 调用。
 * 未配置 URL 时后端返回 LANG_DOWNLOAD_FAILED「暂时无法下载…」——UI 不应走到这里。
 */
export async function installLangAsr(tier?: AsrTier): Promise<LangAsrStatus> {
  if (live()) {
    const st = await call(LangBinding.InstallLangAsr(tier ?? ''))
    return mapStatus(st)
  }
  return getLangAsrStatus()
}

export async function cancelLangAsrInstall(): Promise<void> {
  if (live()) await call(LangBinding.CancelLangAsrInstall())
}

export function watchLangAsr(cb: (s: LangAsrStatus) => void): () => void {
  return onTaskEvent<langasr.Status | LangAsrStatus>('lang:asr', (raw) => cb(mapStatus(raw)))
}

export function watchLangAsrProgress(cb: (p: LangAsrProgress) => void): () => void {
  return onTaskEvent<LangAsrProgress>('lang:asr-progress', cb)
}

/** 提交转字幕；返回新建任务列表（通常 1 条） */
export async function submitSpeechToSubtitle(req: {
  paths: string[]
  language?: string
  format: 'srt' | 'vtt'
  outputDir?: string
}): Promise<Array<{ id: string; status: string; progress: number; result?: { cues?: SubtitleCue[] }; error?: { code: string; message: string } | null }>> {
  if (!live()) throw new AppError('UNSUPPORTED', '模拟层请走本地假识别')
  const tasks = await call(
    LangBinding.SubmitSpeechToSubtitle(
      lang.SpeechToSubtitleRequest.createFrom({
        paths: req.paths,
        language: req.language || 'auto',
        format: req.format,
        ...(req.outputDir ? { outputDir: req.outputDir } : {}),
      }),
    ),
  )
  return (tasks ?? []).map((t) => ({
    id: t.id,
    status: t.status,
    progress: typeof t.progress === 'number' ? t.progress : 0,
    result: t.result
      ? {
          cues: Array.isArray(t.result.cues)
            ? t.result.cues.map((c) => ({
                id: c.id,
                text: c.text,
                startMs: Number(c.startMs) || 0,
                endMs: Number(c.endMs) || 0,
              }))
            : undefined,
        }
      : undefined,
    error: t.error ? { code: t.error.code, message: t.error.message ?? '' } : null,
  }))
}

/** 演示用假字幕（仅模拟 / ?voice=demo） */
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

/**
 * 导出字幕。真机：ExportSubtitleCues（可空 targetPath → 写到实际输出目录）。
 * 模拟：浏览器触发下载。
 */
export async function exportSubtitleCues(
  cues: SubtitleCue[],
  format: 'srt' | 'vtt',
  baseName: string,
  opts?: { taskId?: string; targetPath?: string },
): Promise<{ fileName: string; path?: string }> {
  const v = validateCues(cues)
  if (!v.ok) {
    const err = new AppError('INVALID_ARGUMENT', '还不能导出')
    throw err
  }
  const fileName = `${baseName || '字幕'}.${format}`
  if (live()) {
    const res = await call(
      LangBinding.ExportSubtitleCues(
        lang.ExportSubtitleRequest.createFrom({
          taskId: opts?.taskId ?? '',
          cues: cues.map((c) => langasr.SubtitleCue.createFrom(c)),
          format,
          ...(opts?.targetPath ? { targetPath: opts.targetPath } : {}),
        }),
      ),
    )
    const path = res?.path ?? ''
    const name = path ? path.replace(/\\/g, '/').split('/').pop() || fileName : fileName
    return { fileName: name, path: path || undefined }
  }
  const body = format === 'vtt' ? cuesToVtt(cues) : cuesToSrt(cues)
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

export { toAppError }
