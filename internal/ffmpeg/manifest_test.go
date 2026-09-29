package ffmpeg

import (
	"errors"
	"strings"
	"testing"
)

func TestDefaultManifest(t *testing.T) {
	m, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"windows-amd64", "darwin-arm64", "darwin-amd64", "linux-amd64", "linux-arm64"} {
		p, ok := m.Platforms[k]
		if !ok || !p.Available {
			t.Fatalf("清单缺少平台 %s", k)
		}
		for _, a := range p.Archives {
			// 固定版本地址：不允许出现滚动地址
			if strings.Contains(a.URL, "latest") {
				t.Fatalf("%s 使用了滚动地址: %s", k, a.URL)
			}
			if a.Size <= 0 {
				t.Fatalf("%s 缺少 size", k)
			}
			for _, it := range a.Extract {
				if it.Name == "" || it.Path == "" {
					t.Fatalf("%s 提取项不完整", k)
				}
			}
		}
		// 每个平台必须同时拿到 ffmpeg 和 ffprobe
		var names []string
		for _, a := range p.Archives {
			for _, it := range a.Extract {
				names = append(names, it.Name)
			}
		}
		joined := strings.Join(names, ",")
		if !strings.Contains(joined, "ffmpeg") || !strings.Contains(joined, "ffprobe") {
			t.Fatalf("%s 没有同时提取 ffmpeg 和 ffprobe: %v", k, names)
		}
	}
}

func TestResolveMirror(t *testing.T) {
	m, _ := DefaultManifest()
	// 默认源
	plan, err := m.Resolve("windows-amd64", "")
	if err != nil || plan.MirrorFallback || len(plan.Sources[0].URLs) != 1 {
		t.Fatalf("%+v %v", plan, err)
	}
	// cn：有镜像的平台，镜像在前，原地址兜底在最后
	plan, err = m.Resolve("windows-amd64", "cn")
	if err != nil || plan.MirrorFallback {
		t.Fatalf("%+v %v", plan, err)
	}
	urls := plan.Sources[0].URLs
	if len(urls) < 2 || urls[len(urls)-1] != plan.Sources[0].Archive.URL || urls[0] == plan.Sources[0].Archive.URL {
		t.Fatalf("镜像顺序不对: %v", urls)
	}
	// cn：没有镜像条目的平台退回默认源
	plan, err = m.Resolve("linux-amd64", "cn")
	if err != nil || !plan.MirrorFallback {
		t.Fatalf("应标记 MirrorFallback: %+v %v", plan, err)
	}
	for _, s := range plan.Sources {
		if len(s.URLs) != 1 || s.URLs[0] != s.Archive.URL {
			t.Fatalf("应只有默认地址: %v", s.URLs)
		}
	}
	// 其他镜像名不合法
	for _, bad := range []string{"CN", "cn ", "https://x.example/", "default", "eu"} {
		if _, err := m.Resolve("linux-amd64", bad); err == nil {
			t.Fatalf("镜像 %q 应被拒绝", bad)
		}
		if ValidMirror(bad) {
			t.Fatalf("ValidMirror(%q)", bad)
		}
	}
}

func TestResolveUnavailable(t *testing.T) {
	m, err := ParseManifest([]byte(`{"schemaVersion":1,"platforms":{"linux-arm64":{"available":false,"note":"暂无固定源","archives":[]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"linux-arm64", "plan9-mips"} {
		_, err = m.Resolve(key, "")
		var u *UnavailableError
		if !errors.As(err, &u) || !IsUnavailable(err) {
			t.Fatalf("%s: %v", key, err)
		}
	}
}

func TestParseManifestValidation(t *testing.T) {
	bad := []string{
		`{"platforms":{"a":{"available":true,"archives":[]}}}`,
		`{"platforms":{"a":{"available":true,"archives":[{"url":"ftp://x","sha256":"` + strings.Repeat("a", 64) + `","type":"zip","extract":[{"path":"p","name":"n"}]}]}}}`,
		`{"platforms":{"a":{"available":true,"archives":[{"url":"https://x","sha256":"abc","type":"zip","extract":[{"path":"p","name":"n"}]}]}}}`,
		`{"platforms":{"a":{"available":true,"archives":[{"url":"https://x","sha256":"` + strings.Repeat("a", 64) + `","type":"7z","extract":[{"path":"p","name":"n"}]}]}}}`,
		`{"platforms":{"a":{"available":true,"archives":[{"url":"https://x","sha256":"` + strings.Repeat("a", 64) + `","type":"zip","extract":[]}]}}}`,
		`not json`,
	}
	for i, b := range bad {
		if _, err := ParseManifest([]byte(b)); err == nil {
			t.Fatalf("case %d 应失败", i)
		}
	}
}
