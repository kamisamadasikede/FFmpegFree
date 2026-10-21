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
  min-height: 0;
}

/* v0.58 自适应：两栏（预览 + 320 设置面板）在窄窗口改单列。
   原来 ≤900 时预览区只剩 ~330 宽，16:9 的播放器被压成窄条、设置面板挤成一列标签。
   单列后预览在上、设置面板整宽落到下面，页面纵向滚动。
   注意：预览（.pvp）和播放器（.lp-stage）都是 flex:1 + min-height:0，
   列容器不能也用 flex:none，否则可用高度塌成 0、预览只剩一个标题条。
   这里给预览一个视口比例的高度，让 16:9 播放器有真实的尺寸可算。 */
@media (max-width: 1024px) {
  .hrow {
    flex-direction: column;
    gap: 12px;
    overflow-y: auto;
    overscroll-behavior: contain;
  }
  .left {
    flex: none;
    height: clamp(220px, 42vh, 420px);
    min-height: 0;
  }
  .left > :deep(.pvp) {
    min-height: 0;
  }
  /* 指标卡两列，避免四张卡在 720 宽下每个只剩 70px */
  .left > :deep(.statrow) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .left {
    gap: 10px;
    height: clamp(200px, 38vh, 340px);
  }
}
</style>
