package cat

import (
	"strings"
	"testing"

	"FFmpegFree/internal/store"
)

func TestParseGroupReport_Markdown(t *testing.T) {
	text := "## 完成了什么\n做了登录接口\n\n## 需要注意\n要配环境变量\n\n## 交给谁\n@前端工程师 接页面\n"
	r := parseGroupReport(text)
	if !r.Parsed || r.Did != "做了登录接口" || r.Attn != "要配环境变量" || r.Give != "@前端工程师 接页面" {
		t.Fatalf("%+v", r)
	}
}

func TestParseGroupReport_Fallback(t *testing.T) {
	r := parseGroupReport("随便写了一段")
	if r.Parsed || r.Did != "随便写了一段" {
		t.Fatalf("%+v", r)
	}
}

func TestParseGroupReport_JSON(t *testing.T) {
	r := parseGroupReport(`{"did":"完成接口","attn":"注意鉴权","give":"@前端 接"}`)
	if !r.Parsed || r.Did != "完成接口" || r.Attn != "注意鉴权" {
		t.Fatalf("%+v", r)
	}
}

func TestMatchMentionsLongestFirst(t *testing.T) {
	members := []store.CatGroupMember{
		{ID: "1", Name: "后端"},
		{ID: "2", Name: "后端工程师"},
		{ID: "3", Name: "前端工程师"},
	}
	got := matchMentionsInText("请 @后端工程师 和 @前端工程师 看看", members, "")
	if len(got) != 2 || got[0] != "2" {
		t.Fatalf("%v", got)
	}
}

func TestMatchMentionsOnlyFromGive(t *testing.T) {
	members := []store.CatGroupMember{
		{ID: "fe", Name: "前端"},
		{ID: "be", Name: "后端"},
	}
	body := `## 完成了什么
看了 @前端 的页面

## 需要注意
@后端 也提过

## 交给谁
@后端 接接口
`
	r := parseGroupReport(body)
	if !r.Parsed || r.Give != "@后端 接接口" {
		t.Fatalf("give %+v", r)
	}
	got := matchMentionsInText(r.Give, members, "")
	if len(got) != 1 || got[0] != "be" {
		t.Fatalf("give mentions %v", got)
	}
	// 扫全文会误伤 did/attn；只扫 give 时不应带上前端
	if all := matchMentionsInText(body, members, ""); len(all) != 2 {
		t.Fatalf("sanity full-text mentions %v", all)
	}
}

func TestDefaultHandoffNoAtEveryone(t *testing.T) {
	members := []store.CatGroupMember{
		{ID: "1", Name: "甲", Title: "前端"},
		{ID: "2", Name: "乙", Title: "后端"},
		{ID: "3", Name: "丙", Title: "产品"},
	}
	got := defaultHandoff(members[0], members)
	for _, m := range members {
		if strings.Contains(got, "@"+m.Name) {
			t.Fatalf("must not @ member %q: %q", m.Name, got)
		}
	}
	for _, bad := range []string{"所有其他", "下一位：", "@所有"} {
		if strings.Contains(got, bad) {
			t.Fatalf("forbidden %q in %q", bad, got)
		}
	}
	// 「全员」只允许出现在「禁止…全员」警示里，不得写成「交给全员」诱导。
	if strings.Contains(got, "交给全员") && !strings.Contains(got, "禁止") {
		t.Fatalf("must not push 交给全员: %q", got)
	}
	if !strings.Contains(got, "不必交接") {
		t.Fatalf("want default no-handoff guidance: %q", got)
	}
}
