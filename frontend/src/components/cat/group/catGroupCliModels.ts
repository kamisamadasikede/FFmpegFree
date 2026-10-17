/**
 * 群成员 CLI → 模型列表：与设置页 loadCaps / 底栏 refreshCapabilities 同源。
 * - 正式：listCatModels(agentKind) → BE ListCatModels（真实 id，如 opus/sonnet）
 * - 自定义开且空列表时再 listCatProviderModels
 * - 走查：?cat_sim=caps|codexcaps → simAgentRaw；无 Wails 的 groupScene 建群同
 * - 默认模型：defaultsByAgent[kind].model（在列表里才用），否则空 = 「默认模型」/ 跟随 CLI
 */
import { reactive } from 'vue'
import { listCatModels, type CatModel } from '@/api/cat'
import { isProviderKind, type CatProviderKind } from '@/api/catAgents'
import { listCatProviderModels } from '@/api/catProviderModels'
import { groupScene } from '@/api/catGroupChatMock'
import { catSim, simAgentRaw } from '@/views/cat/catDevSim'
import { customProviderOn } from '@/views/cat/catState'
import { catPanels } from '@/views/cat/catPanels'

const cache = reactive<Partial<Record<CatProviderKind, CatModel[]>>>({})
const inflight = new Map<string, Promise<CatModel[]>>()

export function modelsOf(kind: string): CatModel[] {
  return (isProviderKind(kind) ? cache[kind] : undefined) ?? []
}

/**
 * 与设置页 modelValue / Claude「跟随 CLI」一致：
 * 有 defaultsByAgent 且仍在当前列表 → 用该 id；否则空（「默认模型」）。
 */
export function defaultModelId(kind: string, list: readonly CatModel[] = modelsOf(kind)): string {
  if (!isProviderKind(kind)) return ''
  const want = catPanels.settings.defaultsByAgent?.[kind]?.model ?? ''
  return want && list.some((m) => m.id === want) ? want : ''
}

export async function loadCliModels(kind: string): Promise<CatModel[]> {
  if (!isProviderKind(kind)) return []
  const hit = cache[kind]
  if (hit) return hit
  const pending = inflight.get(kind)
  if (pending) return pending

  const job = (async () => {
    const useSim = import.meta.env.DEV && (catSim.has('caps') || catSim.has('codexcaps'))
    let models: CatModel[] = useSim
      ? (simAgentRaw(kind).models as CatModel[])
      : await listCatModels(kind)
    if (!useSim && !models.length && customProviderOn(kind)) {
      const pm = await listCatProviderModels(kind, false)
      if (pm.state === 'ok' && pm.models.length) models = pm.models
    }
    // 浏览器走查建群无 Wails：listCatModels 恒空，用与 caps 同源模拟
    if (!useSim && !models.length && import.meta.env.DEV && groupScene()) {
      models = simAgentRaw(kind).models as CatModel[]
    }
    cache[kind] = models
    return models
  })()

  inflight.set(kind, job)
  try {
    return await job
  } finally {
    inflight.delete(kind)
  }
}

/** 强制重拉（CLI 切换后调用，清掉该家缓存再取） */
export async function refreshCliModels(kind: string): Promise<CatModel[]> {
  if (isProviderKind(kind)) delete cache[kind]
  return loadCliModels(kind)
}

export function preloadAllCliModels(kinds: readonly string[]): void {
  for (const k of kinds) void loadCliModels(k)
}
