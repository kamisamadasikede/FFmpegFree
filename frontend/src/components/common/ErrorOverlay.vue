<template>
  <div class="ff-err-overlay" role="alert">
    <div class="box">
      <div class="eic"><FIcon name="warn" :size="22" /></div>
      <h5>{{ resolved.title }}</h5>
      <p :title="resolved.description">{{ resolved.description }}</p>
      <span class="code">{{ resolved.code }}<template v-if="detail"> · {{ detail }}</template></span>
      <div v-if="resolved.secondary || resolved.primary" class="acts">
        <button v-if="resolved.secondary" type="button" class="btn" @click="onClick(resolved.secondary)">
          <FIcon v-if="resolved.secondary.icon" :name="resolved.secondary.icon" :size="15" />{{ resolved.secondary.label }}
        </button>
        <button v-if="resolved.primary" type="button" class="btn pri" @click="onClick(resolved.primary, true)">
          <FIcon v-if="resolved.primary.icon" :name="resolved.primary.icon" :size="15" />{{ resolved.primary.label }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// 播放器区域的错误遮罩，样式来自 proto/errors.html 的 .errcard / .errbox。
// 铺满父容器（父容器需要 position: relative），所以放在 PlayerShell 的 #overlay 插槽里即可。
// 事件：retry（重试）、viewLog（查看日志）、primary（主按钮被点击，任何主按钮都会触发，
//       重试型主按钮会同时触发 retry；父组件只需要监听 retry / viewLog，primary 用于“去设置”这类额外动作）。
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import FIcon from '../icon/FIcon.vue'
import { resolveError, type ErrorButton } from '../../errors/errorMessages'

const props = defineProps<{
  code: string
  /** 附加信息，显示在错误码后面，例如“服务器返回 403” */
  detail?: string
  /** 未知错误码时作为描述（后端 message） */
  message?: string
}>()

const emit = defineEmits<{ retry: []; viewLog: []; primary: [] }>()
const router = useRouter()
const resolved = computed(() => resolveError(props.code, props.message))

function onClick(btn: ErrorButton, isPrimary = false) {
  if (btn.action === 'retry') emit('retry')
  else if (btn.action === 'viewLog') emit('viewLog')
  else if (btn.action === 'route' && btn.to) router.push(btn.to)
  if (isPrimary) emit('primary')
}
</script>

<style scoped>
/* 遮罩不跟随主题：播放器区域始终是深色 */
.ff-err-overlay {
  position: absolute;
  inset: 0;
  z-index: 4;
  display: grid;
  place-items: center;
  background: rgba(11, 12, 14, 0.85); /* 原型 0.72，设计评审要求 ≥ 0.85，避免亮画面透出影响可读性 */
  backdrop-filter: blur(4px);
}
.box {
  width: 360px;
  max-width: 100%;
  text-align: center;
  color: #e8eaed;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--ff-space-2);
}
.eic {
  width: 44px;
  height: 44px;
  border-radius: 50%;
  background: rgba(239, 68, 68, 0.16);
  color: #ef4444;
  display: grid;
  place-items: center;
  margin-bottom: var(--ff-space-1);
}
h5 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.5;
}
p {
  margin: 0;
  font-size: 12.5px;
  color: #a0a6b0;
  line-height: 1.6;
  /* 最多两行，超出用省略号 */
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.code {
  display: block;
  font-family: var(--ff-font-mono);
  font-size: 11px;
  line-height: 1.75; /* 原型里 .code 继承了 1.75 的行高，保持一致 */
  color: #8b919b; /* 原型 #6b717b，设计评审调亮，亮画面上也能读 */
}
.acts {
  display: flex;
  gap: var(--ff-space-2);
  margin-top: var(--ff-space-2);
}
.btn {
  height: 28px;
  padding: 0 var(--ff-space-3);
  border-radius: var(--ff-radius-md);
  border: 1px solid #33363d;
  background: #26282e;
  color: #e8eaed;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font: inherit;
  font-size: var(--ff-fs-sm);
  white-space: nowrap;
  cursor: pointer;
  transition: background var(--ff-dur-fast) var(--ff-ease);
}
.btn:hover {
  background: #2f3239;
}
.btn.pri {
  background: var(--ff-primary);
  border-color: var(--ff-primary);
  color: #fff;
}
.btn.pri:hover {
  background: var(--ff-primary-hover);
  border-color: var(--ff-primary-hover);
}
/* 暗色主题的主色偏亮，白字对比度只有 3.16:1，改用近黑字（设计评审） */
:global(html.dark) .btn.pri,
:global(.ff-dark) .btn.pri {
  color: #0b0c0e;
}
.btn:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
</style>
