<template>
  <div class="pdf-page-container">
    <div class="control-panel">
      <el-row :gutter="20" align="middle">
        <el-col :span="10">
          <el-upload
            class="compact-upload"
            action=""
            :http-request="customUpload"
            :show-file-list="false"
            :before-upload="beforeUploadCheck"
          >
            <el-button type="primary" :icon="UploadFilled">上传 PDF</el-button>
          </el-upload>
        </el-col>
        <el-col :span="14">
          <div class="url-input-group">
            <el-input v-model="pdfUrl" placeholder="输入 PDF 网络地址..." clearable>
              <template #append>
                <el-button @click="loadNewPDF">远程加载</el-button>
              </template>
            </el-input>
          </div>
        </el-col>
      </el-row>

      <div class="viewer-toolbar">
        <div class="file-meta">
          <div class="file-name">{{ currentFile?.name || '未选择文件' }}</div>
          <div class="file-info">
            <span>大小：{{ currentFileSize }}</span>
            <span v-if="pageSizeText">页面：{{ pageSizeText }}</span>
          </div>
        </div>
        <div class="zoom-controls">
          <el-button-group>
            <el-button @click="zoomOut" :disabled="scale <= minScale">-</el-button>
            <el-button class="zoom-display">{{ Math.round(scale * 100) }}%</el-button>
            <el-button @click="zoomIn" :disabled="scale >= maxScale">+</el-button>
          </el-button-group>
          <el-slider
            v-model="scale"
            :min="minScale"
            :max="maxScale"
            :step="0.1"
            :show-tooltip="false"
            class="zoom-slider"
          />
        </div>
      </div>

      <el-progress v-if="uploadProgress > 0 && uploadProgress < 100" :percentage="uploadProgress" />
    </div>

    <div class="main-layout">
      <div class="side-bar">
        <div class="file-history" v-if="fileList.length > 0">
          <div class="section-title">最近文件</div>
          <div class="history-list">
            <div v-for="item in fileList" :key="item.name" 
                 class="history-item" :class="{ active: pdfUrl === item.url }"
                 @click="loadFromUpload(item)">
              <el-icon class="pdf-type-icon"><Document /></el-icon>
              <span class="name">{{ item.name }}</span>
              <el-icon class="del-action-btn" @click.stop="handleDelete(item)"><Close /></el-icon>
            </div>
          </div>
        </div>

        <el-divider v-if="hasPdf" />

        <div v-if="hasPdf" class="ppt-thumb-wrapper">
          <div class="section-title">页面预览 ({{ pages }})</div>
          <div v-for="pageNum in lazyPage" :key="pageNum" 
               class="ppt-thumb-item" :class="{ active: pageNum === page }" 
               @click="page = pageNum">
            <span class="thumb-number">{{ pageNum }}</span>
            <div class="thumb-canvas-box">
              <VuePDF :pdf="pdf" :page="pageNum" :fit-parent="true" />
            </div>
          </div>
        </div>
      </div>

      <div class="viewer-canvas" v-loading="loading">
        <div v-if="hasPdf" class="ppt-stage">
          <div class="ppt-slide-scroll-viewport">
            <div class="ppt-slide-paper">
              <VuePDF
                :pdf="pdf"
                :page="page"
                :scale="scale"
                @loaded="handleLoaded"
                @error="handlePdfError"
              />
            </div>
          </div>
          <div class="floating-controls">
            <el-button-group>
              <el-button :icon="ArrowLeft" @click="prevPage" :disabled="page <= 1" />
              <el-button class="page-display">{{ page }} / {{ pages }}</el-button>
              <el-button :icon="ArrowRight" @click="nextPage" :disabled="page >= pages" />
            </el-button-group>
          </div>
        </div>
        <el-empty v-else description="暂无预览内容" :image-size="200" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, computed } from 'vue'
import { VuePDF, usePDF } from '@tato30/vue-pdf'
import { ElMessage, ElMessageBox } from 'element-plus'
import { UploadFilled, Document, Close, ArrowLeft, ArrowRight } from '@element-plus/icons-vue'
import { uploadPDFFile, getPDFFiles, deletePDFFile } from '@/api/pdf/pdf'

// 1. 定义接口解决 TS2339 错误
interface PDFFile {
  name: string;
  url: string;
  size?: number;
}

const page = ref(1)
const lazyPage = ref(10)
const pdfUrl = ref('')
const pdfData = ref<string | null>(null)
const loading = ref(false)
const { pdf, pages } = usePDF(pdfData)
const scale = ref(1)
const minScale = 0.6
const maxScale = 2.2

// 2. 为 ref 指定 PDFFile 数组类型
const fileList = ref<PDFFile[]>([])
const uploadProgress = ref(0)
const hasPdf = computed(() => !!pdfData.value)
const currentFile = computed(() => fileList.value.find((item) => item.url === pdfUrl.value))
const currentFileSize = computed(() => formatSize(currentFile.value?.size))
const pageSizeText = ref('')

const prevPage = () => { if (page.value > 1) page.value-- }
const nextPage = () => {
  if (page.value < pages.value) {
    page.value++
    if (lazyPage.value - page.value < 3) lazyPage.value = Math.min(lazyPage.value + 10, pages.value)
  }
}

const beforeUploadCheck = (file: File) => {
  const ok = file.name.toLowerCase().endsWith('.pdf')
  if (!ok) {
    ElMessage.error('仅支持 PDF 文件')
  }
  return ok
}

const loadNewPDF = () => {
  if (!pdfUrl.value) return
  loading.value = true
  page.value = 1
  lazyPage.value = 10
  scale.value = 1
  pdfData.value = pdfUrl.value
}

// 3. 修改参数类型从 any 到 PDFFile
const loadFromUpload = (item: PDFFile) => {
  loading.value = true
  pdfUrl.value = item.url
  page.value = 1
  lazyPage.value = 10
  scale.value = 1
  pdfData.value = item.url
}

const customUpload = async (options: any) => {
  const formData = new FormData()
  formData.append('file', options.file)
  try {
    const res = await uploadPDFFile(formData, (p: number) => uploadProgress.value = p)
    if (res.data.code === 200) {
      ElMessage.success('上传成功')
      fetchPDFiles()
      loadFromUpload({ 
        name: res.data.data.fileName, 
        url: res.data.data.url,
        size: res.data.data.size
      })
    }
  } catch (e) { ElMessage.error('上传失败') }
  finally { uploadProgress.value = 0 }
}

const fetchPDFiles = async () => {
  const res = await getPDFFiles()
  if (res.data.code === 200) {
    // 确保赋值时类型匹配
    fileList.value = res.data.data || []
  }
}

const handleDelete = (item: PDFFile) => {
  ElMessageBox.confirm('确定删除吗？').then(async () => {
    const res = await deletePDFFile({ name: item.name, url: item.url })
    if (res.data.code === 200) {
      fetchPDFiles()
      if (pdfUrl.value === item.url) pdfData.value = null
    }
  })
}

const zoomIn = () => {
  scale.value = Math.min(maxScale, +(scale.value + 0.1).toFixed(1))
}
const zoomOut = () => {
  scale.value = Math.max(minScale, +(scale.value - 0.1).toFixed(1))
}

const handleLoaded = () => {
  loading.value = false
  updatePageSize()
}

const handlePdfError = () => {
  loading.value = false
  ElMessage.error('PDF 加载失败，请检查文件或地址')
}

const updatePageSize = async () => {
  if (!pdf.value) {
    pageSizeText.value = ''
    return
  }
  try {
    const pageRef: any = await (pdf.value as any).getPage(page.value)
    const viewport = pageRef.getViewport({ scale: 1 })
    const widthMm = Math.round((viewport.width * 25.4) / 72)
    const heightMm = Math.round((viewport.height * 25.4) / 72)
    pageSizeText.value = `${widthMm} × ${heightMm} mm`
  } catch {
    pageSizeText.value = ''
  }
}

const formatSize = (size?: number) => {
  if (!size || size <= 0) return '--'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = size
  let idx = 0
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024
    idx++
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[idx]}`
}

watch(page, () => {
  const activeThumb = document.querySelector('.ppt-thumb-item.active')
  if (activeThumb) activeThumb.scrollIntoView({ behavior: 'smooth', block: 'center' })
  const viewer = document.querySelector('.ppt-slide-scroll-viewport')
  if (viewer) viewer.scrollTop = 0
  updatePageSize()
})

watch(pdfData, (value) => {
  if (!value) {
    loading.value = false
    pageSizeText.value = ''
  }
})

onMounted(fetchPDFiles)
</script>

<style scoped>
/* 样式部分保持不变，已在之前回复中优化去黑边逻辑 */
.pdf-page-container {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background-color: #f0f2f5;
  padding: 16px;
  box-sizing: border-box;
  overflow: hidden;
}
.control-panel { background: #fff; padding: 12px 24px; border-radius: 8px; margin-bottom: 16px; flex-shrink: 0; }
.viewer-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-top: 12px; flex-wrap: wrap; }
.file-meta { display: flex; flex-direction: column; gap: 4px; min-width: 220px; }
.file-name { font-weight: 600; color: #303133; }
.file-info { font-size: 12px; color: #909399; display: flex; gap: 12px; }
.zoom-controls { display: flex; align-items: center; gap: 12px; }
.zoom-slider { width: 160px; }
.zoom-display { min-width: 70px; text-align: center; }
.main-layout { flex: 1; display: flex; gap: 16px; overflow: hidden; }
.side-bar { width: 280px; background: #fff; border-radius: 8px; display: flex; flex-direction: column; padding: 12px; flex-shrink: 0; }
.section-title { font-size: 13px; font-weight: bold; color: #606266; margin-bottom: 12px; }
.file-history { max-height: 200px; display: flex; flex-direction: column; }
.history-list { overflow-y: auto; flex: 1; }
.history-item { display: flex; align-items: center; padding: 8px 10px; margin-bottom: 4px; border-radius: 6px; cursor: pointer; transition: 0.2s; background: #f8f9fa; }
.history-item .name { flex: 1; font-size: 13px; color: #303133 !important; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pdf-type-icon { color: #f56c6c; margin-right: 8px; font-size: 16px; }
.del-action-btn { font-size: 14px; color: #909399; opacity: 0; transition: 0.2s; cursor: pointer; }
.history-item:hover { background-color: #ecf5ff; }
.history-item:hover .del-action-btn { opacity: 1; }
.del-action-btn:hover { color: #f56c6c; background: #ffeded; border-radius: 4px; }
.history-item.active { background-color: #eef6fe; border: 1px solid #409eff; }
.history-item.active .name { color: #409eff !important; font-weight: bold; }
.ppt-thumb-wrapper { flex: 1; overflow-y: auto; padding-right: 4px; }
.ppt-thumb-item { display: flex; gap: 10px; margin-bottom: 15px; cursor: pointer; padding: 8px; border-radius: 6px; border: 2px solid transparent; }
.ppt-thumb-item.active { background: #eef6fe; border-color: #409eff; }
.thumb-canvas-box { flex: 1; height: 120px; background: #f8f9fa; overflow: hidden; display: flex; justify-content: center; }
.thumb-canvas-box :deep(canvas) { max-width: 100% !important; max-height: 100% !important; object-fit: contain; }
.viewer-canvas { flex: 1; background: #323639; border-radius: 8px; overflow: hidden; position: relative; }
.ppt-stage { width: 100%; height: 100%; display: flex; flex-direction: column; }
.ppt-slide-scroll-viewport { flex: 1; overflow-y: auto; padding: 20px 0; display: flex; justify-content: center; }
.ppt-slide-paper { width: 100%; max-width: 900px; background: #fff; box-shadow: 0 10px 30px rgba(0,0,0,0.5); height: fit-content; line-height: 0; }
:deep(.vue-pdf-main) { width: 100% !important; height: auto !important; padding: 0 !important; margin: 0 !important; display: block !important; }
:deep(.vue-pdf-main canvas) { width: 100% !important; height: auto !important; display: block !important; vertical-align: middle; }
.floating-controls { position: absolute; bottom: 30px; left: 50%; transform: translateX(-50%); z-index: 100; }
.page-display { background: #fff !important; color: #333 !important; font-weight: bold; }
::-webkit-scrollbar { width: 6px; }
::-webkit-scrollbar-thumb { background: #b1b3b8; border-radius: 10px; }
</style>
