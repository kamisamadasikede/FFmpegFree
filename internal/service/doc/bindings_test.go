package doc

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// 核对 Wails 生成的 frontend/wailsjs/go/models.ts 里 PDFChunk.data 的类型（契约 6.12.4 第 2 点要求实现 PR 以生成结果为准）。
// 实测（Wails v2.11.0）：Go []byte 在 models.ts 里生成的是 `number[]`，不是契约写的 `string`；
// 但运行时 JSON 是 base64 字符串（见 TestChunkDataIsBase64InJSON），生成的构造函数只是 `this.data = source["data"]`，不做转换。
// 所以前端必须按“运行时是 base64 字符串”解码，并把类型断言成 string（`chunk.data as unknown as string`）。
// 本测试在生成结果变化（例如 Wails 升级后改成 string）时提醒同步契约和前端。
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
	switch m[1] {
	case "number[]":
		t.Logf("生成的 PDFChunk.data 类型是 number[]（与契约写的 string 不一致）；运行时是 base64 字符串，前端需 `as unknown as string` 后 atob")
	case "string":
	default:
		t.Fatalf("PDFChunk.data 的生成类型 %q 出乎意料", m[1])
	}
	if !strings.Contains(src[i:], `this.data = source["data"];`) {
		t.Fatal("生成的构造函数会转换 data，前端解码方式要重新确认")
	}
}
