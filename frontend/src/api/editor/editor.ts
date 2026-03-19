import api from '@/api'

export interface EditSourceItem {
  name: string
  scope: 'user' | 'steam' | 'converted'
  url: string
  duration: string
  date: string
}

export interface VideoTrackClip {
  fileName: string
  scope: 'user' | 'steam' | 'converted'
  trackId: string
  startSec: number
  inSec: number
  outSec: number
  speed: number
  effectPreset: 'none' | 'grayscale' | 'sepia' | 'vintage' | 'cinematic'
  transitionToNext: 'none' | 'fade' | 'wipeleft' | 'wiperight' | 'slideleft' | 'slideright' | 'circleopen' | 'circleclose' | 'dissolve'
  transitionDurationSec: number
  blur: number
}

export interface AudioTrackClip {
  fileName: string
  scope: 'user' | 'steam' | 'converted'
  trackId: string
  startSec: number
  inSec: number
  outSec: number
  speed: number
  volume: number
  delaySec: number
}

export interface EditRenderRequest {
  outputName: string
  outputFormat: 'mp4' | 'mov' | 'mkv' | 'webm'
  width: number
  height: number
  fps: number
  videoTrack: VideoTrackClip[]
  audioTrack: AudioTrackClip[]
  effects: {
    brightness: number
    contrast: number
    saturation: number
    sharpen: number
  }
}

export const getEditSources = async () => api.get('/api/edit/sources')

export const probeEditSource = async (fileName: string, scope: 'user' | 'steam' | 'converted') =>
  api.post('/api/edit/probe', { fileName, scope })

export const renderEditProject = async (payload: EditRenderRequest) => api.post('/api/edit/render', payload)
