package system

import (
	"bytes"
	"image/png"
	"testing"
)

func TestDIBToPNG(t *testing.T) {
	// 1×1 24-bit bottom-up pixel: blue
	dib := make([]byte, 40+4)
	dib[0] = 40
	dib[4] = 1
	dib[8] = 1
	dib[14] = 24
	dib[40], dib[41], dib[42] = 255, 0, 0
	raw, err := dibToPNG(dib)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if r>>8 != 0 || g>>8 != 0 || b>>8 != 255 {
		t.Fatalf("pixel = %d %d %d", r>>8, g>>8, b>>8)
	}
}
