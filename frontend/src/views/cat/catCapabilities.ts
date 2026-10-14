/**
 * Cat 每家 CLI（agentKind）的「对话模式」和「思考强度」选项，统一从这里取。
 *
 * 形状先按架构师口径定：列表 `{ id, label, desc? }[]` + 默认 id。列表为空 → 界面隐藏对应下拉。
 * 契约 v0.44（后端按 agentKind 下发模式 / 强度列表）还没写：
 *   - 强度：仍用现有 ListCatThinkLevels(agentKind) 的结果（setBackendCaps 写进来）；
 *   - 模式：后端还没下发，先用今天写死的 CAT_ACCESS 兜底，三家一样（与现状一致）。
 * 契约合了以后只改 setBackendCaps 的来源，组件不用动。
 */
import { computed, reactive, toValue, type MaybeRefOrGetter } from 'vue'
import { CAT_ACCESS, type CatAccess } from '@/api/catMock'
import type { CatThinkLevel } from '@/api/cat'

export interface CatOption {
  id: string
  label: string
  /** 空 → 不显示说明 */
  desc?: string
  /** 仅模式：这一档会写文件（后端 writesFiles） */
  writesFiles?: boolean
  /** 仅模式：底部胶囊上的短名（2–4 字）；空 → 用 label */
  shortName?: string
  /** 仅模式：橙色警示样式只看这个（= risk 为 'danger'），不按 id 猜 */
  risky?: boolean
  /** 仅模式：契约 risk 三档；缺省 undefined */
  risk?: CatModeRisk
}

/** 模式菜单最多几项（设计：不出滚动条）。Codex 是合并好的组合模式，不另拆沙箱 / 批准两个下拉 */
export const MODE_MENU_MAX = 5

export type CatModeRisk = 'safe' | 'edits' | 'danger'
const asRisk = (v: unknown): CatModeRisk | undefined => (v === 'safe' || v === 'edits' || v === 'danger' ? v : undefined)

export interface CatOptionList {
  /** 模式：按保守 → 放开排序（后端顺序原样） */
  options: CatOption[]
  /** 默认选中的 id；强度为空串 = 「默认」（不传强度） */
  defaultId: string
}

export interface CatAgentCapabilities {
  modes: CatOptionList
  /** 当前所选模型的强度；空列表 → 隐藏强度下拉 */
  efforts: CatOptionList
}

/** 前端内部形状（与后端字段名解耦） */
export interface CatBackendCaps {
  modes?: CatOptionList
  /** 按模型 id 的强度列表；没有该模型时退回 thinks */
  effortsByModel?: Record<string, CatOptionList>
  /** 现有 ListCatThinkLevels（按 agentKind，不分模型）——v0.44 前的来源 */
  thinks?: CatThinkLevel[]
}

/**
 * 契约 v0.44（#255，6.19.18.2）字段名 → 前端形状，唯一一处映射。契约改名只改这里。
 * - 模式：GetCatStatus/RecheckCat 的 agents[].modes[] = { id, label, description, writesFiles, isDefault }
 *   shortLabel / risk('safe'|'edits'|'danger') 可选：缺省 shortLabel → label、无橙色；只有 risk='danger' 才橙色。
 * - 强度：ListCatModels 每项 { id, efforts: string[], defaultEffort }
 */
export const V044_FIELDS = {
  modes: 'modes',
  modeId: 'id',
  modeName: 'label',
  modeDesc: 'description',
  modeWritesFiles: 'writesFiles',
  modeIsDefault: 'isDefault',
  modeShortName: 'shortLabel',
  modeRisk: 'risk',
  modelId: 'id',
  efforts: 'efforts',
  defaultEffort: 'defaultEffort',
} as const

/** 档位显示名（产品 10-14 定稿，三家通用）；不认识的 id 原样显示 */
export const EFFORT_LABELS: Record<string, string> = { low: '低', medium: '中', high: '高', xhigh: '很高', max: '最高', ultra: '极限' }
export const effortLabel = (id: string) => EFFORT_LABELS[id] ?? id

type Raw = Record<string, unknown>
const obj = (v: unknown): Raw => (v && typeof v === 'object' ? (v as Raw) : {})
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : [])
const str = (v: unknown) => (typeof v === 'string' ? v : '')

/** agents[] 里一家的原始状态 → 模式列表；没有 modes → undefined（走兜底）。无 isDefault → 预选第一项（最保守） */
export function mapAgentModes(agentStatus: unknown): CatOptionList | undefined {
  const F = V044_FIELDS
  const modes = arr(obj(agentStatus)[F.modes]).map(obj).filter((m) => str(m[F.modeId]))
  if (!modes.length) return undefined
  const opts: CatOption[] = modes.slice(0, MODE_MENU_MAX).map((m) => ({
    id: str(m[F.modeId]),
    label: str(m[F.modeName]) || str(m[F.modeId]),
    desc: str(m[F.modeDesc]) || undefined,
    writesFiles: m[F.modeWritesFiles] === true,
    shortName: str(m[F.modeShortName]) || undefined,
    risky: str(m[F.modeRisk]) === 'danger',
    risk: asRisk(m[F.modeRisk]),
  }))
  const def = modes.find((m) => m[F.modeIsDefault] === true)
  return { options: opts, defaultId: def ? str(def[F.modeId]) : opts[0].id }
}

/** ListCatModels 原始返回 → 按模型的强度表；模型没带 efforts 字段（旧后端）则不收录，走 agent 级兜底 */
export function mapModelEfforts(rawModels: unknown): Record<string, CatOptionList> | undefined {
  const F = V044_FIELDS
  const out: Record<string, CatOptionList> = {}
  let any = false
  for (const m of arr(rawModels).map(obj)) {
    const id = str(m[F.modelId])
    if (!id || !Array.isArray(m[F.efforts])) continue
    any = true
    const options = arr(m[F.efforts]).map(str).filter(Boolean).map((e) => ({ id: e, label: effortLabel(e) }))
    const d = str(m[F.defaultEffort])
    out[id] = { options, defaultId: options.some((o) => o.id === d) ? d : '' }
  }
  return any ? out : undefined
}

/** 兼容：同时带 modes 和 models 的一份原始数据（走查 mock 用） */
export function mapBackendCaps(raw: unknown): CatBackendCaps {
  const r = obj(raw)
  const out: CatBackendCaps = {}
  const modes = mapAgentModes(r)
  if (modes) out.modes = modes
  const eff = mapModelEfforts(r.models)
  if (eff) out.effortsByModel = eff
  return out
}

/** 今天写死的批准方式（三家相同）——契约 v0.44 落地前的兜底 */
export const FALLBACK_MODES: CatOptionList = {
  options: CAT_ACCESS.filter((a) => a.enabled).map((a) => ({ id: a.id, label: a.name, desc: a.desc, writesFiles: a.id !== 'plan', shortName: a.short, risky: a.id === 'approve', risk: (a.id === 'approve' ? 'danger' : a.id === 'plan' ? 'safe' : 'edits') as CatModeRisk })),
  defaultId: 'approve',
}

const backend = reactive<Record<string, CatBackendCaps>>({})

/** 拉到某家能力后写进来（refreshCapabilities / 设置页 loadCaps 调用） */
export function setBackendCaps(agentKind: string, caps: CatBackendCaps) {
  backend[agentKind] = { ...backend[agentKind], ...caps }
}

/** 纯函数：模式按 agentKind，强度按所选模型（没有按模型的数据时退回 agent 级 thinks） */
export function resolveCapabilities(caps: CatBackendCaps | undefined, modelId = ''): CatAgentCapabilities {
  const modes = caps?.modes && caps.modes.options.length ? { ...caps.modes, options: caps.modes.options.slice(0, MODE_MENU_MAX) } : FALLBACK_MODES
  const perModel = caps?.effortsByModel?.[modelId]
  const efforts: CatOptionList = perModel ?? {
    options: (caps?.thinks ?? []).map((t) => ({ id: t.id, label: EFFORT_LABELS[t.id] ?? t.displayName ?? t.id })),
    defaultId: '',
  }
  return { modes, efforts }
}

/** 当前选的值不在列表里 → 用列表默认（强度默认为空 = 「默认」） */
export function pickSupported(list: CatOptionList, current: string): string {
  return list.options.some((o) => o.id === current) ? current : list.defaultId
}

/** 组件用：按 agentKind（+ 所选模型）拿模式 / 强度列表（响应式，模型变了强度跟着重算） */
export function useCatAgentCapabilities(agentKind: MaybeRefOrGetter<string>, modelId: MaybeRefOrGetter<string> = '') {
  return computed(() => resolveCapabilities(backend[toValue(agentKind)], toValue(modelId)))
}

export function getCatAgentCapabilities(agentKind: string, modelId = ''): CatAgentCapabilities {
  return resolveCapabilities(backend[agentKind], modelId)
}

/** 模式在输入框胶囊上要图标 / 短名：已知 id 沿用 CAT_ACCESS，未知 id 用通用图标 + label */
export function modeMeta(opt: CatOption): Pick<CatAccess, 'icon' | 'short' | 'name'> & { id: string; desc: string; writesFiles: boolean; risky: boolean } {
  const known = CAT_ACCESS.find((a) => a.id === opt.id)
  return {
    id: opt.id,
    name: opt.label,
    short: opt.shortName || opt.label,
    risky: opt.risky === true,
    desc: opt.desc ?? '',
    writesFiles: opt.writesFiles ?? true,
    icon: known?.icon ?? 'hand',
  }
}
