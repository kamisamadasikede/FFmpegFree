// 任务的编码设备信息（契约 v0.18 §9.7 / v0.19）：事件字段合并、“要不要显示回退提示 / 用了哪个设备”的纯函数。api.check.ts 里有自检。
// 界面只显示设备名（“NVIDIA GeForce RTX 4060” / “CPU”），不显示 encoder（h264_nvenc / libx264 …）和设备 id。
import { encoderPanelVisible, listEncoderDevices, type EncoderDevice } from '@/api/encoder'
import { ENCODER_DEVICE_CPU_FALLBACK_NAME, ENCODER_DEVICE_CPU_NAME, ENCODER_DEVICE_GPU_FALLBACK_NAME } from '@/errors/encoderMessages'
import { ref } from 'vue'

/** 任务上的四个编码器字段（都可缺省，见契约 omitempty） */
export interface EncoderFields {
  encoder?: string
  encoderDevice?: string
  hwFallback?: boolean
  hwFallbackReason?: string
}

/** 取出四个字段里有值的（用来复制到终态快照等） */
export function pickEncoderFields(t: EncoderFields): EncoderFields {
  return {
    ...(t.encoder ? { encoder: t.encoder } : {}),
    ...(t.encoderDevice ? { encoderDevice: t.encoderDevice } : {}),
    ...(t.hwFallback ? { hwFallback: true } : {}),
    ...(t.hwFallbackReason ? { hwFallbackReason: t.hwFallbackReason } : {}),
  }
}

/**
 * 把事件里的四个字段合并进任务，**逐个字段**处理，缺省不覆盖：
 *  - encoder / encoderDevice / hwFallbackReason：事件里有非空值才覆盖（回退补发的 running 事件会把它们改成 CPU 编码器 + "cpu"）。
 *  - hwFallback：后端 omitempty，false 不会出现在事件里；只有 true 才写（一个任务一旦回退就不会“取消回退”），缺省 / false 不改已有值。
 */
export function mergeEncoderFields(cur: EncoderFields, p: EncoderFields): void {
  if (p.encoder) cur.encoder = p.encoder
  if (p.encoderDevice) cur.encoderDevice = p.encoderDevice
  if (p.hwFallback === true) cur.hwFallback = true
  if (p.hwFallbackReason) cur.hwFallbackReason = p.hwFallbackReason
}

/** 编码设备相关界面整体是否启用：真实后端接通（ENCODER_BACKEND_READY 且在 Wails 里）或纯浏览器的 ?enc= 预览。标志为 false 时正式包里整体不显示。 */
export const encoderTaskUiEnabled = (): boolean => encoderPanelVisible()

export interface TaskEncoderInput extends EncoderFields {
  /** 任务开始时间（ms）；0 / 缺省 = 从未运行过（排队中取消、退出时还在排队；契约 v0.19：这类任务四个字段仍保留提交时解析的值） */
  startedAt?: number
}

/** 任务是否真的开始过编码：只看 startedAt */
export const taskEverRan = (t: TaskEncoderInput): boolean => (t.startedAt ?? 0) > 0

/** 是否显示“硬件编码失败，已自动改用 CPU”提示：功能启用 + 真的运行过 + hwFallback=true。-c copy / 两遍编码 / 超 4096 走 CPU 等后端本来就不置 hwFallback，这里不再猜 */
export function showFallbackNotice(t: TaskEncoderInput | null | undefined, enabled: boolean = encoderTaskUiEnabled()): boolean {
  return !!t && enabled && taskEverRan(t) && t.hwFallback === true
}

/** 直播页提示条：任务列表里有没有“真的运行过（startedAt>0）且 hwFallback”的直播任务（live_file_push / live_screen_push）。startedAt 为 0 / 缺失、功能没启用都不显示 */
export function liveFallbackShown(tasks: readonly (TaskEncoderInput & { type: string })[], enabled: boolean = encoderTaskUiEnabled()): boolean {
  return tasks.some((t) => t.type.startsWith('live_') && showFallbackNotice(t, enabled))
}

/** 设备显示名：cpu → “CPU”；显卡 → 列表里的 name；查不到（设备已不存在、列表没读到）→ “显卡”。永远不显示 id */
export function deviceDisplayName(id: string, devices: readonly EncoderDevice[] | null | undefined): string {
  if (id === 'cpu') return ENCODER_DEVICE_CPU_NAME
  const d = devices?.find((x) => x.id === id)
  return (d && d.name) || ENCODER_DEVICE_GPU_FALLBACK_NAME
}

/**
 * “使用的设备”文字；不显示时返回 ''：功能没启用、任务没运行过、没有编码器信息、或 -c copy（encoder="copy"，没有设备）。
 */
export function usedDeviceText(t: TaskEncoderInput | null | undefined, devices: readonly EncoderDevice[] | null | undefined, enabled: boolean = encoderTaskUiEnabled()): string {
  if (!t || !enabled || !taskEverRan(t) || !t.encoder || t.encoder === 'copy' || !t.encoderDevice) return ''
  if (t.hwFallback === true && t.encoderDevice === 'cpu') return ENCODER_DEVICE_CPU_FALLBACK_NAME // 显卡编码失败、已回退：“CPU（已回退）”
  return deviceDisplayName(t.encoderDevice, devices)
}

// ---- 设备列表缓存（只为了把 encoderDevice 的 id 换成名字；读不到就退化，不报错）----
const devices = ref<EncoderDevice[] | null>(null)
let loading: Promise<void> | null = null
/** 已读到的设备列表（响应式；未读到 = null）。第一次调用时才去读一次（只在功能启用时） */
export function useEncoderDeviceList() {
  if (devices.value === null && !loading && encoderTaskUiEnabled()) {
    loading = listEncoderDevices()
      .then((l) => { devices.value = l.devices })
      .catch(() => { devices.value = [] })
      .finally(() => { loading = null })
  }
  return devices
}
/** 只给自检用 */
export function resetEncoderDeviceCache(): void {
  devices.value = null
  loading = null
}

// ---- “编码设置”链接：跳到设置页并定位到“编码设备”----
/** 设置页里“编码设备”分组的锚点 id 和路由 query（?section=encoder） */
export const ENCODER_SECTION_ID = 'sec-encoder'
export const ENCODER_SECTION_QUERY = 'encoder'
/** 提示条“编码设置”的跳转目标；设置页读 query.section 后滚动并聚焦该分组（分组只在 encoderPanelVisible() 时存在，不存在就停在页顶） */
export const encoderSettingsLocation = () => ({ path: '/settings/general', query: { section: ENCODER_SECTION_QUERY } })
