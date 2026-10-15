# ffc

产品名始终使用小写 `ffc`。Logo 用圆角笔画绘制三个字母，两个 `f` 共用一条横笔，暖橙色 `c` 呼应猫追蝴蝶动画里的蝴蝶。

项目助手在界面中统一称为 `Agent`，覆盖菜单、页面标题、设置、更新提醒和系统通知。

## 资源

- `frontend/src/assets/brand/ffc-icon.svg`：应用图标矢量源，深石墨色底、米白色 `ff`、杏橙色 `c`。
- `frontend/src/assets/brand/ffc-wordmark.svg`：透明背景字标，用于浅色背景。
- `frontend/src/components/brand/AppLogo.vue`：侧栏、关于页、登录页统一使用的图标组件。
- `appicon.png`、`build/appicon.png`：1024 × 1024 PNG 图标。
- `icon.ico`、`build/windows/icon.ico`：包含 16、20、24、32、40、48、64、128、256 像素图层的 Windows 图标。

主色：石墨色 `#24252B`、米白色 `#F8F2E9`、杏橙色 `#F3AE79`。浅色背景字标中的 `c` 使用深橙色 `#CD682F`。

## 命名兼容

界面与安装包使用 `ffc`；本地配置目录、数据库、组件缓存目录和 Go 模块路径保留 `FFmpegFree`，继续读取已有设置、任务和对话。项目仓库 URL 沿用实际地址。

Wails 使用的 `build/appicon.png` 和 `build/windows/` 图标、元数据模板纳入版本管理，确保新检出项目后直接构建也使用 ffc 品牌。Windows 输出为 `build/bin/ffc.exe`。
