package ffmpeg

import (
	"strings"

	"FFmpegFree/internal/apperr"
)

// ClassifyConvertError 把转换 / 剪辑类 ffmpeg 任务的非零退出归类为契约错误码，stderr 尾部由 Run 放进 detail。
// 不认识的返回 nil（Run 用默认的 PROCESS_FAILED）。
//
//	磁盘满（No space left on device / ENOSPC / Windows "There is not enough space on the disk"）→ CONVERT_DISK_FULL
//	没有写权限、输出目录不存在、只读文件系统           → IO_ERROR
//	输入文件不存在或读不了                             → IO_ERROR（NOT_FOUND 由提交前的检查产生）
//	输入损坏 / 不是媒体文件                             → PROBE_FAILED
//	编码器 / 滤镜在当前 ffmpeg 构建里不存在              → PROCESS_FAILED，message 说明缺少什么
func ClassifyConvertError(tail string, _ error) *apperr.AppError {
	t := strings.ToLower(tail)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(t, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("no space left on device", "disk quota exceeded", "enospc", "there is not enough space on the disk", "not enough space on the disk", "磁盘空间不足"):
		return apperr.New(apperr.ConvertDiskFull, "磁盘空间不足，无法写入输出文件")
	case has("read-only file system"):
		return apperr.New(apperr.IOError, "输出位置是只读的，无法写入")
	case has("permission denied", "operation not permitted", "access is denied", "拒绝访问"):
		return apperr.New(apperr.IOError, "没有权限读写文件，请检查输出目录或输入文件的权限")
	case has("no such file or directory", "the system cannot find the path", "the system cannot find the file"):
		return apperr.New(apperr.IOError, "找不到文件或输出目录")
	case has("unknown encoder", "encoder not found", "unrecognized option 'c:v'"):
		return apperr.New(apperr.ProcessFailed, "当前 ffmpeg 不包含所需的编码器，请安装完整版 ffmpeg 或换一种编码")
	case has("no such filter", "filter not found"):
		return apperr.New(apperr.ProcessFailed, "当前 ffmpeg 不包含所需的滤镜，请安装完整版 ffmpeg")
	case has("invalid data found when processing input", "moov atom not found", "could not find codec parameters", "end of file"):
		return apperr.New(apperr.ProbeFailed, "输入文件已损坏或不是有效的音视频文件")
	case has("could not write header", "incorrect codec parameters"):
		return apperr.New(apperr.ProcessFailed, "所选编码与输出格式不兼容，请换一种编码或格式")
	}
	return nil
}
