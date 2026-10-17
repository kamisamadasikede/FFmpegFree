<template>
  <div class="cg-modal-bg" @click.self="emit('close')">
    <div class="cg-modal" role="dialog" aria-modal="true" :aria-label="t('cat.groupChat.settings')" data-testid="cat-group-settings-dialog">
      <h3>{{ t('cat.groupChat.settings') }}</h3>
      <div class="cg-fld1">
        <label for="cg-rename">{{ t('cat.groupChat.groupName') }}</label>
        <input id="cg-rename" v-model="name" class="cg-in" :class="{ bad }" maxlength="40">
        <div v-if="bad" class="cg-err">{{ t('cat.groupChat.groupNameBad') }}</div>
      </div>
      <div class="cg-fld1" style="margin-top: 0">
        <span>{{ t('cat.groupChat.memberOrder') }}</span>
        <div class="cg-order" role="list">
          <div v-for="(id, i) in order" :key="id" class="it" role="listitem" @dragover.prevent @drop.prevent="dropOn(id)">
            <span class="grip" draggable="true" :title="t('cat.groupChat.dragTip')" @dragstart="dragId = id"><FIcon name="grip" :size="13" /></span>
            <span class="av sm" :style="{ '--c': memberColorVar(store.memberById(id)?.order ?? i) }">{{ Array.from(store.nameOf(id))[0] }}</span>
            {{ store.nameOf(id) }}<span class="tt">{{ store.memberById(id)?.title }}</span>
            <span v-if="i === 0" class="cg-badge acc">{{ t('cat.groupChat.firstBadge') }}</span>
          </div>
        </div>
      </div>
      <div class="cg-switch">
        <div class="tx">
          <b>{{ t('cat.groupChat.autoRoute') }}</b>
          <span>{{ t('cat.groupChat.autoRouteHint') }}</span>
        </div>
        <button type="button" class="cg-tog" :class="{ on: auto }" role="switch" :aria-checked="auto" :aria-label="t('cat.groupChat.autoRoute')" data-testid="cat-group-autoroute" @click="auto = !auto" />
      </div>
      <div class="cg-switch" style="align-items: center">
        <div class="tx"><b>{{ t('cat.groupChat.deleteGroup') }}</b><span>{{ t('cat.groupChat.deleteHint') }}</span></div>
        <button v-if="!confirmDel" type="button" class="cg-btn red" data-testid="cat-group-delete" @click="confirmDel = true">{{ t('cat.groupChat.deleteGroup') }}</button>
      </div>
      <div v-if="confirmDel" class="cg-gen bad" role="alert">
        <div class="bx">
          <div class="h" style="color: var(--cat-text-2); margin: 0">{{ t('cat.groupChat.deleteConfirm', { name: store.group?.name ?? '' }) }}</div>
          <div class="row" style="justify-content: flex-end">
            <button type="button" class="cg-btn" @click="confirmDel = false">{{ t('cat.groupChat.cancel') }}</button>
            <button type="button" class="cg-btn dng" @click="del()">{{ t('cat.groupChat.delete') }}</button>
          </div>
        </div>
      </div>
      <div class="ft">
        <button type="button" class="cg-btn" @click="emit('close')">{{ t('cat.groupChat.cancel') }}</button>
        <button type="button" class="cg-btn pri" :disabled="bad || saving" data-testid="cat-group-settings-save" @click="save()">{{ t('cat.groupChat.save') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { memberColorVar } from '@/api/catGroupChat'
import { useCatGroupChat } from '@/stores/catGroupChat'

const emit = defineEmits<{ close: []; deleted: [] }>()
const { t } = useI18n()
const store = useCatGroupChat()
const name = ref(store.group?.name ?? '')
const bad = computed(() => !name.value.trim() || name.value.trim().length > 40)
const confirmDel = ref(false)

const auto = ref(!!store.group?.autoRoute)
const order = ref(store.membersSorted.map((m) => m.id))
const dragId = ref('')
const saving = ref(false)
function dropOn(target: string) {
  const from = dragId.value
  dragId.value = ''
  if (!from || from === target) return
  const ids = order.value.filter((x) => x !== from)
  ids.splice(ids.indexOf(target), 0, from)
  order.value = ids
}
async function save() {
  if (bad.value) return
  saving.value = true
  try {
    if (name.value.trim() !== store.group?.name) await store.rename(name.value.trim())
    if (auto.value !== !!store.group?.autoRoute) await store.setAutoRoute(auto.value)
    if (order.value.join() !== store.membersSorted.map((m) => m.id).join()) await store.reorder(order.value)
    emit('close')
  } catch {
    ElMessage.error(t('cat.groupChat.saveFailed'))
  } finally {
    saving.value = false
  }
}
async function del() {
  try {
    await store.deleteGroup()
    emit('deleted')
  } catch {
    ElMessage.error(t('cat.groupChat.saveFailed'))
  }
}
</script>
