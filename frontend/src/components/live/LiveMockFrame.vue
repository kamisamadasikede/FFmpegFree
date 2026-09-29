<template>
  <div v-if="variant === 'screen'" class="mock screen">
    <div class="win">
      <div class="side"></div>
      <div class="main">
        <div class="t"></div>
        <div class="cards"><i></i><i></i><i class="hl"></i></div>
        <div class="big"></div>
      </div>
    </div>
    <div class="pip"></div>
  </div>
  <div v-else-if="variant === 'scene'" class="mock scene">
    <div class="sun"></div><div class="m1"></div><div class="m2"></div>
  </div>
  <div v-else class="mock idle"><FIcon :name="icon" :size="28" /><span>{{ hint }}</span></div>
</template>

<script setup lang="ts">
// 画面占位：预览模拟时用（screen = 原型录屏推流的示意画面，scene = 编辑页那张夕阳），idle = 未开始时的提示。
import FIcon from '../icon/FIcon.vue'
import type { IconName } from '../icon/icons'

withDefaults(defineProps<{ variant: 'screen' | 'scene' | 'idle'; hint?: string; icon?: IconName }>(), { icon: 'monitor' })
</script>

<style scoped>
.mock {
  position: absolute;
  inset: 0;
}
.screen {
  background: linear-gradient(135deg, #0f172a, #1e293b);
}
.win {
  position: absolute;
  inset: 6% 5%;
  border-radius: 8px;
  background: #f5f6f8;
  display: flex;
  overflow: hidden;
}
.side {
  width: 18%;
  background: #eef0f3;
}
.main {
  flex: 1;
  padding: 4%;
  display: flex;
  flex-direction: column;
  gap: 6%;
}
.t {
  height: 8%;
  width: 40%;
  background: #d5d9e0;
  border-radius: 4px;
}
.cards {
  flex: 1;
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 4%;
}
.cards i {
  background: #fff;
  border-radius: 6px;
  border: 1px solid #e3e6eb;
}
.cards i.hl {
  background: #eaf0ff;
  border: 0;
}
.big {
  flex: 1;
  background: #fff;
  border-radius: 6px;
  border: 1px solid #e3e6eb;
}
.pip {
  position: absolute;
  right: 4%;
  bottom: 8%;
  width: 20%;
  aspect-ratio: 1;
  border-radius: 50%;
  background: linear-gradient(135deg, #f59e0b, #ec4899);
  border: 3px solid #fff;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}
.scene {
  background: linear-gradient(180deg, #1e3a8a 0%, #7c3aed 38%, #f97316 72%, #fdba74 100%);
}
.sun {
  position: absolute;
  left: 58%;
  top: 46%;
  width: 22%;
  aspect-ratio: 1;
  border-radius: 50%;
  background: radial-gradient(circle, #fff7d6 0%, #fde68a 45%, rgba(253, 230, 138, 0) 70%);
}
.m1 {
  position: absolute;
  left: -5%;
  right: -5%;
  bottom: 0;
  height: 46%;
  background: #1e1b4b;
  clip-path: polygon(0 60%, 14% 30%, 26% 52%, 40% 18%, 55% 50%, 68% 28%, 82% 55%, 100% 35%, 100% 100%, 0 100%);
}
.m2 {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 26%;
  background: #0f0e2a;
  clip-path: polygon(0 50%, 20% 20%, 36% 55%, 52% 30%, 70% 60%, 88% 25%, 100% 45%, 100% 100%, 0 100%);
}
.idle {
  background: #0b0c0e;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: #7c828c;
  font-size: var(--ff-fs-xs);
}
</style>
