// 浏览器预览专用（!hasWailsBackend()）：?edit=<状态> 直接摆出设计稿里的各个状态，用来截图和自查。真实运行（Wails 里）不读取，也不会打包进任何数据流。
// 状态：empty | media | probefail | timeline | export | exporting | done | canceled | fail | previewfail | limit
// 变体：kind=noaudio|name（export）、kind=disk|short（fail）、kind=time（limit）、warn=1（done）、trans=over（timeline）
import { newAudioClip, newVideoClip, type AudioClip, type VideoClip } from '@/api/edit'
import { exportErrorView } from '@/utils/editLogic'
import { useEditor, type SourceItem } from './editor'
import { useExportFlow } from './exportFlow'

const DIR = '/Users/me/Movies/旅行素材'
const M: { n: string; d: number; h: number; a?: boolean }[] = [
  { n: '日落_航拍.mp4', d: 84, h: 1080 },
  { n: '湖边漫步.mov', d: 48, h: 1080 },
  { n: '夜市街拍.mp4', d: 130, h: 2160 },
  { n: '片头_logo.mov', d: 32, h: 720 },
  { n: '背景音乐.mp3', d: 185, h: 0, a: true },
]
const src = (m: (typeof M)[number]): SourceItem => ({ path: `${DIR}/${m.n}`, name: m.n, state: 'ok', duration: m.d, hasVideo: !m.a, hasAudio: true, height: m.h, channels: 2, thumb: '' })
const P = (n: string) => `${DIR}/${n}`

export async function applyPreviewState(state: string, q: URLSearchParams) {
  const ed = useEditor()
  const fl = useExportFlow()
  const kind = q.get('kind') || ''
  ed.project.name = '旅行短片'
  if (state === 'empty') return
  ed.sources.value = M.map((m) => ({ ...src(m) }))
  ed.project.sources = ed.sources.value.map((s) => s.path)
  ed.dirty.value = true
  if (state === 'probefail') {
    ed.sources.value.unshift(
      { path: P('旧素材_2023.mov'), name: '旧素材_2023.mov', state: 'fail', duration: 0, hasVideo: false, hasAudio: false, height: 0, channels: 0, thumb: '', err: { code: 'NOT_FOUND', message: '' } },
      { path: P('损坏的采访录音.mp4'), name: '损坏的采访录音.mp4', state: 'fail', duration: 0, hasVideo: false, hasAudio: false, height: 0, channels: 0, thumb: '', err: { code: 'PROBE_FAILED', message: '' } },
    )
    ed.project.sources = ed.sources.value.map((s) => s.path)
  }
  if (state === 'media') {
    ed.binSel.value = P('日落_航拍.mp4')
    ed.previewMode.value = 'source'
  }
  if (['empty', 'media', 'probefail'].includes(state)) return

  const v = (id: string, n: string, start: number, len: number, track = 'V1', inSec = 0, speed = 1): VideoClip => ({
    ...newVideoClip({ id, path: P(n), durationSec: M.find((m) => m.n === n)!.d, trackId: track, startSec: start }), inSec, outSec: inSec + len * speed, speed,
  })
  const trans = q.get('trans') === 'over'
  const vids: VideoClip[] = [
    v('v1', '片头_logo.mov', 0, 8),
    { ...v('v2', '日落_航拍.mp4', 8, 18, 'V1', 10, 1.5), transitionToNext: 'fade', transitionDurationSec: 0.5 },
    v('v3', '湖边漫步.mov', 26, trans ? 1.6 : 16),
    v('v4', '夜市街拍.mp4', trans ? 27.6 : 42, 10),
    v('v5', '夜市街拍.mp4', 30, 12, 'V2', 20),
  ]
  const aud: AudioClip[] = [{ ...newAudioClip({ id: 'a1', path: P('背景音乐.mp3'), durationSec: 185, trackId: 'A1', startSec: 0 }), outSec: 52 }]
  ed.project.videoTrack = vids
  ed.project.audioTrack = kind === 'noaudio' ? [] : aud
  ed.playhead.value = 12.267
  ed.selectedId.value = 'v2'
  if (trans) ed.selectedId.value = 'v2'
  ed.ensureTrack('V2')
  ed.setPps(16)

  if (state === 'previewfail') {
    ed.previewError.value = true
    ed.previewLocked.value = true
  }
  if (state === 'limit') {
    if (kind === 'time') {
      ed.project.videoTrack = [
        { ...v('t1', '日落_航拍.mp4', 21510, 70), outSec: 70 },
        { ...v('t2', '湖边漫步.mov', 21580, 20), outSec: 20 },
      ]
      ed.project.audioTrack = [{ ...aud[0], startSec: 21480, outSec: 120 }]
      ed.setPps(6)
      ed.offSec.value = 21480
      ed.playhead.value = 21590
      ed.selectedId.value = 't2'
      ed.say('时间线最长 6 小时，放不下这个片段')
    } else {
      const extra: VideoClip[] = []
      for (let i = 0; i < 94; i++) extra.push(v(`x${i}`, '片头_logo.mov', 60 + i * 2, 1.5, `V${3 + (i % 6)}`))
      ed.project.videoTrack = [...vids, ...extra]
      for (let i = 3; i <= 8; i++) ed.ensureTrack(`V${i}`)
      ed.say('片段数量已达上限 100 个，不能再添加')
    }
  }
  if (state === 'export') {
    if (kind === 'name') fl.form.name = '我的旅行短片'.repeat(17).slice(0, 101)
    await fl.openDialog()
    if (kind === 'name') fl.form.name = '我的旅行短片'.repeat(17).slice(0, 101)
    if (q.get('warn') !== '0' && kind !== 'noaudio' && kind !== 'name') fl.warnings.value = [{ code: 'OUT_TRUNCATED', clipId: 'v2', message: '' }]
  }
  if (state === 'exporting') fl.forced.value = { kind: 'run', name: '旅行短片.mp4', pct: 42, speedText: '2.3x', etaText: '剩余约 00:31' }
  if (state === 'done') fl.forced.value = { kind: 'ok', name: '旅行短片 (1).mp4', meta: '1920×1080 · 00:52 · 38.6 MB', ignoredTransitions: q.get('warn') === '1', outputPath: '/Users/me/Movies/FFmpegFree/旅行短片 (1).mp4' }
  if (state === 'canceled') fl.forced.value = { kind: 'cx' }
  if (state === 'fail') {
    const p = ed.project
    const short = kind === 'short'
    const view = exportErrorView(
      kind === 'disk'
        ? { code: 'CONVERT_DISK_FULL', message: '', detail: '' }
        : short
          ? { code: 'INVALID_ARGUMENT', message: '片段太短（按速度折算后不足 0.04 秒）', detail: 'clip=v3 path=' + P('湖边漫步.mov') }
          : { code: 'INVALID_ARGUMENT', message: '入点超出素材时长。', detail: 'clip=v3 path=' + P('湖边漫步.mov') },
      p,
    )
    fl.forced.value = { kind: 'err', view, taskId: '' }
    if (view.clipId) ed.errorClipId.value = view.clipId
    ed.selectedId.value = null
  }
}
