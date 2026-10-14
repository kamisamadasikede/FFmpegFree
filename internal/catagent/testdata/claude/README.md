# Claude transcript fixtures（v0.45）

- `061a686f-48a9-43ac-91f6-cc737232921d.jsonl`：claude 2.1.296 真机（box，cwd=/tmp/ccprobe，未登录）`claude -p hi --output-format stream-json` 写出的会话文件。含 user `hi` + `isApiErrorMessage` 合成失败行。
- `slash-command-2.1.296.jsonl`：真机斜杠命令会话（meta / command-name）。
- `doc-derived-reply.jsonl`：**文档派生**样本（非真机全文）：按官方 sessions 说明与真机行形，覆盖 sidechain / meta / thinking / tool_use / 同 id 分片合并 / 坏行。标为 doc-derived，不得冒充真机采集。
