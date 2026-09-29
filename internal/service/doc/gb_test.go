package doc

import (
	"errors"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// gb2312 把一个 GB2312 双字节码（0xA1A1~0xF7FE）解成 Unicode 字符；GBK 解码器对 GB2312 区是超集，区内结果一致。
func gb2312(hi, lo byte) (rune, error) {
	out, err := simplifiedchinese.GBK.NewDecoder().Bytes([]byte{hi, lo})
	if err != nil {
		return 0, err
	}
	r := []rune(string(out))
	if len(r) != 1 || r[0] == 0xFFFD {
		return 0, errors.New("bad gb2312")
	}
	return r[0], nil
}
