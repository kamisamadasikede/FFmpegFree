package catagent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
)

// Exercise the real stdout-reading path with a native subprocess on Windows too.
// The shell-script adapter tests skip Windows and previously covered memberId only.
func TestBuildAdapter_RunTurn_StructuredRoute(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stdout   string
		members  []string
		everyone bool
	}{
		{"bare multi-member", `{"memberIds":["m1","m2"],"reason":"一起介绍"}`, []string{"m1", "m2"}, false},
		{"wrapped multi-member", `{"text":"","structured_output":{"memberIds":["m1","m2"],"reason":"一起介绍"}}`, []string{"m1", "m2"}, false},
		{"wrapped everyone", `{"text":"","structured_output":{"everyone":true,"reason":"全员介绍"}}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			a := NewBuildAdapter(BuildConfig{
				DataTemp: root, SessionsRoot: root,
				Exec: func(ctx context.Context, _ string, args ...string) *exec.Cmd {
					if i := slices.Index(args, "--json-schema"); i < 0 || i+1 >= len(args) || args[i+1] != RouteSchema {
						t.Fatalf("missing route schema: %v", args)
					}
					if i := slices.Index(args, "--output-format"); i < 0 || i+1 >= len(args) || args[i+1] != "json" {
						t.Fatalf("structured call must request JSON: %v", args)
					}
					return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStructuredRouteProcessHelper$", "--", "ffc-structured-route-helper", tc.stdout)
				},
			})
			a.exe, a.version, a.st.State = os.Args[0], "1.0.52", StateReady
			resp, err := a.RunTurn(TurnOptions{
				Ctx: context.Background(), ConversationID: "structured-route-test", ProjectPath: root,
				JSONSchema: RouteSchema, Ephemeral: true, NoTools: true, Timeout: 10 * time.Second,
				Messages: []WireMessage{{Role: "user", Content: "你们好，介绍一下自己"}},
			})
			if err != nil {
				t.Fatalf("valid route stdout rejected: %v", err)
			}
			var got struct {
				MemberIDs []string `json:"memberIds"`
				Everyone  bool     `json:"everyone"`
			}
			if err := json.Unmarshal([]byte(resp.Message.Content), &got); err != nil {
				t.Fatalf("invalid route body %q: %v", resp.Message.Content, err)
			}
			if !slices.Equal(got.MemberIDs, tc.members) || got.Everyone != tc.everyone {
				t.Fatalf("route body lost targets: %s", resp.Message.Content)
			}
		})
	}
}

func TestStructuredRouteProcessHelper(t *testing.T) {
	for i, arg := range os.Args {
		if arg == "ffc-structured-route-helper" && i+1 < len(os.Args) {
			_, _ = os.Stdout.WriteString(os.Args[i+1] + "\n")
			os.Exit(0)
		}
	}
}
