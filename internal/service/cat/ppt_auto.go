package cat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"FFmpegFree/internal/catagent"
)

// A new-deck request opens the live canvas before the model runs, so the user
// does not have to ask for the preview. Editing an existing page does not
// create another file. Failure only skips the head start; the turn still runs.
var (
	deckCreatePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?:生成|制作|创建|新建|起草)(?:一[个份套页])?.{0,8}(?:pptx|powerpoint|ppt|幻灯片|演示文稿|演示稿)`),
		regexp.MustCompile(`(?:做|写|出|弄)一[个份套页].{0,8}(?:pptx|powerpoint|ppt|幻灯片|演示文稿|演示稿)`),
		regexp.MustCompile(`(?:做|写|出|弄)[个份].{0,6}(?:pptx|powerpoint|ppt|幻灯片|演示文稿|演示稿)`),
		regexp.MustCompile(`(?:做|写|出)(?:pptx|powerpoint|ppt|幻灯片|演示文稿|演示稿)`),
		regexp.MustCompile(`(?i)\b(?:make|create|generate|build|draft|produce)\b.{0,40}\b(?:pptx|powerpoint|ppt|slides|slideshow|deck|presentation)\b`),
	}
	deckPathPattern = regexp.MustCompile(`(?i)(?:^|[\s"'“”‘’「」『』（）()，,：:、])((?:[\p{L}\p{N}_.-]+/)*[\p{L}\p{N}_.-]+\.pptx)`)
)

type deckRequest struct {
	rel      string
	explicit bool
	ok       bool
}

func newDeckRequest(text string) deckRequest {
	raw := strings.TrimSpace(text)
	if raw == "" || len(raw) > 8000 {
		return deckRequest{}
	}
	lower := strings.ToLower(raw)
	if asksHowToUseDeck(lower) || editsExistingDeck(lower) {
		return deckRequest{}
	}
	rel, explicit := explicitDeckPath(raw)
	if at, ok := deckCreateAt(lower); ok {
		if refusedDeck(lower, at) {
			return deckRequest{}
		}
		if explicit {
			return deckRequest{rel: rel, explicit: true, ok: true}
		}
		return deckRequest{rel: "output/slides.pptx", ok: true}
	}
	// "生成 output/季度汇报.pptx" names the file without a short noun next to the verb.
	if explicit {
		if at := createVerbAt(lower); at >= 0 && !refusedDeck(lower, at) {
			return deckRequest{rel: rel, explicit: true, ok: true}
		}
	}
	return deckRequest{}
}

func deckCreateAt(lower string) (int, bool) {
	for _, pattern := range deckCreatePatterns {
		loc := pattern.FindStringIndex(lower)
		if loc == nil {
			continue
		}
		span := lower[loc[0]:loc[1]]
		if strings.Contains(span, "第") && strings.Contains(span, "页") && !strings.Contains(span, "个") && !strings.Contains(span, "份") {
			continue
		}
		return loc[0], true
	}
	return 0, false
}

func createVerbAt(lower string) int {
	best := -1
	for _, verb := range []string{"生成", "制作", "创建", "新建", "起草", "做一", "写一", "出一", "弄一", "做个", "写个", "做份"} {
		if i := strings.Index(lower, verb); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	for _, verb := range []string{"make", "create", "generate", "build", "draft", "produce"} {
		i := asciiWordAt(lower, verb)
		if i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	return best
}

func asciiWordAt(text, word string) int {
	for i := 0; i+len(word) <= len(text); {
		j := strings.Index(text[i:], word)
		if j < 0 {
			return -1
		}
		j += i
		right := j + len(word)
		leftOK := j == 0 || !isASCIIWord(text[j-1])
		rightOK := right == len(text) || !isASCIIWord(text[right])
		if leftOK && rightOK {
			return j
		}
		i = j + 1
	}
	return -1
}

func isASCIIWord(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_'
}

func asksHowToUseDeck(lower string) bool {
	question := false
	for _, q := range []string{"怎么", "如何", "怎样", "为什么", "是什么", "how to", "how do", "how can", "what is", "what are"} {
		if strings.Contains(lower, q) {
			question = true
			break
		}
	}
	if !question {
		return false
	}
	return !strings.Contains(lower, "帮我") && !strings.Contains(lower, "给我") && !strings.Contains(lower, "please")
}

func refusedDeck(lower string, at int) bool {
	start := at - 18
	if start < 0 {
		start = 0
	}
	end := at + 12
	if end > len(lower) {
		end = len(lower)
	}
	window := lower[start:end]
	for _, phrase := range []string{"不要", "别生成", "别做", "别写", "不用", "无需", "don't", "do not"} {
		if strings.Contains(window, phrase) {
			return true
		}
	}
	return false
}

func editsExistingDeck(lower string) bool {
	if strings.Contains(lower, "一份") || strings.Contains(lower, "一套") || strings.Contains(lower, "新建") || strings.Contains(lower, "一个新") || strings.Contains(lower, "新的") || strings.Contains(lower, "a new") || strings.Contains(lower, "from scratch") {
		return false
	}
	edit := strings.Contains(lower, "修改") || strings.Contains(lower, "改成") || strings.Contains(lower, "改下") || strings.Contains(lower, "编辑") || strings.Contains(lower, "edit ") || strings.Contains(lower, "change ") || strings.Contains(lower, "update ")
	if edit {
		return true
	}
	return strings.Contains(lower, "第") && strings.Contains(lower, "页") && strings.Contains(lower, "改")
}

func explicitDeckPath(text string) (string, bool) {
	matches := deckPathPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return "", false
	}
	var output, other [][]int
	for _, match := range matches {
		start := match[2] - 24
		if start < 0 {
			start = 0
		}
		window := text[start:match[2]]
		lower := strings.ToLower(window)
		switch {
		case strings.Contains(window, "保存") || strings.Contains(window, "输出") || strings.Contains(window, "存为") || strings.Contains(window, "存成") || strings.Contains(window, "叫做") || strings.Contains(window, "名为") || strings.Contains(window, "写到") || strings.Contains(lower, " to ") || strings.Contains(lower, " into "):
			output = append(output, match)
		case strings.Contains(window, "根据") || strings.Contains(window, "参考") || strings.Contains(window, "基于") || strings.Contains(lower, "from "):
		default:
			other = append(other, match)
		}
	}
	var best []int
	switch {
	case len(output) > 0:
		best = output[0]
	case len(other) > 0:
		best = other[len(other)-1]
	default:
		return "", false
	}
	rel := strings.TrimPrefix(strings.ReplaceAll(text[best[2]:best[3]], `\`, "/"), "./")
	if rel == "" || strings.Contains(rel, "..") || strings.Contains(rel, ":") || strings.HasPrefix(rel, "/") || strings.Contains(rel, "//") {
		return "", false
	}
	return rel, true
}

func (s *Service) pickPptRel(ctx context.Context, convID, prefer string, explicit bool) string {
	try := func(rel string, allowExisting bool) string {
		abs, got, err := s.pptPath(ctx, ReadFileRequest{ConvID: convID, RelPath: rel})
		if err != nil {
			return ""
		}
		if allowExisting {
			return got
		}
		if _, err = os.Stat(abs); os.IsNotExist(err) {
			return got
		}
		return ""
	}
	if explicit {
		if got := try(prefer, true); got != "" {
			return got
		}
	}
	if got := try("output/slides.pptx", false); got != "" {
		return got
	}
	for i := 2; i <= 30; i++ {
		if got := try(fmt.Sprintf("output/slides-%d.pptx", i), false); got != "" {
			return got
		}
	}
	return ""
}

func (s *Service) pinLivePpt(ctx context.Context, convID, text string, opts *catagent.TurnOptions) {
	req := newDeckRequest(text)
	if !req.ok {
		return
	}
	rel := s.pickPptRel(ctx, convID, req.rel, req.explicit)
	if rel == "" {
		return
	}
	live, created, revision := false, false, 0
	if s.pptManager().Tool(false).State == "ready" {
		openCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		raw, err := s.officeRequest(openCtx, convID, map[string]any{"action": "open", "path": rel, "create": true})
		if err == nil {
			var state struct {
				Revision int `json:"revision"`
			}
			if json.Unmarshal(raw, &state) == nil {
				revision = state.Revision
			}
			live = true
			// The blank file has to exist before the model starts, so the open canvas is a real deck.
			if saved, err := s.officeRequest(openCtx, convID, map[string]any{"action": "save", "path": rel, "expectedRevision": revision}); err == nil {
				var after struct {
					Revision int `json:"revision"`
				}
				if json.Unmarshal(saved, &after) == nil {
					revision = after.Revision
				}
				created = true
			}
		}
		cancel()
	}
	const stream = "Submit exactly one page per office_page/page call. That call draws each element onto the open canvas before it returns; wait for its revision, then build the next page. Do not assemble the whole deck in another tool, script, or library, and do not replace the file in one write. Do not create a second presentation and do not launch an external Office application. The first page replaces the blank page."
	if !live {
		if s.prepareCatPpt(ctx, ReadFileRequest{ConvID: convID, RelPath: rel}) != nil {
			return
		}
		opts.Rules += fmt.Sprintf("\n<ffc_internal_file_preview>The preview tab is already open at %q. Create this exact file with open --create before any page. %s</ffc_internal_file_preview>\n", rel, stream)
		return
	}
	if created {
		opts.Rules += fmt.Sprintf("\n<ffc_internal_file_preview>Before you started, the app created %q and opened it in the live canvas at revision %d. A blank page is already on screen. Generate into this file only. %s</ffc_internal_file_preview>\n", rel, revision, stream)
		return
	}
	opts.Rules += fmt.Sprintf("\n<ffc_internal_file_preview>The live canvas is already open at %q with revision %d. A blank page is on screen. Generate into this file only. %s</ffc_internal_file_preview>\n", rel, revision, stream)
}
