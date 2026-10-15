package i18n

var catalogJa = map[string]string{
	KeyDialogPickDirectory:         "フォルダーを選択",
	KeyDialogPickFiles:             "ファイルを選択",
	KeyDialogConfirmReplaceTitle:   "置き換えの確認",
	KeyDialogConfirmReplaceMessage: "「{name}」は既に存在します。置き換えますか？",
	KeyDialogReplace:               "置き換える",
	KeyDialogDontReplace:           "置き換えない",

	KeyNotifyScheduleDoneTitle:   "スケジュールタスク完了：{name}",
	KeyNotifyScheduleFailedTitle: "スケジュールタスク失敗：{name}",
	KeyNotifyScheduleBody:        "所要時間 {min} 分 {sec} 秒。",
	KeyNotifyScheduleBodyProject: "{project} · 所要時間 {min} 分 {sec} 秒。",
	KeyNotifyScheduleOpenConv:    "ここをタップして会話を開く。",

	KeyNotifyTurnDoneTitle:   "Agent · 会話が完了しました",
	KeyNotifyTurnFailedTitle: "Agent · 返信に失敗しました",
	KeyNotifyTurnNewConv:     "新しい会話",
}
