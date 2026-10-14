package catagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// 模型来源快照（testdata/models/，2026-10-14 采集；来源见 /workspace/notes/cli-modes-effort.md「model source」）。

// Codex 0.162.1 `codex debug models --bundled`（与不带 --bundled 输出相同）。快照只保留 slug / display_name /
// default_reasoning_level / supported_reasoning_levels / visibility / priority；完整输出 sha256
// 69af12f94a39dabd1a1ded8f5257b4884d2118902ea2b5b9cffbf4a1925dfa65。
// 契约 6.19.18.5 Codex 行（原 UNVERIFIED）以此表为准；hide 的模型不下发。
var codexWant = []Model{
	{ID: "gpt-6-astra", Efforts: []string{"low", "medium", "high", "xhigh", "max", "ultra"}, DefaultEffort: "low"},
	{ID: "gpt-6.1-sol", Efforts: []string{"low", "medium", "high", "xhigh", "max", "ultra"}, DefaultEffort: "low"},
	{ID: "gpt-6-sol", Efforts: []string{"low", "medium", "high", "xhigh", "max", "ultra"}, DefaultEffort: "medium"},
	{ID: "gpt-6-luna", Efforts: []string{"low", "medium", "high", "xhigh", "max"}, DefaultEffort: "medium"},
	{ID: "gpt-5.6-sol", Efforts: []string{"low", "medium", "high", "xhigh", "max", "ultra"}, DefaultEffort: "low"},
	{ID: "gpt-5.6-terra", Efforts: []string{"low", "medium", "high", "xhigh", "max", "ultra"}, DefaultEffort: "medium"},
	{ID: "gpt-5.6-luna", Efforts: []string{"low", "medium", "high", "xhigh", "max"}, DefaultEffort: "medium"},
	{ID: "gpt-5.5", Efforts: []string{"low", "medium", "high", "xhigh"}, DefaultEffort: "medium"},
}

func TestModelSnapshot_Codex(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "models", "codex-debug-models-0.162.1.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := parseCodexModelsJSON(raw)
	want := make([]Model, len(codexWant))
	for i, m := range codexWant {
		m.DisplayName = m.ID
		want[i] = m
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("codex catalog drifted:\n got %+v\nwant %+v", got, want)
	}
	// 交叉核对：快照里每个 list 模型都在表里，hide 的都不在。
	var cat struct {
		Models []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &cat); err != nil {
		t.Fatal(err)
	}
	hidden := 0
	for _, m := range cat.Models {
		_, in := FindModel(got, m.Slug)
		if (m.Visibility == "hide") == in {
			t.Errorf("%s visibility=%s listed=%v", m.Slug, m.Visibility, in)
		}
		if m.Visibility == "hide" {
			hidden++
		}
	}
	if hidden != 3 {
		t.Errorf("hidden=%d want 3 (gpt-daybreak-blue-latest, gpt-daybreak-red-latest, codex-auto-review)", hidden)
	}
}

// Grok Build 1.0.40 `grok models`（未登录）。档位来自二进制内嵌模型目录（契约 6.19.18.5）。
func TestModelSnapshot_Grok(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "models", "grok-models.txt"))
	if err != nil {
		t.Fatal(err)
	}
	ids := parseGrokModelsList(string(raw))
	if !reflect.DeepEqual(ids, []string{"grok-4.6", "grok-4.5"}) {
		t.Fatalf("grok models: %q", ids)
	}
	for _, id := range ids {
		m := grokModelWithEfforts(id)
		if m.DefaultEffort != "high" || !ModelSupportsEffort(m, "high") {
			t.Errorf("%s: %+v", id, m)
		}
	}
	if ModelSupportsEffort(grokModelWithEfforts("grok-4.5"), "xhigh") {
		t.Error("grok-4.5 must not list xhigh")
	}
	if m := grokModelWithEfforts("grok-unknown"); m.Efforts == nil || len(m.Efforts) != 0 || m.DefaultEffort != "" {
		t.Errorf("unknown model must be []: %+v", m)
	}
}

// Claude Code 2.1.296：`claude -p "/model"` 输出（未登录）只列别名；我们的静态表里每个别名都必须在里面。
func TestModelSnapshot_Claude(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "models", "claude-2.1.296-slash-model.txt"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	i := strings.Index(s, "Available:")
	if i < 0 {
		t.Fatalf("snapshot shape: %q", s)
	}
	avail := map[string]bool{}
	for _, f := range strings.Split(s[i+len("Available:"):], ",") {
		avail[strings.TrimSpace(f)] = true
	}
	ms := claudeStaticModels()
	want := []Model{
		{ID: "opus", DisplayName: "Opus 5.5", Efforts: claudeFull5, DefaultEffort: "medium"},
		{ID: "sonnet", DisplayName: "Sonnet 5.5", Efforts: claudeFull5, DefaultEffort: "medium"},
		{ID: "haiku", DisplayName: "Haiku 5.5", Efforts: claudeFull5, DefaultEffort: "medium"},
		{ID: "fable", DisplayName: "Fable 5.1", Efforts: claudeFull5, DefaultEffort: "high"},
	}
	if !reflect.DeepEqual(ms, want) {
		t.Fatalf("claude models %+v", ms)
	}
	for _, m := range ms {
		if !strings.HasPrefix(m.ID, "claude-") && !avail[m.ID] {
			t.Errorf("alias %q not in claude /model output", m.ID)
		}
		if m.Efforts == nil {
			t.Errorf("%s: efforts must be [] not nil", m.ID)
		}
		if m.DefaultEffort != "" && !ModelSupportsEffort(m, m.DefaultEffort) {
			t.Errorf("%s default %q not in %q", m.ID, m.DefaultEffort, m.Efforts)
		}
	}
	cases := map[string]string{
		"opus": "medium", "fable": "high", "claude-opus-4-7": "xhigh", "claude-opus-4-6": "high",
		"claude-sonnet-4-6[1m]": "high", "claude-fable-5-1": "high", "claude-sonnet-4-5-20250929": "",
		"claude-sonnet-5-5": "", // 文档原文无此 id，不按前缀猜成 Sonnet 5
		"gpt-5.5":           "",
	}
	for id, def := range cases {
		e, d := claudeModelEfforts(id)
		if d != def {
			t.Errorf("%s: default %q want %q (efforts %q)", id, d, def, e)
		}
	}
	if e, _ := claudeModelEfforts("claude-opus-4-6"); containsArg(e, "xhigh") {
		t.Error("Opus 4.6 has no xhigh")
	}
}

func TestModes_Contract(t *testing.T) {
	want := map[string][]string{
		KindClaudeCLI: {"manual", "plan", "acceptEdits", "auto", "bypassPermissions"},
		KindCodexCLI:  {"read-only", "workspace-write", "approve-for-me", "bypass"},
		KindCatBuild:  {"default", "acceptEdits", "auto", "always-approve"},
	}
	for kind, ids := range want {
		ms := ModesFor(kind)
		var got []string
		defaults := 0
		for _, m := range ms {
			got = append(got, m.ID)
			if m.IsDefault {
				defaults++
			}
			if (m.Risk == RiskSafe) == m.WritesFiles {
				t.Errorf("%s/%s: risk %s vs writesFiles %v", kind, m.ID, m.Risk, m.WritesFiles)
			}
			if m.Label == "" || m.ShortLabel == "" || m.Description == "" {
				t.Errorf("%s/%s: empty copy", kind, m.ID)
			}
			for _, bad := range []string{"CLI", "--", "grok", "claude", "codex"} {
				if strings.Contains(m.Label+m.ShortLabel+m.Description, bad) {
					t.Errorf("%s/%s copy leaks %q", kind, m.ID, bad)
				}
			}
		}
		if !reflect.DeepEqual(got, ids) || defaults != 1 || !ms[0].IsDefault {
			t.Errorf("%s: %q defaults=%d", kind, got, defaults)
		}
		if ms[len(ms)-1].Risk != RiskDanger {
			t.Errorf("%s: last must be danger", kind)
		}
	}
	for _, c := range []struct{ kind, in, out string }{
		{KindCodexCLI, "plan", "read-only"}, {KindCodexCLI, "normal", "read-only"}, {KindCodexCLI, "auto", "approve-for-me"},
		{KindCodexCLI, "approve", "bypass"}, {KindCodexCLI, "", "read-only"},
		{KindClaudeCLI, "normal", "manual"}, {KindClaudeCLI, "plan", "plan"}, {KindClaudeCLI, "approve", "bypassPermissions"},
		{KindCatBuild, "plan", "default"}, {KindCatBuild, "normal", "default"}, {KindCatBuild, "approve", "always-approve"},
		{KindCatBuild, "dontAsk", "default"}, {KindClaudeCLI, "dontAsk", "manual"}, {KindCodexCLI, "danger-full-access", "read-only"},
	} {
		if got := NormalizeMode(c.kind, c.in); got != c.out {
			t.Errorf("%s %q → %q want %q", c.kind, c.in, got, c.out)
		}
	}
}

// 模式文案必须与契约 6.19.18.3 表格逐字一致（v0.44.1）。
func TestModes_MatchContractTable(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "architecture", "contract.md"))
	if err != nil {
		t.Skip("contract not found")
	}
	s := string(raw)
	a := strings.Index(s, "#### 6.19.18.3 模式表")
	b := strings.Index(s, "#### 6.19.18.3a")
	if a < 0 || b < a {
		t.Fatal("6.19.18.3 not found")
	}
	var rows [][]string
	for _, line := range strings.Split(s[a:b], "\n") {
		if !strings.HasPrefix(line, "| ") || len(line) < 3 || line[2] < '1' || line[2] > '9' {
			continue
		}
		var cells []string
		for _, c := range strings.Split(strings.Trim(line, "|"), "|") {
			cells = append(cells, strings.Trim(strings.TrimSpace(c), "`*"))
		}
		rows = append(rows, cells)
	}
	var all []AgentMode
	for _, k := range []string{KindClaudeCLI, KindCodexCLI, KindCatBuild} {
		all = append(all, ModesFor(k)...)
	}
	if len(rows) != len(all) {
		t.Fatalf("rows %d modes %d", len(rows), len(all))
	}
	for i, m := range all {
		r := rows[i]
		got := []string{m.ID, m.Label, m.ShortLabel, m.Description, boolStr(m.WritesFiles), m.Risk, boolStr(m.IsDefault)}
		if !reflect.DeepEqual(got, r[1:8]) {
			t.Errorf("row %d:\n got %q\nwant %q", i, got, r[1:8])
		}
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// 架构师硬要求：下发的每个 Claude 别名都必须在真机 2.1.296 输出（`claude --help` 或 `claude -p "/model"`）
// 和官方文档摘录里都找得到；显示名必须是文档里该别名在 Anthropic API 上的解析版本。
func TestClaudeAliasesVerified(t *testing.T) {
	read := func(p ...string) string {
		b, err := os.ReadFile(filepath.Join(append([]string{"testdata"}, p...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	help := read("help", "claude.txt")
	slash := read("models", "claude-2.1.296-slash-model.txt")
	doc := read("models", "claude-model-config-doc.txt")
	if !strings.Contains(help, "2.1.296") {
		t.Fatal("help snapshot version")
	}
	for _, m := range claudeStaticModels() {
		q := "'" + m.ID + "'"
		inCLI := strings.Contains(help, q) || strings.Contains(slash, " "+m.ID+",")
		inDoc := strings.Contains(doc, "| `"+m.ID+"` | Uses the ")
		if !inCLI || !inDoc {
			t.Errorf("alias %s not verified: cli=%v doc=%v", m.ID, inCLI, inDoc)
		}
		if !strings.Contains(doc, m.DisplayName) {
			t.Errorf("%s display %q not in doc", m.ID, m.DisplayName)
		}
	}
	if !strings.Contains(doc, "the `fable` alias resolves to Fable 5.1") {
		t.Error("fable → Fable 5.1 quote missing")
	}
}
