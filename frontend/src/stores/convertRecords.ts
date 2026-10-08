/**
 * 转换页 v2（转换记录）的 store：源文件行（convert_sources）+ 每行内嵌的转换记录（convert 任务），按契约 v0.23 §6.14 的接口取数。
 * 分页单位是源文件行（ListSources limit / offset，每行内嵌最新 20 条；更多记录用 ListSourceRecords）；搜索走 SearchSources。
 * 记录的实时状态来自任务 store（task:* 事件，不轮询），这里只保存加载时的样子。
 * 设计：转换页-v2-设计说明-v0.1.md §三 / §四 / §7.3；产品决定 v1（共享任务记录、原地重试、勾选在转换后清空、记录不自动清理）。
 */
import { defineStore } from 'pinia'
import { computed, reactive, ref, watch } from 'vue'
import { toAppError } from '@/api/call'
import { listPresets, MAX_SUBMIT, type PresetItem } from '@/api/convert'
import {
  addSources, checkPaths, checkSources, convertV2IsReal, deleteRecords, deleteSource, getRecordThumbnail, getSourceThumbnail, listSourceRecords, listSources,
  MORE_RECORDS, previewOutputName, probeSources, reconvert as apiReconvert, RECORD_LIMIT, recordOf, revealRecord, getSource,
  revealSource as apiRevealSource, searchSources, SOURCE_PAGE, submitSources,
  type ConvertRecord, type ConvertSource, type ConvertSourceEntry, type ConvertSourceStatus, type DeleteResult, type RecordOptions, type ThumbState, type V023Task,
} from '@/api/convertRecords'
import { mockSceneUi } from '@/api/convertRecordsMock'
import { canPickFiles, getDefaultOutputDir, pickDirectory, pickFiles } from '@/api/system'
import { hasWailsBackend } from '@/services/wails'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { useTaskStore, type TaskError, type TaskItem, type TaskStatus } from '@/stores/tasks'
import { conflictReason, dupPresetTitles, isAudioContainer, isAudioOnly, isToday, presetShortTitle, setPresetCatalog, splitPresetName, totalProgress } from '@/utils/convertText'
import { normalizeSourcePath } from '@/utils/sourcePath'
import type { store as goStore } from '../../wailsjs/go/models'

export type ProbeState = 'pending' | 'probing' | 'ok' | 'error'
export type RecordFilter = 'all' | 'active' | 'failed'
/** 源文件行删除（定稿 10-08）：按钮提示 / 菜单项“从列表移除”，弹窗标题如下；源文件本身永远不删 */
export const SOURCE_REMOVE_LABEL = '从列表移除'
export const SOURCE_REMOVE_TITLE = '从列表移除这个文件和它的全部记录'

/** 父行（源文件） */
export interface SourceRow {
  sourceId: string
  path: string
  name: string
  addedAt: number
  lastActivityAt: number
  /** 持久化的媒体信息（可能没有）；G3 起 hasVideo / hasAudio 是真实值，显示走 metaInfoOf（旧数据两个都是 false 时按不知道处理） */
  media?: goStore.MediaInfo
  /** 本次会话里 MediaService.Probe 的结果：冲突预检只认它 */
  info?: goStore.MediaInfo
  probe: ProbeState
  probeError?: TaskError
  /** false = 原位置找不到；undefined = 没检查过（按存在处理） */
  exists?: boolean
  thumb: ThumbState | null
  thumbAsked: boolean
  /** 重复添加时闪一下：时间戳，页面据此加高亮动画 */
  flashAt: number
  /** 后端给的记录总数（本地提交 / 删除时同步加减） */
  recordCount: number
  /** 本行已加载的记录 id（含本地新提交的） */
  loadedIds: string[]
  loadingMore: boolean
}

/** 子记录 = 记录 + 任务 store 里的实时状态 */
export interface KidView extends ConvertRecord {
  outputGone: boolean
}

export interface ParentView {
  src: SourceRow
  kids: KidView[]
  /** 这一行还有多少条更早的记录没加载（“显示更早的 N 条记录”） */
  moreCount: number
  lastActivityAt: number
  open: boolean
  running: number
  queued: number
  failed: number
  /** 摘要里的迷你进度条：进行中的第一项的进度（0~100） */
  runPct: number
  conflict: string | null
  selected: boolean
  checkable: boolean
  /** 搜索命中的记录 id（高亮） */
  hits: Set<string> | null
}

export interface GroupView {
  key: 'today' | 'earlier' | 'pinned'
  label: string
  parents: ParentView[]
}

/** 删除确认弹窗的数据 */
export interface DeleteAsk {
  kind: 'record' | 'source'
  id: string
  title: string
  name: string
  count: number
  activeCount: number
  outputs: number
  outputBytes: number
  /** 源文件是纯音频（弹窗文件行用音符图标） */
  audio?: boolean
  /** 只有 kind=record：一次删多条（DeleteRecords 的 taskIds）；缺省 = [id] */
  ids?: string[]
}

/** 本地记住的折叠状态（按 sourceId）：用户点过的才记，没点过的按默认规则 */
export const FOLD_KEY = 'ffmpegfree.convert.fold.v1'
const FOLD_CAP = 1000
function loadFold(): Record<string, boolean> {
  try {
    const v = JSON.parse(globalThis.localStorage?.getItem(FOLD_KEY) ?? '{}')
    return v && typeof v === 'object' ? v : {}
  } catch {
    return {}
  }
}
function saveFold(m: Record<string, boolean>) {
  try {
    const keys = Object.keys(m)
    if (keys.length > FOLD_CAP) for (const k of keys.slice(0, keys.length - FOLD_CAP)) delete m[k]
    globalThis.localStorage?.setItem(FOLD_KEY, JSON.stringify(m))
  } catch {
    /* 存不下就算了：只是记住折叠状态 */
  }
}

/** 默认展开：有进行中 / 排队 / 失败的记录，或今天有活动；更早的折叠（设计 §3.3） */
export function defaultOpen(kids: readonly { status: string }[], lastActivityAt: number, now = Date.now()): boolean {
  if (kids.some((k) => k.status === 'running' || k.status === 'queued' || k.status === 'failed' || k.status === 'interrupted')) return true
  return isToday(lastActivityAt, now)
}

/**
 * 显示用的媒体信息：当次探测优先，其次 media 缓存。
 * 后端走查修复（G3，PR #92）起 ConvertSource.media 整份入库，hasVideo / hasAudio 是真实值：保留它们，
 * 重启后音频行照样显示采样率 · 声道，无声视频照样显示“没有声音”。
 * 两个都不是 true 的缓存（真实媒体至少有一路，只可能是 G3 之前没补上的旧数据）按“不知道”处理：去掉这两个字段，按宽高判断，不显示“没有声音”。
 */
export function metaInfoOf(s: { info?: goStore.MediaInfo; media?: goStore.MediaInfo }): (Omit<goStore.MediaInfo, 'hasVideo' | 'hasAudio'> & { hasVideo?: boolean; hasAudio?: boolean }) | undefined {
  if (s.info) return s.info
  if (!s.media) return undefined
  if (s.media.hasVideo === true || s.media.hasAudio === true) return s.media
  const { hasVideo: _v, hasAudio: _a, ...rest } = s.media as goStore.MediaInfo & Record<string, unknown>
  void _v
  void _a
  return rest as Omit<goStore.MediaInfo, 'hasVideo' | 'hasAudio'>
}

const ACTIVE: TaskStatus[] = ['queued', 'running']
const FAILED: TaskStatus[] = ['failed', 'interrupted']
const TERMINAL: TaskStatus[] = ['succeeded', 'failed', 'canceled', 'interrupted']
const MOCK_NAMES = ['航拍-西湖日落.mp4', '会议录像-周例会.mkv', '访谈录音_第三期.wav', '产品宣传片-竖版.mov', 'vlog-第7期.mp4']
const errOf = (e: unknown): TaskError => {
  const a = toAppError(e)
  return { code: a.code, message: a.message, ...(a.detail ? { detail: a.detail } : {}) }
}

export const useConvertRecordsStore = defineStore('convertRecords', () => {
  const tasks = useTaskStore()
  const ffmpeg = useFFmpegStore()

  const sources = reactive<Record<string, SourceRow>>({})
  const records = reactive<Record<string, ConvertRecord>>({})
  const outputGone = reactive(new Set<string>())
  const selected = reactive(new Set<string>())
  const foldUser = reactive<Record<string, boolean>>(loadFold())
  /** 本次会话里的展开（提交 / 重试后展开对应父行；模拟场景的初始折叠）——不写本地 */
  const foldSession = reactive<Record<string, boolean>>({})

  const loaded = ref(false)
  const loading = ref(false)
  const loadError = ref<TaskError | null>(null)
  /** ListSources 的分页：已取到第几行、总行数 */
  const listOffset = ref(0)
  const listTotal = ref(0)
  const loadingMore = ref(false)

  const filter = ref<RecordFilter>('all')
  const keyword = ref('')
  /** 搜索结果（null = 不在搜索）：行 id 的顺序 + 命中信息 */
  const searchHits = ref<{ status: ConvertSourceStatus; order: string[]; name: Set<string>; tasks: Map<string, Set<string>>; offset: number; total: number } | null>(null)
  const searching = ref(false)
  /**
   * 全部 / 进行中 / 失败（v0.23.1 ListSources 的 status）：选了进行中 / 失败时按后端返回的行显示（行 id 顺序 + 分页），null = 全部。
   * 行里的记录不按状态过滤（与后端一致：内嵌记录和 recordCount 不过滤，要找失败的那条，展开这一行）。
   * 有搜索关键字时不用这里：SearchSources 带同样的 status（v0.23.2），结果在 searchHits。
   */
  const filterHits = ref<{ status: Exclude<RecordFilter, 'all'>; order: string[]; offset: number; total: number } | null>(null)
  const filtering = ref(false)

  const presets = ref<PresetItem[]>([])
  const presetsLoaded = ref(false)
  const presetsError = ref<TaskError | null>(null)
  const selectedPresetId = ref('')
  const tab = ref<'video' | 'audio'>('video')
  const outputOverride = ref('')
  const defaultOutputDir = ref('')
  const submitting = ref(false)
  const submitError = ref<TaskError | null>(null)
  const notice = ref('')
  const toast = ref<{ text: string; at: number } | null>(null)
  const outputName = ref('')
  /** 最近一次添加文件的时间：页面据此滚回顶部 */
  const addedTick = ref(0)
  /** 任务中心“在转换页查看”：不在已加载页里的行用 GetSource 取来，临时钉在最上面（重新加载 / 翻到它所在的页后取消） */
  const pinned = ref('')
  /** 要定位并高亮的记录（页面滚到它、闪一下） */
  const focus = ref<{ id: string; sourceId: string; at: number } | null>(null)

  function say(text: string) {
    toast.value = { text, at: Date.now() }
  }

  // ---------------- 预设 ----------------
  const selectedPreset = computed(() => presets.value.find((p) => p.id === selectedPresetId.value))
  const isAudioPreset = (p: PresetItem) => isAudioContainer(p.options.container)
  /** 预设卡标题：标题重名（MP4 H.264 / H.265）时加编码简称「MP4 · H.265」；和记录第 2 行共用 presetShortTitle（走查 G2） */
  const presetTitles = computed(() => {
    const dup = dupPresetTitles(presets.value.map((p) => p.name))
    return new Map(presets.value.map((p) => [p.id, presetShortTitle(p.name, p.options.videoCodec || p.options.audioCodec, dup)]))
  })
  const presetTitle = (p: PresetItem) => presetTitles.value.get(p.id) ?? splitPresetName(p.name).title
  const shownPresets = computed(() => presets.value.filter((p) => (tab.value === 'audio') === isAudioPreset(p)))
  function setTab(t: 'video' | 'audio') {
    tab.value = t
    const list = presets.value.filter((p) => (t === 'audio') === isAudioPreset(p))
    if (list.length && !list.some((p) => p.id === selectedPresetId.value)) selectedPresetId.value = list[0].id
  }
  async function loadPresets() {
    try {
      presets.value = await listPresets()
      setPresetCatalog(presets.value.map((p) => p.name))
      presetsError.value = null
      if (!selectedPreset.value && presets.value.length) selectedPresetId.value = presets.value[0].id
      if (selectedPreset.value) tab.value = isAudioPreset(selectedPreset.value) ? 'audio' : 'video'
    } catch (e) {
      presetsError.value = errOf(e)
    } finally {
      presetsLoaded.value = true
    }
  }
  async function loadDefaultDir() {
    try {
      defaultOutputDir.value = await getDefaultOutputDir()
    } catch (e) {
      console.warn('read default output dir failed', e)
    }
  }

  // ---------------- 源文件 / 记录 ----------------
  function upsertSource(s: ConvertSource, recordCount?: number): SourceRow {
    const cur = sources[s.sourceId]
    if (cur) {
      cur.lastActivityAt = Math.max(cur.lastActivityAt, s.lastActivityAt)
      if (s.media) cur.media = s.media
      if (recordCount !== undefined) cur.recordCount = recordCount
      return cur
    }
    sources[s.sourceId] = {
      sourceId: s.sourceId, path: s.path, name: s.name, addedAt: s.addedAt, lastActivityAt: s.lastActivityAt, ...(s.media ? { media: s.media } : {}),
      probe: 'pending', thumb: null, thumbAsked: false, flashAt: 0, recordCount: recordCount ?? 0, loadedIds: [], loadingMore: false,
    }
    return sources[s.sourceId]
  }
  function putRecord(t: V023Task | TaskItem) {
    const r = recordOf(t as V023Task)
    if (!r.sourceId) return
    const cur = records[r.id]
    if (cur && cur.version > r.version) return
    records[r.id] = r
    const row = sources[r.sourceId]
    if (row && !row.loadedIds.includes(r.id)) row.loadedIds.push(r.id)
  }
  /** 写入一页源文件行；返回其中本地原来没有的行数（刷新第一页时据此推进 listOffset） */
  function putEntries(items: ConvertSourceEntry[]): number {
    let added = 0
    for (const e of items) {
      if (!sources[e.source.sourceId]) added++
      upsertSource(e.source, e.recordCount)
      for (const t of e.records) putRecord(t)
    }
    return added
  }
  /**
   * 本地新出现的一条记录（提交返回、task:status 先于提交返回到达、旧后端重试生成新任务）：放进记录表，
   * 并且只在第一次见到这个任务 id 时给所在行的 recordCount +1（走查 S1：监听和 afterSubmit 各加一次，记录数翻倍）。
   * 栏头总数、行摘要、“展开更多”、移除确认框都从 recordCount 来，所以这里是唯一计数的地方。返回是否新增。
   */
  function addRecord(t: V023Task | TaskItem): boolean {
    if (records[t.id]) {
      putRecord(t)
      return false
    }
    putRecord(t)
    const r = records[t.id]
    if (!r) return false
    const s = sources[r.sourceId]
    if (s) s.recordCount++
    return true
  }

  // ---------------- 实时状态 ----------------
  /** 记录 + 任务 store 里的实时状态（任务 store 有这个 id 时以它为准） */
  function liveOf(r: ConvertRecord): KidView {
    const t = tasks.taskById(r.id)
    const gone = outputGone.has(r.id)
    if (!t) return { ...r, outputGone: gone }
    const status = t.status
    return {
      ...r,
      status,
      progress: status === 'succeeded' ? 1 : t.progress,
      speed: t.speed ?? '',
      etaSec: t.etaSec ?? 0,
      error: t.error ?? null,
      outputPath: t.outputPath || r.outputPath,
      startedAt: t.startedAt || r.startedAt,
      finishedAt: t.finishedAt || r.finishedAt,
      encoder: t.encoder,
      encoderDevice: t.encoderDevice,
      hwFallback: t.hwFallback,
      hwFallbackReason: t.hwFallbackReason,
      result: status === 'succeeded' ? t.result ?? r.result : undefined,
      outputGone: status === 'succeeded' ? gone : false,
    }
  }
  const liveById = computed(() => {
    const m = new Map<string, KidView>()
    for (const r of Object.values(records)) m.set(r.id, liveOf(r))
    return m
  })

  // ---------------- 勾选 / 冲突 ----------------
  const isCheckable = (s: SourceRow) => s.exists !== false && s.probe !== 'error'
  /** 冲突只看当次探测（info）：勾选时会重新探测，文件可能已经变了，缓存只用来显示 */
  function conflictOfSource(s: SourceRow): string | null {
    return conflictReason(s.info, s.probe === 'ok', selectedPreset.value?.options.container)
  }
  function toggle(sourceId: string) {
    const s = sources[sourceId]
    if (!s) return
    if (selected.has(sourceId)) selected.delete(sourceId)
    else if (isCheckable(s)) {
      selected.add(sourceId)
      void probePending()
    }
  }
  function clearSelection() {
    selected.clear()
  }
  const selectedRows = computed(() => [...selected].map((id) => sources[id]).filter((s): s is SourceRow => !!s && isCheckable(s)))
  const submittableRows = computed(() => selectedRows.value.filter((s) => s.probe === 'ok' && !conflictOfSource(s)))
  const blockedCount = computed(() => selectedRows.value.filter((s) => !!conflictOfSource(s)).length)
  const probingSelected = computed(() => selectedRows.value.filter((s) => s.probe === 'pending' || s.probe === 'probing').length)

  // ---------------- 折叠 ----------------
  function isOpen(sourceId: string, kids: readonly KidView[], lastActivityAt: number): boolean {
    if (sourceId in foldSession) return foldSession[sourceId]
    if (sourceId in foldUser) return foldUser[sourceId]
    return defaultOpen(kids, lastActivityAt)
  }
  function setOpen(sourceId: string, open: boolean) {
    delete foldSession[sourceId]
    delete foldUser[sourceId] // 重新插入，刷新它在淘汰顺序里的位置
    foldUser[sourceId] = open
    saveFold(foldUser)
  }

  // ---------------- 列表视图 ----------------
  const kidsBySource = computed(() => {
    const m = new Map<string, KidView[]>()
    for (const k of liveById.value.values()) {
      const l = m.get(k.sourceId)
      if (l) l.push(k)
      else m.set(k.sourceId, [k])
    }
    for (const l of m.values()) l.sort((a, b) => b.createdAt - a.createdAt || (a.id < b.id ? 1 : -1))
    return m
  })
  const parents = computed<ParentView[]>(() => {
    const out: ParentView[] = []
    const sh = searchHits.value
    const fh = filterHits.value
    const order = sh ? sh.order : fh ? fh.order : filter.value !== 'all' ? [] : null // 筛选还在取第一页：先不显示行
    const rows = order ? order.map((id) => sources[id]).filter((s): s is SourceRow => !!s) : Object.values(sources)
    for (const src of rows) {
      const all = kidsBySource.value.get(src.sourceId) ?? []
      let kids = all
      let hits: Set<string> | null = null
      if (sh && !sh.name.has(src.sourceId)) {
        // 只有输出文件名命中：只显示命中的记录（设计 §四 11）
        hits = sh.tasks.get(src.sourceId) ?? new Set()
        kids = all.filter((k) => hits!.has(k.id))
      } else if (sh) hits = sh.tasks.get(src.sourceId) ?? null
      const lastActivityAt = src.lastActivityAt
      const running = kids.filter((k) => k.status === 'running')
      const filtered = !!sh // 状态筛选不过滤行里的记录，只有搜索会
      out.push({
        src,
        kids,
        moreCount: filtered ? 0 : Math.max(0, src.recordCount - all.length),
        lastActivityAt,
        open: kids.length > 0 && (filtered || isOpen(src.sourceId, kids, lastActivityAt)),
        running: running.length,
        queued: kids.filter((k) => k.status === 'queued').length,
        failed: kids.filter((k) => FAILED.includes(k.status)).length,
        runPct: running.length ? Math.round(running[0].progress * 100) : 0,
        conflict: selected.has(src.sourceId) ? conflictOfSource(src) : null,
        selected: selected.has(src.sourceId),
        checkable: isCheckable(src),
        hits,
      })
    }
    const pin = pinned.value
    return out.sort((a, b) => (b.src.sourceId === pin ? 1 : 0) - (a.src.sourceId === pin ? 1 : 0) || b.lastActivityAt - a.lastActivityAt || (a.src.sourceId < b.src.sourceId ? 1 : -1))
  })
  const groups = computed<GroupView[]>(() => {
    const today: ParentView[] = []
    const earlier: ParentView[] = []
    const pin: ParentView[] = []
    for (const p of parents.value) (p.src.sourceId === pinned.value && !isToday(p.lastActivityAt) ? pin : isToday(p.lastActivityAt) ? today : earlier).push(p)
    const g: GroupView[] = []
    if (pin.length) g.push({ key: 'pinned', label: '更早', parents: pin }) // 定位来的行临时放在最上面
    if (today.length) g.push({ key: 'today', label: '今天', parents: today })
    if (earlier.length) g.push({ key: 'earlier', label: '更早', parents: earlier })
    return g
  })
  /** 计数“5 个文件 · 8 条记录”：后端总行数和本地已知的取大；记录数按各行的 recordCount 加总（未加载的行不知道，所以是已加载部分） */
  const sourceCount = computed(() => Math.max(listTotal.value, Object.keys(sources).length))
  const recordCount = computed(() => Object.values(sources).reduce((n, s) => n + s.recordCount, 0))
  const hasMore = computed(() => {
    const p = searchHits.value ?? filterHits.value
    return p ? p.offset < p.total : listOffset.value < listTotal.value
  })

  // ---------------- 总进度 / 本轮完成 ----------------
  /** 总进度覆盖所有进行中的转换（含没加载到的行），所以直接用任务 store 的活动列表 */
  const activeConvert = computed(() => tasks.active.filter((t) => t.type === 'convert'))
  const total = computed(() => totalProgress(activeConvert.value))
  const round = reactive(new Set<string>())
  const roundBanner = ref<{ ok: number; fail: number } | null>(null)
  let bannerTimer: ReturnType<typeof setTimeout> | undefined
  function closeBanner() {
    roundBanner.value = null
    clearTimeout(bannerTimer)
  }
  function showBanner(ok: number, fail: number) {
    roundBanner.value = { ok, fail }
    clearTimeout(bannerTimer)
    bannerTimer = setTimeout(() => (roundBanner.value = null), 5000) // 5 秒后自动收起
  }
  watch(
    () => activeConvert.value.map((t) => t.id).join(','),
    (ids) => {
      for (const id of ids ? ids.split(',') : []) round.add(id)
      if (!round.size || ids) return
      let ok = 0
      let fail = 0
      for (const id of round) {
        const t = tasks.taskById(id)
        if (t?.status === 'succeeded') ok++
        else if (t && FAILED.includes(t.status)) fail++
      }
      round.clear()
      if (ok || fail) showBanner(ok, fail)
    },
  )

  // ---------------- 加载 ----------------
  let inited = false
  async function init() {
    if (inited) return
    inited = true
    await tasks.init()
    await Promise.all([loadPresets(), loadDefaultDir(), reload()])
    if (!convertV2IsReal()) applyMockUi()
  }
  function applyMockUi() {
    const ui = mockSceneUi()
    for (const id of ui.selected) if (sources[id] && isCheckable(sources[id])) selected.add(id)
    for (const id of ui.closed) foldSession[id] = false
    for (const id of ui.open) foldSession[id] = true
    if (ui.presetId && presets.value.some((p) => p.id === ui.presetId)) {
      selectedPresetId.value = ui.presetId
      tab.value = isAudioPreset(selectedPreset.value!) ? 'audio' : 'video'
    }
    if (ui.roundDone.length) showBanner(ui.roundDone.length, 0)
    void probePending()
  }
  async function reload() {
    loading.value = true
    try {
      const page = await listSources({ limit: SOURCE_PAGE, offset: 0, recordLimit: RECORD_LIMIT })
      putEntries(page.items)
      pinned.value = ''
      listOffset.value = page.items.length
      listTotal.value = page.total
      loadError.value = null
      loaded.value = true
      void checkExistence(page.items)
      void probePending()
      if (filterHits.value) void loadFiltered(filterHits.value.status)
    } catch (e) {
      loadError.value = errOf(e)
    } finally {
      loading.value = false
    }
  }
  /** 打开页面 / 加载下一页时检查一次：输出文件 / 源文件还在不在（设计 §四 12：只检查一次，预览和打开文件夹前再查一次） */
  async function checkExistence(items: ConvertSourceEntry[]) {
    try {
      const done = items.flatMap((e) => e.records).filter((t) => t.status === 'succeeded').map((t) => t.id)
      const ids = items.map((e) => e.source.sourceId)
      const [paths, srcs] = await Promise.all([done.length ? checkPaths(done) : Promise.resolve([]), ids.length ? checkSources(ids) : Promise.resolve([])])
      for (const c of paths) if (c.found && !c.outputExists) outputGone.add(c.taskId)
      for (const c of srcs) {
        const s = sources[c.sourceId]
        if (!s || !c.found) continue
        s.exists = c.exists
        if (!c.exists) selected.delete(c.sourceId)
      }
    } catch (e) {
      console.warn('check paths failed', e) // 查不了就按都在处理，打开文件夹前还会再查
    }
  }
  /** “加载更早的记录”：普通列表取下一页源文件行；搜索时取搜索的下一页 */
  async function loadMore() {
    if (!hasMore.value || loadingMore.value) return
    loadingMore.value = true
    try {
      const sh = searchHits.value
      if (sh) {
        const p = await searchSources({ keyword: keyword.value.trim(), limit: SOURCE_PAGE, offset: sh.offset, recordLimit: RECORD_LIMIT, status: sh.status })
        if (searchHits.value !== sh) return
        applySearch(p.items, sh)
        sh.offset += p.items.length
        sh.total = p.total
      } else if (filterHits.value) {
        const fh = filterHits.value
        const p = await listSources({ limit: SOURCE_PAGE, offset: fh.offset, recordLimit: RECORD_LIMIT, status: fh.status })
        if (filterHits.value !== fh) return
        putEntries(p.items)
        for (const e of p.items) {
          if (fh.order.includes(e.source.sourceId)) continue
          fh.order.push(e.source.sourceId)
          if (fh.status === 'failed') foldSession[e.source.sourceId] = true
        }
        fh.offset += p.items.length
        fh.total = p.total
        void checkExistence(p.items)
      } else {
        const p = await listSources({ limit: SOURCE_PAGE, offset: listOffset.value, recordLimit: RECORD_LIMIT })
        putEntries(p.items)
        if (p.items.some((e) => e.source.sourceId === pinned.value)) pinned.value = '' // 翻到了它本来的位置
        listOffset.value += p.items.length
        listTotal.value = p.total
        void checkExistence(p.items)
      }
      void probePending()
    } catch (e) {
      say(errOf(e).message)
    } finally {
      loadingMore.value = false
    }
  }
  /** 一行里“显示更早的 N 条记录”（ListSourceRecords，每次 50 条） */
  async function loadMoreRecords(sourceId: string) {
    const s = sources[sourceId]
    if (!s || s.loadingMore) return
    s.loadingMore = true
    try {
      const have = (kidsBySource.value.get(sourceId) ?? []).length
      const p = await listSourceRecords(sourceId, MORE_RECORDS, have)
      for (const t of p.items) putRecord(t)
      s.recordCount = p.total
      const done = p.items.filter((t) => t.status === 'succeeded').map((t) => t.id)
      if (done.length) for (const c of await checkPaths(done).catch(() => [])) if (c.found && !c.outputExists) outputGone.add(c.taskId)
    } catch (e) {
      say(errOf(e).message)
    } finally {
      s.loadingMore = false
    }
  }
  /**
   * 任务中心“在转换页查看”：展开这条记录所在的源文件行、滚到它、高亮（设计 §7.3 第 11 条）。
   * 流程（设计 §7.3 第 11 条，UI 10-08）：清掉搜索 / 筛选 → GetSource(sourceId)（v0.23.1）取这一行临时置顶 → 记录不在内嵌的最新 20 条里时
   * 用 ListSourceRecords 往下取到它为止 → 展开、滚到顶部、高亮 2 秒后 0.4 秒淡出（页面负责滚动和动画）。
   */
  async function locate(taskId: string, sourceId?: string) {
    if (!loaded.value) await reload()
    if (keyword.value || searchHits.value) await search('')
    clearFilter()
    const sid = sourceId || records[taskId]?.sourceId || (tasks.taskById(taskId) as { sourceId?: string } | undefined)?.sourceId || ''
    if (!sid) return say('没有找到这条转换记录')
    // 不管这一行在不在已加载的页里，都用 GetSource 取最新的一份，临时放到最上面（不改 lastActivityAt，刷新列表后回到原位置）
    try {
      const e = await getSource(sid)
      putEntries([e])
      void checkExistence([e])
      pinned.value = sid
    } catch (e) {
      const err = errOf(e)
      return say(err.code === 'NOT_FOUND' ? '这条转换记录已被删除' : err.message)
    }
    for (let i = 0; i < 20 && !records[taskId] && sources[sid] && sources[sid].loadedIds.length < sources[sid].recordCount; i++) {
      const before = sources[sid].loadedIds.length
      await loadMoreRecords(sid)
      if (!sources[sid] || sources[sid].loadedIds.length === before) break
    }
    if (!records[taskId]) return say('这条转换记录已被删除')
    foldSession[sid] = true
    focus.value = { id: taskId, sourceId: sid, at: Date.now() }
  }
  function clearFilter() {
    filter.value = 'all'
    filterHits.value = null
    filterSeq++
    filtering.value = false
  }
  let filterSeq = 0
  /** 进行中 / 失败：ListSources(status)（v0.23.1）取第一页；limit 用于刷新时保留已翻过的行数 */
  async function loadFiltered(status: Exclude<RecordFilter, 'all'>, limit = SOURCE_PAGE, keep = false) {
    const seq = ++filterSeq
    filtering.value = true
    try {
      const p = await listSources({ limit: Math.min(200, Math.max(SOURCE_PAGE, limit)), offset: 0, recordLimit: RECORD_LIMIT, status })
      if (seq !== filterSeq) return
      putEntries(p.items)
      const order = p.items.map((e) => e.source.sourceId)
      const known = new Set(filterHits.value?.status === status ? filterHits.value.order : [])
      if (status === 'failed') for (const id of order) if (!known.has(id)) foldSession[id] = true // 筛选“失败”时行默认展开（§四 11）；刷新时不改用户已收起的行
      // 刷新（keep）：已显示但不再符合的行先留着（进行中的刚完成、失败的刚重试），重新点筛选或重新加载后才去掉，避免行在眼前消失
      const prev = keep && filterHits.value?.status === status ? filterHits.value.order.filter((id) => !order.includes(id) && sources[id]) : []
      filterHits.value = { status, order: [...order, ...prev], offset: p.items.length, total: p.total }
      void checkExistence(p.items)
      void probePending()
    } catch (e) {
      if (seq !== filterSeq) return
      say(errOf(e).message)
      if (!filterHits.value) filter.value = 'all' // 第一次就没取到：回到全部
    } finally {
      if (seq === filterSeq) filtering.value = false
    }
  }
  const statusOf = (f: RecordFilter): ConvertSourceStatus => (f === 'all' ? '' : f)
  /**
   * 全部 / 进行中 / 失败（ListSources / SearchSources 的 status，v0.23.1 / v0.23.2）。
   * 有搜索关键字时带着 status 重新搜索；没有关键字时按 status 列行。
   */
  async function setFilter(f: RecordFilter) {
    const kw = keyword.value.trim()
    if (f === filter.value && (kw ? !!searchHits.value : f === 'all' || !!filterHits.value)) return
    filter.value = f
    filterHits.value = null // 切换时先清掉上一种筛选的行，避免闪出不符合的行
    filterSeq++
    filtering.value = false
    if (kw) return search(keyword.value)
    if (f !== 'all') await loadFiltered(f)
  }
  function applySearch(items: ConvertSourceEntry[], sh: NonNullable<typeof searchHits.value>) {
    putEntries(items)
    for (const e of items) {
      if (!sh.order.includes(e.source.sourceId)) {
        sh.order.push(e.source.sourceId)
        if (sh.status === 'failed') foldSession[e.source.sourceId] = true // 筛选“失败”时行默认展开
      }
      if (e.nameMatched) sh.name.add(e.source.sourceId)
      if (e.matchedTaskIds?.length) {
        sh.tasks.set(e.source.sourceId, new Set(e.matchedTaskIds))
        foldSession[e.source.sourceId] = true
      }
    }
  }
  let searchSeq = 0
  /** 搜索全部记录（SearchSources，分页）；空关键字退出搜索。防抖在页面里做（300ms） */
  async function search(k: string) {
    keyword.value = k
    const kw = k.trim()
    const seq = ++searchSeq
    if (!kw) {
      searchHits.value = null
      searching.value = false
      if (filter.value !== 'all' && !filterHits.value) void loadFiltered(filter.value) // 退出搜索：回到按状态列行
      return
    }
    const status = statusOf(filter.value) // v0.23.2：搜索也带 status
    searching.value = true
    try {
      const p = await searchSources({ keyword: kw.slice(0, 100), limit: SOURCE_PAGE, offset: 0, recordLimit: RECORD_LIMIT, status })
      if (seq !== searchSeq) return
      const sh = { status, order: [] as string[], name: new Set<string>(), tasks: new Map<string, Set<string>>(), offset: p.items.length, total: p.total }
      applySearch(p.items, sh)
      searchHits.value = sh
      void checkExistence(p.items)
    } catch (e) {
      if (seq === searchSeq) say(errOf(e).message)
    } finally {
      if (seq === searchSeq) searching.value = false
    }
  }

  // 筛选中：有转换开始 / 结束时重新取一次筛选的第一页（新出现的进行中 / 失败行加进来；已显示的行保留）
  let filterTimer: ReturnType<typeof setTimeout> | undefined
  watch(
    () => activeConvert.value.map((t) => t.id).join(','),
    () => {
      const fh = filterHits.value
      if (!fh) return
      clearTimeout(filterTimer)
      filterTimer = setTimeout(() => {
        if (filterHits.value === fh) void loadFiltered(fh.status, fh.offset, true)
      }, 400)
    },
  )

  // 别处新建 / 原地重试的转换任务（任务中心重试、旧 Submit）：任务 store 里出现进行中的 convert 任务时同步到记录
  let topTimer: ReturnType<typeof setTimeout> | undefined
  function refreshTop() {
    clearTimeout(topTimer)
    topTimer = setTimeout(async () => {
      try {
        const p = await listSources({ limit: SOURCE_PAGE, offset: 0, recordLimit: RECORD_LIMIT })
        listOffset.value += putEntries(p.items) // 新出现在最上面的行算进已加载的部分（走查 G1）
        listTotal.value = p.total
      } catch (e) {
        console.warn('refresh sources failed', e)
      }
    }, 300)
  }
  watch(
    () => activeConvert.value.map((t) => `${t.id}:${t.version}`).join(','),
    () => {
      if (!loaded.value) return
      for (const t of activeConvert.value) {
        const r = records[t.id]
        if (!r) {
          if (!t.sourceId) continue
          if (!sources[t.sourceId]) refreshTop()
          else addRecord(t) // task:status 可能先于 SubmitSources 返回：和 afterSubmit 共用一处计数（S1）
        } else if (TERMINAL.includes(r.status)) {
          // 原地重试（可能是任务中心点的）：上一次的结果、错误、“文件已被移动”标记都作废
          records[t.id] = { ...r, status: t.status, version: t.version, error: null, result: undefined, hwFallback: undefined, hwFallbackReason: undefined, progress: 0 }
          outputGone.delete(t.id)
          recThumbs.delete(t.id)
        }
      }
    },
  )

  // ---------------- 读取媒体信息（MediaService.Probe） ----------------
  /** 需要当次探测的行：勾选的（冲突预检）、没有 media 缓存且在可视区域的（显示信息） */
  const wantProbe = new Set<string>()
  function requestMeta(s: SourceRow) {
    if (s.probe !== 'pending' || s.exists === false) return
    if (s.media && !selected.has(s.sourceId)) return
    wantProbe.add(s.sourceId)
    void probePending()
  }
  let probing = false
  async function probePending() {
    if (probing || !ffmpeg.ready) return
    probing = true
    try {
      for (;;) {
        const batch = Object.values(sources)
          .filter((s) => s.probe === 'pending' && s.exists !== false && (selected.has(s.sourceId) || wantProbe.has(s.sourceId)))
          .slice(0, MAX_SUBMIT)
        if (!batch.length) break
        for (const s of batch) s.probe = 'probing'
        await probeSources(batch.map((s) => s.path), (results) => {
          for (const res of results) {
            const s = batch.find((b) => b.path === res.path)
            if (!s || !sources[s.sourceId]) continue
            wantProbe.delete(s.sourceId)
            if (res.info) {
              s.info = res.info
              s.probe = 'ok'
            } else if (res.error?.code === 'FFMPEG_NOT_FOUND') {
              s.probe = 'pending'
            } else {
              s.probe = 'error'
              s.probeError = res.error
              selected.delete(s.sourceId) // 读取失败的不能勾选
            }
          }
        })
        if (!ffmpeg.ready) break
      }
    } finally {
      probing = false
    }
  }
  watch(() => ffmpeg.ready, (ok) => ok && void probePending())

  // ---------------- 缩略图（按 id，后端懒生成；行进入可视区域时取） ----------------
  let thumbActive = 0
  const thumbQueue: (() => Promise<void>)[] = []
  function pump() {
    while (thumbActive < 3 && thumbQueue.length) {
      const job = thumbQueue.shift()!
      thumbActive++
      void job().finally(() => {
        thumbActive--
        pump()
      })
    }
  }
  // 包 19 Windows 实测：启动时转换组件还在检测（checking）页面就取缩略图，后端只能返回 FFMPEG_NOT_FOUND，
  // 这一行就整个会话停在类型图标。没就绪时先登记、就绪后再取（同探测的 probePending）；
  // 取的过程中变成未就绪（重新检测 / 安装中）而失败的，也登记下来等就绪重取。
  const thumbWaiting = new Map<string, () => void>()
  watch(
    () => ffmpeg.ready,
    (ok) => {
      if (!ok || !thumbWaiting.size) return
      const jobs = [...thumbWaiting.values()]
      thumbWaiting.clear()
      jobs.forEach((f) => f())
    },
  )
  function ensureThumb(s: SourceRow) {
    if (s.thumbAsked) return
    if (s.exists === false) {
      s.thumbAsked = true
      s.thumb = { kind: 'missing' }
      return
    }
    const again = () => sources[s.sourceId] && ensureThumb(sources[s.sourceId])
    if (!ffmpeg.ready) {
      thumbWaiting.set('s:' + s.sourceId, again)
      return
    }
    s.thumbAsked = true
    thumbQueue.push(async () => {
      const t = await getSourceThumbnail(s.sourceId)
      const cur = sources[s.sourceId]
      if (!cur) return
      cur.thumb = t
      if (t.kind === 'missing') cur.exists = false
      if (t.kind === 'type' && !ffmpeg.ready) {
        cur.thumbAsked = false
        thumbWaiting.set('s:' + s.sourceId, again)
      }
    })
    pump()
  }
  /** 子记录（已完成、输出还在）的缩略图 */
  const recThumbs = reactive(new Map<string, ThumbState>())
  const recThumbAsked = new Set<string>()
  function ensureRecThumb(k: KidView) {
    if (recThumbs.has(k.id) && recThumbAsked.has(k.id)) return
    if (k.status !== 'succeeded' || k.outputGone) return
    if (recThumbAsked.has(k.id)) return
    recThumbAsked.add(k.id)
    if (isAudioContainer(k.options.container ?? '')) {
      recThumbs.set(k.id, { kind: 'type' })
      return
    }
    if (!ffmpeg.ready) {
      recThumbAsked.delete(k.id)
      thumbWaiting.set('r:' + k.id, () => ensureRecThumb(k))
      return
    }
    thumbQueue.push(async () => {
      const t = await getRecordThumbnail(k.id)
      recThumbs.set(k.id, t)
      if (t.kind === 'missing') outputGone.add(k.id)
      if (t.kind === 'type' && !ffmpeg.ready) {
        recThumbAsked.delete(k.id)
        thumbWaiting.set('r:' + k.id, () => ensureRecThumb(k))
      }
    })
    pump()
  }

  // ---------------- 添加文件 ----------------
  async function addPaths(paths: string[]) {
    notice.value = ''
    const seen = new Set<string>()
    const uniq = paths.filter((p) => {
      const id = normalizeSourcePath(p)
      if (!p || seen.has(id)) return false
      seen.add(id)
      return true
    })
    const take = uniq.slice(0, MAX_SUBMIT)
    if (uniq.length > MAX_SUBMIT) notice.value = `一次最多添加 ${MAX_SUBMIT} 个文件，多出的 ${uniq.length - MAX_SUBMIT} 个没有加入。`
    if (!take.length) return
    let list
    try {
      list = await addSources(take)
    } catch (e) {
      notice.value = errOf(e).message
      return
    }
    const now = Date.now()
    const failed: string[] = []
    for (const r of list) {
      if (!r.source) {
        failed.push(`${r.path.split(/[\\/]/).pop()}：${r.error?.message ?? '没有加入'}`)
        continue
      }
      const had = !!sources[r.source.sourceId]
      if (!had) {
        // 新行放在最上面，算进已加载的部分（走查 G1：只加 listTotal 会误出“加载更早的记录”）；后端原来就有、只是没加载到的行，总数不变
        listOffset.value++
        if (!r.existed) listTotal.value++
      }
      const row = upsertSource(r.source)
      row.lastActivityAt = Math.max(r.source.lastActivityAt, now) // 重复添加：移到最上面
      row.exists = true
      if (had) row.flashAt = now
      if (searchHits.value && !searchHits.value.order.includes(row.sourceId)) searchHits.value.order.unshift(row.sourceId)
      if (filterHits.value && !filterHits.value.order.includes(row.sourceId)) filterHits.value.order.unshift(row.sourceId) // 刚添加的行在筛选里也先显示出来
      if (isCheckable(row)) selected.add(row.sourceId) // 新加入的自动勾选
    }
    if (failed.length) notice.value = failed.join('；')
    addedTick.value = now
    void probePending()
  }
  let mockPick = 0
  async function chooseFiles() {
    notice.value = ''
    if (!hasWailsBackend()) {
      // 纯浏览器（模拟）：没有系统文件对话框，加一个示例文件，方便走查
      await addPaths([`D:\\Videos\\${MOCK_NAMES[mockPick++ % MOCK_NAMES.length]}`])
      return
    }
    if (!canPickFiles()) {
      notice.value = '选择文件功能即将上线，请先把文件拖到这里。'
      return
    }
    try {
      const paths = await pickFiles()
      if (paths.length) await addPaths(paths)
    } catch (e) {
      notice.value = errOf(e).message
    }
  }

  // ---------------- 输出位置 / 提交 ----------------
  const effectiveOutputDir = computed(() => outputOverride.value || defaultOutputDir.value)
  async function chooseOutputDir() {
    const dir = hasWailsBackend() ? await pickDirectory('选择这次转换的输出文件夹') : 'D:\\Videos\\FFmpegFree'
    if (dir) outputOverride.value = dir
  }
  const optionsOf = (o: Partial<RecordOptions>): RecordOptions => ({
    container: o.container ?? '', videoCodec: o.videoCodec ?? '', audioCodec: o.audioCodec ?? '', width: o.width ?? 0, height: o.height ?? 0, fps: o.fps ?? 0,
    videoBitrate: o.videoBitrate ?? 0, audioBitrate: o.audioBitrate ?? 0, crf: o.crf ?? 0, targetSizeMb: o.targetSizeMb ?? 0, trimStart: o.trimStart ?? 0, trimEnd: o.trimEnd ?? 0,
  })
  /** 单选时页脚“将保存为…”：PreviewOutputName（不占位，提交时可能不同）；算不出来就不显示 */
  let nameSeq = 0
  watch(
    () => [submittableRows.value.length === 1 ? submittableRows.value[0].sourceId : '', selectedPreset.value?.id ?? '', outputOverride.value, recordCount.value] as const,
    async ([sid]) => {
      const seq = ++nameSeq
      const p = selectedPreset.value
      if (!sid || !p) {
        outputName.value = ''
        return
      }
      try {
        const full = await previewOutputName(sid, optionsOf(p.options), outputOverride.value)
        if (seq === nameSeq) outputName.value = full.split(/[\\/]/).pop() ?? ''
      } catch {
        if (seq === nameSeq) outputName.value = ''
      }
    },
  )

  function afterSubmit(list: V023Task[]) {
    for (const t of list) {
      addRecord(t) // 只在第一次见到这个 id 时计数（S1）
      const s = sources[t.sourceId ?? '']
      if (s) {
        s.lastActivityAt = Math.max(s.lastActivityAt, t.createdAt || Date.now())
        foldSession[s.sourceId] = true
      }
      round.add(t.id)
    }
    tasks.track(list.filter((t) => ACTIVE.includes(t.status)) as never)
  }
  /** 转换：只提交勾选里能转的（冲突 / 读取中的跳过）；成功后清空勾选、展开对应父行 */
  async function submit() {
    const p = selectedPreset.value
    const rows = submittableRows.value.slice()
    if (submitting.value || !p || !rows.length || !ffmpeg.ready) return
    submitting.value = true
    submitError.value = null
    try {
      const list = await submitSources({ sourceIds: rows.map((r) => r.sourceId), options: optionsOf(p.options), outputDir: outputOverride.value, presetId: p.id })
      afterSubmit(list)
      closeBanner()
      clearSelection()
    } catch (e) {
      submitError.value = errOf(e)
    } finally {
      submitting.value = false
    }
  }

  // ---------------- 记录操作 ----------------
  async function cancel(id: string) {
    try {
      await tasks.cancel(id)
    } catch (e) {
      say(errOf(e).message)
    }
  }
  /** 原地重试（失败 / 已中断的“重试”，已取消的“重新转换”）：同一条记录、同一个任务 id（TaskService.Retry） */
  async function retry(id: string) {
    const r = records[id]
    try {
      const t = await tasks.retry(id)
      if (!t) return
      if (r && t.id === id) {
        records[id] = { ...r, status: t.status, progress: 0, speed: '', etaSec: 0, error: null, startedAt: 0, finishedAt: 0, version: t.version, result: undefined, hwFallback: undefined, hwFallbackReason: undefined, outputPath: t.outputPath || r.outputPath }
        outputGone.delete(id)
        recThumbs.delete(id)
        recThumbAsked.delete(id)
      } else if (t.id !== id) addRecord(t) // 旧后端：重试生成了新任务
      round.add(t.id)
      if (r) foldSession[r.sourceId] = true
    } catch (e) {
      say(errOf(e).message)
    }
  }
  /** 已完成的记录“又转一次”（ConvertService.Reconvert，新增一条）。设计稿里没有入口，界面暂不放，见交付说明 */
  async function reconvert(id: string) {
    try {
      afterSubmit([await apiReconvert(id)])
    } catch (e) {
      say(errOf(e).message)
    }
  }
  /** 磁盘空间不足：换个文件夹重新转换（用这条记录的参数和预设，新增一条记录） */
  async function resubmitTo(id: string, dir: string) {
    const r = records[id]
    if (!r) return
    try {
      afterSubmit(await submitSources({ sourceIds: [r.sourceId], options: optionsOf(r.options), outputDir: dir, presetId: r.presetId ?? '' }))
    } catch (e) {
      say(errOf(e).message)
    }
  }

  /** 完成记录“打开所在文件夹”：ConvertService.RevealRecord(taskId)（v0.23.1）；文件不在 → NOT_FOUND reason=file → 标成“文件已被移动或删除”，返回 false */
  async function revealOutput(id: string): Promise<boolean> {
    try {
      await revealRecord(id)
      return true
    } catch (e) {
      const err = errOf(e)
      if (err.code === 'NOT_FOUND' && /^reason=file/.test(err.detail ?? '')) {
        outputGone.add(id)
        return false
      }
      if (err.code === 'NOT_FOUND' && /^reason=record/.test(err.detail ?? '')) {
        dropRecords([id])
        return true
      }
      say(err.message)
      return true
    }
  }
  async function revealSource(sourceId: string): Promise<boolean> {
    const s = sources[sourceId]
    if (!s) return false
    try {
      await apiRevealSource(sourceId)
      return true
    } catch (e) {
      const err = errOf(e)
      if (err.code === 'NOT_FOUND' && /^reason=file/.test(err.detail ?? '')) {
        markSourceGone(sourceId)
        return false
      }
      say(err.message)
      return true
    }
  }
  function markOutputGone(id: string) {
    outputGone.add(id)
  }
  function markSourceGone(sourceId: string) {
    const s = sources[sourceId]
    if (s) {
      s.exists = false
      s.thumb = { kind: 'missing' }
    }
    selected.delete(sourceId)
  }

  // ---------------- 删除 ----------------
  const deletable = (k: KidView) => k.status === 'succeeded' && !k.outputGone && !!k.outputPath
  function deleteAsk(kind: 'record' | 'source', id: string): DeleteAsk | null {
    if (kind === 'record') {
      const k = liveById.value.get(id)
      if (!k) return null
      const out = deletable(k)
      return { kind, id, title: '删除这条转换记录？', name: k.outputPath.split(/[\\/]/).pop() ?? '', count: 1, activeCount: ACTIVE.includes(k.status) ? 1 : 0, outputs: out ? 1 : 0, outputBytes: out ? k.result?.sizeBytes ?? 0 : 0 }
    }
    const s = sources[id]
    if (!s) return null
    const kids = kidsBySource.value.get(id) ?? []
    const outs = kids.filter(deletable)
    const n = Math.max(s.recordCount, kids.length)
    return {
      kind, id, title: SOURCE_REMOVE_TITLE, name: s.name, count: n, audio: isAudioOnly(metaInfoOf(s)), // 定稿（产品 + 设计 10-08）：不论有几条记录都用这一句
      activeCount: kids.filter((k) => ACTIVE.includes(k.status)).length, outputs: outs.length, outputBytes: outs.reduce((x, k) => x + (k.result?.sizeBytes ?? 0), 0),
    }
  }
  /** 执行删除，返回删除结果（页面据此给提示） */
  async function confirmDelete(a: DeleteAsk, deleteOutput: boolean): Promise<DeleteResult> {
    const r = a.kind === 'record' ? await deleteRecords(a.ids ?? [a.id], deleteOutput && a.outputs > 0) : await deleteSource(a.id, deleteOutput && a.outputs > 0)
    dropRecords(r.deletedTaskIds)
    for (const sid of r.deletedSourceIds) {
      if (!sources[sid]) continue
      delete sources[sid]
      selected.delete(sid)
      delete foldUser[sid]
      listTotal.value = Math.max(0, listTotal.value - 1)
      listOffset.value = Math.max(0, listOffset.value - 1) // 删掉已加载的一行，后面的行前移一位
      if (searchHits.value) searchHits.value.order = searchHits.value.order.filter((x) => x !== sid)
      if (filterHits.value) {
        filterHits.value.order = filterHits.value.order.filter((x) => x !== sid)
        filterHits.value.offset = Math.max(0, filterHits.value.offset - 1)
        filterHits.value.total = Math.max(0, filterHits.value.total - 1)
      }
    }
    saveFold(foldUser)
    return r
  }
  function dropRecords(ids: string[]) {
    for (const id of ids) {
      const r = records[id]
      if (r && sources[r.sourceId]) {
        const s = sources[r.sourceId]
        s.recordCount = Math.max(0, s.recordCount - 1)
        s.loadedIds = s.loadedIds.filter((x) => x !== id)
      }
      delete records[id]
      outputGone.delete(id)
      round.delete(id)
      recThumbs.delete(id)
    }
  }

  // ---------------- 页脚 ----------------
  const startBlock = computed<'' | 'ffmpeg' | 'preset' | 'empty' | 'none' | 'probing' | 'conflict' | 'submitting'>(() => {
    if (submitting.value) return 'submitting'
    if (!ffmpeg.ready) return 'ffmpeg'
    if (!selectedPreset.value) return 'preset'
    if (!sourceCount.value) return 'empty'
    if (!selectedRows.value.length) return 'none'
    if (!submittableRows.value.length) return probingSelected.value ? 'probing' : 'conflict'
    return ''
  })

  return {
    // 数据
    sources, records, selected, outputGone, recThumbs, loaded, loading, loadError, filter, filterHits, filtering, keyword, searching, searchHits, notice, toast, addedTick, pinned, focus,
    presets, presetsLoaded, presetsError, selectedPresetId, selectedPreset, tab, shownPresets, outputOverride, defaultOutputDir, effectiveOutputDir,
    submitting, submitError, outputName, round, roundBanner,
    // 派生
    parents, groups, sourceCount, recordCount, hasMore, loadingMore, total, liveById, selectedRows, submittableRows, blockedCount, probingSelected, startBlock,
    // 方法
    init, reload, loadMore, loadMoreRecords, locate, setFilter, search, loadPresets, presetTitle, setTab, isAudioPreset, toggle, clearSelection, isCheckable, conflictOfSource, setOpen,
    addPaths, chooseFiles, chooseOutputDir, submit, afterSubmit, cancel, retry, reconvert, resubmitTo, revealOutput, revealSource, markOutputGone, markSourceGone,
    deleteAsk, confirmDelete, ensureThumb, ensureRecThumb, requestMeta, closeBanner, probePending, liveOf, say,
  }
})
