<template>
  <el-table :data="filterTableData" style="width: 100%;height: 70vh"
            :highlight-current-row="true"
  >
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
    <el-table-column label="拉流地址" prop="steamurl" />
    <el-table-column align="right">
      <template #header>
        <el-input v-model="search" size="small" placeholder="搜索名称" />
      </template>
      <template #default="scope">
        <el-button
            size="small"
            type="danger"
            @click="handleDelete(scope.$index, scope.row)"
        >
          停止推流
        </el-button>
      </template>
    </el-table-column>
  </el-table>
  <el-dialog v-model="isVideoDialogVisible" fullscreen>
    <div class="fullscreen-video-container">
      <video
          id="flvVideo"
          autoplay
          muted
          playsinline
          controls
          class="fullscreen-video"
      ></video>
    </div>
  </el-dialog>
</template>
<script setup lang="ts">
import {computed, nextTick, onMounted, ref} from "vue";
import axios from "axios";
import {ElMessage} from "element-plus";
import flvjs from 'flv.js'
import {GetStreamingFiles, StopStream} from "@/api/steam/steam";
interface VideoInfo {
  name: string
  url: string
  duration: string
  date: string
  steamurl: string
  targetFormat: string
}
const isVideoDialogVisible = ref(false)
const selectedVideoUrl = ref('')
const search = ref('')
const tableData = ref<VideoInfo[]>([])

const filterTableData = computed(() =>
    tableData.value.filter(data =>
        !search.value ||
        data.name.toLowerCase().includes(search.value?.toLowerCase() ?? '')
    )
)
// 点击视频时触发的方法
const playFullScreenVideo = (url: string) => {
  selectedVideoUrl.value = url
  isVideoDialogVisible.value = true
}
const handleDelete = (index: number, row: VideoInfo) => {
  console.log(index, row)
  const response = StopStream(row);
  response.then(response => {
    if (response.data.code === 200) {
      ElMessage({
        message: response.data.data.message,
        type: 'success',
      })
      fetchData()
    } else {
      ElMessage({
        message: '停止失败',
        type: 'error',
      })
    }
  })
}

const fetchData = async () => {
  try {
    const response = await GetStreamingFiles()

    // ✅ 确保即使为空也返回数组
    tableData.value = response.data.streams || []
    console.log(response.data.data)
  } catch (error) {
    tableData.value = []
  }
}
onMounted(() => {
  fetchData()
  setInterval(fetchData, 5000)
})
</script>
<style>
.fullscreen-video-container {
  display: flex;
  justify-content: center;
  align-items: center;
  width: 100%;
  height: 100vh;
  background-color: #000000;
}

.fullscreen-video {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}
</style>