package catagent

import "testing"

func TestCommandHasSession(t *testing.T) {
	const sid = "11111111-2222-5333-8444-555555555555"
	ok := []string{
		`grok -p hi --session-id ` + sid + ` --cwd D:\work`,
		`"C:\Program Files\grok.exe" --resume ` + sid,
		`node grok.js --resume=` + sid,
		`grok --session-id=` + stringsToUpper(sid),
	}
	for _, line := range ok {
		if !commandHasSession(line, sid) {
			t.Fatalf("want match %q", line)
		}
	}
	bad := []string{
		`grok --session-id ` + sid + "0",
		`echo ` + sid,
		`grok --cwd ` + sid,
		"",
	}
	for _, line := range bad {
		if commandHasSession(line, sid) {
			t.Fatalf("want skip %q", line)
		}
	}
}

func TestLiveConvIDsUsesCommandLines(t *testing.T) {
	const conv = "conv-1"
	sid := sessionIDFromConv(conv)
	old := readProcessCommands
	readProcessCommands = func() []processCommand {
		return []processCommand{{pid: 9, line: "grok --resume " + sid + " --cwd D:\\work"}}
	}
	t.Cleanup(func() { readProcessCommands = old })
	got := LiveConvIDs([]string{conv, "other"})
	if len(got) != 1 || got[0] != conv {
		t.Fatalf("got %v", got)
	}
	if LiveConvIDs(nil) != nil {
		t.Fatal("empty input")
	}
}

func stringsToUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}
