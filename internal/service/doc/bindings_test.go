package doc

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// 核对 Wails 生成的 frontend/wailsjs/go/models.ts 里 PDFChunk.data 是 string（后端显式 base64，契约 6.12.4）。
// Go 的 []byte 会被生成成 number[]，所以结构体字段必须是 string；这里防止有人改回 []byte 却没重新生成绑定。
func TestGeneratedPDFChunkDataType(t *testing.T) {
	b, err := os.ReadFile("../../../frontend/wailsjs/go/models.ts")
	if err != nil {
		t.Skip("没有 models.ts:", err)
	}
	src := string(b)
	i := strings.Index(src, "export class PDFChunk")
	if i < 0 {
		t.Fatal("models.ts 里没有 PDFChunk（绑定没有重新生成？wails generate module）")
	}
	m := regexp.MustCompile(`data: ([^;]+);`).FindStringSubmatch(src[i:])
	if m == nil {
		t.Fatal("PDFChunk 里没有 data 字段")
	}
	if m[1] != "string" {
		t.Fatalf("PDFChunk.data 的生成类型是 %q，应为 string（Go 字段不要用 []byte）", m[1])
	}
}
