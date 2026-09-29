package doc

import (
	"context"
	"strings"
	"unicode"
)

// fpdf 的 MultiCell 把汉字当作可断点，断行后从 sep+1 继续，会把断点处那个字丢掉（实测：
// 长中文段落每次自动换行都少一个字）。这里自己按字宽折行，再用 CellFormat 逐行输出。
//
// 算法是单趟线性的：先把文本切成不可再分的 token，再逐个放进当前行；宽度用累加而不是每次重算整行。
// 不做“把禁则字符并入前一个 token”的字符串拼接（那样连续标点是二次方，也曾在行首空白后丢字）。

// isWrapCJK 报告 r 是否可以在它前后直接断行（汉字、假名、谚文、CJK/全角标点）。
func isWrapCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r) || (r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF)
}

// 行首禁则 / 行尾禁则（简化版）：这些字符尽量不出现在行首 / 行尾。
const (
	noLineStart = "，。、．，；：！？）］｝〉》」』】〕”’…—～ゝゞーぁぃぅぇぉっゃゅょゎァィゥェォッャュョヮ,.;:!?)]}%"
	noLineEnd   = "（［｛〈《「『【〔“‘([{"

	// maxHang 是一行最多“挂”在行尾外面的禁则字符数（避头点）；超过就照常换行，所以连续标点也不会无限拖长一行。
	maxHang = 2
	// wrapCheckEvery 是折行循环检查 ctx 的间隔（token 数）。
	wrapCheckEvery = 4096
)

func isSpaceRune(r rune) bool { return r == ' ' || r == '\t' }

type wtok struct {
	s     string
	space bool // 一段空格 / 制表符
	hang  bool // 可以挂在行尾外面的禁则标点（≤2 个字符，全部是行首禁则字符）
	open  bool // 单个行尾禁则字符（开括号 / 开引号）：不能孤零零留在行尾
}

// wrapText 把 s 折成不超过 maxW（mm，由 width 度量）的若干行。'\n' 强制换行；空格处可断，折行处的空格丢弃；
// CJK 字符之间可断；超长的单词按字符硬断。除折行处丢掉的空格外，每个字符恰好出现一次。
// 行宽只在挂起的行首禁则标点（≤maxHang 个）和“单字符本身就比行宽”时才会超出 maxW。
func wrapText(ctx context.Context, s string, maxW float64, width func(string) float64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out []string
	n := 0
	for {
		para := s
		rest, more := "", false
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			para, rest, more = s[:i], s[i+1:], true
		}
		ls, err := wrapLine(ctx, strings.TrimRight(para, "\r"), maxW, width, &n)
		if err != nil {
			return nil, err
		}
		out = append(out, ls...)
		if !more {
			break
		}
		s = rest
	}
	return out, nil
}

func tokenize(ctx context.Context, rs []rune) ([]wtok, error) {
	var toks []wtok
	for i := 0; i < len(rs); {
		if len(toks)%wrapCheckEvery == wrapCheckEvery-1 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		r := rs[i]
		j := i + 1
		space := false
		switch {
		case isSpaceRune(r):
			space = true
			for j < len(rs) && isSpaceRune(rs[j]) {
				j++
			}
		case isWrapCJK(r):
		default:
			for j < len(rs) && !isSpaceRune(rs[j]) && !isWrapCJK(rs[j]) {
				j++
			}
		}
		t := wtok{s: string(rs[i:j]), space: space}
		if !space {
			if j-i <= maxHang {
				t.hang = true
				for _, x := range rs[i:j] {
					if !strings.ContainsRune(noLineStart, x) {
						t.hang = false
						break
					}
				}
			}
			t.open = j-i == 1 && strings.ContainsRune(noLineEnd, r)
		}
		toks = append(toks, t)
		i = j
	}
	return toks, nil
}

func wrapLine(ctx context.Context, s string, maxW float64, width func(string) float64, counter *int) ([]string, error) {
	if s == "" {
		return []string{""}, nil
	}
	toks, err := tokenize(ctx, []rune(s))
	if err != nil {
		return nil, err
	}
	var (
		lines []string
		cur   strings.Builder
		curW  float64
		hung  int
	)
	flush := func() {
		lines = append(lines, strings.TrimRight(cur.String(), " \t"))
		cur.Reset()
		curW, hung = 0, 0
	}
	put := func(t string, w float64) {
		cur.WriteString(t)
		curW += w
	}
	for i, t := range toks {
		if *counter++; *counter%wrapCheckEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		w := width(t.s)
		if t.space {
			// 行首空白丢弃；放不下的空白就是折行点（立即换行，不让后面较窄的 token 贴到前一个词上）
			if cur.Len() > 0 {
				if curW+w <= maxW {
					put(t.s, w)
				} else {
					flush()
				}
			}
			continue
		}
		// 需要的宽度：开括号必须和后面的 token 在同一行
		need := w
		if t.open && i+1 < len(toks) && !toks[i+1].space {
			need += width(toks[i+1].s)
		}
		if curW+need <= maxW {
			put(t.s, w)
			continue
		}
		if t.hang && cur.Len() > 0 && hung+len([]rune(t.s)) <= maxHang {
			// 避头：句号、逗号等挂在行尾外面，而不是掉到下一行行首
			put(t.s, w)
			hung += len([]rune(t.s))
			continue
		}
		if cur.Len() > 0 && strings.TrimSpace(cur.String()) != "" {
			flush()
		} else {
			cur.Reset()
			curW = 0
		}
		if w <= maxW {
			put(t.s, w)
			continue
		}
		// 单个 token 超宽：按字符硬断，每行至少放一个字符，保证前进。
		for _, r := range t.s {
			rw := width(string(r))
			if cur.Len() > 0 && curW+rw > maxW {
				flush()
			}
			put(string(r), rw)
		}
	}
	if cur.Len() > 0 || len(lines) == 0 {
		flush()
	}
	return lines, nil
}
