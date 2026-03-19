<template>
  <div class="stream-table-container">
    <!-- 鎻愮ず -->
    <el-alert title="鎺ㄦ祦璇峰嬁浣跨敤鍚屼竴涓祦鍦板潃锛屽惁鍒欎細浣夸箣鍓嶇殑鎺ㄦ祦鑷姩缁堟銆? type="primary" />

    <!-- 鏂囦欢涓婁紶 -->
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
      <div class="el-upload__text">
        鏂囦欢鎷栧姩姝ゅ 鎴?<em>鐐瑰嚮涓婁紶</em>
      </div>
    </el-upload>

    <!-- 涓婁紶杩涘害鏉?-->
    <el-progress
        v-if="uploadProgress > 0"
        :percentage="uploadProgress"
        :status="uploadProgress === 100 ? 'success' : ''"
    />

    <!-- 瑙嗛琛ㄦ牸 -->
    <el-table
        :data="filterTableData"
        style="width: 100%; height: 70vh"
        :highlight-current-row="true"
        row-key="name"
    >
      <!-- 鐣ョ缉鍥惧垪 -->
    <el-table-column label="鐣ョ缉鍥?>
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

      <!-- 鍚嶇О鍒?-->
      <el-table-column label="鍚嶇О" prop="name" />

      <!-- 鏃堕暱鍒?-->
      <el-table-column label="鏃堕暱" prop="duration" />

      <!-- 淇敼鏃堕棿鍒?-->
      <el-table-column label="淇敼鏃堕棿" prop="date" />

      <!-- 鎿嶄綔鍒?-->
      <el-table-column align="right">
        <template #header>
          <el-input v-model="search" size="small" placeholder="鎼滅储鍚嶇О" />
        </template>
        <template #default="scope">
          <el-button size="small" @click="handlereload(scope.$index, scope.row)">
            鎺ㄦ祦
          </el-button>
          <el-button size="small" type="danger" @click="handleDelete(scope.$index, scope.row)">
            鍒犻櫎
          </el-button>
        </template>
      </el-table-column>

      <!-- 绌烘暟鎹彁绀?-->
      <template #empty>
        <span>鏆傛棤鏁版嵁</span>
      </template>
    </el-table>

    <MediaPreviewDialog
        v-model="isVideoDialogVisible"
        :url="selectedVideoUrl"
        :name="selectedVideoName"
    />

    <!-- 鎺ㄦ祦鍦板潃瀵硅瘽妗?-->
    <el-dialog v-model="isConvertDialogVisible" title="杈撳叆鎺ㄦ祦鍦板潃">
      <el-form @submit.prevent="submitConversion">
        <el-form-item label="鎺ㄦ祦鍦板潃">
          <el-input v-model="steamurl" placeholder="渚嬪锛歳tmp://live.example.com/stream" />
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
        <el-button type="primary" native-type="submit">鎻愪氦</el-button>
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
  streamId?: string
  targetFormat: string
  archiveEnabled?: boolean
  segmentSeconds?: number
  relayTargets?: string[]
  cover?: string
}

// 鏁版嵁瀹氫箟
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
const isPreviewable = (name: string) =>
  previewableExts.some((ext) => name.toLowerCase().endsWith(ext))

// 杩囨护鍚庣殑琛ㄦ牸鏁版嵁
const filterTableData = computed(() =>
    tableData.value.filter((data) =>
        !search.value || data.name.toLowerCase().includes(search.value.toLowerCase())
    )
)

// 鏍￠獙鏄惁鏄悎娉曠殑 rtmp 鎴?rtsp 鍦板潃
const isValidStreamUrl = (url: string): boolean => {
  const pattern = /^(rtmp|rtsp):\/\/.+/
  return pattern.test(url)
}

// 鑾峰彇鏁版嵁
const fetchData = async () => {
  try {
    const response = await getSteamFiles()
    if (response.data && response.data.code === 200) {
      tableData.value = response.data.data
    } else {
      console.error('鑾峰彇鎺ㄦ祦鏂囦欢鍒楄〃澶辫触锛屽搷搴旀暟鎹紓甯?', response)
      tableData.value = []
    }
  } catch (error) {
    if (error instanceof Error) {
      console.error('鑾峰彇鎺ㄦ祦鏂囦欢鍒楄〃澶辫触锛岄敊璇鎯?', error.message, error.stack)
    } else {
      console.error('鑾峰彇鎺ㄦ祦鏂囦欢鍒楄〃澶辫触锛屾湭鐭ラ敊璇?', error)
    }
    tableData.value = []
  }
}

onMounted(async () => {
  await fetchData()
})

// 鑷畾涔変笂浼犳柟娉?
const customUpload = async (options: UploadRequestOptions) => {
  const formData = new FormData()
  const { file } = options
  formData.append('file', file)

  uploadProgress.value = 0 // 閲嶇疆杩涘害鏉?

  const response = await uploadFileSteame(formData, (percent: number) => {
    uploadProgress.value = percent
  })

  if (response.data.code === 200) {
    await fetchData()
    ElMessage.success('涓婁紶鎴愬姛')
  } else {
    ElMessage.error('涓婁紶澶辫触')
  }
}

// 鎾斁鍏ㄥ睆瑙嗛
const playFullScreenVideo = (url: string, name?: string) => {
  selectedVideoUrl.value = url
  selectedVideoName.value = name || ''
  isVideoDialogVisible.value = true
}

// 鍒犻櫎鎿嶄綔
const handleDelete = async (index: number, row: VideoInfo) => {
  const res = await deletesteamVideo(row)

  if (res.data.code === 200) {
    tableData.value = tableData.value.filter((item) => item.name !== row.name)
    ElMessage.success('鍒犻櫎鎴愬姛')
  } else {
    ElMessage.error('鍒犻櫎澶辫触锛? + (res.data.message || '鏈煡閿欒'))
  }
}

// 鎺ㄦ祦鎿嶄綔
const handlereload = (index: number, row: VideoInfo) => {
  selectedVideoForConvert.value = row
  isConvertDialogVisible.value = true
  archiveEnabled.value = false
  segmentSeconds.value = 300
  relayTargetsText.value = ''
}

const submitConversion = async () => {
  if (!selectedVideoForConvert.value) {
    ElMessage.warning('璇烽€夋嫨涓€涓棰戞枃浠?)
    return
  }

  if (!steamurl.value || !isValidStreamUrl(steamurl.value)) {
    ElMessage.warning('璇疯緭鍏ユ湁鏁堢殑 RTMP 鎴?RTSP 鍦板潃锛堝锛歳tmp://xxx 鎴?rtsp://xxx锛?)
    return
  }

  try {
    const relayTargets = relayTargetsText.value
        .split('\n')
        .map((item) => item.trim())
        .filter((item) => item.length > 0)

    const videoInfo = {
      ...selectedVideoForConvert.value,
      steamurl: steamurl.value,
      archiveEnabled: archiveEnabled.value,
      segmentSeconds: segmentSeconds.value,
      relayTargets
    }

    const res = await steamload(videoInfo)

    if (res.data.code === 200) {
      ElMessage.success('鎺ㄦ祦浠诲姟宸叉彁浜?)
      isConvertDialogVisible.value = false
      await fetchData() // 鍒锋柊琛ㄦ牸鏁版嵁
    } else {
      ElMessage.error('鎺ㄦ祦浠诲姟鎻愪氦澶辫触锛岃閲嶈瘯銆?)
    }
  } catch (error) {
    console.error('鎺ㄦ祦閿欒:', error)
    ElMessage.error('鎻愪氦鎺ㄦ祦浠诲姟鏃跺彂鐢熼敊璇?)
  }
}

// 涓婁紶鍓嶆鏌?
const beforeUpload = (file: File): boolean => {
  const isValidType = ['video/mp4'].includes(file.type)
  if (!isValidType) {
    ElMessage.error('鍙兘涓婁紶 MP4 瑙嗛!')
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

