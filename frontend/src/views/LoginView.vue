<template>
  <div class="lg-app" data-testid="login-view">
    <aside class="lg-visual" aria-hidden="true">
      <div class="grid" />
      <i class="lg-orb a" /><i class="lg-orb b" /><i class="lg-orb c" />
      <div class="inner">
        <div class="mark">
          <div class="logo"><FIcon name="play" :size="20" :stroke="2.2" /></div>
          <span>FFmpegFree</span>
        </div>
        <h1 :class="staggerClass(0)">把格式转换<br />做成顺手的事</h1>
        <p class="lead" :class="staggerClass(1)" :aria-label="LEAD_FULL">
          <span>{{ typedLead }}</span><span v-if="typing" class="caret" aria-hidden="true" />
        </p>
        <div class="lg-points" :class="staggerClass(2)">
          <div class="pt"><span class="ic"><FIcon name="cpu" :size="14" /></span>本机转换，文件不上传</div>
          <div class="pt"><span class="ic"><FIcon name="refresh" :size="14" /></span>任务与设置云端同步</div>
          <div class="pt"><span class="ic"><FIcon name="shield" :size="14" /></span>账号数据加密保存</div>
        </div>
      </div>
    </aside>

    <section class="lg-panel" aria-label="登录">
      <button type="button" class="lg-theme" :aria-label="isDark ? '切换到浅色' : '切换到暗色'" @click="toggleTheme">
        <FIcon :name="isDark ? 'sun' : 'moon'" :size="16" />
      </button>
      <div class="lg-form-wrap">
        <div class="lg-form-hd">
          <h2>欢迎回来</h2>
          <p>登录你的账号，继续未完成的工作。</p>
        </div>
        <div v-if="alertText" class="lg-alert" role="alert">
          <FIcon name="warn" :size="16" />
          <span>{{ alertText }}</span>
        </div>
        <form class="lg-form" :class="{ 'is-busy': busy }" :aria-busy="busy" @submit.prevent="onSubmit">
          <div class="lg-field">
            <label for="lg-user">账号</label>
            <div class="box" :class="{ err: fieldErr }">
              <input
                id="lg-user"
                v-model="user"
                type="text"
                placeholder="邮箱或用户名"
                autocomplete="username"
                :disabled="busy"
              />
            </div>
          </div>
          <div class="lg-field">
            <label for="lg-pass">密码</label>
            <div class="box" :class="{ err: fieldErr }">
              <input
                id="lg-pass"
                v-model="pass"
                :type="showPass ? 'text' : 'password'"
                placeholder="请输入密码"
                autocomplete="current-password"
                :disabled="busy"
              />
              <button type="button" class="eye" :aria-label="showPass ? '隐藏密码' : '显示密码'" :disabled="busy" @click="showPass = !showPass">
                <FIcon :name="showPass ? 'eyeoff' : 'eye'" :size="16" />
              </button>
            </div>
          </div>
          <div class="lg-row">
            <button type="button" class="lg-chk" :class="{ on: remember }" role="checkbox" :aria-checked="remember" :disabled="busy" @click="remember = !remember">
              <FIcon v-if="remember" name="check" :size="12" />
            </button>
            记住我
            <span class="sp" />
            <button type="button" class="lg-link" :disabled="busy" @click="onSoon">忘记密码？</button>
          </div>
          <button type="submit" class="lg-submit" :aria-disabled="busy" ::aria-busy="busy" :disabled="busy">
            <i v-if="busy" class="spin" aria-hidden="true" /><span>{{ busy ? '登录中…' : '登录' }}</span>
          </button>
        </form>
        <div class="lg-foot">还没有账号？<button type="button" class="lg-link" @click="onSoon">注册</button></div>
      </div>
      <div v-if="toast" class="lg-toast" role="status">
        <FIcon name="info" :size="16" />
        <span>{{ toast }}</span>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
/**
 * 登录页 UI 预留（login-v1）。未接真登录 / 账号 / 后端。
 * 左栏：标题 → 副文案 → 卖点 stagger fade-up（220ms，间隔 100ms）；副文案一次性打字机。
 * prefers-reduced-motion / html.reduce-motion：直接终态。右侧表单立即可用，不等动画。
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { useTheme, type ThemeMode } from '@/composables/useTheme'
import { prefersReducedMotion } from '@/utils/motion'

const LEAD_FULL = '本机处理，结果跟你的账号走。登录后同步任务与设置，换一台电脑也能接着用。'
/** 与 proto/login-v1：每段 220ms，段间隔 100ms → 下一段在 STEP+GAP 后启动 */
const STEP_MS = 220
const GAP_MS = 100
const STAGE_MS = STEP_MS + GAP_MS // 320
const TYPE_MS = 28

const { mode, isDark } = useTheme()

const user = ref('')
const pass = ref('')
const remember = ref(true)
const showPass = ref(false)
const busy = ref(false)
const fieldErr = ref(false)
const alertText = ref('')
const toast = ref('')
let toastTimer: ReturnType<typeof setTimeout> | undefined

const reduced = prefersReducedMotion()
const reveal = ref(reduced ? 3 : 0)
const typedLead = ref(reduced ? LEAD_FULL : '')
const typing = ref(false)
const timers: ReturnType<typeof setTimeout>[] = []

function staggerClass(step: number) {
  return { 'lg-stagger': true, 'is-in': reveal.value > step }
}

function schedule(fn: () => void, ms: number) {
  timers.push(setTimeout(fn, ms))
}

function runLeftMotion() {
  if (reduced) {
    reveal.value = 3
    typedLead.value = LEAD_FULL
    typing.value = false
    return
  }
  // proto：标题立即 → 320ms 副文案+打字机 → 640ms 卖点（打字可与卖点并行）
  reveal.value = 1
  schedule(() => {
    reveal.value = 2
    typing.value = true
    let i = 0
    const tick = () => {
      if (i <= LEAD_FULL.length) {
        typedLead.value = LEAD_FULL.slice(0, i)
        i += 1
        schedule(tick, TYPE_MS)
      } else {
        typing.value = false
      }
    }
    tick()
  }, STAGE_MS)
  schedule(() => {
    reveal.value = 3
  }, STAGE_MS * 2)
}

onMounted(runLeftMotion)
onBeforeUnmount(() => {
  for (const t of timers) clearTimeout(t)
  if (toastTimer) clearTimeout(toastTimer)
})

function toggleTheme() {
  const next: ThemeMode = isDark.value ? 'light' : 'dark'
  mode.value = next
}

function showToast(msg: string) {
  toast.value = msg
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => {
    toast.value = ''
  }, 2400)
}

function onSoon() {
  if (busy.value) return
  showToast('功能即将开放')
}

function onSubmit() {
  if (busy.value) return
  alertText.value = ''
  fieldErr.value = false
  if (!user.value.trim() || !pass.value) {
    fieldErr.value = true
    alertText.value = '请填写账号和密码。'
    return
  }
  // 预留页：不接真登录，本地提示即可（不伪装网络请求）
  showToast('功能即将开放')
}

</script>

<style scoped>
.lg-app {
  display: flex;
  height: 100vh;
  min-height: 680px;
  min-width: 1024px;
  overflow: hidden;
  isolation: isolate;
}
.lg-visual {
  position: relative;
  flex: 0 0 55%;
  width: 55%;
  overflow: hidden;
  color: #e8eef8;
  background: linear-gradient(155deg, #0b1220 0%, #121a2e 42%, #0e1830 100%);
}
:global(html.dark) .lg-visual {
  background: linear-gradient(155deg, #060910 0%, #0a101c 50%, #081018 100%);
}
.lg-visual .grid {
  position: absolute;
  inset: 0;
  opacity: 0.28;
  background-image:
    linear-gradient(rgba(140, 170, 255, 0.1) 1px, transparent 1px),
    linear-gradient(90deg, rgba(140, 170, 255, 0.1) 1px, transparent 1px);
  background-size: 56px 56px;
  mask-image: radial-gradient(ellipse 80% 70% at 40% 40%, #000 10%, transparent 75%);
}
.lg-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(72px);
  pointer-events: none;
  will-change: transform, opacity;
}
.lg-orb.a {
  width: 480px;
  height: 480px;
  left: -120px;
  top: -80px;
  background: radial-gradient(circle, rgba(79, 124, 255, 0.55), transparent 70%);
  animation: lg-orb-a 18s ease-in-out infinite;
}
.lg-orb.b {
  width: 360px;
  height: 360px;
  right: -60px;
  bottom: 10%;
  background: radial-gradient(circle, rgba(167, 120, 255, 0.4), transparent 70%);
  animation: lg-orb-b 16s ease-in-out infinite;
}
.lg-orb.c {
  width: 420px;
  height: 420px;
  left: 30%;
  bottom: -140px;
  background: radial-gradient(circle, rgba(34, 211, 238, 0.22), transparent 70%);
  animation: lg-orb-c 20s ease-in-out infinite;
}
@keyframes lg-orb-a {
  0%,
  100% {
    transform: translate(0, 0) scale(1);
  }
  50% {
    transform: translate(48px, 36px) scale(1.06);
  }
}
@keyframes lg-orb-b {
  0%,
  100% {
    transform: translate(0, 0) scale(1);
  }
  50% {
    transform: translate(-40px, -28px) scale(1.1);
  }
}
@keyframes lg-orb-c {
  0%,
  100% {
    transform: translate(0, 0);
  }
  50% {
    transform: translate(32px, -40px);
  }
}
.lg-visual .inner {
  position: relative;
  z-index: 1;
  height: 100%;
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 64px 56px;
  gap: 28px;
}
.lg-visual .mark {
  display: flex;
  align-items: center;
  gap: 12px;
}
.lg-visual .logo {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: linear-gradient(145deg, #5b8cff, #7b6cff);
  display: grid;
  place-items: center;
  color: #fff;
  box-shadow:
    0 0 0 1px rgba(255, 255, 255, 0.18) inset,
    0 10px 28px rgba(79, 124, 255, 0.45);
}
.lg-visual .mark span {
  font-size: 14px;
  font-weight: 500;
  letter-spacing: 0.04em;
  color: rgba(232, 238, 248, 0.72);
}
.lg-visual h1 {
  font-size: 40px;
  font-weight: 650;
  letter-spacing: -0.02em;
  line-height: 1.15;
  color: #f4f7fc;
  margin: 0;
  max-width: 420px;
}
.lg-visual .lead {
  font-size: 15px;
  line-height: 1.65;
  color: rgba(200, 210, 230, 0.72);
  max-width: 380px;
  margin: 0;
  min-height: 4.95em;
}
.lg-visual .lead .caret {
  display: inline-block;
  width: 1px;
  height: 1em;
  margin-left: 1px;
  vertical-align: -2px;
  background: rgba(200, 210, 230, 0.7);
  animation: lg-caret 0.9s steps(1) infinite;
}
@keyframes lg-caret {
  50% {
    opacity: 0;
  }
}
.lg-points {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 8px;
}
.lg-points .pt {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: rgba(210, 220, 240, 0.78);
}
.lg-points .ic {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: grid;
  place-items: center;
  flex: none;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: rgba(180, 200, 255, 0.95);
}

/* stagger fade-up：220ms，间隔由 JS 控制 is-in */
.lg-stagger {
  opacity: 0;
  translate: 0 10px;
  transition:
    opacity 220ms var(--ff-ease-out, cubic-bezier(0.16, 1, 0.3, 1)),
    translate 220ms var(--ff-ease-out, cubic-bezier(0.16, 1, 0.3, 1));
}
.lg-stagger.is-in {
  opacity: 1;
  translate: 0 0;
}

.lg-panel {
  flex: 1;
  min-width: 0;
  display: grid;
  place-items: center;
  padding: 48px 56px;
  background: var(--ff-bg-app);
  position: relative;
}
:global(html.dark) .lg-panel {
  background: #0e131c;
}
.lg-panel::before {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: radial-gradient(ellipse 80% 60% at 50% 40%, color-mix(in srgb, var(--ff-primary) 6%, transparent), transparent 70%);
}
:global(html.dark) .lg-panel::before {
  background: radial-gradient(ellipse 70% 50% at 50% 35%, rgba(79, 124, 255, 0.07), transparent 70%);
}
.lg-form-wrap {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 360px;
  display: flex;
  flex-direction: column;
  gap: 28px;
}
.lg-form-hd h2 {
  font-size: 24px;
  font-weight: 650;
  letter-spacing: -0.01em;
  color: var(--ff-text-1);
  margin: 0 0 6px;
}
.lg-form-hd p {
  font-size: 13px;
  color: var(--ff-text-2);
  margin: 0;
  line-height: 1.5;
}
.lg-form {
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.lg-field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.lg-field label {
  font-size: 13px;
  font-weight: 500;
  color: var(--ff-text-1);
}
.lg-field .box {
  height: 42px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  border-radius: 10px;
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  transition:
    border-color 0.15s,
    box-shadow 0.15s;
}
.lg-field .box:hover {
  border-color: color-mix(in srgb, var(--ff-primary) 30%, var(--ff-border));
}
.lg-field .box:focus-within {
  border-color: var(--ff-primary);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--ff-primary) 18%, transparent);
}
.lg-field .box.err {
  border-color: var(--ff-danger);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--ff-danger) 14%, transparent);
}
:global(html.dark) .lg-field .box {
  background: #151b26;
  border-color: #252c3a;
}
.lg-field .box input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  font-size: 14px;
  color: var(--ff-text-1);
  font-family: inherit;
}
.lg-field .box input::placeholder {
  color: var(--ff-text-3);
}
.lg-field .box .eye {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  display: grid;
  place-items: center;
  color: var(--ff-text-2);
  flex: none;
  border: 0;
  background: transparent;
  padding: 0;
  cursor: pointer;
}
.lg-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--ff-text-1);
}
.lg-row .sp {
  flex: 1;
}
.lg-chk {
  width: 16px;
  height: 16px;
  border-radius: 4px;
  border: 1.5px solid var(--ff-border);
  display: grid;
  place-items: center;
  flex: none;
  background: var(--ff-bg-surface);
  padding: 0;
  cursor: pointer;
  color: #fff;
}
.lg-chk.on {
  background: linear-gradient(145deg, #4f7cff, #6b5cff);
  border-color: transparent;
}
.lg-link {
  font-size: 13px;
  color: var(--ff-primary-text);
  background: none;
  border: 0;
  padding: 0;
  cursor: pointer;
  font-family: inherit;
}
.lg-link:disabled {
  opacity: 0.55;
  pointer-events: none;
}
.lg-submit {
  height: 44px;
  width: 100%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 560;
  border: 0;
  border-radius: 10px;
  margin-top: 4px;
  background: linear-gradient(145deg, #4f7cff 0%, #5b6bff 55%, #6b5cff 100%);
  color: #fff;
  box-shadow:
    0 10px 24px rgba(79, 124, 255, 0.28),
    inset 0 1px 0 rgba(255, 255, 255, 0.25);
  letter-spacing: 0.01em;
  cursor: pointer;
  font-family: inherit;
}
.lg-submit .spin {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.35);
  border-top-color: #fff;
  border-radius: 50%;
  animation: lg-spin 0.75s linear infinite;
  margin-right: 8px;
}
@keyframes lg-spin {
  to {
    transform: rotate(360deg);
  }
}
.lg-form.is-busy .box,
.lg-form.is-busy .lg-chk,
.lg-form.is-busy .lg-link {
  opacity: 0.55;
  pointer-events: none;
}
.lg-submit:disabled {
  opacity: 0.55;
  cursor: default;
}
.lg-foot {
  font-size: 13px;
  color: var(--ff-text-2);
  text-align: center;
}
.lg-alert {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 10px;
  font-size: 13px;
  line-height: 1.45;
  border: 1px solid color-mix(in srgb, var(--ff-danger) 28%, var(--ff-border));
  background: color-mix(in srgb, var(--ff-danger) 8%, var(--ff-bg-surface));
  color: var(--ff-danger-text);
}
.lg-toast {
  position: absolute;
  left: 50%;
  bottom: 36px;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  border-radius: 10px;
  font-size: 13px;
  background: var(--ff-bg-elevated);
  border: 1px solid var(--ff-border);
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.12);
  z-index: 5;
  white-space: nowrap;
  color: var(--ff-text-1);
}
.lg-toast :deep(.f-icon) {
  color: var(--ff-text-2);
}
.lg-theme {
  position: absolute;
  top: 20px;
  right: 20px;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  color: var(--ff-text-2);
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  z-index: 2;
  cursor: pointer;
  padding: 0;
}
:global(html.dark) .lg-theme {
  background: #151b26;
  border-color: #252c3a;
}

@media (prefers-reduced-motion: reduce) {
  .lg-orb {
    animation: none !important;
  }
  .lg-stagger {
    transition: none !important;
    opacity: 1;
    translate: 0 0;
  }
  .lg-submit .spin,
  .lg-visual .lead .caret {
    animation: none !important;
  }
}
:global(html.reduce-motion) .lg-orb {
  animation: none !important;
}
:global(html.reduce-motion) .lg-stagger {
  transition: none !important;
  opacity: 1;
  translate: 0 0;
}
:global(html.reduce-motion) .lg-submit .spin,
:global(html.reduce-motion) .lg-visual .lead .caret {
  animation: none !important;
}
</style>
