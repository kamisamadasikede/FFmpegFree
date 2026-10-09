package doc

import (
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"FFmpegFree/internal/apperr"
)

// ---------- 简易转换新增的输入（契约 v0.26，6.12.21）：odt 读 content.xml 的段落文字，txt 按 6.12.17 解码后逐行 ----------

func extractODT(ctx context.Context, path string) (*docModel, error) {
	zr, err := openZip(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	f := findEntry(zr, "content.xml")
	if f == nil {
		return nil, errCorrupt("missing=content.xml")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, errCorrupt("content.xml")
	}
	defer rc.Close()
	dec := xml.NewDecoder(bufio.NewReader(io.LimitReader(rc, maxEntryBytes)))
	m := newModel(12)
	var cur strings.Builder
	depth := 0 // 在 text:p / text:h 里的层数
	for i := 0; ; i++ {
		if i%2000 == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errCorrupt("content.xml")
		}
		switch v := tok.(type) {
		case xml.StartElement:
			switch v.Name.Local {
			case "p", "h":
				depth++
			case "s":
				if depth > 0 {
					cur.WriteByte(' ')
				}
			case "tab":
				if depth > 0 {
					cur.WriteByte('\t')
				}
			case "line-break":
				if depth > 0 {
					cur.WriteByte(' ')
				}
			}
		case xml.CharData:
			if depth > 0 {
				cur.Write(v)
			}
		case xml.EndElement:
			if (v.Name.Local == "p" || v.Name.Local == "h") && depth > 0 {
				depth--
				if depth == 0 {
					if t := strings.TrimSpace(cur.String()); t != "" {
						if err := m.add(unit{kind: unitPara, text: m.clean(t), tick: true}); err != nil {
							return nil, err
						}
					}
					cur.Reset()
				}
			}
		}
	}
	return m, nil
}

func extractTXT(ctx context.Context, path string) (*docModel, error) {
	b, err := readTextFile(path)
	if err != nil {
		return nil, err
	}
	m := newModel(12)
	for i, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if i%2000 == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if t := strings.TrimRight(l, " \t\r"); strings.TrimSpace(t) != "" {
			if err := m.add(unit{kind: unitPara, text: m.clean(t), tick: true}); err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

var _ = errors.New
var _ = apperr.Internal
