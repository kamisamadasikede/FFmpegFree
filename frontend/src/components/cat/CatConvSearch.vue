<template>
  <div class="mask" @mousedown.self="emit('close')">
    <div class="panel" role="dialog" :aria-label="t('cat.misc.s_92a62b4740')" data-testid="cat-conv-search-panel" @keydown.esc="emit('close')">
      <header class="head">
        <h3>{{ t('cat.misc.s_92a62b4740') }}</h3>
        <span class="count">{{ hits.length }}条</span>
        <button type="button" class="x" :aria-label="t('cat.misc.s_b15d91274e')" @click="emit('close')"><FIcon name="x" :size="16" /></button>
      </header>
      <label class="field">
        <FIcon name="search" :size="15" />
        <input
          ref="inputEl"
          v-model="query"
          type="search"
          :placeholder="t('cat.misc.s_960ee596d9')"
          :aria-label="t('cat.misc.s_960ee596d9')"
          data-testid="cat-conv-search-input"
        />
      </label>
      <div ref="listEl" class="list" @scroll.passive="onScroll">
        <p v-if="!hits.length" class="empty">{{ query.trim() ? t('cat.misc.s_d5fd2483cc') : t('cat.misc.s_ea62053dc3') }}</p>
        <button
          v-for="hit in rows"
          :key="hit.index"
          type="button"
          class="hit"
          data-testid="cat-conv-search-hit"
          @click="emit('pick', hit.index)"
        >
          <b v-if="hit.q">{{ hit.q }}</b>
          <span v-if="hit.a">{{ hit.a }}</span>
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
  display: flex;
  justify-content: center;
  padding: 16px 16px 24px;
  background: color-mix(in srgb, var(--cat-scrim) 55%, transparent);
}
.panel {
  width: min(980px, 100%);
  max-height: min(100%, 640px);
  display: flex;
  flex-direction: column;
  border-radius: var(--cat-radius-xl);
  background: var(--cat-elevated);
  border: 1px solid var(--cat-border);
  box-shadow: var(--cat-shadow-lg);
  color: var(--cat-text);
  overflow: hidden;
}
.head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 16px 0 20px;
}
.head h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.count {
  color: var(--cat-muted);
  font-size: 13px;
}
.x {
  margin-left: auto;
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: var(--cat-radius-sm);
  background: none;
  color: var(--cat-muted);
  cursor: pointer;
}
.x:hover {
  background: var(--cat-hover);
  color: var(--cat-text);
}
.field {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  margin: 12px 20px 0;
  padding: 0 12px;
  border: 1px solid var(--cat-border);
  border-radius: var(--cat-radius-md);
  background: var(--cat-sunken);
  color: var(--cat-muted);
}
.field input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: none;
  background: transparent;
  color: var(--cat-text);
  font: inherit;
  font-size: 14px;
}
.field input::-webkit-search-cancel-button {
  appearance: none;
}
.list {
  flex: 1;
  min-height: 0;
  margin-top: 12px;
  overflow-y: auto;
}
.empty {
  margin: 28px 20px;
  text-align: center;
  color: var(--cat-muted);
  font-size: 13px;
}
.hit {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  width: 100%;
  padding: 10px 20px;
  border: 0;
  border-top: 1px solid var(--cat-divider);
  background: transparent;
  color: var(--cat-text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.hit:hover {
  background: var(--cat-hover);
}
.hit b,
.hit span {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  overflow: hidden;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.45;
}
.hit b {
  -webkit-line-clamp: 2;
  font-size: 13px;
  font-weight: 600;
}
.hit span {
  -webkit-line-clamp: 2;
  font-size: 12px;
  color: var(--cat-muted);
}
</style>
