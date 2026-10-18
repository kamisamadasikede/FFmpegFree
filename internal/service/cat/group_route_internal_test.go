package cat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
)

func TestAIRouteTimeoutConstant(t *testing.T) {
	if aiRouteTimeout != 45*time.Second {
		t.Fatalf("aiRouteTimeout=%v want 45s", aiRouteTimeout)
	}
}

func TestIsAIRouteTimeout(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline", context.DeadlineExceeded, true},
		{"wrapped deadline", errors.Join(context.DeadlineExceeded, errors.New("x")), true},
		{"apperr reason=timeout", apperr.New(apperr.CatReplyFailed, "超时").WithDetail("reason=timeout"), true},
		{"apperr idle", apperr.New(apperr.CatReplyFailed, "无响应").WithDetail("reason=idle_timeout"), true},
		{"apperr other", apperr.New(apperr.CatReplyFailed, "挂了").WithDetail("reason=cli"), false},
		{"plain", errors.New("boom"), false},
		{"deadline text", errors.New("context deadline exceeded"), true},
	}
	for _, c := range cases {
		if got := isAIRouteTimeout(c.err); got != c.want {
			t.Fatalf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestUndecidedReasonSuffix(t *testing.T) {
	cases := map[string]string{
		"timeout":     "判断超时",
		"empty":       "输出为空",
		"parse_error": "解析失败",
		"run_error":   "判断执行失败",
		"unknown_id":  "判断结果对不上成员",
		"":            "",
		"other":       "",
	}
	for code, want := range cases {
		if got := undecidedReasonSuffix(code); got != want {
			t.Fatalf("suffix(%q)=%q want %q", code, got, want)
		}
	}
}

func TestUndecidedGreyText(t *testing.T) {
	base := undecidedRouteBase
	if !strings.HasPrefix(base, "没判断出该谁接") {
		t.Fatalf("base locked copy drifted: %q", base)
	}
	if got := undecidedGreyText(""); got != base {
		t.Fatalf("empty code: %q", got)
	}
	got := undecidedGreyText("timeout")
	if got != base+"·判断超时" {
		t.Fatalf("timeout grey: %q", got)
	}
	if strings.Contains(got, "没判断出该谁接：") {
		t.Fatalf("must not replace main sentence with colon form: %q", got)
	}
	// DurationMs inline still separate; grey must keep base so FE ·判断用时 can follow.
	if !strings.HasPrefix(got, base) {
		t.Fatalf("must keep base prefix: %q", got)
	}
}

func TestRouteParseFailReason(t *testing.T) {
	members := []store.CatGroupMember{
		{ID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Name: "前端", Title: "前端工程师"},
	}
	if got := routeParseFailReason(members, "not json"); got != "parse_error" {
		t.Fatalf("parse_error: %q", got)
	}
	if got := routeParseFailReason(members, `{"memberId":"","reason":"x"}`); got != "empty" {
		t.Fatalf("empty: %q", got)
	}
	if got := routeParseFailReason(members, `{"memberId":"nobody","reason":"x"}`); got != "unknown_id" {
		t.Fatalf("unknown_id: %q", got)
	}
	if ids, everyone, why, ok := parseAIRouteOutput(members, `{"memberId":"前端","reason":"开场招呼"}`); !ok || everyone || len(ids) != 1 || ids[0] != members[0].ID || why != "开场招呼" {
		t.Fatalf("parse ok: ids=%v everyone=%v why=%q ok=%v", ids, everyone, why, ok)
	}
}
