package catagent

import "strings"

// 一期只读：headless 下仍传 --always-approve（防挂死），因此必须在命令行上把工具收窄成只读。
//
// 工具名依据（Grok Build CLI 1.0.40，box 上 `grok --help` + 随包 docs/user-guide
// 14-headless-mode.md、22-permissions-and-safety.md、16-subagents.md、07-mcp-servers.md，
// 以及二进制内嵌的 headless 文档「Tool ID for --tools / --disallowed-tools」表）：
//
//	只读：read_file、list_dir、grep
//	写入/编辑：search_replace、write_file（features.write_file / GROK_WRITE_FILE）、apply_patch
//	执行命令：run_terminal_cmd（文档明确：shell 工具 ID 是 run_terminal_cmd，不是 bash）
//	子代理：task；另有 --disallowed-tools 特殊项 Agent 与 --no-subagents
//	网络：web_search、web_fetch；MCP 元工具：search_tool、use_tool
//	生成文件：image_gen、image_edit、video_gen
//
// 注意：CLI 对 --tools 白名单里任何一个无法映射的名字会整体放弃白名单
// （二进制日志串 "tools allowlist had unmappable entries; keeping full grok toolset"），
// 所以白名单只放文档表里确认的三个名字；黑名单里不存在的名字只告警
// （"disallowedTools entry matched nothing"），可以多放作第二道保险。
// 当两者同时出现时 --disallowed-tools 优先。
const (
	CLIToolReadFile = "read_file"
	CLIToolListDir  = "list_dir"
	CLIToolGrep     = "grep"

	CLIToolShell         = "run_terminal_cmd"
	CLIToolSearchReplace = "search_replace"
	CLIToolWriteFile     = "write_file"
	CLIToolApplyPatch    = "apply_patch"
	CLIToolTask          = "task"
	CLIToolAgent         = "Agent"
	CLIToolWebSearch     = "web_search"
	CLIToolWebFetch      = "web_fetch"
	CLIToolMCPSearch     = "search_tool"
	CLIToolMCPUse        = "use_tool"
	CLIToolImageGen      = "image_gen"
	CLIToolImageEdit     = "image_edit"
	CLIToolVideoGen      = "video_gen"

	// 别名 / 其它工具集里的写·执行类名字（第二道保险）。黑名单里认不出的名字只告警，
	// 但这些名字绝不能进白名单（白名单有一个认不出就整体失效）。
	// hashline_edit / run_terminal_command 在 CLI 1.0.40 二进制字符串里出现过；
	// bash / edit / write 是常见别名，是否被 --disallowed-tools 识别未验证。
	CLIToolAliasBash               = "bash"
	CLIToolAliasEdit               = "edit"
	CLIToolAliasWrite              = "write"
	CLIToolHashlineEdit            = "hashline_edit"
	CLIToolRunTerminalCommandAlias = "run_terminal_command"
)

// ReadOnlyCLITools 是传给 --tools 的白名单（只读：读文件 / 列目录 / 搜索）。
var ReadOnlyCLITools = []string{CLIToolReadFile, CLIToolListDir, CLIToolGrep}

// DeniedCLITools 是传给 --disallowed-tools 的黑名单（写入 / 编辑 / 执行 / 子代理 / 网络 / MCP / 生成）。
var DeniedCLITools = []string{
	CLIToolShell, CLIToolSearchReplace, CLIToolWriteFile, CLIToolApplyPatch,
	CLIToolTask, CLIToolAgent,
	CLIToolWebSearch, CLIToolWebFetch,
	CLIToolMCPSearch, CLIToolMCPUse,
	CLIToolImageGen, CLIToolImageEdit, CLIToolVideoGen,
	CLIToolAliasBash, CLIToolAliasEdit, CLIToolAliasWrite,
	CLIToolHashlineEdit, CLIToolRunTerminalCommandAlias,
}

// DenyRules 是 --deny 权限规则（规则层工具类名）。deny 在 always-approve 下仍生效。
var DenyRules = []string{"Bash", "Edit", "Write", "WebFetch", "WebSearch", "MCPTool(*)"}

// readOnlyCLIArgs 返回一期只读限制的命令行参数。
func readOnlyCLIArgs() []string {
	args := []string{
		"--tools", strings.Join(ReadOnlyCLITools, ","),
		"--disallowed-tools", strings.Join(DeniedCLITools, ","),
		"--no-subagents",
		"--disable-web-search",
	}
	for _, r := range DenyRules {
		args = append(args, "--deny", r)
	}
	return args
}
