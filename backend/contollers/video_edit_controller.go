package contollers

import (
	"FFmpegFree/backend/live"
	"FFmpegFree/backend/utils"
	"bytes"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

// EditSourceItem represents a media source for editor.
type EditSourceItem struct {
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	URL      string `json:"url"`
	Duration string `json:"duration"`
	Date     string `json:"date"`
}

// ProbeSourceRequest is used for probing media metadata.
type ProbeSourceRequest struct {
	FileName string `json:"fileName"`
	Scope    string `json:"scope"`
}

// VideoClip describes one video clip in timeline.
type VideoClip struct {
	FileName              string  `json:"fileName"`
	Scope                 string  `json:"scope"`
	TrackID               string  `json:"trackId"`
	StartSec              float64 `json:"startSec"`
	InSec                 float64 `json:"inSec"`
	OutSec                float64 `json:"outSec"`
	Speed                 float64 `json:"speed"`
	EffectPreset          string  `json:"effectPreset"`
	TransitionToNext      string  `json:"transitionToNext"`
	TransitionDurationSec float64 `json:"transitionDurationSec"`
	Blur                  float64 `json:"blur"`
}

// AudioClip describes one audio clip in timeline.
type AudioClip struct {
	FileName string  `json:"fileName"`
	Scope    string  `json:"scope"`
	TrackID  string  `json:"trackId"`
	StartSec float64 `json:"startSec"`
	InSec    float64 `json:"inSec"`
	OutSec   float64 `json:"outSec"`
	Speed    float64 `json:"speed"`
	Volume   float64 `json:"volume"`
	DelaySec float64 `json:"delaySec"`
}

// GlobalEffects stores global video effects.
type GlobalEffects struct {
	Brightness float64 `json:"brightness"`
	Contrast   float64 `json:"contrast"`
	Saturation float64 `json:"saturation"`
	Sharpen    float64 `json:"sharpen"`
}

// EditRenderRequest is editor render payload.
type EditRenderRequest struct {
	OutputName   string        `json:"outputName"`
	OutputFormat string        `json:"outputFormat"`
	Width        int           `json:"width"`
	Height       int           `json:"height"`
	FPS          int           `json:"fps"`
	VideoTrack   []VideoClip   `json:"videoTrack"`
	AudioTrack   []AudioClip   `json:"audioTrack"`
	Effects      GlobalEffects `json:"effects"`
}

type resolvedVideoClip struct {
	InputIndex            int
	TrackOrder            int
	StartSec              float64
	InSec                 float64
	OutSec                float64
	Speed                 float64
	Duration              float64
	Preset                string
	TransitionToNext      string
	TransitionDurationSec float64
	Blur                  float64
}

type resolvedAudioClip struct {
	InputIndex int
	TrackOrder int
	StartSec   float64
	InSec      float64
	OutSec     float64
	Speed      float64
	Volume     float64
	Duration   float64
}

// GetEditSources lists available editor sources.
func GetEditSources(c *gin.Context) {
	scopeDirs := map[string]string{
		"user":      filepath.Join("public", "user"),
		"steam":     filepath.Join("public", "steam"),
		"converted": filepath.Join("public", "convertedUp"),
	}

	items := make([]EditSourceItem, 0, 64)
	for scope, dir := range scopeDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !isEditMediaFile(entry.Name()) {
				continue
			}
			fullPath := filepath.Join(dir, entry.Name())
			fileInfo, statErr := os.Stat(fullPath)
			if statErr != nil {
				continue
			}
			items = append(items, EditSourceItem{
				Name:     entry.Name(),
				Scope:    scope,
				URL:      "http://localhost:19200/" + filepath.ToSlash(fullPath),
				Duration: getVideoDuration(fullPath),
				Date:     fileInfo.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Date > items[j].Date
	})

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"count": len(items),
		"items": items,
	}))
}

// ProbeEditSource probes metadata from one source.
func ProbeEditSource(c *gin.Context) {
	var request ProbeSourceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "failed to parse request"))
		return
	}

	path, err := resolveEditSourcePath(request.Scope, request.FileName)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, err.Error()))
		return
	}

	meta, err := probeMediaMeta(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.Fail(500, fmt.Sprintf("failed to probe source: %v", err)))
		return
	}

	c.JSON(http.StatusOK, utils.Success(meta))
}

// RenderEditProject renders final media via ffmpeg.
func RenderEditProject(c *gin.Context) {
	var request EditRenderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, "failed to parse request"))
		return
	}

	normalizeEditRequest(&request)
	if err := validateEditRequest(request); err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, err.Error()))
		return
	}

	args, outputPath, err := buildEditFFmpegArgs(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.Fail(500, err.Error()))
		return
	}

	cmd := exec.Command(live.FFmpegBinaryPath(), args...)
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = hideWindowProcAttr()
	}

	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		message := string(output)
		if len(message) > 1200 {
			message = message[len(message)-1200:]
		}
		c.JSON(http.StatusInternalServerError, utils.Fail(500, fmt.Sprintf("ffmpeg render failed: %v, log: %s", runErr, message)))
		return
	}

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"outputPath": outputPath,
		"outputUrl":  "http://localhost:19200/" + filepath.ToSlash(outputPath),
		"message":    "render completed",
	}))
}

// normalizeEditRequest fills defaults and sanitizes clips.
func normalizeEditRequest(request *EditRenderRequest) {
	request.OutputName = strings.TrimSpace(request.OutputName)
	request.OutputFormat = strings.ToLower(strings.TrimSpace(request.OutputFormat))
	if request.OutputFormat == "" {
		request.OutputFormat = "mp4"
	}
	if request.Width <= 0 {
		request.Width = 1280
	}
	if request.Height <= 0 {
		request.Height = 720
	}
	if request.FPS <= 0 {
		request.FPS = 30
	}
	if request.Effects.Contrast <= 0 {
		request.Effects.Contrast = 1
	}
	if request.Effects.Saturation <= 0 {
		request.Effects.Saturation = 1
	}

	for i := range request.VideoTrack {
		clip := &request.VideoTrack[i]
		clip.Scope = normalizeSourceScope(clip.Scope)
		clip.FileName = strings.TrimSpace(clip.FileName)
		clip.TrackID = normalizeTrackID(clip.TrackID, "V")
		clip.StartSec = math.Max(0, clip.StartSec)
		clip.InSec = math.Max(0, clip.InSec)
		clip.EffectPreset = strings.ToLower(strings.TrimSpace(clip.EffectPreset))
		clip.TransitionToNext = normalizeTransitionName(clip.TransitionToNext)
		if clip.TransitionDurationSec <= 0 {
			clip.TransitionDurationSec = 0.5
		}
		if clip.TransitionDurationSec > 2 {
			clip.TransitionDurationSec = 2
		}
		if clip.Speed <= 0 {
			clip.Speed = 1
		}
		if clip.Blur < 0 {
			clip.Blur = 0
		}
	}

	for i := range request.AudioTrack {
		clip := &request.AudioTrack[i]
		clip.Scope = normalizeSourceScope(clip.Scope)
		clip.FileName = strings.TrimSpace(clip.FileName)
		clip.TrackID = normalizeTrackID(clip.TrackID, "A")
		if clip.StartSec == 0 && clip.DelaySec > 0 {
			clip.StartSec = clip.DelaySec
		}
		clip.StartSec = math.Max(0, clip.StartSec)
		clip.InSec = math.Max(0, clip.InSec)
		if clip.Speed <= 0 {
			clip.Speed = 1
		}
		// 音量允许为 0（静音），仅在非法负数时兜底。
		if clip.Volume < 0 {
			clip.Volume = 1
		}
		if clip.DelaySec < 0 {
			clip.DelaySec = 0
		}
	}
}

func validateEditRequest(request EditRenderRequest) error {
	supportedFormats := map[string]struct{}{
		"mp4":  {},
		"mov":  {},
		"mkv":  {},
		"webm": {},
	}
	if _, ok := supportedFormats[request.OutputFormat]; !ok {
		return fmt.Errorf("unsupported output format: %s", request.OutputFormat)
	}

	if len(request.VideoTrack) == 0 {
		return fmt.Errorf("video track cannot be empty")
	}

	for i, clip := range request.VideoTrack {
		if clip.FileName == "" {
			return fmt.Errorf("video track clip %d missing fileName", i+1)
		}
		if clip.OutSec > 0 && clip.OutSec <= clip.InSec {
			return fmt.Errorf("video track clip %d has invalid in/out range", i+1)
		}
		if clip.StartSec < 0 {
			return fmt.Errorf("video track clip %d has invalid startSec", i+1)
		}
		if !isValidTransitionName(clip.TransitionToNext) {
			return fmt.Errorf("video track clip %d has unsupported transition: %s", i+1, clip.TransitionToNext)
		}
		if clip.TransitionDurationSec < 0 {
			return fmt.Errorf("video track clip %d has invalid transition duration", i+1)
		}
	}

	for i, clip := range request.AudioTrack {
		if clip.FileName == "" {
			return fmt.Errorf("audio track clip %d missing fileName", i+1)
		}
		if clip.OutSec > 0 && clip.OutSec <= clip.InSec {
			return fmt.Errorf("audio track clip %d has invalid in/out range", i+1)
		}
		if clip.StartSec < 0 {
			return fmt.Errorf("audio track clip %d has invalid startSec", i+1)
		}
	}

	return nil
}

// buildEditFFmpegArgs builds ffmpeg args with timeline composition.
func buildEditFFmpegArgs(request EditRenderRequest) ([]string, string, error) {
	args := []string{"-y"}

	videos := make([]resolvedVideoClip, 0, len(request.VideoTrack))
	for i, clip := range request.VideoTrack {
		path, err := resolveEditSourcePath(clip.Scope, clip.FileName)
		if err != nil {
			return nil, "", err
		}
		args = append(args, "-i", path)

		outSec := clip.OutSec
		if outSec <= clip.InSec {
			outSec = clipSourceDurationSec(path)
		}
		if outSec <= clip.InSec {
			return nil, "", fmt.Errorf("invalid source duration for video: %s", clip.FileName)
		}

		speed := clampSpeed(clip.Speed)
		duration := math.Max(0.04, (outSec-clip.InSec)/speed)
		videos = append(videos, resolvedVideoClip{
			InputIndex:            i,
			TrackOrder:            parseTrackOrder(clip.TrackID),
			StartSec:              clip.StartSec,
			InSec:                 clip.InSec,
			OutSec:                outSec,
			Speed:                 speed,
			Duration:              duration,
			Preset:                clip.EffectPreset,
			TransitionToNext:      clip.TransitionToNext,
			TransitionDurationSec: clip.TransitionDurationSec,
			Blur:                  clip.Blur,
		})
	}

	// 音频导出严格基于 audioTrack：不会自动回退到视频原声。
	audios := make([]resolvedAudioClip, 0, len(request.AudioTrack))
	audioOffset := len(request.VideoTrack)
	for idx, clip := range request.AudioTrack {
		path, err := resolveEditSourcePath(clip.Scope, clip.FileName)
		if err != nil {
			return nil, "", err
		}
		args = append(args, "-i", path)

		outSec := clip.OutSec
		if outSec <= clip.InSec {
			outSec = clipSourceDurationSec(path)
		}
		if outSec <= clip.InSec {
			return nil, "", fmt.Errorf("invalid source duration for audio: %s", clip.FileName)
		}

		speed := clampSpeed(clip.Speed)
		duration := math.Max(0.04, (outSec-clip.InSec)/speed)
		audios = append(audios, resolvedAudioClip{
			InputIndex: audioOffset + idx,
			TrackOrder: parseTrackOrder(clip.TrackID),
			StartSec:   math.Max(0, clip.StartSec),
			InSec:      clip.InSec,
			OutSec:     outSec,
			Speed:      speed,
			Volume:     clampVolume(clip.Volume),
			Duration:   duration,
		})
	}

	timelineDuration := computeTimelineDuration(videos, audios)
	if timelineDuration <= 0 {
		return nil, "", fmt.Errorf("invalid timeline duration")
	}

	filters := make([]string, 0, len(videos)*4+len(audios)+16)
	filters = append(filters, fmt.Sprintf("color=c=black:s=%d:%d:r=%d:d=%.3f[base]", request.Width, request.Height, request.FPS, timelineDuration))

	sort.Slice(videos, func(i, j int) bool {
		if videos[i].TrackOrder != videos[j].TrackOrder {
			return videos[i].TrackOrder < videos[j].TrackOrder
		}
		if videos[i].StartSec != videos[j].StartSec {
			return videos[i].StartSec < videos[j].StartSec
		}
		return videos[i].InputIndex < videos[j].InputIndex
	})

	type overlayVideoSegment struct {
		Label      string
		StartSec   float64
		TrackOrder int
	}

	clipsByTrack := make(map[int][]resolvedVideoClip)
	trackOrders := make([]int, 0, 8)
	for _, seg := range videos {
		if _, ok := clipsByTrack[seg.TrackOrder]; !ok {
			trackOrders = append(trackOrders, seg.TrackOrder)
		}
		clipsByTrack[seg.TrackOrder] = append(clipsByTrack[seg.TrackOrder], seg)
	}
	sort.Ints(trackOrders)

	overlaySegments := make([]overlayVideoSegment, 0, len(videos))
	for _, trackOrder := range trackOrders {
		trackClips := clipsByTrack[trackOrder]
		if len(trackClips) == 0 {
			continue
		}

		sort.Slice(trackClips, func(i, j int) bool {
			if trackClips[i].StartSec != trackClips[j].StartSec {
				return trackClips[i].StartSec < trackClips[j].StartSec
			}
			return trackClips[i].InputIndex < trackClips[j].InputIndex
		})

		clipLabels := make([]string, 0, len(trackClips))
		for i, seg := range trackClips {
			clipLabel := fmt.Sprintf("t%d_clip%d", trackOrder, i)
			videoChain := []string{
				buildTrimExpr(seg.InSec, seg.OutSec, false),
				fmt.Sprintf("setpts=(PTS-STARTPTS)/%.4f", seg.Speed),
				fmt.Sprintf("fps=%d", request.FPS),
				fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", request.Width, request.Height),
				fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black", request.Width, request.Height),
				buildPresetEffect(seg.Preset),
				buildGlobalEffect(request.Effects),
			}
			if seg.Blur > 0 {
				videoChain = append(videoChain, fmt.Sprintf("boxblur=%.2f:1", math.Min(4, seg.Blur)))
			}
			videoChain = append(videoChain, "format=yuv420p")
			filters = append(filters, fmt.Sprintf("[%d:v]%s[%s]", seg.InputIndex, strings.Join(videoChain, ","), clipLabel))
			clipLabels = append(clipLabels, clipLabel)
		}

		chainLabel := clipLabels[0]
		chainStartSec := trackClips[0].StartSec
		chainDuration := trackClips[0].Duration

		for i := 1; i < len(trackClips); i++ {
			prevClip := trackClips[i-1]
			currClip := trackClips[i]
			expectedStart := prevClip.StartSec + prevClip.Duration
			isContiguous := math.Abs(currClip.StartSec-expectedStart) <= 0.12
			nextLabel := clipLabels[i]

			if isContiguous {
				transitionName := normalizeTransitionName(prevClip.TransitionToNext)
				transitionDuration := clampTransitionDuration(prevClip.TransitionDurationSec, prevClip.Duration, currClip.Duration)
				if transitionName != "none" && transitionDuration > 0 {
					xfadeLabel := fmt.Sprintf("t%d_xfade%d", trackOrder, i)
					offsetSec := math.Max(0, chainDuration-transitionDuration)
					filters = append(filters, fmt.Sprintf("[%s][%s]xfade=transition=%s:duration=%.3f:offset=%.3f[%s]", chainLabel, nextLabel, transitionName, transitionDuration, offsetSec, xfadeLabel))
					chainLabel = xfadeLabel
					chainDuration += currClip.Duration - transitionDuration
					continue
				}

				concatLabel := fmt.Sprintf("t%d_concat%d", trackOrder, i)
				filters = append(filters, fmt.Sprintf("[%s][%s]concat=n=2:v=1:a=0[%s]", chainLabel, nextLabel, concatLabel))
				chainLabel = concatLabel
				chainDuration += currClip.Duration
				continue
			}

			overlaySegments = append(overlaySegments, overlayVideoSegment{
				Label:      chainLabel,
				StartSec:   chainStartSec,
				TrackOrder: trackOrder,
			})
			chainLabel = nextLabel
			chainStartSec = currClip.StartSec
			chainDuration = currClip.Duration
		}

		overlaySegments = append(overlaySegments, overlayVideoSegment{
			Label:      chainLabel,
			StartSec:   chainStartSec,
			TrackOrder: trackOrder,
		})
	}

	sort.Slice(overlaySegments, func(i, j int) bool {
		if overlaySegments[i].TrackOrder != overlaySegments[j].TrackOrder {
			return overlaySegments[i].TrackOrder < overlaySegments[j].TrackOrder
		}
		return overlaySegments[i].StartSec < overlaySegments[j].StartSec
	})

	currentVideoLabel := "base"
	for i, seg := range overlaySegments {
		shiftLabel := fmt.Sprintf("vshift%d", i)
		filters = append(filters, fmt.Sprintf("[%s]setpts=PTS+%.3f/TB[%s]", seg.Label, seg.StartSec, shiftLabel))
		nextLabel := fmt.Sprintf("vmix%d", i)
		filters = append(filters, fmt.Sprintf("[%s][%s]overlay=shortest=0:eof_action=pass:repeatlast=0[%s]", currentVideoLabel, shiftLabel, nextLabel))
		currentVideoLabel = nextLabel
	}
	filters = append(filters, fmt.Sprintf("[%s]format=yuv420p[vout]", currentVideoLabel))

	audioOutLabel := "aout"
	if len(audios) == 0 {
		// 未配置音轨时导出静音，确保容器仍有标准音频流。
		filters = append(filters, fmt.Sprintf("anullsrc=r=48000:cl=stereo,atrim=end=%.3f[%s]", timelineDuration, audioOutLabel))
	} else {
		sort.Slice(audios, func(i, j int) bool {
			if audios[i].TrackOrder != audios[j].TrackOrder {
				return audios[i].TrackOrder < audios[j].TrackOrder
			}
			if audios[i].StartSec != audios[j].StartSec {
				return audios[i].StartSec < audios[j].StartSec
			}
			return audios[i].InputIndex < audios[j].InputIndex
		})

		for i, seg := range audios {
			audioChain := []string{
				buildTrimExpr(seg.InSec, seg.OutSec, true),
				"asetpts=PTS-STARTPTS",
			}
			audioChain = append(audioChain, buildATempo(seg.Speed)...)
			audioChain = append(audioChain, fmt.Sprintf("volume=%.3f", seg.Volume))
			if seg.StartSec > 0 {
				delayMs := int(math.Round(seg.StartSec * 1000))
				audioChain = append(audioChain, fmt.Sprintf("adelay=%d|%d", delayMs, delayMs))
			}
			filters = append(filters, fmt.Sprintf("[%d:a]%s[aseg%d]", seg.InputIndex, strings.Join(audioChain, ","), i))
		}

		if len(audios) == 1 {
			filters = append(filters, "[aseg0]anull[amix]")
		} else {
			inputs := make([]string, 0, len(audios))
			for i := range audios {
				inputs = append(inputs, fmt.Sprintf("[aseg%d]", i))
			}
			filters = append(filters, strings.Join(inputs, "")+fmt.Sprintf("amix=inputs=%d:normalize=0:dropout_transition=0[amix]", len(audios)))
		}
		filters = append(filters, fmt.Sprintf("[amix]atrim=end=%.3f,asetpts=PTS-STARTPTS[%s]", timelineDuration, audioOutLabel))
	}

	args = append(args, "-filter_complex", strings.Join(filters, ";"))
	args = append(args, "-map", "[vout]")
	args = append(args, "-map", "[aout]")
	args = append(args, "-t", fmt.Sprintf("%.3f", timelineDuration))

	switch request.OutputFormat {
	case "webm":
		args = append(args, "-c:v", "libvpx-vp9", "-b:v", "2M", "-c:a", "libopus", "-b:a", "128k")
	default:
		args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "20")
		args = append(args, "-c:a", "aac", "-b:a", "192k")
		if request.OutputFormat == "mp4" {
			args = append(args, "-movflags", "+faststart")
		}
	}

	outputDir := filepath.Join("public", "edit")
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return nil, "", fmt.Errorf("failed to create output dir: %w", err)
	}

	baseName := sanitizeOutputName(request.OutputName)
	if baseName == "" {
		baseName = "edit_render"
	}
	fileName := fmt.Sprintf("%s_%s.%s", baseName, time.Now().Format("20060102_150405"), request.OutputFormat)
	outputPath := filepath.Join(outputDir, fileName)
	args = append(args, outputPath)

	return args, outputPath, nil
}

func computeTimelineDuration(videos []resolvedVideoClip, audios []resolvedAudioClip) float64 {
	maxEnd := 0.0
	for _, seg := range videos {
		maxEnd = math.Max(maxEnd, seg.StartSec+seg.Duration)
	}
	for _, seg := range audios {
		maxEnd = math.Max(maxEnd, seg.StartSec+seg.Duration)
	}
	return math.Max(0.1, maxEnd)
}

func resolveEditSourcePath(scope string, fileName string) (string, error) {
	scope = normalizeSourceScope(scope)
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "", fmt.Errorf("source fileName cannot be empty")
	}
	if filepath.Base(fileName) != fileName {
		return "", fmt.Errorf("invalid source fileName")
	}

	dirMap := map[string]string{
		"user":      filepath.Join("public", "user"),
		"steam":     filepath.Join("public", "steam"),
		"converted": filepath.Join("public", "convertedUp"),
	}
	dir, ok := dirMap[scope]
	if !ok {
		return "", fmt.Errorf("unknown source scope: %s", scope)
	}

	path := filepath.Join(dir, fileName)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("source not found: %s", path)
	}
	return path, nil
}

func normalizeSourceScope(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch scope {
	case "steam", "converted":
		return scope
	default:
		return "user"
	}
}

func normalizeTrackID(trackID string, prefix string) string {
	value := strings.ToUpper(strings.TrimSpace(trackID))
	if value == "" {
		return prefix + "1"
	}
	if strings.HasPrefix(value, "V") || strings.HasPrefix(value, "A") {
		return value
	}
	return prefix + value
}

func parseTrackOrder(trackID string) int {
	re := regexp.MustCompile(`\d+`)
	text := re.FindString(trackID)
	if text == "" {
		return 1
	}
	order, err := strconv.Atoi(text)
	if err != nil || order <= 0 {
		return 1
	}
	return order
}

func normalizeTransitionName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "fade", "wipeleft", "wiperight", "slideleft", "slideright", "circleopen", "circleclose", "dissolve":
		return strings.ToLower(strings.TrimSpace(name))
	default:
		return "none"
	}
}

func isValidTransitionName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "none", "fade", "wipeleft", "wiperight", "slideleft", "slideright", "circleopen", "circleclose", "dissolve":
		return true
	default:
		return false
	}
}

func clampTransitionDuration(value float64, leftClipDuration float64, rightClipDuration float64) float64 {
	if value <= 0 {
		return 0
	}
	maxByLeft := math.Max(0, leftClipDuration-0.02)
	maxByRight := math.Max(0, rightClipDuration-0.02)
	maxAllowed := math.Min(2, math.Min(maxByLeft, maxByRight))
	if maxAllowed <= 0 {
		return 0
	}
	return math.Min(value, maxAllowed)
}

func sanitizeOutputName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
	clean := re.ReplaceAllString(name, "_")
	clean = strings.Trim(clean, "_")
	return clean
}

func buildTrimExpr(inSec float64, outSec float64, audio bool) string {
	prefix := "trim"
	if audio {
		prefix = "atrim"
	}
	start := math.Max(0, inSec)
	if outSec > start {
		return fmt.Sprintf("%s=start=%.3f:end=%.3f", prefix, start, outSec)
	}
	return fmt.Sprintf("%s=start=%.3f", prefix, start)
}

func buildPresetEffect(preset string) string {
	switch preset {
	case "grayscale":
		return "hue=s=0"
	case "sepia":
		return "colorchannelmixer=.393:.769:.189:.349:.686:.168:.272:.534:.131"
	case "vintage":
		return "eq=saturation=0.75:contrast=1.10:brightness=-0.04"
	case "cinematic":
		return "eq=contrast=1.15:saturation=1.20:brightness=-0.02"
	default:
		return "null"
	}
}

func buildGlobalEffect(effects GlobalEffects) string {
	brightness := effects.Brightness
	contrast := effects.Contrast
	saturation := effects.Saturation
	if contrast <= 0 {
		contrast = 1
	}
	if saturation <= 0 {
		saturation = 1
	}
	eq := fmt.Sprintf("eq=brightness=%.3f:contrast=%.3f:saturation=%.3f", brightness, contrast, saturation)
	if effects.Sharpen > 0 {
		amount := math.Min(2, effects.Sharpen)
		return eq + fmt.Sprintf(",unsharp=5:5:%.2f:5:5:0.0", amount)
	}
	return eq
}

func buildATempo(speed float64) []string {
	speed = clampSpeed(speed)
	parts := make([]string, 0, 3)
	for speed > 2.0 {
		parts = append(parts, "atempo=2.0")
		speed = speed / 2.0
	}
	for speed < 0.5 {
		parts = append(parts, "atempo=0.5")
		speed = speed * 2.0
	}
	parts = append(parts, fmt.Sprintf("atempo=%.4f", speed))
	return parts
}

func clampSpeed(speed float64) float64 {
	if speed <= 0 {
		return 1
	}
	if speed > 8 {
		return 8
	}
	return speed
}

func clampVolume(volume float64) float64 {
	if volume < 0 {
		return 0
	}
	if volume > 4 {
		return 4
	}
	return volume
}

// isEditMediaFile allows both video and audio in editor source list.
func isEditMediaFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".mov", ".avi", ".mkv", ".flv", ".webm", ".m4v", ".mp3", ".wav", ".aac", ".m4a", ".flac", ".ogg":
		return true
	default:
		return false
	}
}

func clipSourceDurationSec(path string) float64 {
	raw := strings.TrimSpace(getVideoDuration(path))
	if raw == "" || strings.EqualFold(raw, "unknown") {
		return 0
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return 0
	}
	h, errH := strconv.ParseFloat(parts[0], 64)
	m, errM := strconv.ParseFloat(parts[1], 64)
	s, errS := strconv.ParseFloat(parts[2], 64)
	if errH != nil || errM != nil || errS != nil {
		return 0
	}
	return h*3600 + m*60 + s
}

func probeMediaMeta(path string) (gin.H, error) {
	cmd := exec.Command(live.FFmpegBinaryPath(), "-i", path)
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = hideWindowProcAttr()
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	output := stderr.String()
	if runErr != nil {
		if strings.TrimSpace(output) == "" {
			return nil, fmt.Errorf("probe command failed: %w", runErr)
		}
	}

	resolutionPattern := regexp.MustCompile(`(\d{2,5})x(\d{2,5})`)
	fpsPattern := regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?) fps`)

	width := 0
	height := 0
	fps := 0.0

	if match := resolutionPattern.FindStringSubmatch(output); len(match) == 3 {
		w, errW := strconv.Atoi(match[1])
		h, errH := strconv.Atoi(match[2])
		if errW == nil && errH == nil {
			width = w
			height = h
		}
	}
	if match := fpsPattern.FindStringSubmatch(output); len(match) == 2 {
		parsed, parseErr := strconv.ParseFloat(match[1], 64)
		if parseErr == nil {
			fps = parsed
		}
	}

	hasAudio := strings.Contains(output, "Audio:")

	return gin.H{
		"filePath":   path,
		"duration":   getVideoDuration(path),
		"width":      width,
		"height":     height,
		"fps":        fps,
		"hasAudio":   hasAudio,
		"ffmpegInfo": output,
	}, nil
}

func hideWindowProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}
