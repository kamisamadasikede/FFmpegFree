// src/api/index.ts

import axios from 'axios'

const baseURL = `http://localhost:8000`

const api = axios.create({
  baseURL,
})

// 请求拦截器
api.interceptors.request.use(
  (config) => {
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// 响应拦截器 - 修改如下
api.interceptors.response.use(
  (response) => {
    // 如果是 Blob 类型，直接返回整个 response（否则 blob 会损坏）
    if (response.config.responseType === 'blob') {
      return response
    }

    // 否则继续返回 data 字段
    return response
  },
  (error) => {
    return Promise.reject(error)
  }
)

export default api
