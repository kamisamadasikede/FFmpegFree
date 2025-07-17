<script setup>
import { ref, watch } from 'vue'
import { VuePDF, usePDF } from '@tato30/vue-pdf'
import { ElMessage } from 'element-plus'

// 当前页码
const page = ref(1)
//懒加载页码
const lazyPage = ref(5)
// PDF 地址输入框
const pdfUrl = ref('http://localhost:19200/public/pdf/ppt1.pdf')

// 实际用于加载的地址（初始为空）
const loadUrl = ref('')

// 使用 usePDF 加载 PDF（初始为空地址，不加载）
const { pdf, pages } = usePDF(loadUrl)

// 上一页
const prevPage = () => {
  if (page.value > 1) page.value--
}

// 下一页
const nextPage = () => {
  console.log(page.value, lazyPage.value);

  if(lazyPage.value - page.value < 5 && pages.value - lazyPage.value  > 0) {
     if  (pages.value - lazyPage.value  < 10){
          lazyPage.value += pages.value - lazyPage.value
     } else {
          lazyPage.value += 10
     }
     lazyPage.value = Math.min(lazyPage.value, pages.value)
   }
  if (page.value < pages.value) page.value++

}

// 点击缩略图时加载更多页面
const handleThumbnailClick = (pageNum) => {
  page.value = pageNum
  if(lazyPage.value - pageNum < 5 && pages.value - lazyPage.value  > 0) {
     if  (pages.value - lazyPage.value  < 10){
          lazyPage.value += pages.value - lazyPage.value
     } else {
          lazyPage.value += 10
     }
     lazyPage.value = Math.min(lazyPage.value, pages.value)
   }
}

// 左侧缩略图容器 ref
const thumbContainer = ref(null)

// 加载新 PDF
const loadNewPDF = () => {
  if (!pdfUrl.value) {
    ElMessage.warning('请输入 PDF 地址')
    return
  }

  if (!pdfUrl.value.toLowerCase().endsWith('.pdf')) {
    ElMessage.error('请输入有效的 PDF 文件地址（以 .pdf 结尾）')
    return
  }
  console.log(pages.value);
  
  // 设置 loadUrl，触发 usePDF 加载
  loadUrl.value = pdfUrl.value
  page.value = 1
}

// 监听 page 变化，自动滚动到当前缩略图
watch(page, (newPage) => {
  if (!thumbContainer.value) return

  // 获取所有缩略图项
  const items = thumbContainer.value.querySelectorAll('.thumbnail-item')

  if (items.length >= newPage) {
    const currentEl = items[newPage - 1]
    currentEl.scrollIntoView({
      behavior: 'smooth',
      block: 'center' // 居中显示
    })
  }
})
</script>

<template>
  <!-- 输入框区域 -->
  <div style="text-align: center; padding: 20px;">
    <el-input
      v-model="pdfUrl"
      placeholder="请输入 PDF 文件的网络地址"
      style="width: 600px; margin-right: 10px;"
    />
    <el-button type="primary" @click="loadNewPDF">加载 PDF</el-button>
      <!-- PDF 查看器主体 -->
  <div class="pdf-viewer">
    <!-- 左侧：缩略图列表 -->
    <el-aside class="thumbnail-list" width="210px">
      <div
        v-for="pageNum in lazyPage"
        :key="pageNum"
        class="thumbnail-item"
        :class="{ active: pageNum === page }"
        @click="handleThumbnailClick(pageNum)"
      >
        <VuePDF fit-parent :pdf="pdf" :page="pageNum" :zoom="0.3" />
      </div>
    </el-aside>

    <!-- 右侧：当前页放大显示 -->
    <div class="main-preview">
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
    </div>
  </div>
  </div>
</template>

<style scoped>
.pdf-viewer {
  display: flex;
  margin: 2rem auto;
  max-width: 2000px;
  font-family: 'Segoe UI', sans-serif;
  background-color: #f9f9f9;
  border-radius: 10px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.1);
  overflow: hidden;
}

.thumbnail-list {
  padding: 10px;
  border-right: 1px solid #ddd;
  height: 80vh;
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
  width: 1240;
  max-width: 1240;
  min-width: 1240;
  position: relative;
  flex: 1;
  background-color: white;
}

.nav-buttons {
  position: absolute;
  top: 50%;
  left: 0;
  right: 0;
  transform: translateY(-50%);
  display: flex;
  justify-content: space-between;
  pointer-events: none; /* 避免遮挡内容点击 */
}

.nav-buttons .el-button {
  pointer-events: auto;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1);
}
</style>