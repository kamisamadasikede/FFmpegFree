package langasr

import (
	"context"
	"os"
	"path/filepath"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

// ExtractMono16kWAV 用转换组件抽出 16 kHz 单声道 WAV 到 dest（契约 6.18.2）。
func ExtractMono16kWAV(ctx context.Context, exe, input, dest string) error {
	if exe == "" {
		return apperr.New(apperr.FFmpegNotFound, "未找到可用的转换组件，请先安装或手动指定转换组件所在位置")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return apperr.Wrap(apperr.IOError, "无法创建临时文件夹", err)
	}
	args := []string{
		"-i", input,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "pcm_s16le",
		"-f", "wav",
		dest,
	}
	_, err := ffmpeg.Run(ctx, ffmpeg.RunOptions{Exe: exe, Args: args, TailLines: 40})
	if err != nil {
		if apperr.Is(err, apperr.FFmpegNotFound) || apperr.Is(err, apperr.ConvertDiskFull) || apperr.Is(err, apperr.Canceled) {
			return err
		}
		// 抽音失败归入识别失败（用户可见不出现技术词）
		return apperr.New(apperr.LangAsrFailed, MsgFailed).WithDetail("reason=extract")
	}
	fi, err := os.Stat(dest)
	if err != nil || fi.Size() == 0 {
		return apperr.New(apperr.LangAsrFailed, MsgFailed).WithDetail("reason=extract_empty")
	}
	return nil
}
