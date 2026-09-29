<template>
  <div class="adv">
    <button type="button" class="toggle" :aria-expanded="open" @click="open = !open">
      高级选项<FIcon name="down" :size="14" :class="{ up: open }" />
    </button>
    <div v-if="open" class="body">
      <div class="chk">自动录制分段<el-switch v-model="archiveEnabled" size="small" aria-label="自动录制分段" /></div>
      <LiveField v-if="archiveEnabled" label="分段秒数" :control="false">
        <el-input-number v-model="segmentSeconds" aria-label="分段秒数" :min="30" :max="3600" :step="30" controls-position="right" />
      </LiveField>
      <LiveField v-slot="{ id }" label="额外转推目标（每行一个）">
        <el-input :id="id" v-model="relayText" type="textarea" :rows="3" class="mono" placeholder="rtmp://backup.example.com/live/stream1" />
      </LiveField>
    </div>
  </div>
</template>

<script setup lang="ts">
// 保留 v1 的自动录制分段和额外转推目标；原型没有这几项，默认折叠，展开后才占位。
import { ref } from 'vue'
import FIcon from '../icon/FIcon.vue'
import LiveField from './LiveField.vue'

const archiveEnabled = defineModel<boolean>('archiveEnabled', { default: false })
const segmentSeconds = defineModel<number>('segmentSeconds', { default: 300 })
const relayText = defineModel<string>('relayText', { default: '' })
const open = ref(false)
</script>

<style scoped>
.toggle {
  border: 0;
  background: none;
  padding: 0;
  font: inherit;
  font-size: 12px;
  color: var(--ff-text-2);
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
}
.toggle:hover {
  color: var(--ff-text-1);
}
.toggle .up {
  transform: rotate(180deg);
}
.body {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.chk {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
}
.mono :deep(textarea) {
  font-family: var(--ff-font-mono);
  font-size: 12px;
}
</style>
