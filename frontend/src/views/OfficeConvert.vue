<template>
  <div class="office-container">
    <el-tabs v-model="activeTab" type="border-card">
      <el-tab-pane label="上传转换" name="convert">
        <div class="upload-section">
          <el-alert
            title="支持 Word、Excel、PowerPoint 文件转换为 PDF"
            type="info"
            :closable="false"
            style="margin-bottom: 20px;"
          />

          <el-upload
            class="office-upload"
            drag
            :http-request="customUpload"
            :auto-upload="true"
            :before-upload="beforeUpload"
            multiple
          >
            <el-icon class="el-icon--upload"><upload-filled /></el-icon>
            <div class="el-upload__text">
              文件拖动此处 或 <em>点击上传</em>
            </div>
            <template #tip>
              <div class="el-upload__tip">
                支持 .doc, .docx, .xls, .xlsx, .ppt, .pptx 格式
              </div>
            </template>
          </el-upload>

          <el-progress
            v-if="uploadProgress > 0"
            :percentage="uploadProgress"
            :status="uploadProgress === 100 ? 'success' : ''"
            style="margin-top: 20px;"
          />
        </div>

        <div class="file-list-section" v-if="fileList.length > 0">
          <el-divider content-position="left">待转换文件列表</el-divider>

          <el-table :data="fileList" style="width: 100%">
            <el-table-column prop="name" label="文件名" />
            <el-table-column prop="size" label="大小" width="120">
              <template #default="scope">
                {{ formatFileSize(scope.row.size) }}
              </template>
            </el-table-column>
            <el-table-column label="操作" width="200">
              <template #default="scope">
                <el-button
                  type="primary"
                  size="small"
                  @click="convertToPDF(scope.row)"
                  :disabled="convertingFiles.has(scope.row.name)"
                >
                  {{ convertingFiles.has(scope.row.name) ? '转换中...' : '转换为PDF' }}
                </el-button>
                <el-button
                  type="danger"
                  size="small"
                  @click="deleteFile(scope.row)"
                >
                  删除
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>

      <el-tab-pane label="转换记录" name="records">
        <div class="records-section">
          <el-alert
            title="已转换完成的 PDF 文件列表"
            type="success"
            :closable="false"
            style="margin-bottom: 20px;"
          />

          <el-table :data="convertedFiles" style="width: 100%" v-loading="loading">
            <el-table-column label="文件名" prop="name" />
            <el-table-column label="预览" width="100">
              <template #default="scope">
                <el-button
                  type="primary"
                  size="small"
                  @click="previewPDF(scope.row)"
                >
                  预览
                </el-button>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="200">
              <template #default="scope">
                <el-button
                  type="success"
                  size="small"
                  @click="downloadPDF(scope.row)"
                >
                  下载
                </el-button>
                <el-button
                  type="danger"
                  size="small"
                  @click="deletePDF(scope.row)"
                >
                  删除
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="previewVisible" title="PDF预览" fullscreen>
      <iframe
        :src="previewUrl"
        style="width: 100%; height: 80vh; border: none;"
      />
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, UploadRequestOptions } from 'element-plus'
import { UploadFilled } from '@element-plus/icons-vue'
import {
  uploadOfficeFile,
  convertOfficeToPDF,
  getOfficeFiles,
  getConvertedPDFiles,
  downloadOfficePDF,
  deleteOfficeFile,
  deleteOfficePDF,
  OfficeInfo,
} from '@/api/office/office'

interface UploadFile {
  name: string
  size: number
  url: string
}

const activeTab = ref('convert')
const fileList = ref<UploadFile[]>([])
const convertedFiles = ref<OfficeInfo[]>([])
const uploadProgress = ref(0)
const convertingFiles = ref(new Set<string>())
const loading = ref(false)
const previewVisible = ref(false)
const previewUrl = ref('')

const allowedExtensions = ['.doc', '.docx', '.xls', '.xlsx', '.ppt', '.pptx']

const beforeUpload = (file: File): boolean => {
  const ext = file.name.substring(file.name.lastIndexOf('.')).toLowerCase()
  if (!allowedExtensions.includes(ext)) {
    ElMessage.error('不支持的文件格式，仅支持 Word、Excel、PowerPoint 文件')
    return false
  }
  return true
}

const customUpload = async (options: UploadRequestOptions) => {
  const formData = new FormData()
  const { file } = options
  formData.append('file', file)

  uploadProgress.value = 0

  try {
    const response = await uploadOfficeFile(formData, (percent: number) => {
      uploadProgress.value = percent
    })

    if (response.data.code === 200) {
      fileList.value.push({
        name: response.data.data.fileName,
        size: file.size,
        url: response.data.data.url,
      })
      ElMessage.success('上传成功')
    } else {
      ElMessage.error('上传失败')
    }
  } catch (error) {
    console.error('上传错误:', error)
    ElMessage.error('上传失败')
  }
}

const convertToPDF = async (file: UploadFile) => {
  convertingFiles.value.add(file.name)

  try {
    const response = await convertOfficeToPDF({
      name: file.name,
      url: file.url,
    })

    if (response.data.code === 200) {
      ElMessage.success('转换任务已提交，请稍候在转换记录中查看')
    } else {
      ElMessage.error('转换失败: ' + (response.data.message || '未知错误'))
    }
  } catch (error) {
    console.error('转换错误:', error)
    ElMessage.error('转换失败')
  } finally {
    convertingFiles.value.delete(file.name)
  }
}

const deleteFile = async (file: UploadFile) => {
  try {
    const response = await deleteOfficeFile({
      name: file.name,
      url: file.url,
    })

    if (response.data.code === 200) {
      fileList.value = fileList.value.filter((f) => f.name !== file.name)
      ElMessage.success('删除成功')
    } else {
      ElMessage.error('删除失败')
    }
  } catch (error) {
    console.error('删除错误:', error)
    ElMessage.error('删除失败')
  }
}

const fetchConvertedFiles = async () => {
  loading.value = true
  try {
    const response = await getConvertedPDFiles()
    if (response.data.code === 200) {
      convertedFiles.value = response.data.data || []
    }
  } catch (error) {
    console.error('获取转换记录失败:', error)
  } finally {
    loading.value = false
  }
}

const previewPDF = (file: OfficeInfo) => {
  previewUrl.value = file.url
  previewVisible.value = true
}

const downloadPDF = async (file: OfficeInfo) => {
  try {
    const response = await downloadOfficePDF(file.name)
    const blob = new Blob([response])
    const downloadUrl = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = downloadUrl
    link.setAttribute('download', file.name)
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(downloadUrl)
    ElMessage.success('下载成功')
  } catch (error) {
    console.error('下载失败:', error)
    ElMessage.error('下载失败')
  }
}

const deletePDF = async (file: OfficeInfo) => {
  try {
    const response = await deleteOfficePDF({
      name: file.name,
      url: file.url,
    })

    if (response.data.code === 200) {
      convertedFiles.value = convertedFiles.value.filter((f) => f.name !== file.name)
      ElMessage.success('删除成功')
    } else {
      ElMessage.error('删除失败')
    }
  } catch (error) {
    console.error('删除错误:', error)
    ElMessage.error('删除失败')
  }
}

const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

onMounted(() => {
  fetchConvertedFiles()
})
</script>

<style scoped>
.office-container {
  padding: 20px;
  max-width: 1200px;
  margin: 0 auto;
}

.upload-section {
  padding: 20px;
}

.office-upload {
  width: 100%;
}

.file-list-section {
  margin-top: 30px;
}

.records-section {
  padding: 20px;
}
</style>
