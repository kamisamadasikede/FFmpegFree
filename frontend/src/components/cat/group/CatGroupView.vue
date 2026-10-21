<template>
  <section ref="rootEl" class="cg" :class="layout" :aria-label="g?.name || t('cat.groupChat.groupFallback')" :aria-busy="store.loading || restoringFeed" data-testid="cat-group-view">
    <div class="cg-main">
      <header class="cg-hd">
        <span class="ttl" data-testid="cat-group-title">{{ g?.name }}</span>
        <span class="sub">{{ t('cat.groupChat.membersCount', { n: store.members.length }) }}</span>
        <span class="sp" />
        <button type="button" class="cg-ib" :class="{ on: store.membersOpen }" :aria-label="t('cat.groupChat.members')" :aria-expanded="store.membersOpen" data-testid="cat-group-members-toggle" @click="store.membersOpen = !store.membersOpen">
          <span class="cg-stack" aria-hidden="true"><span v-for="m in store.membersSorted.slice(0, 4)" :key="m.id" class="av sm" :style="{ '--c': memberColorVar(m.order) }">{{ initial(m.name) }}</span></span>
        </button>
        <button type="button" class="cg-ib" :class="{ on: !rightCollapsed }" :title="rightCollapsed ? t('cat.page.s_f1f1ec2556') : t('cat.page.s_4ed8aa623c')" :aria-label="rightCollapsed ? t('cat.page.s_8fa248207b') : t('cat.page.s_74609dd7e9')" :aria-pressed="!rightCollapsed" data-testid="cat-group-right-toggle" @click="rightCollapsed = !rightCollapsed"><FIcon name="split" :size="15" /></button>
        <button type="button" class="cg-ib" :title="t('cat.groupChat.settings')" :aria-label="t('cat.groupChat.settings')" data-testid="cat-group-settings" @click="store.settingsOpen = true">
          <FIcon name="set" :size="15" />
        </button>
      </header>

      <CatGroupWorkflow v-if="store.workflows.length" />

      <div ref="feedEl" class="cg-feed" data-testid="cat-group-feed" @scroll.passive="onFeedScroll">
        <div ref="feedContentEl" class="cg-feed-in">
          <button v-if="store.hasMore" type="button" class="cg-btn ghost" style="align-self: center" @click="store.loadOlder()">{{ t('cat.groupChat.loadOlder') }}</button>
          <div v-if="!store.loading && !store.messages.length" class="cg-empty" data-testid="cat-group-empty">
            <div class="ill" aria-hidden="true"><FIcon name="group" :size="30" /></div>
            <h3>{{ store.members.length ? t('cat.groupChat.emptyTitle') : t('cat.groupChat.noMembersYet') }}</h3>
            <p>{{ t('cat.groupChat.emptySub') }}</p>
          </div>
          <template v-for="(m, messageIndex) in visibleMessages" :key="m.seq">
            <div v-if="m.kind === 'user'" class="cg-task" :data-message-seq="m.seq" :class="{ continued: consecutiveMessage(messageIndex) }" :aria-label="t('cat.groupChat.you')" :title="clock(m.createdAt)" data-testid="cat-group-user-msg">
              <div class="tx"><template v-for="(p, i) in parts(m.text)" :key="i"><span v-if="p.at" class="cg-at">{{ p.s }}</span><template v-else>{{ p.s }}</template></template></div>
              <div v-if="m.files?.length" class="cg-images sent" data-testid="cat-group-message-images">
                <button v-for="file in m.files" :key="file.path" type="button" class="cg-image" :aria-label="file.name" @click="previewFile = shownImage(file)"><CatThumb :file="shownImage(file)" /></button>
              </div>
            </div>

            <div v-else-if="m.kind === 'member' || m.systemKind === 'ack' || m.systemKind === 'retry'" class="cg-chat" :data-message-seq="m.seq" :class="{ continued: consecutiveMessage(messageIndex) }" :style="{ '--speaker': colorOf(m.memberId) }">
              <span class="av cg-chat-avatar" aria-hidden="true">{{ initial(store.nameOf(m.memberId)) }}</span>
              <div class="cg-chat-body">
                <div v-if="!consecutiveMessage(messageIndex)" class="cg-chat-name">{{ store.nameOf(m.memberId) }}</div>
                <div v-if="m.systemKind === 'ack' || m.systemKind === 'retry' || isMemberShortReply(m) || !m.report" class="cg-short" :title="clock(m.createdAt)" :data-kind="m.systemKind" :data-testid="m.systemKind === 'ack' ? 'cat-group-ack' : m.systemKind === 'retry' ? 'cat-group-retry-status' : 'cat-group-short'" :role="m.systemKind === 'ack' || m.systemKind === 'retry' ? 'status' : undefined">
                  <div class="tx"><template v-for="(p, i) in parts(memberShortText(m))" :key="i"><span v-if="p.at" class="cg-at">{{ p.s }}</span><template v-else>{{ p.s }}</template></template></div>
                  <button type="button" class="cg-msg-more" :aria-label="t('cat.groupChat.viewProcess')" :title="t('cat.groupChat.viewProcess')" @click="openProcess(m.memberId)"><FIcon name="eye" :size="13" /></button>
                </div>

                <article v-else-if="m.report" class="cg-card" :class="{ over: overreach(m).length && !reverted[m.seq] }" :title="clock(m.createdAt)" data-testid="cat-group-report">
              <div class="cg-sec">
                <p><template v-for="(p, i) in parts(m.report.did)" :key="i"><span v-if="p.at" class="cg-at">{{ p.s }}</span><template v-else>{{ p.s }}</template></template></p>
              </div>
              <div v-if="m.report.parsed && m.report.files.length" class="cg-sec">
                <button type="button" class="cg-fsum" :aria-expanded="!!expanded[m.seq]" @click="expanded[m.seq] = !expanded[m.seq]">
                  <FIcon name="right" :size="12" :class="{ rot: expanded[m.seq] }" />
                  {{ t('cat.groupChat.filesN', { n: m.report.files.length }) }}
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
                  <button type="button" class="cg-msg-more" :aria-label="t('cat.groupChat.viewProcess')" :title="t('cat.groupChat.viewProcess')" @click="openProcess(m.memberId)"><FIcon name="eye" :size="13" /></button>
                </article>
              </div>
            </div>

            <article v-else-if="m.systemKind === 'workflow_summary'" class="cg-result" :data-message-seq="m.seq" data-testid="cat-group-result">
              <h3><FIcon name="check" :size="15" />{{ t('cat.groupChat.workflow_completed') }}</h3>
              <p>{{ workflowSummary(m) }}</p>
            </article>

            <div v-else-if="m.systemKind === 'failed'" class="cg-sys fail" :data-message-seq="m.seq" role="alert" data-testid="cat-group-failed">
              <FIcon name="warn" :size="12" />
              <span>{{ t('cat.groupChat.failed', { name: store.nameOf(m.memberId), reason: m.failReason || m.text || t('cat.groupChat.unknownReason') }) }}</span>
              <span class="bar" />
              <button v-if="!retried[m.seq]" type="button" class="act" :title="t('cat.groupChat.retryTip')" data-testid="cat-group-retry" @click="retry(m)"><FIcon name="retry" :size="12" />{{ t('cat.groupChat.retry') }}</button>
              <span v-else>{{ t('cat.groupChat.retrying') }}</span>
              <button type="button" class="act" @click="openProcess(m.memberId)">{{ t('cat.groupChat.viewProcess') }}</button>
            </div>

            <div v-else-if="m.systemKind === 'stopped'" class="cg-sys" :data-message-seq="m.seq" data-testid="cat-group-stopped">
              <FIcon name="stop" :size="11" />{{ sysText(m) }}<template v-if="m.toMemberId">{{ t('cat.groupChat.stoppedBySwap') }}</template>
            </div>

            <template v-else-if="m.systemKind === 'report_format'" />

            <!-- v0.57：旧静默「交给」灰字（ai 成功且已有成员）隐藏；加载「正在安排…」与 queued/followup/undecided 仍显示 -->
            <div v-else class="cg-sysw" :data-message-seq="m.seq">
              <div class="cg-sys" :data-kind="m.systemKind" data-testid="cat-group-system">
                <FIcon :name="sysIcon(m)" :size="12" />
                <span>{{ m.route?.mode === 'undecided' ? t('cat.groupChat.routeRetryHint') : sysText(m) }}</span>
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
            </div>
          </template>
          <div v-if="store.routingNow" class="cg-sys dot">{{ t('cat.groupChat.routing') }}</div>
        </div>
      </div>

      <div v-if="store.workingMembers.length && !activeWorkflow" class="cg-work" data-testid="cat-group-working">
        <div v-for="m in store.workingMembers" :key="m.id" class="cg-wrow">
          <span v-if="store.workingMembers.length === 1" class="cg-pulse" aria-hidden="true" />
          <span class="av sm" :style="{ '--c': memberColorVar(m.order) }">{{ initial(m.name) }}</span>
          <span class="tx"><b>{{ m.name }}</b> · {{ t('cat.groupChat.workflow_running') }}</span>
          <button type="button" class="lkb" @click="openProcess(m.id)">{{ t('cat.groupChat.viewProcess') }}</button>
          <button type="button" class="lkb" @click="store.stop(m.id)">{{ t('cat.groupChat.stop') }}</button>
        </div>
        <div v-if="store.workingMembers.length > 1" class="cg-wall">
          <span class="cg-pulse" aria-hidden="true" /><b>{{ t('cat.groupChat.nWorking', { n: store.workingMembers.length }) }}</b>
          <span class="sp" />
          <button type="button" class="lkb red" data-testid="cat-group-stop-all" @click="store.stopAll()">{{ t('cat.groupChat.stopAll') }}</button>
        </div>
      </div>

      <div class="cg-comp" :class="{ 'drag-over': dragOver }" :style="{ '--wails-drop-target': 'drop' }" data-testid="cat-group-composer" @dragenter.prevent="dragOver = true" @dragover.prevent="dragOver = true" @dragleave="onDragLeave" @drop.prevent="onDrop">
        <div v-if="images.length" class="cg-images" data-testid="cat-group-draft-images">
          <div v-for="file in images" :key="file.path" class="cg-image">
            <button type="button" class="cg-image-preview" :aria-label="file.name" @click="previewFile = shownImage(file)"><CatThumb :file="shownImage(file)" /></button>
            <button type="button" class="cg-image-remove" :aria-label="t('cat.groupChat.removeImage', { name: file.name })" @click="images = images.filter((f) => f.path !== file.path)"><FIcon name="x" :size="12" /></button>
          </div>
        </div>
        <div v-if="mentionOpen && mentionList.length" class="cg-pop" role="listbox" :aria-label="t('cat.groupChat.mentionList')">
          <div class="ph">{{ AT }} {{ t('cat.groupChat.members') }}</div>
          <button v-for="(o, i) in mentionList" :key="o.id" type="button" class="it" :class="{ on: i === mentionIdx }" role="option" :aria-selected="i === mentionIdx" @mousedown.prevent="pickMention(o.id)">
            <span class="av sm" :style="{ '--c': memberColorVar(o.order) }">{{ initial(o.name) }}</span>{{ o.name }}<span class="tt">{{ o.title }}</span>
            <span class="sp" /><span class="tt" :class="{ busy: store.live[o.id] }"><i v-if="store.live[o.id]" class="cg-breath" />{{ store.live[o.id] ? t('cat.groupChat.busy') : t('cat.groupChat.idle') }}</span>
          </button>
        </div>
        <div class="row">
          <span v-for="id in chips" :key="id" class="cg-chip">
            <span class="cg-chip-label">{{ AT }}{{ store.nameOf(id) }}</span>
            <button type="button" :aria-label="t('cat.groupChat.removeMention', { name: store.nameOf(id) })" @click="removeMention(id)"><FIcon name="x" :size="10" /></button>
          </span>
          <textarea ref="inputEl" v-model="draft" rows="2" :placeholder="chips.length ? '' : t('cat.groupChat.composerPh')" data-testid="cat-group-input" @input="onInput" @keydown="onKey" @paste="onPaste" />
        </div>
        <div class="bar">
          <input ref="imageInput" type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple hidden data-testid="cat-group-image-input" @change="onImageInput" />
          <button type="button" class="cg-ib" :disabled="uploading" :aria-label="t('cat.groupChat.attachImage')" :title="t('cat.groupChat.attachImage')" data-testid="cat-group-add-image" @click="pickImages"><FIcon name="image" :size="17" /></button>
          <button type="button" class="cg-ib" :aria-label="t('cat.groupChat.mentionList')" @click="openMention()">{{ AT }}</button>
          <span class="sp" />
          <button type="button" class="cg-send" :disabled="!canSend" :aria-label="t('cat.groupChat.send')" data-testid="cat-group-send" @click="send()"><FIcon name="send" :size="15" /></button>
        </div>
      </div>
    </div>

    <CatPreviewDialog :open="!!previewFile" :file="previewFile" @update:open="(open) => { if (!open) previewFile = null }" />
    <div v-if="store.membersOpen" class="cg-scrim" @click="store.membersOpen = false" />
    <CatGroupMembers :class="{ open: store.membersOpen }" :closable="true" :initial-open="initialOpen" @close="store.membersOpen = false" @add="addOpen = true" />
    <CatGroupCreateDialog v-if="addOpen" mode="member" @close="addOpen = false" />

    <div v-if="store.drawerMemberId" class="cg-scrim on" @click="store.drawerMemberId = ''" />
    <aside v-if="store.drawerMemberId" class="cg-proc" role="dialog" :aria-label="t('cat.groupChat.viewProcess')" data-testid="cat-group-process">
      <div class="cg-proc-hd">
        <span class="av" :style="{ '--c': colorOf(store.drawerMemberId) }">{{ initial(store.nameOf(store.drawerMemberId)) }}</span>
        <div class="tt">
          <b>{{ store.nameOf(store.drawerMemberId) }} · {{ t('cat.groupChat.viewProcess') }}</b>
          <span>{{ t('cat.groupChat.processSubtitle') }}</span>
        </div>
        <button type="button" class="cg-ib" :aria-label="t('cat.groupChat.close')" @click="store.drawerMemberId = ''"><FIcon name="x" :size="14" /></button>
      </div>
      <div ref="procBody" class="bd" @scroll.passive="onProcScroll">
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
import { formatElapsed, isMemberShortReply, memberColorVar, memberShortText, systemLineText, type CatGroupMessage } from '@/api/catGroupChat'
import { SESSION_DEMO } from '@/api/catGroupChatMock'
import { catPanels } from '@/views/cat/catPanels'
import { rightCollapsed } from '@/views/cat/catLayout'
import { catState } from '@/views/cat/catState'
import { readGroupPosition, saveGroupPosition } from '@/views/cat/catGroupPosition'
import { toAppError } from '@/api/call'
import { catPane } from '@/components/cat/catPreviewWidth'
import { clipboardFilePaths, clipboardImage, pickFiles, savePastedFile } from '@/api/system'
import { onFilesDropped } from '@/api/fileDrop'
import { hasWailsBackend } from '@/services/wails'
import { fileBaseName, CAT_ATTACH_MAX } from '@/components/cat/catAttach'
import type { CatGroupAttachment } from '@/api/catGroupChat'
import type { CatShownFile } from '@/components/cat/catMedia'
import CatThumb from '@/components/cat/CatThumb.vue'
import CatPreviewDialog from '@/components/cat/CatPreviewDialog.vue'

import CatGroupMembers from './CatGroupMembers.vue'
import CatGroupCreateDialog from './CatGroupCreateDialog.vue'
import CatGroupSettings from './CatGroupSettings.vue'
import CatGroupCharter from './CatGroupCharter.vue'
import CatGroupWorkflow from './CatGroupWorkflow.vue'
import './catGroup.css'

const { t } = useI18n()
const store = useCatGroupChat()
const g = computed(() => store.group)
watch(() => catState.projects.find((p) => p.id === g.value?.projectId), (project) => {
  if (project) project.open = true
}, { immediate: true })
const AT = '@'
const everyoneLabel = computed(() => t('cat.groupChat.everyone'))
const activeWorkflow = computed(() => store.workflows.some((w) => w.status === 'running' || w.status === 'blocked'))
const workflowChains = computed(() => new Set(store.workflows.map((w) => w.chainId)))
const visibleMessages = computed(() => store.messages.filter((m) => {
  if (m.kind === 'user' || m.kind === 'member' || m.systemKind === 'workflow_summary') return true
  if (['coord', 'report_format', 'no_handoff'].includes(m.systemKind ?? '')) return false
  if (m.systemKind === 'route') return m.route?.mode === 'undecided'
  if (workflowChains.value.has(m.chainId) && ['failed', 'stopped', 'reassign'].includes(m.systemKind ?? '')) return false
  return true
}))
function workflowSummary(m: CatGroupMessage) {
  const w = store.workflows.find((x) => x.chainId === m.chainId)
  return w ? w.tasks.map((task) => task.result).filter(Boolean).join('\n\n') : m.text
}

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
function consecutiveMessage(index: number) {
  const current = visibleMessages.value[index]
  const previous = visibleMessages.value[index - 1]
  const currentKind = current.systemKind === 'ack' || current.systemKind === 'retry' ? 'member' : current.kind
  const previousKind = previous?.systemKind === 'ack' || previous?.systemKind === 'retry' ? 'member' : previous?.kind
  return !!previous && ['user', 'member'].includes(currentKind) && currentKind === previousKind && current.memberId === previous.memberId && current.createdAt - previous.createdAt < 5 * 60_000
}
const sysText = (m: CatGroupMessage) => systemLineText(m, store.nameOf)
const overreach = (m: CatGroupMessage) => (m.report?.files ?? []).filter((f) => f.tag === 'overreach')

const drawerBlocks = computed(() => (store.drawerMemberId ? (store.processBlocks[store.drawerMemberId] ?? []) : []))
function toolParts(text: string): { name: string; arg: string; out: string } {
  const lines = text.split('\n')
  const first = lines[0] ?? ''
  const m = /^(\S+)\s*(.*)$/.exec(first)
  return { name: m?.[1] ?? 'tool', arg: m?.[2] ?? '', out: lines.slice(1).join('\n') }
}
const drawerRows = computed(() =>
  drawerBlocks.value.map((b) => {
    if (b.kind !== 'tool') return { id: b.id, kind: b.kind, text: b.text.split('<cat_task_result>')[0].trimEnd(), streaming: b.streaming, name: '', arg: '', out: '' }
    const p = toolParts(b.text)
    return { id: b.id, kind: b.kind, text: b.text, streaming: b.streaming, name: p.name, arg: p.arg, out: p.out }
  }),
)
const procBody = ref<HTMLElement | null>(null)
let stickProcess = true
function onProcScroll() {
  const el = procBody.value
  if (!el) return
  stickProcess = el.scrollHeight - el.scrollTop - el.clientHeight < 48
}
function pinProcessBottom() {
  const el = procBody.value
  if (el) el.scrollTop = el.scrollHeight
}
watch(() => store.drawerMemberId, async (id) => {
  if (!id) return
  stickProcess = true
  await nextTick()
  pinProcessBottom()
  requestAnimationFrame(pinProcessBottom)
})
watch(
  () => {
    const rows = drawerRows.value
    const last = rows[rows.length - 1]
    const loading = store.drawerMemberId ? store.processLoading[store.drawerMemberId] : false
    return rows.length + ':' + (last ? last.text.length + last.out.length : 0) + (loading ? ':1' : ':0')
  },
  async () => {
    if (!store.drawerMemberId || !stickProcess) return
    await nextTick()
    pinProcessBottom()
    requestAnimationFrame(pinProcessBottom)
  },
)
function openProcess(id?: string) {
  if (!id) return
  void store.openProcess(id)
}

/** 正文里的 @成员名 / @全体 高亮（最长名优先；没匹配上的保持普通文字） */
function parts(text: string): { s: string; at: boolean }[] {
  const names = [
    ...store.members.map((m) => m.name).filter(Boolean),
    everyoneLabel.value,
  ].sort((a, b) => b.length - a.length)
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
  if (workflowChains.value.has(m.chainId)) return false
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

const dirOf = (p: string) => p.slice(0, p.lastIndexOf('/') + 1)
const baseOf = (p: string) => p.slice(p.lastIndexOf('/') + 1)
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
// Keep an in-flight send or image import bound to the group where it started.
const drafts = reactive<Record<string, { text: string; chips: string[]; files: CatGroupAttachment[]; sending: boolean; uploads: number }>>({})
const draftSlot = computed(() => {
  const id = catPanels.activeGroupId
  if (!drafts[id]) drafts[id] = { text: '', chips: [], files: [], sending: false, uploads: 0 }
  return drafts[id]
})
const draft = computed({ get: () => draftSlot.value.text, set: (v) => { draftSlot.value.text = v } })
const chips = computed({ get: () => draftSlot.value.chips, set: (v) => { draftSlot.value.chips = v } })
const images = computed({ get: () => draftSlot.value.files, set: (v) => { draftSlot.value.files = v } })
const uploading = computed(() => draftSlot.value.uploads > 0)
const imageInput = ref<HTMLInputElement | null>(null)
const dragOver = ref(false)
const previewFile = ref<CatShownFile | null>(null)
const shownImage = (file: CatGroupAttachment): CatShownFile => ({ ...file, kind: 'image' })
const isImage = (name: string) => /\.(png|jpe?g|gif|webp)$/i.test(name)
function attachPaths(paths: string[], slot = draftSlot.value) {
  let invalid = false, capped = false
  for (const path of paths) {
    if (!isImage(path)) { invalid = true; continue }
    if (slot.files.some((f) => f.path.toLowerCase() === path.toLowerCase())) continue
    if (slot.files.length >= CAT_ATTACH_MAX) { capped = true; break }
    slot.files.push({ name: fileBaseName(path), path })
  }
  if (invalid) ElMessage.warning(t('cat.groupChat.imageFormats'))
  if (capped) ElMessage.warning(t('cat.groupChat.imageLimit'))
}
async function pickImages() {
  if (!hasWailsBackend()) { imageInput.value?.click(); return }
  const slot = draftSlot.value
  try { attachPaths(await pickFiles({ name: t('cat.groupChat.attachImage'), patterns: ['*.png', '*.jpg', '*.jpeg', '*.gif', '*.webp'] }, true), slot) }
  catch { ElMessage.warning(t('cat.groupChat.imageFailed')) }
}
async function importImages(files: File[]) {
  const slot = draftSlot.value
  slot.uploads++
  try {
    for (const file of files) {
      if (slot.files.length >= CAT_ATTACH_MAX) { ElMessage.warning(t('cat.groupChat.imageLimit')); break }
      if (!/^image\/(png|jpeg|gif|webp)$/.test(file.type) && !isImage(file.name)) { ElMessage.warning(t('cat.groupChat.imageFormats')); continue }
      if (file.size > 15 * 1024 * 1024) { ElMessage.warning(t('cat.groupChat.imageTooLarge')); continue }
      const path = (file as File & { path?: string }).path || await savePastedFile(file.name || 'paste.png', new Uint8Array(await file.arrayBuffer()))
      if (!path) throw new Error('image save failed')
      attachPaths([path], slot)
    }
  } catch { ElMessage.warning(t('cat.groupChat.imageFailed')) }
  finally { slot.uploads-- }
}
async function onImageInput(e: Event) {
  const el = e.target as HTMLInputElement
  const files = Array.from(el.files ?? [])
  el.value = ''
  await importImages(files)
}
function onDragLeave(e: DragEvent) {
  if (!(e.relatedTarget instanceof Node) || !(e.currentTarget as HTMLElement).contains(e.relatedTarget)) dragOver.value = false
}
async function onDrop(e: DragEvent) {
  dragOver.value = false
  const files = Array.from(e.dataTransfer?.files ?? [])
  // Native drops are delivered once by the Wails callback with absolute paths.
  if (hasWailsBackend()) {
    attachPaths(files.map((f) => (f as File & { path?: string }).path).filter((p): p is string => !!p))
  } else if (files.length) await importImages(files)
}
async function onPaste(e: ClipboardEvent) {
  const data = e.clipboardData
  const files = Array.from(data?.items ?? []).filter((item) => item.kind === 'file' && item.type.startsWith('image/')).map((item) => item.getAsFile()).filter((f): f is File => !!f)
  if (files.length) { e.preventDefault(); await importImages(files); return }
  if (data?.getData('text/plain')) return
  e.preventDefault()
  const slot = draftSlot.value
  slot.uploads++
  try {
    const paths = await clipboardFilePaths()
    if (paths.length) attachPaths(paths, slot)
    else { const image = await clipboardImage(); if (image) attachPaths([image], slot) }
  } finally { slot.uploads-- }
}
const inputEl = ref<HTMLTextAreaElement | null>(null)
const mentionOpen = ref(false)
const mentionQuery = ref('')
const mentionIdx = ref(0)
const mentionList = computed(() => store.membersSorted.filter((m) => !chips.value.includes(m.id) && m.name.toLowerCase().includes(mentionQuery.value.toLowerCase())))
const canSend = computed(() => (!!draft.value.trim() || images.value.length > 0) && store.members.length > 0 && !store.loading && !uploading.value && !draftSlot.value.sending)
function onInput() {
  const el = inputEl.value
  if (!el) return
  const hit = /@([^\s@]*)$/.exec(draft.value.slice(0, el.selectionStart ?? draft.value.length))
  mentionOpen.value = !!hit
  mentionQuery.value = hit?.[1] ?? ''
  mentionIdx.value = 0
}
function openMention() {
  const el = inputEl.value
  const start = el?.selectionStart ?? draft.value.length
  const end = el?.selectionEnd ?? start
  draft.value = draft.value.slice(0, start) + AT + draft.value.slice(end)
  void nextTick(() => {
    el?.focus()
    el?.setSelectionRange(start + 1, start + 1)
    onInput()
  })
}
function pickMention(id: string) {
  const el = inputEl.value
  const pos = el?.selectionStart ?? draft.value.length
  const before = draft.value.slice(0, pos).replace(/@[^\s@]*$/, '')
  draft.value = before + draft.value.slice(pos)
  if (!chips.value.includes(id)) chips.value.push(id)
  mentionOpen.value = false
  void nextTick(() => {
    el?.focus()
    el?.setSelectionRange(before.length, before.length)
  })
}
function removeMention(id: string) {
  chips.value = chips.value.filter((x) => x !== id)
  inputEl.value?.focus()
}
function onKey(e: KeyboardEvent) {
  // Let the input method confirm Chinese text before handling mention/send keys.
  if (e.isComposing || e.keyCode === 229) return
  const list = mentionList.value
  if (mentionOpen.value && e.key === 'Escape') { e.preventDefault(); mentionOpen.value = false; return }
  if (mentionOpen.value && list.length) {
    if (e.key === 'ArrowDown') { e.preventDefault(); mentionIdx.value = (mentionIdx.value + 1) % list.length; return }
    if (e.key === 'ArrowUp') { e.preventDefault(); mentionIdx.value = (mentionIdx.value - 1 + list.length) % list.length; return }
    if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); pickMention(list[mentionIdx.value].id); return }
  }
  if (e.key === 'Backspace' && chips.value.length && inputEl.value?.selectionStart === 0 && inputEl.value.selectionEnd === 0) {
    e.preventDefault()
    chips.value.pop()
  }
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) { e.preventDefault(); void send() }
}
async function send() {
  if (!canSend.value) return
  const mentions = [...chips.value]
  const text = [...mentions.map((id) => `${AT}${store.nameOf(id)}`), draft.value.trim()].join(' ')
  const slot = draftSlot.value
  const keep = { draft: slot.text, chips: mentions, files: [...slot.files] }
  slot.text = ''
  slot.chips = []
  slot.files = []
  slot.sending = true
  scrollEnd()
  try {
    await store.send(text, mentions, keep.files)
  } catch {
    slot.text = [keep.draft, slot.text].filter(Boolean).join('\n')
    slot.chips = [...new Set([...keep.chips, ...slot.chips])]
    slot.files = [...keep.files, ...slot.files.filter((f) => !keep.files.some((old) => old.path === f.path))]
    ElMessage.error(t('cat.groupChat.sendFailed'))
  } finally { slot.sending = false }
}

const feedEl = ref<HTMLElement | null>(null)
const feedContentEl = ref<HTMLElement | null>(null)
let followingLatest = true
const restoringFeed = ref(false)
let readingGroupId = ''
let positionTimer: ReturnType<typeof setTimeout> | undefined
let feedResize: ResizeObserver | null = null
let scrollFrame = 0
function rememberFeedPosition() {
  clearTimeout(positionTimer)
  const el = feedEl.value
  if (!el || !readingGroupId || restoringFeed.value || store.group?.id !== readingGroupId) return
  const top = el.getBoundingClientRect().top
  const anchor = Array.from(el.querySelectorAll<HTMLElement>('[data-message-seq]')).find((row) => row.getBoundingClientRect().bottom > top)
  saveGroupPosition(readingGroupId, {
    atBottom: followingLatest,
    seq: Number(anchor?.dataset.messageSeq) || 0,
    offset: anchor ? anchor.getBoundingClientRect().top - top : 0,
  })
}
function onFeedScroll() {
  if (restoringFeed.value) return
  const el = feedEl.value
  if (el) followingLatest = el.scrollHeight - el.clientHeight - el.scrollTop < 64
  clearTimeout(positionTimer)
  positionTimer = setTimeout(rememberFeedPosition, 150)
}
function scrollEnd() {
  followingLatest = true
  void nextTick(() => {
    if (!followingLatest) return
    const el = feedEl.value
    if (el) el.scrollTop = el.scrollHeight
    // Reconcile viewport changes from the composer and running-task strip.
    cancelAnimationFrame(scrollFrame)
    scrollFrame = requestAnimationFrame(() => {
      const feed = feedEl.value
      if (feed && followingLatest) feed.scrollTop = feed.scrollHeight
    })
  })
}
watch(() => visibleMessages.value.at(-1)?.seq, (seq, previous) => {
  if (restoringFeed.value || seq === undefined || seq === previous) return
  if (visibleMessages.value.at(-1)?.kind === 'user' || followingLatest) scrollEnd()
}, { flush: 'post' })

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
  if (ui?.swapOpen) { const m = [...store.messages].reverse().find((x) => x.systemKind === 'route'); if (m) reassignFor.value = m.seq }
  if (s === 'members' || s === 'narrow-drawer') store.membersOpen = true
  if (s === 'members') initialOpen.value = 'b'
  if (s === 'rolegen') rolegenOpen.value = true
  if (s === 'session') store.setProcessDemo('f', SESSION_DEMO)
  if (s === 'mention') { draft.value = AT; mentionOpen.value = true; mentionQuery.value = '' }
  if (s === 'narrow-single') catPane.side = 264 // 1024 窗口：左栏 264 + 中栏 760，右栏收成抽屉（设计走查 建议 9）
}
function onDeleted() {
  saveGroupPosition(catPanels.activeGroupId, null)
  readingGroupId = ''
  store.settingsOpen = false
  catPanels.activeGroupId = ''
  catPanels.view = 'chat'
}

watch(
  () => catPanels.activeGroupId,
  async (id, _previous, onCleanup) => {
    rememberFeedPosition()
    if (!id) return
    let cancelled = false
    onCleanup(() => { cancelled = true })
    restoringFeed.value = true
    readingGroupId = ''
    cancelAnimationFrame(scrollFrame)
    const position = readGroupPosition(id)
    followingLatest = position?.atBottom ?? true
    try {
      await store.open(id)
      if (cancelled) return
      // A saved anchor can be older than the latest page. Load back to it before
      // measuring, so refresh never silently replaces history with the latest 50.
      while (position && !position.atBottom && position.seq > 0 && store.hasMore && store.messages[0]?.seq > position.seq) {
        const first = store.messages[0].seq
        await store.loadOlder(200)
        if (cancelled) return
        if (store.messages[0]?.seq === first) break
      }
      applyScene()
      await nextTick()
      if (cancelled) return
      const feed = feedEl.value
      if (feed) {
        if (store.sceneUi?.scroll === 'top') { followingLatest = false; feed.scrollTop = 0 }
        else if (position && !position.atBottom) {
          const anchor = feed.querySelector<HTMLElement>(`[data-message-seq="${position.seq}"]`)
          if (anchor) feed.scrollTop += anchor.getBoundingClientRect().top - feed.getBoundingClientRect().top - position.offset
          else feed.scrollTop = 0
        } else feed.scrollTop = feed.scrollHeight
      }
      await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
      if (!cancelled) readingGroupId = id
    } catch (e) {
      if (cancelled) return
      if (toAppError(e).code === 'NOT_FOUND') onDeleted()
      else ElMessage.error(toAppError(e).message)
    } finally {
      if (!cancelled) {
        restoringFeed.value = false
        if (followingLatest) scrollEnd()
      }
    }
  },
  { immediate: true },
)
let offImageDrop: (() => void) | undefined
onMounted(() => {
  window.addEventListener('pagehide', rememberFeedPosition)
  offImageDrop = onFilesDropped((paths) => { dragOver.value = false; attachPaths(paths) })
  feedResize = new ResizeObserver(() => {
    if (!restoringFeed.value && followingLatest) scrollEnd()
  })
  if (feedEl.value) feedResize.observe(feedEl.value)
  if (feedContentEl.value) feedResize.observe(feedContentEl.value)
  if (!rootEl.value) return
  width.value = rootEl.value.clientWidth
  ro = new ResizeObserver(([e]) => (width.value = e.contentRect.width))
  ro.observe(rootEl.value)
})
onBeforeUnmount(() => {
  rememberFeedPosition()
  window.removeEventListener('pagehide', rememberFeedPosition)
  ro?.disconnect(); feedResize?.disconnect(); cancelAnimationFrame(scrollFrame); offImageDrop?.()
})
void addOpen
void initialOpen
void layout
</script>
