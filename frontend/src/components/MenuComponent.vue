<template>
  <div class="menu-shell">
    <div class="menu-brand">
      <div class="brand-mark">FF</div>
      <div>
        <div class="brand-title">FFmpegFree</div>
        <div class="brand-subtitle">闊宠棰戝伐鍏风</div>
      </div>
    </div>

    <el-menu
      :default-active="$route.path"
      class="app-menu"
      router
      @open="handleOpen"
      @close="handleClose"
    >
      <el-sub-menu index="1">
        <template #title>
          <el-icon><Camera /></el-icon>
          <span>瑙嗛</span>
        </template>
        <el-menu-item route="/" index="/">杞崲</el-menu-item>
        <el-menu-item route="/convert" index="/convert">姝ｅ湪杞崲涓?/el-menu-item>
        <el-menu-item route="/convertup" index="/convertup">宸插畬鎴?/el-menu-item>
      </el-sub-menu>

      <el-sub-menu index="2">
        <template #title>
          <el-icon><VideoCamera /></el-icon>
          <span>娴佸獟浣撳伐鍏?/span>
        </template>
        <el-menu-item route="/steamup" index="/steamup">鎺ㄦ祦</el-menu-item>
        <el-menu-item route="/steamlist" index="/steamlist">姝ｅ湪鎺ㄦ祦涓?/el-menu-item>
        <el-menu-item route="/live-ops" index="/live-ops">直播运维面板</el-menu-item>
        <el-menu-item route="/MediaRecorder" index="/MediaRecorder">灞忓箷褰曞埗鎺ㄦ祦</el-menu-item>
        <el-menu-item route="/LivePlayer" index="/LivePlayer">鐩存挱鎷夋祦锛團LV锛?/el-menu-item>
      </el-sub-menu>

      <el-sub-menu index="3">
        <template #title>
          <el-icon><MessageBox /></el-icon>
          <span>鏂囨。杞崲</span>
        </template>
        <el-menu-item route="/pdf-preview" index="/pdf-preview">PDF 棰勮</el-menu-item>
        <el-menu-item route="/office-convert" index="/office-convert">Office 杞?PDF</el-menu-item>
      </el-sub-menu>

      <el-sub-menu index="4">
        <template #title>
          <el-icon><Tools /></el-icon>
          <span>宸ュ叿</span>
        </template>
        <el-menu-item route="/json-tools" index="/json-tools">JSON 宸ュ叿</el-menu-item>
        <el-menu-item route="/openclaw-install" index="/openclaw-install">OpenClaw 涓€閿畨瑁?/el-menu-item>
      </el-sub-menu>

      <el-menu-item route="/about" index="/about">
        <el-icon><InfoFilled /></el-icon>
        <span>鍏充簬</span>
      </el-menu-item>
    </el-menu>
  </div>
</template>

<script lang="ts" setup>
const handleOpen = () => {}
const handleClose = () => {}
</script>

<style scoped>
.menu-shell {
  display: flex;
  flex-direction: column;
  gap: 8px;
  height: 100%;
}

.menu-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px;
  border-radius: 12px;
  background: linear-gradient(135deg, rgba(37, 99, 235, 0.14), rgba(37, 99, 235, 0.06));
  border: 1px solid rgba(37, 99, 235, 0.22);
  box-shadow: 0 4px 14px rgba(37, 99, 235, 0.14);
}

.brand-mark {
  width: 38px;
  height: 38px;
  border-radius: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #2563eb;
  color: #fff;
  font-weight: 700;
  letter-spacing: 0.5px;
}

.brand-title {
  font-weight: 700;
  font-size: 14px;
}

.brand-subtitle {
  color: #6b7280;
  font-size: 11px;
  margin-top: 2px;
}
</style>

