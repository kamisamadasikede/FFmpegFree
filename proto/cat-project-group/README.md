# Cat 项目群聊 · 高保真原型 v0.2.4

对齐契约 `docs/architecture/contract.md` 6.21.12–6.21.16；设计说明见 `../Cat-项目群聊-设计说明-v0.2.4.md`。
v0.2.2（产品 10-17）：排队文案「排队中 · 前面还有 {n} 个」「交给 {成员}：正在干活，排在后面」。
v0.2.3（产品 10-17 定 A）：判断失败 / 超时 / 定不出时不起成员、不交给排第一；灰字「没判断出该谁接，请 @ 对应成员，或再说具体要做什么。」（可带判断用时）；该行无「换人」。dispatch 浅暗已重拍。正式图标见 `icons/`。
v0.2.4（产品 10-17 v0.56，对标 Grok Bot）：并行默认；「交给谁」可空；写了 @ 仍唤醒；灰字「已请 {成员} 接着」（消息流不写第 n/6）；空 give 不强写「没有交给下一位」；招呼/澄清走短复。

- 打开：`index.html?s=<场景>&theme=dark`（直接用浏览器打开文件即可，样式引用 `frontend/src/styles/tokens.css` + `cat-tokens.css`）
- 场景：list create rolegen main members mention working report error dispatch swap overstep overstep-undo overstep-result session settings narrow-drawer narrow-single
- 截图：`node shot.mjs [场景...]`（需要 `playwright-core`，默认用 `/usr/bin/google-chrome`，可设 `CHROME=` / `PW_PATH=`），2x，浅暗两版输出到 `shots/<场景>-{light,dark}.png`
- 窄窗：narrow-drawer 1000px（右栏变抽屉），narrow-single 720px（单栏）
