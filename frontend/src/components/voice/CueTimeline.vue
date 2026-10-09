<script setup lang="ts">
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { formatCueTime, parseCueTime, validateCues, type SubtitleCue } from '@/api/lang'
import { CUE_MAX_CHARS } from '@/utils/langText'

const props = defineProps<{ cues: SubtitleCue[]; editing: boolean }>()
const emit = defineEmits<{
  update: [id: string, patch: Partial<Pick<SubtitleCue, 'text' | 'startMs' | 'endMs'>>]
}>()

const countText = computed(() => {
  if (!props.cues.length) return '空'
  const v = validateCues(props.cues)
  return v.ok ? `${props.cues.length} 条 · 可改字和起止时间` : `${props.cues.length} 条 · 有错误`
})

function onStart(id: string, raw: string) {
  const ms = parseCueTime(raw)
  if (ms == null) return
  emit('update', id, { startMs: ms })
}
function onEnd(id: string, raw: string) {
  const ms = parseCueTime(raw)
  if (ms == null) return
  emit('update', id, { endMs: ms })
}
function cueBad(c: SubtitleCue): boolean {
  if (!(c.endMs > c.startMs)) return true
  if ([...c.text].length > CUE_MAX_CHARS) return true
  const others = props.cues.filter((x) => x.id !== c.id)
  return others.some((o) => !(c.endMs <= o.startMs || o.endMs <= c.startMs))
}
</script>

<template>
  <div class="lg-timeline" :class="{ edit: editing && cues.length }">
    <div class="th">
      字幕时间轴
      <span>{{ countText }}</span>
      <span class="sp" />
    </div>
    <div v-if="!cues.length" class="tb">
      <FIcon name="caption" />
      <span>生成后可在这里改字幕</span>
    </div>
    <div v-else class="cues">
      <div v-for="(c, i) in cues" :key="c.id" class="lg-cue" :class="{ bad: cueBad(c) }">
        <span class="n">{{ i + 1 }}</span>
        <div class="tm">
          <input
            :value="formatCueTime(c.startMs)"
            aria-label="开始时间"
            @change="onStart(c.id, ($event.target as HTMLInputElement).value)"
          />
          <span>→</span>
          <input
            :value="formatCueTime(c.endMs)"
            aria-label="结束时间"
            @change="onEnd(c.id, ($event.target as HTMLInputElement).value)"
          />
        </div>
        <textarea
          rows="2"
          :value="c.text"
          :aria-label="`字幕第 ${i + 1} 条`"
          @input="emit('update', c.id, { text: ($event.target as HTMLTextAreaElement).value })"
        />
      </div>
    </div>
  </div>
</template>
