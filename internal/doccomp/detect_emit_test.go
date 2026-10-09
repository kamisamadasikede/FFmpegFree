package doccomp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

// 检测结束（ready / missing / outdated）都发 doc:component，前端据此重拉格式表。
func TestDetectEmitsComponentOnCompletion(t *testing.T) {
	cases := []struct {
		want string
		ver  string
		err  error
	}{
		{StateReady, "7.6.4.1", nil},
		{StateMissing, "", installFailed("check=smoke")},
		{StateOutdated, "7.1.0.3", installFailed("check=outdated")},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			exe := filepath.Join(t.TempDir(), "soffice")
			os.WriteFile(exe, []byte("x"), 0o755)
			release := make(chan struct{})
			ev := &events{}
			m := New(Config{Dir: t.TempDir(), Emit: ev.emit, Candidates: func() []string { return []string{exe} },
				Validate: func(ctx context.Context, e, tmp string) (string, error) {
					<-release
					return c.ver, c.err
				}})
			m.Start()
			// 检测中：Wait(0) 立即返回 checking，不等
			start := time.Now()
			if s := m.Wait(context.Background(), 0); s.State != StateChecking || time.Since(start) > 50*time.Millisecond {
				t.Fatalf("%+v", s)
			}
			close(release)
			s := waitState(t, m, c.want)
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				ev.mu.Lock()
				var got *Status
				for i, n := range ev.name {
					if n == EventComponent {
						if st, ok := ev.list[i].(Status); ok && st.State == c.want {
							got = &st
						}
					}
				}
				ev.mu.Unlock()
				if got != nil {
					if got.ComponentState != c.want {
						t.Fatalf("%+v", got)
					}
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatalf("没有收到 %s 的 doc:component（%+v）", c.want, s)
		})
	}
	_ = apperr.Internal
}
