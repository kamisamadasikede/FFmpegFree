/**
 * 编码设备（GPU 加速）接口封装。后端字段以架构师 / 后端最新说明为准（契约文档随后端 PR 同步，最终以后端 PR 为准）：
 *   SystemService.ListEncoderDevices() → { ffmpegReady, devices[] }；第一项永远是 id = "cpu" 的 CPU
 *   SystemService.GetEncoderPreference() → "auto" | "cpu" | <设备 id>，默认 "auto"
 *   SystemService.SetEncoderPreference(id)
 * 所选设备不可用时偏好保持原值，列表里该设备 available=false 并给 reason。返回里没有编码器名，界面不显示 NVENC 这类名字。
 * 注意：`ffmpegReady` 放在列表的哪一层（对象的字段 / 单独返回值）后端 PR 里才最终确定，这里按 `{ ffmpegReady, devices }` 对象处理，见 normalizeList。
 *
 * 开关 ENCODER_BACKEND_READY（flags.ts，默认 false）：
 *   - false 且在 Wails 里：设置页完全不显示“编码设备”。
 *   - false 且纯浏览器：默认也不显示；地址加 `?enc=` 才显示模拟层（产品经理要求：模拟的显卡名不给正式用户看）。
 *   - true 且在 Wails 里：调用真实绑定（按名字取 window.go，绑定生成后可改成直接 import wailsjs）。
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
  /** available=false 时的原因（后端给的，可能偏技术，界面不直接显示） */
  reason?: string
}

export interface EncoderDeviceList {
  ffmpegReady: boolean
  devices: EncoderDevice[]
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
      available: d.available !== false,
      ...(typeof d.reason === 'string' && d.reason ? { reason: d.reason } : {}),
    }))
    .filter((d) => d.id)
  if (!devices.some((d) => d.id === PREF_CPU)) devices.unshift({ id: PREF_CPU, name: 'CPU', vendor: 'unknown', kind: 'cpu', available: true })
  return { ffmpegReady: r.ffmpegReady !== false, devices }
}

// ───────────── 模拟层（?enc=detecting|found|found-open|found-gpu|none|none-open|unavail|fail|noff，与设计稿原型参数一致）─────────────
const SIM_NV: EncoderDevice = { id: 'nvidia-0', name: 'NVIDIA GeForce RTX 4060', vendor: 'nvidia', kind: 'gpu', available: true }
const SIM_INTEL: EncoderDevice = { id: 'intel-0', name: 'Intel UHD Graphics 770', vendor: 'intel', kind: 'gpu', available: true }
const SIM_CPU: EncoderDevice = { id: 'cpu', name: 'CPU', vendor: 'unknown', kind: 'cpu', available: true }
let simPref: EncoderPreference | null = null

function simScenario(): string {
  const v = simParam('enc') ?? ''
  return v === '' || v === '1' ? 'found' : v
}
function simInitialPref(): EncoderPreference {
  const s = simScenario()
  return s === 'found-gpu' ? SIM_NV.id : s === 'unavail' ? SIM_NV.id : PREF_AUTO
}

export async function listEncoderDevices(): Promise<EncoderDeviceList> {
  if (encoderIsReal()) return normalizeList(await callService<unknown>(SERVICE, 'ListEncoderDevices'))
  const s = simScenario()
  if (s === 'detecting') return new Promise(() => undefined) // 一直“检测中”
  await simDelay(500)
  if (s === 'fail') simError('INTERNAL', '读取编码器列表失败')
  if (s === 'noff') return { ffmpegReady: false, devices: [SIM_CPU] }
  if (s === 'none' || s === 'none-open') return { ffmpegReady: true, devices: [SIM_CPU] }
  if (s === 'unavail') return { ffmpegReady: true, devices: [SIM_CPU, { ...SIM_NV, available: false, reason: '试跑编码失败（驱动异常）' }, SIM_INTEL] }
  return { ffmpegReady: true, devices: [SIM_CPU, SIM_NV, SIM_INTEL] }
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

/** 只给自检用：重置模拟层的偏好 */
export function resetEncoderSim(): void {
  simPref = null
}
