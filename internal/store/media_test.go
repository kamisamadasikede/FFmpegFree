package store

import (
	"context"
	"testing"
)

func TestMediaUpsertListDelete(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	a, err := s.UpsertMedia(ctx, "/a/1.mp4", MediaInfo{ID: "M1", Path: "/A/1.mp4", Name: "1.mp4", Size: 10, Duration: 2.5,
		Width: 320, Height: 240, VideoCodec: "h264", AudioCodec: "aac", Bitrate: 999, ProbedAt: 100})
	if err != nil || a.ID != "M1" {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := s.UpsertMedia(ctx, "/a/2.mp3", MediaInfo{ID: "M2", Path: "/a/2.mp3", Name: "2.mp3", AudioCodec: "mp3", ProbedAt: 200}); err != nil {
		t.Fatal(err)
	}
	// 同一 path_key 再次写入：保留原 id，更新内容
	b, err := s.UpsertMedia(ctx, "/a/1.mp4", MediaInfo{ID: "M9", Path: "/a/1.mp4", Name: "1.mp4", Size: 20, Duration: 3, VideoCodec: "h264", ProbedAt: 300})
	if err != nil || b.ID != "M1" {
		t.Fatalf("应保留原 id: %+v %v", b, err)
	}
	list, err := s.ListRecentMedia(ctx, 10)
	if err != nil || len(list) != 2 {
		t.Fatalf("%+v %v", list, err)
	}
	if list[0].ID != "M1" || list[0].Size != 20 || list[0].Path != "/a/1.mp4" || !list[0].HasVideo || list[0].HasAudio {
		t.Fatalf("按 probed_at 倒序且已更新: %+v", list[0])
	}
	if list[1].ID != "M2" || !list[1].HasAudio || list[1].HasVideo {
		t.Fatalf("%+v", list[1])
	}
	if l, _ := s.ListRecentMedia(ctx, 1); len(l) != 1 {
		t.Fatalf("limit: %d", len(l))
	}
	if err := s.DeleteMedia(ctx, []string{"M1", "nope"}); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.ListRecentMedia(ctx, 0); len(list) != 1 || list[0].ID != "M2" {
		t.Fatalf("%+v", list)
	}
	if err := s.DeleteMedia(ctx, nil); err != nil {
		t.Fatal(err)
	}
}
