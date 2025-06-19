<template>
  <el-upload
    class="upload-demo"
    drag
    :http-request="customUpload"
    :auto-upload="true"
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
          转换
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
</template>


<script setup lang="ts">
import {computed, onMounted, ref} from "vue";
import {ElUpload, ElButton, ElProgress, ElMessage} from 'element-plus'
import { UploadFilled } from '@element-plus/icons-vue'
import { uploadFile } from '@/api/upload/upload'
import {convertreload, deleteUp, deleteUpsc, getConvertingFiles} from "@/api/video/video";
interface VideoInfo {
  name: string
  url: string
  duration: string
  date: string
  targetFormat: string
}
const tableDataLoaded = ref(false)
const isConvertDialogVisible = ref(false)
const targetFormat = ref<string>('')
const selectedVideoForConvert = ref<VideoInfo | null>(null)
const supportedFormats = ['avi', 'mkv', 'mov', 'flv','mp4','gif','webm']
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

    const response = await uploadFile(formData, (percent : number) => {
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
  if (!selectedVideoForConvert.value || !targetFormat.value) {
    alert('请选择目标格式')
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
      fetchData() // 刷新表格数据
    } else {
      alert('转换任务提交失败，请重试。')
    }
  } catch (error) {
    console.error('转换错误:', error)
    alert('提交转换时发生错误')
  }
}

const handlereload = (index: number, row: VideoInfo) => {
  console.log(index, row)
  selectedVideoForConvert.value = row
  isConvertDialogVisible.value = true

}
const handleDelete = async (index: number, row: VideoInfo) => {
  const res = await deleteUp(row)

  if (res.data.code === 200) {
    // ✅ 直接从 tableData 中过滤掉当前 row
    tableData.value = tableData.value.filter(item => item.name !== row.name)

    ElMessage.success('删除成功')
  } else {
    ElMessage.error('删除失败')
  }
}
const beforeUpload = (file :File) => {
  const isValidType = ['image/jpeg', 'image/png', 'video/mp4'].includes(file.type)
  if (!isValidType) {
    alert('只能上传图片或视频!')
    return false
  }
  return true
}


const fetchData = async () => {
  const response = await getConvertingFiles()
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
