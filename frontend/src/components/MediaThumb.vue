<template>
  <div class="media-thumb" :class="{ clickable }" @click="handleClick">
    <video
      v-if="useVideo"
      class="preview-video"
      :src="url"
      :poster="cover"
      preload="metadata"
      muted
      playsinline
    ></video>
    <img v-else-if="cover" class="preview-video" :src="cover" alt="preview" />
    <div v-else class="media-fallback">No Preview</div>
    <div v-if="!clickable" class="media-badge">不支持预览</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    url: string
    name: string
    cover?: string
    clickable?: boolean
  }>(),
  { clickable: true }
)

const emit = defineEmits<{
  (e: 'preview', url: string): void
}>()

const useVideo = computed(() => {
  const lowerName = props.name.toLowerCase()
  return ['.mp4', '.webm', '.mov'].some((ext) => lowerName.endsWith(ext))
})

const handleClick = () => {
  if (props.clickable) {
    emit('preview', props.url)
  }
}
</script>

<style scoped>
.media-thumb {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 14px;
  overflow: hidden;
}

.media-thumb.clickable {
  cursor: pointer;
}

.media-thumb:not(.clickable) .preview-video {
  cursor: default;
}

.media-fallback {
  width: 260px;
  aspect-ratio: 16 / 9;
  border-radius: 14px;
  background: #e5e7eb;
  color: #6b7280;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
}

.media-badge {
  position: absolute;
  right: 8px;
  bottom: 8px;
  background: rgba(17, 24, 39, 0.7);
  color: #fff;
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 999px;
}
</style>
