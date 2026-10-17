<template>
  <div class="team-shell" :class="{ collapsed }">
    <aside class="team-sidebar">
      <header class="brand"><FIcon name="bot" :size="20" /><strong v-if="!collapsed">Agent</strong></header>
      <button class="collapse" :title="collapsed ? '展开侧栏' : '收起侧栏'" :aria-expanded="!collapsed" @click="collapsed = !collapsed"><FIcon :name="collapsed ? 'right' : 'left'" :size="13" /></button>
      <RouterLink class="nav" to="/cat" title="单聊"><FIcon name="chat" :size="17" /><span v-if="!collapsed">单聊</span></RouterLink>
      <button class="nav" :class="{ on: view === 'rooms' }" title="协作群" @click="view = 'rooms'"><FIcon name="users" :size="17" /><span v-if="!collapsed">协作群</span><b v-if="unread" class="count">{{ unread }}</b></button>
      <button class="nav" :class="{ on: view === 'employees' }" title="数字员工" @click="view = 'employees'"><FIcon name="bot" :size="17" /><span v-if="!collapsed">数字员工</span></button>
      <button class="nav" :class="{ on: view === 'inbox' }" title="@ 我的" @click="openInbox"><FIcon name="bell" :size="17" /><span v-if="!collapsed">@ 我的</span></button>
      <div v-if="!collapsed" class="rooms">
        <div class="section-label">群聊<button title="新建群聊" aria-label="新建群聊" @click="newRoom"><FIcon name="plus" :size="15" /></button></div>
        <button v-for="room in teamState.bootstrap?.rooms" :key="room.id" class="room-row" :class="{ on: teamState.selected === room.id && view === 'rooms' }" @click="openRoom(room.id)"><FIcon name="chat" :size="15" /><span>{{ room.name }}</span><small v-if="room.paused">暂停</small><small v-if="room.archived">归档</small></button>
        <p v-if="!teamState.bootstrap?.rooms.length" class="muted">新建群，邀请员工一起工作</p>
      </div>
      <footer>
        <button class="nav" :title="teamState.bootstrap?.human.name || '本人资料'" @click="editHuman"><span class="avatar small">{{ teamState.bootstrap?.human.name?.slice(0, 1) || '我' }}</span><span v-if="!collapsed">{{ teamState.bootstrap?.human.name }} · {{ teamState.bootstrap?.human.role }}</span></button>
        <RouterLink class="nav" to="/" title="回到工具箱"><FIcon name="toolbox" :size="17" /><span v-if="!collapsed">回到工具箱</span></RouterLink>
        <button class="nav" title="设置" @click="settings"><FIcon name="set" :size="17" /><span v-if="!collapsed">设置</span></button>
      </footer>
    </aside>

    <section v-if="view === 'employees'" class="directory">
      <header class="page-header"><div><h2>数字员工</h2><p>定义岗位和工作要求，为每位员工选择 CLI。</p></div><button class="primary" @click="editEmployee()"><FIcon name="plus" :size="15" />新建员工</button></header>
      <div v-if="!teamState.bootstrap?.employees.length" class="empty"><FIcon name="bot" :size="32" /><h3>组建你的数字团队</h3><p>员工可以使用同一个 CLI，各自拥有独立身份与任务会话。</p><button class="primary" @click="editEmployee()">创建第一位员工</button></div>
      <div class="employee-grid"><button v-for="employee in teamState.bootstrap?.employees" :key="employee.id" class="employee-card" @click="editEmployee(employee)"><span class="avatar">{{ employee.avatar || employee.name.slice(0, 1) }}</span><div><h3>{{ employee.name }} <small v-if="!employee.enabled">已停用</small></h3><p>{{ employee.role || '未设置岗位' }}</p><small>{{ engine(employee.agentKind) }} · {{ employee.model || '默认模型' }}</small><p class="summary">{{ employee.specialties || '添加专长和交付标准' }}</p></div><FIcon name="edit" :size="15" /></button></div>
    </section>

    <section v-else-if="view === 'inbox'" class="directory inbox">
      <header class="page-header"><div><h2>@ 我的</h2><p>员工的汇报、问题和需要你决定的事项。</p></div><button :class="{ on: onlyQuestions }" @click="onlyQuestions = !onlyQuestions">{{ onlyQuestions ? '显示全部' : '只看待答复' }}</button></header>
      <button v-for="item in inboxItems" :key="item.id" class="inbox-item" :class="{ unread: !item.readAt }" @click="openRoom(item.roomId, item.messageId)"><FIcon :name="item.needsReply && !item.answeredAt ? 'chat' : 'bell'" :size="18" /><div><strong>{{ roomName(item.roomId) }}</strong><p>{{ item.needsReply && !item.answeredAt ? '有员工等待你答复' : '有员工 @ 你' }}</p><small>{{ time(item.createdAt) }} · {{ item.readAt ? '已读' : '未读' }}{{ item.answeredAt ? ' · 已答复' : '' }}</small></div><FIcon name="right" :size="15" /></button>
      <div v-if="!inboxItems.length" class="empty"><FIcon name="check" :size="30" /><p>暂时没有{{ onlyQuestions ? '待答复事项' : '收到的 @ 消息' }}</p></div>
    </section>

    <template v-else>
      <section v-if="snapshot" class="conversation">
        <header class="conversation-header"><div><h2>{{ snapshot.room.name }}</h2><p>{{ members.length }} 位成员<span v-if="working"> · {{ working }} 位员工工作中</span><span v-if="snapshot.room.paused"> · 已暂停派活</span></p></div><button title="成员与任务" @click="rightOpen = !rightOpen"><FIcon name="list" :size="18" /></button><button title="群设置" @click="editRoom"><FIcon name="set" :size="17" /></button></header>
        <div v-if="teamState.error || actionError" class="notice error" role="alert">{{ actionError || teamState.error }}<button aria-label="关闭提示" @click="actionError = ''; teamState.error = ''">×</button></div>
        <div v-if="snapshot.room.archived" class="notice">此群已归档，历史记录仍可查看。</div>
        <div ref="messagesEl" class="messages" @scroll="onScroll">
          <button v-if="snapshot.hasEarlier" class="load-earlier" :disabled="loadingEarlier" @click="loadEarlier">{{ loadingEarlier ? '加载中…' : '加载更早消息' }}</button>
          <div v-if="!snapshot.messages.length" class="empty"><FIcon name="chat" :size="30" /><h3>从一个 @ 开始</h3><p>给员工派活，或先和协调人讨论。员工也能在群里向同事求助。</p></div>
          <article v-for="message in snapshot.messages" :id="`team-message-${message.id}`" :key="message.id" class="message" :class="{ mine: member(message.fromMemberId)?.kind === 'human', mentioned: message.mentions?.includes(humanMember?.id || ''), highlight: teamState.highlight === message.id, result: message.kind === 'result' }">
            <span class="avatar small">{{ member(message.fromMemberId)?.name.slice(0, 1) || '?' }}</span>
            <div class="message-body"><div class="message-meta"><strong>{{ member(message.fromMemberId)?.name || '已离群成员' }}</strong><span>{{ member(message.fromMemberId)?.role }}</span><small>{{ time(message.createdAt) }}</small><span v-if="message.kind === 'result'" class="status">{{ taskStatusLabel(message.status) }}</span></div>
              <div v-if="message.replyToId" class="quote">回复 {{ member(snapshot.messages.find(m => m.id === message.replyToId)?.fromMemberId || '')?.name || '原消息' }}</div>
              <div v-if="message.mentions?.length" class="message-mentions"><span v-for="mid in message.mentions" :key="mid">@{{ member(mid)?.name || '已离群成员' }}</span></div>
              <div class="message-text" v-html="markdown(message)" @click="onMarkdownClick" />
              <span v-if="message.status === 'streaming'" class="streaming">正在回复…</span>
              <small v-if="message.kind === 'steer'" class="delivery">{{ deliveryLabel(message.status) }}</small>
              <div class="message-actions"><button v-if="member(message.fromMemberId)?.kind === 'agent'" @click="reply(message)">回复</button><button v-if="message.taskId" @click="selectedTaskId = message.taskId; rightTab = 'tasks'; rightOpen = true">查看任务</button></div>
            </div>
          </article>
        </div>
        <button v-if="newMessages && !stickBottom" class="new-messages" @click="scrollBottom">有新消息 ↓</button>
        <form class="composer" @submit.prevent="send">
          <div v-if="replyMessage" class="context-chip">回复 {{ member(replyMessage.fromMemberId)?.name }}：{{ replyMessage.content.slice(0, 65) }}<button type="button" aria-label="取消回复" @click="draft.replyToId = ''">×</button></div>
          <div v-if="steerTask" class="context-chip">引导 {{ member(steerTask.memberId)?.name }} · {{ steerTask.title }}<button type="button" aria-label="取消引导" @click="draft.steerTaskId = ''">×</button></div>
          <div v-if="draft.mentions.length" class="mention-chips"><span v-for="mid in draft.mentions" :key="mid" :class="{ invalid: !members.some(m => m.id === mid) }">@{{ member(mid)?.name || '已失效成员' }}<button type="button" aria-label="移除接收对象" @click="draft.mentions = draft.mentions.filter(id => id !== mid)">×</button></span></div>
          <div class="input-wrap"><textarea ref="inputEl" v-model="draft.content" aria-label="群消息" placeholder="@ 员工安排任务，或向协调人发送消息…" rows="3" :disabled="snapshot.room.archived" @input="onInput" @keydown="onInputKey" @compositionstart="composing = true" @compositionend="composing = false" />
            <div v-if="mentionOpen" class="mention-menu" role="listbox" aria-label="选择群成员"><div class="mention-head">群成员 · {{ members.length }}</div><button v-for="(candidate, index) in mentionCandidates" :key="candidate.id" type="button" role="option" :aria-selected="index === mentionIndex" :class="{ on: index === mentionIndex }" @mousedown.prevent="addMention(candidate.id)"><span class="avatar small">{{ candidate.name.slice(0, 1) }}</span><span><strong>{{ candidate.name }}</strong><small>{{ candidate.kind === 'human' ? '本人 · ' : '' }}{{ candidate.role || '数字员工' }}</small></span></button><p v-if="!mentionCandidates.length">没有匹配的成员</p></div>
          </div>
          <div class="composer-footer"><button type="button" title="@ 群成员" @click="mentionOpen = !mentionOpen; mentionQuery = ''">@</button><span class="composer-hint">{{ composerHint }}</span><select v-model="sendMode" aria-label="发送方式"><option value="new_task">安排任务</option><option value="publish">仅发消息</option></select><button class="primary" type="submit" :disabled="sending || !draft.content.trim() || snapshot.room.archived">{{ sending ? '发送中…' : steerTask ? inputLabel(steerTask) : replyMessage ? '回复' : '发送' }}<FIcon name="up" :size="14" /></button></div>
        </form>
      </section>
      <section v-else class="directory"><div v-if="teamState.loading" class="empty">正在加载群聊…</div><div v-else class="empty"><FIcon name="users" :size="36" /><h2>让数字员工一起工作</h2><p>创建协作群，邀请不同岗位的员工。你可以随时 @ 他们安排任务。</p><p v-if="teamState.error" class="error">{{ teamState.error }}</p><button class="primary" @click="newRoom">新建协作群</button><button @click="view = 'employees'">管理数字员工</button></div></section>
      <section v-if="snapshot && previewPath" class="team-preview-panel"><header><div class="preview-tabs"><button v-for="path in previewTabs" :key="path" :class="{ on: path === previewPath }" :title="path" @click="previewPath = path">{{ path.split('/').pop() }}<span role="button" tabindex="0" aria-label="关闭文件" @click.stop="closePreview(path)" @keydown.enter.stop="closePreview(path)">×</span></button></div><button title="收起预览" aria-label="收起预览" @click="previewPath = ''">×</button></header><TeamOfficePreview :room-id="snapshot.room.id" :path="previewPath" /></section>
      <aside v-if="snapshot && rightOpen && !previewPath" class="details"><header class="detail-tabs"><button :class="{ on: rightTab === 'tasks' }" @click="rightTab = 'tasks'">任务 <small>{{ snapshot.tasks.length }}</small></button><button :class="{ on: rightTab === 'members' }" @click="rightTab = 'members'">成员 <small>{{ members.length }}</small></button><button :class="{ on: rightTab === 'files' }" @click="rightTab = 'files'; refreshFiles()">文件</button><button class="detail-close" aria-label="收起右侧面板" @click="rightOpen = false">×</button></header>
        <div v-if="rightTab === 'members'" class="detail-scroll"><div v-for="m in members" :key="m.id" class="member-card"><span class="avatar small">{{ m.name.slice(0, 1) }}</span><div><strong>{{ m.name }}<small v-if="m.kind === 'human'"> · 我</small></strong><p>{{ m.role || '数字员工' }}</p><small>{{ m.kind === 'human' ? '群主' : memberStatus(m.id) }}</small><div v-if="m.kind === 'agent'" class="member-actions"><button @click="addMention(m.id)">@ 发消息</button><button v-if="activeTask(m.id)?.canSteer" @click="guide(activeTask(m.id)!)">引导当前任务</button><button class="danger" @click="removeMember(m)">移出群</button></div></div></div><button class="add-member" @click="memberDialog = true">＋ 邀请员工</button><p class="muted">岗位是工作要求；CLI 权限决定实际访问范围。共同工作目录内的写入会排队。</p></div>
        <div v-else-if="rightTab === 'files'" class="detail-scroll"><button class="add-member" @click="refreshFiles">刷新文件</button><p v-if="filesError" class="error">{{ filesError }}</p><button v-for="file in groupFiles" :key="file.path" class="team-file" :title="file.path" @click="openPreview(file.path)"><FIcon name="file" :size="15" /><span>{{ file.path }}</span></button><p v-if="filesTruncated" class="muted">仅展示前 500 个可预览文件，可让员工通过 team_open_file 打开指定路径。</p><p class="muted">支持 PPTX、DOCX、XLSX、PDF、HTML 和 Markdown。生成的文件也会关联到对应任务。</p><button v-for="artifact in snapshot.artifacts" :key="artifact.id" class="team-file" @click="openPreview(artifact.path)">{{ artifact.path }}</button></div>
        <div v-else class="detail-scroll"><div v-if="!snapshot.tasks.length" class="muted">派活后在这里查看任务进展</div><section v-for="task in snapshot.tasks" :key="task.id" class="task-card" :class="{ selected: selectedTaskId === task.id }"><button class="task-head" @click="selectedTaskId = selectedTaskId === task.id ? '' : task.id"><span class="task-dot" :class="task.status" /><strong>{{ task.title }}</strong><FIcon name="down" :size="12" /></button><div class="task-sub"><span>{{ member(task.memberId)?.name }}</span><span>{{ taskStatusLabel(task.status) }}</span></div><p v-if="task.activity && ['running', 'starting'].includes(task.status)" class="activity">{{ task.activity }}</p><p v-if="task.error" class="task-error">{{ task.error }}</p><div v-if="selectedTaskId === task.id" class="task-expanded"><p>{{ task.instruction }}</p><p v-if="task.acceptance.length">验收：{{ task.acceptance.join('；') }}</p><p v-if="task.waitFor.length">等待 {{ task.waitFor.length }} 项协作结果</p><small>验证记录：{{ task.validation === 'not_run' ? '未执行' : task.validation }}（员工报告）</small></div><div class="task-actions"><button v-if="task.canSteer" @click="guide(task)" :title="inputHint(task)">{{ inputLabel(task) }}</button><button v-if="['queued','starting','running','waiting_user','waiting_agent'].includes(task.status)" class="danger" @click="stop(task)">停止</button><button v-if="['needs_review','failed','cancelled'].includes(task.status)" @click="resume(task)">检查后继续</button></div></section></div>
      </aside>
    </template>

    <el-drawer v-model="employeeDialog" :title="employeeForm.id ? '编辑数字员工' : '新建数字员工'" size="450px" append-to-body>
      <form class="team-form" @submit.prevent="saveEmployee"><label>名字<input v-model="employeeForm.name" required maxlength="60" placeholder="例如：小林" /></label><label>头像标记<input v-model="employeeForm.avatar" maxlength="2" placeholder="首字母或表情" /></label><label>岗位<input v-model="employeeForm.role" maxlength="200" placeholder="例如：前端工程师" /></label><label>专长<textarea v-model="employeeForm.specialties" rows="3" placeholder="擅长处理哪些工作" /></label><label>关注点<textarea v-model="employeeForm.focus" rows="2" placeholder="例如：交互细节、可访问性、组件复用" /></label><label>交付标准<textarea v-model="employeeForm.standards" rows="3" placeholder="怎样才算完成，交付哪些验证结果" /></label><label>已确认的知识笔记<textarea v-model="employeeForm.knowledge" rows="3" placeholder="本人维护的长期规范；将随任务上下文提供给员工" /></label><label>执行 CLI<select v-model="employeeForm.agentKind" @change="employeeForm.permissionMode = ''; employeeForm.transport = ''"><option value="cat_build">Grok Build</option><option value="codex_cli">Codex</option><option value="claude_cli">Claude Code</option></select></label><label>模型<input v-model="employeeForm.model" placeholder="留空继承 CLI 默认模型" /></label><label>推理强度<select v-model="employeeForm.effort"><option value="">默认</option><option value="low">低</option><option value="medium">中</option><option value="high">高</option><option value="xhigh">很高</option></select></label><label>权限模式<select v-model="employeeForm.permissionMode"><option value="">CLI 默认权限</option><option v-for="mode in employeeModes" :key="mode.value" :value="mode.value">{{ mode.label }}</option></select></label><label>运行通道<select v-model="employeeForm.transport"><option value="">自动兼容</option><option v-if="employeeForm.agentKind === 'codex_cli'" value="app-server">原生 app-server（支持运行中引导）</option><option v-if="employeeForm.agentKind === 'claude_cli'" value="stream-json">原生流式输入（检查点追加）</option><option v-if="employeeForm.agentKind === 'cat_build'" value="stdio">原生 ACP（仅下一轮输入）</option></select></label><label class="inline"><input v-model="employeeForm.enabled" type="checkbox" />启用员工</label><p class="muted">复用当前 CLI 的登录、服务商、技能和 MCP 配置；每个任务使用独立会话。</p><p v-if="formError" class="error" role="alert">{{ formError }}</p><button type="submit" class="primary" :disabled="saving">保存员工</button></form>
    </el-drawer>
    <el-dialog v-model="roomDialog" :title="roomEditing ? '群设置' : '新建协作群'" width="460px" append-to-body><form class="team-form" @submit.prevent="saveRoom"><label>群名<input v-model="roomForm.name" required maxlength="100" placeholder="例如：产品研发群" /></label><label v-if="!roomEditing">项目<select v-model="roomForm.projectId"><option value="">独立群工作目录</option><option v-for="project in projects" :key="project.id" :value="project.id" :disabled="project.missing">{{ project.name }}{{ project.missing ? '（路径不可用）' : '' }}</option></select></label><fieldset v-if="!roomEditing"><legend>邀请员工</legend><label v-for="employee in teamState.bootstrap?.employees" :key="employee.id" class="inline"><input v-model="roomForm.employeeIds" type="checkbox" :value="employee.id" />{{ employee.name }} · {{ employee.role || engine(employee.agentKind) }}</label><button v-if="!teamState.bootstrap?.employees.length" type="button" @click="roomDialog = false; editEmployee()">先创建数字员工</button></fieldset><label>协调人（可选）<select v-model="roomForm.coordinatorId"><option value="">不设置，发送时指定 @ 对象</option><option v-for="employee in coordinatorOptions" :key="employee.id" :value="employee.id">{{ employee.name }}</option></select></label><template v-if="roomEditing"><label class="inline"><input v-model="roomPaused" type="checkbox" />暂停新任务和自动委派</label><label class="inline"><input v-model="roomArchived" type="checkbox" />归档群聊</label></template><p class="muted">{{ roomEditing ? snapshot?.room.workspace : '本人将自动加入。创建后即可派活，无需等待全部 CLI 启动。' }}</p><p v-if="formError" class="error">{{ formError }}</p><button type="submit" class="primary" :disabled="saving">{{ roomEditing ? '保存设置' : '创建群聊' }}</button></form></el-dialog>
    <el-dialog v-model="humanDialog" title="本人身份" width="420px" append-to-body><form class="team-form" @submit.prevent="saveHuman"><label>名字<input v-model="humanForm.name" required maxlength="60" /></label><label>身份<input v-model="humanForm.role" maxlength="200" placeholder="负责人" /></label><label>头像标记<input v-model="humanForm.avatar" maxlength="2" /></label><label class="inline"><input v-model="humanForm.notifyMentions" type="checkbox" />有人 @ 我时发送系统通知</label><p class="muted">前台、后台和托盘均提交通知；实际横幅由系统通知设置决定。</p><p v-if="formError" class="error">{{ formError }}</p><button class="primary" :disabled="saving">保存资料</button></form></el-dialog>
    <el-dialog v-model="memberDialog" title="邀请数字员工" width="400px" append-to-body><div class="team-form"><button v-for="employee in availableEmployees" :key="employee.id" @click="invite(employee)">{{ employee.name }} · {{ employee.role || engine(employee.agentKind) }} ＋</button><p v-if="!availableEmployees.length" class="muted">现有员工均已在群中，可到数字员工页创建更多员工。</p></div></el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import TeamOfficePreview from './TeamOfficePreview.vue'
import { listTeamFiles, type TeamFile } from '@/api/team'
import { renderCatMarkdown } from '@/utils/catMarkdown'
import { openExternalLink as openExternal } from '@/utils/openExternal'
import { onEvent, hasWailsBackend } from '@/services/wails'
import { openView } from '@/views/cat/catPanels'
import { listCatProjects } from '@/api/catProjects'
import type { CatProject } from '@/api/catProjects'
import { createTeamRoom, saveTeamEmployee, saveTeamHuman, sendTeamMessage, updateTeamRoom, getTeamRoom, stopTeamTask, continueTeamTask, setTeamMember, markTeamRead, type Employee, type Human, type TeamMember, type TeamMessage, type TeamTask } from '@/api/team'
import { teamState, loadTeams, selectTeam, refreshTeamEvents, readTeamDraft, writeTeamDraft, taskStatusLabel, teamError, type TeamDraft } from './teamState'

const route = useRoute(), router = useRouter()
const compactLayout = window.matchMedia('(max-width: 1050px)')
const view = ref<'rooms'|'employees'|'inbox'>('rooms'), collapsed = ref(false), rightOpen = ref(!compactLayout.matches), rightTab = ref<'tasks'|'members'|'files'>('tasks')
const fitDetails = () => { if (compactLayout.matches) rightOpen.value = false }
compactLayout.addEventListener('change', fitDetails)
const snapshot = computed(() => teamState.current?.room.id === teamState.selected ? teamState.current : null)
const members = computed(() => snapshot.value?.members.filter(m => m.active) || [])
const humanMember = computed(() => members.value.find(m => m.kind === 'human'))
const member = (id: string) => snapshot.value?.members.find(m => m.id === id)
const unread = computed(() => teamState.bootstrap?.inbox.filter(i => !i.readAt).length || 0)
const working = computed(() => new Set(snapshot.value?.tasks.filter(t => t.status === 'running').map(t => t.memberId)).size)
const previewPath = ref(''), previewTabs = ref<string[]>([]), groupFiles = ref<TeamFile[]>([]), filesTruncated = ref(false), filesError = ref('')
function openPreview(path: string) { rightOpen.value = false; if (!previewTabs.value.includes(path)) previewTabs.value.push(path); previewPath.value = path }
function closePreview(path: string) { previewTabs.value = previewTabs.value.filter(p => p !== path); if (previewPath.value === path) previewPath.value = previewTabs.value.at(-1) || '' }
async function refreshFiles() { const room = snapshot.value?.room.id; if (!room) return; filesError.value = ''; try { const result = await listTeamFiles(room); if (snapshot.value?.room.id === room) { groupFiles.value = result.files; filesTruncated.value = result.truncated } } catch (e) { filesError.value = teamError(e) } }
watch(() => teamState.openArtifact, a => { if (a && a.roomId === snapshot.value?.room.id) openPreview(a.path) })
watch(() => snapshot.value?.room.id, () => { previewPath.value = ''; previewTabs.value = []; groupFiles.value = [] })
const selectedTaskId = ref(''), onlyQuestions = ref(false)
const inboxItems = computed(() => (teamState.bootstrap?.inbox || []).filter(i => !onlyQuestions.value || (i.needsReply && !i.answeredAt)))
const roomName = (id: string) => teamState.bootstrap?.rooms.find(r => r.id === id)?.name || '协作群'
const engine = (kind: string) => ({ cat_build: 'Grok Build', codex_cli: 'Codex', claude_cli: 'Claude Code' }[kind] || kind)
const time = (ms: number) => new Date(ms).toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
const activeTask = (mid: string) => snapshot.value?.tasks.find(t => t.memberId === mid && ['running','starting','waiting_user'].includes(t.status))
const memberStatus = (mid: string) => activeTask(mid) ? taskStatusLabel(activeTask(mid)!.status) : '空闲'
const markdown = (m: TeamMessage) => renderCatMarkdown(m.content, m.status === 'streaming').map(b => b.html).join('')
const deliveryLabel = (s: string) => ({ sending: '投递中', native_accepted: '原生 CLI 已接收', rejected: '未送达', unknown: '接收结果未知，请先检查任务' }[s] || s)
async function onMarkdownClick(e: MouseEvent) { const target = e.target as HTMLElement; const a = target.closest<HTMLAnchorElement>('a[data-cat-ext]'); if (a) { e.preventDefault(); await openExternal(a.href) }; const code = target.closest('[data-cat-copy]')?.parentElement?.nextElementSibling?.textContent; if (code) await navigator.clipboard.writeText(code) }

const draft = reactive<TeamDraft>({ content: '', mentions: [], replyToId: '', steerTaskId: '' }), sending = ref(false), actionError = ref(''), sendMode = ref<'new_task'|'publish'>('new_task')
const replyMessage = computed(() => snapshot.value?.messages.find(m => m.id === draft.replyToId)), steerTask = computed(() => snapshot.value?.tasks.find(t => t.id === draft.steerTaskId))
const inputEl = ref<HTMLTextAreaElement>(), composing = ref(false), mentionOpen = ref(false), mentionQuery = ref(''), mentionIndex = ref(0)
let atStart = -1
const mentionCandidates = computed(() => members.value.filter(m => `${m.name} ${m.role}`.toLowerCase().includes(mentionQuery.value.toLowerCase())))
const composerHint = computed(() => steerTask.value ? (inputHint(steerTask.value)) : draft.mentions.length ? '忙碌员工的新任务会独立排队' : snapshot.value?.room.coordinatorId ? `默认发给 ${member(snapshot.value.room.coordinatorId)?.name}` : '@ 选择接收员工 · Enter 发送，Shift+Enter 换行')
let draftRoom = teamState.selected, opening = 0
watch(draft, () => writeTeamDraft(draftRoom, draft), { deep: true, flush: 'sync' })
async function openRoom(id: string, messageId = '') {
  writeTeamDraft(draftRoom, draft)
  const ticket = ++opening; draftRoom = ''; Object.assign(draft, readTeamDraft(id)); draftRoom = id
  await selectTeam(id, messageId); if (ticket !== opening) return; view.value = 'rooms'; actionError.value = ''; stickBottom.value = true
  await nextTick()
  if (messageId) document.getElementById(`team-message-${messageId}`)?.scrollIntoView({ block: 'center' }); else scrollBottom()
  markVisibleRead()
}
async function openInbox() { await loadTeams(); view.value = 'inbox' }
async function send() {
  if (!snapshot.value || sending.value || !draft.content.trim()) return
  const roomId = snapshot.value.room.id, pending = structuredClone({ ...draft, mentions: [...draft.mentions] })
  const requestKey = JSON.stringify([roomId, pending.content, pending.mentions, pending.replyToId, pending.steerTaskId, sendMode.value, steerTask.value?.runId]); if (draft.requestKey !== requestKey) { draft.requestKey = requestKey; draft.requestId = crypto.randomUUID() }
  sending.value = true; actionError.value = ''
  try {
    const result = await sendTeamMessage({ apiVersion: 1, requestId: draft.requestId!, roomId, intent: steerTask.value ? 'steer' : replyMessage.value ? 'reply' : sendMode.value, content: pending.content, targetMemberIds: steerTask.value ? [steerTask.value.memberId] : pending.mentions, replyToMessageId: pending.replyToId, targetTaskId: pending.steerTaskId, expectedRunId: steerTask.value?.runId })
    const failure = result.deliveries.find(d => ['rejected','unknown'].includes(d.status))
    if (teamState.selected === roomId && draft.content === pending.content && JSON.stringify(draft.mentions) === JSON.stringify(pending.mentions) && draft.replyToId === pending.replyToId && draft.steerTaskId === pending.steerTaskId && !failure) Object.assign(draft, { content: '', mentions: [], replyToId: '', steerTaskId: '', requestId: '', requestKey: '' })
    if (failure) actionError.value = failure.error || '引导未确认送达，输入已保留'
    await refreshTeamEvents(); stickBottom.value = true; await nextTick(); scrollBottom()
  } catch (e) { actionError.value = teamError(e) } finally { sending.value = false; inputEl.value?.focus() }
}
function onInput() { const cursor = inputEl.value?.selectionStart ?? draft.content.length; const before = draft.content.slice(0, cursor); const match = before.match(/(?:^|\s)@([^\s@]*)$/); if (match) { mentionOpen.value = true; mentionQuery.value = match[1]; atStart = cursor - match[1].length - 1; mentionIndex.value = 0 } else { mentionOpen.value = false; atStart = -1 } }
function addMention(mid: string) { if (!draft.mentions.includes(mid)) draft.mentions.push(mid); if (atStart >= 0) { const end = inputEl.value?.selectionStart ?? draft.content.length; draft.content = draft.content.slice(0, atStart) + draft.content.slice(end) }; atStart = -1; mentionOpen.value = false; inputEl.value?.focus() }
function onInputKey(e: KeyboardEvent) { if (composing.value || e.isComposing) return; if (mentionOpen.value) { if (e.key === 'Escape') { e.preventDefault(); mentionOpen.value = false; return }; if (['ArrowDown','ArrowUp'].includes(e.key)) { e.preventDefault(); const count = mentionCandidates.value.length; if (count) mentionIndex.value = (mentionIndex.value + (e.key === 'ArrowDown' ? 1 : -1) + count) % count; return }; if (e.key === 'Enter') { e.preventDefault(); const m = mentionCandidates.value[mentionIndex.value]; if (m) addMention(m.id); return } }; if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); void send() } }
function reply(m: TeamMessage) { draft.replyToId = m.id; draft.steerTaskId = ''; inputEl.value?.focus() }
function guide(t: TeamTask) { draft.steerTaskId = t.id; draft.replyToId = ''; inputEl.value?.focus() }
async function stop(t: TeamTask) { try { await stopTeamTask(t.id); await refreshTeamEvents() } catch (e) { actionError.value = teamError(e) } }
async function resume(t: TeamTask) { try { const { value } = await ElMessageBox.prompt('先检查已发生的修改，再填写继续要求。系统不会自动重放旧操作。', '继续任务', { inputType: 'textarea', inputValidator: v => !!v?.trim() || '请填写继续要求' }); await continueTeamTask(t.id, value); await refreshTeamEvents() } catch (e) { if (e !== 'cancel' && e !== 'close') actionError.value = teamError(e) } }

const messagesEl = ref<HTMLElement>(), stickBottom = ref(true), newMessages = ref(false), loadingEarlier = ref(false)
function onScroll() { const el = messagesEl.value; if (!el) return; stickBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 90; if (stickBottom.value) { newMessages.value = false; markVisibleRead() } }
function scrollBottom() { const el = messagesEl.value; if (el) el.scrollTop = el.scrollHeight; stickBottom.value = true; newMessages.value = false; markVisibleRead() }
let reading = false
function markVisibleRead() { if (reading || document.hidden || !snapshot.value || view.value !== 'rooms') return; const box = messagesEl.value?.getBoundingClientRect(); if (!box) return; const ids = snapshot.value.inbox.filter(i => !i.readAt && (() => { const r = document.getElementById(`team-message-${i.messageId}`)?.getBoundingClientRect(); return r && r.top < box.bottom && r.bottom > box.top })()).map(i => i.messageId); if (!ids.length) return; reading = true; void markTeamRead(snapshot.value.room.id, ids).then(() => loadTeams()).finally(() => { reading = false }) }
watch(() => snapshot.value?.eventSeq, async () => { await nextTick(); if (stickBottom.value) scrollBottom(); else newMessages.value = true })
async function loadEarlier() { if (!snapshot.value) return; const s = snapshot.value; const el = messagesEl.value, oldHeight = el?.scrollHeight || 0; loadingEarlier.value = true; try { const old = await getTeamRoom(s.room.id, s.messages[0]?.roomSeq); if (snapshot.value !== s) return; s.messages.unshift(...old.messages); s.hasEarlier = old.hasEarlier; await nextTick(); if (el) el.scrollTop += el.scrollHeight - oldHeight } catch (e) { actionError.value = teamError(e) } finally { loadingEarlier.value = false } }

const employeeDialog = ref(false), humanDialog = ref(false), roomDialog = ref(false), memberDialog = ref(false), saving = ref(false), formError = ref(''), roomEditing = ref(false), roomPaused = ref(false), roomArchived = ref(false)
const emptyEmployee = (): Employee => ({ id: '', name: '', avatar: '', role: '', specialties: '', focus: '', standards: '', knowledge: '', agentKind: 'codex_cli', model: '', effort: '', permissionMode: '', transport: '', enabled: true, version: 0 })
const employeeForm = reactive(emptyEmployee()), humanForm = reactive<Human>({ id: '', name: '', role: '', avatar: '', notifyMentions: true }), projects = ref<CatProject[]>([])
const roomForm = reactive({ name: '', projectId: '', employeeIds: [] as string[], coordinatorId: '' })
const employeeModes = computed(() => (teamState.bootstrap?.modes?.[employeeForm.agentKind] || []).map(m => ({ value: m.id, label: m.label })))
const inputLabel = (task: TeamTask) => task.inputMode === 'checkpoint' ? '追加输入' : '立即引导'
const inputHint = (task: TeamTask) => task.inputMode === 'checkpoint' ? 'CLI 已接收后，在原生检查点处理补充要求' : task.inputMode === 'active_turn' ? '追加到当前原生轮次' : '当前通道仅支持下一轮输入，可安排独立新任务'
const coordinatorOptions = computed(() => roomEditing.value ? members.value.filter(m => m.kind === 'agent').map(m => ({ id: m.id, name: m.name })) : (teamState.bootstrap?.employees || []).filter(e => roomForm.employeeIds.includes(e.id)))
const availableEmployees = computed(() => (teamState.bootstrap?.employees || []).filter(e => !members.value.some(m => m.profileId === e.id)))
function editEmployee(employee?: Employee) { Object.assign(employeeForm, employee || emptyEmployee()); formError.value = ''; employeeDialog.value = true }
function editHuman() { if (teamState.bootstrap) Object.assign(humanForm, teamState.bootstrap.human); formError.value = ''; humanDialog.value = true }
async function newRoom() { roomEditing.value = false; Object.assign(roomForm, { name: '', projectId: '', employeeIds: [], coordinatorId: '' }); formError.value = ''; roomDialog.value = true; try { projects.value = await listCatProjects() } catch { projects.value = [] } }
function editRoom() { if (!snapshot.value) return; roomEditing.value = true; Object.assign(roomForm, snapshot.value.room); roomPaused.value = snapshot.value.room.paused; roomArchived.value = snapshot.value.room.archived; formError.value = ''; roomDialog.value = true }
async function saveForm(fn: () => Promise<void>) { saving.value = true; formError.value = ''; try { await fn(); await loadTeams() } catch (e) { formError.value = teamError(e) } finally { saving.value = false } }
const saveEmployee = () => saveForm(async () => { await saveTeamEmployee({ ...employeeForm }); employeeDialog.value = false })
const saveHuman = () => saveForm(async () => { await saveTeamHuman({ ...humanForm }); humanDialog.value = false; if (snapshot.value) await selectTeam(snapshot.value.room.id) })
const saveRoom = () => saveForm(async () => { const room = roomEditing.value && snapshot.value ? await updateTeamRoom({ ...snapshot.value.room, name: roomForm.name, coordinatorId: roomForm.coordinatorId, paused: roomPaused.value, archived: roomArchived.value }) : await createTeamRoom({ ...roomForm, employeeIds: [...roomForm.employeeIds] }); roomDialog.value = false; await openRoom(room.id) })
async function invite(employee: Employee) { if (!snapshot.value) return; try { await setTeamMember(snapshot.value.room.id, employee.id, true); memberDialog.value = false; await selectTeam(snapshot.value.room.id) } catch (e) { actionError.value = teamError(e) } }
async function removeMember(m: TeamMember) { try { await ElMessageBox.confirm(`移出 ${m.name} 后，排队任务会取消、群工具权限会撤销。正在运行的 CLI 可在任务面板单独停止。`, '移出员工'); await setTeamMember(m.roomId, m.profileId, false); await selectTeam(m.roomId) } catch (e) { if (e !== 'cancel' && e !== 'close') actionError.value = teamError(e) } }
async function settings() { openView('settings'); await router.push('/cat') }
let stopEvents = () => {}, poll: ReturnType<typeof setInterval> | undefined
watch(() => [route.query.room, route.query.message], ([room, message]) => { if (typeof room === 'string') void openRoom(room, typeof message === 'string' ? message : '') })
onMounted(async () => { if (!hasWailsBackend()) { teamState.error = '请在 ffc 桌面应用中使用群聊，浏览器预览不会启动 CLI。'; return }; try { stopEvents = onEvent<{ roomId: string; kind: string }>('team:event', e => { if (e.roomId === teamState.selected) void refreshTeamEvents(); else if (['inbox','room','roster'].includes(e.kind)) void loadTeams() }); await loadTeams(); const room = route.query.room; if (typeof room === 'string') await openRoom(room, typeof route.query.message === 'string' ? route.query.message : ''); else if (teamState.bootstrap?.rooms.length) await openRoom(teamState.selected || teamState.bootstrap.rooms[0].id); poll = setInterval(() => { void refreshTeamEvents(); if (view.value === 'inbox') void loadTeams() }, 2000) } catch (e) { teamState.error = teamError(e) } })
onBeforeUnmount(() => { writeTeamDraft(draftRoom, draft); compactLayout.removeEventListener('change', fitDetails); stopEvents(); if (poll) clearInterval(poll) })
</script>
<style scoped src="./team.css"></style>
