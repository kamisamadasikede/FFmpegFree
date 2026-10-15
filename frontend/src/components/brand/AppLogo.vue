<template>
  <img class="app-logo" :src="icon" :width="size" :height="size" :alt="decorative ? '' : APP_NAME" :aria-hidden="decorative || undefined" draggable="false" />
</template>

<script setup lang="ts">
import icon from '@/assets/brand/ffc-icon.svg'
import { APP_NAME } from '@/config/brand'

withDefaults(defineProps<{ size?: number; decorative?: boolean }>(), { size: 32, decorative: false })
</script>

<style scoped>
.app-logo { display: block; flex: none; object-fit: contain; user-select: none; }
</style>
