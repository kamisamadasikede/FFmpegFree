package doc

import (
	"context"
	"errors"
	"testing"
)

func TestCacheKeyStable(t *testing.T) {
	a := cacheKey("/a", 1, 2, "office")
	b := cacheKey("/a", 1, 2, "office")
	c := cacheKey("/a", 1, 2, "wps")
	if a != b || a == c || len(a) != 64 {
		t.Fatalf("a=%s b=%s c=%s", a, b, c)
	}
}

func TestPreviewEnginesNilRegistry(t *testing.T) {
	s := &Service{}
	if got := s.previewEngines(context.Background(), "docx"); len(got) != 0 {
		t.Fatalf("没接引擎时应为空：%v", got)
	}
}

func TestCancelPreviewCancelsGeneration(t *testing.T) {
	s := &Service{}
	ctx, cancel := context.WithCancel(context.Background())
	s.trackEntry(previewEntry{id: "p1", cancel: cancel})
	if err := s.CancelDocPreview("p1"); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("取消预览应取消生成")
	}
	if s.updateEntry("p1", "", "") {
		t.Fatal("已释放的预览不应再登记")
	}
	_ = s.CancelDocPreview("p1") // 幂等
}

func TestPreviewEvictionCancelsOldest(t *testing.T) {
	s := &Service{}
	ctx0, cancel0 := context.WithCancel(context.Background())
	s.trackEntry(previewEntry{id: "old", cancel: cancel0})
	for i := 0; i < maxLivePreviews; i++ {
		s.trackPreview(newPreviewID(), "", "")
	}
	if ctx0.Err() == nil {
		t.Fatal("超过 8 个预览时最早的应被释放")
	}
}

func TestPreviewFailedFallsBackToRaw(t *testing.T) {
	var got []DocPreviewEvent
	s := &Service{cfg: Config{Emit: func(_ string, p any) { got = append(got, p.(DocPreviewEvent)) }}}
	s.trackEntry(previewEntry{id: "a"})
	s.trackEntry(previewEntry{id: "b"})
	s.finishPreviewFailed(previewJob{id: "a", rawURL: "/localfile/x"}, errors.New("boom"))
	s.finishPreviewFailed(previewJob{id: "b"}, errors.New("boom"))
	if len(got) != 2 {
		t.Fatalf("events=%v", got)
	}
	if got[0].State != "ready" || got[0].Kind != "raw" || got[0].URL != "/localfile/x" {
		t.Fatalf("docx/xlsx 生成失败应退到 raw：%+v", got[0])
	}
	if got[1].State != "failed" || got[1].Error == nil {
		t.Fatalf("其余格式应 failed：%+v", got[1])
	}
}
