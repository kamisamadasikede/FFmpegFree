<template>
  <el-table :data="filterTableData" style="width: 100%;height: 70vh"
            :highlight-current-row="true"
            row-key="name"
            v-if="tableDataLoaded">
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
    <el-table-column align="right">
      <template #header>
        <el-input v-model="search" size="small" placeholder="搜索名称" />
      </template>
      <template #default="scope">
        <el-button size="small" @click="handleEdit(scope.row)">
          下载视频
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
</template>
<script setup lang="ts">
import {computed, onMounted, ref} from "vue";

import {ElMessage} from "element-plus";
import {convertUp, deleteUpsc, downloadVideo} from "@/api/video/video";

interface VideoInfo {
  name: string
  url: string
  duration: string
  date: string
  targetFormat: string
}
const tableDataLoaded = ref(false)
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

const fetchData = async () => {
  const res = await convertUp();
  console.log(res.data.code)
  if (res.data.code === 200){

    tableData.value = res.data.data
  }else {
    tableData.value = []
  }
}
const handleEdit = async (row: VideoInfo) => {
  console.log(row)
  try {
    const response = await downloadVideo(row.name)
    console.log([response])
    // 创建 Blob 对象
    const blob = new Blob([response])
    const downloadUrl = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = downloadUrl
    link.setAttribute('download', row.name)
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(downloadUrl)
  } catch (error) {
    console.error('文件下载失败:', error)
    ElMessage({
      message: '文件下载失败',
      type: 'error',
    })
  }
}
const handleDelete = async (index: number, row: VideoInfo) => {
  const res = await deleteUpsc(row)

  if (res.data.code === 200) {
    // ✅ 直接从 tableData 中过滤掉当前 row
    tableData.value = tableData.value.filter(item => item.name !== row.name)

    ElMessage.success('删除成功')
  } else {
    ElMessage.error('删除失败')
  }
}



onMounted(() => {
  fetchData().finally(() => {
    tableDataLoaded.value = true
  })
  setInterval(fetchData, 10000)
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