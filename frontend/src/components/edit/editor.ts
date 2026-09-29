// 剪辑页状态与动作（模块级单例：离开页面再回来，工程和导出条还在）。
// 只经 src/api（edit / media / system）和 stores/tasks，不引用 wailsjs，不依赖 v1 的 @/api 与 V1_API_READY。
import { computed, reactive, ref, shallowRef } from 'vue'
import {
  EDIT_BACKEND_READY, createPreviewSource, fillOutSec, listProjects, loadProject, newAudioClip, newClipId, newEditProject, newVideoClip, saveProject, toAppError,
  type AudioClip, type EditProject, type EditProjectMeta, type PreviewSource, type VideoClip,
} from '@/api/edit'
import { probeFiles, thumbnailOf } from '@/api/media'
import { pickFiles, revealInFolder } from '@/api/system'
import type { ExportErrorView } from '@/utils/editLogic'
import {
  MAX_AUDIO_TRACKS, MAX_CLIPS, MAX_SOURCES, MAX_TIMELINE_SEC, MAX_VIDEO_TRACKS, MIN_CLIP_SEC, TEXT, clipCountFull, clipEnd, clipLen, clipsOnTrack, deleteNeedsConfirm, exceedsTimeline,
  formatClock, isAudioTrackId, isVideoTrackId, nextTouching, maxTransitionSec, overlapWith, placeProblem, placeProblemText, resolveSplitTarget, round6, snapPoints, snapStart, splitBlockReason,
  splitClip, timelineDuration, trackNo, SNAP_PX, type AnyClip, type PlaceProblem,
} from '@/utils/editLogic'
import { fileBaseName } from '@/utils/format'

export interface SourceItem {
  path: string
  name: string
  state: 'probing' | 'ok' | 'fail'
  duration: number
  hasVideo: boolean
  hasAudio: boolean
  height: number
  channels: number
  thumb: string
  err?: { code: string; message: string; detail?: string }
}

export type DialogState =
  | null
  | { kind: 'removeSource'; path: string; n: number }
  | { kind: 'deleteClips'; ids: string[] }
  | { kind: 'unsaved'; then: () => void }

export interface ToastState {
  text: string
  seq: number
}

/** 预览状态条：导出相关的展示模型（由 EditExportStrip 渲染）。真实数据来自 tasks store，预览钩子可以直接给 forceStrip */
export type StripView =
  | { kind: 'run'; name: string; pct: number; speedText: string; etaText: string }
  | { kind: 'ok'; name: string; meta: string; ignoredTransitions: boolean; outputPath: string }
  | { kind: 'cx' }
  | { kind: 'err'; view: ExportErrorView; taskId: string }

function create() {
  // ───────── 工程 ─────────
  const project = reactive<EditProject>(newEditProject('未命名工程'))
  const sources = ref<SourceItem[]>([])
  const vTracks = ref<string[]>(['V2', 'V1'])
  const aTracks = ref<string[]>(['A1', 'A2'])
  const selectedId = ref<string | null>(null)
  const errorClipId = ref<string | null>(null)
  const playhead = ref(0)
  const pps = ref(16)
  const offSec = ref(0)
  const snapOn = ref(true)
  const dirty = ref(false)
  const dialog = ref<DialogState>(null)
  const toast = ref<ToastState | null>(null)
  const binSel = ref<string | null>(null)
  const projects = ref<EditProjectMeta[]>([])
  const forceStrip = ref<StripView | null>(null)
  const stripDismissed = ref(false)
  const exportTaskId = ref('')
  /** 提交导出时的工程快照：定位片段、重试都以它为准（导出后用户可能继续编辑） */
  const exportSnap = shallowRef<EditProject | null>(null)
  const exportOutName = ref('')
  const exportMeta = ref('')

  const allClips = computed<AnyClip[]>(() => [...project.videoTrack, ...project.audioTrack])
  const total = computed(() => timelineDuration(allClips.value))
  const laneIds = computed(() => [...vTracks.value, ...aTracks.value])
  const selected = computed<AnyClip | null>(() => allClips.value.find((c) => c.id === selectedId.value) ?? null)
  const usedVideoTracks = computed(() => new Set(project.videoTrack.map((c) => c.trackId)).size)
  const usedAudioTracks = computed(() => new Set(project.audioTrack.map((c) => c.trackId)).size)
  const clipsFull = computed(() => clipCountFull(allClips.value.length))
  const sourcesFull = computed(() => sources.value.length >= MAX_SOURCES)
  const hasVideoClips = computed(() => project.videoTrack.length > 0)

  const srcOf = (path: string) => sources.value.find((s) => s.path === path)
  const nameOfClip = (c: AnyClip) => srcOf(c.path)?.name ?? fileBaseName(c.path)
  const durOf = (c: AnyClip) => srcOf(c.path)?.duration || c.outSec
  const clipById = (id: string) => allClips.value.find((c) => c.id === id) ?? null

  /** 素材库 → 时间线的拖动（MediaBin 写，Timeline 读并在松手时处理） */
  const sourceDrag = ref<{ path: string; x: number; y: number } | null>(null)
  const hooks: { dropSource?: () => void; cancelDrag?: () => void } = {}

  // ───────── 提示 ─────────
  let toastTimer: ReturnType<typeof setTimeout> | undefined
  let toastSeq = 0
  function say(text: string) {
    toast.value = { text, seq: ++toastSeq }
    clearTimeout(toastTimer)
    toastTimer = setTimeout(() => (toast.value = null), 3000)
  }
  /** 转场只在首尾相接的片段之间有效，时长不超过较短片段的一半：编辑后静默缩短（不提示），放不下 0.1 秒就取消该转场 */
  function fixTransitions() {
    for (const c of project.videoTrack) {
      if (c.transitionToNext === 'none' || !c.transitionToNext || !c.transitionDurationSec) continue
      const n = nextTouching(allClips.value, c as AnyClip)
      if (!n) continue
      const max = maxTransitionSec(c, n)
      if (c.transitionDurationSec > max) {
        if (max >= 0.1 - 1e-9) c.transitionDurationSec = max
        else {
          c.transitionToNext = 'none'
          c.transitionDurationSec = 0
        }
      }
    }
  }
  const touch = () => {
    fixTransitions()
    dirty.value = true
  }

  // ───────── 素材库 ─────────
  function fillSource(s: SourceItem, r: Awaited<ReturnType<typeof probeFiles>>[number]) {
    if (r.error || !r.info) {
      s.state = 'fail'
      s.err = r.error ?? { code: 'INTERNAL', message: '' }
      return
    }
    const i = r.info
    s.state = 'ok'
    s.err = undefined
    s.duration = i.duration || 0
    s.hasVideo = i.hasVideo !== false && !!i.width
    s.hasAudio = i.hasAudio !== false
    s.height = i.height || 0
    s.channels = i.channels || 0
    if (s.hasVideo) thumbnailOf(s.path, Math.min(1, s.duration / 2), 96).then((t) => (s.thumb = t))
  }
  async function probeInto(items: SourceItem[]) {
    const paths = items.map((i) => i.path)
    await probeFiles(paths, (batch) => {
      for (const r of batch) {
        const s = sources.value.find((x) => x.path === r.path)
        if (s) fillSource(s, r)
      }
    })
  }
  async function addSources(paths: string[]) {
    const fresh = [...new Set(paths)].filter((p) => p && !project.sources.includes(p))
    if (!fresh.length) return
    const room = MAX_SOURCES - project.sources.length
    if (room <= 0) return say(TEXT.sourceLimitToast)
    const take = fresh.slice(0, room)
    if (take.length < fresh.length) say(TEXT.sourceLimitToast)
    const items = take.map<SourceItem>((p) => ({ path: p, name: fileBaseName(p), state: 'probing', duration: 0, hasVideo: false, hasAudio: false, height: 0, channels: 0, thumb: '' }))
    for (const it of items) {
      sources.value.push(reactive(it))
      project.sources.push(it.path)
    }
    touch()
    await probeInto(sources.value.filter((s) => take.includes(s.path)))
  }
  async function importFiles() {
    if (sourcesFull.value) return say(TEXT.sourceLimitToast)
    try {
      const picked = await pickFiles()
      if (picked.length) await addSources(picked)
    } catch (e) {
      say(toAppError(e).message)
    }
  }
  async function retryProbe(path: string) {
    const s = srcOf(path)
    if (!s) return
    s.state = 'probing'
    await probeInto([s])
  }
  function removeSourceRequest(path: string) {
    const n = allClips.value.filter((c) => c.path === path).length
    if (n) dialog.value = { kind: 'removeSource', path, n }
    else removeSource(path, false)
  }
  function removeSource(path: string, withClips: boolean) {
    if (withClips) removeClips(allClips.value.filter((c) => c.path === path).map((c) => c.id))
    sources.value = sources.value.filter((s) => s.path !== path)
    project.sources = project.sources.filter((p) => p !== path)
    if (binSel.value === path) binSel.value = null
    touch()
  }

  // ───────── 轨道 ─────────
  function ensureTrack(id: string) {
    if (isVideoTrackId(id) && !vTracks.value.includes(id)) vTracks.value = [...vTracks.value, id].sort((a, b) => trackNo(b) - trackNo(a))
    if (isAudioTrackId(id) && !aTracks.value.includes(id)) aTracks.value = [...aTracks.value, id].sort((a, b) => trackNo(a) - trackNo(b))
  }
  const canAddTrack = (kind: 'V' | 'A') => (kind === 'V' ? vTracks.value.length < MAX_VIDEO_TRACKS : aTracks.value.length < MAX_AUDIO_TRACKS)
  function addTrack(kind: 'V' | 'A') {
    if (!canAddTrack(kind)) return say(TEXT.trackLimitTip)
    const list = kind === 'V' ? vTracks.value : aTracks.value
    for (let n = 1; n <= 8; n++) {
      const id = `${kind}${n}`
      if (!list.includes(id)) return ensureTrack(id)
    }
  }

  // ───────── 片段 ─────────
  const kindOkFor = (s: SourceItem | undefined, trackId: string) => !!s && (isVideoTrackId(trackId) ? s.hasVideo : s.hasAudio)

  function makeClip(s: SourceItem, trackId: string, startSec: number): AnyClip {
    return isVideoTrackId(trackId)
      ? newVideoClip({ path: s.path, durationSec: s.duration, trackId, startSec })
      : newAudioClip({ path: s.path, durationSec: s.duration, trackId, startSec })
  }
  function pushClip(c: AnyClip) {
    if (isVideoTrackId(c.trackId)) project.videoTrack.push(c as VideoClip)
    else project.audioTrack.push(c as AudioClip)
    ensureTrack(c.trackId)
    touch()
  }
  function removeClips(ids: string[]) {
    project.videoTrack = project.videoTrack.filter((c) => !ids.includes(c.id))
    project.audioTrack = project.audioTrack.filter((c) => !ids.includes(c.id))
    if (selectedId.value && ids.includes(selectedId.value)) selectedId.value = null
    if (errorClipId.value && ids.includes(errorClipId.value)) errorClipId.value = null
    touch()
  }
  /** “+”：追加到 V1（视频）/ A1（音频）最后一个片段之后 */
  function addToTimeline(path: string) {
    const s = srcOf(path)
    if (!s || s.state !== 'ok') return
    if (clipsFull.value) return say(TEXT.clipLimitToast)
    const trackId = s.hasVideo ? 'V1' : 'A1'
    const last = Math.max(0, ...clipsOnTrack(allClips.value, trackId).map(clipEnd))
    let c: AnyClip
    try {
      c = makeClip(s, trackId, round6(last))
    } catch (e) {
      return say(toAppError(e).message)
    }
    const p = placeProblem(allClips.value, c, { adding: true, kindOk: true })
    if (p) return say(placeProblemText(p))
    pushClip(c)
    selectedId.value = c.id
    revealTime(c.startSec)
  }
  /** 素材库拖到时间线某条轨道 */
  function dropSource(path: string, trackId: string, startSec: number): PlaceProblem {
    const s = srcOf(path)
    if (!s || s.state !== 'ok') return 'track'
    let c: AnyClip
    try {
      c = makeClip(s, trackId, Math.max(0, round6(startSec)))
    } catch {
      return 'track'
    }
    const p = placeProblem(allClips.value, c, { adding: true, kindOk: kindOkFor(s, trackId) })
    if (p) return p
    pushClip(c)
    selectedId.value = c.id
    return null
  }
  /** 把 clip 换到新位置 / 新轨道（拖动松手）。返回冲突原因，null = 已移动 */
  function evaluateMove(id: string, trackId: string, startSec: number): PlaceProblem {
    const c = clipById(id)
    if (!c) return 'track'
    const s = srcOf(c.path)
    const cand = { ...c, trackId, startSec: Math.max(0, round6(startSec)) } as AnyClip
    // 视频片段只能在 V 轨，音频片段只能在 A 轨（素材有对应的流也可跨类型，但已放上的 clip 类型固定）
    const kindOk = isVideoTrackId(c.trackId) === isVideoTrackId(trackId) && (!s || s.state !== 'ok' || kindOkFor(s, trackId))
    return placeProblem(allClips.value, cand, { adding: false, kindOk })
  }
  /** 拖入素材时的候选判断（cand 是尚未加入的 clip） */
  function evaluateCandidate(cand: AnyClip): PlaceProblem {
    return placeProblem(allClips.value, cand, { adding: true, kindOk: true })
  }
  function moveClip(id: string, trackId: string, startSec: number) {
    const c = clipById(id)
    if (!c) return
    c.trackId = trackId
    c.startSec = Math.max(0, round6(startSec))
    touch()
  }
  /** 属性区 / 裁剪手柄 / 键盘的统一改法：先在副本上判断，不合法返回原因，不改 */
  function tryUpdate(id: string, patch: Partial<VideoClip & AudioClip>): PlaceProblem {
    const c = clipById(id)
    if (!c) return 'track'
    const cand = { ...c, ...patch } as AnyClip
    if (overlapWith(allClips.value, cand)) return 'overlap'
    if (exceedsTimeline(allClips.value, cand)) return 'timeline'
    Object.assign(c, patch)
    touch()
    return null
  }
  function deleteClips(ids: string[]) {
    if (!ids.length) return
    if (deleteNeedsConfirm(ids.length)) {
      dialog.value = { kind: 'deleteClips', ids }
      return
    }
    removeClips(ids)
    say(`已删除 ${ids.length} 个片段`)
  }
  const deleteSelected = () => selectedId.value && deleteClips([selectedId.value])
  function splitAtPlayhead(): boolean {
    const reason = splitBlockReason(allClips.value, selectedId.value, playhead.value)
    if (reason) {
      say(reason)
      return false
    }
    const target = resolveSplitTarget(allClips.value, selectedId.value, playhead.value)!
    const parts = splitClip(target, playhead.value, [newClipId(target.trackId[0] === 'V' ? 'v' : 'a'), newClipId(target.trackId[0] === 'V' ? 'v' : 'a')])
    if (!parts) return false
    const list = (isVideoTrackId(target.trackId) ? project.videoTrack : project.audioTrack) as AnyClip[]
    const i = list.findIndex((c) => c.id === target.id)
    list.splice(i, 1, parts[0], parts[1])
    selectedId.value = parts[1].id
    touch()
    return true
  }
  const splitReason = computed(() => splitBlockReason(allClips.value, selectedId.value, playhead.value))
  function snapTime(start: number, len: number, excludeId: string | null, altKey = false): { start: number; at: number | null } {
    if (!snapOn.value || altKey) return { start, at: null }
    return snapStart(start, len, snapPoints(allClips.value, excludeId, playhead.value), SNAP_PX / pps.value)
  }

  // ───────── 时间线视图 ─────────
  const laneWidth = ref(720)
  const setPps = (v: number) => (pps.value = Math.min(64, Math.max(2, Math.round(v * 100) / 100)))
  function zoomBy(f: number, anchorSec?: number) {
    const a = anchorSec ?? (playhead.value >= offSec.value && playhead.value <= offSec.value + laneWidth.value / pps.value ? playhead.value : offSec.value)
    const before = (a - offSec.value) * pps.value
    setPps(pps.value * f)
    offSec.value = Math.max(0, a - before / pps.value)
  }
  function fitWindow() {
    const t = Math.max(total.value, 10)
    setPps((laneWidth.value - 24) / t)
    offSec.value = 0
  }
  function revealTime(t: number) {
    const span = laneWidth.value / pps.value
    if (t < offSec.value || t > offSec.value + span * 0.9) offSec.value = Math.max(0, t - span * 0.1)
  }
  function seek(t: number) {
    playhead.value = Math.min(Math.max(0, round6(t)), MAX_TIMELINE_SEC)
  }

  // ───────── 预览 ─────────
  const previewMode = ref<'timeline' | 'source'>('timeline')
  const previewTime = ref(0)
  const playing = ref(false)
  const muted = ref(false)
  const previewError = ref(false)
  const previewUrl = ref('')
  let previewSrc: PreviewSource | null = null
  let previewSrcPath = ''
  const previewSource = computed(() => (previewMode.value === 'source' ? srcOf(binSel.value ?? '') : undefined))
  const previewDuration = computed(() => (previewSource.value ? previewSource.value.duration : total.value))
  const previewCurrent = computed({
    get: () => (previewMode.value === 'source' ? previewTime.value : playhead.value),
    set: (v: number) => {
      if (previewMode.value === 'source') previewTime.value = v
      else seek(v)
    },
  })
  /** 播放头下最上面的视频片段（V 编号大的在上） */
  const clipAtPlayhead = computed<VideoClip | null>(() => {
    const t = playhead.value
    const under = project.videoTrack.filter((c) => t >= c.startSec && t < clipEnd(c))
    return under.sort((a, b) => trackNo(b.trackId) - trackNo(a.trackId))[0] ?? null
  })
  const previewPath = computed(() => (previewMode.value === 'source' ? previewSource.value?.path : selected.value?.path ?? clipAtPlayhead.value?.path) ?? '')
  async function loadPreview(force = false) {
    const path = previewPath.value
    if (!path) {
      previewError.value = false
      return
    }
    if (!force && path === previewSrcPath && !previewError.value) return
    previewSrcPath = path
    previewSrc = createPreviewSource(path)
    try {
      previewUrl.value = (await previewSrc.load()).url
      previewError.value = false
    } catch {
      previewUrl.value = ''
      previewError.value = true
    }
  }
  /** <video> 的 error 事件：404 → 重取地址（createPreviewSource 内最多 2 次）；其余（解码失败等）→ 预览失败态 */
  async function onMediaError() {
    try {
      const u = await previewSrc?.onMediaError()
      if (u) {
        previewUrl.value = u.url
        return
      }
    } catch {
      /* 落到失败态 */
    }
    previewError.value = true
  }
  async function reloadPreview() {
    await loadPreview(true)
  }
  async function revealPreviewFile() {
    try {
      await revealInFolder(previewPath.value)
    } catch (e) {
      say(toAppError(e).message)
    }
  }
  let raf = 0
  let last = 0
  function tick(now: number) {
    const dt = (now - last) / 1000
    last = now
    if (previewMode.value === 'source') {
      previewTime.value = Math.min(previewDuration.value, previewTime.value + dt)
      if (previewTime.value >= previewDuration.value) return stop()
    } else {
      seek(playhead.value + dt)
      revealTime(playhead.value)
      if (playhead.value >= total.value) return stop()
    }
    raf = requestAnimationFrame(tick)
  }
  function play() {
    if (previewError.value || previewDuration.value <= 0) return
    if (previewCurrent.value >= previewDuration.value) previewCurrent.value = 0
    playing.value = true
    last = performance.now()
    raf = requestAnimationFrame(tick)
  }
  function stop() {
    playing.value = false
    cancelAnimationFrame(raf)
  }
  const togglePlay = () => (playing.value ? stop() : play())
  function selectClip(id: string | null) {
    selectedId.value = id
    if (id) {
      const c = clipById(id)
      previewMode.value = 'timeline'
      if (c) seek(c.startSec)
    }
  }
  function previewSourceRow(path: string) {
    stop()
    binSel.value = path
    previewMode.value = 'source'
    previewTime.value = 0
  }
  function stepFrame(dir: -1 | 1, big = false) {
    stop()
    previewCurrent.value = Math.max(0, previewCurrent.value + dir * (big ? 1 : 1 / 30))
  }

  // ───────── 保存 / 打开 ─────────
  const saving = ref(false)
  function currentProject(): EditProject {
    return JSON.parse(JSON.stringify(project)) as EditProject
  }
  async function save(): Promise<boolean> {
    if (saving.value) return false
    if (!dirty.value && project.id) return true
    saving.value = true
    try {
      const meta = await saveProject(currentProject())
      project.id = meta.id
      dirty.value = false
      return true
    } catch (e) {
      say(`保存失败：${toAppError(e).message}`)
      return false
    } finally {
      saving.value = false
    }
  }
  async function refreshProjects() {
    projects.value = await listProjects(50)
  }
  async function openProject(id: string): Promise<string> {
    // 返回 '' 成功，否则错误文案
    try {
      const { project: p, missingPaths } = await loadProject(id)
      stop()
      Object.assign(project, { ...p, videoTrack: p.videoTrack.map((c) => fillOutSecSafe(c)), audioTrack: p.audioTrack.map((c) => fillOutSecSafe(c)) })
      vTracks.value = ['V2', 'V1']
      aTracks.value = ['A1', 'A2']
      for (const c of allClips.value) ensureTrack(c.trackId)
      sources.value = [...new Set([...p.sources, ...allClips.value.map((c) => c.path)])].map((path) =>
        reactive<SourceItem>({ path, name: fileBaseName(path), state: 'probing', duration: 0, hasVideo: false, hasAudio: false, height: 0, channels: 0, thumb: '' }),
      )
      project.sources = sources.value.map((s) => s.path)
      selectedId.value = null
      errorClipId.value = null
      playhead.value = 0
      offSec.value = 0
      binSel.value = null
      previewMode.value = 'timeline'
      dirty.value = false
      const items = sources.value.slice()
      const miss = new Set(missingPaths)
      for (const s of items) {
        if (miss.has(s.path)) {
          s.state = 'fail'
          s.err = { code: 'NOT_FOUND', message: '' }
        }
      }
      probeInto(items.filter((s) => !miss.has(s.path)))
      return ''
    } catch (e) {
      const err = toAppError(e)
      return err.code === 'UNSUPPORTED' ? '这个工程来自更新的版本，请升级 FFmpegFree 后再打开' : err.message
    }
  }
  function fillOutSecSafe<T extends AnyClip>(c: T): T {
    try {
      return fillOutSec(c, c.inSec + 1)
    } catch {
      return c
    }
  }
  function newProject() {
    stop()
    Object.assign(project, newEditProject('未命名工程'), { sources: [], videoTrack: [], audioTrack: [] })
    sources.value = []
    vTracks.value = ['V2', 'V1']
    aTracks.value = ['A1', 'A2']
    selectedId.value = null
    dirty.value = false
  }

  const exporting = computed(() => !!exportTaskId.value && stripBusy.value)
  const stripBusy = ref(false)

  return {
    EDIT_BACKEND_READY, MAX_CLIPS, MIN_CLIP_SEC, project, sources, vTracks, aTracks, selectedId, errorClipId, playhead, pps, offSec, snapOn, dirty, dialog, toast, binSel, projects,
    forceStrip, stripDismissed, exportTaskId, exportSnap, exportOutName, exportMeta, stripBusy, exporting,
    allClips, total, laneIds, selected, usedVideoTracks, usedAudioTracks, clipsFull, sourcesFull, hasVideoClips, srcOf, nameOfClip, durOf, clipById,
    say, touch, addSources, importFiles, retryProbe, removeSourceRequest, removeSource, ensureTrack, canAddTrack, addTrack, kindOkFor, addToTimeline, dropSource, evaluateMove, evaluateCandidate, moveClip,
    tryUpdate, deleteClips, removeClips, deleteSelected, splitAtPlayhead, splitReason, snapTime, laneWidth, setPps, zoomBy, fitWindow, revealTime, seek, nextTouching: (c: AnyClip) => nextTouching(allClips.value, c),
    maxTransitionFor: (c: AnyClip) => {
      const n = nextTouching(allClips.value, c)
      return n ? maxTransitionSec(c, n) : 0
    },
    sourceDrag, hooks, previewMode, previewTime, playing, muted, previewError, previewUrl, previewSource, previewDuration, previewCurrent, clipAtPlayhead, previewPath, loadPreview, onMediaError, reloadPreview,
    revealPreviewFile, play, stop, togglePlay, selectClip, previewSourceRow, stepFrame, saving, currentProject, save, refreshProjects, openProject, newProject,
    clipLen, formatClock,
  }
}

export type Editor = ReturnType<typeof create>
let inst: Editor | null = null
export function useEditor(): Editor {
  return (inst ??= create())
}
