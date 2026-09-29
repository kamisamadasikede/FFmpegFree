import { OnFileDrop, OnFileDropOff } from '../../wailsjs/runtime/runtime'
import { hasWailsBackend } from '@/services/wails'

/**
 * 监听拖入窗口的文件（拿到的是本地绝对路径，前端不读 WebView 的 File 对象）。
 * 用 Wails 运行时的 OnFileDrop：需要 main.go 开启 DragAndDrop.EnableFileDrop，
 * 放置区域要带 CSS `--wails-drop-target: drop`（useDropTarget = true，只有落在该区域才回调）。
 * 契约里的 `app:files-dropped` 事件后端目前没有发，如果以后后端统一发事件，只需要改这个文件。
 * 浏览器预览下什么也不做。返回取消监听函数。
 */
export function onFilesDropped(cb: (paths: string[]) => void): () => void {
  if (!hasWailsBackend()) return () => {}
  OnFileDrop((_x, _y, paths) => {
    if (paths?.length) cb(paths)
  }, true)
  return () => OnFileDropOff()
}
