<template>
  <section class="panel group" :aria-labelledby="headingId">
    <div class="phead">
      <h2 :id="headingId">语音识别组件</h2>
      <span class="dsub">转字幕用</span>
    </div>
    <div class="srow fcomp">
      <div class="l">
        <span class="fok"><i class="fdot off" aria-hidden="true" />语音识别组件未就绪</span>
        <small>{{ ASR_NOT_PUBLISHED }}</small>
      </div>
    </div>
    <div class="srow feng">
      <div class="l">
        <span class="fok">识别档位</span>
        <small>默认「标准」。切换只改引导体积，发布后需手动下载，不会自动下载。</small>
      </div>
      <div ref="ddEl" class="fdd">
        <button
          type="button"
          class="fdd-btn"
          aria-haspopup="listbox"
          :aria-expanded="ddOpen"
          aria-label="识别档位"
          @click="ddOpen = !ddOpen"
        >
          <span>{{ currentLabel }}</span><FIcon name="down" :size="14" />
        </button>
        <ul v-if="ddOpen" class="fdd-list" role="listbox" aria-label="识别档位" @keydown.esc.stop="ddOpen = false">
          <li
            v-for="o in options"
            :key="o.id"
            role="option"
            :aria-selected="o.id === store.tier"
            tabindex="0"
            :class="{ on: o.id === store.tier }"
            @click="choose(o.id)"
            @keydown.enter.prevent="choose(o.id)"
          >
            <FIcon v-if="o.id === store.tier" name="check" :size="14" class="ck" /><i v-else class="ck" />
            <div><b>{{ o.label }}</b><small>{{ o.hint }}</small></div>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
// 设置页「语音识别组件」块（契约 6.18.3 / asrTier）。组件未发布：无下载按钮。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { useLangAsrStore } from '@/stores/langAsr'
import type { AsrTier } from '@/api/lang'
import { ASR_NOT_PUBLISHED, TIER_HD_SIZE, TIER_STANDARD_SIZE } from '@/utils/langText'

defineProps<{ headingId?: string }>()
const store = useLangAsrStore()
const ddOpen = ref(false)
const ddEl = ref<HTMLElement | null>(null)

const options = [
  { id: 'standard' as AsrTier, label: '标准', hint: TIER_STANDARD_SIZE },
  { id: 'hd' as AsrTier, label: '高清', hint: TIER_HD_SIZE },
]
const currentLabel = computed(() => options.find((o) => o.id === store.tier)?.label ?? '标准')

async function choose(id: AsrTier) {
  ddOpen.value = false
  await store.changeTier(id)
}
function onDocDown(e: MouseEvent) {
  if (ddOpen.value && ddEl.value && !ddEl.value.contains(e.target as Node)) ddOpen.value = false
}
onMounted(() => {
  document.addEventListener('mousedown', onDocDown)
  if (!store.loaded) void store.init()
})
onBeforeUnmount(() => document.removeEventListener('mousedown', onDocDown))
</script>

<style scoped>
.dsub { font-size: 12px; color: var(--ff-text-3); }
.fcomp { align-items: flex-start; }
.fcomp .l { min-width: 0; flex: 1; }
.fok { display: flex; align-items: center; gap: 8px; font-size: 13px; line-height: 20px; font-weight: 500; color: var(--ff-text-1); }
.fdot { width: 8px; height: 8px; border-radius: 50%; background: var(--ff-success); flex: none; }
.fdot.off { background: var(--ff-text-3); }
.fcomp .l small { display: block; font-size: 12px; line-height: 18px; color: var(--ff-text-2); margin-top: 2px; }
.feng { align-items: center; }
.feng .l { flex: 1; min-width: 0; }
.feng .l small { display: block; font-size: 12px; line-height: 18px; color: var(--ff-text-2); margin-top: 2px; }
.fdd { position: relative; flex: none; }
.fdd-btn { width: 160px; height: 30px; display: flex; align-items: center; justify-content: space-between; gap: 6px; padding: 0 10px; border-radius: 6px; border: 1px solid var(--ff-border); background: var(--ff-bg-surface); color: var(--ff-text-1); font-size: 13px; cursor: pointer; }
.fdd-btn[aria-expanded='true'] { border-color: var(--ff-primary); box-shadow: 0 0 0 2px color-mix(in srgb, var(--ff-primary) 20%, transparent); }
.fdd-btn svg { color: var(--ff-text-3); }
.fdd-list { position: absolute; right: 0; top: calc(100% + 6px); z-index: 30; width: 224px; margin: 0; padding: 4px; list-style: none; border-radius: 8px; background: var(--ff-bg-elevated); border: 1px solid var(--ff-border); box-shadow: 0 8px 24px rgba(0, 0, 0, 0.16); }
.fdd-list li { display: flex; gap: 6px; align-items: flex-start; padding: 7px 8px; border-radius: 6px; cursor: pointer; outline: none; }
.fdd-list li:hover, .fdd-list li:focus-visible, .fdd-list li.on { background: var(--ff-bg-hover); }
.fdd-list .ck { width: 14px; height: 14px; flex: none; margin-top: 3px; color: var(--ff-primary-text); }
.fdd-list b { display: block; font-size: 13px; line-height: 20px; font-weight: 400; color: var(--ff-text-1); }
.fdd-list small { display: block; font-size: 12px; line-height: 16px; color: var(--ff-text-2); }
</style>
