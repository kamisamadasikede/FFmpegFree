<template>
  <div class="cgs" data-testid="cat-group-section">
    <div class="cgs-hd">
      {{ t('cat.groupChat.section') }} · {{ groups.length }}<span class="sp" />
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
      <span class="ic"><FIcon name="chat" :size="14" /></span><span class="t">{{ g.name }}</span>
      <template v-if="g.id === store.group?.id">
        <i v-if="store.workingMembers.length" class="cgs-dot" :title="t('cat.groupChat.busy')" />
        <span class="cg-stack sm" aria-hidden="true">
          <span v-for="m in store.membersSorted.slice(0, 3)" :key="m.id" class="av xs" :style="{ '--c': memberColorVar(m.order) }">{{ Array.from(m.name)[0] }}</span>
          <span v-if="store.members.length > 3" class="av xs more">+{{ store.members.length - 3 }}</span>
        </span>
      </template>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import FIcon from '@/components/icon/FIcon.vue'
import { memberColorVar } from '@/api/catGroupChat'
import { useCatGroupChat } from '@/stores/catGroupChat'
import { catPanels, openCreateGroup, openGroup } from '@/views/cat/catPanels'
import './catGroup.css'

const props = defineProps<{ projectId: string; missing?: boolean }>()
const { t } = useI18n()
const store = useCatGroupChat()
const groups = computed(() => store.groupsByProject[props.projectId] ?? [])
onMounted(() => void store.loadGroups(props.projectId))
</script>
