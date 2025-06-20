// src/api/video/video.ts
import axios from 'axios'
import api from '../index'
interface VideoInfo {
  name: string
  url: string
  duration: string
  date: string
  steamurl: string
  targetFormat: string
}

export const getConvertingFiles = async () => {
  return api.get('/api/selectvideofile')
}
export const getSteamFiles = async () => {
  return api.get('/api/getSteamFiles')
}
export const convertreload = async (videoInfo: VideoInfo) => {
  try {
    const response   = await api.post('/api/convert', videoInfo)
    return response
  } catch (error) {
    console.error('视频转换请求失败:', error)
    throw error
  }
}
export const steamload = async (videoInfo: VideoInfo) => {
  try {
    const response   = await api.post('/api/steamload', videoInfo)
    return response
  } catch (error) {
    console.error('视频转换请求失败:', error)
    throw error
  }
}
export const convertUp = async () => {
    return  api.get('/api/convertup')
}
// 下载视频
export const downloadVideo = async (videoName: string): Promise<Blob> => {
  try {
    const response = await api.get<Blob>(
        `/api/download?name=${encodeURIComponent(videoName)}`,
        {
          responseType: 'blob',
        }
    )
    return response.data
  } catch (error) {
    console.error('文件下载失败:', error)
    throw error
  }
}
export const deleteUpsc = async (row:VideoInfo) => {

  return  api.post('/api/deleteUpsc',{...row})
}
export const deleteUp = async (row:VideoInfo) => {

  return  api.post('/api/deleteUp',{...row})
}
export const deletesteamVideo = async (row:VideoInfo) => {

  return  api.post('/api/deletesteamVideo',{...row})
}
export const GetConvertingFiles = async () => {
  return api.get('/api/GetConvertingFiles')
}
export const RemoveConvertingTask = async (row:VideoInfo) => {

  return  api.post('/api/RemoveConvertingTask',{...row})
}