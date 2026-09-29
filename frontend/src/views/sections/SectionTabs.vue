<template>
  <div class="section" :class="{ fill: route.meta.fill }">
    <div v-if="tabs.length > 1" class="tabs" role="tablist">
      <RouterLink
        v-for="tab in tabs"
        :key="tab.path"
        :to="tab.path"
        class="tab"
        :class="{ active: route.path === tab.path }"
        role="tab"
      >
        {{ tab.label }}
      </RouterLink>
    </div>
    <div class="section-body">
      <RouterView />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

// 读取当前一级路由的子路由，生成页签
const route = useRoute()
const router = useRouter()
const tabs = computed(() => {
  const parent = route.matched[0]
  const record = router.getRoutes().find((r) => r.path === parent?.path)
  return (record?.children ?? [])
    .filter((c) => c.meta?.tab)
    .map((c) => ({ label: c.meta!.tab as string, path: `${parent.path}/${c.path}` }))
})
</script>

<style scoped>
.section {
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-4);
  min-height: 100%;
}
.section.fill {
  height: 100%; /* 路由 meta.fill：页面自己撑满内容区（工具页的编辑器需要确定高度） */
}
.tabs {
  display: flex;
  gap: var(--ff-space-1);
  border-bottom: 1px solid var(--ff-border);
}
.tab {
  padding: var(--ff-space-2) var(--ff-space-3);
  margin-bottom: -1px;
  color: var(--ff-text-2);
  border-bottom: 2px solid transparent;
  transition: color var(--ff-dur-fast) var(--ff-ease);
}
.tab:hover {
  color: var(--ff-text-1);
}
.tab.active {
  color: var(--ff-primary);
  border-bottom-color: var(--ff-primary);
  font-weight: 500;
}
.section-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
</style>
