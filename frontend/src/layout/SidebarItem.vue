<template>
  <el-tooltip v-if="gated" content="需要先安装转换组件" effect="light" placement="right" :show-after="300" :hide-after="0">
    <a
      class="nav-item gated"
      :class="{ active: isActive }"
      role="button"
      tabindex="0"
      :aria-label="item.label"
      @click.prevent="openInstall"
      @keydown.enter.prevent="openInstall"
      @keydown.space.prevent="openInstall"
    >
      <FIcon :name="item.icon" />
      <span v-show="!collapsed" class="label">{{ item.label }}</span>
    </a>
  </el-tooltip>
  <RouterLink v-else :to="item.path" class="nav-item" :class="{ active: isActive }" :title="collapsed ? item.label : undefined">
    <FIcon :name="item.icon" />
    <span v-show="!collapsed" class="label">{{ item.label }}</span>
    <span v-if="item.tag && !collapsed" class="tag">{{ item.tag }}</span>
    <span v-if="badge && !collapsed" class="badge">{{ badge }}</span>
  </RouterLink>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import { useFFmpegStore } from '@/stores/ffmpeg'
import type { NavItem } from './navigation'

const props = defineProps<{ item: NavItem; collapsed: boolean; badge?: number }>()
const route = useRoute()
const ffmpeg = useFFmpegStore()

// ffmpeg 不可用时，依赖它的入口保持可见但置灰：不导航，点击改为重新打开安装对话框（「稍后」不会让它恢复）
const gated = computed(() => !!props.item.needsFFmpeg && ffmpeg.featuresBlocked)
function openInstall() {
  ffmpeg.dialogOpen = true
}

// route.matched 为空说明首次导航还没完成（此时 route.path 是初始的 '/'，会让「转换」闪一下高亮），先不高亮任何项
const isActive = computed(
  () =>
    route.matched.length > 0 &&
    (props.item.path === '/' ? route.path === '/' : route.path === props.item.path || route.path.startsWith(props.item.path + '/')),
)
</script>

<style scoped>
.nav-item {
  position: relative;
  height: 36px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 10px;
  margin-bottom: 2px;
  border-radius: var(--ff-radius-md);
  color: var(--ff-text-2);
  font-size: 13.5px;
  white-space: nowrap;
  transition:
    background var(--ff-dur-fast) var(--ff-ease),
    color var(--ff-dur-fast) var(--ff-ease);
}
.nav-item:hover {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
/* 选中态：软底 + 主色文字 + 左侧 3px 短条（原型 .nav-item.is-active） */
.nav-item.active {
  background: var(--ff-bg-active);
  color: var(--ff-text-1);
  font-weight: 600;
}
.nav-item.active::before {
  content: '';
  position: absolute;
  left: -10px;
  top: 50%;
  width: 3px;
  height: 18px;
  transform: translateY(-50%);
  border-radius: 0 3px 3px 0;
  background: var(--ff-primary);
}
.nav-item.gated {
  color: var(--ff-text-3);
  cursor: pointer;
}
.nav-item.gated:hover {
  background: transparent;
}
.nav-item.gated.active {
  background: transparent; /* 置灰且是当前页：不高亮（原型 31/32） */
  font-weight: 400;
}
.nav-item.gated.active::before {
  display: none;
}
.nav-item.gated:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: -2px;
}
.tag {
  margin-left: auto;
  font-size: 10px;
  line-height: 16px;
  padding: 0 5px;
  border-radius: 4px;
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
  font-weight: 700;
}
.badge {
  margin-left: auto;
  min-width: 20px;
  height: 18px;
  padding: 0 6px;
  border-radius: 9px;
  background: var(--ff-primary);
  color: var(--ff-on-primary);
  font-size: 11px;
  font-weight: 700;
  display: grid;
  place-items: center;
}

/* ---------- 窄窗口：只留图标 ---------- */
@media (max-width: 1024px) {
  .nav-item {
    justify-content: center;
    height: 40px;
    padding: 0;
    gap: 0;
  }
  .nav-item.active::before {
    display: none;
  }
  .nav-item .label,
  .nav-item .tag,
  .nav-item .badge {
    display: none;
  }
}
</style>
