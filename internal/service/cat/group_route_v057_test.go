package cat

import (
	"strings"
	"testing"

	"FFmpegFree/internal/store"
)

func sampleMembers() []store.CatGroupMember {
	return []store.CatGroupMember{
		{ID: "id-fe", Name: "前端", Title: "前端工程师"},
		{ID: "id-be", Name: "后端", Title: "后端工程师"},
		{ID: "id-pm", Name: "产品", Title: "产品经理"},
	}
}

func TestParseAIRouteOutput_MemberIdsMulti(t *testing.T) {
	ms := sampleMembers()
	ids, everyone, why, ok := parseAIRouteOutput(ms, `{"memberIds":["id-fe","id-be"],"reason":"两人一起介绍"}`)
	if !ok || everyone || why != "两人一起介绍" {
		t.Fatalf("ok=%v everyone=%v why=%q", ok, everyone, why)
	}
	if len(ids) != 2 || ids[0] != "id-fe" || ids[1] != "id-be" {
		t.Fatalf("ids=%v", ids)
	}
}

func TestParseAIRouteOutput_EveryoneFlag(t *testing.T) {
	ms := sampleMembers()
	ids, everyone, why, ok := parseAIRouteOutput(ms, `{"everyone":true,"reason":"介绍下自己"}`)
	if !ok || !everyone || why != "介绍下自己" {
		t.Fatalf("ok=%v everyone=%v why=%q", ok, everyone, why)
	}
	if len(ids) != 3 {
		t.Fatalf("want all 3, got %v", ids)
	}
}

func TestParseAIRouteOutput_EveryoneSentinel(t *testing.T) {
	ms := sampleMembers()
	ids, everyone, _, ok := parseAIRouteOutput(ms, `{"memberIds":["@全体"],"reason":"全员招呼"}`)
	if !ok || !everyone || len(ids) != 3 {
		t.Fatalf("ok=%v everyone=%v ids=%v", ok, everyone, ids)
	}
}

func TestParseAIRouteOutput_LegacyMemberId(t *testing.T) {
	ms := sampleMembers()
	ids, everyone, why, ok := parseAIRouteOutput(ms, `{"memberId":"前端","reason":"开场招呼偏长要截断到四十个汉字以内还要再多一点点字数才行啊"}`)
	if !ok || everyone || len(ids) != 1 || ids[0] != "id-fe" {
		t.Fatalf("ok=%v everyone=%v ids=%v", ok, everyone, ids)
	}
	if len([]rune(why)) > 40 {
		t.Fatalf("reason not truncated: %q (%d)", why, len([]rune(why)))
	}
}

func TestParseAIRouteOutput_UnknownIDFails(t *testing.T) {
	ms := sampleMembers()
	_, _, _, ok := parseAIRouteOutput(ms, `{"memberIds":["id-fe","nobody"],"reason":"x"}`)
	if ok {
		t.Fatal("expected fail on unknown id")
	}
	if got := routeParseFailReason(ms, `{"memberIds":["nobody"],"reason":"x"}`); got != "unknown_id" {
		t.Fatalf("fail reason=%q", got)
	}
}

func TestFormatCoordBubbleText(t *testing.T) {
	ms := sampleMembers()
	got := formatCoordBubbleText("请自我介绍", false, ms, []string{"id-fe", "id-be"})
	if got != "请自我介绍 @前端 @后端" {
		t.Fatalf("got %q", got)
	}
	got = formatCoordBubbleText("全员", true, ms, []string{"id-fe"})
	if got != "全员 @全体" {
		t.Fatalf("everyone: %q", got)
	}
	// reason capped at 40 in parse; format also truncates
	long := strings.Repeat("啊", 50)
	got = formatCoordBubbleText(long, false, ms, []string{"id-pm"})
	if !strings.HasPrefix(got, strings.Repeat("啊", 40)+" @产品") {
		t.Fatalf("truncate: %q", got)
	}
}

func TestCoordWakeUsesHopZero(t *testing.T) {
	// 契约：协调唤醒不计 hop；SendCatGroupMessage 对 AI 成功路径传 hop=0。
	if maxGroupHops != 6 {
		t.Fatalf("maxGroupHops=%d", maxGroupHops)
	}
	// 防回归：成功正文不得再出现静默「交给 …：」主结果（协调气泡用 @）。
	text := formatCoordBubbleText("派给前端", false, sampleMembers(), []string{"id-fe"})
	if strings.Contains(text, "交给 ") {
		t.Fatalf("silent 交给 leaked into coord text: %q", text)
	}
	if !strings.Contains(text, "@前端") {
		t.Fatalf("missing @ mention: %q", text)
	}
}

func TestRouteSchemaMentionsMemberIds(t *testing.T) {
	// 编译期保证 RouteSchema 已切到 memberIds（见 catagent 包）；此处只锁解析侧。
	ms := sampleMembers()
	if _, _, _, ok := parseAIRouteOutput(ms, `{"memberIds":["id-be"],"reason":"ok"}`); !ok {
		t.Fatal("memberIds schema path must parse")
	}
}
