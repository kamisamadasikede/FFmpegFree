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
