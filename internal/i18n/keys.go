package i18n

// Catalog keys for backend-only user-facing strings (契约 6.20.3).
// Frontend-mapped errors keep Chinese message as兜底 and are not listed here.
const (
	LangSystem = "system"
	LangZhCN   = "zh-CN"
	LangEn     = "en"
	LangJa     = "ja"

	KeyDialogPickDirectory         = "dialog.pickDirectory"
	KeyDialogPickFiles             = "dialog.pickFiles"
	KeyDialogConfirmReplaceTitle   = "dialog.confirmReplaceTitle"
	KeyDialogConfirmReplaceMessage = "dialog.confirmReplaceMessage"
	KeyDialogReplace               = "dialog.replace"
	KeyDialogDontReplace           = "dialog.dontReplace"

	KeyNotifyScheduleDoneTitle   = "notify.schedule.doneTitle"
	KeyNotifyScheduleFailedTitle = "notify.schedule.failedTitle"
	KeyNotifyScheduleBody        = "notify.schedule.body"
	KeyNotifyScheduleBodyProject = "notify.schedule.bodyProject"
	KeyNotifyScheduleOpenConv    = "notify.schedule.openConv"

	KeyNotifyTurnDoneTitle   = "notify.turn.doneTitle"
	KeyNotifyTurnFailedTitle = "notify.turn.failedTitle"
	KeyNotifyTurnNewConv     = "notify.turn.newConversation"

	// Cat group chat (契约 6.21.15)
	KeyCatGroupReportUnparsed = "cat.group.reportUnparsed"
	// 契约 6.21.17 ④ 5：同一成员正在干活时入队的派活灰字（route.queued=true）。
	KeyCatGroupRouteQueued = "cat.group.routeQueued"
)
