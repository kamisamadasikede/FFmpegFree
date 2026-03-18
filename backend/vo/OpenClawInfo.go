package vo

type OpenClawInstallRequest struct {
	PackageName string `json:"packageName"`
	Registry    string `json:"registry"`
}

type OpenClawInstallStep struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Detail    string `json:"detail"`
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt"`
}

type OpenClawInstallLog struct {
	Time    string `json:"time"`
	StepID  string `json:"stepId"`
	Step    string `json:"step"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

type OpenClawInstallStatus struct {
	State      string                `json:"state"`
	Package    string                `json:"package"`
	Progress   int                   `json:"progress"`
	CurrentID  string                `json:"currentId"`
	Current    string                `json:"current"`
	Message    string                `json:"message"`
	Error      string                `json:"error"`
	StartedAt  string                `json:"startedAt"`
	UpdatedAt  string                `json:"updatedAt"`
	FinishedAt string                `json:"finishedAt"`
	Steps      []OpenClawInstallStep `json:"steps"`
	Logs       []OpenClawInstallLog  `json:"logs"`
}

type OpenClawAuthCheckResult struct {
	Installed      bool     `json:"installed"`
	NeedAuth       bool     `json:"needAuth"`
	Provider       string   `json:"provider"`
	MissingAuth    []string `json:"missingAuth"`
	DefaultModel   string   `json:"defaultModel"`
	ConfigureCmd   string   `json:"configureCmd"`
	SetupTokenCmds []string `json:"setupTokenCmds"`
	ModelsOutput   string   `json:"modelsOutput"`
	Error          string   `json:"error"`
	CheckedAt      string   `json:"checkedAt"`
}
