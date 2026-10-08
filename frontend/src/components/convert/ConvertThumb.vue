<script setup lang="ts">
// 转换页封面（设计 §7.3 第 13 条；包 20 产品经理 + 设计 10-08 改版）。父行和子记录同一尺寸、同一圆角。
//   有画面：缩略图（§14.2：不再叠播放图标，左上已有格式角标）
//   没有画面 / 还在生成 / 生成失败 / 子记录未完成：类型封面——视频 = 胶片，图片 = 图片图标，音频一律音符；尺寸和缩略图一样，切换不跳
//   文件不在了：虚线占位 + 禁止图标
// 左上角格式角标（MP4、GIF、FLAC…），视频右下角时长。
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import type { IconName } from '@/components/icon/icons'
import type { ThumbState } from '@/api/convertRecords'
import type { CoverKind } from '@/utils/convertText'

const props = withDefaults(
  defineProps<{
    state?: ThumbState | null
    /** 文件不在了（优先于 state） */
    gone?: boolean
    /** 子记录未完成：不取缩略图，直接显示类型封面 */
    pend?: boolean
    /** 子记录尺寸（56×32） */
    sm?: boolean
    /** 封面类型（没有画面时显示哪种类型封面；音频永远显示音符封面） */
    cover?: CoverKind
    /** 左上角格式角标（大写） */
    fmt?: string
    /** 右下角时长（只给视频） */
    dur?: string
    /** 可以点（预览）；不可点时渲染成 div */
    clickable?: boolean
    label?: string
  }>(),
  { state: null, gone: false, pend: false, sm: false, cover: 'video', fmt: '', dur: '',  clickable: false, label: '' },
)
const emit = defineEmits<{ click: [] }>()

/** 显示哪一种：gone / img / 类型封面（video | image | audio） */
const kind = computed(() => {
  if (props.gone || props.state?.kind === 'missing') return 'gone'
  if (props.cover === 'audio') return 'audio'
  if (!props.pend && props.state?.kind === 'img') return 'img'
  return props.cover
})
const ICON: Record<CoverKind, IconName> = { video: 'film', image: 'image', audio: 'music' }
const COV: Record<CoverKind, string> = { video: 'c-v', image: 'c-i', audio: 'c-a' }
/** 还在生成：类型封面 + 扫光（还没取到结果；子记录未完成不算） */
const gen = computed(() => kind.value !== 'gone' && kind.value !== 'img' && !props.pend && !props.state)
const url = computed(() => (props.state?.kind === 'img' ? props.state.url : ''))
const btn = computed(() => props.clickable && kind.value !== 'gone')
</script>
<template>
  <component
    :is="btn ? 'button' : 'div'"
    :type="btn ? 'button' : undefined"
    class="cv-th"
    :class="[kind, kind === 'gone' || kind === 'img' ? '' : `cv-cov ${COV[kind]}`, { sm, gen }]"
    :aria-busy="gen || undefined"
    :data-cover="kind"
    :aria-label="btn ? label : undefined"
    :title="gen ? '正在生成缩略图' : btn ? label : undefined"
    @click="btn && emit('click')"
  >
    <FIcon v-if="kind === 'gone'" name="block" :size="14" />
    <template v-else-if="kind === 'img'">
      <img :src="url" alt="" draggable="false" />
    </template>
    <FIcon v-else :name="ICON[kind]" :size="14" />
    <em v-if="fmt" class="cv-cf">{{ fmt }}</em>
    <span v-if="dur && kind !== 'gone'">{{ dur }}</span>
  </component>
</template>
