<template>
  <div class="stream-table-container">
    <!-- 提示 -->
    <el-alert title="推流请勿使用同一个流地址，否则会使之前的推流自动终止。" type="primary" />

    <!-- 文件上传 -->
    <el-upload
        class="upload-demo"
        drag
        :http-request="customUpload"
        :auto-upload="true"
        :before-upload="beforeUpload"
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
            推流
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

    <!-- 推流地址对话框 -->
    <el-dialog v-model="isConvertDialogVisible" title="输入推流地址">
      <el-form @submit.prevent="submitConversion">
        <el-form-item label="推流地址">
          <el-input v-model="steamurl" placeholder="例如：rtmp://live.example.com/stream" />
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

import { uploadFileSteame } from '@/api/upload/upload'
import {
  convertreload,
  deletesteamVideo,
  getSteamFiles,
  steamload
} from '@/api/video/video'

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
const steamurl = ref<string>('')
const selectedVideoForConvert = ref<VideoInfo | null>(null)
const uploadProgress = ref(0)

// 过滤后的表格数据
const filterTableData = computed(() =>
    tableData.value.filter((data) =>
        !search.value || data.name.toLowerCase().includes(search.value.toLowerCase())
    )
)

// 校验是否是合法的 rtmp 或 rtsp 地址
const isValidStreamUrl = (url: string): boolean => {
  const pattern = /^(rtmp|rtsp):\/\/.+/
  return pattern.test(url)
}

// 获取数据
const fetchData = async () => {
  try {
    const response = await getSteamFiles()
    if (response.data.code === 200) {
      tableData.value = response.data.data
    } else {
      tableData.value = []
    }
  } catch (error) {
    console.error('获取推流文件列表失败:', error)
    tableData.value = []
  }
}

onMounted(async () => {
  await fetchData()
})

// 自定义上传方法
const customUpload = async (options: UploadRequestOptions) => {
  const formData = new FormData()
  const { file } = options
  formData.append('file', file)

  uploadProgress.value = 0 // 重置进度条

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

// 播放全屏视频
const playFullScreenVideo = (url: string) => {
  selectedVideoUrl.value = url
  isVideoDialogVisible.value = true
}

// 删除操作
const handleDelete = async (index: number, row: VideoInfo) => {
  const res = await deletesteamVideo(row)

  if (res.data.code === 200) {
    tableData.value = tableData.value.filter((item) => item.name !== row.name)
    ElMessage.success('删除成功')
  } else {
    ElMessage.error('删除失败：' + (res.data.message || '未知错误'))
  }
}

// 推流操作
const handlereload = (index: number, row: VideoInfo) => {
  selectedVideoForConvert.value = row
  isConvertDialogVisible.value = true
}

const submitConversion = async () => {
  if (!selectedVideoForConvert.value) {
    ElMessage.warning('请选择一个视频文件')
    return
  }

  if (!steamurl.value || !isValidStreamUrl(steamurl.value)) {
    ElMessage.warning('请输入有效的 RTMP 或 RTSP 地址（如：rtmp://xxx 或 rtsp://xxx）')
    return
  }

  try {
    const videoInfo = {
      ...selectedVideoForConvert.value,
      steamurl: steamurl.value
    }

    const res = await steamload(videoInfo)

    if (res.data.code === 200) {
      ElMessage.success('推流任务已提交')
      isConvertDialogVisible.value = false
      await fetchData() // 刷新表格数据
    } else {
      ElMessage.error('推流任务提交失败，请重试。')
    }
  } catch (error) {
    console.error('推流错误:', error)
    ElMessage.error('提交推流任务时发生错误')
  }
}

// 上传前检查
const beforeUpload = (file: File): boolean => {
  const isValidType = ['video/mp4'].includes(file.type)
  if (!isValidType) {
    ElMessage.error('只能上传 MP4 视频!')
    return false
  }
  return true
}
</script>

<style scoped>
.stream-table-container {
  padding: 20px;
}
.fullscreen-video {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
</style>