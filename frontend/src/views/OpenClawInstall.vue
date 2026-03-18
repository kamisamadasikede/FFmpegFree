<template>
  <div class="openclaw-page">
    <section class="panel install-panel">
      <div class="panel-title-row">
        <div>
          <h2>OpenClaw 一键安装</h2>
          <p>自动检测环境并执行安装，实时显示步骤状态和执行日志。</p>
        </div>
        <el-tag :type="stateTagType">{{ stateLabel }}</el-tag>
      </div>

      <el-form label-width="120px" class="install-form" @submit.prevent>
        <el-form-item label="包名">
          <el-input v-model="form.packageName" placeholder="openclaw" />
        </el-form-item>
        <el-form-item label="NPM 源(可选)">
          <el-input v-model="form.registry" placeholder="https://registry.npmmirror.com" />
        </el-form-item>
        <el-form-item>
          <div class="action-row">
            <el-button type="primary" :loading="starting" :disabled="isRunning" @click="startInstall">一键安装</el-button>
            <el-button @click="fetchStatus">刷新状态</el-button>
          </div>
        </el-form-item>
      </el-form>

      <el-progress :percentage="installStatus.progress || 0" :status="progressStatus" />
      <div class="meta-line">
        <span>当前步骤：{{ installStatus.current || '--' }}</span>
        <span v-if="installStatus.updatedAt">更新时间：{{ formatTime(installStatus.updatedAt) }}</span>
      </div>
      <div v-if="installStatus.error" class="error-box">{{ installStatus.error }}</div>
    </section>

    <section class="panel guide-panel">
      <div class="panel-title-row guide-title-row">
        <div>
          <h3>OpenClaw 配置引导</h3>
          <p>包含 API 环境引导和认证引导，帮助用户一步步完成可用配置。</p>
        </div>
      </div>

      <el-tabs v-model="guideTab" class="guide-tabs">
        <el-tab-pane label="API 环境引导" name="api">
          <el-steps :active="guideStep" finish-status="success" simple>
            <el-step title="选择提供方" />
            <el-step title="填写 API 参数" />
            <el-step title="生成配置命令" />
            <el-step title="执行与验证" />
          </el-steps>

          <div class="guide-body">
            <div v-if="guideStep === 0" class="guide-block">
              <el-form label-width="120px">
                <el-form-item label="API 类型">
                  <el-select v-model="guideForm.provider" style="width: 320px">
                    <el-option label="OpenAI Compatible" value="openai" />
                    <el-option label="Azure OpenAI" value="azure" />
                    <el-option label="自定义网关" value="custom" />
                  </el-select>
                </el-form-item>
                <el-alert type="info" :closable="false" title="建议先确认 API 服务可访问，再继续配置密钥与模型。" />
              </el-form>
            </div>

            <div v-else-if="guideStep === 1" class="guide-block">
              <el-form label-width="140px" class="api-form">
                <el-form-item label="API Base URL">
                  <el-input v-model="guideForm.baseUrl" placeholder="https://api.example.com/v1" />
                </el-form-item>
                <el-form-item label="API Key">
                  <el-input v-model="guideForm.apiKey" show-password placeholder="sk-xxxx" />
                </el-form-item>
                <el-form-item label="默认模型">
                  <el-input v-model="guideForm.model" placeholder="gpt-4.1-mini" />
                </el-form-item>
                <el-form-item label="组织ID(可选)">
                  <el-input v-model="guideForm.orgId" placeholder="org_xxx" />
                </el-form-item>
                <el-form-item label="超时(ms)">
                  <el-input-number v-model="guideForm.timeoutMs" :min="1000" :max="180000" :step="1000" />
                </el-form-item>
                <el-form-item v-if="guideForm.provider === 'azure'" label="API Version">
                  <el-input v-model="guideForm.apiVersion" placeholder="2024-10-21" />
                </el-form-item>
              </el-form>
            </div>

            <div v-else-if="guideStep === 2" class="guide-block">
              <div class="command-head">
                <span>PowerShell 配置命令</span>
                <el-button size="small" @click="copyText(powershellScript)">复制</el-button>
              </div>
              <pre class="command-box">{{ powershellScript }}</pre>

              <div class="command-head">
                <span>.env 配置内容</span>
                <el-button size="small" @click="copyText(envFileContent)">复制</el-button>
              </div>
              <pre class="command-box">{{ envFileContent }}</pre>
            </div>

            <div v-else class="guide-block">
              <el-alert type="success" :closable="false" title="按顺序执行并勾选完成项" />
              <el-checkbox-group v-model="guideChecklist" class="check-list">
                <el-checkbox label="已执行环境变量命令" value="env" />
                <el-checkbox label="已重启终端或应用" value="restart" />
                <el-checkbox label="已执行 openclaw --version 验证" value="verify" />
              </el-checkbox-group>
              <el-button type="primary" :disabled="guideChecklist.length < 3" @click="finishGuide">完成引导</el-button>
            </div>
          </div>

          <div class="guide-actions">
            <el-button :disabled="guideStep === 0" @click="prevGuideStep">上一步</el-button>
            <el-button type="primary" :disabled="guideStep === 3" @click="nextGuideStep">下一步</el-button>
          </div>
        </el-tab-pane>

        <el-tab-pane label="认证引导" name="auth">
          <div class="auth-toolbar">
            <el-button type="primary" @click="checkAuth">检测认证状态</el-button>
            <el-button @click="copyText('openclaw configure')">复制 openclaw configure</el-button>
          </div>

          <el-alert
            v-if="authStatus.error"
            type="error"
            :closable="false"
            :title="`认证检测失败：${authStatus.error}`"
            class="auth-alert"
          />

          <el-alert
            v-else-if="authChecked && authStatus.needAuth"
            type="warning"
            :closable="false"
            :title="`检测到未认证 Provider：${authStatus.missingAuth.join(', ')}`"
            class="auth-alert"
          />

          <el-alert
            v-else-if="authChecked && !authStatus.needAuth"
            type="success"
            :closable="false"
            title="认证状态正常，可直接使用 OpenClaw。"
            class="auth-alert"
          />

          <div class="auth-grid">
            <article class="auth-card">
              <h4>步骤 1：通用配置</h4>
              <p>优先执行官方引导配置，按提示填写 Provider、Token、默认模型。</p>
              <pre class="command-box small">openclaw configure</pre>
              <el-button size="small" @click="copyText('openclaw configure')">复制命令</el-button>
            </article>

            <article class="auth-card" v-if="authStatus.missingAuth.includes('anthropic') || !authChecked">
              <h4>步骤 2：Anthropic Token（如需）</h4>
              <p>如果缺少 anthropic 认证，执行下面两条命令。</p>
              <pre class="command-box small">claude setup-token
openclaw models auth setup-token</pre>
              <div class="action-row">
                <el-button size="small" @click="copyText('claude setup-token')">复制命令1</el-button>
                <el-button size="small" @click="copyText('openclaw models auth setup-token')">复制命令2</el-button>
              </div>
            </article>

            <article class="auth-card">
              <h4>步骤 3：验证结果</h4>
              <p>执行后用 models 命令确认 Missing auth 已消失。</p>
              <pre class="command-box small">openclaw models</pre>
              <el-button size="small" @click="copyText('openclaw models')">复制命令</el-button>
            </article>
          </div>

          <div v-if="authStatus.modelsOutput" class="command-head">
            <span>最近一次检测输出</span>
            <el-button size="small" @click="copyText(authStatus.modelsOutput)">复制输出</el-button>
          </div>
          <pre v-if="authStatus.modelsOutput" class="command-box">{{ authStatus.modelsOutput }}</pre>
        </el-tab-pane>
      </el-tabs>
    </section>

    <section class="panel steps-panel">
      <div class="steps-title">安装执行节点</div>
      <div class="step-list">
        <article v-for="(step, idx) in installStatus.steps" :key="step.id" class="step-item" :class="`status-${step.status}`">
          <div class="step-head">
            <div class="step-name">{{ idx + 1 }}. {{ step.title }}</div>
            <el-tag size="small" :type="stepTagType(step.status)">{{ stepLabel(step.status) }}</el-tag>
          </div>
          <div class="step-detail">{{ step.detail || '等待执行' }}</div>
          <div class="step-time">
            <span v-if="step.startedAt">开始：{{ formatTime(step.startedAt) }}</span>
            <span v-if="step.endedAt">结束：{{ formatTime(step.endedAt) }}</span>
          </div>
        </article>
      </div>
    </section>

    <section class="panel logs-panel">
      <div class="logs-head">
        <div class="steps-title">执行信息</div>
        <el-button size="small" @click="copyLogs" :disabled="!installStatus.logs?.length">复制日志</el-button>
      </div>
      <div ref="logContainerRef" class="log-container">
        <div v-if="!installStatus.logs?.length" class="log-empty">暂无执行信息</div>
        <div v-for="(log, idx) in installStatus.logs" :key="`${log.time}-${idx}`" class="log-item" :class="`log-${log.level}`">
          <span class="log-time">{{ formatTime(log.time) }}</span>
          <span class="log-level">[{{ log.level.toUpperCase() }}]</span>
          <span v-if="log.step" class="log-step">[{{ log.step }}]</span>
          <span class="log-message">{{ log.message }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  checkOpenClawAuth,
  getOpenClawInstallStatus,
  OpenClawAuthCheckResult,
  OpenClawInstallStatus,
  OpenClawStepStatus,
  startOpenClawInstall
} from '@/api/openclaw/openclaw'

const form = reactive({
  packageName: 'openclaw',
  registry: ''
})

const guideTab = ref<'api' | 'auth'>('api')

const guideForm = reactive({
  provider: 'openai',
  baseUrl: 'https://api.openai.com/v1',
  apiKey: '',
  model: 'gpt-4.1-mini',
  orgId: '',
  timeoutMs: 60000,
  apiVersion: '2024-10-21'
})

const guideStep = ref(0)
const guideChecklist = ref<string[]>([])

const installStatus = ref<OpenClawInstallStatus>({
  state: 'idle',
  package: 'openclaw',
  progress: 0,
  currentId: '',
  current: '',
  message: '',
  error: '',
  startedAt: '',
  updatedAt: '',
  finishedAt: '',
  steps: [],
  logs: []
})

const authStatus = ref<OpenClawAuthCheckResult>({
  installed: false,
  needAuth: false,
  provider: '',
  missingAuth: [],
  defaultModel: '',
  configureCmd: 'openclaw configure',
  setupTokenCmds: ['openclaw models auth setup-token'],
  modelsOutput: '',
  error: '',
  checkedAt: ''
})
const authChecked = ref(false)

const starting = ref(false)
const logContainerRef = ref<HTMLElement | null>(null)
let pollTimer: ReturnType<typeof setInterval> | null = null

const isRunning = computed(() => installStatus.value.state === 'running')

const envPairs = computed(() => {
  const pairs: Array<{ key: string; value: string }> = [
    { key: 'OPENCLAW_API_BASE', value: guideForm.baseUrl.trim() },
    { key: 'OPENCLAW_API_KEY', value: guideForm.apiKey.trim() },
    { key: 'OPENCLAW_MODEL', value: guideForm.model.trim() },
    { key: 'OPENCLAW_TIMEOUT_MS', value: String(guideForm.timeoutMs) }
  ]

  if (guideForm.orgId.trim()) {
    pairs.push({ key: 'OPENCLAW_ORG_ID', value: guideForm.orgId.trim() })
  }
  if (guideForm.provider === 'azure') {
    pairs.push({ key: 'OPENCLAW_AZURE_API_VERSION', value: guideForm.apiVersion.trim() })
  }
  pairs.push({ key: 'OPENCLAW_PROVIDER', value: guideForm.provider })

  return pairs
})

const envFileContent = computed(() => envPairs.value.map((item) => `${item.key}=${item.value}`).join('\n'))

const powershellScript = computed(() => {
  const lines = envPairs.value.map((item) => `setx ${item.key} "${item.value.replace(/"/g, '\\"')}"`)
  lines.push('')
  lines.push('openclaw --version')
  return lines.join('\n')
})

const stateLabel = computed(() => {
  switch (installStatus.value.state) {
    case 'running':
      return '执行中'
    case 'success':
      return '已完成'
    case 'failed':
      return '失败'
    default:
      return '未开始'
  }
})

const stateTagType = computed(() => {
  switch (installStatus.value.state) {
    case 'running':
      return 'warning'
    case 'success':
      return 'success'
    case 'failed':
      return 'danger'
    default:
      return 'info'
  }
})

const progressStatus = computed(() => {
  if (installStatus.value.state === 'failed') return 'exception'
  if (installStatus.value.state === 'success') return 'success'
  return ''
})

const stepLabel = (status: OpenClawStepStatus) => {
  switch (status) {
    case 'running':
      return '进行中'
    case 'success':
      return '成功'
    case 'failed':
      return '失败'
    case 'skipped':
      return '跳过'
    default:
      return '待执行'
  }
}

const stepTagType = (status: OpenClawStepStatus) => {
  switch (status) {
    case 'running':
      return 'warning'
    case 'success':
      return 'success'
    case 'failed':
      return 'danger'
    case 'skipped':
      return 'info'
    default:
      return ''
  }
}

const formatTime = (value: string) => {
  if (!value) return '--'
  return new Date(value).toLocaleString()
}

const validateGuideStep = () => {
  if (guideStep.value === 1) {
    if (!guideForm.baseUrl.trim() || !guideForm.apiKey.trim() || !guideForm.model.trim()) {
      ElMessage.warning('请先完整填写 API Base URL、API Key、默认模型')
      return false
    }
  }
  return true
}

const nextGuideStep = () => {
  if (!validateGuideStep()) return
  if (guideStep.value < 3) {
    guideStep.value += 1
  }
}

const prevGuideStep = () => {
  if (guideStep.value > 0) {
    guideStep.value -= 1
  }
}

const finishGuide = () => {
  ElMessage.success('API 环境引导完成')
}

const copyText = async (text: string) => {
  if (!text.trim()) {
    ElMessage.warning('没有可复制内容')
    return
  }
  await navigator.clipboard.writeText(text)
  ElMessage.success('已复制到剪贴板')
}

const fetchStatus = async () => {
  try {
    const res = await getOpenClawInstallStatus()
    if (res.data.code === 200) {
      installStatus.value = res.data.data
    }
  } catch (error: any) {
    if (error?.response?.status === 404) {
      ElMessage.error('后端接口未加载（404），请重启应用后再试')
      stopPolling()
      return
    }
    ElMessage.error('获取安装状态失败')
  }
}

const checkAuth = async () => {
  try {
    const res = await checkOpenClawAuth()
    if (res.data.code === 200) {
      authStatus.value = res.data.data
      authChecked.value = true
      if (!authStatus.value.installed) {
        ElMessage.warning('未检测到 openclaw，请先安装')
      } else if (authStatus.value.needAuth) {
        ElMessage.warning('检测到认证缺失，请按认证引导操作')
      } else {
        ElMessage.success('认证状态正常')
      }
    }
  } catch (error: any) {
    if (error?.response?.status === 404) {
      ElMessage.error('认证检测接口 404，请重启后端/应用')
      return
    }
    ElMessage.error('认证检测失败')
  }
}

const startInstall = async () => {
  starting.value = true
  try {
    const res = await startOpenClawInstall({
      packageName: form.packageName.trim() || 'openclaw',
      registry: form.registry.trim()
    })
    if (res.data.code !== 200) {
      ElMessage.error(res.data.message || '启动安装失败')
      return
    }
    ElMessage.success('安装任务已启动')
    await fetchStatus()
    startPolling()
  } catch (error: any) {
    if (error?.response?.status === 404) {
      ElMessage.error('安装接口 404：请先重启后端/应用以加载新接口')
      return
    }
    ElMessage.error('启动安装失败')
  } finally {
    starting.value = false
  }
}

const startPolling = () => {
  if (pollTimer) return
  pollTimer = setInterval(async () => {
    await fetchStatus()
    if (installStatus.value.state !== 'running') {
      stopPolling()
    }
  }, 1500)
}

const stopPolling = () => {
  if (!pollTimer) return
  clearInterval(pollTimer)
  pollTimer = null
}

const copyLogs = async () => {
  if (!installStatus.value.logs?.length) return
  const content = installStatus.value.logs
    .map((log) => `${formatTime(log.time)} [${log.level.toUpperCase()}]${log.step ? ` [${log.step}]` : ''} ${log.message}`)
    .join('\n')
  await navigator.clipboard.writeText(content)
  ElMessage.success('执行日志已复制')
}

watch(
  () => installStatus.value.state,
  (state) => {
    if (state === 'running') {
      startPolling()
    } else {
      stopPolling()
    }
  }
)

watch(
  () => installStatus.value.logs?.length || 0,
  async () => {
    await nextTick()
    if (!logContainerRef.value) return
    logContainerRef.value.scrollTop = logContainerRef.value.scrollHeight
  }
)

onMounted(async () => {
  await fetchStatus()
  await checkAuth()
  if (installStatus.value.state === 'running') {
    startPolling()
  }
})

onUnmounted(() => {
  stopPolling()
})
</script>

<style scoped>
.openclaw-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.panel-title-row {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 10px;
  margin-bottom: 10px;
}

.panel-title-row h2,
.panel-title-row h3 {
  margin: 0;
  font-size: 20px;
  line-height: 1.2;
}

.panel-title-row p {
  margin: 6px 0 0;
  color: var(--text-muted);
  font-size: 13px;
}

.guide-title-row h3 {
  font-size: 18px;
}

.install-form {
  margin-bottom: 10px;
}

.action-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.meta-line {
  margin-top: 8px;
  font-size: 12px;
  color: var(--text-muted);
  display: flex;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
}

.error-box {
  margin-top: 10px;
  border: 1px solid rgba(220, 38, 38, 0.35);
  background: rgba(220, 38, 38, 0.1);
  color: #b91c1c;
  padding: 10px;
  border-radius: 10px;
  font-size: 13px;
}

.guide-tabs {
  margin-top: 8px;
}

.guide-body {
  margin-top: 12px;
}

.guide-block {
  margin-top: 12px;
}

.api-form {
  max-width: 760px;
}

.command-head {
  margin-top: 10px;
  margin-bottom: 8px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
  color: var(--text-muted);
}

.command-box {
  margin: 0;
  padding: 10px;
  border-radius: 10px;
  border: 1px solid var(--border-soft);
  background: #0f172a;
  color: #e2e8f0;
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}

.command-box.small {
  margin-bottom: 10px;
}

.check-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 14px 0;
}

.guide-actions {
  margin-top: 14px;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.auth-toolbar {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 8px;
}

.auth-alert {
  margin-top: 10px;
}

.auth-grid {
  margin-top: 12px;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.auth-card {
  border: 1px solid var(--border-soft);
  border-radius: 10px;
  background: var(--surface-muted);
  padding: 10px;
}

.auth-card h4 {
  margin: 0;
  font-size: 14px;
}

.auth-card p {
  margin: 8px 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-muted);
}

.logs-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}

.steps-title {
  font-size: 15px;
  font-weight: 700;
}

.step-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.step-item {
  border: 1px solid var(--border-soft);
  border-radius: 10px;
  padding: 10px;
  background: var(--surface-muted);
}

.step-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}

.step-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.step-detail {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-muted);
  white-space: pre-wrap;
  word-break: break-word;
}

.step-time {
  margin-top: 6px;
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  font-size: 11px;
  color: var(--text-soft);
}

.status-running {
  border-color: rgba(245, 158, 11, 0.45);
}

.status-success {
  border-color: rgba(22, 163, 74, 0.4);
}

.status-failed {
  border-color: rgba(220, 38, 38, 0.45);
}

.log-container {
  border: 1px solid var(--border-soft);
  border-radius: 10px;
  background: #0f172a;
  color: #e2e8f0;
  max-height: 300px;
  overflow: auto;
  padding: 8px;
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
}

.log-empty {
  color: #94a3b8;
  padding: 8px;
}

.log-item {
  line-height: 1.6;
  padding: 3px 0;
  border-bottom: 1px dashed rgba(148, 163, 184, 0.2);
}

.log-item:last-child {
  border-bottom: none;
}

.log-time {
  color: #93c5fd;
  margin-right: 6px;
}

.log-level {
  margin-right: 6px;
}

.log-step {
  margin-right: 6px;
  color: #cbd5e1;
}

.log-info .log-level {
  color: #38bdf8;
}

.log-warn .log-level {
  color: #fbbf24;
}

.log-error .log-level {
  color: #f87171;
}

.log-message {
  white-space: pre-wrap;
  word-break: break-word;
}

@media (max-width: 1080px) {
  .auth-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 760px) {
  .panel-title-row {
    flex-direction: column;
  }

  .guide-actions {
    justify-content: flex-start;
  }
}
</style>
