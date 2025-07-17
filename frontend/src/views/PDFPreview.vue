<script setup>
import { ref } from 'vue'
import { VuePDF, usePDF } from '@tato30/vue-pdf'
import { ElMessage } from 'element-plus'

const page = ref(1)
const pdfUrl = ref('http://localhost:19200/public/pdf/ppt1.pdf')

// 加载 PDF 的逻辑
const { pdf, pages } = usePDF(pdfUrl)

const prevPage = () => {
  if (page.value > 1) page.value--
}

const nextPage = () => {
  if (page.value < pages.value) page.value++
}

// 加载新 PDF 的方法
function loadNewPDF() {
  if (!pdfUrl.value) {
    ElMessage.warning('请输入 PDF 地址')
    return
  }

  if (!pdfUrl.value.toLowerCase().endsWith('.pdf')) {
    ElMessage.error('请输入有效的 PDF 文件地址（以 .pdf 结尾）')
    return
  }

  // 重新加载 PDF（Vue 会自动响应 ref 的变化）
  // 不需要手动赋值 pdf，usePDF 会自动处理
}
</script>

<template>
  <div class="pdf-viewer">
    <!-- 左侧：缩略图列表 -->
    <el-aside class="thumbnail-list" width="210px">
      <div
        v-for="pageNum in pages"
        :key="pageNum"
        class="thumbnail-item"
        :class="{ active: pageNum === page }"
        @click="page = pageNum"
      >
        <VuePDF fit-parent :pdf="pdf" :page="pageNum" :zoom="0.3" />
      </div>
    </el-aside>

    <!-- 右侧：当前页放大显示 -->
    <el-main class="main-preview">
      <VuePDF fit-parent :pdf="pdf" :page="page" />

      <!-- 悬浮按钮 -->
      <div class="nav-buttons">
        <el-button circle @click="prevPage" :disabled="page <= 1">
          <el-icon><ArrowLeft /></el-icon>
        </el-button>
        <el-button circle @click="nextPage" :disabled="page >= pages">
          <el-icon><ArrowRight /></el-icon>
        </el-button>
      </div>
    </el-main>
  </div>
</template>

<style scoped>
.pdf-viewer {
  display: flex;
  margin: 2rem auto;
  max-width: px;
  font-family: 'Segoe UI', sans-serif;
  background-color: #f9f9f9;
  border-radius: 10px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.1);
  overflow: hidden;
}

.thumbnail-list {
  padding: 10px;
  border-right: 1px solid #ddd;
  height: 90vh;
  overflow-y: auto;
  background-color: #fff;
}

.thumbnail-item {
  margin: 4px 0;
  cursor: pointer;
  border: 2px solid transparent;
  transition: border-color 0.3s ease;
}

.thumbnail-item.active {
  border-color: #409eff;
  background-color: #e9ecef;
  border-radius: 6px;
}

.main-preview {
  width: 1190 px;
  position: relative;
  flex: 1;
  padding: 10px;
  background-color: white;
}


</style>