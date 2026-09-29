/**
 * 编码设备（GPU 加速）接口封装。对着后端 #60 的真实绑定（wailsjs/go/app/SystemService、models.ts 的 system.EncoderDeviceList 等；契约 v0.15 §9.6）：
 *   SystemService.ListEncoderDevices() / RefreshEncoderDevices() → { ffmpegReady, devices[] }；第一项永远是 id = "cpu"；ffmpeg 未就绪时只有 cpu 且 ffmpegReady=false（不报错）
 *   SystemService.GetEncoderPreference() → "auto" | "cpu" | <设备 id>，默认 "auto"
 *   SystemService.GetEncoderPreferenceInfo() → { id, name, available, reason? }（设备不可用 / 不存在时 name 是保存偏好时记下的名字）
 *   SystemService.SetEncoderPreference(id)：非法或不在当前列表里 → INVALID_ARGUMENT 且不改原值；存在但 available=false 的允许保存
 * 设备字段：id、name、vendor、kind、discrete（独显）、available、reason?，另有 encoders{h264,hevc}（内部编码器名，本文件丢弃，界面不显示 NVENC 这类名字）。
 * 偏好指向的设备不可用 / 不存在时，后端在列表**末尾**追加一项 available=false 的占位（id=偏好值，name=记下的名字）。
 *
 * 开关 ENCODER_BACKEND_READY（flags.ts，**已为 true**：后端第二个 PR #67 / #68 已合入，回退提示和设备名已接线 #69）：
 *   - true 且在 Wails 里：设置页显示“编码设备”，调用真实绑定；`?enc=` 无效。
 *   - 纯浏览器（没有 window.go）：默认不显示；地址加 `?enc=` 才显示模拟层（仅开发 / 设计走查用；模拟的显卡名不给正式用户看）。
 *   - 开关改回 false（应急回滚）：不论在不在 Wails 里，正式包设置页和任务里都完全不显示编码设备相关界面。
 */
import { callService } from '@/api/call'
import { ENCODER_BACKEND_READY } from '@/api/flags'
import { simDelay, simError, simParam } from '@/api/sim'
import { hasWailsBackend } from '@/services/wails'

export { ENCODER_BACKEND_READY }

export type EncoderVendor = 'nvidia' | 'intel' | 'amd' | 'apple' | 'unknown'

export interface EncoderDevice {
  /** 稳定 id（偏好里存它；不要存显示名或序号）。第一项永远是 "cpu" */
  id: string
  name: string
  vendor: EncoderVendor
  kind: 'gpu' | 'cpu'
  available: boolean
  /** 独立显卡（后端字段；auto 选择时独显优先于集显）。设置页下拉据此把独显排在前面；CPU 恒为 false */
  discrete: boolean
  /** available=false 时的原因（后端给的，可能偏技术，界面不直接显示） */
  reason?: string
}

export interface EncoderDeviceList {
  ffmpegReady: boolean
  devices: EncoderDevice[]
}

/** 偏好的可显示形式（后端 GetEncoderPreferenceInfo）：id = auto | cpu | 设备 id；name = "自动" / "CPU（软件编码）" / 显卡名（设备不可用时也带上保存时记下的名字） */
export interface EncoderPreferenceInfo {
  id: string
  name: string
  available: boolean
  reason?: string
}

/** 偏好值：'auto' | 'cpu' | 设备 id */
export type EncoderPreference = string
export const PREF_AUTO = 'auto'
export const PREF_CPU = 'cpu'

export const encoderIsReal = (): boolean => ENCODER_BACKEND_READY && hasWailsBackend()

/** 设置页是否显示“编码设备”：后端接通（真实）才显示；纯浏览器只有地址带 ?enc= 才显示模拟层 */
export function encoderPanelVisible(): boolean {
  return encoderIsReal() || (!hasWailsBackend() && simParam('enc') !== null)
}

const SERVICE = 'SystemService'

/**
 * 设备排序（与后端 ResolveEncoder 的 auto 选择一致）：cpu 永远第一；显卡里独显在前、集显在后，同级保持后端给的顺序（稳定排序）。
 * 所以下拉里第一张可用显卡就是“自动”会选的那张。后端追加在末尾的“所选设备不存在”占位（available=false）不在下拉里显示，排序不影响它。
 */
export function sortDevices(devices: EncoderDevice[]): EncoderDevice[] {
  const rank = (d: EncoderDevice) => (d.kind !== 'gpu' ? 0 : d.discrete ? 1 : 2)
  return devices.map((d, i) => ({ d, i })).sort((a, b) => rank(a.d) - rank(b.d) || a.i - b.i).map((x) => x.d)
}

/** 偏好信息：缺字段补默认，name 只当字符串 */
export function normalizeInfo(raw: unknown): EncoderPreferenceInfo {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return {
    id: String(r.id || PREF_AUTO),
    name: String(r.name ?? ''),
    available: r.available !== false,
    ...(typeof r.reason === 'string' && r.reason ? { reason: r.reason } : {}),
  }
}

/** 容忍后端返回形状的小偏差：缺字段补默认；保证第一项是 cpu */
export function normalizeList(raw: unknown): EncoderDeviceList {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const arr = Array.isArray(raw) ? raw : Array.isArray(r.devices) ? (r.devices as unknown[]) : []
  const vendors: EncoderVendor[] = ['nvidia', 'intel', 'amd', 'apple', 'unknown']
  const devices: EncoderDevice[] = arr
    .filter((d): d is Record<string, unknown> => !!d && typeof d === 'object')
    .map((d) => ({
      id: String(d.id ?? ''),
      name: String(d.name ?? ''),
      vendor: vendors.includes(d.vendor as EncoderVendor) ? (d.vendor as EncoderVendor) : 'unknown',
      kind: d.kind === 'gpu' ? ('gpu' as const) : ('cpu' as const),
      discrete: d.discrete === true,
      available: d.available !== false,
      ...(typeof d.reason === 'string' && d.reason ? { reason: d.reason } : {}),
    }))
    .filter((d) => d.id)
  if (!devices.some((d) => d.id === PREF_CPU)) devices.unshift({ id: PREF_CPU, name: 'CPU', vendor: 'unknown', kind: 'cpu', discrete: false, available: true })
  return { ffmpegReady: r.ffmpegReady !== false, devices: sortDevices(devices) }
}

// ───────────── 模拟层（?enc=detecting|found|found-open|found-gpu|multi|none|none-open|unavail|fail|noff，与设计稿原型参数一致；multi = 多显卡排序演示）─────────────
/** 开发预览：?encname=long 把模拟的 NVIDIA 显卡改成长名字，看长设备名的截断（仅模拟层） */
const SIM_NV_NAME = simParam('encname') === 'long' ? 'NVIDIA GeForce RTX 4060 Laptop GPU with Max-Q Design' : 'NVIDIA GeForce RTX 4060'
const SIM_NV: EncoderDevice = { id: 'nvidia-0', name: SIM_NV_NAME, vendor: 'nvidia', kind: 'gpu', discrete: true, available: true }
const SIM_INTEL: EncoderDevice = { id: 'intel-0', name: 'Intel UHD Graphics 770', vendor: 'intel', kind: 'gpu', discrete: false, available: true }
const SIM_AMD: EncoderDevice = { id: 'amd-0', name: 'AMD Radeon RX 7600', vendor: 'amd', kind: 'gpu', discrete: true, available: true }
const SIM_CPU: EncoderDevice = { id: 'cpu', name: 'CPU', vendor: 'unknown', kind: 'cpu', discrete: false, available: true }
let simPref: EncoderPreference | null = null

function simScenario(): string {
  const v = simParam('enc') ?? ''
  return v === '' || v === '1' ? 'found' : v
}
function simInitialPref(): EncoderPreference {
  const s = simScenario()
  return s === 'found-gpu' ? SIM_NV.id : s === 'unavail' ? SIM_NV.id : PREF_AUTO
}

/** 模拟层当前场景的设备列表（不含延迟 / 失败；multi 故意把集显放前面，靠 normalizeList 的排序纠正） */
function simDevices(): EncoderDeviceList {
  const s = simScenario()
  if (s === 'noff') return { ffmpegReady: false, devices: [SIM_CPU] }
  if (s === 'none' || s === 'none-open') return { ffmpegReady: true, devices: [SIM_CPU] }
  if (s === 'unavail') return normalizeList({ ffmpegReady: true, devices: [SIM_CPU, { ...SIM_NV, available: false, reason: '试跑编码失败（驱动异常）' }, SIM_INTEL] })
  if (s === 'multi' || s === 'multi-open') return normalizeList({ ffmpegReady: true, devices: [SIM_CPU, SIM_INTEL, SIM_AMD, SIM_NV] })
  return normalizeList({ ffmpegReady: true, devices: [SIM_CPU, SIM_NV, SIM_INTEL] })
}

export async function listEncoderDevices(): Promise<EncoderDeviceList> {
  if (encoderIsReal()) return normalizeList(await callService<unknown>(SERVICE, 'ListEncoderDevices'))
  const s = simScenario()
  if (s === 'detecting') return new Promise(() => undefined) // 一直“检测中”
  await simDelay(500)
  if (s === 'fail') simError('INTERNAL', '读取编码器列表失败')
  return simDevices()
}

export async function getEncoderPreference(): Promise<EncoderPreference> {
  if (encoderIsReal()) return String((await callService<unknown>(SERVICE, 'GetEncoderPreference')) || PREF_AUTO)
  if (simPref === null) simPref = simInitialPref()
  return simPref
}

export async function setEncoderPreference(id: EncoderPreference): Promise<void> {
  if (encoderIsReal()) {
    await callService<unknown>(SERVICE, 'SetEncoderPreference', id)
    return
  }
  await simDelay(150)
  simPref = id
}

/** 重新检测（装了驱动 / 换了显卡后手动刷新）；模拟层等同 listEncoderDevices */
export async function refreshEncoderDevices(): Promise<EncoderDeviceList> {
  if (encoderIsReal()) return normalizeList(await callService<unknown>(SERVICE, 'RefreshEncoderDevices'))
  return listEncoderDevices()
}

/** 偏好 + 显示名 + 当前是否可用，设置页显示“自动 / CPU / 具体显卡名”用；模拟层按列表推出 */
export async function getEncoderPreferenceInfo(): Promise<EncoderPreferenceInfo> {
  if (encoderIsReal()) {
    return normalizeInfo(await callService<unknown>(SERVICE, 'GetEncoderPreferenceInfo'))
  }
  const id = await getEncoderPreference()
  await simDelay(60)
  if (id === PREF_AUTO) return { id, name: '自动', available: true }
  if (id === PREF_CPU) return { id, name: 'CPU（软件编码）', available: true }
  // 与后端一致：设备不可用 / 不存在时仍带保存偏好时记下的名字（模拟层用 SIM 设备名；不在模拟列表里的 id 没记过名字 → ''）
  const d = simDevices().devices.find((x) => x.id === id) ?? [SIM_NV, SIM_INTEL, SIM_AMD].find((x) => x.id === id)
  if (!d) return { id, name: '', available: false, reason: '没有检测到这个设备' }
  const cur = simDevices().devices.find((x) => x.id === id)
  return { id, name: d.name, available: !!cur && cur.available, ...(cur?.reason ? { reason: cur.reason } : !cur ? { reason: '没有检测到这个设备' } : {}) }
}

/** 只给自检用：重置模拟层的偏好 */
export function resetEncoderSim(): void {
  simPref = null
}
