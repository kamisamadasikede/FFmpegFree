<template>
  <svg class="f-icon" viewBox="0 0 24 24" :style="{ width: size + 'px', height: size + 'px', strokeWidth: stroke }" aria-hidden="true" v-html="iconPaths[name]" />
</template>

<script setup lang="ts">
import { iconPaths, type IconName } from './icons'

withDefaults(defineProps<{ name: IconName; size?: number; stroke?: number }>(), { size: 18, stroke: 1.8 })
</script>

<style scoped>
.f-icon {
  flex: none;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-linejoin: round;
}
</style>
