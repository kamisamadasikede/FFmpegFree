/**
 * 项目群聊 store（契约 §6.21.12 / 6.21.14 ④）。
 * 群消息流只吃 cat:group-message + cat:group-route；输入框上方的「正在干活」条吃 cat:group-member。
 */
import { defineStore } from 'pinia'
import { computed, reactive, ref } from 'vue'
import { groupApi, groupRealApi, normalizeMember, normalizeMessage, type CatGroup, type CatGroupMember, type CatGroupMessage, type CatGroupQueuedTask, type CatGroupFile, type CatGroupRunning, type CatRouteDetail, type RevertResult } from '@/api/catGroupChat'
import { groupMock, groupScene } from '@/api/catGroupChatMock'
import { getCatConversation, onCatMessage, onCatTurn } from '@/api/cat'
import { applyStreamEvent, newStreamMsg, type CatStreamEvent, type CatStreamMsg } from '@/api/catStream'
import { demoProcessBlocks, groupIdFromGroupConv, memberIdFromGroupConv, type ProcessBlock } from '@/stores/catGroupProcess'

export type { ProcessBlock } from '@/stores/catGroupProcess'
export { demoProcessBlocks } from '@/stores/catGroupProcess'

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
  /** 成员本轮过程块（live 订阅 cat:message；打开抽屉时若空再拉 GetCatConversation） */
  const processBlocks = reactive<Record<string, ProcessBlock[]>>({})
  const processLoading = reactive<Record<string, boolean>>({})
  /** @deprecated 保留给旧走查看板；新代码用 processBlocks */
  const processText = reactive<Record<string, string>>({})
  /** 本次会话里已撤销越界改动的汇报 seq（成员栏「越界待处理」用） */
  const revertedSeqs = reactive<number[]>([])
  /** 成员排队任务（契约 v0.55.6 6.21.17 ④：cat:group-member.queue[]） */
  const queued = reactive<Record<string, CatGroupQueuedTask[]>>({})
  /** 成员状态（契约 v0.55.6 6.21.17 ④ member.status：idle | running | queued） */
  const memberStatus = reactive<Record<string, string>>({})
  /** 「文件」页签（6.21.17 ①） */
  const files = ref<CatGroupFile[]>([])
  const settingsOpen = ref(false)
  const membersOpen = ref(false)
  const scene = groupScene()
  const sceneUi = scene && !groupRealApi() ? groupMock.sceneUi() : null
  const now = ref(Date.now())
  let timer: ReturnType<typeof setInterval> | null = null
  let subscribed = false
  /** messageId → 流式拼装（与单聊同一套 applyStreamEvent） */
  const processStreams = new Map<string, CatStreamMsg>()
  /** messageId → memberId，方便按成员清流 */
  const processStreamOwner = new Map<string, string>()

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
      // 排队灰字先到、cat:group-member 晚到或丢了时，先按灰字记一条排队，避免右栏只显示「正在干活」；成员事件一到就整表覆盖
      const qid = m.route?.memberId || m.toMemberId
      if (m.systemKind === 'route' && m.route?.queued && qid && m.chainId && !(queued[qid] ?? []).some((x) => x.chainId === m.chainId)) {
        const cur = queued[qid] ?? []
        queued[qid] = [...cur, { chainId: m.chainId, hop: m.hop ?? 0, fromSeq: m.seq, queuePosition: cur.length }]
        if (!live[qid] && memberStatus[qid] !== 'running') memberStatus[qid] = 'queued'
      }
      if (m.kind === 'member' && m.report?.files.length) void loadFiles()
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
      applyQueue(e.member.id, e.queue ?? e.member.queue ?? [], e.member.status)
      syncBriefs()
    })
    // 「查看过程」：成员会话流（convId=group:<g>:<m>），与单聊同一套 cat:message / cat:turn
    onCatMessage((e) => applyProcessMessage(e))
    onCatTurn((e) => {
      const mid = memberIdFromGroupConv(e.convId)
      if (!mid) return
      const gid = groupIdFromGroupConv(e.convId)
      if (gid && activeId.value && gid !== activeId.value) return
      if (e.status === 'running') clearProcess(mid)
    })
  }

  function clearProcess(mid: string) {
    processBlocks[mid] = []
    for (const [msgId, owner] of [...processStreamOwner.entries()]) {
      if (owner === mid) {
        processStreams.delete(msgId)
        processStreamOwner.delete(msgId)
      }
    }
  }

  function applyProcessMessage(e: CatStreamEvent) {
    const mid = memberIdFromGroupConv(e.convId)
    if (!mid) return
    const gid = groupIdFromGroupConv(e.convId)
    if (gid && activeId.value && gid !== activeId.value) return
    // 跳过单聊同口径的 status / error / retry 块：过程抽屉只展示思考、工具、正文
    if (e.blockType === 'status' || e.blockType === 'error' || e.blockType === 'retry') return

    let m = processStreams.get(e.messageId)
    if (!m) {
      m = newStreamMsg(e.messageId)
      processStreams.set(e.messageId, m)
      processStreamOwner.set(e.messageId, mid)
    }
    const changed = applyStreamEvent(m, e)
    if (!changed) return

    const kind: ProcessBlock['kind'] =
      e.blockType === 'thought' ? 'think' : e.blockType === 'tool' ? 'tool' : 'text'
    const list = (processBlocks[mid] ??= [])
    let row = list.find((b) => b.id === e.messageId)
    if (!row) {
      if (!m.text && m.done) return
      list.push({ id: e.messageId, kind, text: m.text, streaming: !m.done })
    } else {
      row.text = m.text
      row.streaming = !m.done
      if (kind !== 'text') row.kind = kind
    }
    if (m.done) {
      processStreams.delete(e.messageId)
      processStreamOwner.delete(e.messageId)
    }
  }

  /** 打开「查看过程」：先展示已订阅到的 live 块；若空再拉成员 backing 会话的 CLI 记录 */
  async function openProcess(memberId: string) {
    if (!memberId) return
    drawerMemberId.value = memberId
    if ((processBlocks[memberId]?.length ?? 0) > 0) return
    if (!groupRealApi()) return
    processLoading[memberId] = true
    try {
      const detail = await getCatConversation(memberId)
      if (drawerMemberId.value !== memberId) return
      if ((processBlocks[memberId]?.length ?? 0) > 0) return
      if (!detail?.messages?.length) return
      const blocks: ProcessBlock[] = []
      for (const msg of detail.messages) {
        if (msg.role === 'user' || msg.role === 'system' || msg.role === 'status') continue
        const text = (msg.content ?? '').trim()
        if (!text) continue
        const kind: ProcessBlock['kind'] =
          msg.role === 'thought' ? 'think' : msg.role === 'tool' ? 'tool' : 'text'
        blocks.push({ id: msg.id, kind, text: msg.content })
      }
      if (blocks.length) processBlocks[memberId] = blocks.slice(-80)
    } catch {
      /* 拉不到就保持空态「暂时没有过程」 */
    } finally {
      processLoading[memberId] = false
    }
  }

  /** 走查：直接塞过程块并打开抽屉 */
  function setProcessDemo(memberId: string, raw: string) {
    processBlocks[memberId] = demoProcessBlocks(raw)
    drawerMemberId.value = memberId
  }

  function applyQueue(id: string, q: CatGroupQueuedTask[], status?: string) {
    if (q.length) queued[id] = q
    else delete queued[id]
    if (status) memberStatus[id] = status
    else delete memberStatus[id]
  }
  /** 侧栏头像：当前群成员变了就本地更新摘要（6.21.17 ②：不新增事件） */
  function syncBriefs() {
    const g = group.value
    const list = g && groupsByProject[g.projectId]
    const row = list?.find((x) => x.id === g!.id)
    if (row) row.members = [...members.value].sort((a, b) => a.order - b.order).map((m) => ({ id: m.id, name: m.name, title: m.title, agentKind: m.agentKind, order: m.order, avatarIndex: (m.order % 5) + 1 }))
  }
  async function loadFiles() {
    const id = activeId.value
    if (!id) return
    try {
      const list = await groupApi().listFiles(id)
      if (activeId.value === id) files.value = list ?? []
    } catch {
      /* 拉不到就保留上一次 */
    }
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
    for (const key of Object.keys(processBlocks)) delete processBlocks[key]
    for (const key of Object.keys(processLoading)) delete processLoading[key]
    processStreams.clear()
    processStreamOwner.clear()
    drawerMemberId.value = ''
    try {
      const api = groupApi()
      const [{ group: g, members: ms }, page] = await Promise.all([api.getGroup(groupId), api.listMessages(groupId, 0, 50)])
      if (activeId.value !== groupId) return
      group.value = g
      members.value = ms.map(normalizeMember)
      for (const k of Object.keys(queued)) delete queued[k]
      for (const k of Object.keys(memberStatus)) delete memberStatus[k]
      for (const m of members.value) applyQueue(m.id, m.queue ?? [], m.status)
      files.value = []
      void loadFiles()
      messages.value = page.messages.map(normalizeMessage)
      hasMore.value = page.hasMore
      if (!groupRealApi()) for (const e of groupMock.sceneRunning()) {
        if (e.running) live[e.member.id] = { ...e.running, receivedAt: Date.now() }
        if (e.queue?.length) queued[e.member.id] = e.queue
        memberStatus[e.member.id] = e.member.status || 'running'
      }
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
    drawerMemberId, processBlocks, processLoading, processText, revertedSeqs, queued, memberStatus, files, loadFiles, settingsOpen, membersOpen, scene, sceneUi, workingMembers,
    memberById, nameOf, elapsedOf, loadGroups, open, loadOlder, createGroup, rename, setAutoRoute, deleteGroup,
    addMember, updateMember, removeMember, reorder, regenerate, saveCharter, send, reassign, stop, stopAll, retryTurn, revert, routeDetail, openProcess, setProcessDemo,
  }
})
