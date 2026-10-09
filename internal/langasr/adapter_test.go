package langasr

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

func TestRunASR_FakeBinarySuccess(t *testing.T) {
	root, work, audio := setupASRDirs(t)
	touchEntry(t, root)
	cues, err := RunASR(context.Background(), RunOptions{
		ComponentRoot: root, WorkDir: work, AudioPath: audio, Language: "auto", Tier: TierStandard,
		Exec: fakeASR(t, func(req Request, respPath string) int {
			if req.Version != 1 || req.AudioPath != audio || req.Language != "auto" || req.Tier != TierStandard {
				t.Fatalf("bad req %+v", req)
			}
			writeResp(t, respPath, Response{Version: 1, Cues: []SubtitleCue{{Text: "你好", StartMs: 0, EndMs: 1200}}})
			return 0
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 || cues[0].Text != "你好" || cues[0].ID == "" || cues[0].EndMs != 1200 {
		t.Fatalf("%+v", cues)
	}
}

func TestRunASR_EmptyCues(t *testing.T) {
	root, work, audio := setupASRDirs(t)
	touchEntry(t, root)
	_, err := RunASR(context.Background(), RunOptions{
		ComponentRoot: root, WorkDir: work, AudioPath: audio,
		Exec: fakeASR(t, func(_ Request, respPath string) int {
			writeResp(t, respPath, Response{Version: 1, Cues: nil})
			return 0
		}),
	})
	if !apperr.Is(err, apperr.LangAsrEmpty) || apperr.From(err).Message != MsgEmpty {
		t.Fatalf("%v", err)
	}
}

func TestRunASR_NonZeroExit(t *testing.T) {
	root, work, audio := setupASRDirs(t)
	touchEntry(t, root)
	_, err := RunASR(context.Background(), RunOptions{
		ComponentRoot: root, WorkDir: work, AudioPath: audio, Timeout: 3 * time.Second,
		Exec: fakeASR(t, func(Request, string) int { return 3 }),
	})
	if !apperr.Is(err, apperr.LangAsrFailed) || apperr.From(err).Message != MsgFailed {
		t.Fatalf("%v", err)
	}
}

func TestRunASR_MissingEntry(t *testing.T) {
	_, err := RunASR(context.Background(), RunOptions{
		ComponentRoot: t.TempDir(), WorkDir: t.TempDir(), AudioPath: filepath.Join(t.TempDir(), "a.wav"),
	})
	if !apperr.Is(err, apperr.LangAsrNotReady) {
		t.Fatalf("%v", err)
	}
}

func setupASRDirs(t *testing.T) (root, work, audio string) {
	t.Helper()
	root = t.TempDir()
	work = filepath.Join(t.TempDir(), "work")
	_ = os.MkdirAll(work, 0o755)
	audio = filepath.Join(work, "a.wav")
	if err := os.WriteFile(audio, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, work, audio
}

func touchEntry(t *testing.T, root string) {
	t.Helper()
	name := "asr"
	if runtime.GOOS == "windows" {
		name = "asr.exe"
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte("placeholder"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeResp(t *testing.T, path string, resp Response) {
	t.Helper()
	b, _ := json.Marshal(resp)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeASR 返回一个 Exec：真正拉起 `os.Args[0] -test.run=^$` 太麻烦；
// 这里直接在父进程里完成协议，并返回一个立刻成功/失败的 helper 命令。
func fakeASR(t *testing.T, handle func(req Request, respPath string) int) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	t.Helper()
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		reqPath, respPath := "", ""
		for i := 0; i+1 < len(args); i++ {
			switch args[i] {
			case "--request":
				reqPath = args[i+1]
			case "--response":
				respPath = args[i+1]
			}
		}
		var req Request
		if raw, err := os.ReadFile(reqPath); err == nil {
			_ = json.Unmarshal(raw, &req)
		}
		code := handle(req, respPath)
		// 用 `true`/`false` 或 shell exit：跨平台用 go test 二进制自身不好。
		// 采用 python/ perl 不依赖。用 `sh -c exit N`（Windows 用 cmd）。
		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, "cmd", "/C", "exit", itoa(code))
		}
		return exec.CommandContext(ctx, "sh", "-c", "exit "+itoa(code))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
