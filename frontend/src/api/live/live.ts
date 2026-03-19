import api from '@/api'

export interface LiveHealthItem {
  streamId: string
  displayName: string
  input: string
  targets: string[]
  source: 'file' | 'screen' | 'relay'
  status: string
  archiveEnabled: boolean
  segmentSeconds: number
  archiveDir: string
  fps: number
  bitrateKbps: number
  ingressBitrateKbps: number
  speed: number
  outTimeMs: number
  estimatedLatencyMs: number
  dropFrames: number
  dupFrames: number
  health: 'healthy' | 'warning' | 'critical'
  diagnosis: string
  lastError?: string
  startedAt: string
  updatedAt: string
}

export interface LiveArchiveItem {
  streamId: string
  fileName: string
  fileUrl: string
  sizeBytes: number
  updatedAt: string
}

export interface RelayTaskItem {
  streamId: string
  displayName: string
  sourceUrl: string
  targets: string[]
  status: string
  health: 'healthy' | 'warning' | 'critical'
  latencyMs: number
  dropFrames: number
}

export interface RelayStartPayload {
  displayName: string
  sourceUrl: string
  targets: string[]
  archiveEnabled: boolean
  segmentSeconds: number
}

export const getLiveHealth = async () => api.get('/api/live/health')

export const getLiveArchives = async () => api.get('/api/live/archives')

export const startRelay = async (payload: RelayStartPayload) => api.post('/api/live/relay/start', payload)

export const stopRelay = async (streamId: string) => api.post('/api/live/relay/stop', { streamId })

export const listRelay = async () => api.get('/api/live/relay/list')

