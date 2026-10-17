package cat

import (
	"strings"
	"testing"

	"FFmpegFree/internal/store"
)

func TestGroupRulesText_V056Handoff(t *testing.T) {
	self := store.CatGroupMember{ID: "1", Name: "甲", Title: "后端", Charter: store.CatCharter{Owns: "接口", NotTouch: "页面", Handoff: defaultHandoff(store.CatGroupMember{ID: "1"}, nil)}}
	members := []store.CatGroupMember{self, {ID: "2", Name: "乙", Title: "前端"}}
	got := groupRulesText(self, members)
	for _, want := range []string{"不必交接", "短自然语言", "可空", "禁止做完交给全员"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in rules:\n%s", want, got)
		}
	}
	for _, bad := range []string{"做完后若有下一位", "在回复里 @ 负责的人交出去"} {
		if strings.Contains(got, bad) {
			t.Fatalf("old push-handoff wording still present: %q", bad)
		}
	}
}
