<template>
  <span class="pic" :class="[file.kind, { mini, tile }]">
    <img v-if="showImage" :src="src" alt="" data-testid="cat-attach-thumb" />
    <video v-else-if="showVideo" :src="src" muted playsinline preload="metadata" @loadedmetadata="prime" />
    <FIcon v-else :name="file.kind === 'image' ? 'image' : 'file'" :size="mini ? 14 : 22" />
    <i v-if="file.kind === 'video'" class="play" aria-hidden="true" />
  </span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { computed, onMounted, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { catMediaObjectURL, catMediaURL, imageDataUrl, showsThumb, type CatShownFile } from './catMedia'

const props = defineProps<{ file: CatShownFile; mini?: boolean; tile?: boolean }>()
const src = ref('')

const showImage = computed(() => props.file.kind === 'image' && !!src.value)
const showVideo = computed(() => props.file.kind === 'video' && !!src.value)

function localSrc(): string {
  if (props.file.kind === 'image' && props.file.data) return imageDataUrl(props.file)
  return ''
}

async function load() {
  const path = props.file.path
  const kind = props.file.kind
  const inline = localSrc()
  if (inline) {
    src.value = inline
    return
  }
  src.value = ''
  if (!path || !showsThumb(kind)) return
  if (kind === 'image') {
    const obj = await catMediaObjectURL(path)
    if (props.file.path !== path || props.file.kind !== kind) return
    if (obj) {
      src.value = obj
      return
    }
  }
  const hit = await catMediaURL(path)
  if (props.file.path !== path || props.file.kind !== kind) return
  if (hit) src.value = hit.url
}

function prime(e: Event) {
  const v = e.target
  if (!(v instanceof HTMLVideoElement)) return
  if (Number.isFinite(v.duration) && v.duration > 0.2 && v.currentTime < 0.05) {
    try {
      v.currentTime = 0.1
    } catch {
      /* 有的视频不允许拖到开头之外 */
    }
  }
}

onMounted(() => {
  void load()
})
watch(() => [props.file.path, props.file.data, props.file.kind] as const, () => {
  void load()
})
</script>

<style scoped>
.pic {
  position: relative;
  display: grid;
  place-items: center;
  width: 100%;
  height: 100%;
  min-height: 0;
  overflow: hidden;
  color: var(--cat-muted);
}
.pic img,
.pic video {
  width: 100%;
  height: 100%;
  object-fit: cover;
  pointer-events: none;
  background: var(--cat-media-bg);
}
.pic.mini {
  width: 22px;
  height: 22px;
  border-radius: 4px;
  flex: none;
}
.pic.tile {
  width: 72px;
  height: 56px;
  border-radius: 8px;
  flex: none;
}
.pic.tile .play {
  border-width: 4px 0 4px 7px;
}
.play {
  position: absolute;
  width: 0;
  height: 0;
  border-style: solid;
  border-width: 5px 0 5px 8px;
  border-color: transparent transparent transparent var(--cat-on-media);
  filter: drop-shadow(0 0 1px var(--cat-media-shadow));
  pointer-events: none;
}
</style>
