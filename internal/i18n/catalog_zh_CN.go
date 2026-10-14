package i18n

var catalogZhCN = map[string]string{
	KeyDialogPickDirectory:         "选择文件夹",
	KeyDialogPickFiles:             "选择文件",
	KeyDialogConfirmReplaceTitle:   "确认替换",
	KeyDialogConfirmReplaceMessage: "“{name}”已存在，要替换它吗？",
	KeyDialogReplace:               "替换",
	KeyDialogDontReplace:           "不替换",

	KeyNotifyScheduleDoneTitle:   "定时任务已完成：{name}",
	KeyNotifyScheduleFailedTitle: "定时任务运行失败：{name}",
	KeyNotifyScheduleBody:        "用时 {min} 分 {sec} 秒。",
	KeyNotifyScheduleBodyProject: "{project} · 用时 {min} 分 {sec} 秒。",
	KeyNotifyScheduleOpenConv:    "点这里打开对话。",

	KeyNotifyTurnDoneTitle:   "Cat · 对话已完成",
	KeyNotifyTurnFailedTitle: "Cat · 回复失败",
	KeyNotifyTurnNewConv:     "新对话",
}
