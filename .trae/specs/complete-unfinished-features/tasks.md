# Tasks
- [x] Task 1: 完成 About.vue 页面内容：实现项目介绍页面，展示项目名称、版本、技术栈、功能特性等信息
  - [x] SubTask 1.1: 设计 About 页面布局和内容结构
  - [x] SubTask 1.2: 实现项目信息展示组件
  - [x] SubTask 1.3: 添加样式美化页面
- [x] Task 2: 实现 Office 文件转 PDF 功能后端接口：创建后端控制器处理 office 文件上传和转换
  - [x] SubTask 2.1: 创建 OfficeController 处理文件上传
  - [x] SubTask 2.2: 实现 Word 转 PDF 功能（使用 libreoffice 或 unoconv）
  - [x] SubTask 2.3: 实现 Excel 转 PDF 功能
  - [x] SubTask 2.4: 实现 PowerPoint 转 PDF 功能
  - [x] SubTask 2.5: 添加 DTO 注释声明
  - [x] SubTask 2.6: 注册路由
- [x] Task 3: 实现 Office 文件转 PDF 功能前端页面：创建前端转换页面和 API 调用
  - [x] SubTask 3.1: 创建 OfficeConvert.vue 页面
  - [x] SubTask 3.2: 实现文件上传组件
  - [x] SubTask 3.3: 实现转换进度显示
  - [x] SubTask 3.4: 添加路由配置
  - [x] SubTask 3.5: 创建前端 API 调用

# Task Dependencies
- [Task 2] 依赖 [Task 1]: 否（并行执行）
- [Task 3] 依赖 [Task 2]: 是（需要后端接口）
