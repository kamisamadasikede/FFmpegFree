<template>
  <div class="hrow">
    <div class="left"><slot name="main" /></div>
    <slot name="panel" />
  </div>
</template>

<script setup lang="ts">
// 页签内容的两栏骨架：左列（播放器 + 指标卡）撑满，右列 320 设置面板。原型的 .hrow。
</script>

<style scoped>
.hrow {
  display: flex;
  gap: 16px;
  flex: 1;
  min-height: 0;
}
.left {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}
</style>
