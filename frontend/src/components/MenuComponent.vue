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
        <el-icon><VideoCamera /></el-icon>
        <span>pdf</span>
      </template>
      <el-menu-item route="/pdf-preview" index="/pdf-preview">pdf预览</el-menu-item>
   
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
