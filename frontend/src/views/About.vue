<template>
  <div class="about-layout">
    <section class="panel hero-card">
      <div class="hero-main">
        <el-tag class="hero-badge" effect="plain">Desktop Toolkit</el-tag>
        <h1>FFmpegFree</h1>
        <p>
          面向直播与音视频生产的桌面工具集，提供直播诊断、自动录制归档、一键转推、多轨剪辑与导出等核心能力，
          聚焦稳定性、实时性和工程效率。
        </p>
        <div class="hero-actions">
          <el-button type="primary" @click="openGitHub">GitHub 仓库</el-button>
          <el-button text @click="scrollToTech">查看技术栈</el-button>
        </div>
      </div>

      <div class="hero-facts">
        <div v-for="fact in quickFacts" :key="fact.label" class="fact-card">
          <span class="fact-label">{{ fact.label }}</span>
          <span class="fact-value">{{ fact.value }}</span>
        </div>
      </div>
    </section>

    <section class="metrics-grid">
      <article v-for="item in metrics" :key="item.label" class="panel metric-card">
        <el-icon><component :is="item.icon" /></el-icon>
        <div class="metric-body">
          <div class="metric-value">{{ item.value }}</div>
          <div class="metric-label">{{ item.label }}</div>
        </div>
      </article>
    </section>

    <section class="content-grid">
      <article class="panel info-card">
        <div class="card-title">
          <el-icon><Grid /></el-icon>
          <span>核心能力</span>
        </div>
        <div class="feature-grid">
          <div v-for="item in features" :key="item.title" class="feature-card">
            <el-icon class="feature-icon"><component :is="item.icon" /></el-icon>
            <div class="feature-body">
              <h4>{{ item.title }}</h4>
              <p>{{ item.desc }}</p>
            </div>
          </div>
        </div>
      </article>

      <article id="tech-section" class="panel info-card">
        <div class="card-title">
          <el-icon><Cpu /></el-icon>
          <span>技术栈</span>
        </div>
        <div class="stack-grid">
          <div v-for="stack in stacks" :key="stack.title" class="stack-card">
            <h4>{{ stack.title }}</h4>
            <p v-for="line in stack.lines" :key="line">{{ line }}</p>
          </div>
        </div>
      </article>
    </section>

    <section class="panel info-card">
      <div class="card-title">
        <el-icon><Files /></el-icon>
        <span>支持格式</span>
      </div>
      <div class="format-wrap">
        <el-tag v-for="fmt in formats" :key="fmt" class="fmt">{{ fmt }}</el-tag>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import {
  Cpu,
  Files,
  Grid,
  Monitor,
  Document,
  Promotion,
  VideoCamera,
  VideoPlay
} from '@element-plus/icons-vue'

const quickFacts = [
  { label: '版本', value: 'v1.0.0' },
  { label: '核心引擎', value: 'FFmpeg' },
  { label: '后端能力', value: 'Go + Gin' },
  { label: '前端框架', value: 'Vue3 + TS' }
]

const metrics = [
  { value: '7+', label: '视频格式', icon: VideoCamera },
  { value: '4', label: '核心能力', icon: Grid },
  { value: '2', label: '文档能力', icon: Document },
  { value: '实时', label: '状态同步', icon: Promotion }
]

const features = [
  {
    title: '直播健康诊断面板',
    desc: '实时监看 FPS、延迟、丢帧和状态波动，快速定位卡顿与异常。',
    icon: Monitor
  },
  {
    title: '自动录制与分段归档',
    desc: '直播自动落盘并按策略切片归档，便于回放、检索与素材沉淀。',
    icon: VideoCamera
  },
  {
    title: '一键转推工具',
    desc: '支持多平台分发推流，减少重复操作并提升内容覆盖效率。',
    icon: Promotion
  },
  {
    title: '多轨剪辑与导出',
    desc: '支持视频轨/音轨时间线编辑、监看器预览、转场特效与 FFmpeg 导出。',
    icon: VideoPlay
  }
]
const stacks = [
  { title: '前端', lines: ['Vue3 + TypeScript', 'Element Plus', 'Monaco Editor'] },
  { title: '后端', lines: ['Go + Gin', 'REST + SSE', '任务编排与控制'] },
  { title: '媒体链路', lines: ['FFmpeg 转码与合成', '多轨时间线渲染', '导出与归档输出'] }
]

const formats = ['.mp4', '.avi', '.mkv', '.mov', '.flv', '.gif', '.webm']

const openGitHub = () => {
  window.open('https://github.com/bmcbdt/FFmpegFree', '_blank')
}

const scrollToTech = () => {
  document.getElementById('tech-section')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
</script>

<style scoped>
.about-layout {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.hero-card {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(280px, 1fr);
  gap: 14px;
  background:
    linear-gradient(140deg, rgba(37, 99, 235, 0.12), rgba(37, 99, 235, 0) 72%),
    var(--surface);
}

.hero-main {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.hero-badge {
  width: fit-content;
}

.hero-main h1 {
  margin: 0;
  font-size: 30px;
  line-height: 1.1;
  letter-spacing: 0.3px;
}

.hero-main p {
  margin: 0;
  color: var(--text-muted);
  line-height: 1.7;
  font-size: 14px;
  max-width: 620px;
}

.hero-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.hero-facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.fact-card {
  background: rgba(255, 255, 255, 0.8);
  border: 1px solid var(--border-soft);
  border-radius: 12px;
  padding: 10px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.fact-label {
  font-size: 12px;
  color: var(--text-soft);
}

.fact-value {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary);
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.metric-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px;
}

.metric-card .el-icon {
  font-size: 18px;
  color: var(--el-color-primary);
  background: rgba(37, 99, 235, 0.12);
  padding: 8px;
  border-radius: 10px;
}

.metric-body {
  min-width: 0;
}

.metric-value {
  font-size: 17px;
  font-weight: 700;
}

.metric-label {
  font-size: 12px;
  color: var(--text-muted);
}

.content-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(0, 0.95fr);
  gap: 12px;
}

.info-card {
  padding: 14px;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
  font-size: 15px;
  font-weight: 700;
}

.feature-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.feature-card {
  background: var(--surface-muted);
  border: 1px solid var(--border-soft);
  border-radius: 12px;
  padding: 10px;
  display: flex;
  gap: 8px;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.feature-card:hover {
  transform: translateY(-1px);
  box-shadow: var(--shadow-2);
}

.feature-icon {
  color: var(--el-color-primary);
  margin-top: 1px;
}

.feature-body h4 {
  margin: 0;
  font-size: 14px;
}

.feature-body p {
  margin: 5px 0 0;
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1.55;
}

.stack-grid {
  display: grid;
  gap: 10px;
}

.stack-card {
  background: var(--surface-muted);
  border: 1px solid var(--border-soft);
  border-radius: 12px;
  padding: 10px;
}

.stack-card h4 {
  margin: 0 0 6px;
  font-size: 14px;
}

.stack-card p {
  margin: 0 0 4px;
  font-size: 12px;
  color: var(--text-muted);
}

.format-wrap {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.fmt {
  margin: 0;
}

@media (max-width: 980px) {
  .hero-card,
  .content-grid {
    grid-template-columns: 1fr;
  }

  .metrics-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 680px) {
  .hero-main h1 {
    font-size: 26px;
  }

  .hero-facts,
  .metrics-grid,
  .feature-grid {
    grid-template-columns: 1fr;
  }
}
</style>


