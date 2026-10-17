<template>
  <section ref="rootEl" class="cg" :class="layout" :aria-label="g?.name || t('cat.groupChat.groupFallback')" data-testid="cat-group-view">
    <div class="cg-main">
      <header class="cg-hd">
        <span class="ttl" data-testid="cat-group-title">{{ g?.name }}</span>
        <span class="sub">{{ t('cat.groupChat.membersCount', { n: store.members.length }) }}</span>
        <span class="sp" />
        <span class="cg-stack" aria-hidden="true">
          <span v-for="m in store.membersSorted.slice(0, 5)" :key="m.id" class="av sm" :style="{ '--c': memberColorVar(m.order) }">{{ initial(m.name) }}</span>
        </span>
        <button v-if="layout !== 'wide'" type="button" class="cg-ib" :class="{ on: store.membersOpen }" data-testid="cat-group-members-toggle" @click="store.membersOpen = !store.membersOpen">
          <FIcon name="list" :size="15" /><span v-if="layout === 'mid'">{{ t('cat.groupChat.openMembers') }}</span>
        </button>
        <button type="button" class="cg-ib" :title="t('cat.groupChat.settings')" :aria-label="t('cat.groupChat.settings')" data-testid="cat-group-settings" @click="store.settingsOpen = true">
          <FIcon name="set" :size="15" />
        </button>
      </header>

      <div ref="feedEl" class="cg-feed" data-testid="cat-group-feed">
        <div class="cg-feed-in">
          <button v-if="store.hasMore" type="button" class="cg-btn ghost" style="align-self: center" @click="store.loadOlder()">{{ t('cat.groupChat.loadOlder') }}</button>
          <div v-if="!store.loading && !store.messages.length" class="cg-empty" data-testid="cat-group-empty">
            <div class="ill" aria-hidden="true"><FIcon name="group" :size="30" /></div>
            <h3>{{ store.members.length ? t('cat.groupChat.emptyTitle') : t('cat.groupChat.noMembersYet') }}</h3>
            <p>{{ t('cat.groupChat.emptySub') }}</p>
            <ol>
              <li><span>1</span>{{ t('cat.groupChat.emptyStep1') }}</li>
              <li><span>2</span>{{ t('cat.groupChat.emptyStep2') }}</li>
              <li><span>3</span>{{ t('cat.groupChat.emptyStep3') }}</li>
            </ol>
          </div>
          <div v-if="store.messages.length" class="cg-day"><span>{{ t('cat.groupChat.today') }}</span></div>
          <template v-for="m in store.messages" :key="m.seq">
            <div v-if="m.kind === 'user'" class="cg-task" data-testid="cat-group-user-msg">
              <div class="hd">
                <span class="av sm me">{{ initial(t('cat.groupChat.you')) }}</span>
                <b>{{ t('cat.groupChat.you') }}</b><span class="tag">{{ t('cat.groupChat.task') }}</span>
                <span class="tm">{{ clock(m.createdAt) }}</span>
              </div>
              <div class="tx"><template v-for="(p, i) in parts(m.text)" :key="i"><span v-if="p.at" class="cg-at">{{ p.s }}</span><template v-else>{{ p.s }}</template></template></div>
            </div>

            <div v-else-if="m.kind === 'member' && isMemberShortReply(m)" class="cg-short" data-testid="cat-group-short">
              <div class="hd">
                <span class="av sm" :style="{ '--c': colorOf(m.memberId) }">{{ initial(store.nameOf(m.memberId)) }}</span>
                <b>{{ store.nameOf(m.memberId) }}</b><span class="tt">{{ store.memberById(m.memberId)?.title }}</span>
                <span class="cli">{{ cliName(store.memberById(m.memberId)?.agentKind) }}</span>
                <span class="tm">{{ clock(m.createdAt) }}</span>
              </div>
              <div class="tx"><template v-for="(p, i) in parts(memberShortText(m))" :key="i"><span v-if="p.at" class="cg-at">{{ p.s }}</span><template v-else>{{ p.s }}</template></template></div>
              <!-- 短复轻于汇报卡：无 token/耗时页脚；仅保留「查看完整过程」 -->
              <div v-if="m.turnId || m.memberId" class="ft">
                <button type="button" class="lk" @click="openProcess(m.memberId)"><FIcon name="eye" :size="12" />{{ t('cat.groupChat.viewFull') }}</button>
              </div>
            </div>

            <article v-else-if="m.kind === 'member' && m.report" class="cg-card" :class="{ over: overreach(m).length && !reverted[m.seq] }" data-testid="cat-group-report">
              <div class="cg-who">
                <span class="av" :style="{ '--c': colorOf(m.memberId) }">{{ initial(store.nameOf(m.memberId)) }}</span>
                <b>{{ store.nameOf(m.memberId) }}</b><span class="tt">{{ store.memberById(m.memberId)?.title }}</span>
                <span class="cli">{{ cliName(store.memberById(m.memberId)?.agentKind) }}</span>
                <span class="sp" /><span class="tm">{{ clock(m.createdAt) }}</span>
              </div>
              <div class="cg-sec">
                <h4>{{ t('cat.groupChat.did') }}</h4>
                <p><template v-for="(p, i) in parts(m.report.did)" :key="i"><span v-if="p.at" class="cg-at">{{ p.s }}</span><template v-else>{{ p.s }}</template></template></p>
                <div v-if="!m.report.parsed" class="cg-note" data-testid="cat-group-report-format"><FIcon name="info" :size="12" />{{ t('cat.groupChat.notFormatted') }}</div>
              </div>
              <div v-if="m.report.parsed && m.report.files.length" class="cg-sec">
                <h4>{{ t('cat.groupChat.files') }}</h4>
                <button type="button" class="cg-fsum" :aria-expanded="!!expanded[m.seq]" @click="expanded[m.seq] = !expanded[m.seq]">
                  <FIcon name="right" :size="12" :class="{ rot: expanded[m.seq] }" />
                  {{ t('cat.groupChat.filesN', { n: m.report.files.length }) }}
                  <span class="ad">+{{ sum(m, 'added') }}</span><span class="dl">−{{ sum(m, 'deleted') }}</span>
                  <span v-if="overreach(m).length" class="cg-tag over">{{ t('cat.groupChat.overN', { n: overreach(m).length }) }}</span>
                </button>
                <ul v-if="expanded[m.seq]" class="cg-files rows">
                  <li v-for="f in m.report.files" :key="f.path" :class="{ ov: f.tag === 'overreach' }">
                    <FIcon name="file" :size="12" class="fi" />
                    <span class="p"><span class="dir">{{ dirOf(f.path) }}</span>{{ baseOf(f.path) }}</span>
                    <span v-if="f.tag === 'overreach'" class="cg-tag over">{{ t('cat.groupChat.tagOverreach') }}</span>
                    <span v-else-if="f.tag === 'unknown'" class="cg-tag unk" :title="t('cat.groupChat.unknownTip')">{{ t('cat.groupChat.tagUnknown') }}</span>
                    <span class="nums"><span v-if="f.added" class="ad">+{{ f.added }}</span><span v-if="f.deleted" class="dl">−{{ f.deleted }}</span></span>
                  </li>
                </ul>
              </div>
              <div v-if="overreach(m).length" class="cg-over" data-testid="cat-group-overreach">
                <div class="h">
                  <FIcon name="warn" :size="13" />{{ t('cat.groupChat.overreachTitle', { n: overreach(m).length }) }}<span class="sp" />
                  <button type="button" class="cg-btn org" :disabled="reverted[m.seq]" data-testid="cat-group-revert" @click="revertMsg = m">{{ t('cat.groupChat.undoThese') }}</button>
                </div>
                <ul class="cg-files">
                  <li v-for="f in overreach(m)" :key="f.path" :class="revState(m, f.path)">
                    <span class="p"><span class="dir">{{ dirOf(f.path) }}</span>{{ baseOf(f.path) }}</span>
                    <span v-if="revState(m, f.path) === 'done'" class="st ok">{{ t('cat.groupChat.revertedTag') }}</span>
                    <span v-else-if="revState(m, f.path) === 'skip'" class="st bad">{{ t('cat.groupChat.notReverted') }}</span>
                  </li>
                </ul>
                <div v-for="s in revertSkips[m.seq] ?? []" :key="s" class="cg-skip">{{ t('cat.groupChat.revertSkipped', { file: s }) }}</div>
              </div>
              <div v-if="m.report.parsed && m.report.attn" class="cg-sec">
                <h4><FIcon name="warn" :size="12" class="warn" />{{ t('cat.groupChat.attn') }}</h4>
                <p><template v-for="(p, i) in parts(m.report.attn)" :key="i"><span v-if="p.at" class="cg-at">{{ p.s }}</span><template v-else>{{ p.s }}</template></template></p>
              </div>
              <div v-if="m.report.parsed && m.report.give" class="cg-sec">
                <h4>{{ t('cat.groupChat.give') }}</h4>
                <p v-for="(ln, li) in m.report.give.split('\n')" :key="li" class="give"><template v-for="(p, i) in parts(ln)" :key="i"><span v-if="p.at" class="cg-at chip">{{ p.s }}</span><template v-else>{{ p.s }}</template></template></p>
              </div>
              <div v-if="m.report.footer" class="cg-foot">
                <span v-if="m.report.footer.finishedAt">{{ t('cat.groupChat.footerDone', { time: clock(m.report.footer.finishedAt) }) }}</span>
                <span v-else-if="m.report.footer.totalMs">{{ t('cat.groupChat.footerTime', { time: formatElapsed(m.report.footer.totalMs) }) }}</span>
                <span v-if="m.report.footer.outputTokens">· {{ t('cat.groupChat.footerOut', { n: kfmt(m.report.footer.outputTokens) }) }}</span>
                <span v-if="m.report.footer.tokensPerSec">· {{ t('cat.groupChat.footerRate', { n: m.report.footer.tokensPerSec }) }}</span>
                <span>·</span>
                <button type="button" class="lk" @click="openProcess(m.memberId)"><FIcon name="eye" :size="12" />{{ t('cat.groupChat.viewFull') }}</button>
              </div>
            </article>

            <div v-else-if="m.systemKind === 'failed'" class="cg-sys fail" role="alert" data-testid="cat-group-failed">
              <FIcon name="warn" :size="12" />
              <span>{{ t('cat.groupChat.failed', { name: store.nameOf(m.memberId), reason: m.failReason || m.text || t('cat.groupChat.unknownReason') }) }}</span>
              <span class="bar" />
              <button v-if="!retried[m.seq]" type="button" class="act" :title="t('cat.groupChat.retryTip')" data-testid="cat-group-retry" @click="retry(m)"><FIcon name="retry" :size="12" />{{ t('cat.groupChat.retry') }}</button>
              <span v-else>{{ t('cat.groupChat.retrying') }}</span>
              <button type="button" class="act" @click="openProcess(m.memberId)">{{ t('cat.groupChat.viewProcess') }}</button>
            </div>

            <div v-else-if="m.systemKind === 'stopped'" class="cg-sys" data-testid="cat-group-stopped">
              <FIcon name="stop" :size="11" />{{ sysText(m) }}<template v-if="m.toMemberId">{{ t('cat.groupChat.stoppedBySwap') }}</template>
            </div>

            <template v-else-if="m.systemKind === 'report_format'" />

            <div v-else class="cg-sysw">
              <div class="cg-sys" :data-kind="m.systemKind" data-testid="cat-group-system">
                <FIcon :name="sysIcon(m)" :size="12" />
                <button v-if="m.route && (m.route.mode === 'ai' || m.route.mode === 'fallback' || m.route.mode === 'undecided')" type="button" class="lk" @click="toggleRoute(m)">{{ sysText(m) }}</button>
                <span v-else>{{ sysText(m) }}</span>
                <span v-if="m.route?.durationMs" class="took">· {{ t('cat.groupChat.routeTookInline', { time: formatElapsed(m.route.durationMs) }) }}</span>
                <span v-if="canReassign(m)" style="position: relative">
                  <button type="button" class="act" data-testid="cat-group-reassign" @click="reassignFor = reassignFor === m.seq ? 0 : m.seq"><FIcon name="swap" :size="12" />{{ t('cat.groupChat.reassign') }}</button>
                  <div v-if="reassignFor === m.seq" class="cg-pop down swap" role="menu" :aria-label="t('cat.groupChat.swapTitle')">
                    <div class="ph">{{ t('cat.groupChat.swapTitle') }}</div>
                    <button v-for="o in store.membersSorted" :key="o.id" type="button" class="it" role="menuitem" :disabled="o.id === (m.toMemberId || m.route?.memberId)" @click="doReassign(m, o.id)">
                      <span class="av sm" :style="{ '--c': memberColorVar(o.order) }">{{ initial(o.name) }}</span>{{ o.name }}
                      <span class="sp" /><span class="tt" :class="{ busy: store.live[o.id] && o.id !== (m.toMemberId || m.route?.memberId) }"><i v-if="store.live[o.id] && o.id !== (m.toMemberId || m.route?.memberId)" class="cg-breath" />{{ o.id === (m.toMemberId || m.route?.memberId) ? t('cat.groupChat.current') : store.live[o.id] ? t('cat.groupChat.busy') : t('cat.groupChat.idle') }}</span>
                    </button>
                    <div class="pn">{{ t('cat.groupChat.swapNote') }}</div>
                  </div>
                </span>
              </div>
              <div v-if="routeOpen[m.seq]" class="cg-route" data-testid="cat-group-route-detail">
                <h5>{{ t('cat.groupChat.routeJudge') }}</h5>
                <dl>
                  <dt>{{ t('cat.groupChat.routeCli') }}</dt><dd>{{ cliName(store.membersSorted[0]?.agentKind) }}</dd>
                  <template v-if="routeOpen[m.seq].durationMs"><dt>{{ t('cat.groupChat.routeTook') }}</dt><dd>{{ formatElapsed(routeOpen[m.seq].durationMs!) }}</dd></template>
                  <dt>{{ t('cat.groupChat.routeSaw') }}</dt><dd>{{ routeOpen[m.seq].input }}</dd>
                </dl>
                <h5>{{ t('cat.groupChat.routeRaw') }}</h5>
                <pre class="cg-pre">{{ routeOpen[m.seq].output }}</pre>
              </div>
            </div>
          </template>
          <div v-if="store.routingNow" class="cg-sys dot">{{ t('cat.groupChat.routing') }}</div>
        </div>
      </div>

      <div v-if="store.workingMembers.length" class="cg-work" data-testid="cat-group-working">
        <div v-for="m in store.workingMembers" :key="m.id" class="cg-wrow">
          <span v-if="store.workingMembers.length === 1" class="cg-pulse" aria-hidden="true" />
          <span class="av sm" :style="{ '--c': memberColorVar(m.order) }">{{ initial(m.name) }}</span>
          <span class="tx"><b>{{ m.name }}</b>{{ t('cat.groupChat.working', { name: '', time: formatElapsed(store.elapsedOf(m.id)) }) }}<template v-if="(store.live[m.id]?.toolCount ?? 0) > 0">{{ t('cat.groupChat.workingTools', { n: store.live[m.id].toolCount }) }}</template></span>
          <button type="button" class="lkb" @click="openProcess(m.id)">{{ t('cat.groupChat.viewProcess') }}</button>
          <button type="button" class="lkb" @click="store.stop(m.id)">{{ t('cat.groupChat.stop') }}</button>
        </div>
        <div v-if="store.workingMembers.length > 1" class="cg-wall">
          <span class="cg-pulse" aria-hidden="true" /><b>{{ t('cat.groupChat.nWorking', { n: store.workingMembers.length }) }}</b>
          <span class="sp" />
          <button type="button" class="lkb red" data-testid="cat-group-stop-all" @click="store.stopAll()">{{ t('cat.groupChat.stopAll') }}</button>
        </div>
      </div>

      <div class="cg-comp" data-testid="cat-group-composer">
        <div v-if="mentionOpen && mentionList.length" class="cg-pop" role="listbox" :aria-label="t('cat.groupChat.mentionList')">
          <div class="ph">{{ AT }} {{ t('cat.groupChat.members') }}</div>
          <button v-for="(o, i) in mentionList" :key="o.id" type="button" class="it" :class="{ on: i === mentionIdx }" role="option" :aria-selected="i === mentionIdx" @mousedown.prevent="pickMention(o.id)">
            <span class="av sm" :style="{ '--c': memberColorVar(o.order) }">{{ initial(o.name) }}</span>{{ o.name }}<span class="tt">{{ o.title }}</span>
            <span class="sp" /><span class="tt" :class="{ busy: store.live[o.id] }"><i v-if="store.live[o.id]" class="cg-breath" />{{ store.live[o.id] ? t('cat.groupChat.busy') : t('cat.groupChat.idle') }}</span>
          </button>
        </div>
        <div class="row">
          <span v-for="id in chips" :key="id" class="cg-chip">
            {{ AT }}{{ store.nameOf(id) }}
            <button type="button" :aria-label="t('cat.groupChat.removeMention', { name: store.nameOf(id) })" @click="chips = chips.filter((x) => x !== id)"><FIcon name="x" :size="10" /></button>
          </span>
          <textarea ref="inputEl" v-model="draft" rows="2" :placeholder="chips.length ? '' : t('cat.groupChat.composerPh')" data-testid="cat-group-input" @input="onInput" @keydown="onKey" />
        </div>
        <div class="bar">
          <button type="button" class="cg-ib" :aria-label="t('cat.groupChat.attach')" disabled><FIcon name="plus" :size="15" /></button>
          <button type="button" class="cg-ib" :aria-label="t('cat.groupChat.mentionList')" @click="openMention()">{{ AT }}</button>
          <span class="sp" />
          <button type="button" class="cg-send" :disabled="!canSend" :aria-label="t('cat.groupChat.send')" data-testid="cat-group-send" @click="send()"><FIcon name="send" :size="15" /></button>
        </div>
      </div>
      <div class="cg-chint">{{ t('cat.groupChat.composerHint') }}</div>
    </div>

    <div v-if="layout !== 'wide' && store.membersOpen" class="cg-scrim" @click="store.membersOpen = false" />
    <CatGroupMembers :class="{ open: store.membersOpen }" :closable="layout !== 'wide'" :initial-open="initialOpen" @close="store.membersOpen = false" @add="addOpen = true" />
    <CatGroupCreateDialog v-if="addOpen" mode="member" @close="addOpen = false" />

    <div v-if="store.drawerMemberId" class="cg-scrim on" @click="store.drawerMemberId = ''" />
    <aside v-if="store.drawerMemberId" class="cg-proc" role="dialog" :aria-label="t('cat.groupChat.viewProcess')" data-testid="cat-group-process">
      <div class="cg-proc-hd">
        <span class="av" :style="{ '--c': colorOf(store.drawerMemberId) }">{{ initial(store.nameOf(store.drawerMemberId)) }}</span>
        <div class="tt">
          <b>{{ store.nameOf(store.drawerMemberId) }} · {{ t('cat.groupChat.viewProcess') }}</b>
          <span>{{ t('cat.groupChat.sessionSub', { cli: cliName(store.memberById(store.drawerMemberId)?.agentKind), model: store.memberById(store.drawerMemberId)?.modelId || t('cat.groupChat.defaultModel'), n: drawerHop + 1 }) }}</span>
        </div>
        <button type="button" class="cg-ib" :aria-label="t('cat.groupChat.close')" @click="store.drawerMemberId = ''"><FIcon name="x" :size="14" /></button>
      </div>
      <div class="cg-proc-ban"><FIcon name="info" :size="12" />{{ t('cat.groupChat.sessionBanner') }}</div>
      <div class="bd">
        <div v-if="store.processLoading[store.drawerMemberId]" class="cg-hint">{{ t('cat.groupChat.processLoading') }}</div>
        <template v-else-if="drawerRows.length">
          <template v-for="row in drawerRows" :key="row.id">
            <div v-if="row.kind === 'tool'" class="cg-tool" :aria-busy="row.streaming || undefined">
              <span class="v">{{ row.name }}</span><code v-if="row.arg">{{ row.arg }}</code>
              <pre v-if="row.out">{{ row.out }}</pre>
            </div>
            <div v-else-if="row.kind === 'think'" class="cg-think" :aria-busy="row.streaming || undefined">{{ row.text }}</div>
            <p v-else>{{ row.text }}</p>
          </template>
        </template>
        <div v-else class="cg-hint">{{ t('cat.groupChat.processEmpty') }}</div>
      </div>
      <div class="cg-proc-ft">
        <template v-if="store.live[store.drawerMemberId]">
          <span class="cg-pulse" aria-hidden="true" />
          <span>{{ t('cat.groupChat.sessionRunning', { time: formatElapsed(store.elapsedOf(store.drawerMemberId)), n: store.live[store.drawerMemberId].toolCount }) }}</span>
          <span class="sp" />
          <button type="button" class="cg-btn" @click="store.stop(store.drawerMemberId)"><FIcon name="stop" :size="11" />{{ t('cat.groupChat.stop') }}</button>
        </template>
        <span v-else>{{ t('cat.groupChat.idle') }}</span>
      </div>
    </aside>

    <div v-if="revertMsg" class="cg-modal-bg" @click.self="revertMsg = null">
      <div class="cg-modal" role="alertdialog" aria-modal="true" data-testid="cat-group-revert-confirm">
        <h3>{{ t('cat.groupChat.revertTitle', { name: store.nameOf(revertMsg.memberId) }) }}</h3>
        <div class="body">{{ t('cat.groupChat.revertBody', { n: overreach(revertMsg).length }) }}</div>
        <ul class="cg-files rows" style="margin-top: 10px">
          <li v-for="f in overreach(revertMsg)" :key="f.path" class="ov">
            <FIcon name="file" :size="12" class="fi" /><span class="p"><span class="dir">{{ dirOf(f.path) }}</span>{{ baseOf(f.path) }}</span>
            <span class="cg-tag over">{{ t('cat.groupChat.tagOverreach') }}</span>
            <span class="nums"><span v-if="f.added" class="ad">+{{ f.added }}</span><span v-if="f.deleted" class="dl">−{{ f.deleted }}</span></span>
          </li>
        </ul>
        <div class="cg-hint" style="margin-top: 8px; color: var(--ff-text-2)">{{ t('cat.groupChat.revertUnknownNote') }}</div>
        <div class="ft">
          <button type="button" class="cg-btn" @click="revertMsg = null">{{ t('cat.groupChat.cancel') }}</button>
          <button type="button" class="cg-btn dngf" :disabled="reverting" data-testid="cat-group-revert-ok" @click="doRevert()">{{ t('cat.groupChat.revertOk') }}</button>
        </div>
      </div>
    </div>

    <div v-if="rolegenOpen" class="cg-modal-bg" @click.self="rolegenOpen = false">
      <div class="cg-modal wide rolegen" role="dialog" aria-modal="true" data-testid="cat-group-rolegen">
        <h3>{{ t('cat.groupChat.charterHead') }}</h3>
        <div class="cg-rg-list"><CatGroupCharter v-for="m in store.membersSorted" :key="m.id" :member="m" /></div>
        <div class="ft">
          <span class="cg-hint" style="margin-right: auto">{{ t('cat.groupChat.rolegenDone', { done: charterReady, n: store.members.length }) }}</span>
          <button type="button" class="cg-btn pri" @click="rolegenOpen = false">{{ t('cat.groupChat.ok') }}</button>
        </div>
      </div>
    </div>

    <CatGroupSettings v-if="store.settingsOpen" @close="store.settingsOpen = false" @deleted="onDeleted" />
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { useCatGroupChat } from '@/stores/catGroupChat'
import { formatElapsed, isMemberShortReply, memberColorVar, memberShortText, systemLineText, type CatGroupMessage, type CatRouteDetail } from '@/api/catGroupChat'
import { SESSION_DEMO } from '@/api/catGroupChatMock'
import { catPanels } from '@/views/cat/catPanels'
import { catPane } from '@/components/cat/catPreviewWidth'

import CatGroupMembers from './CatGroupMembers.vue'
import CatGroupCreateDialog from './CatGroupCreateDialog.vue'
import CatGroupSettings from './CatGroupSettings.vue'
import CatGroupCharter from './CatGroupCharter.vue'
import './catGroup.css'

const { t } = useI18n()
const store = useCatGroupChat()
const g = computed(() => store.group)
const AT = '@'

// 布局按群聊区域自身宽度算（应用窗口最小 1024px，窗口宽度到不了 760）：
// 设计 v0.2.1 的 ≥1100 / 760 是含左栏（约 250px）的窗口宽度，这里折算成区域宽度 ≥850 三栏、510–849 成员栏变抽屉、<510 单栏
const rootEl = ref<HTMLElement | null>(null)
const width = ref(1200)
let ro: ResizeObserver | null = null
const layout = computed(() => (width.value >= 850 ? 'wide' : width.value >= 510 ? 'mid' : 'narrow'))

const initial = (n: string) => Array.from(n.trim())[0]?.toUpperCase() ?? ''
const colorOf = (id?: string) => memberColorVar(store.memberById(id)?.order ?? 0)
const clock = (ms: number) => {
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
const sysText = (m: CatGroupMessage) => systemLineText(m, store.nameOf)
const overreach = (m: CatGroupMessage) => (m.report?.files ?? []).filter((f) => f.tag === 'overreach')

const drawerHop = computed(() => [...store.messages].reverse().find((m) => m.memberId === store.drawerMemberId || m.toMemberId === store.drawerMemberId)?.hop ?? 0)
const drawerBlocks = computed(() => (store.drawerMemberId ? (store.processBlocks[store.drawerMemberId] ?? []) : []))
function toolParts(text: string): { name: string; arg: string; out: string } {
  const lines = text.split('\n')
  const first = lines[0] ?? ''
  const m = /^(\S+)\s*(.*)$/.exec(first)
  return { name: m?.[1] ?? 'tool', arg: m?.[2] ?? '', out: lines.slice(1).join('\n') }
}
const drawerRows = computed(() =>
  drawerBlocks.value.map((b) => {
    if (b.kind !== 'tool') return { id: b.id, kind: b.kind, text: b.text, streaming: b.streaming, name: '', arg: '', out: '' }
    const p = toolParts(b.text)
    return { id: b.id, kind: b.kind, text: b.text, streaming: b.streaming, name: p.name, arg: p.arg, out: p.out }
  }),
)
function openProcess(id?: string) {
  if (!id) return
  void store.openProcess(id)
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
  // v0.55.9：undecided 不起成员、不出现「换人」
  if (m.systemKind !== 'route' || !m.chainId || store.members.length < 2) return false
  if (m.route?.mode === 'undecided') return false
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

const routeOpen = reactive<Record<number, CatRouteDetail>>({})
async function toggleRoute(m: CatGroupMessage) {
  if (routeOpen[m.seq]) { delete routeOpen[m.seq]; return }
  try {
    routeOpen[m.seq] = await store.routeDetail(m.route?.routeSeq || m.seq)
  } catch {
    /* 拿不到详情就不展开 */
  }
}
const CLI: Record<string, string> = { claude_cli: 'Claude', codex_cli: 'Codex', cat_build: 'Grok Build', gemini_cli: 'Gemini' }
const cliName = (k?: string) => CLI[k ?? ''] ?? k ?? ''
const dirOf = (p: string) => p.slice(0, p.lastIndexOf('/') + 1)
const baseOf = (p: string) => p.slice(p.lastIndexOf('/') + 1)
const kfmt = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k` : String(n))
const sum = (m: CatGroupMessage, k: 'added' | 'deleted') => (m.report?.files ?? []).reduce((n, f) => n + (f[k] ?? 0), 0)
function sysIcon(m: CatGroupMessage) {
  if (m.systemKind === 'reassign') return 'swap'
  if (m.systemKind === 'hop_limit' || m.systemKind === 'no_handoff') return 'info'
  return 'dispatch'
}
const revDone = reactive<Record<number, string[]>>({})
function revState(m: CatGroupMessage, path: string) {
  if (!reverted[m.seq]) return ''
  return (revDone[m.seq] ?? []).includes(path) ? 'done' : 'skip'
}

// 越界撤销（仅确定归属的文件；来源不明不给撤销）
const expanded = reactive<Record<number, boolean>>({})
const reverted = reactive<Record<number, boolean>>({})
const revertSkips = reactive<Record<number, string[]>>({})
const revertMsg = ref<CatGroupMessage | null>(null)
const reverting = ref(false)
async function doRevert() {
  const m = revertMsg.value
  if (!m) return
  reverting.value = true
  try {
    const r = await store.revert(m.memberId || '', m.turnId || '', overreach(m).map((f) => f.path))
    reverted[m.seq] = true
    store.revertedSeqs.push(m.seq)
    revDone[m.seq] = r.reverted
    revertSkips[m.seq] = r.skipped.map((s) => s.path)
    if (r.reverted.length) ElMessage.success(t('cat.groupChat.revertDone', { n: r.reverted.length }))
  } catch {
    ElMessage.error(t('cat.groupChat.revertFailed'))
  } finally {
    reverting.value = false
    revertMsg.value = null
  }
}

// 失败重试：同一任务再跑一轮，hop 不加（6.21.15 ①）
const retried = reactive<Record<number, boolean>>({})
async function retry(m: CatGroupMessage) {
  retried[m.seq] = true
  try {
    await store.retryTurn(m.memberId || '', m.chainId)
  } catch {
    retried[m.seq] = false
    ElMessage.error(t('cat.groupChat.retryFailed'))
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
const rolegenOpen = ref(false)
const charterReady = computed(() => store.members.filter((m) => m.charter.status === 'ok').length)
const initialOpen = ref('')
function applyScene() {
  const s = store.scene
  if (!s) return
  if (s === 'overstep-undo') revertMsg.value = store.messages.find((m) => overreach(m).length) ?? null
  if (s === 'settings') store.settingsOpen = true
  const ui = store.sceneUi
  if (ui?.revertDone) {
    const m = store.messages.find((x) => overreach(x).length)
    if (m) {
      const files = overreach(m).map((f) => f.path)
      reverted[m.seq] = true
      store.revertedSeqs.push(m.seq)
      revDone[m.seq] = files.slice(0, -1)
      revertSkips[m.seq] = files.slice(-1)
      expanded[m.seq] = true
      ElMessage.success(t('cat.groupChat.revertDone', { n: files.length - 1 }))
    }
  }
  if (s === 'overstep') { const m = store.messages.find((x) => overreach(x).length); if (m) expanded[m.seq] = true }
  if (ui?.openRoute) { const m = store.messages.find((x) => x.route?.mode === 'ai'); if (m) void toggleRoute(m) }
  if (ui?.swapOpen) { const m = [...store.messages].reverse().find((x) => x.systemKind === 'route'); if (m) reassignFor.value = m.seq }
  if (s === 'members' || s === 'narrow-drawer') store.membersOpen = true
  if (s === 'members') initialOpen.value = 'b'
  if (s === 'rolegen') rolegenOpen.value = true
  if (s === 'session') store.setProcessDemo('f', SESSION_DEMO)
  if (s === 'mention') { draft.value = AT; mentionOpen.value = true; mentionQuery.value = '' }
  if (s === 'narrow-single') catPane.side = 264 // 1024 窗口：左栏 264 + 中栏 760，右栏收成抽屉（设计走查 建议 9）
}
function onDeleted() {
  store.settingsOpen = false
  catPanels.view = 'chat'
}

watch(
  () => catPanels.activeGroupId,
  async (id) => {
    if (!id) return
    await store.open(id)
    applyScene()
    if (store.sceneUi?.scroll === 'top') void nextTick(() => feedEl.value && (feedEl.value.scrollTop = 0))
    else scrollEnd()
  },
  { immediate: true },
)
onMounted(() => {
  if (!rootEl.value) return
  width.value = rootEl.value.clientWidth
  ro = new ResizeObserver(([e]) => (width.value = e.contentRect.width))
  ro.observe(rootEl.value)
})
onBeforeUnmount(() => ro?.disconnect())
void addOpen
void initialOpen
void layout
</script>
