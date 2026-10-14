<template>
  <div class="mask" @mousedown.self="emit('close')">
    <div class="panel" role="dialog" :aria-label="t('cat.misc.s_92a62b4740')" data-testid="cat-conv-search-panel" @keydown.esc="emit('close')">
      <div class="bar">
        <label class="field">
          <FIcon name="search" :size="16" />
          <input
            ref="inputEl"
            v-model="query"
            type="search"
            :placeholder="t('cat.misc.s_960ee596d9')"
            :aria-label="t('cat.misc.s_960ee596d9')"
            data-testid="cat-conv-search-input"
          />
        </label>
        <span class="count">{{ hits.length }}条</span>
      </div>
      <div ref="listEl" class="list" @scroll.passive="onScroll">
        <p v-if="!hits.length" class="empty">{{ query.trim() ? t('cat.misc.s_d5fd2483cc') : t('cat.misc.s_ea62053dc3') }}</p>
        <button
          v-for="(hit, i) in rows"
          :key="hit.index"
          type="button"
          class="hit"
          data-testid="cat-conv-search-hit"
          @click="emit('pick', hit.index)"
        >
          <span class="no">#{{ i + 1 }}</span>
          <span v-if="hit.q" class="q"><b>Q：</b>{{ hit.q }}</span>
          <span v-if="hit.a" class="a"><b>A：</b>{{ hit.a }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { computed, nextTick, onMounted, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import type { CatBlock } from '@/api/catMock'
import { CAT_SEARCH_PAGE, searchTurns, visibleSearchHits } from './catConvSearch'

const props = defineProps<{ blocks: CatBlock[] }>()
const emit = defineEmits<{ close: []; pick: [index: number] }>()

const query = ref('')
const shown = ref(CAT_SEARCH_PAGE)
const inputEl = ref<HTMLInputElement>()
const listEl = ref<HTMLElement>()

const hits = computed(() => searchTurns(props.blocks, query.value))
const rows = computed(() => visibleSearchHits(hits.value, shown.value))

watch(query, () => {
  shown.value = CAT_SEARCH_PAGE
  if (listEl.value) listEl.value.scrollTop = 0
})

function onScroll() {
  const el = listEl.value
  if (!el || shown.value >= hits.value.length) return
  if (el.scrollTop + el.clientHeight < el.scrollHeight - 48) return
  shown.value = Math.min(hits.value.length, shown.value + CAT_SEARCH_PAGE)
}

onMounted(() => nextTick(() => inputEl.value?.focus()))
</script>

<style scoped>
.mask {
  position: absolute;
  z-index: 30;
  inset: 44px 0 0;
}
.panel {
  margin: 8px 48px 0;
  max-height: min(72%, 640px);
  display: flex;
  flex-direction: column;
  background: var(--ff-bg-surface, #fff);
  border: 1px solid var(--ff-border);
  border-radius: 16px;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.14);
  overflow: hidden;
}
.bar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px 8px;
}
.field {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  height: 40px;
  padding: 0 12px;
  border: 1px solid var(--ff-border);
  border-radius: 12px;
  background: var(--ff-bg-surface, #fff);
  color: var(--ff-text-3);
}
.field input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: none;
  background: transparent;
  color: var(--ff-text-1);
  font: inherit;
  font-size: 14px;
}
.field input::-webkit-search-cancel-button {
  appearance: none;
}
.count {
  flex: none;
  color: var(--ff-text-3);
  font-size: 13px;
}
.list {
  flex: 1;
  min-height: 0;
  max-height: 520px;
  overflow-y: auto;
  padding: 4px 12px 12px;
}
.empty {
  margin: 28px 0;
  text-align: center;
  color: var(--ff-text-3);
  font-size: 13px;
}
.hit {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  width: 100%;
  margin: 0 0 8px;
  padding: 12px 14px;
  border: 0;
  border-radius: 12px;
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.hit:hover {
  background: var(--ff-bg-active, var(--ff-bg-hover));
  filter: brightness(0.98);
}
.no {
  color: var(--ff-text-3);
  font-size: 12px;
}
.q,
.a {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.55;
  font-size: 13.5px;
}
.a {
  color: var(--ff-text-2);
}
.q b,
.a b {
  font-weight: 650;
}
</style>
