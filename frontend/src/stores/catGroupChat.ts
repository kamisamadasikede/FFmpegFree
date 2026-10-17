/**
 * 项目群聊 store（契约 §6.21.12 / 6.21.14 ④）。
 * 群消息流只吃 cat:group-message + cat:group-route；输入框上方的「正在干活」条吃 cat:group-member。
 */
import { defineStore } from 'pinia'
import { computed, reactive, ref } from 'vue'
import { groupApi, groupRealApi, normalizeMember, normalizeMessage, type CatGroup, type CatGroupMember, type CatGroupMessage, type CatGroupRunning, type CatRouteDetail, type RevertResult } from '@/api/catGroupChat'
import { groupMock, groupScene } from '@/api/catGroupChatMock'
import { onEvent } from '@/services/wails'

export interface LiveRun extends CatGroupRunning { receivedAt: number }

export const useCatGroupChat = defineStore('catGroupChat', () => {
  const groupsByProject = reactive<Record<string, CatGroup[]>>({})
  const activeId = ref('')
  const group = ref<CatGroup | null>(null)
  const members = ref<CatGroupMember[]>([])
  const messages = ref<CatGroupMessage[]>([])
  const hasMore = ref(false)
  const loading = ref(false)
  const live = reactive<Record<string, LiveRun>>({})
  const routing = reactive<Record<string, boolean>>({})
  const drawerMemberId = ref('')
  const processText = reactive<Record<string, string>>({})
  const settingsOpen = ref(false)
  const membersOpen = ref(false)
  const scene = groupScene()
  const now = ref(Date.now())
  let timer: ReturnType<typeof setInterval> | null = null
  let subscribed = false

  const membersSorted = computed(() => [...members.value].sort((a, b) => a.order - b.order))
  const memberById = (id?: string) => members.value.find((m) => m.id === id)
  const nameOf = (id?: string) => memberById(id)?.name ?? ''
  const workingMembers = computed(() => membersSorted.value.filter((m) => live[m.id]))
  const routingNow = computed(() => Object.values(routing).some(Boolean))
  function elapsedOf(id: string): number {
    const r = live[id]
    if (!r) return 0
    return scene ? r.elapsedMs : r.elapsedMs + Math.max(0, now.value - r.receivedAt)
  }
  function ensureTicker() {
    if (scene || timer) return
    timer = setInterval(() => (now.value = Date.now()), 1000)
  }
  function upsertMember(m: CatGroupMember) {
    const i = members.value.findIndex((x) => x.id === m.id)
    if (i >= 0) members.value.splice(i, 1, m)
    else members.value.push(m)
  }
  function addMessage(m: CatGroupMessage) {
    if (messages.value.some((x) => x.seq === m.seq)) return
    messages.value.push(m)
    messages.value.sort((a, b) => a.seq - b.seq)
  }

  function subscribe() {
    if (subscribed) return
    subscribed = true
    const api = groupApi()
    api.onMessage((raw) => {
      const m = normalizeMessage(raw)
      if (m.groupId !== activeId.value) return
      addMessage(m)
      if (m.chainId && m.systemKind === 'route') routing[m.chainId] = false
    })
    api.onRoute((e) => {
      if (e.groupId !== activeId.value) return
      routing[e.chainId] = e.state === 'routing'
    })
    api.onMember((e) => {
      if (e.groupId !== activeId.value) return
      if (e.removed) members.value = members.value.filter((x) => x.id !== e.member.id)
      else upsertMember(normalizeMember(e.member))
      if (e.running) live[e.member.id] = { ...e.running, receivedAt: Date.now() }
      else delete live[e.member.id]
    })
    // 「查看过程」：成员自己的会话流（convId=group:<g>:<m>），抽屉里只做文字预览
    onEvent<{ convId?: string; text?: string; delta?: string }>('cat:message', (p) => {
      const cid = p?.convId ?? ''
      if (!cid.startsWith('group:')) return
      const mid = cid.split(':')[2] ?? ''
      processText[mid] = ((processText[mid] ?? '') + (p.delta ?? p.text ?? '')).slice(-6000)
    })
  }

  async function loadGroups(projectId: string) {
    try {
      groupsByProject[projectId] = await groupApi().listGroups(projectId)
    } catch {
      groupsByProject[projectId] = groupsByProject[projectId] ?? []
    }
    return groupsByProject[projectId]
  }

  async function open(groupId: string) {
    subscribe()
    ensureTicker()
    activeId.value = groupId
    loading.value = true
    for (const key of Object.keys(live)) delete live[key]
    try {
      const api = groupApi()
      const [{ group: g, members: ms }, page] = await Promise.all([api.getGroup(groupId), api.listMessages(groupId, 0, 50)])
      if (activeId.value !== groupId) return
      group.value = g
      members.value = ms.map(normalizeMember)
      messages.value = page.messages.map(normalizeMessage)
      hasMore.value = page.hasMore
      if (!groupRealApi()) for (const e of groupMock.sceneRunning()) if (e.running) live[e.member.id] = { ...e.running, receivedAt: Date.now() }
    } finally {
      loading.value = false
    }
  }

  async function loadOlder() {
    if (!hasMore.value || !messages.value.length) return
    const page = await groupApi().listMessages(activeId.value, messages.value[0].seq, 50)
    messages.value = [...page.messages.map(normalizeMessage), ...messages.value]
    hasMore.value = page.hasMore
  }
  async function createGroup(projectId: string, name: string) {
    const g = await groupApi().createGroup(projectId, name)
    ;(groupsByProject[projectId] ??= []).push(g)
    return g
  }
  async function rename(name: string) {
    if (!group.value) return
    const g = await groupApi().renameGroup(group.value.id, name)
    group.value = g
    const list = groupsByProject[g.projectId]
    const i = list?.findIndex((x) => x.id === g.id) ?? -1
    if (list && i >= 0) list.splice(i, 1, g)
  }
  async function setAutoRoute(on: boolean) {
    if (!group.value) return
    group.value = await groupApi().setAutoRoute(group.value.id, on)
  }
  async function deleteGroup() {
    const g = group.value
    if (!g) return
    await groupApi().deleteGroup(g.id)
    groupsByProject[g.projectId] = (groupsByProject[g.projectId] ?? []).filter((x) => x.id !== g.id)
    group.value = null
    activeId.value = ''
  }
  async function addMember(req: { name: string; title: string; agentKind: string; modelId: string; thinkLevelId?: string }) {
    const m = normalizeMember(await groupApi().addMember({ groupId: activeId.value, ...req }))
    upsertMember(m)
    return m
  }
  async function updateMember(req: { memberId: string; name?: string; title?: string; agentKind?: string; modelId?: string }) {
    upsertMember(normalizeMember(await groupApi().updateMember(req)))
  }
  async function removeMember(memberId: string) {
    await groupApi().removeMember(memberId)
    members.value = members.value.filter((m) => m.id !== memberId)
  }
  async function reorder(ids: string[]) {
    const prev = members.value
    members.value = ids.map((id, order) => ({ ...memberById(id)!, order }))
    try {
      members.value = (await groupApi().reorderMembers(activeId.value, ids)).map(normalizeMember)
    } catch (e) {
      members.value = prev
      throw e
    }
  }
  async function regenerate(memberId: string) {
    upsertMember(normalizeMember(await groupApi().regenerateCharter(memberId)))
  }
  async function saveCharter(memberId: string, patch: { owns?: string; notTouch?: string; dirs: string[] }) {
    upsertMember(normalizeMember(await groupApi().updateCharter({ memberId, ...patch })))
  }
  async function send(text: string, mentions: string[]) {
    const r = await groupApi().send(activeId.value, text, mentions)
    addMessage(normalizeMessage(r.userMessage))
    return r.chainId
  }
  async function reassign(chainId: string, toMemberId: string) {
    const r = await groupApi().reassign(activeId.value, chainId, toMemberId)
    addMessage(normalizeMessage(r.system))
  }
  const stop = (memberId: string) => groupApi().stopMember(memberId)
  const stopAll = () => groupApi().stopGroup(activeId.value)
  const retryTurn = (memberId: string, chainId: string) => groupApi().retry(activeId.value, memberId, chainId)
  const revert = (memberId: string, turnId: string, files: string[]): Promise<RevertResult> => groupApi().revert(memberId, turnId, files)
  const routeDetail = (seq: number): Promise<CatRouteDetail> => groupApi().routeDetail(activeId.value, seq)

  return {
    groupsByProject, activeId, group, members, membersSorted, messages, hasMore, loading, live, routing, routingNow,
    drawerMemberId, processText, settingsOpen, membersOpen, scene, workingMembers,
    memberById, nameOf, elapsedOf, loadGroups, open, loadOlder, createGroup, rename, setAutoRoute, deleteGroup,
    addMember, updateMember, removeMember, reorder, regenerate, saveCharter, send, reassign, stop, stopAll, retryTurn, revert, routeDetail,
  }
})
