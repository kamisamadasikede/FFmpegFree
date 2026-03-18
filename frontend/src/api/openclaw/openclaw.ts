import api from '../index'

export type OpenClawStepStatus = 'pending' | 'running' | 'success' | 'failed' | 'skipped'

export interface OpenClawInstallStep {
  id: string
  title: string
  status: OpenClawStepStatus
  detail: string
  startedAt: string
  endedAt: string
}

export interface OpenClawInstallStatus {
  state: 'idle' | 'running' | 'success' | 'failed'
  package: string
  progress: number
  currentId: string
  current: string
  message: string
  error: string
  startedAt: string
  updatedAt: string
  finishedAt: string
  steps: OpenClawInstallStep[]
  logs: OpenClawInstallLog[]
}

export interface OpenClawInstallLog {
  time: string
  stepId: string
  step: string
  level: 'info' | 'warn' | 'error'
  message: string
}

export interface OpenClawInstallRequest {
  packageName?: string
  registry?: string
}

export interface OpenClawAuthCheckResult {
  installed: boolean
  needAuth: boolean
  provider: string
  missingAuth: string[]
  defaultModel: string
  configureCmd: string
  setupTokenCmds: string[]
  modelsOutput: string
  error: string
  checkedAt: string
}

export interface OpenClawQuickConfigRequest {
  provider: 'anthropic' | 'openai' | 'openrouter'
  apiKey: string
  defaultModel: string
  useGuestMode: boolean
  persistEnv: boolean
}

export interface OpenClawModelItem {
  key: string
  name: string
  available: boolean
  local: boolean
  tags: string[]
}

export interface OpenClawQuickConfigResult {
  success: boolean
  message: string
  provider: string
  defaultModel: string
  availableCount: number
  guestModelCount: number
  guestModelReady: boolean
  availableModels: OpenClawModelItem[]
  guestModels: OpenClawModelItem[]
  rawStatusJson: string
  rawListJson: string
  steps: string[]
  error: string
  checkedAt: string
}

export const startOpenClawInstall = (data: OpenClawInstallRequest) => {
  return api.post('/api/openclaw/install/start', data)
}

export const getOpenClawInstallStatus = () => {
  return api.get('/api/openclaw/install/status')
}

export const checkOpenClawAuth = () => {
  return api.get('/api/openclaw/auth/check')
}

export const configureOpenClawAndQueryModels = (data: OpenClawQuickConfigRequest) => {
  return api.post('/api/openclaw/configure/query-models', data)
}
