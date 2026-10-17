<template>
  <section class="cg" :class="layout" :aria-label="g?.name || t('cat.groupChat.groupFallback')" data-testid="cat-group-view">
    <div class="cg-main">
      <header class="cg-hd">
        <span class="ttl" data-testid="cat-group-title">{{ g?.name }}</span>
        <span class="sub">{{ t('cat.groupChat.membersCount', { n: store.members.length }) }}</span>
        <span class="sp" />
        <span class="cg-stack" aria-hidden="true">
          <span v-for="m in store.membersSorted.slice(0, 5)" :key="m.id" class="av sm" :style="{ '--c': memberColorVar(m.order) }">{{ initial(m.name) }}</span>
        </span>
        
        
      </header>

      <div ref="feedEl" class="cg-feed" data-testid="cat-group-feed">
        <div class="cg-feed-in">
          <button v-if="store.hasMore" type="button" class="cg-btn ghost" style="align-self: center" @click="store.loadOlder()">{{ t('cat.groupChat.loadOlder') }}</button>
          <div v-if="!store.loading && !store.messages.length" class="cg-empty">{{ store.members.length ? t('cat.groupChat.emptyFeed') : t('cat.groupChat.noMembersYet') }}</div>
          <template v-for="m in store.messages" :key="m.seq">
            <div v-if="m.kind === 'user'" class="cg-user" data-testid="cat-group-user-msg">
              <template v-for="(p, i) in parts(m.text)" :key="i"><span v-if="p.at" class="cg-at">{{ p.s }}</span><template v-else>{{ p.s }}</template></template>
            </div>

            <div v-else-if="m.kind === 'member' && m.report" class="cg-rep" data-testid="cat-group-report">
              <span class="av" :style="{ '--c': colorOf(m.memberId) }" :title="t('cat.groupChat.viewProcess')" @click="openProcess(m.memberId)">{{ initial(store.nameOf(m.memberId)) }}</span>
              <article class="cg-card" :class="{ over: overreach(m).length }">
                <div class="cg-who">
                  <b>{{ store.nameOf(m.memberId) }}</b><span class="tt">{{ store.memberById(m.memberId)?.title }}</span>
                  <span class="sp" /><span class="tm">{{ clock(m.createdAt) }}</span>
                </div>
                <div class="cg-sec" style="margin-top: 0">
                  <h4 v-if="m.report.parsed">{{ t('cat.groupChat.did') }}</h4>
                  <p><template v-for="(p, i) in parts(m.report.did)" :key="i"><span v-if="p.at" class="cg-at">{{ p.s }}</span><template v-else>{{ p.s }}</template></template></p>
                </div>
                
                <div v-if="m.report.give" class="cg-sec">
                  <h4>{{ t('cat.groupChat.give') }}</h4>
                  <p><template v-for="(p, i) in parts(m.report.give)" :key="i"><span v-if="p.at" class="cg-at chip">{{ p.s }}</span><template v-else>{{ p.s }}</template></template></p>
                </div>
                <div v-if="m.report.footer?.totalMs" class="cg-foot">
                  <span>{{ t('cat.groupChat.footerTime', { time: formatElapsed(m.report.footer.totalMs) }) }}</span>
                  <span v-if="m.report.footer.outputTokens">{{ t('cat.groupChat.footerTokens', { n: m.report.footer.outputTokens }) }}</span>
                  <span class="sp" />
                  <button type="button" class="lk" @click="openProcess(m.memberId)">{{ t('cat.groupChat.viewProcess') }}</button>
                </div>
              </article>
            </div>

            

            <div v-else-if="m.systemKind === 'report_format'" class="cg-sys fmt" data-testid="cat-group-report-format">
              <FIcon name="info" :size="12" />{{ sysText(m) }}
            </div>

            <div v-else class="cg-sys" :data-kind="m.systemKind" data-testid="cat-group-system">
              <button v-if="m.route && (m.route.mode === 'ai' || m.route.mode === 'fallback')" type="button" class="lk" @click="openRoute(m)">{{ sysText(m) }}</button>
              <span v-else>{{ sysText(m) }}</span>
              <span v-if="canReassign(m)" style="position: relative">
                <button type="button" class="act" data-testid="cat-group-reassign" @click="reassignFor = reassignFor === m.seq ? 0 : m.seq">{{ t('cat.groupChat.reassign') }}</button>
                <div v-if="reassignFor === m.seq" class="cg-pop down" role="menu" :aria-label="t('cat.groupChat.reassignTo')">
                  <button v-for="o in store.membersSorted.filter((x) => x.id !== (m.toMemberId || m.route?.memberId))" :key="o.id" type="button" class="it" role="menuitem" @click="doReassign(m, o.id)">
                    <span class="av sm" :style="{ '--c': memberColorVar(o.order) }">{{ initial(o.name) }}</span>{{ o.name }}<span class="tt">{{ o.title }}</span>
                  </button>
                </div>
              </span>
            </div>
          </template>
          <div v-if="store.routingNow" class="cg-sys dot">{{ t('cat.groupChat.routing') }}</div>
        </div>
      </div>

      

      <div class="cg-comp" data-testid="cat-group-composer">
        <div v-if="mentionOpen && mentionList.length" class="cg-pop" role="listbox" :aria-label="t('cat.groupChat.mentionList')">
          <button v-for="(o, i) in mentionList" :key="o.id" type="button" class="it" :class="{ on: i === mentionIdx }" role="option" :aria-selected="i === mentionIdx" @mousedown.prevent="pickMention(o.id)">
            <span class="av sm" :style="{ '--c': memberColorVar(o.order) }">{{ initial(o.name) }}</span>{{ o.name }}<span class="tt">{{ o.title }}</span>
          </button>
        </div>
        <div v-if="chips.length" class="row">
          <span v-for="id in chips" :key="id" class="cg-chip">
            <span class="av sm" :style="{ '--c': colorOf(id) }">{{ initial(store.nameOf(id)) }}</span>{{ store.nameOf(id) }}
            <button type="button" :aria-label="t('cat.groupChat.removeMention', { name: store.nameOf(id) })" @click="chips = chips.filter((x) => x !== id)"><FIcon name="x" :size="11" /></button>
          </span>
        </div>
        <textarea ref="inputEl" v-model="draft" rows="2" :placeholder="t('cat.groupChat.composerPh')" data-testid="cat-group-input" @input="onInput" @keydown="onKey" />
        <div class="bar">
          <button type="button" class="cg-ib" :aria-label="t('cat.groupChat.mentionList')" @click="openMention()">{{ AT }}</button>
          <span class="sp" />
          <button type="button" class="cg-send" :disabled="!canSend" :aria-label="t('cat.groupChat.send')" data-testid="cat-group-send" @click="send()"><FIcon name="send" :size="15" /></button>
        </div>
      </div>
    </div>

    

    

    

    <div v-if="routeInfo" class="cg-modal-bg" @click.self="routeInfo = null">
      <div class="cg-modal wide" role="dialog" aria-modal="true">
        <h3>{{ t('cat.groupChat.routeDetail') }}</h3>
        <div class="cg-hint">{{ t('cat.groupChat.routeInput') }}</div>
        <pre class="cg-pre">{{ routeInfo.input }}</pre>
        <div class="cg-hint">{{ t('cat.groupChat.routeOutput') }}</div>
        <pre class="cg-pre">{{ routeInfo.output }}</pre>
        <div class="ft"><button type="button" class="cg-btn" @click="routeInfo = null">{{ t('cat.groupChat.close') }}</button></div>
      </div>
    </div>

    
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { useCatGroupChat } from '@/stores/catGroupChat'
import { formatElapsed, memberColorVar, systemLineText, type CatGroupMessage, type CatRouteDetail } from '@/api/catGroupChat'
import { catPanels } from '@/views/cat/catPanels'
import './catGroup.css'

const { t } = useI18n()
const store = useCatGroupChat()
const g = computed(() => store.group)
const AT = '@'

// ≥1100 三栏；760–1099 成员栏变抽屉；<760 单栏
const width = ref(typeof window !== 'undefined' ? window.innerWidth : 1280)
const onResize = () => (width.value = window.innerWidth)
const layout = computed(() => (width.value >= 1100 ? 'wide' : width.value >= 760 ? 'mid' : 'narrow'))

const initial = (n: string) => Array.from(n.trim())[0]?.toUpperCase() ?? ''
const colorOf = (id?: string) => memberColorVar(store.memberById(id)?.order ?? 0)
const clock = (ms: number) => {
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
const sysText = (m: CatGroupMessage) => systemLineText(m, store.nameOf)
const overreach = (m: CatGroupMessage) => (m.report?.files ?? []).filter((f) => f.tag === 'overreach')

function openProcess(id?: string) {
  
  void id
}

/** 正文里的 @成员名 高亮（最长名优先；没匹配上的保持普通文字） */
function parts(text: string): { s: string; at: boolean }[] {
  const names = store.members.map((m) => m.name).filter(Boolean).sort((a, b) => b.length - a.length)
  if (!names.length) return [{ s: text, at: false }]
  const re = new RegExp(`@(${names.map((n) => n.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})`, 'g')
  const out: { s: string; at: boolean }[] = []
  let last = 0
  for (const hit of text.matchAll(re)) {
    if (hit.index! > last) out.push({ s: text.slice(last, hit.index), at: false })
    out.push({ s: hit[0], at: true })
    last = hit.index! + hit[0].length
  }
  if (last < text.length) out.push({ s: text.slice(last), at: false })
  return out
}

// 换人：只给每条链最新一条派活灰字
const reassignFor = ref(0)
function canReassign(m: CatGroupMessage) {
  if (m.systemKind !== 'route' || !m.chainId || store.members.length < 2) return false
  const lastRoute = [...store.messages].reverse().find((x) => x.chainId === m.chainId && x.systemKind === 'route')
  return lastRoute?.seq === m.seq
}
async function doReassign(m: CatGroupMessage, to: string) {
  reassignFor.value = 0
  try {
    await store.reassign(m.chainId, to)
  } catch {
    ElMessage.error(t('cat.groupChat.sendFailed'))
  }
}

const routeInfo = ref<CatRouteDetail | null>(null)
async function openRoute(m: CatGroupMessage) {
  try {
    routeInfo.value = await store.routeDetail(m.route?.routeSeq || m.seq)
  } catch {
    /* 拿不到详情就不弹 */
  }
}

// 输入框：@ 选人（mentions 以选人为准），chip 整体删除
const draft = ref('')
const chips = ref<string[]>([])
const inputEl = ref<HTMLTextAreaElement | null>(null)
const mentionOpen = ref(false)
const mentionQuery = ref('')
const mentionIdx = ref(0)
const mentionList = computed(() => store.membersSorted.filter((m) => !chips.value.includes(m.id) && m.name.toLowerCase().includes(mentionQuery.value.toLowerCase())))
const canSend = computed(() => !!draft.value.trim() && store.members.length > 0)
function onInput() {
  const el = inputEl.value
  if (!el) return
  const hit = /@([^\s@]*)$/.exec(draft.value.slice(0, el.selectionStart ?? draft.value.length))
  mentionOpen.value = !!hit
  mentionQuery.value = hit?.[1] ?? ''
  mentionIdx.value = 0
}
function openMention() {
  draft.value += AT
  void nextTick(() => {
    inputEl.value?.focus()
    onInput()
  })
}
function pickMention(id: string) {
  const el = inputEl.value
  const pos = el?.selectionStart ?? draft.value.length
  draft.value = draft.value.slice(0, pos).replace(/@[^\s@]*$/, '') + draft.value.slice(pos)
  if (!chips.value.includes(id)) chips.value.push(id)
  mentionOpen.value = false
  void nextTick(() => el?.focus())
}
function onKey(e: KeyboardEvent) {
  const list = mentionList.value
  if (mentionOpen.value && list.length) {
    if (e.key === 'ArrowDown') { e.preventDefault(); mentionIdx.value = (mentionIdx.value + 1) % list.length; return }
    if (e.key === 'ArrowUp') { e.preventDefault(); mentionIdx.value = (mentionIdx.value - 1 + list.length) % list.length; return }
    if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); pickMention(list[mentionIdx.value].id); return }
    if (e.key === 'Escape') { mentionOpen.value = false; return }
  }
  if (e.key === 'Backspace' && !draft.value && chips.value.length) chips.value.pop()
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) { e.preventDefault(); void send() }
}
async function send() {
  if (!canSend.value) return
  const mentions = [...chips.value]
  const text = [...mentions.map((id) => `${AT}${store.nameOf(id)}`), draft.value.trim()].join(' ')
  const keep = { draft: draft.value, chips: mentions }
  draft.value = ''
  chips.value = []
  try {
    await store.send(text, mentions)
  } catch {
    draft.value = keep.draft
    chips.value = keep.chips
    ElMessage.error(t('cat.groupChat.sendFailed'))
  }
}

const feedEl = ref<HTMLElement | null>(null)
function scrollEnd() {
  void nextTick(() => feedEl.value && (feedEl.value.scrollTop = feedEl.value.scrollHeight))
}
watch(() => store.messages.length, scrollEnd)

// 走查场景（?cat_group_scene=…）：直接摆好对应面板 / 弹窗
const addOpen = ref(false)
const initialOpen = ref('')
function applyScene() {
  const s = store.scene
  if (!s) return
  
}

watch(
  () => catPanels.activeGroupId,
  async (id) => {
    if (!id) return
    await store.open(id)
    applyScene()
    scrollEnd()
  },
  { immediate: true },
)
onMounted(() => window.addEventListener('resize', onResize))
onBeforeUnmount(() => window.removeEventListener('resize', onResize))
void addOpen
void initialOpen
void layout
</script>
