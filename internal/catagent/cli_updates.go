package catagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"FFmpegFree/internal/proc"
)

// CLIUpdate is a live check, never a persisted installation status.
type CLIUpdate struct {
	AgentKind      string `json:"agentKind"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Available      bool   `json:"available"`
	CanUpdate      bool   `json:"canUpdate"`
	UpdateURL      string `json:"updateUrl,omitempty"`
}

type CLIUpdater interface {
	CheckCLIUpdate(context.Context) (CLIUpdate, error)
	UpdateCLI(context.Context) error
}

var cliUpdateHTTP = &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{Proxy: func(req *http.Request) (*url.URL, error) {
	if p, err := http.ProxyFromEnvironment(req); p != nil || err != nil {
		return p, err
	}
	if p := cliSystemProxy(); p != "" {
		return url.Parse(p)
	}
	return nil, nil
}}}

func newerCLI(current, latest string) bool {
	a, ok := parseCLIVersion(current)
	b, valid := parseCLIVersion(latest)
	if !ok || !valid {
		return false
	}
	if a != b {
		return cliVersionBelow(current, latest)
	}
	// A stable release also supersedes its own prerelease.
	match := cliVersionRe.FindStringIndex(current)
	return match != nil && strings.HasPrefix(current[match[1]:], "-") && !strings.Contains(latest, "-")
}

func npmLatest(ctx context.Context, pkg string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://registry.npmjs.org/"+pkg+"/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := cliUpdateHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("version lookup failed")
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", err
	}
	if _, ok := parseCLIVersion(body.Version); !ok {
		return "", errors.New("invalid release version")
	}
	return body.Version, nil
}

// Only update the npm prefix that owns the detected executable. In particular,
// a desktop app's bundled Codex must not be replaced by an unrelated global npm install.
func npmOwnerPrefix(exe, pkg string) string {
	if isShimExt(exe) {
		prefix := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(prefix, "node_modules", filepath.FromSlash(pkg), "package.json")); err == nil {
			return prefix
		}
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return ""
	}
	p := filepath.ToSlash(resolved)
	marker := "/node_modules/" + pkg + "/"
	if i := strings.Index(p, marker); i >= 0 {
		prefix := strings.TrimSuffix(p[:i], "/lib")
		if _, err := os.Stat(filepath.Join(filepath.FromSlash(p[:i]), "node_modules", filepath.FromSlash(pkg), "package.json")); err == nil {
			return filepath.FromSlash(prefix)
		}
	}
	return ""
}

func (a *CLIAgentAdapter) updatePackage() string {
	if a.kind == cliKindCodex {
		return "@openai/codex"
	}
	return "@anthropic-ai/claude-code"
}

func (a *CLIAgentAdapter) CheckCLIUpdate(ctx context.Context) (CLIUpdate, error) {
	st, exe := a.Status(), a.ExecutablePath()
	r := CLIUpdate{AgentKind: a.name, CurrentVersion: st.Version}
	if st.State != StateReady || exe == "" {
		return r, nil
	}
	latest, err := npmLatest(ctx, a.updatePackage())
	if err != nil {
		return r, err
	}
	r.LatestVersion, r.Available = latest, newerCLI(st.Version, latest)
	r.CanUpdate = a.kind == cliKindClaude || npmOwnerPrefix(exe, a.updatePackage()) != ""
	if !r.CanUpdate {
		r.UpdateURL = "https://developers.openai.com/codex/cli/"
	}
	return r, nil
}

func (a *CLIAgentAdapter) UpdateCLI(ctx context.Context) (err error) {
	exe := a.ExecutablePath()
	defer func() {
		if err == nil {
			invalidateCLIVersion(exe)
		}
	}()
	if exe == "" {
		return errors.New("CLI missing")
	}
	if prefix := npmOwnerPrefix(exe, a.updatePackage()); prefix != "" {
		npm := "npm"
		if runtime.GOOS == "windows" {
			npm = "npm.cmd"
		}
		p, err := a.lookPath(npm)
		if err != nil && winFallbackEnabled() {
			p = winFallback("", "npm")
		}
		if p == "" {
			return errors.New("npm missing")
		}
		return runCLIUpdate(a.runCmd(ctx, p, "install", "--global", "--prefix", prefix, a.updatePackage()+"@latest", "--no-audit", "--no-fund"))
	}
	if a.kind == cliKindClaude {
		return runCLIUpdate(a.runCmd(ctx, exe, "update"))
	}
	return errors.New("this installation requires its original installer")
}

func runCLIUpdate(cmd *exec.Cmd) error {
	configureCLIUpdateEnv(cmd)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	return proc.Run(cmd)
}

func invalidateCLIVersion(exe string) {
	versionCacheMu.Lock()
	defer versionCacheMu.Unlock()
	for k := range versionCache {
		if k.path == exe {
			delete(versionCache, k)
		}
	}
}

func configureCLIUpdateEnv(cmd *exec.Cmd) {
	if os.Getenv("HTTPS_PROXY") != "" || os.Getenv("https_proxy") != "" || os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
		return
	}
	if proxy := cliSystemProxy(); proxy != "" {
		if cmd.Env == nil {
			cmd.Env = os.Environ()
		}
		cmd.Env = append(cmd.Env, "HTTPS_PROXY="+proxy, "HTTP_PROXY="+proxy)
	}
}

func (a *BuildAdapter) CheckCLIUpdate(ctx context.Context) (CLIUpdate, error) {
	r := CLIUpdate{AgentKind: KindCatBuild, CurrentVersion: a.Status().Version, CanUpdate: true}
	if a.Status().State != StateReady || a.ExecutablePath() == "" {
		return r, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	cmd := a.runCmd(probeCtx, a.ExecutablePath(), "update", "--check", "--json")
	configureCLIUpdateEnv(cmd)
	out, err := cmd.Output()
	if err != nil {
		// Older Grok releases may not understand --check/--json. The official
		// stable channel pointer is a plain version string and needs no CLI flags.
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, "https://x.ai/cli/stable", nil)
		if reqErr != nil {
			return r, reqErr
		}
		resp, fetchErr := cliUpdateHTTP.Do(req)
		if fetchErr != nil {
			return r, fetchErr
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return r, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return r, readErr
		}
		r.LatestVersion = strings.TrimSpace(string(body))
		if _, ok := parseCLIVersion(r.LatestVersion); !ok {
			return r, err
		}
		r.Available = newerCLI(r.CurrentVersion, r.LatestVersion)
		return r, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(out, &body); err != nil {
		return r, err
	}
	for _, key := range []string{"latest_version", "latestVersion", "latest", "available_version"} {
		if json.Unmarshal(body[key], &r.LatestVersion) == nil && r.LatestVersion != "" {
			break
		}
	}
	if _, ok := parseCLIVersion(r.LatestVersion); !ok {
		return r, errors.New("invalid release version")
	}
	r.Available = newerCLI(r.CurrentVersion, r.LatestVersion)
	// The native checker knows the user's release channel, including alpha versions.
	for _, key := range []string{"updateAvailable", "update_available"} {
		if raw, ok := body[key]; ok && json.Unmarshal(raw, &r.Available) == nil {
			break
		}
	}
	return r, nil
}

func (a *BuildAdapter) UpdateCLI(ctx context.Context) (err error) {
	exe := a.ExecutablePath()
	defer func() {
		if err == nil {
			invalidateCLIVersion(exe)
		}
	}()
	if a.ExecutablePath() == "" {
		return errors.New("CLI missing")
	}
	return runCLIUpdate(a.runCmd(ctx, a.ExecutablePath(), "update"))
}
