package cat

import (
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
