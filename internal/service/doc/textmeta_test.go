package doc

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"

	"FFmpegFree/internal/apperr"
)

func TestDetectEncodingAndLineEnding(t *testing.T) {
	utf8 := []byte("你好\n世界")
	if detectEncoding(utf8) != encUTF8 || detectLineEnding(utf8) != lineLF {
		t.Fatalf("utf8: %s %s", detectEncoding(utf8), detectLineEnding(utf8))
	}
	bom := append(append([]byte{}, utf8BOM...), utf8...)
	if detectEncoding(bom) != encUTF8BOM {
		t.Fatal(detectEncoding(bom))
	}
	gb, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("你好\r\n世界"))
	if err != nil {
		t.Fatal(err)
	}
	if detectEncoding(gb) != encGBK || detectLineEnding(gb) != lineCRLF {
		t.Fatalf("gbk: %s %s", detectEncoding(gb), detectLineEnding(gb))
	}
}

func TestEncodeStrictGBKRoundTripAndRefuseEmoji(t *testing.T) {
	text := "中文测试ABC"
	b, err := encodeStrictGBK(text)
	if err != nil {
		t.Fatal(err)
	}
	back, err := simplifiedchinese.GBK.NewDecoder().Bytes(b)
	if err != nil || string(back) != text {
		t.Fatalf("roundtrip %q %v", back, err)
	}
	_, err = encodeStrictGBK("有emoji😀")
	if err == nil || !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("want encoding refuse: %v", err)
	}
	ae := apperr.From(err)
	if !strings.Contains(ae.Detail, "reason=encoding") || !strings.Contains(ae.Detail, "char=U+") || !strings.Contains(ae.Detail, "line=") {
		t.Fatalf("detail=%q", ae.Detail)
	}
	if strings.Contains(ae.Message, "ffmpeg") || strings.Contains(ae.Message, "LibreOffice") {
		t.Fatal(ae.Message)
	}
}

func TestEncodeTextBytesNoGB18030FourByte(t *testing.T) {
	b, err := encodeTextBytes("普通中文", encGBK, lineLF)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+3 < len(b); i++ {
		// GB18030 四字节：0x81-0xFE, 0x30-0x39, 0x81-0xFE, 0x30-0x39
		if b[i] >= 0x81 && b[i+1] >= 0x30 && b[i+1] <= 0x39 && b[i+2] >= 0x81 && b[i+3] >= 0x30 && b[i+3] <= 0x39 {
			t.Fatalf("unexpected GB18030 4-byte at %d: %x", i, b[i:i+4])
		}
	}
}
