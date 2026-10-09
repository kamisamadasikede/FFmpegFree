package doc

import (
	"encoding/binary"
	"os"
	"testing"
	"unicode/utf16"
)

type cfbStream struct {
	name string
	data []byte
}

// writeCFB 写一个最小的 OLE 复合文档（v3，512 字节扇区，所有流都按 ≥4096 字节放在常规扇区，不用 mini stream）。
// 只给测试用：加密检测按流名和内容判断，这里能精确构造每一种情况。
func writeCFB(t *testing.T, path string, streams ...cfbStream) {
	t.Helper()
	const sec = 512
	const endChain, freeSect, fatSect, noStream = 0xFFFFFFFE, 0xFFFFFFFF, 0xFFFFFFFD, 0xFFFFFFFF
	nEntries := 1 + len(streams)
	dirSecs := (nEntries*128 + sec - 1) / sec
	type placed struct {
		start, n int
		size     int
	}
	var pl []placed
	next := 1 + dirSecs
	for _, s := range streams {
		size := len(s.data)
		if size < 4096 {
			size = 4096
		}
		n := (size + sec - 1) / sec
		pl = append(pl, placed{start: next, n: n, size: size})
		next += n
	}
	total := next
	if total > 128 {
		t.Fatal("测试 CFB 太大")
	}
	fat := make([]uint32, 128)
	for i := range fat {
		fat[i] = freeSect
	}
	fat[0] = fatSect
	for i := 1; i <= dirSecs; i++ {
		fat[i] = uint32(i + 1)
		if i == dirSecs {
			fat[i] = endChain
		}
	}
	for _, p := range pl {
		for i := 0; i < p.n; i++ {
			fat[p.start+i] = uint32(p.start + i + 1)
			if i == p.n-1 {
				fat[p.start+i] = endChain
			}
		}
	}
	hdr := make([]byte, sec)
	copy(hdr, oleMagic)
	le := binary.LittleEndian
	le.PutUint16(hdr[24:], 0x3E)
	le.PutUint16(hdr[26:], 3)
	le.PutUint16(hdr[28:], 0xFFFE)
	le.PutUint16(hdr[30:], 9)
	le.PutUint16(hdr[32:], 6)
	le.PutUint32(hdr[44:], 1)
	le.PutUint32(hdr[48:], 1)
	le.PutUint32(hdr[56:], 4096)
	le.PutUint32(hdr[60:], endChain)
	le.PutUint32(hdr[68:], endChain)
	for i := 0; i < 109; i++ {
		le.PutUint32(hdr[76+4*i:], freeSect)
	}
	le.PutUint32(hdr[76:], 0)
	buf := append([]byte(nil), hdr...)
	fb := make([]byte, sec)
	for i, v := range fat {
		le.PutUint32(fb[4*i:], v)
	}
	buf = append(buf, fb...)
	dir := make([]byte, dirSecs*sec)
	entry := func(i int, name string, typ byte, child, right uint32, start uint32, size uint32) {
		e := dir[i*128 : (i+1)*128]
		u := utf16.Encode([]rune(name))
		for k, c := range u {
			le.PutUint16(e[2*k:], c)
		}
		le.PutUint16(e[64:], uint16(2*(len(u)+1)))
		e[66], e[67] = typ, 1
		le.PutUint32(e[68:], noStream)
		le.PutUint32(e[72:], right)
		le.PutUint32(e[76:], child)
		le.PutUint32(e[116:], start)
		le.PutUint32(e[120:], size)
	}
	child := uint32(noStream)
	if len(streams) > 0 {
		child = 1
	}
	entry(0, "Root Entry", 5, child, noStream, endChain, 0)
	for i, s := range streams {
		right := uint32(noStream)
		if i+1 < len(streams) {
			right = uint32(i + 2)
		}
		entry(i+1, s.name, 2, noStream, right, uint32(pl[i].start), uint32(pl[i].size))
	}
	for i := nEntries; i < dirSecs*4; i++ {
		e := dir[i*128 : (i+1)*128]
		le.PutUint32(e[68:], noStream)
		le.PutUint32(e[72:], noStream)
		le.PutUint32(e[76:], noStream)
	}
	buf = append(buf, dir...)
	for i, s := range streams {
		b := make([]byte, pl[i].n*sec)
		copy(b, s.data)
		buf = append(buf, b...)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

// biff 拼 BIFF 记录。
func biff(recs ...[2]any) []byte {
	var b []byte
	for _, r := range recs {
		typ, data := r[0].(int), r[1].([]byte)
		h := make([]byte, 4)
		binary.LittleEndian.PutUint16(h, uint16(typ))
		binary.LittleEndian.PutUint16(h[2:], uint16(len(data)))
		b = append(append(b, h...), data...)
	}
	return b
}
