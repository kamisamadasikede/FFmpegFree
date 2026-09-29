package ffmpeg

import (
	"errors"
	"strings"
)

// tee muxer 的 slave 描述里 | 是 slave 分隔符，' \ [ ] , : = 等是选项语法，不转义会静默写到错误的位置或直接失败
// （契约 6.10「tee 段转义规则」，ffmpeg 7.1.5 实测）。

// TeeEscape 把 s 里所有不在 [A-Za-z0-9_./-] 内、也不是非 ASCII 的字符前面加一个反斜杠。
// 非 ASCII（中日韩等）不转义（实测可用）。用于 tee 描述里的存档路径和推流地址。
func TeeEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		if r < 0x80 && !teePlain(byte(r)) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func teePlain(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return c == '_' || c == '.' || c == '/' || c == '-'
}

// ErrTeePath 表示路径不能用在 tee 描述里（Windows 设备 / 长路径前缀、控制字符）。
var ErrTeePath = errors.New("存档路径不合法")

// TeePath 把存档文件的路径转成 tee 描述里的目标：`file:` 前缀 + 转义后的路径。
// goos 为 windows 时先把 \ 换成 /（盘符冒号转义后写成 C\:/…；UNC 路径 \\server\share\x 变成 //server/share/x）。
// 拒绝以 \\?\ 或 \\.\（以及 windows 上的 //?/、//./）开头的路径，和含控制字符的路径。
func TeePath(goos, path string) (string, error) {
	for _, bad := range []string{`\\?\`, `\\.\`} {
		if strings.HasPrefix(path, bad) {
			return "", ErrTeePath
		}
	}
	if goos == "windows" {
		path = strings.ReplaceAll(path, `\`, "/")
		if strings.HasPrefix(path, "//?/") || strings.HasPrefix(path, "//./") {
			return "", ErrTeePath
		}
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return "", ErrTeePath
		}
	}
	return "file:" + TeeEscape(path), nil
}
