# Codex 会话样本出处

- `rollout-2026-10-14T10-37-32-01a13845-….jsonl`：**真机样本**。codex-cli 0.162.1（Linux x64，未登录），
  `CODEX_HOME=/tmp/cxh codex exec --json --skip-git-repo-check "say hi"` 原样写出的
  `$CODEX_HOME/sessions/2026/10/14/rollout-*.jsonl`，未改动。未登录所以只有用户句和 401 失败，没有助手回复。
- `exec-json-unauth-0.162.1.jsonl`：同一次运行 `codex exec --json` 的 stdout 原样（含 `thread.started` 的 thread_id）。
- `rollout-doc-derived-reply.jsonl`：**非真机**。按 codex 0.162.1 二进制里的 rollout 字段名
  （response_item / message / output_text / reasoning / summary_text / function_call / function_call_output）
  和上面真机文件的外层结构手写，补足“助手回复、思考、工具调用”三种行。登录后应换成真机样本。
