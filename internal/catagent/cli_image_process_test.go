package catagent

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	mode := os.Getenv("FFC_CLI_IMAGE_PROCESS")
	if mode == "" {
		return
	}
	var input json.RawMessage
	if mode == "claude" {
		line, err := bufio.NewReader(os.Stdin).ReadBytes('\n')
		if err != nil {
			os.Exit(2)
		}
		input = line
	}
	raw, _ := json.Marshal(struct {
		Args  []string
		Input json.RawMessage
	}{os.Args[1:], input})
	if err := os.WriteFile(os.Getenv("FFC_CLI_IMAGE_CAPTURE"), raw, 0600); err != nil {
		os.Exit(3)
	}
	if mode == "codex" {
		fmt.Println(`{"type":"thread.started","thread_id":"image-session"}`)
		fmt.Println(`{"type":"item.completed","item":{"id":"reply","type":"agent_message","text":"image received"}}`)
		fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}`)
	} else {
		fmt.Println(`{"type":"result","session_id":"image-session","subtype":"success","result":"image received","structured_output":{"memberIds":["m1"],"reason":"image received","everyone":false,"tasks":[]}}`)
	}
	os.Exit(0)
}

func TestCLIImages_ExecTransportSendsNativeImages(t *testing.T) {
	for _, kind := range []string{"codex", "claude"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "image.png")
			raw, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a2ioAAAAASUVORK5CYII=")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			capture := filepath.Join(t.TempDir(), "capture.json")
			t.Setenv("FFC_CLI_IMAGE_PROCESS", kind)
			t.Setenv("FFC_CLI_IMAGE_CAPTURE", capture)
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cfg := CLIAgentConfig{DataRoot: t.TempDir(), DataTemp: t.TempDir(), Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
				return exec.CommandContext(ctx, exe, args...)
			}}
			a := NewCodexAdapter(cfg)
			if kind == "claude" {
				a = NewClaudeAdapter(cfg)
			}
			a.st, a.exe, a.version, a.checked = Status{State: StateReady}, exe, "2.1.300", true
			turns := 2
			if kind == "claude" {
				turns = 3
			}
			for turn := 0; turn < turns; turn++ {
				o := TurnOptions{Ctx: context.Background(), ConversationID: "images", ProjectPath: t.TempDir(), Messages: []WireMessage{{Role: "user", Content: "describe this"}}, Files: []PromptFile{{Name: "image.png", Path: path}}}
				if kind == "claude" && turn == 1 {
					o.JSONSchema = RouteSchema
				}
				if kind == "claude" && turn == 2 {
					o.LiveSteer, o.Transport = true, "stream-json"
				}
				resp, err := a.RunTurn(o)
				if err != nil || !strings.Contains(resp.Message.Content, "image received") {
					t.Fatalf("turn %d: %+v %v", turn, resp, err)
				}
				var got struct {
					Args  []string
					Input struct {
						Message struct {
							Content []struct {
								Type   string
								Text   string
								Source struct {
									Type      string
									MediaType string `json:"media_type"`
									Data      string
								}
							}
						}
					}
				}
				body, err := os.ReadFile(capture)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatal(err)
				}
				if kind == "codex" {
					joined := strings.Join(got.Args, "\n")
					if !strings.Contains(joined, "--image\n"+path+"\n--\n") {
						t.Fatalf("native image missing or prompt consumed: %v", got.Args)
					}
					if turn == 1 && !strings.Contains(joined, "resume") {
						t.Fatal("resume not exercised")
					}
				} else {
					if !strings.Contains(strings.Join(got.Args, " "), "--input-format stream-json") {
						t.Fatal("image input format missing")
					}
					if !strings.Contains(strings.Join(got.Args, " "), "--output-format stream-json") {
						t.Fatal("Claude image input requires stream-json output")
					}
					if len(got.Input.Message.Content) != 2 {
						t.Fatalf("native image content missing: %s", body)
					}
					image := got.Input.Message.Content[1]
					if image.Type != "image" || image.Source.Type != "base64" || image.Source.MediaType != "image/png" || image.Source.Data != base64.StdEncoding.EncodeToString(raw) {
						t.Fatalf("image payload corrupted: %s", body)
					}
					if got.Input.Message.Content[0].Text != "describe this" {
						t.Fatal("text lost")
					}
				}
			}
		})
	}
}
