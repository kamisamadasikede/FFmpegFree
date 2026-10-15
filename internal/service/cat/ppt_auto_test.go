package cat

import "testing"

func TestNewDeckRequest(t *testing.T) {
	cases := []struct {
		text     string
		rel      string
		explicit bool
		ok       bool
	}{
		{text: "生成一个ppt", rel: "output/slides.pptx", ok: true},
		{text: "帮我做一份幻灯片", rel: "output/slides.pptx", ok: true},
		{text: "制作演示文稿", rel: "output/slides.pptx", ok: true},
		{text: "make a PPT", rel: "output/slides.pptx", ok: true},
		{text: "generate native ppt", rel: "output/slides.pptx", ok: true},
		{text: "create a slide deck", rel: "output/slides.pptx", ok: true},
		{text: "生成 output/季度汇报.pptx", rel: "output/季度汇报.pptx", explicit: true, ok: true},
		{text: "根据提纲生成一份，保存为 output/report.pptx", rel: "output/report.pptx", explicit: true, ok: true},
		{text: "怎么用 ppt"},
		{text: "如何制作演示文稿"},
		{text: "不要生成ppt"},
		{text: "don't make a ppt"},
		{text: "把第3页改成蓝色"},
		{text: "修改 deck.pptx 的标题"},
		{text: "做一份脚本"},
		{text: "hello"},
		{text: "生成一个ppt，第1页是封面", rel: "output/slides.pptx", ok: true},
		{text: "生成第3页的ppt"},
	}
	for _, tc := range cases {
		got := newDeckRequest(tc.text)
		if got.ok != tc.ok || got.explicit != tc.explicit || got.rel != tc.rel {
			t.Fatalf("%q → %+v, want rel=%q explicit=%v ok=%v", tc.text, got, tc.rel, tc.explicit, tc.ok)
		}
	}
}
