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
  desc?: string
}

export interface CatOptionList {
  options: CatOption[]
  /** 默认选中的 id；强度为空串 = 「默认」（不传强度） */
  defaultId: string
}

export interface CatAgentCapabilities {
  modes: CatOptionList
  efforts: CatOptionList
}

/** 后端（或走查假数据）给到的原始能力；没给的字段走兜底 */
export interface CatBackendCaps {
  modes?: CatOptionList
  thinks?: CatThinkLevel[]
}

/** 今天写死的批准方式（三家相同）——契约 v0.44 落地前的兜底 */
export const FALLBACK_MODES: CatOptionList = {
  options: CAT_ACCESS.filter((a) => a.enabled).map((a) => ({ id: a.id, label: a.name, desc: a.desc })),
  defaultId: 'approve',
}

const backend = reactive<Record<string, CatBackendCaps>>({})

/** 拉到某家能力后写进来（refreshCapabilities / 设置页 loadCaps 调用） */
export function setBackendCaps(agentKind: string, caps: CatBackendCaps) {
  backend[agentKind] = { ...backend[agentKind], ...caps }
}

/** 纯函数：给定原始能力算出两份列表（单测 / 非响应式场景用） */
export function resolveCapabilities(caps: CatBackendCaps | undefined): CatAgentCapabilities {
  const modes = caps?.modes && caps.modes.options.length ? caps.modes : FALLBACK_MODES
  const efforts: CatOptionList = {
    options: (caps?.thinks ?? []).map((t) => ({ id: t.id, label: t.displayName })),
    defaultId: '',
  }
  return { modes, efforts }
}

/** 组件用：按 agentKind 拿模式 / 强度列表（响应式） */
export function useCatAgentCapabilities(agentKind: MaybeRefOrGetter<string>) {
  return computed(() => resolveCapabilities(backend[toValue(agentKind)]))
}

/** 模式在输入框胶囊上要图标 / 短名：已知 id 沿用 CAT_ACCESS，未知 id 用通用图标 + label */
export function modeMeta(opt: CatOption): Pick<CatAccess, 'icon' | 'short' | 'name' | 'desc'> & { id: string } {
  const known = CAT_ACCESS.find((a) => a.id === opt.id)
  return {
    id: opt.id,
    name: opt.label,
    short: known && known.name === opt.label ? known.short : opt.label,
    desc: opt.desc ?? '',
    icon: known?.icon ?? 'hand',
  }
}
