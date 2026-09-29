<template>
  <aside class="lpanel">
    <div class="phead"><h2>{{ title }}</h2></div>
    <div class="pbody"><slot /></div>
    <div class="foot"><small>{{ note }}</small><span class="sp" /><slot name="action" /></div>
  </aside>
</template>

<script setup lang="ts">
// 右侧 320px 设置面板：标题 + 表单 + 底部操作栏，样式来自 proto/pages.html 的 .panel / .phead / .pbody / .foot。
defineProps<{ title: string; note?: string }>()
</script>

<style scoped>
.lpanel {
  width: 320px;
  flex: none;
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: 10px;
}
.phead {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--ff-border);
  flex: none;
}
h2 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  line-height: 21px;
}
.pbody {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
.pbody :deep(.el-switch) {
  height: 16px;
  line-height: 16px;
}
.pbody :deep(.el-switch--small .el-switch__core) {
  min-width: 28px;
}
.pbody :deep(.el-select__wrapper) {
  min-height: 28px;
  height: 28px;
  padding: 0 10px;
  font-size: 13px;
  line-height: 26px;
}
.pbody :deep(.el-select__selected-item) {
  color: var(--ff-text-1);
}
.foot {
  padding: 12px 16px;
  border-top: 1px solid var(--ff-border);
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
}
.foot small {
  color: var(--ff-text-3);
  font-size: 12px;
}
.sp {
  flex: 1;
}
</style>
