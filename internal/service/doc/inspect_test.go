package doc

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestInspectEncryptionAndCorruption(t *testing.T) {
	dir := t.TempDir()
	p := func(n string) string { return filepath.Join(dir, n) }
	ctx := context.Background()
	fib := func(flags uint16) []byte {
		b := make([]byte, 64)
		binary.LittleEndian.PutUint16(b, 0xA5EC)
		binary.LittleEndian.PutUint16(b[0x0A:], flags)
		return b
	}
	bof := [2]any{0x0809, make([]byte, 16)}
	sheet := [2]any{0x0085, make([]byte, 8)}
	eof := [2]any{0x000A, []byte{}}
	cu := func(token uint32) []byte {
		b := make([]byte, 32)
		binary.LittleEndian.PutUint32(b[12:], token)
		return b
	}
	writeCFB(t, p("enc.docx"), cfbStream{"EncryptionInfo", []byte{4, 0, 4, 0}}, cfbStream{"EncryptedPackage", []byte{1}})
	writeCFB(t, p("old.docx"), cfbStream{"WordDocument", fib(0)})
	writeCFB(t, p("enc.doc"), cfbStream{"WordDocument", fib(0x0100 | 0x0200)})
	writeCFB(t, p("ok.doc"), cfbStream{"WordDocument", fib(0x0200)})
	writeCFB(t, p("nowd.doc"), cfbStream{"Data", []byte{1}})
	writeCFB(t, p("enc.xls"), cfbStream{"Workbook", biff(bof, [2]any{0x002F, make([]byte, 54)}, sheet, eof)})
	writeCFB(t, p("ok.xls"), cfbStream{"Workbook", biff(bof, sheet, sheet, sheet, eof, bof, [2]any{0x002F, []byte{}})})
	writeCFB(t, p("book.xls"), cfbStream{"Book", biff(bof, sheet, eof)})
	writeCFB(t, p("enc1.ppt"), cfbStream{"PowerPoint Document", []byte{1}}, cfbStream{"EncryptedSummary", []byte{1}})
	writeCFB(t, p("enc2.ppt"), cfbStream{"Current User", cu(0xF3D1C4DF)}, cfbStream{"PowerPoint Document", []byte{1}})
	writeCFB(t, p("ok.ppt"), cfbStream{"Current User", cu(0xE391C05F)}, cfbStream{"PowerPoint Document", []byte{1}})
	writeZip(t, p("enc.odt"), map[string]string{"mimetype": "application/vnd.oasis.opendocument.text", "content.xml": "xx",
		"META-INF/manifest.xml": `<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"><manifest:file-entry manifest:full-path="content.xml"><manifest:encryption-data manifest:checksum-type="SHA1"/></manifest:file-entry></manifest:manifest>`})
	writeZip(t, p("ok.ods"), map[string]string{"content.xml": `<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"><office:body><office:spreadsheet><table:table table:name="a"/><table:table table:name="b"/></office:spreadsheet></office:body></office:document-content>`,
		"META-INF/manifest.xml": `<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"/>`})
	writeZip(t, p("nocontent.odt"), map[string]string{"META-INF/manifest.xml": "<m/>"})
	makeXlsx(t, p("three.xlsx"), map[string][][]string{"一": {{"1"}}, "二": {{"2"}}, "三": {{"3"}}}, []string{"一", "二", "三"})
	makeDocx(t, p("ok.docx"), "你好")
	writeZip(t, p("nodoc.docx"), map[string]string{"x.xml": "<x/>"})
	os.WriteFile(p("notzip.docx"), []byte("hello world, not a zip"), 0o644)
	os.WriteFile(p("empty.txt"), nil, 0o644)
	os.WriteFile(p("garbage.doc"), []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}, 0o644)
	os.WriteFile(p("rtf.doc"), []byte(`{\rtf1 hello}`), 0o644)
	os.WriteFile(p("html.xls"), []byte(`<html><table><tr><td>1</td></tr></table></html>`), 0o644)
	os.WriteFile(p("a.csv"), []byte("a,b\n"), 0o644)
	os.WriteFile(p("a.md"), []byte("# x\n"), 0o644)

	for _, c := range []struct {
		file, ext string
		code      apperr.Code
		sheets    int
	}{
		{"enc.docx", "docx", apperr.DocEncrypted, 0},
		{"old.docx", "docx", "", 0},
		{"enc.doc", "doc", apperr.DocEncrypted, 0},
		{"ok.doc", "doc", "", 0},
		{"nowd.doc", "doc", apperr.DocCorrupt, 0},
		{"enc.xls", "xls", apperr.DocEncrypted, 0},
		{"ok.xls", "xls", "", 3},
		{"book.xls", "xls", "", 1},
		{"enc1.ppt", "ppt", apperr.DocEncrypted, 0},
		{"enc2.ppt", "ppt", apperr.DocEncrypted, 0},
		{"ok.ppt", "ppt", "", 0},
		{"enc.odt", "odt", apperr.DocEncrypted, 0},
		{"ok.ods", "ods", "", 2},
		{"nocontent.odt", "odt", apperr.DocCorrupt, 0},
		{"three.xlsx", "xlsx", "", 3},
		{"ok.docx", "docx", "", 0},
		{"nodoc.docx", "docx", apperr.DocCorrupt, 0},
		{"notzip.docx", "docx", apperr.DocCorrupt, 0},
		{"empty.txt", "txt", apperr.DocCorrupt, 0},
		{"garbage.doc", "doc", apperr.DocCorrupt, 0},
		{"rtf.doc", "doc", "", 0},
		{"html.xls", "xls", "", -1},
		{"a.csv", "csv", "", 1},
		{"a.md", "md", "", 0},
	} {
		n, err := inspectDoc(ctx, p(c.file), c.ext)
		if c.code == "" {
			if err != nil || n != c.sheets {
				t.Errorf("%s: sheets=%d err=%v，应为 %d", c.file, n, err, c.sheets)
			}
			continue
		}
		if !apperr.Is(err, c.code) {
			t.Errorf("%s: %v，应为 %s", c.file, err, c.code)
		}
	}
}

func TestDecodeTextGB18030(t *testing.T) {
	gbk := []byte{0xC4, 0xE3, 0xBA, 0xC3, ',', 0xCA, 0xC0, 0xBD, 0xE7} // 你好,世界
	if got := string(decodeText(gbk)); got != "你好,世界" {
		t.Fatal(got)
	}
	if got := string(decodeText([]byte("\xEF\xBB\xBF你好"))); got != "你好" {
		t.Fatal(got)
	}
}

func TestFormatMatrix(t *testing.T) {
	m := buildMatrix(false, nil)
	if m.ComponentReady || len(m.Inputs) != 16 || len(m.Sources) != 14 {
		t.Fatalf("%+v", m)
	}
	find := func(m DocFormatMatrix, src, tg string) DocTarget {
		for _, s := range m.Sources {
			if s.Ext == src {
				for _, t := range s.Targets {
					if t.Ext == tg {
						return t
					}
				}
			}
		}
		return DocTarget{Ext: "-"}
	}
	for _, c := range []struct {
		src, tg                    string
		ready, needs, simple, avai bool
		hint                       string
	}{
		{"docx", "pdf", false, true, true, true, HintSimpleMode},
		{"odt", "pdf", false, true, true, true, HintSimpleMode},
		{"txt", "pdf", false, true, true, true, HintSimpleMode},
		{"doc", "pdf", false, true, false, false, ""},
		{"md", "html", false, false, false, true, ""},
		{"html", "md", false, false, false, true, HintMDLossy},
		{"docx", "md", false, true, false, false, HintMDLossy},
		{"xlsx", "csv", true, true, false, true, HintCSVFirstSheet},
		{"csv", "xlsx", true, true, false, true, ""},
		{"docx", "pdf", true, true, false, true, ""},
		{"pptx", "odp", true, true, false, true, ""},
	} {
		got := find(buildMatrix(c.ready, nil), c.src, c.tg)
		if got.NeedsComponent != c.needs || got.Simple != c.simple || got.Available != c.avai || got.HintKey != c.hint ||
			(!got.Available) != (got.DisabledReason == "需要文档组件") {
			t.Errorf("%s→%s ready=%v: %+v", c.src, c.tg, c.ready, got)
		}
	}
	for _, s := range m.Sources {
		if s.Targets[0].Ext != "pdf" {
			t.Errorf("%s: pdf 应在最前", s.Ext)
		}
		for _, tg := range s.Targets {
			if tg.Ext == s.Ext {
				t.Errorf("%s 不应转成自己", s.Ext)
			}
		}
	}
	if find(m, "docx", "xlsx").Ext != "-" || find(m, "csv", "txt").Ext != "-" {
		t.Error("跨类不转")
	}
}
