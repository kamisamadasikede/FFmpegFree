package catagent

import "strings"

// Claude Code 2.1.296 模型列表（v0.44，6.19.18.5）。
//
// CLI 没有列模型的子命令或开关（`claude --help`，2.1.296）。`claude -p "/model"` 只打印一行别名清单
// （testdata/models/claude-2.1.296-slash-model.txt，未登录也能跑），不含完整 id，也不含档位，
// 而且每跑一次会多一个本地会话文件，所以运行时不调用它，只在测试里用快照核对别名。
// 因此这里是静态表：
//   - 只下发别名：sonnet / opus / haiku / fable —— `/model` 输出与 D https://code.claude.com/docs/en/model-config
//     「Model aliases」表；别名解析到的版本按「Anthropic API」一行（opus→Opus 5.5、sonnet→Sonnet 5.5、
//     haiku→Haiku 5.5、fable→Fable 5.1）。其他服务商解析结果不同（同页表格），档位按 Anthropic API 给。
//   - 完整 id 不下发（产品 10-14）；家族表里只收 model-config 页原文出现过的 id，供已存完整 id 查档位。
// 档位：D https://code.claude.com/docs/en/model-config「Adjust effort level」：
//   Fable 5.1 / Fable 5、Opus 5.5 / Sonnet 5.5 / Haiku 5.5、Opus 5 / Sonnet 5 / Opus 4.8 / Opus 4.7：low…max；
//   Opus 4.6 / Sonnet 4.6：low, medium, high, max；"Models not listed here do not support effort"。
//   默认："high on every model that supports effort, except that Opus 5.5, Sonnet 5.5, and Haiku 5.5 default to
//   medium, Opus 4.7 defaults to xhigh"。

var (
	claudeFull5 = []string{"low", "medium", "high", "xhigh", "max"}
	claudeNoX   = []string{"low", "medium", "high", "max"}
)

type claudeEffortGroup struct {
	efforts []string
	def     string
}

// claudeModelTable 是档位查表用的家族表（含文档原文出现过的完整 id，供已存的完整 id 校验 / 过滤）。
var claudeModelTable = []struct {
	id    string
	group claudeEffortGroup
}{
	{"opus", claudeEffortGroup{claudeFull5, "medium"}},   // → Opus 5.5
	{"sonnet", claudeEffortGroup{claudeFull5, "medium"}}, // → Sonnet 5.5
	{"haiku", claudeEffortGroup{claudeFull5, "medium"}},  // → Haiku 5.5
	{"fable", claudeEffortGroup{claudeFull5, "high"}},    // → Fable 5.1
	{"claude-fable-5-1", claudeEffortGroup{claudeFull5, "high"}},
	{"claude-fable-5", claudeEffortGroup{claudeFull5, "high"}},
	{"claude-opus-5-5", claudeEffortGroup{claudeFull5, "medium"}},
	{"claude-haiku-5-5", claudeEffortGroup{claudeFull5, "medium"}},
	{"claude-opus-5", claudeEffortGroup{claudeFull5, "high"}},
	{"claude-sonnet-5", claudeEffortGroup{claudeFull5, "high"}},
	{"claude-opus-4-8", claudeEffortGroup{claudeFull5, "high"}},
	{"claude-opus-4-7", claudeEffortGroup{claudeFull5, "xhigh"}}, // 原文见 modelOverrides 示例
	{"claude-opus-4-6", claudeEffortGroup{claudeNoX, "high"}},
	{"claude-sonnet-4-6", claudeEffortGroup{claudeNoX, "high"}},
	{"claude-sonnet-4-5", claudeEffortGroup{nil, ""}}, // 不在档位表 → 不支持
	{"claude-haiku-4-5", claudeEffortGroup{nil, ""}},
}

// claudeListed 是 ListCatModels 下发的 4 个别名（产品 10-14 定：id = 别名，不列完整 id；不预选，
// 模型为空 = 跟随 CLI、不传 --model）。显示名 = 别名在 Anthropic API 上解析到的版本
// （D https://code.claude.com/docs/en/model-config「Model aliases」表：opus→Opus 5.5、sonnet→Sonnet 5.5、
// haiku→Haiku 5.5；「Unless you set ANTHROPIC_DEFAULT_FABLE_MODEL, the fable alias resolves to Fable 5.1」）。
// 注意：其他服务商（Bedrock / Foundry 等）或自定义服务商下别名解析到的版本不同，显示名与档位按 Anthropic API 给。
var claudeListed = []struct{ id, name string }{
	{"opus", "Opus 5.5"},
	{"sonnet", "Sonnet 5.5"},
	{"haiku", "Haiku 5.5"},
	{"fable", "Fable 5.1"},
}

// claudeStaticModels 返回下发的 Claude 模型（4 个别名，带该版本的档位与默认档）。
func claudeStaticModels() []Model {
	out := make([]Model, 0, len(claudeListed))
	for _, r := range claudeListed {
		e, d := claudeModelEfforts(r.id)
		out = append(out, Model{ID: r.id, DisplayName: r.name, Efforts: e, DefaultEffort: d})
	}
	return out
}

// claudeModelEfforts 按 id 匹配家族（6.19.18.5：按 id 前缀匹配，匹配不上 → []）。
// 去掉 `[1m]` 后缀；只接受完全相同或「id-8位以上日期」（如 claude-sonnet-4-5-20250929）。
// 不做更宽的前缀匹配：claude-sonnet-5-5 若按前缀会落进 claude-sonnet-5（默认 high），而 Sonnet 5.5 默认 medium。
func claudeModelEfforts(id string) ([]string, string) {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.TrimSuffix(id, "[1m]")
	best, bestLen := -1, 0
	for i, r := range claudeModelTable {
		if id == r.id || (strings.HasPrefix(r.id, "claude-") && isDateSuffix(id, r.id)) {
			if len(r.id) > bestLen {
				best, bestLen = i, len(r.id)
			}
		}
	}
	if best < 0 {
		return []string{}, ""
	}
	g := claudeModelTable[best].group
	return append([]string{}, g.efforts...), g.def
}

func isDateSuffix(id, base string) bool {
	if !strings.HasPrefix(id, base+"-") {
		return false
	}
	rest := id[len(base)+1:]
	if len(rest) < 8 {
		return false
	}
	for _, c := range rest {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
