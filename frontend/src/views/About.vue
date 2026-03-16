<template>
  <div class="about-container">
    <el-card class="about-card">
      <template #header>
        <div class="card-header">
          <h1>🎥 FFmpegFree</h1>
          <el-tag type="success" effect="dark">版本 1.0.0</el-tag>
        </div>
      </template>

      <div class="intro-section">
        <p class="intro-text">
          一个基于 Vue3 + TypeScript + Go 的跨平台桌面端音视频格式转换工具
        </p>
      </div>

      <el-divider content-position="left">⚙️ 技术栈</el-divider>

      <div class="tech-stack">
        <el-row :gutter="20">
          <el-col :span="8">
            <div class="tech-item">
              <el-icon :size="32" color="#409EFF"><Monitor /></el-icon>
              <h3>前端</h3>
              <p>Vue3 + TypeScript</p>
              <p>Vite + Element Plus</p>
              <p>Axios</p>
            </div>
          </el-col>
          <el-col :span="8">
            <div class="tech-item">
              <el-icon :size="32" color="#67C23A"><Cpu /></el-icon>
              <h3>后端</h3>
              <p>Go + Gin</p>
              <p>Wails 桌面端</p>
              <p>FFmpeg</p>
            </div>
          </el-col>
          <el-col :span="8">
            <div class="tech-item">
              <el-icon :size="32" color="#E6A23C"><Connection /></el-icon>
              <h3>通信</h3>
              <p>HTTP + JSON</p>
              <p>SSE</p>
              <p>WebSocket</p>
            </div>
          </el-col>
        </el-row>
      </div>

      <el-divider content-position="left">📦 支持的格式</el-divider>

      <div class="formats-section">
        <el-tag type="info" class="format-tag">.mp4</el-tag>
        <el-tag type="info" class="format-tag">.avi</el-tag>
        <el-tag type="info" class="format-tag">.mkv</el-tag>
        <el-tag type="info" class="format-tag">.mov</el-tag>
        <el-tag type="info" class="format-tag">.flv</el-tag>
        <el-tag type="info" class="format-tag">.gif</el-tag>
        <el-tag type="info" class="format-tag">.webm</el-tag>
      </div>

      <el-divider content-position="left">🚀 功能特性</el-divider>

      <div class="features-section">
        <el-row :gutter="16">
          <el-col :span="12" v-for="feature in features" :key="feature.title">
            <div class="feature-item">
              <el-icon :size="24" :color="feature.color">
                <component :is="feature.icon" />
              </el-icon>
              <div class="feature-content">
                <h4>{{ feature.title }}</h4>
                <p>{{ feature.desc }}</p>
              </div>
            </div>
          </el-col>
        </el-row>
      </div>

      <el-divider content-position="left">📡 流媒体工具</el-divider>

      <div class="stream-section">
        <el-alert
          title="支持多种流媒体协议"
          type="info"
          :closable="false"
        />
        <el-row :gutter="20" class="stream-features">
          <el-col :span="8">
            <div class="stream-item">
              <h4>📤 文件推流</h4>
              <p>支持 RTMP 推流</p>
            </div>
          </el-col>
          <el-col :span="8">
            <div class="stream-item">
              <h4>🖥️ 屏幕录制</h4>
              <p>实时推流录制</p>
            </div>
          </el-col>
          <el-col :span="8">
            <div class="stream-item">
              <h4>📺 FLV 拉流</h4>
              <p>直播流播放</p>
            </div>
          </el-col>
        </el-row>
      </div>

      <el-divider content-position="left">🤝 贡献指南</el-divider>

      <div class="contribute-section">
        <p>欢迎提交 Issue 和 Pull Request！如果你有兴趣添加更多格式的支持，请 Fork 本仓库并提交你的修改。</p>
        <el-button type="primary" @click="openGitHub">访问 GitHub</el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import {
  Monitor,
  Cpu,
  Connection,
  VideoCamera,
  Upload,
  VideoPlay,
  Document,
  Promotion
} from '@element-plus/icons-vue'

const features = [
  {
    title: '视频格式转换',
    desc: '支持多种主流视频格式互转',
    icon: VideoCamera,
    color: '#409EFF'
  },
  {
    title: '文件推流上传',
    desc: '支持音视频文件推流到指定地址',
    icon: Upload,
    color: '#67C23A'
  },
  {
    title: '屏幕录制推流',
    desc: '支持屏幕区域录制并实时推流',
    icon: Monitor,
    color: '#E6A23C'
  },
  {
    title: '直播拉流播放',
    desc: '支持 FLV 格式直播流播放',
    icon: VideoPlay,
    color: '#F56C6C'
  },
  {
    title: 'PDF 预览',
    desc: '支持 PDF 文件在线预览',
    icon: Document,
    color: '#909399'
  },
  {
    title: 'SSE/WebSocket',
    desc: '实时事件推送，状态即时更新',
    icon: Promotion,
    color: '#C0C4CC'
  }
]

const openGitHub = () => {
  window.open('https://github.com/bmcbdt/FFmpegFree', '_blank')
}
</script>

<style scoped>
.about-container {
  padding: 20px;
  max-width: 1200px;
  margin: 0 auto;
}

.about-card {
  border-radius: 10px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-header h1 {
  margin: 0;
  color: #409EFF;
}

.intro-section {
  text-align: center;
  padding: 20px 0;
}

.intro-text {
  font-size: 18px;
  color: #606266;
  line-height: 1.6;
}

.tech-stack {
  padding: 20px 0;
}

.tech-item {
  text-align: center;
  padding: 20px;
  background: #f5f7fa;
  border-radius: 8px;
  transition: all 0.3s;
}

.tech-item:hover {
  transform: translateY(-5px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.tech-item h3 {
  margin: 10px 0;
  color: #303133;
}

.tech-item p {
  margin: 5px 0;
  color: #909399;
  font-size: 14px;
}

.formats-section {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: center;
  padding: 20px 0;
}

.format-tag {
  font-size: 16px;
  padding: 8px 16px;
}

.features-section {
  padding: 20px 0;
}

.feature-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 16px;
  background: #f5f7fa;
  border-radius: 8px;
  margin-bottom: 16px;
}

.feature-content h4 {
  margin: 0 0 5px 0;
  color: #303133;
}

.feature-content p {
  margin: 0;
  color: #909399;
  font-size: 14px;
}

.stream-section {
  padding: 20px 0;
}

.stream-features {
  margin-top: 20px;
}

.stream-item {
  text-align: center;
  padding: 16px;
  background: #ecf5ff;
  border-radius: 8px;
  border: 1px solid #d9ecff;
}

.stream-item h4 {
  margin: 0 0 8px 0;
  color: #409EFF;
}

.stream-item p {
  margin: 0;
  color: #606266;
  font-size: 14px;
}

.contribute-section {
  text-align: center;
  padding: 20px 0;
}

.contribute-section p {
  color: #606266;
  margin-bottom: 20px;
}
</style>
