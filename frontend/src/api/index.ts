// v1 的本地 gin HTTP 服务已随后端重写移除，这里不再发起任何网络请求。
// 仍 import 本模块的页面（剪辑、Office 转 PDF、PDF 上传）在迁移到 Wails 服务之前处于“暂不可用”状态：
// 页面顶部有 MigrationNotice，相关按钮置灰；万一还有调用漏过来，一律立即 reject，不联网。

/** v1 接口是否可用。迁移完成、改用 Wails 服务后，各页面自己的调用替换掉，这个开关随之删除。 */
export const V1_API_READY = false

export class V1UnavailableError extends Error {
  constructor() {
    super('该功能正在迁移到 v2，暂不可用')
    this.name = 'V1UnavailableError'
  }
}

interface RequestConfig {
  headers?: Record<string, string>
  responseType?: 'blob' | 'json'
  onUploadProgress?: (e: { loaded: number; total?: number }) => void
}

const unavailable = <T>(): Promise<{ data: T }> => Promise.reject(new V1UnavailableError())

const api = {
  get: <T = any>(_url: string, _config?: RequestConfig) => unavailable<T>(),
  post: <T = any>(_url: string, _data?: unknown, _config?: RequestConfig) => unavailable<T>(),
}

export default api
