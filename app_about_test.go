package main

import (
	"testing"

	"FFmpegFree/internal/apperr"
)

// 绑定层薄封装：确认 App 上的两个方法转发到 internal/about。
func TestAppAboutBindings(t *testing.T) {
	a := &App{}
	if s, err := a.GetLicenseText("OFL"); err != nil || s == "" {
		t.Fatalf("GetLicenseText(OFL): %v", err)
	}
	if _, err := a.GetLicenseText("ofl"); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("未知名字应 INVALID_ARGUMENT: %v", err)
	}
	if v := a.GetAppVersion(); v != "开发版" {
		t.Fatalf("默认版本应为 开发版: %q", v)
	}
}
