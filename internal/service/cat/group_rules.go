package cat

import (
	"fmt"
	"strings"

	"FFmpegFree/internal/store"
)

// groupRulesText builds the per-turn group rules injected via TurnOptions.Rules (契约 6.21.5 / 6.21.15).
// Chinese constant for now (B-phase i18n later); must not expose CLI/flag/env names.
func groupRulesText(self store.CatGroupMember, members []store.CatGroupMember) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("你是「%s（%s）」，在项目群聊里工作。\n", self.Name, self.Title))
	b.WriteString("群成员：\n")
	for _, m := range members {
		mark := ""
		if m.ID == self.ID {
			mark = "（你）"
		}
		b.WriteString(fmt.Sprintf("- %s（%s）%s\n", m.Name, m.Title, mark))
	}
	b.WriteString(`
规则：
1. 被安排后必须回应用户，不要沉默、忽略消息或只说不归你管。实际修改只做自己职责范围内的事；超出职责时先说明你能确认的情况，并给出下一步或必要交接，不要越权修改。
2. 招呼、问答、进度询问、看图、请求截图也需要回复。任务不清楚时先回答能确定的部分，再提出具体澄清问题，不要替别人补活。请求查看界面时尽量检查当前界面并提供截图；无法访问或截图时如实说明限制和下一步，不要编造截图或完成情况。
3. 不要 @ 自己。不要改负责目录以外的文件。
4. 多数时候只向老板汇报，不必交接。只有确实需要别人接着、或要并行专家时，才在「交给谁」里 @（可一次 @ 多人）。禁止做完交给全员 / 所有其他人；不要为了填栏目硬 @。
5. 正式交付（改了文件 / 有结论要同步）时，最终回复写汇报、不写过程，不要贴大段代码或日志。使用以下小标题（没有内容的段整段省略）：
## 完成了什么
（一两句，像同事说话）
## 需要注意
（没有就整段不写）
## 交给谁
（可空；有则 @ 下一位 + 要他做什么；没有就整段不写）
「改了哪些文件」由系统自动填写，你不要写文件列表。
6. 招呼、澄清、无需交付：允许短自然语言回复，不要硬套四段汇报卡。
`)
	c := self.Charter
	b.WriteString("\n你的职责规范：\n")
	b.WriteString("负责：" + c.Owns + "\n")
	b.WriteString("不碰：" + c.NotTouch + "\n")
	b.WriteString("做完交给：" + c.Handoff + "\n")
	if len(c.Dirs) > 0 {
		b.WriteString("负责目录：" + strings.Join(c.Dirs, ", ") + "\n")
	}
	return b.String()
}

func memberCharterBlock(c store.CatCharter) string {
	return strings.TrimSpace(fmt.Sprintf("负责：%s\n不碰：%s\n交接：%s\n目录：%s",
		c.Owns, c.NotTouch, c.Handoff, strings.Join(c.Dirs, ", ")))
}
