# 生成 NotoSansSC-Regular-subset.ttf：实例化 wght=400 → 子集 → 改 name 表。
# 用法：python mkfont.py 'NotoSansSC[wght].ttf' NotoSansSC-Regular-subset.ttf
# 依赖：fonttools（版本见 README.md）。
import sys
from fontTools.ttLib import TTFont
from fontTools.varLib import instancer
from fontTools import subset

src, out = sys.argv[1], sys.argv[2]
FAMILY = "FFmpegFree CJK Subset"  # OFL 保留字体名 "Source"：子集（修改版）不能沿用 Noto Sans SC 名，更不能含 Source

f = TTFont(src, recalcTimestamp=False)
f = instancer.instantiateVariableFont(f, {"wght": 400}, updateFontNames=False)

# 子集范围：GB2312 全部汉字与符号区 + ASCII/Latin-1 + 通用标点 + CJK 标点 + 假名 + 全角形式 + 箭头/数学/几何
chars = set()
for hi in range(0xA1, 0xF8):
    for lo in range(0xA1, 0xFF):
        try:
            chars.add(bytes([hi, lo]).decode("gb2312"))
        except UnicodeDecodeError:
            pass
for a, b in [(0x20, 0x7E), (0xA0, 0xFF), (0x2000, 0x206F), (0x3000, 0x303F), (0x3040, 0x30FF),
             (0xFF00, 0xFFEF), (0x2190, 0x21FF), (0x2200, 0x22FF), (0x25A0, 0x25FF)]:
    chars.update(chr(c) for c in range(a, b + 1))

opt = subset.Options()
opt.layout_features = ["kern", "vert"]
opt.name_IDs = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 16, 17]
opt.notdef_outline = True
opt.hinting = False
opt.desubroutinize = True
s = subset.Subsetter(opt)
s.populate(text="".join(chars))
s.subset(f)

name = f["name"]
# 只留 Windows/英文记录；nameID 0（版权，含 Reserved Font Name 声明）与 13/14（OFL 许可）原样保留
name.names = [r for r in name.names if not (r.platformID == 3 and r.langID != 1033) and r.platformID != 1]


def setn(i, v):
    name.names = [r for r in name.names if r.nameID != i]
    name.setName(v, i, 3, 1, 1033)


setn(1, FAMILY)
setn(2, "Regular")
setn(3, "FFmpegFreeCJKSubset-Regular;subset")
setn(4, FAMILY + " Regular")
setn(5, "Version 1.0; subset of Noto Sans SC 2.004 wght=400")
setn(6, "FFmpegFreeCJKSubset-Regular")
# 7（商标声明，含 Source）、8~12、16/17（排版家族名，原为 Noto Sans SC）、21/22/25/257 一律删除
for i in (7, 8, 9, 10, 11, 12, 16, 17, 21, 22, 25, 257):
    name.names = [r for r in name.names if r.nameID != i]
f.save(out)
