/**
 * 办公画布接口层自检（契约 6.19.16）。
 */
import {
  OFFICE_CANVAS_COPY as C,
  MOCK_OFFICE_DECK,
  getOfficeCanvasStatus,
  installOfficeCanvas,
  exportOfficeCanvasPptx,
  setOfficeSimStatus,
  isOfficeCanvasSim,
} from './officeCanvas'
import { AppError } from './call'

export async function officeCanvasChecks(): Promise<void> {
  if (!isOfficeCanvasSim()) {
    // 有 Wails 时跳过模拟断言（仍校验文案常量）
    if (C.needTitle !== '需要办公画布组件') throw new Error('needTitle')
    if (C.needBody !== '用于幻灯片实时预览与导出。下载后即可使用。') throw new Error('needBody')
    if (C.failedUnconfigured !== '暂时无法下载办公画布组件，请稍后重试。') throw new Error('failedUnconfigured')
    if (C.exportFailPrefix !== '导出失败：') throw new Error('exportFailPrefix')
    if (MOCK_OFFICE_DECK.slides.length < 3) throw new Error('mock deck < 3')
    return
  }

  if (C.needTitle !== '需要办公画布组件') throw new Error('needTitle')
  if (C.needBody !== '用于幻灯片实时预览与导出。下载后即可使用。') throw new Error('needBody')
  if (C.failedUnconfigured !== '暂时无法下载办公画布组件，请稍后重试。') throw new Error('failedUnconfigured')
  if (MOCK_OFFICE_DECK.slides.length < 3) throw new Error('mock deck < 3')

  setOfficeSimStatus({
    state: 'missing',
    canDownload: false,
    downloadBytes: 16 * 1024 * 1024,
    error: null,
  })
  const st = await getOfficeCanvasStatus()
  if (st.state !== 'missing' || st.canDownload !== false) throw new Error('sim missing status')

  let threw = false
  try {
    await installOfficeCanvas()
  } catch (e) {
    threw = true
    const err = e as AppError
    if (err.message !== C.failedUnconfigured) throw new Error('install unconfigured message: ' + err.message)
  }
  if (!threw) throw new Error('install should fail when unconfigured')
  const failed = await getOfficeCanvasStatus()
  if (failed.state !== 'failed') throw new Error('expected failed after unconfigured install')

  setOfficeSimStatus({ state: 'ready', source: 'downloaded', version: 't', canDownload: true, error: null })
  const exp = await exportOfficeCanvasPptx({ convId: 'c1', artifactId: 'a1' })
  if (!exp.path || !exp.bytes) throw new Error('export mock result')
}
