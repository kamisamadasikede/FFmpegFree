// Package jsontool 实现 JSON 工具的格式化、比对和校验，纯函数，不落库。
//
// 和 v1 的区别：
//   - 格式化和压缩直接在原始字节上做（json.Indent / json.Compact），保留键的原始顺序，
//     大整数和小数也原样保留，不会被转成 float64 丢精度；
//   - 语法错误位置取自 json.SyntaxError.Offset，按字符（不是字节）计算行列，中文内容也准确；
//   - 比对结果按路径排序，每次输出顺序稳定。
package jsontool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
)

// ErrorPos 是语法错误的位置，行列从 1 开始。
type ErrorPos struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type FormatRequest struct {
	Json    string `json:"json"`
	Indent  int    `json:"indent"`  // 缩进空格数，<=0 时默认 4
	Compact bool   `json:"compact"` // 压缩为单行
}

type FormatResponse struct {
	Formatted string   `json:"formatted"`
	Error     string   `json:"error"`
	ErrorPos  ErrorPos `json:"errorPos"`
}

type CompareRequest struct {
	Json1 string `json:"json1"`
	Json2 string `json:"json2"`
}

type Difference struct {
	Type     string `json:"type"` // added | removed | modified
	Path     string `json:"path"`
	OldValue string `json:"oldValue"`
	NewValue string `json:"newValue"`
}

type CompareResponse struct {
	Identical   bool         `json:"identical"`
	Differences []Difference `json:"differences"`
	Error       string       `json:"error"`
	ErrorPos    ErrorPos     `json:"errorPos"`
}

type ValidateRequest struct {
	Json string `json:"json"`
}

type ValidateResponse struct {
	Valid    bool     `json:"valid"`
	Error    string   `json:"error"`
	ErrorPos ErrorPos `json:"errorPos"`
}

const maxIndent = 16

// Format 格式化或压缩。输入为空返回 INVALID_ARGUMENT；JSON 语法错误放在响应的 Error 字段里，不算调用失败。
func Format(req FormatRequest) (FormatResponse, error) {
	if strings.TrimSpace(req.Json) == "" {
		return FormatResponse{}, apperr.New(apperr.InvalidArgument, "JSON 字符串不能为空")
	}
	if msg, pos, bad := check(req.Json); bad {
		return FormatResponse{Error: "JSON 语法错误: " + msg, ErrorPos: pos}, nil
	}
	var buf bytes.Buffer
	var err error
	if req.Compact {
		err = json.Compact(&buf, []byte(req.Json))
	} else {
		indent := req.Indent
		if indent <= 0 {
			indent = 4
		}
		if indent > maxIndent {
			indent = maxIndent
		}
		err = json.Indent(&buf, []byte(strings.TrimSpace(req.Json)), "", strings.Repeat(" ", indent))
	}
	if err != nil {
		return FormatResponse{}, apperr.Wrap(apperr.Internal, "格式化失败", err)
	}
	return FormatResponse{Formatted: buf.String()}, nil
}

// Validate 校验语法。输入为空视为无效（不报错），和 v1 行为一致。
func Validate(req ValidateRequest) (ValidateResponse, error) {
	if strings.TrimSpace(req.Json) == "" {
		return ValidateResponse{Valid: false, Error: "JSON 字符串不能为空"}, nil
	}
	if msg, pos, bad := check(req.Json); bad {
		return ValidateResponse{Valid: false, Error: msg, ErrorPos: pos}, nil
	}
	return ValidateResponse{Valid: true}, nil
}

// Compare 比对两段 JSON 的结构差异。
func Compare(req CompareRequest) (CompareResponse, error) {
	if strings.TrimSpace(req.Json1) == "" || strings.TrimSpace(req.Json2) == "" {
		return CompareResponse{}, apperr.New(apperr.InvalidArgument, "两个 JSON 字符串都不能为空")
	}
	v1, msg, pos, bad := decode(req.Json1)
	if bad {
		return CompareResponse{Error: "第一个 JSON 语法错误: " + msg, ErrorPos: pos}, nil
	}
	v2, msg, pos, bad := decode(req.Json2)
	if bad {
		return CompareResponse{Error: "第二个 JSON 语法错误: " + msg, ErrorPos: pos}, nil
	}
	diffs := []Difference{}
	compare(v1, v2, "", &diffs)
	return CompareResponse{Identical: len(diffs) == 0, Differences: diffs}, nil
}

// check 只做语法校验。
func check(s string) (msg string, pos ErrorPos, bad bool) {
	_, msg, pos, bad = decode(s)
	return
}

// decode 用 UseNumber 解码，并拒绝 "{} {}" 这种后面还有多余内容的输入。
func decode(s string) (v any, msg string, pos ErrorPos, bad bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, "内容为空", ErrorPos{Line: 1, Column: 1}, true
		}
		return nil, describe(err), position(s, err, dec.InputOffset()), true
	}
	end := int(dec.InputOffset())
	if _, err := dec.Token(); err != io.EOF {
		return nil, "JSON 结束后还有多余内容", offsetToPos(s, skipSpace(s, end)), true
	}
	return v, "", ErrorPos{}, false
}

func describe(err error) string {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "内容不完整，可能缺少右括号或引号"
	}
	return err.Error()
}

func position(s string, err error, fallback int64) ErrorPos {
	var se *json.SyntaxError
	if errors.As(err, &se) {
		// Offset 指向出错字符之后，减 1 落到出错字符本身。
		off := int(se.Offset) - 1
		if off < 0 {
			off = 0
		}
		return offsetToPos(s, off)
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return offsetToPos(s, len(s))
	}
	return offsetToPos(s, int(fallback))
}

func skipSpace(s string, off int) int {
	for off < len(s) && strings.ContainsRune(" \t\r\n", rune(s[off])) {
		off++
	}
	return off
}

// offsetToPos 把字节偏移换算成行列，列按字符计数。
func offsetToPos(s string, off int) ErrorPos {
	if off > len(s) {
		off = len(s)
	}
	prefix := s[:off]
	line := strings.Count(prefix, "\n") + 1
	lineStart := strings.LastIndexByte(prefix, '\n') + 1
	col := utf8.RuneCountInString(prefix[lineStart:]) + 1
	return ErrorPos{Line: line, Column: col}
}

func compare(a, b any, path string, out *[]Difference) {
	switch va := a.(type) {
	case map[string]any:
		vb, ok := b.(map[string]any)
		if !ok {
			*out = append(*out, modified(path, a, b))
			return
		}
		keys := make([]string, 0, len(va)+len(vb))
		for k := range va {
			keys = append(keys, k)
		}
		for k := range vb {
			if _, dup := va[k]; !dup {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := joinKey(path, k)
			x, inA := va[k]
			y, inB := vb[k]
			switch {
			case inA && !inB:
				*out = append(*out, Difference{Type: "removed", Path: p, OldValue: render(x)})
			case !inA && inB:
				*out = append(*out, Difference{Type: "added", Path: p, NewValue: render(y)})
			default:
				compare(x, y, p, out)
			}
		}
	case []any:
		vb, ok := b.([]any)
		if !ok {
			*out = append(*out, modified(path, a, b))
			return
		}
		n := max(len(va), len(vb))
		for i := 0; i < n; i++ {
			p := fmt.Sprintf("%s[%d]", path, i)
			switch {
			case i >= len(va):
				*out = append(*out, Difference{Type: "added", Path: p, NewValue: render(vb[i])})
			case i >= len(vb):
				*out = append(*out, Difference{Type: "removed", Path: p, OldValue: render(va[i])})
			default:
				compare(va[i], vb[i], p, out)
			}
		}
	default:
		if !scalarEqual(a, b) {
			*out = append(*out, modified(path, a, b))
		}
	}
}

// scalarEqual 比较标量；数字按数值比较，所以 1 和 1.0 视为相同。
func scalarEqual(a, b any) bool {
	na, okA := a.(json.Number)
	nb, okB := b.(json.Number)
	if okA && okB {
		if na == nb {
			return true
		}
		fa, errA := na.Float64()
		fb, errB := nb.Float64()
		return errA == nil && errB == nil && fa == fb
	}
	return a == b
}

func modified(path string, a, b any) Difference {
	return Difference{Type: "modified", Path: path, OldValue: render(a), NewValue: render(b)}
}

// render 把值输出成紧凑 JSON，比 v1 的 %v（输出 map[a:1]）更易读。
func render(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// joinKey 生成 a.b 形式的路径；键里有点号、括号或空格时用 ["key"] 形式，避免歧义。
func joinKey(path, key string) string {
	if key == "" || strings.ContainsAny(key, `.[]" `) {
		return fmt.Sprintf(`%s[%q]`, path, key)
	}
	if path == "" {
		return key
	}
	return path + "." + key
}
