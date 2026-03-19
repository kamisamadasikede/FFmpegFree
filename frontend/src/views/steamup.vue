<template>
  <div class="stream-table-container">
    <el-alert title="推流时请避免复用同一个推流地址，否则旧会话可能被覆盖。" type="primary" />

    <el-upload
      class="upload-demo app-upload"
      drag
      :http-request="customUpload"
      :auto-upload="true"
      :before-upload="beforeUpload"
      multiple
    >
      <el-icon class="el-icon--upload">
        <upload-filled />
      </el-icon>
      <div class="el-upload__text">文件拖拽到此处或 <em>点击上传</em></div>
    </el-upload>

    <el-progress
      v-if="uploadProgress > 0"
      :percentage="uploadProgress"
      :status="uploadProgress === 100 ? 'success' : ''"
    />

    <el-table :data="filterTableData" style="width: 100%; height: 70vh" :highlight-current-row="true" row-key="name">
      <el-table-column label="缩略图">
        <template #default="scope">
          <MediaThumb
            :url="scope.row.url"
            :name="scope.row.name"
            :cover="scope.row.cover"
            :clickable="isPreviewable(scope.row.name)"
            @preview="(url) => playFullScreenVideo(url, scope.row.name)"
          />
        </template>
      </el-table-column>

      <el-table-column label="名称" prop="name" />
      <el-table-column label="时长" prop="duration" />
      <el-table-column label="修改时间" prop="date" />

      <el-table-column align="right">
        <template #header>
          <el-input v-model="search" size="small" placeholder="搜索名称" />
        </template>
        <template #default="scope">
          <el-button size="small" @click="handlereload(scope.$index, scope.row)">推流</el-button>
          <el-button size="small" type="danger" @click="handleDelete(scope.$index, scope.row)">删除</el-button>
        </template>
      </el-table-column>

      <template #empty>
        <span>暂无数据</span>
      </template>
    </el-table>

    <MediaPreviewDialog v-model="isVideoDialogVisible" :url="selectedVideoUrl" :name="selectedVideoName" />

    <el-dialog v-model="isConvertDialogVisible" title="输入推流参数">
      <el-form @submit.prevent="submitConversion">
        <el-form-item label="推流地址">
          <el-input v-model="steamurl" placeholder="例如：rtmp://live.example.com/stream" />
        </el-form-item>

        <el-form-item label="自动录制分段">
          <el-switch v-model="archiveEnabled" />
        </el-form-item>

        <el-form-item label="分段秒数">
          <el-input-number v-model="segmentSeconds" :min="30" :max="3600" :step="30" />
        </el-form-item>

        <el-form-item label="额外转推目标（每行一个）">
          <el-input
            v-model="relayTargetsText"
            type="textarea"
            :rows="4"
            placeholder="rtmp://backup.example.com/live/stream1&#10;rtmp://backup2.example.com/live/stream1"
          />
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
import MediaThumb from '@/components/MediaThumb.vue'
import MediaPreviewDialog from '@/components/MediaPreviewDialog.vue'

import { uploadFileSteame } from '@/api/upload/upload'
import { deletesteamVideo, getSteamFiles, steamload } from '@/api/video/video'

interface VideoInfo {
  name: string
  url: string
  duration: string
  date: string
  steamurl: string
  streamId?: string
  targetFormat: string
  archiveEnabled?: boolean
  segmentSeconds?: number
  relayTargets?: string[]
  cover?: string
}

const search = ref('')
const tableData = ref<VideoInfo[]>([])
const isVideoDialogVisible = ref(false)
const selectedVideoUrl = ref('')
const selectedVideoName = ref('')
const isConvertDialogVisible = ref(false)
const steamurl = ref<string>('')
const archiveEnabled = ref(false)
const segmentSeconds = ref(300)
const relayTargetsText = ref('')
const selectedVideoForConvert = ref<VideoInfo | null>(null)
const uploadProgress = ref(0)

const previewableExts = ['.mp4', '.mov', '.webm', '.mkv', '.avi', '.flv']
const isPreviewable = (name: string) => previewableExts.some((ext) => name.toLowerCase().endsWith(ext))

// 列表层面的本地搜索，避免每次搜索都请求后端。
const filterTableData = computed(() =>
  tableData.value.filter((data) => !search.value || data.name.toLowerCase().includes(search.value.toLowerCase())),
)

const isValidStreamUrl = (url: string): boolean => /^(rtmp|rtsp):\/\/.+/.test(url)

const fetchData = async () => {
  try {
    const response = await getSteamFiles()
    if (response.data && response.data.code === 200) {
      tableData.value = response.data.data
      return
    }
    tableData.value = []
  } catch (error) {
    console.error('加载推流素材失败:', error)
    tableData.value = []
  }
}

onMounted(async () => {
  await fetchData()
})

const customUpload = async (options: UploadRequestOptions) => {
  const formData = new FormData()
  formData.append('file', options.file)

  uploadProgress.value = 0
  const response = await uploadFileSteame(formData, (percent: number) => {
    uploadProgress.value = percent
  })

  if (response.data.code === 200) {
    await fetchData()
    ElMessage.success('上传成功')
  } else {
    ElMessage.error('上传失败')
  }
}

const playFullScreenVideo = (url: string, name?: string) => {
  selectedVideoUrl.value = url
  selectedVideoName.value = name || ''
  isVideoDialogVisible.value = true
}

const handleDelete = async (index: number, row: VideoInfo) => {
  const res = await deletesteamVideo(row)
  if (res.data.code === 200) {
    tableData.value = tableData.value.filter((item) => item.name !== row.name)
    ElMessage.success('删除成功')
    return
  }
  ElMessage.error(`删除失败: ${res.data.message || '未知错误'}`)
}

const handlereload = (index: number, row: VideoInfo) => {
  selectedVideoForConvert.value = row
  isConvertDialogVisible.value = true
  archiveEnabled.value = false
  segmentSeconds.value = 300
  relayTargetsText.value = ''
}

const submitConversion = async () => {
  if (!selectedVideoForConvert.value) {
    ElMessage.warning('请先选择一个视频文件')
    return
  }

  if (!steamurl.value || !isValidStreamUrl(steamurl.value)) {
    ElMessage.warning('请输入有效的 RTMP 或 RTSP 地址')
    return
  }

  try {
    // 按行拆分一键转推目标，前后空格会被自动清理。
    const relayTargets = relayTargetsText.value
      .split('\n')
      .map((item) => item.trim())
      .filter((item) => item.length > 0)

    // 与后端增强推流接口字段保持一致：
    // - archiveEnabled: 是否开启自动录制
    // - segmentSeconds: 分段切片时长
    // - relayTargets: 额外转推目标
    const videoInfo = {
      ...selectedVideoForConvert.value,
      steamurl: steamurl.value,
      archiveEnabled: archiveEnabled.value,
      segmentSeconds: segmentSeconds.value,
      relayTargets,
    }

    const res = await steamload(videoInfo)

    if (res.data.code === 200) {
      ElMessage.success('推流任务已提交')
      isConvertDialogVisible.value = false
      steamurl.value = ''
      await fetchData()
    } else {
      ElMessage.error(`推流任务提交失败: ${res.data.message || '未知错误'}`)
    }
  } catch (error) {
    console.error('推流请求失败:', error)
    ElMessage.error('提交推流任务时发生错误')
  }
}

const beforeUpload = (file: File): boolean => {
  const isValidType = ['video/mp4'].includes(file.type)
  if (!isValidType) {
    ElMessage.error('只支持上传 MP4 视频')
    return false
  }
  return true
}
</script>

<style scoped>
.stream-table-container {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.app-upload {
  width: 100%;
}

.fullscreen-video {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
</style>
