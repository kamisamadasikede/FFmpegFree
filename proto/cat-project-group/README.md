# Cat 项目群聊 · 高保真原型 v0.2.2

对齐契约 `docs/architecture/contract.md` 6.21.12–6.21.16；设计说明见 `../Cat-项目群聊-设计说明-v0.2.2.md`。
10-17 本地 /workspace 被清后重建，内容与产品已 PASS 的 v0.2.1 一致。
v0.2.2（产品 10-17）：补排队文案「排队中 · 前面还有 {n} 个」「交给 {成员}：正在干活，排在后面」；members、dispatch 两场景已重拍。正式图标见 `icons/`。

- 打开：`index.html?s=<场景>&theme=dark`（直接用浏览器打开文件即可，样式引用 `frontend/src/styles/tokens.css` + `cat-tokens.css`）
- 场景：list create rolegen main members mention working report error dispatch swap overstep overstep-undo overstep-result session settings narrow-drawer narrow-single
- 截图：`node shot.mjs [场景...]`（需要 `playwright-core`，默认用 `/usr/bin/google-chrome`，可设 `CHROME=` / `PW_PATH=`），2x，浅暗两版输出到 `shots/<场景>-{light,dark}.png`
- 窄窗：narrow-drawer 1000px（右栏变抽屉），narrow-single 720px（单栏）
