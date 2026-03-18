<template>
  <div class="openclaw-page">
    <section class="panel">
      <div class="panel-title-row">
        <div>
          <h2>OpenClaw 一键安装</h2>
          <p>自动检测环境并安装，实时显示安装进度和执行日志。</p>
        </div>
        <el-tag :type="stateTagType">{{ stateLabel }}</el-tag>
      </div>

      <el-form label-width="108px" class="install-form" @submit.prevent>
        <el-row :gutter="12">
          <el-col :xs="24" :md="10">
            <el-form-item label="包名">
              <el-input v-model="installForm.packageName" placeholder="openclaw" />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="14">
            <el-form-item label="NPM 源(可选)">
              <el-input v-model="installForm.registry" placeholder="https://registry.npmmirror.com" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item>
          <div class="action-row">
            <el-button type="primary" :loading="startingInstall" :disabled="isRunning" @click="startInstall">
              一键安装
            </el-button>
            <el-button @click="fetchStatus">刷新状态</el-button>
          </div>
        </el-form-item>
      </el-form>

      <el-progress :percentage="installStatus.progress || 0" :status="progressStatus" />
      <div class="meta-line">
        <span>当前步骤: {{ installStatus.current || '--' }}</span>
        <span v-if="installStatus.updatedAt">更新时间: {{ formatTime(installStatus.updatedAt) }}</span>
      </div>
      <div v-if="installStatus.error" class="error-box">{{ installStatus.error }}</div>
    </section>

    <section class="panel">
      <div class="panel-title-row">
        <div>
          <h2>配置并查询模型</h2>
          <p>支持官方 Provider 与自定义第三方 API/Key 接入，执行后自动查询可用模型。</p>
        </div>
        <el-tag :type="quickConfigDone ? 'success' : 'info'">
          {{ quickConfigDone ? '已完成' : '待执行' }}
        </el-tag>
      </div>

      <el-form label-width="120px" class="quick-form" @submit.prevent>
        <el-row :gutter="12">
          <el-col :xs="24" :md="8">
            <el-form-item label="Provider">
              <el-select v-model="quickConfig.provider">
                <el-option label="Anthropic" value="anthropic" />
                <el-option label="OpenAI" value="openai" />
                <el-option label="OpenRouter" value="openrouter" />
                <el-option label="自定义第三方 (OpenAI 兼容)" value="custom" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="16">
            <el-form-item label="API Key">
              <el-input
                v-model="quickConfig.apiKey"
                show-password
                placeholder="可留空，留空时使用系统已有环境变量"
              />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item label="默认模型(可选)">
              <el-input v-model="quickConfig.defaultModel" placeholder="例如: openrouter/openai/gpt-4o-mini" />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="12">
            <el-form-item label="配置选项">
              <div class="switch-row">
                <el-switch v-model="quickConfig.useGuestMode" inline-prompt active-text="游客优先" inactive-text="手动指定" />
                <el-switch v-model="quickConfig.persistEnv" inline-prompt active-text="写入环境" inactive-text="仅本次" />
              </div>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row v-if="quickConfig.provider === 'custom'" :gutter="12" class="custom-env-row">
          <el-col :xs="24" :md="12">
            <el-form-item label="第三方 API Base">
              <el-input v-model="quickConfig.apiBase" placeholder="https://api.example.com/v1" />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="6">
            <el-form-item label="Key 变量名">
              <el-input v-model="quickConfig.apiKeyEnv" placeholder="OPENAI_API_KEY" />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :md="6">
            <el-form-item label="Base 变量名">
              <el-input v-model="quickConfig.apiBaseEnv" placeholder="OPENAI_BASE_URL" />
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-alert
              type="info"
              :closable="false"
              title="自定义第三方会把你填写的 API Key/API Base 注入到环境变量后再执行 openclaw models。"
            />
          </el-col>
        </el-row>

        <el-form-item>
          <div class="action-row">
            <el-button type="primary" :loading="quickConfigLoading" @click="runQuickConfig">
              一键配置并查询
            </el-button>
            <el-button :disabled="!quickConfigResult" @click="copyQuickConfigSteps">复制执行信息</el-button>
          </div>
        </el-form-item>
      </el-form>

      <div v-if="quickConfigResult" class="quick-result">
        <el-alert
          :type="quickConfigResult.success ? 'success' : 'error'"
          :closable="false"
          :title="quickConfigResult.success ? '配置完成' : `执行失败: ${quickConfigResult.error || quickConfigResult.message}`"
        />

        <div class="result-card-grid">
          <article class="result-card">
            <div class="result-label">可用模型</div>
            <div class="result-value">{{ quickConfigResult.availableCount }}</div>
          </article>
          <article class="result-card">
            <div class="result-label">游客模型</div>
            <div class="result-value">{{ quickConfigResult.guestModelCount }}</div>
          </article>
          <article class="result-card">
            <div class="result-label">默认模型</div>
            <div class="result-text">{{ quickConfigResult.defaultModel || '--' }}</div>
          </article>
          <article class="result-card">
            <div class="result-label">游客模型可用</div>
            <el-tag :type="quickConfigResult.guestModelReady ? 'success' : 'danger'">
              {{ quickConfigResult.guestModelReady ? '已完成' : '未完成' }}
            </el-tag>
          </article>
        </div>

        <div class="steps-box">
          <div class="steps-box-title">执行步骤</div>
          <ol>
            <li v-for="(step, idx) in quickConfigResult.steps" :key="`${idx}-${step}`">
              {{ step }}
            </li>
          </ol>
        </div>

        <div class="model-table-grid">
          <div class="table-box">
            <div class="table-title">可用模型 (最多100条)</div>
            <el-table :data="quickConfigResult.availableModels" size="small" max-height="300" empty-text="暂无可用模型">
              <el-table-column label="模型Key" prop="key" min-width="250" show-overflow-tooltip />
              <el-table-column label="名称" prop="name" min-width="180" show-overflow-tooltip />
              <el-table-column label="标签" min-width="120">
                <template #default="{ row }">
                  <span>{{ row.tags?.join(', ') || '--' }}</span>
                </template>
              </el-table-column>
            </el-table>
          </div>
          <div class="table-box">
            <div class="table-title">游客模型 (可直接使用)</div>
            <el-table :data="quickConfigResult.guestModels" size="small" max-height="300" empty-text="暂无可用游客模型">
              <el-table-column label="模型Key" prop="key" min-width="250" show-overflow-tooltip />
              <el-table-column label="名称" prop="name" min-width="180" show-overflow-tooltip />
              <el-table-column label="标签" min-width="120">
                <template #default="{ row }">
                  <span>{{ row.tags?.join(', ') || '--' }}</span>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>

        <el-collapse class="debug-collapse">
          <el-collapse-item title="调试输出: models list 原始结果">
            <pre class="command-box">{{ quickConfigResult.rawListJson || '--' }}</pre>
          </el-collapse-item>
          <el-collapse-item title="调试输出: models status 原始结果">
            <pre class="command-box">{{ quickConfigResult.rawStatusJson || '--' }}</pre>
          </el-collapse-item>
        </el-collapse>
      </div>
    </section>

    <section class="panel">
      <div class="panel-title-row">
        <div>
          <h2>认证检查</h2>
          <p>用于排查模型不可用时的授权问题。</p>
        </div>
      </div>

      <div class="action-row">
        <el-button type="primary" @click="checkAuth">检测认证状态</el-button>
        <el-button @click="copyText('openclaw configure')">复制 openclaw configure</el-button>
      </div>

      <el-alert
        v-if="authStatus.error"
        type="error"
        :closable="false"
        :title="`认证检测失败: ${authStatus.error}`"
        class="auth-alert"
      />
      <el-alert
        v-else-if="authChecked && authStatus.needAuth"
        type="warning"
        :closable="false"
        :title="`缺少认证: ${authStatus.missingAuth.join(', ')}`"
        class="auth-alert"
      />
      <el-alert
        v-else-if="authChecked && !authStatus.needAuth"
        type="success"
        :closable="false"
        title="认证状态正常"
        class="auth-alert"
      />

      <div class="command-head">
        <span>建议命令</span>
      </div>
      <pre class="command-box">openclaw configure
openclaw models
claude setup-token
openclaw models auth setup-token</pre>

      <div v-if="authStatus.modelsOutput" class="command-head">
        <span>最近一次检测输出</span>
        <el-button size="small" @click="copyText(authStatus.modelsOutput)">复制</el-button>
      </div>
      <pre v-if="authStatus.modelsOutput" class="command-box">{{ authStatus.modelsOutput }}</pre>
    </section>

    <section class="panel">
      <div class="logs-head">
        <div class="steps-title">安装执行节点</div>
      </div>
      <div class="step-list">
        <article v-for="(step, idx) in installStatus.steps" :key="step.id" class="step-item" :class="`status-${step.status}`">
          <div class="step-head">
            <div class="step-name">{{ idx + 1 }}. {{ step.title }}</div>
            <el-tag size="small" :type="stepTagType(step.status)">{{ stepLabel(step.status) }}</el-tag>
          </div>
          <div class="step-detail">{{ step.detail || '等待执行' }}</div>
          <div class="step-time">
            <span v-if="step.startedAt">开始: {{ formatTime(step.startedAt) }}</span>
            <span v-if="step.endedAt">结束: {{ formatTime(step.endedAt) }}</span>
          </div>
        </article>
      </div>
    </section>

    <section class="panel">
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
  configureOpenClawAndQueryModels,
  getOpenClawInstallStatus,
  OpenClawAuthCheckResult,
  OpenClawInstallStatus,
  OpenClawQuickConfigResult,
  OpenClawStepStatus,
  startOpenClawInstall
} from '@/api/openclaw/openclaw'

const installForm = reactive({
  packageName: 'openclaw',
  registry: ''
})

const quickConfig = reactive({
  provider: 'anthropic' as 'anthropic' | 'openai' | 'openrouter' | 'custom',
  apiKey: '',
  apiBase: '',
  apiKeyEnv: '',
  apiBaseEnv: '',
  defaultModel: '',
  useGuestMode: true,
  persistEnv: false
})

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

const startingInstall = ref(false)
const quickConfigLoading = ref(false)
const quickConfigDone = ref(false)
const quickConfigResult = ref<OpenClawQuickConfigResult | null>(null)
const authChecked = ref(false)
const logContainerRef = ref<HTMLElement | null>(null)
let pollTimer: ReturnType<typeof setInterval> | null = null

const isRunning = computed(() => installStatus.value.state === 'running')

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
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

const copyText = async (text: string) => {
  if (!text.trim()) {
    ElMessage.warning('没有可复制内容')
    return
  }
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败，请手动复制')
  }
}

const fetchStatus = async () => {
  try {
    const res = await getOpenClawInstallStatus()
    if (res.data.code === 200) installStatus.value = res.data.data
  } catch (error: any) {
    if (error?.response?.status === 404) {
      ElMessage.error('安装状态接口未加载(404)，请重启应用')
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
    }
  } catch (error: any) {
    if (error?.response?.status === 404) {
      ElMessage.error('认证接口 404，请重启应用')
      return
    }
    ElMessage.error('认证检测失败')
  }
}

const startInstall = async () => {
  startingInstall.value = true
  try {
    const res = await startOpenClawInstall({
      packageName: installForm.packageName.trim() || 'openclaw',
      registry: installForm.registry.trim()
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
      ElMessage.error('安装接口 404，请重启应用')
      return
    }
    ElMessage.error('启动安装失败')
  } finally {
    startingInstall.value = false
  }
}

const runQuickConfig = async () => {
  if (quickConfig.provider === 'custom' && !quickConfig.apiBase.trim()) {
    ElMessage.warning('自定义第三方模式需要填写 API Base URL')
    return
  }

  quickConfigLoading.value = true
  quickConfigDone.value = false
  quickConfigResult.value = null
  try {
    const res = await configureOpenClawAndQueryModels({
      provider: quickConfig.provider,
      apiKey: quickConfig.apiKey.trim(),
      apiBase: quickConfig.apiBase.trim(),
      apiKeyEnv: quickConfig.apiKeyEnv.trim(),
      apiBaseEnv: quickConfig.apiBaseEnv.trim(),
      defaultModel: quickConfig.defaultModel.trim(),
      useGuestMode: quickConfig.useGuestMode,
      persistEnv: quickConfig.persistEnv
    })
    if (res.data.code !== 200) {
      ElMessage.error(res.data.message || '配置失败')
      return
    }
    quickConfigResult.value = res.data.data
    quickConfigDone.value = !!res.data.data?.guestModelReady
    if (res.data.data?.success) {
      ElMessage.success('配置成功，模型列表已更新')
    } else {
      ElMessage.warning(res.data.data?.error || res.data.data?.message || '配置未完成')
    }
  } catch (error: any) {
    if (error?.response?.status === 404) {
      ElMessage.error('配置接口 404，请确认后端已重启并加载新路由')
      return
    }
    ElMessage.error('配置失败，请检查执行日志')
  } finally {
    quickConfigLoading.value = false
  }
}

const copyQuickConfigSteps = async () => {
  if (!quickConfigResult.value) return
  const lines = [
    `执行结果: ${quickConfigResult.value.success ? '成功' : '失败'}`,
    `提示: ${quickConfigResult.value.message || '--'}`,
    `错误: ${quickConfigResult.value.error || '--'}`,
    `可用模型: ${quickConfigResult.value.availableCount}`,
    `游客模型: ${quickConfigResult.value.guestModelCount}`,
    `游客模型可用: ${quickConfigResult.value.guestModelReady ? '是' : '否'}`,
    '',
    '步骤:',
    ...(quickConfigResult.value.steps || []).map((step, index) => `${index + 1}. ${step}`)
  ]
  await copyText(lines.join('\n'))
}

const copyLogs = async () => {
  if (!installStatus.value.logs?.length) return
  const content = installStatus.value.logs
    .map((log) => `${formatTime(log.time)} [${log.level.toUpperCase()}]${log.step ? ` [${log.step}]` : ''} ${log.message}`)
    .join('\n')
  await copyText(content)
}

const startPolling = () => {
  if (pollTimer) return
  pollTimer = setInterval(async () => {
    await fetchStatus()
    if (installStatus.value.state !== 'running') stopPolling()
  }, 1500)
}

const stopPolling = () => {
  if (!pollTimer) return
  clearInterval(pollTimer)
  pollTimer = null
}

watch(
  () => quickConfig.provider,
  (provider) => {
    if (provider === 'custom') {
      if (!quickConfig.apiKeyEnv.trim()) quickConfig.apiKeyEnv = 'OPENAI_API_KEY'
      if (!quickConfig.apiBaseEnv.trim()) quickConfig.apiBaseEnv = 'OPENAI_BASE_URL'
      return
    }
    quickConfig.apiBase = ''
    quickConfig.apiKeyEnv = ''
    quickConfig.apiBaseEnv = ''
  },
  { immediate: true }
)

watch(
  () => installStatus.value.state,
  (state) => {
    if (state === 'running') {
      startPolling()
      return
    }
    stopPolling()
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
  if (installStatus.value.state === 'running') startPolling()
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
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 10px;
}

.panel-title-row h2 {
  margin: 0;
  font-size: 20px;
  line-height: 1.2;
}

.panel-title-row p {
  margin: 6px 0 0;
  color: var(--text-muted);
  font-size: 13px;
}

.install-form,
.quick-form {
  margin-bottom: 10px;
}

.custom-env-row {
  margin-bottom: 8px;
}

.switch-row {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.action-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.meta-line {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 12px;
  color: var(--text-muted);
}

.error-box {
  margin-top: 10px;
  border-radius: 10px;
  border: 1px solid rgba(220, 38, 38, 0.35);
  background: rgba(220, 38, 38, 0.1);
  color: #b91c1c;
  padding: 10px;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-word;
}

.quick-result {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.result-card-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.result-card {
  border: 1px solid var(--border-soft);
  border-radius: 10px;
  background: var(--surface-muted);
  padding: 10px;
}

.result-label {
  font-size: 12px;
  color: var(--text-muted);
}

.result-value {
  margin-top: 6px;
  font-size: 24px;
  font-weight: 700;
  line-height: 1;
}

.result-text {
  margin-top: 6px;
  font-size: 13px;
  line-height: 1.4;
  word-break: break-word;
}

.steps-box {
  border: 1px solid var(--border-soft);
  border-radius: 10px;
  background: var(--surface-muted);
  padding: 10px 12px;
}

.steps-box-title {
  font-size: 13px;
  font-weight: 700;
  margin-bottom: 6px;
}

.steps-box ol {
  margin: 0;
  padding-left: 18px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  color: var(--text-muted);
  font-size: 12px;
}

.model-table-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.table-box {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.table-title {
  font-size: 13px;
  font-weight: 700;
}

.debug-collapse {
  border: 1px solid var(--border-soft);
  border-radius: 10px;
  overflow: hidden;
}

.auth-alert {
  margin-top: 10px;
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
  border-color: rgba(22, 163, 74, 0.45);
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
  .result-card-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .model-table-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 760px) {
  .panel-title-row {
    flex-direction: column;
  }

  .result-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>
