<template>
  <!-- 几何简标（设计 v0.4.1 §8.7）：星形 / 圆点 / 方括号，不嵌官方 logo -->
  <svg
    class="cam"
    :class="kind"
    :width="size"
    :height="size"
    viewBox="0 0 16 16"
    aria-hidden="true"
    focusable="false"
  >
    <!-- Grok Build：四角星 -->
    <path
      v-if="kind === 'cat_build'"
      d="M8 1.6 9.4 6.2 14.2 6.6 10.6 9.7 11.8 14.4 8 11.8 4.2 14.4 5.4 9.7 1.8 6.6 6.6 6.2Z"
      fill="currentColor"
    />
    <!-- Claude：实心圆 + 内环 -->
    <g v-else-if="kind === 'claude_cli'">
      <circle cx="8" cy="8" r="6" fill="currentColor" opacity="0.22" />
      <circle cx="8" cy="8" r="3.6" fill="currentColor" />
    </g>
    <!-- Codex：方括号 -->
    <path
      v-else
      d="M5.2 3.2H3.4v9.6h1.8M10.8 3.2h1.8v9.6h-1.8"
      fill="none"
      stroke="currentColor"
      stroke-width="1.6"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
  </svg>
</template>

<script setup lang="ts">
import type { CatProviderKind } from '@/api/catAgents'

defineOptions({ name: 'CatAgentMark' })
withDefaults(
  defineProps<{ kind: CatProviderKind; size?: number }>(),
  { size: 16 },
)
</script>

<style scoped>
.cam {
  flex: none;
  display: block;
}
.cam.cat_build {
  color: var(--cat-accent, #5b8def);
}
.cam.claude_cli {
  color: #d97757;
}
.cam.codex_cli {
  color: var(--cat-text-2, #6b7280);
}
</style>
