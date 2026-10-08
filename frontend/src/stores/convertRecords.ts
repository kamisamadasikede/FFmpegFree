/**
 * 转换页 v2（转换记录）的 store：把源文件（convert_sources）和转换记录（convert 任务）按 sourceId 合成“父行 + 子记录”。
 * 记录的实时状态来自任务 store（task:* 事件，不轮询）；记录本身（预设、输出信息）来自 api/convertRecords.ts。
 * 设计：转换页-v2-设计说明-v0.1.md §三 / §四 / §7.3；产品决定 v1（共享任务记录、原地重试、勾选在转换后清空、记录不自动清理）。
 */
import { defineStore } from 'pinia'
import { computed, reactive, ref, watch } from 'vue'
import { toAppError } from '@/api/call'
import { listPresets, MAX_SUBMIT, type PresetItem } from '@/api/convert'
import {
  addSources, checkPaths, checkSources, convertV2IsReal, deleteBySource, deleteRecords, FIRST_PAGE, getRecords, getThumbnail, listRecords, listSources, MORE_PAGE,
  normalizeSourcePath, previewOutputName, removeSource, revealPath, saveSourceInfo, searchRecords, submitRecords,
  type ConvertRecord, type ConvertSource, type RecordFilter, type RecordOptions,
} from '@/api/convertRecords'
import { mockSceneUi } from '@/api/convertRecordsMock'
import { probeFiles } from '@/api/media'
import { canPickFiles, getDefaultOutputDir, pickDirectory, pickFiles } from '@/api/system'
import { hasWailsBackend } from '@/services/wails'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { useTaskStore, type TaskError, type TaskStatus } from '@/stores/tasks'
import { conflictReason, isAudioContainer, isAudioOnly, isToday, splitPresetName, totalProgress } from '@/utils/convertText'
import { fileBaseName } from '@/utils/format'
import { codecName } from '@/utils/mediaText'
import type { store as goStore } from '../../wailsjs/go/models'

export type ProbeState = 'pending' | 'probing' | 'ok' | 'error'

/** 父行（源文件） */
export interface SourceRow {
  sourceId: string
  path: string
  name: string
  addedAt: number
  info?: goStore.MediaInfo
  probe: ProbeState
  probeError?: TaskError
  /** false = 原位置找不到；undefined = 没检查过（按存在处理） */
  exists?: boolean
  thumb: string
  thumbState: 'idle' | 'loading' | 'done'
  /** 重复添加时闪一下：时间戳，页面据此加高亮动画 */
  flashAt: number
  /** 是否在 convert_sources 表里（只出现在记录里的输入文件为 false；读取到的信息只回写表里的行） */
  persisted: boolean
}

/** 子记录 = 记录 + 任务 store 里的实时状态 */
export interface KidView extends ConvertRecord {
  outputGone: boolean
}

export interface ParentView {
  src: SourceRow
  kids: KidView[]
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
}

export interface GroupView {
  key: 'today' | 'earlier'
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

/** 父行的最后活动时间：加入时间、记录的创建 / 开始 / 结束时间里最大的 */
export function lastActivityOf(addedAt: number, kids: readonly { createdAt: number; startedAt: number; finishedAt: number }[]): number {
  return kids.reduce((m, k) => Math.max(m, k.createdAt || 0, k.startedAt || 0, k.finishedAt || 0), addedAt || 0)
}

const ACTIVE: TaskStatus[] = ['queued', 'running']
const FAILED: TaskStatus[] = ['failed', 'interrupted']
const MOCK_NAMES = ['航拍-西湖日落.mp4', '会议录像-周例会.mkv', '访谈录音_第三期.wav', '产品宣传片-竖版.mov', 'vlog-第7期.mp4']

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
  const listTotal = ref(0)
  const cursor = ref('')
  const loadingMore = ref(false)

  const filter = ref<RecordFilter>('all')
  const filterIds = ref<Set<string> | null>(null)
  const keyword = ref('')
  const searchIds = ref<Set<string> | null>(null)
  const searchSourceIds = ref<Set<string>>(new Set())
  const searching = ref(false)

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
  const outputName = ref('')
  /** 最近一次添加文件的时间：页面据此滚回顶部 */
  const addedTick = ref(0)

  // ---------------- 预设 ----------------
  const selectedPreset = computed(() => presets.value.find((p) => p.id === selectedPresetId.value))
  const isAudioPreset = (p: PresetItem) => isAudioContainer(p.options.container)
  /** 预设卡标题：标题重名（MP4 H.264 / H.265）时加编码简称「MP4 · H.265」 */
  const presetTitles = computed(() => {
    const count = new Map<string, number>()
    for (const p of presets.value) count.set(splitPresetName(p.name).title, (count.get(splitPresetName(p.name).title) ?? 0) + 1)
    const m = new Map<string, string>()
    for (const p of presets.value) {
      const t = splitPresetName(p.name).title
      const codec = (count.get(t) ?? 0) > 1 ? codecName(p.options.videoCodec || p.options.audioCodec) : ''
      m.set(p.id, codec ? `${t} · ${codec}` : t)
    }
    return m
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
      presetsError.value = null
      if (!selectedPreset.value && presets.value.length) selectedPresetId.value = presets.value[0].id
      if (selectedPreset.value) tab.value = isAudioPreset(selectedPreset.value) ? 'audio' : 'video'
    } catch (e) {
      const err = toAppError(e)
      presetsError.value = { code: err.code, message: err.message, detail: err.detail }
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

  // ---------------- 源文件 ----------------
  function upsertSource(s: ConvertSource, persisted: boolean): SourceRow {
    const cur = sources[s.sourceId]
    if (cur) {
      if (s.addedAt && persisted) cur.addedAt = s.addedAt
      if (s.info && cur.probe !== 'ok') {
        cur.info = s.info
        cur.probe = 'ok'
      }
      if (s.exists !== undefined) cur.exists = s.exists
      cur.persisted = cur.persisted || persisted
      return cur
    }
    const row: SourceRow = {
      sourceId: s.sourceId, path: s.path, name: fileBaseName(s.path), addedAt: s.addedAt, probe: s.info ? 'ok' : s.probeError ? 'error' : 'pending',
      ...(s.info ? { info: s.info } : {}), ...(s.probeError ? { probeError: s.probeError } : {}), ...(s.exists !== undefined ? { exists: s.exists } : {}),
      thumb: '', thumbState: 'idle', flashAt: 0, persisted,
    }
    sources[s.sourceId] = row
    return sources[s.sourceId]
  }
  /** 记录的输入文件不在源文件表里时，补一个父行（加入时间按最早一条记录） */
  function ensureSourceFor(r: ConvertRecord) {
    if (sources[r.sourceId]) return
    upsertSource({ sourceId: r.sourceId, path: r.inputPath, addedAt: r.createdAt }, false)
  }
  function putRecords(list: ConvertRecord[]) {
    for (const r of list) {
      records[r.id] = r
      ensureSourceFor(r)
    }
  }

  // ---------------- 实时状态 ----------------
  /** 记录 + 任务 store 里的实时状态（任务 store 有这个 id 时以它为准） */
  function liveOf(r: ConvertRecord): KidView {
    const t = tasks.taskById(r.id)
    const gone = outputGone.has(r.id)
    if (!t) return { ...r, outputGone: gone }
    return {
      ...r,
      status: t.status,
      progress: t.status === 'succeeded' ? 1 : t.progress,
      speed: t.speed ?? '',
      etaSec: t.etaSec ?? 0,
      error: t.error ?? null,
      outputPath: t.outputPath || r.outputPath,
      startedAt: t.startedAt || r.startedAt,
      finishedAt: t.finishedAt || r.finishedAt,
      encoder: t.encoder ?? r.encoder,
      encoderDevice: t.encoderDevice ?? r.encoderDevice,
      hwFallback: t.hwFallback,
      hwFallbackReason: t.hwFallbackReason,
      outputGone: gone,
    }
  }
  const liveAll = computed(() => Object.values(records).map(liveOf))
  const liveById = computed(() => new Map(liveAll.value.map((k) => [k.id, k])))

  // ---------------- 勾选 / 冲突 ----------------
  const isCheckable = (s: SourceRow) => s.exists !== false && s.probe !== 'error'
  function conflictOfSource(s: SourceRow): string | null {
    return conflictReason(s.info, s.probe === 'ok', selectedPreset.value?.options.container)
  }
  function toggle(sourceId: string) {
    const s = sources[sourceId]
    if (!s) return
    if (selected.has(sourceId)) selected.delete(sourceId)
    else if (isCheckable(s)) selected.add(sourceId)
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
  const parents = computed<ParentView[]>(() => {
    const bySrc = new Map<string, KidView[]>()
    for (const k of liveAll.value) {
      const l = bySrc.get(k.sourceId)
      if (l) l.push(k)
      else bySrc.set(k.sourceId, [k])
    }
    const out: ParentView[] = []
    const fIds = filterIds.value
    const sIds = searchIds.value
    const kw = keyword.value.trim().toLowerCase()
    for (const src of Object.values(sources)) {
      let kids = (bySrc.get(src.sourceId) ?? []).slice().sort((a, b) => b.createdAt - a.createdAt)
      const lastActivityAt = lastActivityOf(src.addedAt, kids)
      if (sIds) {
        // 搜索：后端返回的记录 + 文件名匹配的源文件（还没有记录的也算）
        const nameHit = searchSourceIds.value.has(src.sourceId) || (!!kw && src.name.toLowerCase().includes(kw))
        const hitKids = kids.filter((k) => sIds.has(k.id) || (!!kw && fileBaseName(k.outputPath).toLowerCase().includes(kw)))
        if (!nameHit && !hitKids.length) continue
        if (!nameHit) kids = hitKids
      }
      if (filter.value !== 'all') {
        const want = filter.value === 'active' ? ACTIVE : FAILED
        kids = kids.filter((k) => want.includes(k.status) || (fIds?.has(k.id) && want.includes(k.status)))
        if (!kids.length) continue
      }
      const running = kids.filter((k) => k.status === 'running')
      const pv: ParentView = {
        src,
        kids,
        lastActivityAt,
        open: kids.length > 0 && isOpen(src.sourceId, kids, lastActivityAt),
        running: running.length,
        queued: kids.filter((k) => k.status === 'queued').length,
        failed: kids.filter((k) => FAILED.includes(k.status)).length,
        runPct: running.length ? Math.round(running[0].progress * 100) : 0,
        conflict: selected.has(src.sourceId) ? conflictOfSource(src) : null,
        selected: selected.has(src.sourceId),
        checkable: isCheckable(src),
      }
      out.push(pv)
    }
    return out.sort((a, b) => b.lastActivityAt - a.lastActivityAt || a.src.name.localeCompare(b.src.name))
  })
  const groups = computed<GroupView[]>(() => {
    const today: ParentView[] = []
    const earlier: ParentView[] = []
    for (const p of parents.value) (isToday(p.lastActivityAt) ? today : earlier).push(p)
    const g: GroupView[] = []
    if (today.length) g.push({ key: 'today', label: '今天', parents: today })
    if (earlier.length) g.push({ key: 'earlier', label: '更早', parents: earlier })
    return g
  })
  const sourceCount = computed(() => Object.keys(sources).length)
  /** 记录总数：后端总数和本地已知的取大（新提交 / 删除后本地先变） */
  const recordCount = computed(() => Math.max(listTotal.value, Object.keys(records).length))
  const hasMore = computed(() => !!cursor.value)

  // ---------------- 总进度 / 本轮完成 ----------------
  const total = computed(() => totalProgress(liveAll.value))
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
    () => liveAll.value.filter((k) => ACTIVE.includes(k.status)).map((k) => k.id).join(','),
    (ids) => {
      for (const id of ids ? ids.split(',') : []) round.add(id)
      if (!round.size || ids) return
      let ok = 0
      let fail = 0
      for (const id of round) {
        const k = liveById.value.get(id)
        if (k?.status === 'succeeded') ok++
        else if (k && FAILED.includes(k.status)) fail++
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
  }
  async function reload() {
    loading.value = true
    try {
      const [srcs, page] = await Promise.all([listSources(), listRecords({ filter: 'all', limit: FIRST_PAGE, cursor: '' })])
      for (const s of srcs) upsertSource(s, true)
      putRecords(page.items)
      listTotal.value = page.total
      cursor.value = page.nextCursor
      loadError.value = null
      loaded.value = true
      void checkExistence()
      void probePending()
    } catch (e) {
      const err = toAppError(e)
      loadError.value = { code: err.code, message: err.message, detail: err.detail }
    } finally {
      loading.value = false
    }
  }
  /** 打开页面时检查一次：输出文件 / 源文件还在不在（设计 §4：只检查一次，打开文件夹前再查一次） */
  async function checkExistence() {
    try {
      const done = Object.values(records).filter((r) => liveOf(r).status === 'succeeded').map((r) => r.id)
      const [paths, srcs] = await Promise.all([done.length ? checkPaths(done) : Promise.resolve([]), checkSources(Object.keys(sources))])
      for (const c of paths) {
        if (!c.outputExists) outputGone.add(c.taskId)
        const r = records[c.taskId]
        if (r && !c.inputExists && sources[r.sourceId]) sources[r.sourceId].exists = false
      }
      for (const c of srcs) if (sources[c.sourceId]) sources[c.sourceId].exists = c.exists
    } catch (e) {
      console.warn('check paths failed', e) // 查不了就按都在处理，打开文件夹前还会再查
    }
  }
  /** “加载更早的记录”：按当前模式（全部 / 筛选 / 搜索）取下一页 */
  async function loadMore() {
    if (!cursor.value || loadingMore.value) return
    loadingMore.value = true
    try {
      if (searchIds.value) {
        const p = await searchRecords(keyword.value.trim(), MORE_PAGE, cursor.value)
        putRecords(p.items)
        for (const r of p.items) searchIds.value.add(r.id)
        cursor.value = p.nextCursor
      } else {
        const p = await listRecords({ filter: filter.value, limit: MORE_PAGE, cursor: cursor.value })
        putRecords(p.items)
        if (filterIds.value) for (const r of p.items) filterIds.value.add(r.id)
        cursor.value = p.nextCursor
      }
      void probePending()
    } catch (e) {
      notice.value = toAppError(e).message
    } finally {
      loadingMore.value = false
    }
  }
  async function setFilter(f: RecordFilter) {
    filter.value = f
    if (f === 'all') {
      filterIds.value = null
      if (!searchIds.value) {
        const p = await listRecords({ filter: 'all', limit: FIRST_PAGE, cursor: '' }).catch(() => null)
        if (p) {
          putRecords(p.items)
          listTotal.value = p.total
          cursor.value = p.nextCursor
        }
      }
      return
    }
    try {
      const p = await listRecords({ filter: f, limit: FIRST_PAGE, cursor: '' })
      if (filter.value !== f) return
      putRecords(p.items)
      filterIds.value = new Set(p.items.map((r) => r.id))
      if (!searchIds.value) cursor.value = p.nextCursor
    } catch (e) {
      notice.value = toAppError(e).message
    }
  }
  let searchSeq = 0
  /** 搜索全部记录（后端 SearchConvert，分页）；空关键字退出搜索。防抖在页面里做（300ms） */
  async function search(k: string) {
    keyword.value = k
    const kw = k.trim()
    const seq = ++searchSeq
    if (!kw) {
      searchIds.value = null
      searchSourceIds.value = new Set()
      searching.value = false
      await setFilter(filter.value)
      return
    }
    searching.value = true
    try {
      const p = await searchRecords(kw, FIRST_PAGE, '')
      if (seq !== searchSeq) return
      putRecords(p.items)
      for (const s of p.sources) upsertSource(s, true)
      searchIds.value = new Set(p.items.map((r) => r.id))
      searchSourceIds.value = new Set(p.sources.map((s) => s.sourceId))
      cursor.value = p.nextCursor
    } catch (e) {
      if (seq === searchSeq) notice.value = toAppError(e).message
    } finally {
      if (seq === searchSeq) searching.value = false
    }
  }

  // 别处新建 / 原地重试的转换任务（任务中心重试、另一个窗口提交）：任务 store 里出现了本地不认识的进行中 convert 任务，就取一下记录
  const fetching = new Set<string>()
  async function fetchRecords(ids: string[]) {
    const want = ids.filter((id) => !fetching.has(id))
    if (!want.length) return
    for (const id of want) fetching.add(id)
    try {
      putRecords(await getRecords(want))
    } catch (e) {
      console.warn('get records failed', e)
    } finally {
      for (const id of want) fetching.delete(id)
    }
  }
  watch(
    () => tasks.active.filter((t) => t.type === 'convert' && !records[t.id]).map((t) => t.id).join(','),
    (ids) => {
      if (ids && loaded.value) void fetchRecords(ids.split(','))
    },
  )
  // 刚完成的记录还没有输出信息（大小 / 时长 / 分辨率）：补取一次；同时它的输出文件当然在
  const resultFetched = new Set<string>()
  watch(
    () => liveAll.value.filter((k) => k.status === 'succeeded' && !k.result && !resultFetched.has(k.id)).map((k) => k.id).join(','),
    (ids) => {
      if (!ids) return
      const list = ids.split(',')
      for (const id of list) resultFetched.add(id)
      void fetchRecords(list)
    },
  )

  // ---------------- 读取媒体信息 / 缩略图 ----------------
  let probing = false
  async function probePending() {
    if (probing || !ffmpeg.ready) return
    probing = true
    try {
      for (;;) {
        const batch = Object.values(sources).filter((s) => s.probe === 'pending' && s.exists !== false).slice(0, MAX_SUBMIT)
        if (!batch.length) break
        for (const s of batch) s.probe = 'probing'
        await probeFiles(batch.map((s) => s.path), (results) => {
          for (const res of results) {
            const s = batch.find((b) => b.path === res.path)
            if (!s || !sources[s.sourceId]) continue
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
            if (s.persisted && s.probe !== 'pending') void saveSourceInfo(s.sourceId, s.info, s.probeError).catch(() => {})
          }
        })
        if (!ffmpeg.ready) break
      }
    } finally {
      probing = false
    }
  }
  watch(() => ffmpeg.ready, (ok) => ok && void probePending())

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
  /** 父行缩略图（进入视野时才取） */
  function ensureThumb(s: SourceRow) {
    if (s.thumbState !== 'idle' || s.probe !== 'ok' || !s.info || s.exists === false) return
    if (isAudioOnly(s.info)) {
      s.thumbState = 'done'
      return
    }
    s.thumbState = 'loading'
    thumbQueue.push(async () => {
      s.thumb = await getThumbnail(s.path, Math.min(10, (s.info?.duration ?? 0) * 0.1), 128).catch(() => '')
      s.thumbState = 'done'
    })
    pump()
  }
  /** 子记录（已完成）的输出缩略图 */
  const outThumbs = reactive<Record<string, string>>({})
  const outThumbAsked = new Set<string>()
  function ensureOutThumb(k: KidView) {
    if (outThumbAsked.has(k.id) || k.status !== 'succeeded' || k.outputGone || isAudioContainer(k.options.container)) return
    outThumbAsked.add(k.id)
    thumbQueue.push(async () => {
      outThumbs[k.id] = await getThumbnail(k.outputPath, Math.min(10, (k.result?.durationSec ?? 0) * 0.1), 96).catch(() => '')
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
    let list: ConvertSource[]
    try {
      list = await addSources(take)
    } catch (e) {
      notice.value = toAppError(e).message
      return
    }
    const now = Date.now()
    for (const s of list) {
      const had = !!sources[s.sourceId]
      const row = upsertSource({ ...s, addedAt: Math.max(s.addedAt, now) }, true)
      row.addedAt = Math.max(s.addedAt, now) // 重复添加：移到最上面
      if (had) row.flashAt = now
      if (isCheckable(row)) selected.add(row.sourceId) // 新加入的自动勾选
    }
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
      notice.value = toAppError(e).message
    }
  }

  // ---------------- 输出位置 / 提交 ----------------
  const effectiveOutputDir = computed(() => outputOverride.value || defaultOutputDir.value)
  async function chooseOutputDir() {
    const dir = hasWailsBackend() ? await pickDirectory('选择这次转换的输出文件夹') : 'D:\\Videos\\FFmpegFree'
    if (dir) outputOverride.value = dir
  }
  /** 单选时页脚“将保存为…”：后端算（同名加序号）；算不出来就不显示 */
  let nameSeq = 0
  watch(
    () => [submittableRows.value.length === 1 ? submittableRows.value[0].path : '', selectedPreset.value?.id ?? '', outputOverride.value, recordCount.value] as const,
    async ([path, pid]) => {
      const seq = ++nameSeq
      if (!path || !pid || !selectedPreset.value) {
        outputName.value = ''
        return
      }
      try {
        const n = await previewOutputName(path, pid, selectedPreset.value.options.container, outputOverride.value)
        if (seq === nameSeq) outputName.value = n
      } catch {
        if (seq === nameSeq) outputName.value = ''
      }
    },
  )

  const optionsOf = (p: PresetItem): RecordOptions => ({
    container: p.options.container, videoCodec: p.options.videoCodec, audioCodec: p.options.audioCodec, width: p.options.width, height: p.options.height, fps: p.options.fps, audioBitrate: p.options.audioBitrate,
  })
  const toTaskLike = (r: ConvertRecord) => ({
    id: r.id, type: 'convert', status: r.status, title: fileBaseName(r.outputPath), inputPaths: [r.inputPath], outputPath: r.outputPath, progress: r.progress, speed: r.speed, etaSec: r.etaSec,
    outTimeSec: 0, params: '', version: r.version, error: r.error, createdAt: r.createdAt, startedAt: r.startedAt, finishedAt: r.finishedAt,
  })

  /** 转换：只提交勾选里能转的（冲突 / 读取中的跳过）；成功后清空勾选、展开对应父行 */
  async function submit() {
    const p = selectedPreset.value
    const rows = submittableRows.value.slice()
    if (submitting.value || !p || !rows.length || !ffmpeg.ready) return
    submitting.value = true
    submitError.value = null
    try {
      const list = await submitRecords(rows.map((r) => r.path), { id: p.id, name: p.name, options: optionsOf(p) }, outputOverride.value)
      putRecords(list)
      listTotal.value += list.length
      tasks.track(list.filter((r) => ACTIVE.includes(r.status)).map(toTaskLike) as never)
      for (const r of list) {
        round.add(r.id)
        foldSession[r.sourceId] = true
      }
      closeBanner()
      clearSelection()
    } catch (e) {
      const err = toAppError(e)
      submitError.value = { code: err.code, message: err.message, detail: err.detail }
    } finally {
      submitting.value = false
    }
  }

  // ---------------- 记录操作 ----------------
  async function cancel(id: string) {
    await tasks.cancel(id)
  }
  /** 原地重试（失败 / 已中断 / 已取消的“重新转换”）：同一条记录、同一个任务 id，清掉旧错误和回退提示 */
  async function retry(id: string) {
    const t = await tasks.retry(id)
    if (!t) return
    const r = records[id]
    if (t.id === id && r) {
      records[id] = {
        ...r, status: t.status, progress: 0, speed: '', etaSec: 0, error: null, startedAt: 0, finishedAt: 0, version: t.version,
        result: undefined, encoder: t.encoder, encoderDevice: t.encoderDevice, hwFallback: undefined, hwFallbackReason: undefined,
      }
      outputGone.delete(id)
      resultFetched.delete(id)
      delete outThumbs[id]
      outThumbAsked.delete(id)
    } else if (t.id !== id) {
      await fetchRecords([t.id]) // 旧后端：重试生成了新任务
    }
    round.add(t.id)
    if (r) foldSession[r.sourceId] = true
  }
  /** 磁盘空间不足：换个文件夹重新转换（新增一条记录） */
  async function resubmitTo(id: string, dir: string) {
    const r = records[id]
    if (!r) return
    const list = await submitRecords([r.inputPath], { id: r.presetId, name: r.presetName, options: r.options }, dir)
    putRecords(list)
    listTotal.value += list.length
    for (const x of list) round.add(x.id)
  }

  /** 打开前先查文件还在不在；不在就标记并返回 false（页面提示“文件已被移动或删除”） */
  async function revealOutput(id: string): Promise<boolean> {
    const k = liveById.value.get(id)
    if (!k) return false
    const [c] = await checkPaths([id]).catch(() => [undefined])
    if (c && !c.outputExists) {
      outputGone.add(id)
      return false
    }
    await revealPath(k.outputPath)
    return true
  }
  async function revealSource(sourceId: string): Promise<boolean> {
    const s = sources[sourceId]
    if (!s) return false
    const [c] = await checkSources([sourceId]).catch(() => [undefined])
    if (c && !c.exists) {
      s.exists = false
      selected.delete(sourceId)
      return false
    }
    await revealPath(s.path)
    return true
  }
  function markOutputGone(id: string) {
    outputGone.add(id)
  }
  function markSourceGone(sourceId: string) {
    const s = sources[sourceId]
    if (s) s.exists = false
    selected.delete(sourceId)
  }

  // ---------------- 删除 ----------------
  const deletable = (k: KidView) => k.status === 'succeeded' && !k.outputGone && !!k.outputPath
  function deleteAsk(kind: 'record' | 'source', id: string): DeleteAsk | null {
    if (kind === 'record') {
      const k = liveById.value.get(id)
      if (!k) return null
      const out = deletable(k)
      return { kind, id, title: '删除这条转换记录？', name: fileBaseName(k.outputPath), count: 1, activeCount: ACTIVE.includes(k.status) ? 1 : 0, outputs: out ? 1 : 0, outputBytes: out ? k.result?.sizeBytes ?? 0 : 0 }
    }
    const s = sources[id]
    if (!s) return null
    const kids = liveAll.value.filter((k) => k.sourceId === id)
    const outs = kids.filter(deletable)
    return {
      kind, id, title: kids.length ? `删除“${s.name}”和它的 ${kids.length} 条转换记录？` : `从列表里移除“${s.name}”？`, name: s.name, count: kids.length,
      activeCount: kids.filter((k) => ACTIVE.includes(k.status)).length, outputs: outs.length, outputBytes: outs.reduce((n, k) => n + (k.result?.sizeBytes ?? 0), 0),
    }
  }
  /** 执行删除，返回删掉的记录数 */
  async function confirmDelete(a: DeleteAsk, deleteOutput: boolean): Promise<number> {
    if (a.kind === 'record') {
      const n = await deleteRecords([a.id], deleteOutput && a.outputs > 0)
      dropRecords([a.id])
      return n
    }
    const ids = Object.values(records).filter((r) => r.sourceId === a.id).map((r) => r.id)
    const n = a.count ? await deleteBySource(a.id, deleteOutput && a.outputs > 0) : (await removeSource(a.id), 0)
    dropRecords(ids)
    delete sources[a.id]
    selected.delete(a.id)
    delete foldUser[a.id]
    saveFold(foldUser)
    return n
  }
  function dropRecords(ids: string[]) {
    for (const id of ids) {
      delete records[id]
      outputGone.delete(id)
      round.delete(id)
    }
    listTotal.value = Math.max(0, listTotal.value - ids.length)
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
    sources, records, selected, outputGone, outThumbs, loaded, loading, loadError, filter, keyword, searching, notice, addedTick,
    presets, presetsLoaded, presetsError, selectedPresetId, selectedPreset, tab, shownPresets, outputOverride, defaultOutputDir, effectiveOutputDir,
    submitting, submitError, outputName, round, roundBanner,
    // 派生
    parents, groups, sourceCount, recordCount, hasMore, loadingMore, total, liveById, selectedRows, submittableRows, blockedCount, probingSelected, startBlock,
    // 方法
    init, reload, loadMore, setFilter, search, loadPresets, presetTitle, setTab, isAudioPreset, toggle, clearSelection, isCheckable, conflictOfSource, setOpen,
    addPaths, chooseFiles, chooseOutputDir, submit, cancel, retry, resubmitTo, revealOutput, revealSource, markOutputGone, markSourceGone,
    deleteAsk, confirmDelete, ensureThumb, ensureOutThumb, closeBanner, probePending, liveOf,
  }
})
