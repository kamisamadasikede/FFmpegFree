package live

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type HealthLevel string

const (
	HealthHealthy  HealthLevel = "healthy"
	HealthWarning  HealthLevel = "warning"
	HealthCritical HealthLevel = "critical"
)

type StreamSource string

const (
	StreamSourceFile   StreamSource = "file"
	StreamSourceScreen StreamSource = "screen"
	StreamSourceRelay  StreamSource = "relay"
)

type StartOptions struct {
	DisplayName    string
	Input          string
	PrimaryTarget  string
	RelayTargets   []string
	ArchiveEnabled bool
	SegmentSeconds int
	Source         StreamSource
}

// StreamSnapshot 是直播诊断面板的统一状态模型。
// 指标来源有两类：
// 1) ffmpeg -progress 输出（fps/speed/bitrate/out_time/drop_frames）
// 2) websocket 输入字节统计（用于估算屏幕采集入口码率）
type StreamSnapshot struct {
	StreamID           string       `json:"streamId"`
	DisplayName        string       `json:"displayName"`
	Input              string       `json:"input"`
	Targets            []string     `json:"targets"`
	Source             StreamSource `json:"source"`
	Status             string       `json:"status"`
	ArchiveEnabled     bool         `json:"archiveEnabled"`
	SegmentSeconds     int          `json:"segmentSeconds"`
	ArchiveDir         string       `json:"archiveDir"`
	FPS                float64      `json:"fps"`
	BitrateKbps        float64      `json:"bitrateKbps"`
	IngressBitrateKbps float64      `json:"ingressBitrateKbps"`
	Speed              float64      `json:"speed"`
	OutTimeMs          int64        `json:"outTimeMs"`
	EstimatedLatencyMs int64        `json:"estimatedLatencyMs"`
	DropFrames         int64        `json:"dropFrames"`
	DupFrames          int64        `json:"dupFrames"`
	Health             HealthLevel  `json:"health"`
	Diagnosis          string       `json:"diagnosis"`
	LastError          string       `json:"lastError,omitempty"`
	StartedAt          time.Time    `json:"startedAt"`
	UpdatedAt          time.Time    `json:"updatedAt"`
}

type ArchiveItem struct {
	StreamID  string `json:"streamId"`
	FileName  string `json:"fileName"`
	FileURL   string `json:"fileUrl"`
	SizeBytes int64  `json:"sizeBytes"`
	UpdatedAt string `json:"updatedAt"`
}

type streamState struct {
	snapshot            StreamSnapshot
	lastIngressAt       time.Time
	ingressBytesInSlice int64
}

type Manager struct {
	mu             sync.RWMutex
	baseArchiveDir string
	streams        map[string]*streamState
	seq            atomic.Uint64
}

// NewManager 创建内存态直播状态管理器。
// 这里保留会话历史，便于健康面板和归档面板展示已结束/失败任务。
func NewManager(baseArchiveDir string) *Manager {
	return &Manager{
		baseArchiveDir: baseArchiveDir,
		streams:        make(map[string]*streamState),
	}
}

// Start 在 ffmpeg 启动前先登记会话。
// 调用方在 cmd.Start 成功后应执行 MarkRunning，失败时执行 TouchFailure。
func (m *Manager) Start(options StartOptions) (StreamSnapshot, error) {
	targets := sanitizeTargets(append([]string{options.PrimaryTarget}, options.RelayTargets...))
	if len(targets) == 0 {
		return StreamSnapshot{}, fmt.Errorf("no valid output targets")
	}

	segmentSeconds := options.SegmentSeconds
	if segmentSeconds <= 0 {
		segmentSeconds = 300
	}

	streamID := m.nextID()
	archiveDir := ""
	if options.ArchiveEnabled {
		archiveDir = filepath.Join(m.baseArchiveDir, streamID)
	}

	now := time.Now()
	snapshot := StreamSnapshot{
		StreamID:       streamID,
		DisplayName:    strings.TrimSpace(options.DisplayName),
		Input:          strings.TrimSpace(options.Input),
		Targets:        targets,
		Source:         options.Source,
		Status:         "starting",
		ArchiveEnabled: options.ArchiveEnabled,
		SegmentSeconds: segmentSeconds,
		ArchiveDir:     archiveDir,
		Health:         HealthHealthy,
		Diagnosis:      "starting",
		StartedAt:      now,
		UpdatedAt:      now,
	}

	m.mu.Lock()
	m.streams[streamID] = &streamState{
		snapshot:      snapshot,
		lastIngressAt: now,
	}
	m.mu.Unlock()

	return snapshot, nil
}

// MarkRunning 将会话状态从 starting 切换到 running。
func (m *Manager) MarkRunning(streamID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.streams[streamID]
	if !ok {
		return
	}
	state.snapshot.Status = "running"
	state.snapshot.Diagnosis = "running"
	state.snapshot.UpdatedAt = time.Now()
}

// MarkFinished 记录会话终态。
// stoppedByUser 单独区分，避免把用户主动停止误判为失败。
func (m *Manager) MarkFinished(streamID string, stoppedByUser bool, runErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.streams[streamID]
	if !ok {
		return
	}

	now := time.Now()
	state.snapshot.UpdatedAt = now
	if stoppedByUser {
		state.snapshot.Status = "stopped"
		if state.snapshot.Diagnosis == "" || state.snapshot.Diagnosis == "running" {
			state.snapshot.Diagnosis = "stopped by user"
		}
		return
	}

	if runErr != nil {
		state.snapshot.Status = "failed"
		state.snapshot.LastError = runErr.Error()
		state.snapshot.Health = HealthCritical
		state.snapshot.Diagnosis = "ffmpeg exited with error"
		return
	}

	state.snapshot.Status = "completed"
	state.snapshot.Diagnosis = "completed"
}

// TouchFailure 用于标记启动阶段或进度解析阶段的异常。
func (m *Manager) TouchFailure(streamID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.streams[streamID]
	if !ok {
		return
	}

	state.snapshot.Status = "failed"
	state.snapshot.Health = HealthCritical
	state.snapshot.UpdatedAt = time.Now()
	if err != nil {
		state.snapshot.LastError = err.Error()
		state.snapshot.Diagnosis = "setup failed"
	}
}

// AddIngressBytes 用于 websocket 推流场景估算入口码率。
// 采用约 0.8s 窗口做聚合，避免单包抖动导致指标噪声过大。
func (m *Manager) AddIngressBytes(streamID string, count int) {
	if count <= 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.streams[streamID]
	if !ok {
		return
	}

	state.ingressBytesInSlice += int64(count)
	now := time.Now()
	elapsed := now.Sub(state.lastIngressAt).Seconds()
	if elapsed < 0.8 {
		return
	}

	state.snapshot.IngressBitrateKbps = float64(state.ingressBytesInSlice*8) / 1000 / elapsed
	state.ingressBytesInSlice = 0
	state.lastIngressAt = now
	state.snapshot.UpdatedAt = now
}

// UpdateProgress 解析 ffmpeg -progress 输出并刷新快照指标。
// EstimatedLatencyMs 通过“墙上时钟耗时 - 已编码媒体时长”做近似估算。
func (m *Manager) UpdateProgress(streamID string, key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.streams[streamID]
	if !ok {
		return
	}

	snap := &state.snapshot
	now := time.Now()
	snap.UpdatedAt = now

	switch key {
	case "fps":
		snap.FPS = parseNumeric(value)
	case "bitrate":
		snap.BitrateKbps = parseBitrate(value)
	case "speed":
		snap.Speed = parseSpeed(value)
	case "out_time_ms":
		parsed := int64(parseNumeric(value))
		snap.OutTimeMs = parsed
		encodedMs := parsed / 1000
		elapsedMs := now.Sub(snap.StartedAt).Milliseconds()
		if elapsedMs > encodedMs {
			snap.EstimatedLatencyMs = elapsedMs - encodedMs
		} else {
			snap.EstimatedLatencyMs = 0
		}
	case "drop_frames":
		snap.DropFrames = int64(parseNumeric(value))
	case "dup_frames":
		snap.DupFrames = int64(parseNumeric(value))
	case "progress":
		if strings.EqualFold(strings.TrimSpace(value), "continue") {
			snap.Status = "running"
		}
	}

	applyHealth(snap)
}

func (m *Manager) Snapshot(streamID string) (StreamSnapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.streams[streamID]
	if !ok {
		return StreamSnapshot{}, false
	}
	return copySnapshot(state.snapshot), true
}

func (m *Manager) ListSnapshots() []StreamSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]StreamSnapshot, 0, len(m.streams))
	for _, state := range m.streams {
		items = append(items, copySnapshot(state.snapshot))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items
}

func (m *Manager) ListArchives() ([]ArchiveItem, error) {
	entries, err := filepath.Glob(filepath.Join(m.baseArchiveDir, "*", "*.mp4"))
	if err != nil {
		return nil, fmt.Errorf("glob archive files: %w", err)
	}

	archives := make([]ArchiveItem, 0, len(entries))
	for _, fullPath := range entries {
		info, statErr := osStat(fullPath)
		if statErr != nil {
			continue
		}

		relPath, relErr := filepath.Rel("public", fullPath)
		if relErr != nil {
			continue
		}

		dir := filepath.Dir(fullPath)
		streamID := filepath.Base(dir)
		archives = append(archives, ArchiveItem{
			StreamID:  streamID,
			FileName:  filepath.Base(fullPath),
			FileURL:   "http://localhost:19200/public/" + filepath.ToSlash(relPath),
			SizeBytes: info.Size(),
			UpdatedAt: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	sort.Slice(archives, func(i, j int) bool {
		return archives[i].UpdatedAt > archives[j].UpdatedAt
	})
	return archives, nil
}

func (m *Manager) nextID() string {
	id := m.seq.Add(1)
	return fmt.Sprintf("live-%d-%d", time.Now().Unix(), id)
}

func copySnapshot(snapshot StreamSnapshot) StreamSnapshot {
	copied := snapshot
	copied.Targets = append([]string(nil), snapshot.Targets...)
	return copied
}

func sanitizeTargets(targets []string) []string {
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

func applyHealth(snapshot *StreamSnapshot) {
	// 终态直接给出确定健康值，避免历史告警残留在已结束任务上。
	if snapshot.Status == "failed" {
		snapshot.Health = HealthCritical
		snapshot.Diagnosis = "ffmpeg exited with error"
		return
	}
	if snapshot.Status == "completed" || snapshot.Status == "stopped" {
		snapshot.Health = HealthHealthy
		if snapshot.Diagnosis == "" {
			snapshot.Diagnosis = snapshot.Status
		}
		return
	}

	level := HealthHealthy
	reasons := make([]string, 0, 4)
	if snapshot.EstimatedLatencyMs > 3500 {
		level = HealthCritical
		reasons = append(reasons, "latency > 3500ms")
	} else if snapshot.EstimatedLatencyMs > 1800 {
		if level != HealthCritical {
			level = HealthWarning
		}
		reasons = append(reasons, "latency > 1800ms")
	}

	if snapshot.Speed > 0 && snapshot.Speed < 0.85 {
		level = HealthCritical
		reasons = append(reasons, "encode speed < 0.85x")
	} else if snapshot.Speed > 0 && snapshot.Speed < 0.95 {
		if level != HealthCritical {
			level = HealthWarning
		}
		reasons = append(reasons, "encode speed < 0.95x")
	}

	if snapshot.DropFrames > 120 {
		level = HealthCritical
		reasons = append(reasons, "drop frames > 120")
	} else if snapshot.DropFrames > 30 {
		if level != HealthCritical {
			level = HealthWarning
		}
		reasons = append(reasons, "drop frames > 30")
	}

	if snapshot.FPS > 0 && snapshot.FPS < 18 {
		if level == HealthHealthy {
			level = HealthWarning
		}
		reasons = append(reasons, "fps < 18")
	}

	snapshot.Health = level
	if len(reasons) == 0 {
		snapshot.Diagnosis = "running"
		return
	}
	snapshot.Diagnosis = strings.Join(reasons, "; ")
}
