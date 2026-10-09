package doccomp

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type xcuItems struct {
	Items []struct {
		Path string `xml:"path,attr"`
		Prop struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value"`
		} `xml:"prop"`
	} `xml:"item"`
}

func readXCU(t *testing.T, p string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var x xcuItems
	if err := xml.Unmarshal(b, &x); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	m := map[string]string{}
	for _, it := range x.Items {
		m[it.Path+"/"+it.Prop.Name] = strings.TrimSpace(it.Prop.Value)
	}
	return m
}

var wantKeys = map[string]string{
	"/org.openoffice.Office.Common/Security/Scripting/MacroSecurityLevel":     "3",
	"/org.openoffice.Office.Common/Security/Scripting/DisableMacrosExecution": "true",
	"/org.openoffice.Office.Writer/Content/Update/Link":                       "0",
	"/org.openoffice.Office.Calc/Content/Update/Link":                         "1",
}

func checkKeys(t *testing.T, m map[string]string) {
	t.Helper()
	for k, v := range wantKeys {
		if m[k] != v {
			t.Errorf("%s = %q，应为 %q", k, m[k], v)
		}
	}
}

func TestSeedProfileWritesSecurityKeys(t *testing.T) {
	prof := filepath.Join(t.TempDir(), "profile")
	if err := SeedProfile(prof); err != nil {
		t.Fatal(err)
	}
	checkKeys(t, readXCU(t, filepath.Join(prof, "user", "registrymodifications.xcu")))
}

// Convert 每次都在任务的临时配置目录里预置这个文件（用假组件看启动时文件已经在）。
func TestConvertSeedsProfileBeforeStart(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "fake")
	// 假组件：把启动时看到的 xcu 复制成输出
	os.WriteFile(exe, []byte("#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in -env:UserInstallation=file://*) p=\"${a#-env:UserInstallation=file://}\";; esac; done\nprev=\"\"\nfor a in \"$@\"; do if [ \"$prev\" = \"--outdir\" ]; then out=\"$a\"; fi; prev=\"$a\"; done\ncp \"$p/user/registrymodifications.xcu\" \"$out/in.pdf\"\n"), 0o755)
	in := filepath.Join(dir, "in.txt")
	os.WriteFile(in, []byte("x"), 0o644)
	out, err := Convert(context.Background(), Job{Exe: exe, Input: in, ConvertTo: "pdf", WorkDir: filepath.Join(dir, "w"), OutExt: "pdf", Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	checkKeys(t, readXCU(t, out))
}

// 真组件：转换之后组件会改写 registrymodifications.xcu，我们写的值要原样保留（说明组件认了这些键）。
func TestRealProfileKeysHonored(t *testing.T) {
	exe := realSoffice(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "a.txt")
	os.WriteFile(in, []byte("hello"), 0o644)
	w := filepath.Join(dir, "w")
	if _, err := Convert(context.Background(), Job{Exe: exe, Input: in, ConvertTo: "pdf:writer_pdf_Export", InFilter: "Text (encoded):UTF8", WorkDir: w, OutExt: "pdf", Timeout: 2 * time.Minute}); err != nil {
		t.Fatal(err)
	}
	checkKeys(t, readXCU(t, filepath.Join(w, "profile", "user", "registrymodifications.xcu")))
}
