# 内嵌字体：Noto Sans SC Regular 子集

`NotoSansSC-Regular-subset.ttf` 由 `go:embed` 内嵌（`../fonts.go`），是 fpdf v0.9.0 生成 PDF 时的主用字体。

| 项 | 值 |
|---|---|
| 来源文件 | google/fonts 仓库 `ofl/notosanssc/NotoSansSC[wght].ttf`（可变字体，17 772 300 字节，MD5 `0968e7d36e329171a6034639c88ff718`，SHA-256 `a3041811a78c361b1de50f953c805e0244951c21c5bd412f7232ef0d899af0da`） |
| 提交 SHA | `2894aab31764f10f29c421bdfd2340d3b382d384`（google/fonts，2022-12-09，该文件的最后一次修改；main 上的同一文件与它逐字节相同） |
| 许可 | SIL OFL 1.1，`OFL.txt` 是 google/fonts 同目录的 `OFL.txt` 原样拷贝（不改一个字节，4388 字节，SHA-256 `1c05c68c34f9708415aada51f17e1b0092d2cea709bf4a94cd38114f9e73d7d9`；与 google/fonts main 同目录文件逐字节相同） |
| 工具 | fontTools 4.66.0（Python 3.13.5，Brotli 1.2.0） |
| 生成脚本 | `mkfont.py`（同目录） |
| 产物 | 2 741 704 字节（≈2.61 MiB），SHA-256 `b96fad9e311f2f0254f2b3dc4db8ac0fb791c4f7a2211e2f04d02e68dd1f20ae`，TrueType 轮廓（`glyf`），无 `fvar` / `CFF `，9010 个字符 |

## 生成命令

```sh
pip install fonttools brotli
curl -LO 'https://raw.githubusercontent.com/google/fonts/2894aab31764f10f29c421bdfd2340d3b382d384/ofl/notosanssc/NotoSansSC%5Bwght%5D.ttf'
python mkfont.py 'NotoSansSC[wght].ttf' NotoSansSC-Regular-subset.ttf
```

脚本做三件事：
1. `fontTools.varLib.instancer` 固定 `wght=400`（去掉 `fvar` 等可变表）；
2. `fontTools.subset` 子集化：GB2312 全部汉字与符号区、JIS X 0208 第一水准汉字（EUC-JP 0xB0A1–0xCFFE，2965 字，含 語 読 黒 龍 等日文常用汉字）、ASCII、Latin-1（U+00A0–00FF）、通用标点（U+2000–206F）、CJK 标点（U+3000–303F）、平假名 / 片假名（U+3040–30FF）、全角形式（U+FF00–FFEF）、箭头 / 数学 / 几何（U+2190–21FF、2200–22FF、25A0–25FF）；保留 `kern` / `vert`，去 hinting；
3. 改 `name` 表：nameID 1 / 4 / 6 改成 `FFmpegFree CJK Subset`（及 `-Regular` 变体），nameID 2 / 3 / 5 重写，nameID 7 / 8–12 / 16 / 17 / 21 / 22 / 25 / 257 删除；**nameID 0（版权，含 Reserved Font Name 声明）与 13 / 14（OFL 许可与链接）原样保留**。

## 复现性

`mkfont.py` 在同一 fontTools 版本下两次运行的产物逐字节相同（本目录的文件就是这样得到的）。契约 §6.12.1 记录的样品是 2 355 692 字节（只含 GB2312，无日文汉字），SHA-256 `48c44ed1…`，与本文件的 `b96fad9e…` **不同**：本文件多了 JIS 第一水准汉字（+386 KB），并且
`mkfont.py` 里 `TTFont(..., recalcTimestamp=False)`（保持源文件的 head.modified，不写入当前时间），样品没有这一项，所以 `head.modified` 也不同；name 表相同，GB2312 部分字形相同。以本目录文件与上表 SHA-256 为准（`TestEmbeddedFontSHA256MatchesReadme` 断言）。

## 已知限制

- GB2312 与 JIS 第一水准以外的字（JIS 第二水准的生僻日文汉字、繁体专用字、谚文、emoji）不在子集内，显示为 `.notdef` 方框（转换日志会写缺字数）。
- OFL 保留字体名（下载到的 OFL.txt 声明保留名 `Source`）：子集化属于修改，所以内部字体名不含 `Source`（单测 `TestEmbeddedFontNameHasNoSource` 断言：除 nameID 0 外任何记录都不含 Source；nameID 0 / 13 / 14 保留）。这是保守合规做法，不是法律结论。
- 不同 fontTools 版本 / 参数可能得到不同字节；这里记录的是实际用的版本和命令。

## 读取字体

Go 侧用自写的 sfnt 读取器（`../sfnt.go`，只读 `cmap` 与 `name` 表，约 200 行），**没有新增 `golang.org/x/image` 依赖**；`go.mod` 没有变化（测试里用 `golang.org/x/text` 解 GB2312，该模块在 feat/edit-impl 已升为直接依赖）。
