<template>
  <el-menu
      :default-active="$route.path"
      class="el-menu-vertical-demo"
      @open="handleOpen"
      @close="handleClose"
      router
      style="height: 100vh; width: 200px"
  >
    <el-sub-menu index="1">
      <template #title>
        <el-icon><location /></el-icon>
        <span>视频</span>
      </template>
      <el-menu-item route="/" index="/">转换</el-menu-item>
      <el-menu-item route="/convert" index="/convert">正在转换中</el-menu-item>
      <el-menu-item route="/convertup" index="/convertup">已完成</el-menu-item>
    </el-sub-menu>
  </el-menu>
</template>

<script lang="ts" setup>
import { Location } from '@element-plus/icons-vue'
const handleOpen = (key: string, keyPath: string[]) => {
  console.log(key, keyPath)
}
const handleClose = (key: string, keyPath: string[]) => {
  console.log(key, keyPath)
}
</script>
