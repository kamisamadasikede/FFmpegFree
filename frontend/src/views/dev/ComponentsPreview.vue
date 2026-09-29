<template>
  <div class="dev">
    <h1>公共组件预览（仅开发环境）</h1>
    <p class="hint">?only=light 或 ?only=dark 只看一个主题；默认每个主题区块各渲染一遍，用于截图对照 proto/errors.html。</p>
    <p class="hint">自检：<b :style="{ color: checks.length ? 'var(--ff-danger)' : 'var(--ff-success)' }">{{ checks.length ? checks.join('；') : '错误码与 playerError 自检全部通过' }}</b></p>

    <section v-for="theme in shownThemes" :key="theme" :class="['block', theme === 'dark' ? 'ff-dark' : 'ff-light']">
      <h2>{{ theme === 'dark' ? '深色' : '浅色' }}</h2>
      <div class="grid8">
        <div v-for="code in overlayCodes" :key="code" class="pv">
          <ErrorOverlay :code="code" @retry="log('retry', code)" @view-log="log('viewLog', code)" @primary="log('primary', code)" />
        </div>
        <div class="pv surface">
          <div class="lab">推流地址</div>
          <div class="input bad ff-input-bad">http:/live.example</div>
          <InlineError code="LIVE_URL_INVALID" />
          <div class="tag">LIVE_URL_INVALID · 行内</div>
        </div>
        <div class="pv surface span2">
          <div class="lab3">任务中心 · 失败行（推流中断 / 连接失败）</div>
          <div class="row">
            <div class="fmeta"><div class="fname">B站直播间推流</div><div class="finfo">录屏推流 · 1080p30 · 已推流 00:42:18</div></div>
            <span class="tagfail">失败</span>
            <button class="rbtn"><FIcon name="retry" :size="15" />重试</button>
          </div>
          <ErrorLine code="LIVE_PUSH_INTERRUPTED" />
          <ErrorLine code="INTERNAL" message="ffmpeg exited with code 1" />
        </div>
      </div>

      <h3>PlayerShell（编辑页 / 直播页 / 错误态 / 空壳）</h3>
      <div class="players">
        <PlayerShell v-model:playing="playing" v-model:muted="muted" v-model:current="current" v-model:duration="duration" chip="1.0×">
          <div class="demo"><div class="sun"></div><div class="m1"></div><div class="m2"></div><div class="cap">傍晚的山，像被点燃了一样</div></div>
        </PlayerShell>
        <PlayerShell mode="status" status-text="正在推流" status-hint="主显示器 · 摄像头画中画" chip="麦克风 开">
          <div class="demo live"></div>
          <template #overlay><ErrorOverlay code="LIVE_PUSH_REJECTED" detail="服务器返回 403" /></template>
        </PlayerShell>
        <PlayerShell :current="0" :duration="0" />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import FIcon from '../../components/icon/FIcon.vue'
import ErrorOverlay from '../../components/common/ErrorOverlay.vue'
import InlineError from '../../components/common/InlineError.vue'
import ErrorLine from '../../components/common/ErrorLine.vue'
import PlayerShell from '../../components/common/PlayerShell.vue'
import { runErrorChecks } from '../../errors/playerError.check'

// 临时页面：路由只在 import.meta.env.DEV 下注册，正式包里没有
const themes = ['light', 'dark'] as const
// ?only=light|dark 只渲染一个主题（截图用）；hash 路由下参数在 hash 里
const only = new URLSearchParams(location.hash.split('?')[1] || location.search).get('only')
const shownThemes = themes.filter((t) => !only || t === only)
const overlayCodes = [
  'LIVE_CONNECT_FAILED',
  'LIVE_PUSH_REJECTED',
  'LIVE_PLAY_FAILED',
  'LIVE_CORS_BLOCKED',
  'LIVE_PUSH_INTERRUPTED',
  'FFMPEG_NOT_FOUND',
  'SCREEN_PERMISSION_DENIED',
]
const playing = ref(false)
const muted = ref(false)
const current = ref(72.32)
const duration = ref(225)
const checks = runErrorChecks()
function log(ev: string, code: string) {
  console.info('[dev/components]', ev, code)
}
</script>

<style scoped>
.dev {
  position: fixed; /* 盖住应用外壳，方便和原型整页对照 */
  inset: 0;
  z-index: 100;
  padding: 32px;
  background: var(--ff-bg-app);
  color: var(--ff-text-1);
  min-height: 100%;
  overflow: auto;
  user-select: text;
}
h1 { font-size: 18px; margin: 0 0 4px; }
.hint { color: var(--ff-text-2); margin: 0 0 8px; }
.block { background: var(--ff-bg-app); color: var(--ff-text-1); padding: 24px; margin: 0 -24px; }
h2 { font-size: 15px; margin: 0 0 12px; }
h3 { font-size: 13px; margin: 24px 0 12px; color: var(--ff-text-2); }
.grid8 { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; }
.pv {
  height: 230px;
  border-radius: 10px;
  background: #0b0c0e;
  position: relative;
  overflow: hidden;
  border: 1px solid var(--ff-border);
}
.pv.surface {
  background: var(--ff-bg-surface);
  padding: 24px;
  display: flex;
  flex-direction: column;
  justify-content: center;
}
.pv.span2 { grid-column: span 2; padding: 20px; gap: 10px; }
.lab { font-size: 12px; color: var(--ff-text-2); margin-bottom: 6px; }
.lab3 { font-size: 12px; color: var(--ff-text-3); }
.tag { font-size: 11px; color: var(--ff-text-3); margin-top: 14px; font-family: var(--ff-font-mono); }
.input {
  height: 28px;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  background: var(--ff-bg-surface);
  font-size: 12px;
  font-family: var(--ff-font-mono);
}
.row { display: flex; align-items: center; gap: 12px; padding: 10px 8px; border-radius: 8px; border: 1px solid var(--ff-border); }
.fmeta { flex: 1; min-width: 0; }
.fname { font-weight: 500; }
.finfo { font-size: 12px; color: var(--ff-text-3); }
.tagfail { height: 20px; padding: 0 7px; border-radius: 4px; font-size: 12px; display: inline-flex; align-items: center; background: color-mix(in srgb, var(--ff-danger) 14%, transparent); color: var(--ff-danger); }
.rbtn { height: 28px; padding: 0 12px; border-radius: 6px; border: 1px solid var(--ff-border); background: var(--ff-bg-surface); color: var(--ff-text-1); display: inline-flex; align-items: center; gap: 6px; font: inherit; font-size: 13px; }
.players { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; align-items: start; }
/* 以下沿用原型 .frame 的日落画面，只是占位 */
.demo { position: absolute; inset: 0; background: linear-gradient(180deg, #1e3a8a 0%, #7c3aed 38%, #f97316 72%, #fdba74 100%); overflow: hidden; }
.demo.live { background: linear-gradient(135deg, #0f172a, #1e293b); }
.demo .sun { position: absolute; left: 58%; top: 46%; width: 22%; aspect-ratio: 1; border-radius: 50%; background: radial-gradient(circle, #fff7d6 0%, #fde68a 45%, rgba(253, 230, 138, 0) 70%); }
.demo .m1 { position: absolute; left: -5%; right: -5%; bottom: 0; height: 46%; background: #1e1b4b; clip-path: polygon(0 60%, 14% 30%, 26% 52%, 40% 18%, 55% 50%, 68% 28%, 82% 55%, 100% 35%, 100% 100%, 0 100%); }
.demo .m2 { position: absolute; left: 0; right: 0; bottom: 0; height: 26%; background: #0f0e2a; clip-path: polygon(0 50%, 20% 20%, 36% 55%, 52% 30%, 70% 60%, 88% 25%, 100% 45%, 100% 100%, 0 100%); }
.demo .cap { position: absolute; left: 0; right: 0; bottom: 9%; text-align: center; color: #fff; font-size: 15px; font-weight: 600; text-shadow: 0 1px 4px rgba(0, 0, 0, 0.6); }
</style>
