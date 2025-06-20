<template>
  <div class="video-table-container">
    <!-- 文件上传 -->
    <el-upload
        class="upload-demo"
        drag
        :http-request="customUpload"
        :auto-upload="true"
        multiple
        style="width: 100%; min-width: 600px"
    >
      <el-icon class="el-icon--upload">
        <upload-filled />
      </el-icon>
      <div class="el-upload__text">
        文件拖动此处 或 <em>点击上传</em>
      </div>
    </el-upload>

    <!-- 上传进度条 -->
    <el-progress
        v-if="uploadProgress > 0"
        :percentage="uploadProgress"
        :status="uploadProgress === 100 ? 'success' : ''"
    />

    <!-- 视频表格 -->
    <el-table
        :data="filterTableData"
        style="width: 100%; height: 70vh"
        :highlight-current-row="true"
        row-key="name"
    >
      <!-- 略缩图列 -->
      <el-table-column label="略缩图">
        <template #default="scope">
          <video
              :src="scope.row.url"
              style="width: 260px; cursor: pointer"
              @click="playFullScreenVideo(scope.row.url)"
          ></video>
        </template>
      </el-table-column>

      <!-- 名称列 -->
      <el-table-column label="名称" prop="name" />

      <!-- 时长列 -->
      <el-table-column label="时长" prop="duration" />

      <!-- 修改时间列 -->
      <el-table-column label="修改时间" prop="date" />

      <!-- 操作列 -->
      <el-table-column align="right">
        <template #header>
          <el-input v-model="search" size="small" placeholder="搜索名称" />
        </template>
        <template #default="scope">
          <el-button size="small" @click="handlereload(scope.$index, scope.row)">
            转换
          </el-button>
          <el-button size="small" type="danger" @click="handleDelete(scope.$index, scope.row)">
            删除
          </el-button>
        </template>
      </el-table-column>

      <!-- 空数据提示 -->
      <template #empty>
        <span>暂无数据</span>
      </template>
    </el-table>

    <!-- 全屏播放视频对话框 -->
    <el-dialog v-model="isVideoDialogVisible" fullscreen>
      <div class="fullscreen-video-container">
        <video
            :src="selectedVideoUrl"
            autoplay
            controls
            class="fullscreen-video"
        ></video>
      </div>
    </el-dialog>

    <!-- 转换格式对话框 -->
    <el-dialog v-model="isConvertDialogVisible" title="选择转换格式">
      <el-form @submit.prevent="submitConversion">
        <el-form-item label="目标格式">
          <el-select v-model="targetFormat" placeholder="请选择">
            <el-option
                v-for="format in supportedFormats"
                :key="format"
                :label="format.toUpperCase()"
                :value="format"
            />
          </el-select>
        </el-form-item>
        <el-button type="primary" native-type="submit">提交</el-button>
      </el-form>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, UploadRequestOptions } from 'element-plus'
import { UploadFilled } from '@element-plus/icons-vue'

import { uploadFile } from '@/api/upload/upload'
import { convertreload, deleteUp, getConvertingFiles } from '@/api/video/video'

interface VideoInfo {
  name: string
  url: string
  duration: string
  date: string
  steamurl: string
  targetFormat: string
}

// 数据定义
const search = ref('')
const tableData = ref<VideoInfo[]>([])
const isVideoDialogVisible = ref(false)
const selectedVideoUrl = ref('')
const isConvertDialogVisible = ref(false)
const targetFormat = ref<string>('')
const selectedVideoForConvert = ref<VideoInfo | null>(null)
const supportedFormats = ['avi', 'mkv', 'mov', 'flv', 'mp4', 'gif', 'webm']
const uploadProgress = ref(0)

// 过滤后的表格数据
const filterTableData = computed(() =>
    tableData.value.filter((data) =>
        !search.value || data.name.toLowerCase().includes(search.value.toLowerCase())
    )
)

// 获取数据
const fetchData = async () => {
  const response = await getConvertingFiles()
  if (response.data.code === 200) {
    tableData.value = response.data.data
  } else {
    tableData.value = []
  }
}

onMounted(async () => {
  try {
    await fetchData()
  } catch (error) {
    console.error('加载数据失败:', error)
    tableData.value = []
  }
})

// 自定义上传方法
const customUpload = async (options: UploadRequestOptions) => {
  const formData = new FormData()
  const { file } = options
  formData.append('file', file)

  uploadProgress.value = 0 // 重置进度条

  const response = await uploadFile(formData, (percent: number) => {
    uploadProgress.value = percent
  })

  if (response.data.code === 200) {
    await fetchData()
    ElMessage.success('上传成功')
  } else {
    ElMessage.error('上传失败')
  }
}

// 播放全屏视频
const playFullScreenVideo = (url: string) => {
  selectedVideoUrl.value = url
  isVideoDialogVisible.value = true
}

// 删除操作
const handleDelete = async (index: number, row: VideoInfo) => {
  const res = await deleteUp(row)

  if (res.data.code === 200) {
    tableData.value = tableData.value.filter((item) => item.name !== row.name)
    ElMessage.success('删除成功')
  } else {
    ElMessage.error('删除失败')
  }
}

// 转换操作
const handlereload = (index: number, row: VideoInfo) => {
  selectedVideoForConvert.value = row
  isConvertDialogVisible.value = true
}

const submitConversion = async () => {
  if (!selectedVideoForConvert.value || !targetFormat.value) {
    ElMessage.warning('请选择目标格式')
    return
  }

  try {
    const videoInfo = {
      ...selectedVideoForConvert.value,
      targetFormat: targetFormat.value
    }

    const res = await convertreload(videoInfo)

    if (res.data.code === 200) {
      ElMessage.success('转换任务提交成功')
      isConvertDialogVisible.value = false
      await fetchData() // 刷新表格数据
    } else {
      ElMessage.error('转换任务提交失败，请重试。')
    }
  } catch (error) {
    console.error('转换错误:', error)
    ElMessage.error('提交转换时发生错误')
  }
}
</script>

<style scoped>
.video-table-container {
  padding: 20px;
}
.fullscreen-video {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
</style>