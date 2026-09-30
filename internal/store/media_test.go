package store

import (
	"context"
	"encoding/json"
	"strings"
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

func TestMediaTableKeepsMostRecent(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	s.SetMediaKeep(5)
	for i := 1; i <= 12; i++ {
		id := string(rune('A' + i))
		if _, err := s.UpsertMedia(ctx, "/k/"+id, MediaInfo{ID: id, Path: "/k/" + id, Name: id, ProbedAt: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.ListRecentMedia(ctx, 200)
	if len(list) != 5 || list[0].ProbedAt != 12 || list[4].ProbedAt != 8 {
		t.Fatalf("应只保留最近 5 条: %+v", list)
	}
	// 重新探测旧记录（已被删）= 新插入；重新探测保留区内的记录不增加行数
	if _, err := s.UpsertMedia(ctx, "/k/"+string(rune('A'+10)), MediaInfo{ID: "zz", Path: "x", Name: "x", ProbedAt: 20}); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.ListRecentMedia(ctx, 200); len(list) != 5 || list[0].ProbedAt != 20 {
		t.Fatalf("%+v", list)
	}
}

func TestMediaTableDefaultKeepIs1000(t *testing.T) {
	if DefaultMediaKeep != 1000 {
		t.Fatal("契约约定保留 1000 条")
	}
	ctx := context.Background()
	s, _ := openTemp(t)
	tx, _ := s.DB().Begin()
	for i := 0; i < 1100; i++ {
		tx.Exec(`INSERT INTO media (id, path, path_key, name, probed_at) VALUES (?,?,?,?,?)`, i, "p", "k"+string(rune(i+1000)), "n", i)
	}
	tx.Commit()
	if _, err := s.UpsertMedia(ctx, "new", MediaInfo{ID: "NEW", Path: "n", Name: "n", ProbedAt: 5000}); err != nil {
		t.Fatal(err)
	}
	var n int
	s.DB().QueryRow(`SELECT COUNT(*) FROM media`).Scan(&n)
	if n != 1000 {
		t.Fatalf("应保留 1000 条: %d", n)
	}
	var minAt int64
	s.DB().QueryRow(`SELECT MIN(probed_at) FROM media`).Scan(&minAt)
	if minAt != 101 { // 保留 NEW(5000) + 999 条最新的（101..1099）
		t.Fatalf("删的应是最旧的: min=%d", minAt)
	}
}

// hasVideo / hasAudio 始终输出（契约 v0.22）：false 时也要有字段，前端才能用 === false 判断“没有画面 / 没有音轨”。
func TestMediaInfoJSONAlwaysHasVideoAudio(t *testing.T) {
	cases := []struct {
		name string
		in   MediaInfo
		want []string
	}{
		{"全 false", MediaInfo{ID: "M1"}, []string{`"hasVideo":false`, `"hasAudio":false`}},
		{"纯音频", MediaInfo{ID: "M2", HasAudio: true}, []string{`"hasVideo":false`, `"hasAudio":true`}},
		{"无声视频", MediaInfo{ID: "M3", HasVideo: true}, []string{`"hasVideo":true`, `"hasAudio":false`}},
		{"都有", MediaInfo{ID: "M4", HasVideo: true, HasAudio: true}, []string{`"hasVideo":true`, `"hasAudio":true`}},
	}
	for _, c := range cases {
		b, err := json.Marshal(c.in)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range c.want {
			if !strings.Contains(string(b), w) {
				t.Errorf("%s: %s 不含 %s", c.name, b, w)
			}
		}
		var back map[string]any
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"hasVideo", "hasAudio"} {
			if _, ok := back[k].(bool); !ok {
				t.Errorf("%s: %s 应是布尔且存在: %v", c.name, k, back[k])
			}
		}
	}
	// 字段回环：false 也保持 false。
	var m MediaInfo
	if err := json.Unmarshal([]byte(`{"id":"M5","hasVideo":false,"hasAudio":true}`), &m); err != nil || m.HasVideo || !m.HasAudio {
		t.Fatalf("%+v %v", m, err)
	}
}
