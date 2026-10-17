# 项目群聊 · 正式图标（群 / 换人 / 分派）

替换前端 `fe/cat-groupchat-v055` 里的临时图标：

| 文件 | 建议 `icons.ts` key | 含义 | 替换的临时图标 | 用在哪 |
|---|---|---|---|---|
| `group.svg` | `group` | 多人（users） | `chat` | 左栏群项（`--cat-orange`，14–16px）、群空状态插画（30px）、群顶栏标题前 |
| `swap.svg` | `swap` | 人像 + 循环箭头 = 换人 | `convert` | 派活灰字行尾「换人」按钮（12–14px，`--cat-accent`）、「已换人：{原} → {新}」行首 |
| `dispatch.svg` | `dispatch` | 转交 / 分叉箭头 = 派活 | `branch` | 派活灰字行首（「交给 {成员}：…」「已请 {成员} 接着」，12–14px，继承 `--ff-text-2`） |

## 规格
- `viewBox="0 0 24 24"`，`fill="none"`，`stroke="currentColor"`，`stroke-width="1.75"`，`stroke-linecap="round"`，`stroke-linejoin="round"`。
- 颜色一律继承 `currentColor`，SVG 里不写色值；群项橙色由父元素 `color: var(--cat-orange)` 给。
- 推荐尺寸 12 / 14 / 16 / 20px，最小 12px（灰字行内）。预览见 `icons-preview.png`（16 / 20px，浅暗，与现有 `stop` `refresh` `warn` `info` `edit` `plus` 及三个临时图标并排）。

## 接入 FIcon
在 `frontend/src/components/icon/icons.ts` 里加三项（只放 `<svg>` 里面的元素）：

```ts
'group': '<circle cx="9" cy="8" r="3.5"/><path d="M3 20v-.5A5.5 5.5 0 0 1 8.5 14h1A5.5 5.5 0 0 1 15 19.5v.5"/><path d="M15.5 4.6a3.5 3.5 0 0 1 0 6.8"/><path d="M18 14.3a5.5 5.5 0 0 1 3 4.9v.8"/>',
'swap': '<circle cx="8.5" cy="7.5" r="3.5"/><path d="M2.5 20a6 6 0 0 1 9.5-4.9"/><path d="M14 15.5a3.5 3.5 0 0 1 6.2-1.9"/><path d="M20.5 11.5v2.4h-2.4"/><path d="M21 17.5a3.5 3.5 0 0 1-6.2 1.9"/><path d="M14.5 21.5v-2.4h2.4"/>',
'dispatch': '<path d="M3 12h5"/><path d="M8 12c3.5 0 4.5-6 8.5-6H20"/><path d="M8 12c3.5 0 4.5 6 8.5 6H20"/><path d="m17.5 3.5 2.5 2.5-2.5 2.5"/><path d="m17.5 15.5 2.5 2.5-2.5 2.5"/>',
```

用法：`<FIcon name="group" :size="14" :stroke="1.75" />`。FIcon 默认描边 1.8，与 1.75 肉眼无差，可不传 `stroke`。

## 重出预览
`node preview.mjs`：需要 `/workspace/design-tools/node_modules/playwright-core` 和 `/usr/bin/google-chrome`，并读取 fe 分支的 `icons.ts`（路径在脚本顶部）做并排对比。
