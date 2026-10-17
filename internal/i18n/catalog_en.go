package i18n

var catalogEn = map[string]string{
	KeyDialogPickDirectory:         "Select Folder",
	KeyDialogPickFiles:             "Select Files",
	KeyDialogConfirmReplaceTitle:   "Replace File",
	KeyDialogConfirmReplaceMessage: "“{name}” already exists. Do you want to replace it?",
	KeyDialogReplace:               "Replace",
	KeyDialogDontReplace:           "Don't Replace",

	KeyNotifyScheduleDoneTitle:   "Scheduled task completed: {name}",
	KeyNotifyScheduleFailedTitle: "Scheduled task failed: {name}",
	KeyNotifyScheduleBody:        "Took {min} min {sec} sec.",
	KeyNotifyScheduleBodyProject: "{project} · Took {min} min {sec} sec.",
	KeyNotifyScheduleOpenConv:    "Click to open the conversation.",

	KeyNotifyTurnDoneTitle:   "Agent · Conversation finished",
	KeyNotifyTurnFailedTitle: "Agent · Reply failed",
	KeyNotifyTurnNewConv:     "New conversation",

	KeyCatGroupReportUnparsed: "Reply was not in report format; showing the last reply only.",
}
