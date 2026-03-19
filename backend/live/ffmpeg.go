package live

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ConsumeProgress 解析 ffmpeg -progress 输出（key=value）。
// 非 key-value 的日志行会被忽略，因此可以直接读取混合日志的 stderr。
func ConsumeProgress(reader io.Reader, onKV func(key, value string)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		onKV(key, value)
	}
	return scanner.Err()
}

// BuildTeeOutput 生成 ffmpeg tee 输出配置：
// - 主目标 + 转推目标按 flv 输出
// - 可选 segment 输出用于本地分段归档
func BuildTeeOutput(targets []string, archiveDir string, segmentSeconds int) (string, error) {
	cleanTargets := sanitizeTargets(targets)
	if len(cleanTargets) == 0 {
		return "", fmt.Errorf("no targets configured")
	}

	outputs := make([]string, 0, len(cleanTargets)+1)
	for _, target := range cleanTargets {
		if strings.Contains(target, "|") {
			return "", fmt.Errorf("target contains unsupported character '|': %s", target)
		}
		outputs = append(outputs, "[f=flv:onfail=ignore]"+target)
	}

	if strings.TrimSpace(archiveDir) != "" {
		if segmentSeconds <= 0 {
			segmentSeconds = 300
		}
		segmentPattern := filepath.ToSlash(filepath.Join(archiveDir, "%Y%m%d_%H%M%S.mp4"))
		segment := fmt.Sprintf("[f=segment:segment_time=%d:reset_timestamps=1:strftime=1]", segmentSeconds) + segmentPattern
		outputs = append(outputs, segment)
	}

	return strings.Join(outputs, "|"), nil
}

func parseNumeric(value string) float64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parseBitrate(value string) float64 {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" || trimmed == "n/a" {
		return 0
	}
	trimmed = strings.TrimSuffix(trimmed, "kbits/s")
	trimmed = strings.TrimSpace(trimmed)
	return parseNumeric(trimmed)
}

func parseSpeed(value string) float64 {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" || trimmed == "n/a" {
		return 0
	}
	trimmed = strings.TrimSuffix(trimmed, "x")
	trimmed = strings.TrimSpace(trimmed)
	return parseNumeric(trimmed)
}

func osStat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// EnsureArchiveDir 提前创建归档目录，避免 ffmpeg 运行时因目录不存在而失败。
func EnsureArchiveDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}
	return nil
}

// FFmpegBinaryPath 返回项目内置 ffmpeg 路径（优先 windows 可执行文件）。
func FFmpegBinaryPath() string {
	if _, err := os.Stat("./ffmpeg/ffmpeg.exe"); err == nil {
		return "./ffmpeg/ffmpeg.exe"
	}
	return "./ffmpeg/ffmpeg"
}
