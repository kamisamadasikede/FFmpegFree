package doc

import (
	_ "embed"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unicode"
)

// 内嵌字体：Noto Sans SC Regular（wght=400）的静态 TrueType 子集，来源、子集范围和生成命令见 fonts/README.md。
// 子集文件的内部字体名是 “FFmpegFree CJK Subset”（OFL 的保留字体名限制，nameID 1/4/6/16/17 不含 Source）。
//
//go:embed fonts/NotoSansSC-Regular-subset.ttf
var embeddedTTF []byte

// EmbeddedFontName 是 DocFont.Name 里内嵌字体的名字。
const EmbeddedFontName = "noto-sans-sc-embedded"

// SystemFont 是一个系统字体候选（必须是 TrueType 轮廓的 .ttf，fpdf 不能加载 .ttc / .otf）。
type SystemFont struct{ Name, Path string }

// defaultSystemFonts 是平台默认候选，按顺序找第一个存在且可加载的（Windows 真机未验证）。
func defaultSystemFonts() []SystemFont {
	switch runtime.GOOS {
	case "windows":
		root := os.Getenv("WINDIR")
		if root == "" {
			root = os.Getenv("SystemRoot")
		}
		if root == "" {
			root = `C:\Windows`
		}
		d := filepath.Join(root, "Fonts")
		return []SystemFont{
			{"simhei", filepath.Join(d, "simhei.ttf")},
			{"msyh", filepath.Join(d, "msyh.ttf")},
			{"simsun", filepath.Join(d, "simsun.ttf")},
		}
	case "darwin":
		return []SystemFont{
			{"arialunicode", "/Library/Fonts/Arial Unicode.ttf"},
			{"arialunicode", "/Library/Fonts/Arial Unicode MS.ttf"},
			{"arialunicode", "/System/Library/Fonts/Supplemental/Arial Unicode.ttf"},
		}
	default:
		return []SystemFont{{"dejavu", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"}}
	}
}

// face 是一份可用的字体。
type face struct {
	name string
	data []byte
	f    *sfnt
}

func (fc *face) has(r rune) bool { return fc.f.cmap.Has(r) }

// cjk：字体是否覆盖一组 GB2312 一级汉字样本（内嵌字体为 true）。
func (fc *face) cjk() bool {
	for _, r := range "的一是不了人我在有他这中大来上国个到说们为子和你地出道也时年得就那要下以生会自着去之过家学对可她里后小么心多天而能好都然没日于起还发成事只作当想看文用" {
		if !fc.has(r) {
			return false
		}
	}
	return true
}

type sysSlot struct {
	SystemFont
	once sync.Once
	face *face
}

// fontSet 管理内嵌字体和系统候选，系统字体首次用到时读取并缓存。
type fontSet struct {
	embedded *face
	system   []*sysSlot
}

func newFontSet(cfg Config) *fontSet {
	fs := &fontSet{}
	if !cfg.DisableEmbedded {
		data := cfg.EmbeddedFont
		if data == nil {
			data = embeddedTTF
		}
		if sf, err := parseSfnt(data); err == nil {
			fs.embedded = &face{name: EmbeddedFontName, data: data, f: sf}
		}
	}
	cands := cfg.SystemFonts
	if cands == nil {
		cands = defaultSystemFonts()
	}
	for _, c := range cands {
		fs.system = append(fs.system, &sysSlot{SystemFont: c})
	}
	return fs
}

const maxSystemFontBytes = 64 << 20

func (sl *sysSlot) load() *face {
	sl.once.Do(func() {
		fi, err := os.Stat(sl.Path)
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxSystemFontBytes {
			return
		}
		data, err := os.ReadFile(sl.Path)
		if err != nil {
			return
		}
		sf, err := parseSfnt(data)
		if err != nil {
			return
		}
		sl.face = &face{name: sl.Name, data: data, f: sf}
	})
	return sl.face
}

// firstSystem 返回第一个可加载的系统字体。
func (fs *fontSet) firstSystem() *face {
	for _, sl := range fs.system {
		if f := sl.load(); f != nil {
			return f
		}
	}
	return nil
}

// primary 是 GetDocCapabilities 里的主用字体：内嵌优先，其次第一个可用的系统字体。
func (fs *fontSet) primary() DocFont {
	f := fs.embedded
	if f == nil {
		f = fs.firstSystem()
	}
	if f == nil {
		return DocFont{}
	}
	return DocFont{Available: true, Name: f.name, Cjk: f.cjk()}
}

// fontChoice 是给一份文档选定的字体；face 为 nil 表示内置 Helvetica（只有纯 Latin-1 文档才会这样）。
type fontChoice struct {
	face *face
}

func (c fontChoice) name() string {
	if c.face == nil {
		return "helvetica"
	}
	return c.face.name
}

// missingRunes 统计字体没有字形的字符：(种类数, 出现次数, 最多 20 个样例)。空白与控制字符不算。
func missingRunes(f *face, runes map[rune]int) (kinds, total int, sample []rune) {
	for r, n := range runes {
		if unicode.IsSpace(r) || r < 0x20 {
			continue
		}
		if f == nil {
			if r > 0xFF {
				kinds++
				total += n
			}
			continue
		}
		if !f.has(r) {
			kinds++
			total += n
			if len(sample) < 20 {
				sample = append(sample, r)
			}
		}
	}
	return
}

// choose 按文档整份选一个字体，不做逐字回退（契约 6.12.1）：
//  1. 内嵌字体覆盖全部字符 → 内嵌；
//  2. 否则第一个覆盖全部字符的系统字体；
//  3. 否则仍用内嵌字体（缺字为方框，不算失败）；
//  4. 没有内嵌字体：用第一个可用的系统字体；一个都没有时，纯 Latin-1 文档用内置 Helvetica，含 U+00FF 以上的字符返回 ok=false。
func (fs *fontSet) choose(runes map[rune]int) (fontChoice, bool) {
	if fs.embedded != nil {
		if k, _, _ := missingRunes(fs.embedded, runes); k == 0 {
			return fontChoice{fs.embedded}, true
		}
	}
	var firstSys *face
	for _, sl := range fs.system {
		f := sl.load()
		if f == nil {
			continue
		}
		if firstSys == nil {
			firstSys = f
		}
		if k, _, _ := missingRunes(f, runes); k == 0 {
			return fontChoice{f}, true
		}
	}
	if fs.embedded != nil {
		return fontChoice{fs.embedded}, true
	}
	if firstSys != nil {
		return fontChoice{firstSys}, true
	}
	if k, _, _ := missingRunes(nil, runes); k == 0 {
		return fontChoice{}, true
	}
	return fontChoice{}, false
}
