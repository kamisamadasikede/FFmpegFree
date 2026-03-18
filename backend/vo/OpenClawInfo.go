package vo

type OpenClawInstallRequest struct {
	PackageName string `json:"packageName"`
	Registry    string `json:"registry"`
}

type OpenClawQuickConfigRequest struct {
	Provider     string `json:"provider"`
	APIKey       string `json:"apiKey"`
	APIBase      string `json:"apiBase"`
	APIKeyEnv    string `json:"apiKeyEnv"`
	APIBaseEnv   string `json:"apiBaseEnv"`
	DefaultModel string `json:"defaultModel"`
	UseGuestMode bool   `json:"useGuestMode"`
	PersistEnv   bool   `json:"persistEnv"`
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

type OpenClawModelItem struct {
	Key       string   `json:"key"`
	Name      string   `json:"name"`
	Available bool     `json:"available"`
	Local     bool     `json:"local"`
	Tags      []string `json:"tags"`
}

type OpenClawQuickConfigResult struct {
	Success         bool                `json:"success"`
	Message         string              `json:"message"`
	Provider        string              `json:"provider"`
	DefaultModel    string              `json:"defaultModel"`
	AvailableCount  int                 `json:"availableCount"`
	GuestModelCount int                 `json:"guestModelCount"`
	GuestModelReady bool                `json:"guestModelReady"`
	AvailableModels []OpenClawModelItem `json:"availableModels"`
	GuestModels     []OpenClawModelItem `json:"guestModels"`
	RawStatusJSON   string              `json:"rawStatusJson"`
	RawListJSON     string              `json:"rawListJson"`
	Steps           []string            `json:"steps"`
	Error           string              `json:"error"`
	CheckedAt       string              `json:"checkedAt"`
}
