package apperr

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestErrorIsJSON(t *testing.T) {
	err := Wrap(IOError, "写入失败", errors.New("disk full"))
	var got map[string]string
	if e := json.Unmarshal([]byte(err.Error()), &got); e != nil {
		t.Fatalf("Error() 不是合法 JSON: %v", e)
	}
	if got["code"] != "IO_ERROR" || got["message"] != "写入失败" || got["detail"] != "disk full" {
		t.Fatalf("字段不对: %v", got)
	}
}

func TestFromAndIs(t *testing.T) {
	base := New(NotFound, "任务不存在")
	wrapped := fmt.Errorf("get task: %w", base)
	if !Is(wrapped, NotFound) {
		t.Fatal("Is 应能穿过 fmt.Errorf 包装")
	}
	if From(wrapped) != base {
		t.Fatal("From 应返回原 AppError")
	}
	if From(errors.New("boom")).Code != Internal {
		t.Fatal("普通 error 应归为 INTERNAL")
	}
	if From(nil) != nil {
		t.Fatal("nil 应返回 nil")
	}
}

func TestLiveCodesMatchContract(t *testing.T) {
	want := map[Code]string{
		LiveURLInvalid:         "LIVE_URL_INVALID",
		LiveConnectFailed:      "LIVE_CONNECT_FAILED",
		LivePushRejected:       "LIVE_PUSH_REJECTED",
		LivePushInterrupted:    "LIVE_PUSH_INTERRUPTED",
		ScreenPermissionDenied: "SCREEN_PERMISSION_DENIED",
		LiveSourceGone:         "LIVE_SOURCE_GONE",
		ProbeFailed:            "PROBE_FAILED",
		ConvertDiskFull:        "CONVERT_DISK_FULL",
		Unsupported:            "UNSUPPORTED",
	}
	for c, s := range want {
		if string(c) != s {
			t.Fatalf("%q != %q", c, s)
		}
	}
}

func TestParamsJSONRoundTrip(t *testing.T) {
	err := New(InvalidArgument, "语言设置不正确").
		WithDetail("reason=bad_language").
		WithParams(map[string]string{"lang": "fr"})
	raw := err.Error()
	var got AppError
	if e := json.Unmarshal([]byte(raw), &got); e != nil {
		t.Fatalf("unmarshal: %v", e)
	}
	if got.Code != InvalidArgument || got.Message != "语言设置不正确" || got.Detail != "reason=bad_language" {
		t.Fatalf("fields: %+v", got)
	}
	if got.Params["lang"] != "fr" {
		t.Fatalf("params: %v", got.Params)
	}

	// 旧格式（无 params）兼容
	old := `{"code":"NOT_FOUND","message":"找不到","detail":"x"}`
	var legacy AppError
	if e := json.Unmarshal([]byte(old), &legacy); e != nil {
		t.Fatalf("legacy: %v", e)
	}
	if legacy.Params != nil {
		t.Fatalf("legacy params should be nil: %v", legacy.Params)
	}
	// 再序列化不应带出 params
	b, _ := json.Marshal(&legacy)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if _, ok := m["params"]; ok {
		t.Fatalf("omit empty params failed: %s", b)
	}
}
