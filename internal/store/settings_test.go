package store

import (
	"context"
	"testing"
)

func TestSettingRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)

	var str string
	if found, err := s.GetSetting(ctx, "ffmpegPath", &str); err != nil || found {
		t.Fatalf("不存在的键应返回 found=false, got found=%v err=%v", found, err)
	}

	if err := s.SetSetting(ctx, "ffmpegPath", "/opt/ffmpeg"); err != nil {
		t.Fatal(err)
	}
	if found, err := s.GetSetting(ctx, "ffmpegPath", &str); err != nil || !found || str != "/opt/ffmpeg" {
		t.Fatalf("读回不一致: found=%v str=%q err=%v", found, str, err)
	}

	// 覆盖写
	if err := s.SetSetting(ctx, "ffmpegPath", "/usr/bin"); err != nil {
		t.Fatal(err)
	}
	s.GetSetting(ctx, "ffmpegPath", &str)
	if str != "/usr/bin" {
		t.Fatalf("覆盖失败: %q", str)
	}

	var b bool
	if err := s.SetSetting(ctx, "ffmpegPromptDismissed", true); err != nil {
		t.Fatal(err)
	}
	if found, _ := s.GetSetting(ctx, "ffmpegPromptDismissed", &b); !found || !b {
		t.Fatalf("bool 设置读回不一致: %v", b)
	}
}
