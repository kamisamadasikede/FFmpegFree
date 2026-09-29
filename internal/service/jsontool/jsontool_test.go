package jsontool

import (
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestFormatKeepsOrderAndPrecision(t *testing.T) {
	in := `{"z":1,"a":12345678901234567890,"m":0.10}`
	got, err := Format(FormatRequest{Json: in, Indent: 2})
	if err != nil || got.Error != "" {
		t.Fatalf("err=%v resp=%+v", err, got)
	}
	want := "{\n  \"z\": 1,\n  \"a\": 12345678901234567890,\n  \"m\": 0.10\n}"
	if got.Formatted != want {
		t.Fatalf("格式化结果不对:\n%s", got.Formatted)
	}
	c, _ := Format(FormatRequest{Json: "{\n \"a\" : [1, 2]\n}", Compact: true})
	if c.Formatted != `{"a":[1,2]}` {
		t.Fatalf("压缩结果不对: %s", c.Formatted)
	}
}

func TestFormatEmptyIsInvalidArgument(t *testing.T) {
	if _, err := Format(FormatRequest{Json: "  "}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("期望 INVALID_ARGUMENT，实际 %v", err)
	}
}

func TestSyntaxErrorPosition(t *testing.T) {
	cases := []struct {
		in   string
		line int
		col  int
	}{
		{"{\n  \"名字\": \"张三\",\n  \"age\": ,\n}", 3, 10},
		{`{"a":1}}`, 1, 8},
		{`{"a":1} {"b":2}`, 1, 9},
	}
	for _, c := range cases {
		r, err := Validate(ValidateRequest{Json: c.in})
		if err != nil || r.Valid {
			t.Fatalf("%q 应无效: err=%v resp=%+v", c.in, err, r)
		}
		if r.ErrorPos.Line != c.line || r.ErrorPos.Column != c.col {
			t.Fatalf("%q 错误位置期望 %d:%d，实际 %d:%d (%s)", c.in, c.line, c.col, r.ErrorPos.Line, r.ErrorPos.Column, r.Error)
		}
	}
	r, _ := Validate(ValidateRequest{Json: `{"a": [1, 2`})
	if r.Valid || r.ErrorPos.Line != 1 {
		t.Fatalf("不完整输入应报错: %+v", r)
	}
}

func TestCompare(t *testing.T) {
	r, err := Compare(CompareRequest{
		Json1: `{"b":1,"a":{"x":1.0,"y":[1,2]},"c":"old","a.b":1}`,
		Json2: `{"a":{"x":1,"y":[1]},"c":"new","d":true,"a.b":1}`,
	})
	if err != nil || r.Error != "" {
		t.Fatalf("err=%v resp=%+v", err, r)
	}
	want := []Difference{
		{Type: "removed", Path: "a.y[1]", OldValue: "2"},
		{Type: "removed", Path: "b", OldValue: "1"},
		{Type: "modified", Path: "c", OldValue: `"old"`, NewValue: `"new"`},
		{Type: "added", Path: "d", NewValue: "true"},
	}
	if len(r.Differences) != len(want) {
		t.Fatalf("差异数量不对: %+v", r.Differences)
	}
	for i := range want {
		if r.Differences[i] != want[i] {
			t.Fatalf("第 %d 条差异期望 %+v，实际 %+v", i, want[i], r.Differences[i])
		}
	}
	same, _ := Compare(CompareRequest{Json1: `{"a":[1,{"b":null}]}`, Json2: `{"a":[1,{"b":null}]}`})
	if !same.Identical || same.Differences == nil {
		t.Fatalf("相同输入应 identical，且 differences 为空数组而不是 null: %+v", same)
	}
	bad, _ := Compare(CompareRequest{Json1: `{}`, Json2: `{`})
	if bad.Error == "" {
		t.Fatal("第二个 JSON 语法错误应返回 Error")
	}
}
