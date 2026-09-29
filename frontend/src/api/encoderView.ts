// 设置页“编码设备”的状态推导（纯函数，api.check.ts 里有自检）。界面不显示编码器名（NVENC / QSV…），后端也不返回。
import type { EncoderDevice, EncoderDeviceList, EncoderPreference, EncoderPreferenceInfo } from '@/api/encoder'
import { PREF_AUTO, PREF_CPU } from '@/api/encoder'
import {
  ENCODER_CPU_NOTE, ENCODER_DETECTING_NOTE, ENCODER_DETECTING_SELECT, ENCODER_FAILED_NOTE, ENCODER_NONE_NOTE, ENCODER_NO_FFMPEG_NOTE,
  ENCODER_OPTION_AUTO, ENCODER_OPTION_CPU, ENCODER_UNAVAILABLE_NOTE, ENCODER_UNKNOWN_SELECTED, encoderAutoNote, encoderGpuNote,
} from '@/errors/encoderMessages'

export type EncoderNoteTone = 'info' | 'ok' | 'warn' | 'err'
export interface EncoderView {
  state: 'detecting' | 'noff' | 'failed' | 'ready'
  /** 下拉框里显示的文字 */
  selectText: string
  /** 下拉禁用（aria-disabled）：检测中、ffmpeg 未就绪 */
  selectDisabled: boolean
  /** 下拉警告描边（所选显卡当前不可用） */
  selectWarn: boolean
  redetectDisabled: boolean
  note: { tone: EncoderNoteTone; text: string; spinner?: boolean; action?: 'resetAuto' }
  /** 下拉里的显卡项（只列可用的）；空 = 没有分隔线和“显卡”小标题 */
  gpus: EncoderDevice[]
  /** 当前选中的选项 key：'auto' | 'cpu' | 设备 id；所选不可用时为空串（菜单里没有对应项） */
  selectedKey: string
}

export interface EncoderInput {
  loading: boolean
  failed: boolean
  list: EncoderDeviceList | null
  pref: EncoderPreference
  /** GetEncoderPreferenceInfo 的结果：所选显卡的显示名（不可用 / 不存在时也是保存时记下的名字）和当前是否可用；没读到时为 null */
  info?: EncoderPreferenceInfo | null
  /** ffmpeg store 是否就绪（未就绪也算 noff） */
  ffmpegReady: boolean
}

export function deriveEncoderView(i: EncoderInput): EncoderView {
  const base = { selectDisabled: false, selectWarn: false, redetectDisabled: false, gpus: [] as EncoderDevice[], selectedKey: PREF_AUTO }
  const prefName = (): string => {
    if (i.pref === PREF_AUTO) return ENCODER_OPTION_AUTO
    if (i.pref === PREF_CPU) return ENCODER_OPTION_CPU
    return (i.info && i.info.id === i.pref && i.info.name) || i.list?.devices.find((d) => d.id === i.pref)?.name || ENCODER_UNKNOWN_SELECTED
  }
  if (!i.ffmpegReady || (i.list && !i.list.ffmpegReady)) {
    return { ...base, state: 'noff', selectText: ENCODER_OPTION_AUTO, selectDisabled: true, redetectDisabled: true, note: { tone: 'warn', text: ENCODER_NO_FFMPEG_NOTE } }
  }
  if (i.loading) {
    return { ...base, state: 'detecting', selectText: ENCODER_DETECTING_SELECT, selectDisabled: true, redetectDisabled: true, note: { tone: 'info', text: ENCODER_DETECTING_NOTE, spinner: true } }
  }
  if (i.failed || !i.list) {
    return { ...base, state: 'failed', selectText: prefName(), selectedKey: i.pref, note: { tone: 'err', text: ENCODER_FAILED_NOTE } }
  }
  const gpus = i.list.devices.filter((d) => d.kind === 'gpu' && d.available)
  const ready = { ...base, state: 'ready' as const, gpus }
  if (i.pref === PREF_AUTO) {
    return { ...ready, selectText: ENCODER_OPTION_AUTO, selectedKey: PREF_AUTO, note: { tone: 'info', text: gpus.length ? encoderAutoNote(gpus.length, gpus[0].name) : ENCODER_NONE_NOTE } }
  }
  if (i.pref === PREF_CPU) {
    return { ...ready, selectText: ENCODER_OPTION_CPU, selectedKey: PREF_CPU, note: { tone: 'info', text: ENCODER_CPU_NOTE } }
  }
  const dev = i.list.devices.find((d) => d.id === i.pref)
  // 列表和偏好信息都说可用才算可用（任何一边说不可用 / 不存在都走警告）
  const infoOk = !i.info || i.info.id !== i.pref || i.info.available
  if (dev && dev.available && infoOk) return { ...ready, selectText: dev.name, selectedKey: dev.id, note: { tone: 'ok', text: encoderGpuNote(dev.name) } }
  // 所选显卡当前不可用：偏好保持原值，不阻止转码（转码会改用 CPU），只用警告色
  return { ...ready, selectText: prefName(), selectedKey: '', selectWarn: true, note: { tone: 'warn', text: ENCODER_UNAVAILABLE_NOTE, action: 'resetAuto' } }
}

/**
 * 事件序号：每次发起请求 next() 拿一个号，返回时 isCurrent(号) 为 false 说明期间又发起过新请求，这次结果作废（丢弃）。
 * 设置页面板用两组：一组管“设备列表 + 检测中”，一组管“偏好 + 偏好信息”（沿用 ffmpeg store 的做法，自检见 api.check.ts）。
 */
export function createSeq(): { next: () => number; isCurrent: (n: number) => boolean } {
  let n = 0
  return { next: () => ++n, isCurrent: (t) => t === n }
}
