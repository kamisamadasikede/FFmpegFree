// 直播页三张表单（文件推流 / 录屏推流 / 拉流播放）的输入：放在 store 里，切换菜单（页面组件被销毁）后原样还在；
// 并存到本机 localStorage（ffmpegfree.live.forms.v1），下次打开程序恢复上次填的内容（老板要求，架构师定：存本机设置）。
// 隐私：推流地址 / 推流码 / 口令只存在本机（WebView 的 localStorage），不发给任何服务、不写日志、不打 console；界面上推流地址里的推流码默认遮挡（LiveInput mask-key）。
// 不存：预览开关（产品经理已定：不记住上次选择，每次打开默认开）、错误状态、进行中的会话（会话以后端 / liveSessions 为准）。
import { defineStore } from 'pinia'
import { reactive, watch } from 'vue'
import type { CaptureSource, LiveMaterial } from '@/api/live'
import { previewParams } from '@/services/wails'

export const LIVE_FORMS_KEY = 'ffmpegfree.live.forms.v1'
/** 写盘防抖（毫秒） */
export const LIVE_FORMS_SAVE_DELAY = 400
const MAX_TEXT = 4096

export interface FilePushForm {
  material: LiveMaterial | null
  baseUrl: string
  key: string
}
export interface ScreenPushForm {
  /** 采集来源 id（屏幕 / 窗口）；恢复后不在当前列表里就退回默认（第一个屏幕） */
  sourceId: string
  baseUrl: string
  key: string
  archiveOn: boolean
  archiveDir: string
}
export interface PullForm {
  url: string
  lowLatency: boolean
  muted: boolean
}
export interface LiveForms {
  file: FilePushForm
  screen: ScreenPushForm
  pull: PullForm
}

export const defaultLiveForms = (): LiveForms => ({
  file: { material: null, baseUrl: '', key: '' },
  screen: { sourceId: '', baseUrl: '', key: '', archiveOn: false, archiveDir: '' },
  pull: { url: '', lowLatency: true, muted: false },
})

type Storage = Pick<globalThis.Storage, 'getItem' | 'setItem'>
const storage = (): Storage | undefined => {
  try {
    return globalThis.localStorage ?? undefined
  } catch {
    return undefined
  }
}

const str = (v: unknown, d: string) => (typeof v === 'string' && v.length <= MAX_TEXT ? v : d)
const bool = (v: unknown, d: boolean) => (typeof v === 'boolean' ? v : d)
const obj = (v: unknown): Record<string, unknown> => (v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {})
function material(v: unknown): LiveMaterial | null {
  const m = obj(v)
  const path = str(m.path, '')
  if (!path) return null
  return { path, name: str(m.name, '') || path.split(/[\\/]/).pop() || path, duration: str(m.duration, '') }
}

/** 解析存档：版本不对 / JSON 坏了 → 全部默认；单个字段类型不对 → 只把这个字段退回默认，其余保留 */
export function parseLiveForms(raw: string | null | undefined): LiveForms {
  const d = defaultLiveForms()
  if (!raw) return d
  let v: Record<string, unknown>
  try {
    v = obj(JSON.parse(raw))
  } catch {
    return d
  }
  if (v.v !== 1) return d
  const f = obj(v.file)
  const s = obj(v.screen)
  const p = obj(v.pull)
  return {
    file: { material: material(f.material), baseUrl: str(f.baseUrl, d.file.baseUrl), key: str(f.key, d.file.key) },
    screen: {
      sourceId: str(s.sourceId, d.screen.sourceId),
      baseUrl: str(s.baseUrl, d.screen.baseUrl),
      key: str(s.key, d.screen.key),
      archiveOn: bool(s.archiveOn, d.screen.archiveOn),
      archiveDir: str(s.archiveDir, d.screen.archiveDir),
    },
    pull: { url: str(p.url, d.pull.url), lowLatency: bool(p.lowLatency, d.pull.lowLatency), muted: bool(p.muted, d.pull.muted) },
  }
}

export const serializeLiveForms = (f: LiveForms): string => JSON.stringify({ v: 1, file: f.file, screen: f.screen, pull: f.pull })

/**
 * 录屏来源恢复：保存的 id 还在当前列表里就沿用，否则退回默认（第一个屏幕，没有屏幕取第一项；列表为空返回 ''）。
 * 窗口 id 每次启动可能变化，变了就是“不在了”，安静地退回默认，不报错。
 */
export function restoreSourceId(list: Pick<CaptureSource, 'id' | 'kind'>[], saved: string): string {
  if (!list.length) return ''
  if (saved && list.some((x) => x.id === saved)) return saved
  return (list.find((x) => x.kind === 'screen') ?? list[0]).id
}

/** 浏览器预览（?form= / ?live= / ?rows=）预置的演示表单不读也不写本机存档，免得把演示值存下来 */
const demo = (): boolean => {
  try {
    return ['form', 'live', 'rows'].some((k) => previewParams.has(k))
  } catch {
    return false
  }
}

export const useLiveFormsStore = defineStore('liveForms', () => {
  const persist = !demo()
  const init = persist ? parseLiveForms(storage()?.getItem(LIVE_FORMS_KEY)) : defaultLiveForms()
  const file = reactive<FilePushForm>(init.file)
  const screen = reactive<ScreenPushForm>(init.screen)
  const pull = reactive<PullForm>(init.pull)
  /** 这次启动从存档里恢复了素材文件，还没核对它是否仍然存在 */
  let materialUnchecked = !!init.file.material

  let timer: ReturnType<typeof setTimeout> | null = null
  function flush() {
    if (timer) clearTimeout(timer)
    timer = null
    if (!persist) return
    try {
      storage()?.setItem(LIVE_FORMS_KEY, serializeLiveForms({ file, screen, pull }))
    } catch {
      /* 存储满 / 被禁用：只是下次不恢复，不影响使用；不打日志（内容里有推流地址） */
    }
  }
  function scheduleSave() {
    if (!persist) return
    if (timer) clearTimeout(timer)
    timer = setTimeout(flush, LIVE_FORMS_SAVE_DELAY)
  }
  watch([file, screen, pull], scheduleSave, { deep: true })
  // 关窗口前把防抖中的修改写掉
  try {
    globalThis.addEventListener?.('pagehide', flush)
    globalThis.addEventListener?.('beforeunload', flush)
  } catch {
    /* node 自检环境 */
  }

  /**
   * 恢复的素材文件已经不在（或不再能推流）→ 只清掉素材，地址 / 推流码等其余字段保留。每次启动只核对一次；
   * exists 由页面传入（真实环境 = MediaService.Probe）。用户在核对期间换了文件就不动。
   */
  async function validateRestoredMaterial(exists: (path: string) => Promise<boolean>): Promise<void> {
    if (!materialUnchecked) return
    materialUnchecked = false
    const m = file.material
    if (!m) return
    let ok = true
    try {
      ok = await exists(m.path)
    } catch {
      ok = false
    }
    if (!ok && file.material === m) file.material = null
  }

  function reset() {
    const d = defaultLiveForms()
    Object.assign(file, d.file)
    Object.assign(screen, d.screen)
    Object.assign(pull, d.pull)
  }

  return { file, screen, pull, flush, validateRestoredMaterial, reset }
})
