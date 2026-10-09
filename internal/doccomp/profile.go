package doccomp

import (
	"os"
	"path/filepath"
)

// profileXCU 是预置进每个任务临时配置目录的 registrymodifications.xcu（安全要求，v0.26.1）：
//   - 宏一律不执行：Common/Security/Scripting 的 MacroSecurityLevel=3（非常高）、DisableMacrosExecution=true；
//   - 外部链接和 OLE 链接加载时不更新：Writer Content/Update/Link=0（sw 的 NEVER=0）、
//     Calc Content/Update/Link=1（Calc 的取值是 0 总是 / 1 从不 / 2 询问，“从不”是 1，不是 0）。
//
// 文件放在 <profile>/user/ 下，组件启动时读取（找不到就用自带默认值：宏级别 2、Calc 询问、Writer 询问）。
const profileXCU = `<?xml version="1.0" encoding="UTF-8"?>
<oor:items xmlns:oor="http://openoffice.org/2001/registry" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<item oor:path="/org.openoffice.Office.Common/Security/Scripting"><prop oor:name="MacroSecurityLevel" oor:op="fuse"><value>3</value></prop></item>
<item oor:path="/org.openoffice.Office.Common/Security/Scripting"><prop oor:name="DisableMacrosExecution" oor:op="fuse"><value>true</value></prop></item>
<item oor:path="/org.openoffice.Office.Writer/Content/Update"><prop oor:name="Link" oor:op="fuse"><value>0</value></prop></item>
<item oor:path="/org.openoffice.Office.Calc/Content/Update"><prop oor:name="Link" oor:op="fuse"><value>1</value></prop></item>
</oor:items>
`

// SeedProfile 在临时配置目录里写好 user/registrymodifications.xcu（每次启动组件前调用）。
func SeedProfile(profile string) error {
	dir := filepath.Join(profile, "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "registrymodifications.xcu"), []byte(profileXCU), 0o644)
}
