<template>
  <div class="menu-shell">
    <div class="menu-brand">
      <div class="brand-mark">FF</div>
      <div>
        <div class="brand-title">FFmpegFree</div>
        <div class="brand-subtitle">多媒体桌面工具</div>
      </div>
    </div>
    <el-menu
      :default-active="$route.path"
      class="app-menu"
      @open="handleOpen"
      @close="handleClose"
      router
    >
    <el-sub-menu index="1">
      <template #title>
        <el-icon><Camera /></el-icon>
        <span>视频</span>
      </template>
      <el-menu-item route="/" index="/">转换</el-menu-item>
      <el-menu-item route="/convert" index="/convert">正在转换中</el-menu-item>
      <el-menu-item route="/convertup" index="/convertup">已完成</el-menu-item>
    </el-sub-menu>
    <el-sub-menu index="2">
      <template #title>
        <el-icon><VideoCamera /></el-icon>
        <span>流媒体工具</span>
      </template>
      <el-menu-item route="/steamup" index="/steamup">推流</el-menu-item>
      <el-menu-item route="/steamlist" index="/steamlist">正在推流中</el-menu-item>
      <el-menu-item route="/MediaRecorder" index="/MediaRecorder">屏幕录制推流</el-menu-item>
      <el-menu-item route="/LivePlayer" index="/LivePlayer">直播拉流(只支持flv拉流)</el-menu-item>
    </el-sub-menu>
        <el-sub-menu index="3">
      <template #title>
        <el-icon><MessageBox /></el-icon>
        <span>文档转换</span>
      </template>
      <el-menu-item route="/pdf-preview" index="/pdf-preview">PDF预览</el-menu-item>
      <el-menu-item route="/office-convert" index="/office-convert">Office转PDF</el-menu-item>
    </el-sub-menu>
    <el-sub-menu index="4">
      <template #title>
        <el-icon><Tools /></el-icon>
        <span>工具</span>
      </template>
      <el-menu-item route="/json-tools" index="/json-tools">JSON工具</el-menu-item>
    </el-sub-menu>
    <el-menu-item route="/about" index="/about">
      <el-icon><InfoFilled /></el-icon>
      <span>关于</span>
    </el-menu-item>
    </el-menu>
  </div>
</template>

<script lang="ts" setup>
const handleOpen = (key: string, keyPath: string[]) => {
  console.log(key, keyPath)
}
const handleClose = (key: string, keyPath: string[]) => {
  console.log(key, keyPath)
}
</script>

<style scoped>
.menu-shell {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
}

.menu-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 12px;
  border-radius: 16px;
  background: rgba(37, 99, 235, 0.08);
  border: 1px solid rgba(37, 99, 235, 0.18);
}

.brand-mark {
  width: 44px;
  height: 44px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #2563eb;
  color: #fff;
  font-weight: 700;
  letter-spacing: 0.5px;
}

.brand-title {
  font-weight: 700;
  font-size: 16px;
}

.brand-subtitle {
  color: #6b7280;
  font-size: 12px;
  margin-top: 2px;
}
</style>
