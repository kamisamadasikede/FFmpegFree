<template>
  <el-alert title="推流请勿使用同一个流地址，否则会使之前的推流自动终止。" type="primary" />
  <el-upload
      class="upload-demo"
      drag
      :http-request="customUpload"
      :auto-upload="true"
      :before-upload="beforeUpload"
      multiple
      style="width: 100%;min-width: 600px"
  >
    <el-icon class="el-icon--upload">
      <upload-filled />
    </el-icon>
    <div class="el-upload__text">
      文件拖动此处 或 <em>点击上传</em>
    </div>
  </el-upload>
  <!-- 显示上传进度 -->
  <el-progress
      v-if="uploadProgress > 0"
      :percentage="uploadProgress"
      :status="uploadProgress === 100 ? 'success' : ''"
  />
  <el-table :data="filterTableData" style="width: 100%;height: 70vh"
            :highlight-current-row="true"
            row-key="name"
            v-if="tableDataLoaded">
    <el-table-column label="略缩图"  >
      <template #default="scope">
        <video
            :src="scope.row.url"      style="width: 260px;"
            @click="playFullScreenVideo(scope.row.url)"
        ></video>
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
        <el-button
            size="small"
            type=""
            @click="handlereload(scope.$index, scope.row)"
        >
          推流
        </el-button>
        <el-button
            size="small"
            type="danger"
            @click="handleDelete(scope.$index, scope.row)"
        >
          删除
        </el-button>
      </template>
    </el-table-column>
  </el-table>
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
  <el-dialog v-model="isConvertDialogVisible" title="输入推流地址">
    <el-form @submit.prevent="submitConversion">
      <el-form-item label="目标格式">
        <el-input v-model="steamurl" placeholder="输入推流地址">
        </el-input>
      </el-form-item>
      <el-button type="primary" native-type="submit">提交</el-button>
    </el-form>
  </el-dialog>
</template>


<script setup lang="ts">
import {computed, onMounted, ref} from "vue";
import {ElUpload, ElButton, ElProgress, ElMessage} from 'element-plus'
import * as wails from '../../wailsjs/runtime'
import { UploadFilled } from '@element-plus/icons-vue'
import { uploadFileSteame} from '@/api/upload/upload'
import {
  convertreload,
  deletesteamVideo,
  deleteUp,
  deleteUpsc,
  getConvertingFiles,
  getSteamFiles,
  steamload
} from "@/api/video/video";
interface VideoInfo {
  name: string
  url: string
  duration: string
  date: string
  steamurl: string
  targetFormat: string
}
interface StreamEventData {
  filename: string
  streamUrl: string
  status: 'completed' | 'failed'
  error?: string
}
// 校验是否是合法的 rtmp 或 rtsp 地址
const isValidStreamUrl = (url: string): boolean => {
  const pattern = /^(rtmp|rtsp):\/\/.+/
  return pattern.test(url)
}
const tableDataLoaded = ref(false)
const isConvertDialogVisible = ref(false)
const steamurl = ref<string>('')
const selectedVideoForConvert = ref<VideoInfo | null>(null)
const search = ref('')
const tableData = ref<VideoInfo[]>([])
const isVideoDialogVisible = ref(false)
const selectedVideoUrl = ref('')
const uploadRef = ref()
const uploadProgress = ref(0)
const filterTableData = computed(() =>
    tableData.value.filter(data =>
        !search.value ||
        data.name.toLowerCase().includes(search.value?.toLowerCase() ?? '')
    )
)
// 点击视频时触发的方法
const playFullScreenVideo = (url :string) => {
  selectedVideoUrl.value = url
  isVideoDialogVisible.value = true
}
import type { UploadRequestOptions } from 'element-plus'
const customUpload = async (options:UploadRequestOptions) => {
  const formData = new FormData()
  const { file } = options
  formData.append('file', file)

  uploadProgress.value = 0 // 重置进度条

  const response = await uploadFileSteame(formData, (percent : number) => {
    uploadProgress.value = percent
  })
  if (response.data.code === 200) {
    fetchData()
    uploadRef.value.clearFiles()
  }else {
    alert('上传失败')
  }

}
const submitConversion = async () => {
  if (!selectedVideoForConvert.value) {
    ElMessage.error('请选择一个视频文件')
    return
  }

  if (!steamurl.value || !isValidStreamUrl(steamurl.value)) {
    ElMessage.error('请输入有效的 RTMP 或 RTSP 地址（如：rtmp://xxx 或 rtsp://xxx）')
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
      fetchData() // 刷新表格数据
    } else {
      ElMessage.error('推流任务提交失败，请重试。')
    }
  } catch (error) {
    console.error('推流错误:', error)
    ElMessage.error('提交推流任务时发生错误')
  }
}

const handlereload = (index: number, row: VideoInfo) => {
  console.log(index, row)
  selectedVideoForConvert.value = row
  isConvertDialogVisible.value = true

}
const handleDelete = async (index: number, row: VideoInfo) => {
  const res = await deletesteamVideo(row)

  if (res.data.code === 200) {
    // ✅ 直接从 tableData 中过滤掉当前 row
    tableData.value = tableData.value.filter(item => item.name !== row.name)
    ElMessage.success('删除成功')
  } else {
    ElMessage.error('删除失败：'+res.data.error)
  }
}
const beforeUpload = (file :File) => {
  const isValidType = ['video/mp4'].includes(file.type)
  if (!isValidType) {
    alert('只能上传图片或视频!')
    return false
  }
  return true
}


const fetchData = async () => {
  const response = await getSteamFiles()
  if (response.data.code === 200) {
    tableData.value = response.data.data
  } else {
    tableData.value = []
  }
}

onMounted(() => {
  fetchData().finally(() => {
    tableDataLoaded.value = true
  })

})
</script>
