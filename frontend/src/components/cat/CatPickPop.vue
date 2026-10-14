<template>
  <teleport to="body">
    <div
      ref="card"
      class="pop"
      data-cat-pick-pop
      data-testid="cat-pick-pop"
      role="dialog"
      :aria-label="t('cat.misc.s_9f4e192388')"
      :style="{ left: pos.left + 'px', top: pos.top + 'px' }"
      @keydown="onKey"
      @pointerdown.stop
    >
      <div class="ttl" :title="title">{{ title }}</div>
      <div v-if="selector" class="path" :title="selector">{{ selector }}</div>
      <textarea
        ref="ta"
        v-model="note"
        class="box"
        rows="3"
        :placeholder="intent === 'ask' ? COPY.pickPhAsk : COPY.pickPhEdit"
      />
      <div class="lb">{{ COPY.pickIntent }}</div>
      <div class="intents" role="group" :aria-label="COPY.pickIntent">
        <button type="button" :class="{ on: intent === 'edit' }" :aria-pressed="intent === 'edit'" @click="intent = 'edit'">
          <FIcon name="edit" :size="14" /><span>{{ COPY.pickEdit }}</span>
        </button>
        <button type="button" :class="{ on: intent === 'ask' }" :aria-pressed="intent === 'ask'" @click="intent = 'ask'">
          <FIcon name="ask" :size="14" /><span>{{ COPY.pickAsk }}</span>
        </button>
      </div>
      <div class="ft">
        <button type="button" class="cancel" @click="emit('close')">{{ COPY.pickCancel }}</button>
        <button type="button" class="add" data-testid="cat-pick-add" @click="submit">
          <span>{{ COPY.pickAdd }}</span>
          <kbd>Ctrl ↵</kbd>
        </button>
      </div>
    </div>
  </teleport>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { CAT_PREVIEW_COPY as COPY } from '@/api/catFilePreview'
import { placePickPop, type PickBox } from './catPick'

const props = defineProps<{ title: string; selector: string; anchor: PickBox }>()
const emit = defineEmits<{ add: [note: string, intent: 'edit' | 'ask']; close: [] }>()

const card = ref<HTMLElement>()
const ta = ref<HTMLTextAreaElement>()
const note = ref('')
const intent = ref<'edit' | 'ask'>('edit')
const pos = ref(placePickPop(props.anchor, { width: 336, height: 280 }, { width: window.innerWidth, height: window.innerHeight }))

function layout() {
  const view = { width: window.innerWidth, height: window.innerHeight }
  const el = card.value
  pos.value = placePickPop(props.anchor, { width: el?.offsetWidth || 336, height: el?.offsetHeight || 280 }, view)
}

function submit() {
  emit('add', note.value, intent.value)
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    emit('close')
    return
  }
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
    e.preventDefault()
    submit()
  }
}

watch(() => [props.anchor.left, props.anchor.top, props.anchor.width, props.anchor.height], () => nextTick(layout))

onMounted(() => {
  layout()
  nextTick(() => ta.value?.focus())
  window.addEventListener('resize', layout)
})
onBeforeUnmount(() => window.removeEventListener('resize', layout))
</script>

<style scoped>
.pop {
  position: fixed;
  z-index: 4200;
  width: 336px;
  box-sizing: border-box;
  padding: 14px 14px 12px;
  border-radius: 12px;
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.16), 0 2px 6px rgba(0, 0, 0, 0.06);
  color: var(--ff-text-1);
}
.ttl {
  font-size: 15px;
  font-weight: 650;
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.path {
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.4;
  color: var(--ff-text-3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.box {
  display: block;
  width: 100%;
  box-sizing: border-box;
  margin-top: 12px;
  min-height: 88px;
  resize: vertical;
  border: 1px solid var(--ff-border);
  border-radius: 8px;
  background: var(--ff-bg-app);
  color: var(--ff-text-1);
  font: inherit;
  font-size: 13px;
  line-height: 1.5;
  padding: 8px 10px;
}
.box:focus {
  outline: none;
  border-color: var(--ff-text-3);
}
.box::placeholder {
  color: var(--ff-text-3);
}
.lb {
  margin: 12px 0 6px;
  font-size: 13px;
  color: var(--ff-text-2);
}
.intents {
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: 10px;
  background: var(--ff-bg-app);
  border: 1px solid var(--ff-border);
}
.intents button {
  flex: 1;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--ff-text-2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.intents button.on {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.ft {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
}
.cancel,
.add {
  border: none;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.cancel {
  height: 32px;
  padding: 0 10px;
  border-radius: 8px;
  background: transparent;
  color: var(--ff-text-2);
}
.cancel:hover {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.add {
  height: 32px;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px 0 12px;
  border-radius: 8px;
  background: var(--ff-text-1);
  color: var(--ff-bg-surface);
}
.add kbd {
  font: inherit;
  font-size: 11px;
  opacity: 0.72;
}
</style>
