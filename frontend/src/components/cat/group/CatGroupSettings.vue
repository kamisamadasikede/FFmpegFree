<template>
  <div class="cg-modal-bg" @click.self="emit('close')">
    <div class="cg-modal" role="dialog" aria-modal="true" :aria-label="t('cat.groupChat.settings')" data-testid="cat-group-settings-dialog">
      <h3>{{ t('cat.groupChat.settings') }}</h3>
      <div class="cg-fld" style="margin: 14px 0 12px">
        <label for="cg-rename">{{ t('cat.groupChat.groupName') }}</label>
        <input id="cg-rename" v-model="name" class="cg-in" :class="{ bad }" maxlength="40" style="height: 34px" @keydown.enter.prevent="saveName()" @blur="saveName()">
        <div v-if="bad" class="cg-err">{{ t('cat.groupChat.groupNameBad') }}</div>
      </div>
      <div class="cg-switch">
        <div class="tx">
          <b>{{ t('cat.groupChat.autoRoute') }}</b>
          <span>{{ t('cat.groupChat.autoRouteHint') }}</span>
        </div>
        <button type="button" class="cg-tog" :class="{ on: store.group?.autoRoute }" role="switch" :aria-checked="!!store.group?.autoRoute" :aria-label="t('cat.groupChat.autoRoute')" data-testid="cat-group-autoroute" @click="toggle()" />
      </div>
      <div class="cg-switch">
        <div class="tx">
          <b>{{ t('cat.groupChat.first') }} · {{ store.membersSorted[0]?.name }}</b>
          <span>{{ t('cat.groupChat.firstTip') }}</span>
        </div>
      </div>
      <div class="cg-switch" style="align-items: center">
        <div class="tx"><b style="color: var(--cat-danger-text)">{{ t('cat.groupChat.deleteGroup') }}</b></div>
        <button v-if="!confirmDel" type="button" class="cg-btn red" data-testid="cat-group-delete" @click="confirmDel = true">{{ t('cat.groupChat.delete') }}</button>
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
      <div class="ft"><button type="button" class="cg-btn" @click="emit('close')">{{ t('cat.groupChat.close') }}</button></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useCatGroupChat } from '@/stores/catGroupChat'

const emit = defineEmits<{ close: []; deleted: [] }>()
const { t } = useI18n()
const store = useCatGroupChat()
const name = ref(store.group?.name ?? '')
const bad = computed(() => !name.value.trim() || name.value.trim().length > 40)
const confirmDel = ref(false)

async function saveName() {
  if (bad.value || name.value.trim() === store.group?.name) return
  try {
    await store.rename(name.value.trim())
  } catch {
    ElMessage.error(t('cat.groupChat.saveFailed'))
  }
}
async function toggle() {
  try {
    await store.setAutoRoute(!store.group?.autoRoute)
  } catch {
    ElMessage.error(t('cat.groupChat.saveFailed'))
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
