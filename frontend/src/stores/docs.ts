import { defineStore } from 'pinia'
import { ref } from 'vue'
import { MAX_RECENT_LIMIT, listRecentPDFs, removeRecentPDFs, type PDFFile } from '@/api/doc'
import { toAppError } from '@/api/call'
import { docErrorText } from '@/errors/errorMessages'
import { ElMessage } from 'element-plus'

/** Wails 的文件拖入回调只有一个入口：DocsLayout 注册一次，按当前 Tab 分发给这里登记的处理函数（KeepAlive 下各页在 activated / deactivated 时登记 / 撤销） */
export const dropHandlers: { office?: (paths: string[]) => void; pdf?: (paths: string[]) => void } = {}

/**
 * 文档页两个 Tab 共用的状态：右侧「最近打开的 PDF」列表（doc_recent，OpenPDF 是唯一写入点）和当前预览的路径（用于整行高亮）。
 * 列表只放这里，两个 Tab 不各自读一份。
 */
export const useDocsStore = defineStore('docs', () => {
  const recent = ref<PDFFile[]>([])
  const loading = ref(true)
  /** 当前在 PDF 预览里打开（或试图打开）的路径；用于最近列表高亮，包括「找不到文件」的失败态 */
  const currentPath = ref('')
  /** 最近一次被用户从列表移除的 PDF 路径；PDF 预览监听它，正在看的被移除时清空预览（句柄已被撤销） */
  const removedPath = ref('')

  async function loadRecent() {
    try {
      recent.value = await listRecentPDFs(MAX_RECENT_LIMIT)
    } catch (e) {
      console.error('读取最近打开的 PDF 失败', e)
    } finally {
      loading.value = false
    }
  }

  async function removeRecent(f: PDFFile) {
    try {
      await removeRecentPDFs([f.id])
      recent.value = recent.value.filter((x) => x.id !== f.id)
      removedPath.value = ''
      removedPath.value = f.path
    } catch (e) {
      const err = toAppError(e)
      ElMessage.error(docErrorText(err.code, err.message, err.detail))
    }
  }

  return { recent, loading, currentPath, removedPath, loadRecent, removeRecent }
})
