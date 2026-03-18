package contollers

import (
	"FFmpegFree/backend/utils"
	"FFmpegFree/backend/vo"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	stepPending = "pending"
	stepRunning = "running"
	stepSuccess = "success"
	stepFailed  = "failed"
	stepSkipped = "skipped"

	stateIdle    = "idle"
	stateRunning = "running"
	stateSuccess = "success"
	stateFailed  = "failed"

	maxInstallLogs = 300
)

type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
	LookPath(file string) (string, error)
}

type execRunner struct{}

func (r execRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	output := strings.TrimSpace(out.String())
	if err != nil {
		if output == "" {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		return output, fmt.Errorf("%s: %w", name, err)
	}
	return output, nil
}

func (r execRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

type packageInstallCommand struct {
	Name string
	Args []string
	Note string
}

type openClawInstallManager struct {
	mu     sync.RWMutex
	status vo.OpenClawInstallStatus
	cancel context.CancelFunc
	runner commandRunner
}

func newOpenClawInstallManager(runner commandRunner) *openClawInstallManager {
	return &openClawInstallManager{
		status: vo.OpenClawInstallStatus{
			State:   stateIdle,
			Package: "openclaw",
			Steps:   defaultOpenClawSteps(),
			Logs:    make([]vo.OpenClawInstallLog, 0, maxInstallLogs),
		},
		runner: runner,
	}
}

func defaultOpenClawSteps() []vo.OpenClawInstallStep {
	return []vo.OpenClawInstallStep{
		{ID: "detect_system", Title: "Detect OS and package manager", Status: stepPending},
		{ID: "check_node_env", Title: "Check Node/NPM environment", Status: stepPending},
		{ID: "install_node_env", Title: "Install Node/NPM environment", Status: stepPending},
		{ID: "verify_npm", Title: "Verify NPM availability", Status: stepPending},
		{ID: "install_openclaw", Title: "Install OpenClaw package", Status: stepPending},
		{ID: "verify_openclaw", Title: "Verify OpenClaw installation", Status: stepPending},
	}
}

func (m *openClawInstallManager) start(req vo.OpenClawInstallRequest) error {
	packageName := strings.TrimSpace(req.PackageName)
	if packageName == "" {
		packageName = "openclaw"
	}

	m.mu.Lock()
	if m.status.State == stateRunning {
		m.mu.Unlock()
		return fmt.Errorf("install task is already running")
	}
	steps := defaultOpenClawSteps()
	m.status = vo.OpenClawInstallStatus{
		State:     stateRunning,
		Package:   packageName,
		Progress:  0,
		CurrentID: "",
		Current:   "",
		Message:   "install task started",
		Error:     "",
		StartedAt: nowRFC3339(),
		UpdatedAt: nowRFC3339(),
		Steps:     steps,
		Logs:      make([]vo.OpenClawInstallLog, 0, maxInstallLogs),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	m.cancel = cancel
	m.appendLogLocked("", "", "info", fmt.Sprintf("start install task, package=%s", packageName))
	if strings.TrimSpace(req.Registry) != "" {
		m.appendLogLocked("", "", "info", fmt.Sprintf("using registry=%s", strings.TrimSpace(req.Registry)))
	}
	m.mu.Unlock()

	go m.run(ctx, packageName, strings.TrimSpace(req.Registry))
	return nil
}

func (m *openClawInstallManager) statusSnapshot() vo.OpenClawInstallStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copied := m.status
	copied.Steps = append([]vo.OpenClawInstallStep(nil), m.status.Steps...)
	copied.Logs = append([]vo.OpenClawInstallLog(nil), m.status.Logs...)
	return copied
}

func (m *openClawInstallManager) run(ctx context.Context, packageName string, registry string) {
	defer func() {
		m.mu.Lock()
		m.cancel = nil
		m.status.UpdatedAt = nowRFC3339()
		if m.status.State == stateRunning {
			m.status.State = stateSuccess
			m.status.Progress = 100
			m.status.Message = "install completed"
			m.status.FinishedAt = nowRFC3339()
			m.appendLogLocked("", "", "info", "install completed")
		}
		m.mu.Unlock()
	}()

	installCmd := packageInstallCommand{}
	var nodeDetected bool
	var npmDetected bool

	if err := m.runStep(0, func() (string, bool, error) {
		cmd, err := nodeInstallCommand(runtime.GOOS, m.runner)
		if err != nil {
			return "", false, err
		}
		installCmd = cmd
		return fmt.Sprintf("os=%s, arch=%s, manager=%s", runtime.GOOS, runtime.GOARCH, cmd.Note), false, nil
	}); err != nil {
		return
	}

	if err := m.runStep(1, func() (string, bool, error) {
		nodeVersion, nodePath, nodeOK := detectCommandVersion(ctx, m.runner, "node", "--version")
		npmVersion, npmPath, npmOK := detectCommandVersion(ctx, m.runner, "npm", "--version")
		nodeDetected = nodeOK
		npmDetected = npmOK
		msg := fmt.Sprintf("node=%s (%s), npm=%s (%s)", emptyFallback(nodeVersion, "not found"), emptyFallback(nodePath, "n/a"), emptyFallback(npmVersion, "not found"), emptyFallback(npmPath, "n/a"))
		return msg, false, nil
	}); err != nil {
		return
	}

	if nodeDetected && npmDetected {
		if err := m.runStep(2, func() (string, bool, error) {
			return "node and npm already installed", true, nil
		}); err != nil {
			return
		}
	} else {
		if err := m.runStep(2, func() (string, bool, error) {
			output, err := m.runCommand(ctx, 2, installCmd.Name, installCmd.Args...)
			if err != nil {
				return trimOutput(output), false, fmt.Errorf("install node/npm failed: %w", err)
			}
			return trimOutput(output), false, nil
		}); err != nil {
			return
		}
	}

	if err := m.runStep(3, func() (string, bool, error) {
		npmVersion, _, ok := detectCommandVersion(ctx, m.runner, "npm", "--version")
		if !ok {
			return "", false, fmt.Errorf("npm is still unavailable after environment setup")
		}
		return fmt.Sprintf("npm version: %s", npmVersion), false, nil
	}); err != nil {
		return
	}

	if err := m.runStep(4, func() (string, bool, error) {
		args := []string{"install", "-g", packageName}
		if registry != "" {
			args = append(args, "--registry", registry)
		}
		output, err := m.runCommand(ctx, 4, "npm", args...)
		if err != nil {
			return trimOutput(output), false, fmt.Errorf("openclaw install failed: %w", err)
		}
		return trimOutput(output), false, nil
	}); err != nil {
		return
	}

	if err := m.runStep(5, func() (string, bool, error) {
		verifyOutput, err := m.runCommand(ctx, 5, "npm", "list", "-g", packageName, "--depth=0")
		if err != nil {
			return trimOutput(verifyOutput), false, fmt.Errorf("package verification failed: %w", err)
		}
		if !strings.Contains(strings.ToLower(verifyOutput), strings.ToLower(packageName)) {
			return trimOutput(verifyOutput), false, fmt.Errorf("package %s not found in global npm list", packageName)
		}

		versionOutput, versionErr := m.runCommand(ctx, 5, packageName, "--version")
		if versionErr == nil && strings.TrimSpace(versionOutput) != "" {
			return fmt.Sprintf("package verified, version: %s", strings.TrimSpace(versionOutput)), false, nil
		}
		return "package verified via npm list", false, nil
	}); err != nil {
		return
	}
}

func (m *openClawInstallManager) runStep(index int, fn func() (detail string, skip bool, err error)) error {
	m.setStepRunning(index)

	detail, skip, err := fn()
	if err != nil {
		m.setStepFailed(index, err, detail)
		return err
	}
	if skip {
		m.setStepSkipped(index, detail)
		return nil
	}
	m.setStepSuccess(index, detail)
	return nil
}

func (m *openClawInstallManager) appendLog(stepID string, stepTitle string, level string, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendLogLocked(stepID, stepTitle, level, message)
}

func (m *openClawInstallManager) appendLogLocked(stepID string, stepTitle string, level string, message string) {
	entry := vo.OpenClawInstallLog{
		Time:    nowRFC3339(),
		StepID:  stepID,
		Step:    stepTitle,
		Level:   level,
		Message: strings.TrimSpace(message),
	}
	if entry.Message == "" {
		return
	}
	m.status.Logs = append(m.status.Logs, entry)
	if len(m.status.Logs) > maxInstallLogs {
		m.status.Logs = m.status.Logs[len(m.status.Logs)-maxInstallLogs:]
	}
	m.status.UpdatedAt = nowRFC3339()
}

func (m *openClawInstallManager) runCommand(ctx context.Context, stepIndex int, name string, args ...string) (string, error) {
	step := m.statusSnapshot().Steps[stepIndex]
	m.appendLog(step.ID, step.Title, "info", "run command: "+commandString(name, args))
	output, err := m.runner.Run(ctx, name, args...)
	if strings.TrimSpace(output) != "" {
		m.appendLog(step.ID, step.Title, "info", "command output:\n"+trimOutput(output))
	}
	if err != nil {
		m.appendLog(step.ID, step.Title, "error", err.Error())
		return output, err
	}
	return output, nil
}

func (m *openClawInstallManager) setStepRunning(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	step := &m.status.Steps[index]
	step.Status = stepRunning
	step.StartedAt = nowRFC3339()
	step.EndedAt = ""
	step.Detail = ""
	m.status.CurrentID = step.ID
	m.status.Current = step.Title
	m.status.Message = step.Title
	m.status.UpdatedAt = nowRFC3339()
	m.status.Progress = calculateProgress(m.status.Steps)
	m.appendLogLocked(step.ID, step.Title, "info", "step started")
}

func (m *openClawInstallManager) setStepSuccess(index int, detail string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	step := &m.status.Steps[index]
	step.Status = stepSuccess
	step.EndedAt = nowRFC3339()
	step.Detail = detail
	m.status.UpdatedAt = nowRFC3339()
	m.status.Progress = calculateProgress(m.status.Steps)
	if detail != "" {
		m.appendLogLocked(step.ID, step.Title, "info", detail)
	}
	m.appendLogLocked(step.ID, step.Title, "info", "step finished")
}

func (m *openClawInstallManager) setStepSkipped(index int, detail string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	step := &m.status.Steps[index]
	step.Status = stepSkipped
	step.StartedAt = nowRFC3339()
	step.EndedAt = nowRFC3339()
	step.Detail = detail
	m.status.UpdatedAt = nowRFC3339()
	m.status.Progress = calculateProgress(m.status.Steps)
	m.appendLogLocked(step.ID, step.Title, "warn", "step skipped: "+detail)
}

func (m *openClawInstallManager) setStepFailed(index int, runErr error, detail string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	step := &m.status.Steps[index]
	step.Status = stepFailed
	step.EndedAt = nowRFC3339()
	step.Detail = detail
	m.status.State = stateFailed
	m.status.Error = runErr.Error()
	m.status.Message = step.Title + " failed"
	m.status.FinishedAt = nowRFC3339()
	m.status.UpdatedAt = nowRFC3339()
	m.status.Progress = calculateProgress(m.status.Steps)
	if detail != "" {
		m.appendLogLocked(step.ID, step.Title, "error", detail)
	}
	m.appendLogLocked(step.ID, step.Title, "error", runErr.Error())
}

func detectCommandVersion(ctx context.Context, runner commandRunner, name string, versionArg string) (string, string, bool) {
	path, err := runner.LookPath(name)
	if err != nil {
		return "", "", false
	}
	out, err := runner.Run(ctx, name, versionArg)
	if err != nil {
		return "", path, false
	}
	return strings.TrimSpace(out), path, true
}

func nodeInstallCommand(goos string, runner commandRunner) (packageInstallCommand, error) {
	switch goos {
	case "windows":
		if _, err := runner.LookPath("winget"); err == nil {
			return packageInstallCommand{
				Name: "winget",
				Args: []string{"install", "OpenJS.NodeJS.LTS", "-e", "--accept-source-agreements", "--accept-package-agreements", "--silent"},
				Note: "winget",
			}, nil
		}
		if _, err := runner.LookPath("choco"); err == nil {
			return packageInstallCommand{
				Name: "choco",
				Args: []string{"install", "nodejs-lts", "-y"},
				Note: "choco",
			}, nil
		}
		if _, err := runner.LookPath("scoop"); err == nil {
			return packageInstallCommand{
				Name: "scoop",
				Args: []string{"install", "nodejs-lts"},
				Note: "scoop",
			}, nil
		}
	case "darwin":
		if _, err := runner.LookPath("brew"); err == nil {
			return packageInstallCommand{
				Name: "brew",
				Args: []string{"install", "node"},
				Note: "brew",
			}, nil
		}
	case "linux":
		if _, err := runner.LookPath("apt-get"); err == nil {
			return packageInstallCommand{
				Name: "sudo",
				Args: []string{"apt-get", "install", "-y", "nodejs", "npm"},
				Note: "apt-get",
			}, nil
		}
		if _, err := runner.LookPath("yum"); err == nil {
			return packageInstallCommand{
				Name: "sudo",
				Args: []string{"yum", "install", "-y", "nodejs", "npm"},
				Note: "yum",
			}, nil
		}
		if _, err := runner.LookPath("dnf"); err == nil {
			return packageInstallCommand{
				Name: "sudo",
				Args: []string{"dnf", "install", "-y", "nodejs", "npm"},
				Note: "dnf",
			}, nil
		}
	}
	return packageInstallCommand{}, errors.New("no supported package manager found for node/npm installation")
}

func calculateProgress(steps []vo.OpenClawInstallStep) int {
	if len(steps) == 0 {
		return 0
	}
	completed := 0
	for _, step := range steps {
		if step.Status == stepSuccess || step.Status == stepSkipped {
			completed++
		}
	}
	return int(float64(completed) / float64(len(steps)) * 100)
}

func nowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}

func emptyFallback(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func trimOutput(output string) string {
	text := strings.TrimSpace(output)
	if len(text) > 900 {
		return text[:900] + "...(truncated)"
	}
	return text
}

func commandString(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, name)
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\"") {
			parts = append(parts, fmt.Sprintf("%q", arg))
			continue
		}
		parts = append(parts, arg)
	}
	return strings.Join(parts, " ")
}

var openClawInstaller = newOpenClawInstallManager(execRunner{})

func StartOpenClawInstall(c *gin.Context) {
	var req vo.OpenClawInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil && !strings.Contains(err.Error(), "EOF") {
		c.JSON(http.StatusBadRequest, utils.Fail(400, "invalid request payload"))
		return
	}
	if err := openClawInstaller.start(req); err != nil {
		c.JSON(http.StatusOK, utils.Fail(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message": "openclaw install task started",
		"package": emptyFallback(strings.TrimSpace(req.PackageName), "openclaw"),
	}))
}

func GetOpenClawInstallStatus(c *gin.Context) {
	c.JSON(http.StatusOK, utils.Success(openClawInstaller.statusSnapshot()))
}

func CheckOpenClawAuth(c *gin.Context) {
	result := vo.OpenClawAuthCheckResult{
		Installed:      false,
		NeedAuth:       false,
		Provider:       "",
		MissingAuth:    []string{},
		DefaultModel:   "",
		ConfigureCmd:   "openclaw configure",
		SetupTokenCmds: []string{"openclaw models auth setup-token"},
		ModelsOutput:   "",
		Error:          "",
		CheckedAt:      nowRFC3339(),
	}

	if _, err := openClawInstaller.runner.LookPath("openclaw"); err != nil {
		result.Error = "openclaw not found in PATH"
		c.JSON(http.StatusOK, utils.Success(result))
		return
	}
	result.Installed = true

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	output, err := openClawInstaller.runner.Run(ctx, "openclaw", "models")
	result.ModelsOutput = trimModelOutput(output)
	if err != nil {
		result.Error = err.Error()
	}

	defaultModel, provider, missing := parseOpenClawModelsOutput(output)
	result.DefaultModel = defaultModel
	result.Provider = provider
	result.MissingAuth = missing
	result.NeedAuth = len(missing) > 0

	if containsString(missing, "anthropic") {
		result.SetupTokenCmds = []string{
			"claude setup-token",
			"openclaw models auth setup-token",
		}
	}

	c.JSON(http.StatusOK, utils.Success(result))
}

func parseOpenClawModelsOutput(output string) (string, string, []string) {
	lines := strings.Split(output, "\n")
	defaultModel := ""
	provider := ""
	missing := make([]string, 0)
	inMissingSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if inMissingSection {
				inMissingSection = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "Default") {
			model := valueAfterColon(trimmed)
			if model != "-" && model != "" {
				defaultModel = model
				provider = providerFromModel(model)
			}
			continue
		}

		if strings.HasPrefix(trimmed, "Missing auth") {
			inMissingSection = true
			continue
		}

		if inMissingSection && strings.HasPrefix(trimmed, "- ") {
			entry := strings.TrimPrefix(trimmed, "- ")
			fields := strings.Fields(entry)
			if len(fields) > 0 {
				p := strings.ToLower(strings.TrimSpace(fields[0]))
				if p != "" && !containsString(missing, p) {
					missing = append(missing, p)
				}
			}
		}
	}

	if provider == "" && len(missing) > 0 {
		provider = missing[0]
	}
	return defaultModel, provider, missing
}

func valueAfterColon(line string) string {
	idx := strings.Index(line, ":")
	if idx == -1 || idx >= len(line)-1 {
		return ""
	}
	return strings.TrimSpace(line[idx+1:])
}

func providerFromModel(model string) string {
	idx := strings.Index(model, "/")
	if idx <= 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(model[:idx]))
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func trimModelOutput(output string) string {
	text := strings.TrimSpace(output)
	if len(text) > 8000 {
		return text[:8000] + "...(truncated)"
	}
	return text
}

type openClawListPayload struct {
	Count  int                    `json:"count"`
	Models []vo.OpenClawModelItem `json:"models"`
}

func parseModelItemsFromAny(payload any) []vo.OpenClawModelItem {
	switch typed := payload.(type) {
	case []any:
		models := make([]vo.OpenClawModelItem, 0, len(typed))
		for _, item := range typed {
			model, ok := parseSingleModelItem(item)
			if ok {
				models = append(models, model)
			}
		}
		return models
	case map[string]any:
		if modelsRaw, ok := typed["models"]; ok {
			return parseModelItemsFromAny(modelsRaw)
		}
	}
	return []vo.OpenClawModelItem{}
}

func parseSingleModelItem(payload any) (vo.OpenClawModelItem, bool) {
	obj, ok := payload.(map[string]any)
	if !ok {
		return vo.OpenClawModelItem{}, false
	}

	key := stringFromAny(obj["key"])
	if key == "" {
		key = stringFromAny(obj["id"])
	}
	if key == "" {
		return vo.OpenClawModelItem{}, false
	}

	name := stringFromAny(obj["name"])
	if name == "" {
		name = key
	}

	local := boolFromAny(obj["local"])
	available := boolFromAny(obj["available"])
	tags := stringArrayFromAny(obj["tags"])

	if provider := strings.TrimSpace(stringFromAny(obj["provider"])); provider != "" && !strings.Contains(key, "/") {
		key = provider + "/" + key
	}

	return vo.OpenClawModelItem{
		Key:       key,
		Name:      name,
		Available: available,
		Local:     local,
		Tags:      tags,
	}, true
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		text := strings.TrimSpace(strings.ToLower(typed))
		return text == "true" || text == "1" || text == "yes"
	case float64:
		return typed != 0
	default:
		return false
	}
}

func stringArrayFromAny(value any) []string {
	typed, ok := value.([]any)
	if !ok {
		return []string{}
	}
	result := make([]string, 0, len(typed))
	for _, item := range typed {
		if text := strings.TrimSpace(stringFromAny(item)); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func parseModelItems(raw string) ([]vo.OpenClawModelItem, error) {
	jsonPayload := extractJSONPayload(raw)
	if jsonPayload == "" {
		return nil, fmt.Errorf("model list output is not json")
	}

	var arrayPayload []vo.OpenClawModelItem
	if err := json.Unmarshal([]byte(jsonPayload), &arrayPayload); err == nil {
		return arrayPayload, nil
	}

	var typedPayload openClawListPayload
	if err := json.Unmarshal([]byte(jsonPayload), &typedPayload); err == nil && len(typedPayload.Models) > 0 {
		return typedPayload.Models, nil
	}

	var dynamicPayload any
	if err := json.Unmarshal([]byte(jsonPayload), &dynamicPayload); err != nil {
		return nil, fmt.Errorf("unmarshal model list json failed: %w", err)
	}
	models := parseModelItemsFromAny(dynamicPayload)
	if len(models) == 0 {
		return nil, fmt.Errorf("no models found in model list")
	}
	return models, nil
}

func extractJSONPayload(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed
	}

	firstObj := strings.Index(trimmed, "{")
	lastObj := strings.LastIndex(trimmed, "}")
	if firstObj >= 0 && lastObj > firstObj {
		candidate := strings.TrimSpace(trimmed[firstObj : lastObj+1])
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}

	firstArr := strings.Index(trimmed, "[")
	lastArr := strings.LastIndex(trimmed, "]")
	if firstArr >= 0 && lastArr > firstArr {
		candidate := strings.TrimSpace(trimmed[firstArr : lastArr+1])
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}

	return ""
}

func runOpenClawCommand(ctx context.Context, args []string, envVars map[string]string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		baseArgs := make([]string, 0, len(args)+2)
		baseArgs = append(baseArgs, "/c", "openclaw")
		baseArgs = append(baseArgs, args...)
		cmd = exec.CommandContext(ctx, "cmd", baseArgs...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	} else {
		cmd = exec.CommandContext(ctx, "openclaw", args...)
	}

	env := os.Environ()
	for key, value := range envVars {
		env = append(env, key+"="+value)
	}
	cmd.Env = env

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	output := strings.TrimSpace(out.String())
	if err != nil {
		if output == "" {
			return "", err
		}
		return output, fmt.Errorf("%w: %s", err, output)
	}
	return output, nil
}

func providerToEnvVar(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	default:
		return ""
	}
}

func isGuestModel(model vo.OpenClawModelItem) bool {
	key := strings.ToLower(model.Key)
	name := strings.ToLower(model.Name)
	if strings.Contains(key, ":free") || strings.Contains(name, "free") {
		return true
	}
	for _, tag := range model.Tags {
		if strings.EqualFold(strings.TrimSpace(tag), "free") {
			return true
		}
	}
	return false
}

func pickGuestModel(models []vo.OpenClawModelItem) string {
	for _, model := range models {
		if isGuestModel(model) {
			return model.Key
		}
	}
	return ""
}

func persistEnvVarWindows(key string, value string) error {
	cmd := exec.Command("setx", key, value)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(out.String())
		if text == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, text)
	}
	return nil
}

func ConfigureOpenClawAndQueryModels(c *gin.Context) {
	var req vo.OpenClawQuickConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(400, "invalid request payload"))
		return
	}

	result := vo.OpenClawQuickConfigResult{
		Success:         false,
		Provider:        strings.ToLower(strings.TrimSpace(req.Provider)),
		DefaultModel:    strings.TrimSpace(req.DefaultModel),
		AvailableModels: make([]vo.OpenClawModelItem, 0),
		GuestModels:     make([]vo.OpenClawModelItem, 0),
		Steps:           make([]string, 0, 8),
		CheckedAt:       nowRFC3339(),
	}

	if result.Provider == "" {
		result.Provider = "anthropic"
	}

	if _, err := openClawInstaller.runner.LookPath("openclaw"); err != nil {
		result.Error = "openclaw not found in PATH"
		result.Message = "please install openclaw first"
		c.JSON(http.StatusOK, utils.Success(result))
		return
	}
	result.Steps = append(result.Steps, "checked openclaw command")

	envVarName := providerToEnvVar(result.Provider)
	if envVarName == "" {
		result.Error = "unsupported provider, use anthropic/openai/openrouter"
		c.JSON(http.StatusOK, utils.Success(result))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	envOverrides := map[string]string{}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey != "" {
		envOverrides[envVarName] = apiKey
		result.Steps = append(result.Steps, fmt.Sprintf("loaded api key from request (%s)", envVarName))
		if req.PersistEnv && runtime.GOOS == "windows" {
			if err := persistEnvVarWindows(envVarName, apiKey); err == nil {
				result.Steps = append(result.Steps, fmt.Sprintf("persisted env var by setx: %s", envVarName))
			} else {
				result.Steps = append(result.Steps, fmt.Sprintf("persist env failed: %v", err))
			}
		} else if req.PersistEnv {
			result.Steps = append(result.Steps, "persist env is only supported on windows currently")
		}
	} else {
		result.Steps = append(result.Steps, fmt.Sprintf("api key not provided, using current environment: %s", envVarName))
	}

	listRaw, listErr := runOpenClawCommand(ctx, []string{"models", "list", "--json", "--all"}, envOverrides)
	result.RawListJSON = trimModelOutput(listRaw)
	if listErr != nil {
		result.Error = listErr.Error()
		result.Message = "failed to query model list"
		c.JSON(http.StatusOK, utils.Success(result))
		return
	}
	result.Steps = append(result.Steps, "queried model list")

	modelItems, err := parseModelItems(listRaw)
	if err != nil {
		result.Error = fmt.Sprintf("failed to parse model list: %v", err)
		result.Message = "failed to parse model list"
		c.JSON(http.StatusOK, utils.Success(result))
		return
	}

	available := make([]vo.OpenClawModelItem, 0, len(modelItems))
	guest := make([]vo.OpenClawModelItem, 0, len(modelItems))
	for _, model := range modelItems {
		if model.Available {
			available = append(available, model)
			if isGuestModel(model) {
				guest = append(guest, model)
			}
		}
	}
	result.AvailableCount = len(available)
	result.GuestModelCount = len(guest)
	result.GuestModelReady = len(guest) > 0
	if len(available) > 100 {
		result.AvailableModels = available[:100]
	} else {
		result.AvailableModels = available
	}
	if len(guest) > 100 {
		result.GuestModels = guest[:100]
	} else {
		result.GuestModels = guest
	}

	defaultModel := result.DefaultModel
	if req.UseGuestMode && defaultModel == "" {
		defaultModel = pickGuestModel(guest)
		if defaultModel != "" {
			result.Steps = append(result.Steps, fmt.Sprintf("picked guest model: %s", defaultModel))
		}
	}

	if defaultModel != "" {
		if _, err := runOpenClawCommand(ctx, []string{"models", "set", defaultModel}, envOverrides); err != nil {
			result.Error = fmt.Sprintf("set default model failed: %v", err)
			result.Message = "model set failed"
			c.JSON(http.StatusOK, utils.Success(result))
			return
		}
		result.DefaultModel = defaultModel
		result.Steps = append(result.Steps, "set default model: "+defaultModel)
	}

	statusRaw, statusErr := runOpenClawCommand(ctx, []string{"models", "status", "--json"}, envOverrides)
	result.RawStatusJSON = trimModelOutput(statusRaw)
	if statusErr != nil {
		result.Steps = append(result.Steps, fmt.Sprintf("query models status skipped: %v", statusErr))
	} else {
		result.Steps = append(result.Steps, "queried models status")
	}

	result.Success = true
	result.Message = "configured and queried models successfully"
	c.JSON(http.StatusOK, utils.Success(result))
}
