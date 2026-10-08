<script setup lang="ts">
// 转换页缩略图（设计 §7.3 第 13 条）：三种显示——正常缩略图；文件不在了 = 虚线占位 + 禁止图标；纯音频 / 取不到画面 = 类型图标。
// 子记录还没完成（排队 / 转换中 / 失败 / 已取消）不取缩略图，显示灰色占位（胶片 / 音符）。
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import type { ThumbState } from '@/api/convertRecords'

const props = withDefaults(
  defineProps<{
    state?: ThumbState | null
    /** 文件不在了（优先于 state） */
    gone?: boolean
    /** 子记录未完成：灰色占位 */
    pend?: boolean
    /** 音频（类型图标 / 占位用音符） */
    audio?: boolean
    /** 右下角时长 */
    dur?: string
    /** 叠播放图标（完成的子记录） */
    play?: boolean
    sm?: boolean
    /** 可以点（预览）；不可点时渲染成 div */
    clickable?: boolean
    label?: string
  }>(),
  { state: null, gone: false, pend: false, audio: false, dur: '', play: false, sm: false, clickable: false, label: '' },
)
const emit = defineEmits<{ click: [] }>()

const kind = computed(() => {
  if (props.gone || props.state?.kind === 'missing' || props.state?.kind === 'gone') return 'gone'
  if (props.pend) return 'pend'
  if (props.state?.kind === 'type') return props.audio ? 'audio' : 'type'
  if (props.state?.kind === 'img') return 'img'
  return props.audio ? 'audio' : 'load'
})
const url = computed(() => (props.state?.kind === 'img' ? props.state.url : ''))
</script>
<template>
  <component
    :is="clickable && kind !== 'gone' ? 'button' : 'div'"
    :type="clickable && kind !== 'gone' ? 'button' : undefined"
    class="cv-th"
    :class="[kind, { sm }]"
    :aria-label="clickable && kind !== 'gone' ? label : undefined"
    :title="clickable && kind !== 'gone' ? label : undefined"
    @click="clickable && kind !== 'gone' && emit('click')"
  >
    <template v-if="kind === 'gone'"><FIcon name="block" /></template>
    <template v-else-if="kind === 'pend'"><FIcon :name="audio ? 'music' : 'film'" /></template>
    <template v-else-if="kind === 'audio'"><FIcon name="music" /></template>
    <template v-else-if="kind === 'type'"><FIcon name="doc" /></template>
    <template v-else-if="kind === 'img'">
      <img :src="url" alt="" draggable="false" />
      <div v-if="play" class="pl"><FIcon name="play" /></div>
    </template>
    <span v-if="dur && kind !== 'gone' && kind !== 'pend'">{{ dur }}</span>
  </component>
</template>
