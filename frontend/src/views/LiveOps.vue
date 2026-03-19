<template>
  <div class="live-ops-page">
    <section class="panel">
      <div class="header-row">
        <h2>直播健康诊断面板</h2>
        <el-button @click="refreshAll">刷新</el-button>
      </div>
      <div class="summary-grid">
        <el-card shadow="never"><div class="metric"><span>总会话</span><strong>{{ summary.total }}</strong></div></el-card>
        <el-card shadow="never"><div class="metric"><span>活跃</span><strong>{{ summary.active }}</strong></div></el-card>
        <el-card shadow="never"><div class="metric"><span>预警</span><strong>{{ summary.warning }}</strong></div></el-card>
        <el-card shadow="never"><div class="metric"><span>严重</span><strong>{{ summary.critical }}</strong></div></el-card>
      </div>

      <el-table :data="healthItems" style="width: 100%; margin-top: 12px" height="360">
        <el-table-column label="会话ID" prop="streamId" min-width="180" />
        <el-table-column label="名称" prop="displayName" min-width="120" />
        <el-table-column label="来源" prop="source" width="90" />
        <el-table-column label="状态" prop="status" width="90" />
        <el-table-column label="健康" width="90">
          <template #default="scope">
            <el-tag :type="healthTagType(scope.row.health)">{{ scope.row.health }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="FPS" width="80">
          <template #default="scope">{{ scope.row.fps?.toFixed?.(1) ?? 0 }}</template>
        </el-table-column>
        <el-table-column label="码率(kbps)" width="110">
          <template #default="scope">{{ scope.row.bitrateKbps?.toFixed?.(1) ?? 0 }}</template>
        </el-table-column>
        <el-table-column label="入口码率(kbps)" width="130">
          <template #default="scope">{{ scope.row.ingressBitrateKbps?.toFixed?.(1) ?? 0 }}</template>
        </el-table-column>
        <el-table-column label="延迟(ms)" width="100" prop="estimatedLatencyMs" />
        <el-table-column label="掉帧" width="80" prop="dropFrames" />
        <el-table-column label="诊断" prop="diagnosis" min-width="200" show-overflow-tooltip />
      </el-table>
    </section>

    <section class="panel">
      <div class="header-row">
        <h2>一键转推工具</h2>
      </div>
      <el-form label-position="top">
        <el-form-item label="任务名称">
          <el-input v-model="relayForm.displayName" placeholder="例如：主直播转推" />
        </el-form-item>
        <el-form-item label="源地址（RTMP/RTSP/HTTP-FLV）">
          <el-input v-model="relayForm.sourceUrl" placeholder="rtmp://source.example.com/live/room1" />
        </el-form-item>
        <el-form-item label="目标地址（每行一个）">
          <el-input
            v-model="relayForm.targetsText"
            type="textarea"
            :rows="4"
            placeholder="rtmp://target1/live/room1&#10;rtmp://target2/live/room1"
          />
        </el-form-item>
        <div class="inline-row">
          <el-form-item label="自动录制" style="margin-right: 16px;">
            <el-switch v-model="relayForm.archiveEnabled" />
          </el-form-item>
          <el-form-item label="分段秒数">
            <el-input-number v-model="relayForm.segmentSeconds" :min="30" :max="3600" :step="30" />
          </el-form-item>
        </div>
        <el-button type="primary" @click="submitRelay">一键启动转推</el-button>
      </el-form>

      <el-table :data="relayItems" style="width: 100%; margin-top: 12px" max-height="280">
        <el-table-column label="会话ID" prop="streamId" min-width="180" />
        <el-table-column label="名称" prop="displayName" min-width="120" />
        <el-table-column label="健康" width="90">
          <template #default="scope">
            <el-tag :type="healthTagType(scope.row.health)">{{ scope.row.health }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="延迟(ms)" prop="latencyMs" width="100" />
        <el-table-column label="掉帧" prop="dropFrames" width="90" />
        <el-table-column label="操作" width="120">
          <template #default="scope">
            <el-button type="danger" size="small" @click="stopRelayTask(scope.row.streamId)">停止</el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="panel">
      <div class="header-row">
        <h2>自动录制与分段归档</h2>
      </div>
      <el-table :data="archives" style="width: 100%" max-height="340">
        <el-table-column label="会话ID" prop="streamId" min-width="180" />
        <el-table-column label="文件名" prop="fileName" min-width="220" />
        <el-table-column label="大小(MB)" width="110">
          <template #default="scope">{{ (scope.row.sizeBytes / 1024 / 1024).toFixed(2) }}</template>
        </el-table-column>
        <el-table-column label="更新时间" prop="updatedAt" width="180" />
        <el-table-column label="下载" width="120">
          <template #default="scope">
            <el-link :href="scope.row.fileUrl" target="_blank" type="primary">下载</el-link>
          </template>
        </el-table-column>
      </el-table>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getLiveArchives,
  getLiveHealth,
  listRelay,
  startRelay,
  stopRelay,
  type LiveArchiveItem,
  type LiveHealthItem,
  type RelayTaskItem,
} from '@/api/live/live'

const healthItems = ref<LiveHealthItem[]>([])
const relayItems = ref<RelayTaskItem[]>([])
const archives = ref<LiveArchiveItem[]>([])

const summary = reactive({
  total: 0,
  active: 0,
  warning: 0,
  critical: 0,
})

const relayForm = reactive({
  displayName: '',
  sourceUrl: '',
  targetsText: '',
  archiveEnabled: false,
  segmentSeconds: 300,
})

let timer: ReturnType<typeof setInterval> | null = null

const healthTagType = (health: string) => {
  if (health === 'critical') return 'danger'
  if (health === 'warning') return 'warning'
  return 'success'
}

const refreshHealth = async () => {
  const response = await getLiveHealth()
  if (response.data.code !== 200) {
    return
  }
  healthItems.value = response.data.data.items || []
  const incomingSummary = response.data.data.summary || {}
  summary.total = incomingSummary.total || 0
  summary.active = incomingSummary.active || 0
  summary.warning = incomingSummary.warning || 0
  summary.critical = incomingSummary.critical || 0
}

const refreshArchives = async () => {
  const response = await getLiveArchives()
  if (response.data.code !== 200) {
    return
  }
  archives.value = response.data.data.items || []
}

const refreshRelay = async () => {
  const response = await listRelay()
  if (response.data.code !== 200) {
    return
  }
  relayItems.value = response.data.data.items || []
}

const refreshAll = async () => {
  try {
    // 并发刷新三块数据，保证面板一致性且避免串行等待。
    await Promise.all([refreshHealth(), refreshRelay(), refreshArchives()])
  } catch (error) {
    console.error('refresh live ops failed', error)
  }
}

const submitRelay = async () => {
  // 文本域按行拆分转推目标，自动去空行。
  const targets = relayForm.targetsText
    .split('\n')
    .map((item) => item.trim())
    .filter((item) => item.length > 0)

  if (!relayForm.sourceUrl.trim() || targets.length === 0) {
    ElMessage.warning('请填写源地址和至少一个目标地址')
    return
  }

  const response = await startRelay({
    displayName: relayForm.displayName.trim(),
    sourceUrl: relayForm.sourceUrl.trim(),
    targets,
    archiveEnabled: relayForm.archiveEnabled,
    segmentSeconds: relayForm.segmentSeconds,
  })

  if (response.data.code === 200) {
    ElMessage.success('转推任务已启动')
    await refreshAll()
  } else {
    ElMessage.error(response.data.message || '转推任务启动失败')
  }
}

const stopRelayTask = async (streamId: string) => {
  const response = await stopRelay(streamId)
  if (response.data.code === 200) {
    ElMessage.success('转推任务已停止')
    await refreshAll()
  } else {
    ElMessage.error(response.data.message || '停止失败')
  }
}

onMounted(() => {
  refreshAll()
  timer = setInterval(refreshAll, 4000)
})

onUnmounted(() => {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
})
</script>

<style scoped>
.live-ops-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.header-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.header-row h2 {
  margin: 0;
  font-size: 16px;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
}

.metric {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.metric span {
  color: var(--text-muted);
}

.metric strong {
  font-size: 20px;
}

.inline-row {
  display: flex;
  align-items: center;
}

@media (max-width: 900px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .inline-row {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>

