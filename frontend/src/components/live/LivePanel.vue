<template>
  <aside class="lpanel" :class="{ form: !flat }">
    <div class="phead"><h2>{{ title }}</h2><slot name="head" /></div>
    <div class="pbody" :class="{ flat }"><slot /></div>
    <div v-if="note || $slots.action" class="foot"><small v-if="note">{{ note }}</small><span class="sp" /><slot name="action" /></div>
    <slot name="foot" />
  </aside>
</template>

<script setup lang="ts">
// 设计稿 v0.2 的面板：标题 14/600 + 表单区 + 底部操作栏（右对齐）。flat=true 是会话列表面板（弹性宽度、无内边距），否则是 320px 设置面板（1024 宽 304px）。
defineProps<{ title: string; note?: string; flat?: boolean }>()
</script>

<style scoped>
.lpanel {
  flex: none;
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: 10px;
}
.lpanel.form {
  width: 320px;
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
  font-size: var(--ff-fs-md);
  font-weight: 600;
  line-height: 21px;
}
.pbody {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
.pbody.flat {
  padding: 0;
  gap: 0;
  overflow: hidden;
}
.pbody :deep(.el-switch) {
  height: 16px;
  line-height: 16px;
}
.pbody :deep(.el-switch--small .el-switch__core) {
  min-width: 28px;
}
.foot {
  padding: 12px 16px;
  border-top: 1px solid var(--ff-border);
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex: none;
}
.foot small {
  color: var(--ff-text-2);
  font-size: var(--ff-fs-xs);
}
.sp {
  flex: 1;
}
/* 1024 宽：设置面板 320 → 304，表单内边距 16 → 12，字段间距 12 → 8 */
@media (max-width: 1199px) {
  .lpanel.form {
    width: 304px;
  }
  .pbody:not(.flat) {
    padding: 12px;
    gap: 8px;
  }
  .foot {
    padding: 12px;
  }
}
</style>
