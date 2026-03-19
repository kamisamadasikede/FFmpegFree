package contollers

import (
	"FFmpegFree/backend/live"
	"FFmpegFree/backend/sse"
	"FFmpegFree/backend/utils"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
)

type relayTask struct {
	streamID      string
	displayName   string
	sourceURL     string
	targets       []string
	cmd           *exec.Cmd
	stoppedByUser atomic.Bool
}

type relayStartRequest struct {
	DisplayName    string   `json:"displayName"`
	SourceURL      string   `json:"sourceUrl"`
	Targets        []string `json:"targets"`
	ArchiveEnabled bool     `json:"archiveEnabled"`
	SegmentSeconds int      `json:"segmentSeconds"`
}

type relayStopRequest struct {
	StreamID string `json:"streamId"`
}

type liveFileStreamTask struct {
	streamID       string
	name           string
	inputPath      string
	streamURL      string
	relayTargets   []string
	archiveEnabled bool
	segmentSeconds int
	startedAt      time.Time
	cmd            *exec.Cmd
	stoppedByUser  atomic.Bool
}

type liveFileStreamRequest struct {
	Name           string   `json:"name"`
	SteamUrl       string   `json:"steamurl"`
	StreamID       string   `json:"streamId"`
	RelayTargets   []string `json:"relayTargets"`
	ArchiveEnabled bool     `json:"archiveEnabled"`
	SegmentSeconds int      `json:"segmentSeconds"`
}

var relayTasksMutex sync.Mutex
var relayTasks = make(map[string]*relayTask)
var liveFileTasksMutex sync.Mutex
var liveFileTasks = make(map[string]*liveFileStreamTask)

func sanitizeRelayTargets(targets []string) []string {
	result := make([]string, 0, len(targets))
	seen := make(map[string]struct{})
	for _, target := range targets {
		trimmed := strings.TrimSpace(target)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func findLiveFileTaskByComposite(name string, streamURL string) (string, *liveFileStreamTask) {
	for streamID, task := range liveFileTasks {
		if task == nil {
			continue
		}
		if task.name == name && task.streamURL == streamURL {
			return streamID, task
		}
	}
	return "", nil
}

func formatOutDuration(outTimeMs int64) string {
	if outTimeMs <= 0 {
		return "00:00:00"
	}
	totalSeconds := outTimeMs / 1000000
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// StartLiveFileStream 启动增强版“文件推流”流程：
// - 单文件输入
// - 1 个主推流目标
// - 可选多目标转推
// - 可选本地分段归档
//
// 这里单独保留新接口，不直接改旧 /api/steamload，
// 目的是在不破坏历史行为的前提下逐步上线 P0/P1 能力。
func StartLiveFileStream(c *gin.Context) {
	var request liveFileStreamRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "invalid request payload"))
		return
	}

	request.Name = strings.TrimSpace(request.Name)
	request.SteamUrl = strings.TrimSpace(request.SteamUrl)
	request.RelayTargets = sanitizeRelayTargets(request.RelayTargets)

	if request.Name == "" || request.SteamUrl == "" {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "name and steamurl are required"))
		return
	}

	inputPath := filepath.Join("public", "steam", request.Name)
	if _, err := os.Stat(inputPath); err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, fmt.Sprintf("input file not found: %v", err)))
		return
	}
	if !isVideoFile(request.Name) {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "unsupported video file type"))
		return
	}

	snapshot, err := live.Global.Start(live.StartOptions{
		DisplayName:    request.Name,
		Input:          inputPath,
		PrimaryTarget:  request.SteamUrl,
		RelayTargets:   request.RelayTargets,
		ArchiveEnabled: request.ArchiveEnabled,
		SegmentSeconds: request.SegmentSeconds,
		Source:         live.StreamSourceFile,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, fmt.Sprintf("start stream validation failed: %v", err)))
		return
	}

	if err := live.EnsureArchiveDir(snapshot.ArchiveDir); err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		c.JSON(http.StatusInternalServerError, utils.Fail(500, fmt.Sprintf("prepare archive directory failed: %v", err)))
		return
	}

	teeOutput, err := live.BuildTeeOutput(snapshot.Targets, snapshot.ArchiveDir, snapshot.SegmentSeconds)
	if err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		c.JSON(http.StatusBadRequest, utils.Fail(500, fmt.Sprintf("build stream outputs failed: %v", err)))
		return
	}

	args := []string{
		"-re", "-i", inputPath,
		"-map", "0:v:0",
		"-map", "0:a?",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-g", "30",
		"-keyint_min", "30",
		"-sc_threshold", "0",
		"-c:a", "aac",
		"-b:a", "128k",
		"-progress", "pipe:2",
		"-nostats",
		"-f", "tee",
		teeOutput,
	}
	cmd := exec.Command(live.FFmpegBinaryPath(), args...)
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		c.JSON(http.StatusInternalServerError, utils.Fail(500, fmt.Sprintf("create ffmpeg progress pipe failed: %v", err)))
		return
	}

	if err := cmd.Start(); err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		c.JSON(http.StatusInternalServerError, utils.Fail(500, fmt.Sprintf("start stream failed: %v", err)))
		return
	}
	live.Global.MarkRunning(snapshot.StreamID)

	task := &liveFileStreamTask{
		streamID:       snapshot.StreamID,
		name:           request.Name,
		inputPath:      inputPath,
		streamURL:      request.SteamUrl,
		relayTargets:   append([]string(nil), snapshot.Targets[1:]...),
		archiveEnabled: snapshot.ArchiveEnabled,
		segmentSeconds: snapshot.SegmentSeconds,
		startedAt:      time.Now(),
		cmd:            cmd,
	}

	liveFileTasksMutex.Lock()
	liveFileTasks[snapshot.StreamID] = task
	liveFileTasksMutex.Unlock()

	go func(streamID string, reader io.Reader) {
		if progressErr := live.ConsumeProgress(reader, func(key, value string) {
			live.Global.UpdateProgress(streamID, key, value)
		}); progressErr != nil && progressErr != io.EOF {
			live.Global.TouchFailure(streamID, progressErr)
		}
	}(snapshot.StreamID, stderr)

	go func(current *liveFileStreamTask) {
		waitErr := current.cmd.Wait()
		stoppedByUser := current.stoppedByUser.Load()
		live.Global.MarkFinished(current.streamID, stoppedByUser, waitErr)

		status := "completed"
		errorMsg := fmt.Sprintf("stream completed: %s", current.name)
		if stoppedByUser {
			status = "stopped"
			errorMsg = fmt.Sprintf("stream stopped by user: %s", current.name)
		} else if waitErr != nil {
			status = "failed"
			errorMsg = fmt.Sprintf("stream exited with error: %s, err: %v", current.name, waitErr)
		}

		eventData := map[string]interface{}{
			"streamId":  current.streamID,
			"filename":  current.name,
			"streamUrl": current.streamURL,
			"status":    status,
			"error":     errorMsg,
		}
		jsonData, _ := json.Marshal(eventData)
		sse.BroadcastMessage(string(jsonData))

		liveFileTasksMutex.Lock()
		delete(liveFileTasks, current.streamID)
		liveFileTasksMutex.Unlock()
	}(task)

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message":         "stream task started",
		"file":            request.Name,
		"stream":          request.SteamUrl,
		"streamId":        snapshot.StreamID,
		"archiveEnabled":  snapshot.ArchiveEnabled,
		"segmentSeconds":  snapshot.SegmentSeconds,
		"relayTargetSize": len(snapshot.Targets) - 1,
	}))
}

// StopLiveFileStream 支持按 streamId 停止（推荐），并兼容 name+steamurl 兜底匹配。
func StopLiveFileStream(c *gin.Context) {
	var request liveFileStreamRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "invalid request payload"))
		return
	}

	request.StreamID = strings.TrimSpace(request.StreamID)
	request.Name = strings.TrimSpace(request.Name)
	request.SteamUrl = strings.TrimSpace(request.SteamUrl)

	liveFileTasksMutex.Lock()
	streamID := request.StreamID
	task := liveFileTasks[streamID]
	if task == nil && request.Name != "" && request.SteamUrl != "" {
		streamID, task = findLiveFileTaskByComposite(request.Name, request.SteamUrl)
	}
	liveFileTasksMutex.Unlock()

	if task == nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "stream task not found"))
		return
	}

	task.stoppedByUser.Store(true)
	if task.cmd.Process != nil {
		if err := task.cmd.Process.Kill(); err != nil {
			c.JSON(http.StatusInternalServerError, utils.Fail(500, "stop stream failed"))
			return
		}
	}

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"streamId": streamID,
		"message":  "stream stopped",
	}))
}

// ListLiveFileStreams 以兼容 steamlist 的字段结构返回当前增强版推流任务。
func ListLiveFileStreams(c *gin.Context) {
	liveFileTasksMutex.Lock()
	items := make([]gin.H, 0, len(liveFileTasks))
	for _, task := range liveFileTasks {
		if task == nil {
			continue
		}
		snapshot, _ := live.Global.Snapshot(task.streamID)
		items = append(items, gin.H{
			"name":           task.name,
			"url":            "",
			"duration":       formatOutDuration(snapshot.OutTimeMs),
			"date":           task.startedAt.Format("2006-01-02 15:04:05"),
			"steamurl":       task.streamURL,
			"streamId":       task.streamID,
			"targetFormat":   "",
			"archiveEnabled": task.archiveEnabled,
			"segmentSeconds": task.segmentSeconds,
			"relayTargets":   append([]string(nil), task.relayTargets...),
		})
	}
	liveFileTasksMutex.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"count":   len(items),
		"streams": items,
	})
}

// StartRelay 启动“一键转推”任务（单源多目标）。
// 与文件推流/屏幕推流共用同一套监控与归档能力。
func StartRelay(c *gin.Context) {
	var request relayStartRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "invalid request payload"))
		return
	}

	request.SourceURL = strings.TrimSpace(request.SourceURL)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.Targets = sanitizeRelayTargets(request.Targets)

	if request.SourceURL == "" {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "sourceUrl is required"))
		return
	}
	if len(request.Targets) == 0 {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "at least one target is required"))
		return
	}
	if request.DisplayName == "" {
		request.DisplayName = "relay task"
	}

	snapshot, err := live.Global.Start(live.StartOptions{
		DisplayName:    request.DisplayName,
		Input:          request.SourceURL,
		PrimaryTarget:  request.Targets[0],
		RelayTargets:   request.Targets[1:],
		ArchiveEnabled: request.ArchiveEnabled,
		SegmentSeconds: request.SegmentSeconds,
		Source:         live.StreamSourceRelay,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, fmt.Sprintf("start relay validation failed: %v", err)))
		return
	}

	if err := live.EnsureArchiveDir(snapshot.ArchiveDir); err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		c.JSON(http.StatusInternalServerError, utils.Fail(500, fmt.Sprintf("prepare archive directory failed: %v", err)))
		return
	}

	teeOutput, err := live.BuildTeeOutput(snapshot.Targets, snapshot.ArchiveDir, snapshot.SegmentSeconds)
	if err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		c.JSON(http.StatusBadRequest, utils.Fail(500, fmt.Sprintf("build relay outputs failed: %v", err)))
		return
	}

	args := []string{
		"-i", request.SourceURL,
		"-map", "0:v:0",
		"-map", "0:a?",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-g", "30",
		"-keyint_min", "30",
		"-sc_threshold", "0",
		"-c:a", "aac",
		"-b:a", "128k",
		"-progress", "pipe:2",
		"-nostats",
		"-f", "tee",
		teeOutput,
	}
	cmd := exec.Command(live.FFmpegBinaryPath(), args...)
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		c.JSON(http.StatusInternalServerError, utils.Fail(500, fmt.Sprintf("create ffmpeg progress pipe failed: %v", err)))
		return
	}

	if err := cmd.Start(); err != nil {
		live.Global.TouchFailure(snapshot.StreamID, err)
		c.JSON(http.StatusInternalServerError, utils.Fail(500, fmt.Sprintf("start relay failed: %v", err)))
		return
	}
	live.Global.MarkRunning(snapshot.StreamID)

	task := &relayTask{
		streamID:    snapshot.StreamID,
		displayName: request.DisplayName,
		sourceURL:   request.SourceURL,
		targets:     append([]string(nil), snapshot.Targets...),
		cmd:         cmd,
	}

	relayTasksMutex.Lock()
	relayTasks[snapshot.StreamID] = task
	relayTasksMutex.Unlock()

	go func(streamID string, reader io.Reader) {
		if progressErr := live.ConsumeProgress(reader, func(key, value string) {
			live.Global.UpdateProgress(streamID, key, value)
		}); progressErr != nil && progressErr != io.EOF {
			live.Global.TouchFailure(streamID, progressErr)
		}
	}(snapshot.StreamID, stderr)

	go func(current *relayTask) {
		waitErr := current.cmd.Wait()
		stoppedByUser := current.stoppedByUser.Load()
		live.Global.MarkFinished(current.streamID, stoppedByUser, waitErr)

		status := "completed"
		errorMsg := "relay completed"
		if stoppedByUser {
			status = "stopped"
			errorMsg = "relay stopped by user"
		} else if waitErr != nil {
			status = "failed"
			errorMsg = fmt.Sprintf("relay exited with error: %v", waitErr)
		}

		eventData := map[string]interface{}{
			"streamId":  current.streamID,
			"filename":  current.displayName,
			"streamUrl": current.sourceURL,
			"status":    status,
			"error":     errorMsg,
		}
		jsonData, _ := json.Marshal(eventData)
		sse.BroadcastMessage(string(jsonData))

		relayTasksMutex.Lock()
		delete(relayTasks, current.streamID)
		relayTasksMutex.Unlock()
	}(task)

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"streamId":       snapshot.StreamID,
		"displayName":    request.DisplayName,
		"targets":        snapshot.Targets,
		"archiveEnabled": snapshot.ArchiveEnabled,
		"segmentSeconds": snapshot.SegmentSeconds,
	}))
}

// StopRelay 按 streamId 停止转推任务。
func StopRelay(c *gin.Context) {
	var request relayStopRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "invalid request payload"))
		return
	}

	request.StreamID = strings.TrimSpace(request.StreamID)
	if request.StreamID == "" {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "streamId is required"))
		return
	}

	relayTasksMutex.Lock()
	task, exists := relayTasks[request.StreamID]
	relayTasksMutex.Unlock()
	if !exists || task == nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "relay task not found"))
		return
	}

	task.stoppedByUser.Store(true)
	if task.cmd.Process != nil {
		if err := task.cmd.Process.Kill(); err != nil {
			c.JSON(http.StatusInternalServerError, utils.Fail(500, "stop relay failed"))
			return
		}
	}

	c.JSON(http.StatusOK, utils.Success(gin.H{"streamId": request.StreamID, "message": "relay stopped"}))
}

// ListRelay 返回当前正在运行的转推任务，供运维面板展示。
func ListRelay(c *gin.Context) {
	relayTasksMutex.Lock()
	items := make([]gin.H, 0, len(relayTasks))
	for _, task := range relayTasks {
		snapshot, _ := live.Global.Snapshot(task.streamID)
		items = append(items, gin.H{
			"streamId":    task.streamID,
			"displayName": task.displayName,
			"sourceUrl":   task.sourceURL,
			"targets":     append([]string(nil), task.targets...),
			"status":      snapshot.Status,
			"health":      snapshot.Health,
			"latencyMs":   snapshot.EstimatedLatencyMs,
			"dropFrames":  snapshot.DropFrames,
		})
	}
	relayTasksMutex.Unlock()

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"count": len(items),
		"items": items,
	}))
}

// GetLiveHealth 是统一的直播诊断接口（健康等级、延迟、掉帧、码率等）。
func GetLiveHealth(c *gin.Context) {
	snapshots := live.Global.ListSnapshots()

	summary := gin.H{
		"total":    len(snapshots),
		"active":   0,
		"warning":  0,
		"critical": 0,
	}
	for _, item := range snapshots {
		if item.Status == "running" || item.Status == "starting" {
			summary["active"] = summary["active"].(int) + 1
		}
		if item.Health == live.HealthWarning {
			summary["warning"] = summary["warning"].(int) + 1
		}
		if item.Health == live.HealthCritical {
			summary["critical"] = summary["critical"].(int) + 1
		}
	}

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"summary": summary,
		"items":   snapshots,
	}))
}

// GetLiveArchives 枚举 public/archive 下所有分段归档文件。
func GetLiveArchives(c *gin.Context) {
	archives, err := live.Global.ListArchives()
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.Fail(500, fmt.Sprintf("list archives failed: %v", err)))
		return
	}

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"count": len(archives),
		"items": archives,
	}))
}

// KillLiveOpsProcesses 在应用退出时调用，防止遗留 ffmpeg 子进程。
func KillLiveOpsProcesses() {
	liveFileTasksMutex.Lock()
	for _, task := range liveFileTasks {
		if task != nil && task.cmd != nil && task.cmd.Process != nil {
			_ = task.cmd.Process.Kill()
		}
	}
	liveFileTasksMutex.Unlock()

	relayTasksMutex.Lock()
	for _, task := range relayTasks {
		if task != nil && task.cmd != nil && task.cmd.Process != nil {
			_ = task.cmd.Process.Kill()
		}
	}
	relayTasksMutex.Unlock()
}
