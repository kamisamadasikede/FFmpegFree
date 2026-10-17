package cat

import (
	"encoding/json"
	"regexp"
	"strings"

	"FFmpegFree/internal/store"
)

var (
	reReportDid  = regexp.MustCompile(`(?m)^##\s*完成了什么\s*$`)
	reReportAttn = regexp.MustCompile(`(?m)^##\s*需要注意\s*$`)
	reReportGive = regexp.MustCompile(`(?m)^##\s*交给谁\s*$`)
	reAnyH2      = regexp.MustCompile(`(?m)^##\s+.+$`)
)

// parseGroupReport extracts did/attn/give from the last assistant reply (契约 6.21.15).
func parseGroupReport(text string) store.CatGroupReport {
	text = strings.TrimSpace(text)
	r := store.CatGroupReport{Files: []store.CatGroupFileTag{}}
	if text == "" {
		return r
	}
	var j struct {
		Did  string `json:"did"`
		Attn string `json:"attn"`
		Give string `json:"give"`
	}
	if json.Unmarshal([]byte(extractJSONObject(text)), &j) == nil && strings.TrimSpace(j.Did) != "" {
		r.Did, r.Attn, r.Give, r.Parsed = strings.TrimSpace(j.Did), strings.TrimSpace(j.Attn), strings.TrimSpace(j.Give), true
		return r
	}
	if !reReportDid.MatchString(text) {
		r.Did = text
		r.Parsed = false
		return r
	}
	r.Parsed = true
	r.Did = sectionBetween(text, reReportDid, []*regexp.Regexp{reReportAttn, reReportGive})
	r.Attn = sectionBetween(text, reReportAttn, []*regexp.Regexp{reReportGive, reReportDid})
	r.Give = sectionBetween(text, reReportGive, []*regexp.Regexp{reReportDid, reReportAttn})
	r.Did = strings.TrimSpace(r.Did)
	r.Attn = strings.TrimSpace(r.Attn)
	r.Give = strings.TrimSpace(r.Give)
	if r.Did == "" {
		r.Parsed = false
		r.Did = text
	}
	return r
}

func sectionBetween(text string, start *regexp.Regexp, stops []*regexp.Regexp) string {
	loc := start.FindStringIndex(text)
	if loc == nil {
		return ""
	}
	rest := text[loc[1]:]
	end := len(rest)
	for _, stop := range stops {
		if m := stop.FindStringIndex(rest); m != nil && m[0] < end {
			end = m[0]
		}
	}
	// Also stop at any other ## heading
	if m := reAnyH2.FindStringIndex(rest); m != nil && m[0] < end {
		// only if it's not our own start (already sliced)
		end = m[0]
	}
	return rest[:end]
}

// matchMentionsInText finds @{name} against members; longest name first; excludes selfID.
func matchMentionsInText(text string, members []store.CatGroupMember, selfID string) []string {
	type pair struct {
		name string
		id   string
	}
	list := make([]pair, 0, len(members))
	for _, m := range members {
		if m.ID == selfID {
			continue
		}
		list = append(list, pair{m.Name, m.ID})
	}
	// longest name first
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if len([]rune(list[j].name)) > len([]rune(list[i].name)) {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	seen := map[string]bool{}
	var out []string
	work := text
	for _, p := range list {
		needle := "@" + p.name
		if strings.Contains(work, needle) && !seen[p.id] {
			seen[p.id] = true
			out = append(out, p.id)
			// Blank out matched spans so shorter names cannot match inside longer ones.
			work = strings.ReplaceAll(work, needle, strings.Repeat(" ", len(needle)))
		}
	}
	return out
}

var followupPhrases = []string{
	"再改一下", "再改改", "这个不对", "继续", "接着做", "重做", "再试一次",
	"fix it", "continue", "again", "retry", "不对", "改一下",
}

func isFollowupText(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	for _, p := range followupPhrases {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}
