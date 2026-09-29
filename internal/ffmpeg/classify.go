package ffmpeg

import (
	"strings"

	"FFmpegFree/internal/apperr"
)

// ClassifyConvertError 把转换 / 剪辑类 ffmpeg 任务的非零退出归类为契约错误码，stderr 尾部由 Run 放进 detail。
// 不认识的返回 nil（Run 用默认的 PROCESS_FAILED）。
//
// 匹配前先去掉 stderr 里携带用户数据的段落：`Input #` / `Output #` / `Stream mapping:` 及其缩进的元数据、
// `Stream #`、`Metadata:`、`Duration:` 行（文件名、标题、艺术家等可能包含任何词）。
// 剩下的行里，系统错误文本（strerror）只按"行尾"匹配（ffmpeg 总是把它放在句子最后，文件名在它前面），
// 磁盘满还要求出现写入阶段的固定句式（Error writing trailer / Error muxing packet / Error while writing /
// av_interleaved_write_frame / av_write_frame），不因为文件名里有 ENOSPC 等词误报。
//
//	磁盘满（No space left on device / ENOSPC / Disk quota exceeded / Windows "There is not enough space on the disk"）→ CONVERT_DISK_FULL
//	没有写权限、输出目录不存在、只读文件系统           → IO_ERROR
//	输入文件不存在或读不了                             → IO_ERROR（NOT_FOUND 由提交前的检查产生）
//	输入损坏 / 不是媒体文件（Invalid data / moov atom not found / Could not find codec parameters）→ PROBE_FAILED
//	编码器 / 滤镜在当前 ffmpeg 构建里不存在              → PROCESS_FAILED，message 说明缺少什么
func ClassifyConvertError(tail string, _ error) *apperr.AppError {
	lines := classifiableLines(tail)
	joined := strings.Join(lines, "\n")
	contains := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(joined, s) {
				return true
			}
		}
		return false
	}
	endsWith := func(subs ...string) bool {
		for _, l := range lines {
			l = strings.TrimRight(l, " \t\r.。!")
			for _, s := range subs {
				if strings.HasSuffix(l, s) {
					return true
				}
			}
		}
		return false
	}
	switch {
	case endsWith("no space left on device", "disk quota exceeded", "enospc", "there is not enough space on the disk", "not enough space on the disk", "磁盘空间不足") &&
		contains("error writing trailer", "error muxing packet", "error while writing", "av_interleaved_write_frame", "av_write_frame", "error writing", "write error"):
		return apperr.New(apperr.ConvertDiskFull, "磁盘空间不足，无法写入输出文件")
	case endsWith("read-only file system"):
		return apperr.New(apperr.IOError, "输出位置是只读的，无法写入")
	case endsWith("permission denied", "operation not permitted", "access is denied", "拒绝访问"):
		return apperr.New(apperr.IOError, "没有权限读写文件，请检查输出目录或输入文件的权限")
	case endsWith("no such file or directory", "the system cannot find the path specified", "the system cannot find the file specified", "the system cannot find the path", "the system cannot find the file"):
		return apperr.New(apperr.IOError, "找不到文件或输出目录")
	case contains("unknown encoder", "encoder not found", "unrecognized option 'c:v'"):
		return apperr.New(apperr.ProcessFailed, "当前 ffmpeg 不包含所需的编码器，请安装完整版 ffmpeg 或换一种编码")
	case contains("no such filter", "filter not found"):
		return apperr.New(apperr.ProcessFailed, "当前 ffmpeg 不包含所需的滤镜，请安装完整版 ffmpeg")
	case endsWith("invalid data found when processing input") || contains("moov atom not found", "could not find codec parameters"):
		return apperr.New(apperr.ProbeFailed, "输入文件已损坏或不是有效的音视频文件")
	case contains("could not write header", "incorrect codec parameters"):
		return apperr.New(apperr.ProcessFailed, "所选编码与输出格式不兼容，请换一种编码或格式")
	}
	return nil
}

// classifiableLines 返回小写化后、去掉输入 / 输出信息块和元数据行的 stderr 行。
// tail 可能从块中间开始（只保留最后 50 行），所以开头处按"仍在块内"处理：开头连续的缩进行丢弃。
func classifiableLines(tail string) []string {
	var out []string
	inBlock := true
	for _, raw := range strings.Split(tail, "\n") {
		raw = strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		indented := raw[0] == ' ' || raw[0] == '\t'
		low := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(raw, "Input #"), strings.HasPrefix(raw, "Output #"), strings.HasPrefix(raw, "Stream mapping:"):
			inBlock = true
			continue
		case strings.HasPrefix(low, "stream #"), strings.HasPrefix(low, "metadata:"), strings.HasPrefix(low, "duration:"),
			strings.HasPrefix(low, "chapter #"), strings.HasPrefix(low, "side data:"):
			continue
		case indented && inBlock:
			continue
		}
		inBlock = false
		out = append(out, low)
	}
	return out
}
