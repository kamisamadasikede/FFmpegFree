<template>
  <div class="cgs" data-testid="cat-group-section">
    <div class="cgs-hd">
      <FIcon name="chat" :size="11" />{{ t('cat.groupChat.section') }}<span class="sp" />
      <button v-if="!missing" type="button" :title="t('cat.groupChat.newGroup')" :aria-label="t('cat.groupChat.newGroup')" data-testid="cat-group-new" @click.stop="openCreateGroup(projectId)"><FIcon name="plus" :size="12" /></button>
    </div>
    <button
      v-for="g in groups"
      :key="g.id"
      type="button"
      class="cgs-it"
      :class="{ on: catPanels.view === 'group' && catPanels.activeGroupId === g.id }"
      :aria-current="catPanels.view === 'group' && catPanels.activeGroupId === g.id ? 'page' : undefined"
      data-testid="cat-group-item"
      @click="openGroup(g.id)"
    >
      <FIcon name="chat" :size="14" /><span class="t">{{ g.name }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import FIcon from '@/components/icon/FIcon.vue'
import { useCatGroupChat } from '@/stores/catGroupChat'
import { catPanels, openCreateGroup, openGroup } from '@/views/cat/catPanels'
import './catGroup.css'

const props = defineProps<{ projectId: string; missing?: boolean }>()
const { t } = useI18n()
const store = useCatGroupChat()
const groups = computed(() => store.groupsByProject[props.projectId] ?? [])
onMounted(() => void store.loadGroups(props.projectId))
</script>
