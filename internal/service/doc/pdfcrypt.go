package doc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"io"
	"regexp"
	"strconv"

	"FFmpegFree/internal/apperr"
)

// ---------- PDF 加密分类：只有所有者密码 vs 要密码才能打开（v0.28 补充） ----------

// reasonOwnerOnly：PDF 只设了权限（所有者）密码，不用密码能打开，但纯 Go 库解不了这种加密（如 AES-256）。
const reasonOwnerOnly = "owner_only"

func errPDFOwnerOnly() *apperr.AppError {
	return apperr.New(apperr.DocEncrypted, "这个 PDF 设置了权限保护，暂时不能转换。").WithDetail("reason=" + reasonOwnerOnly)
}

type pdfEncState int

const (
	pdfEncUnknown   pdfEncState = iota // 有 /Encrypt 但读不懂 → 按要密码处理
	pdfEncNone                         // 没有 /Encrypt
	pdfEncOwnerOnly                    // 空用户密码能通过校验
	pdfEncUser                         // 要用户密码
)

// pdfEncryptErr 把“库打不开、但有加密”的 PDF 分成 owner_only 和要密码两种。
func pdfEncryptErr(f io.ReaderAt, size int64) *apperr.AppError {
	if classifyPDFEncryption(f, size) == pdfEncOwnerOnly {
		return errPDFOwnerOnly()
	}
	return errEncrypted()
}

var (
	pdfEncryptRefRe = regexp.MustCompile(`/Encrypt\s*(?:(\d+)\s+(\d+)\s+R|(<<))`)
	pdfIDRe         = regexp.MustCompile(`/ID\s*\[`)
)

// classifyPDFEncryption 自己读 trailer 的 /Encrypt 字典（Standard 处理程序），用空用户密码校验 U。
func classifyPDFEncryption(f io.ReaderAt, size int64) pdfEncState {
	const tail, head = 256 << 10, 64 << 10
	var region []byte
	var loc []int
	for _, w := range [][2]int64{{size - tail, tail}, {0, head}} {
		b := readAtClamp(f, size, w[0], w[1])
		all := pdfEncryptRefRe.FindAllSubmatchIndex(b, -1)
		if len(all) > 0 {
			region, loc = b, all[len(all)-1] // 增量更新：取最后一个
			break
		}
	}
	if loc == nil {
		return pdfEncNone
	}
	var enc map[string]any
	if loc[6] >= 0 { // 内联字典
		p := &pdfLex{b: region, i: loc[6]}
		d, _ := p.value().(map[string]any)
		enc = d
	} else {
		enc = findPDFObjDict(f, size, string(region[loc[2]:loc[3]]), string(region[loc[4]:loc[5]]))
	}
	if enc == nil {
		return pdfEncUnknown
	}
	// /ID 在同一个 trailer 里，取离 /Encrypt 最近的那个
	var id0 []byte
	if ids := pdfIDRe.FindAllIndex(region, -1); len(ids) > 0 {
		best := ids[0]
		for _, m := range ids {
			if abs(m[0]-loc[0]) < abs(best[0]-loc[0]) {
				best = m
			}
		}
		p := &pdfLex{b: region, i: best[1] - 1}
		if arr, ok := p.value().([]any); ok && len(arr) > 0 {
			id0, _ = arr[0].([]byte)
		}
	}
	if n, _ := enc["Filter"].(pdfName); n != "Standard" {
		return pdfEncUnknown
	}
	ok, known := pdfEmptyUserPassword(enc, id0)
	switch {
	case !known:
		return pdfEncUnknown
	case ok:
		return pdfEncOwnerOnly
	}
	return pdfEncUser
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func readAtClamp(f io.ReaderAt, size, off, n int64) []byte {
	if off < 0 {
		n += off
		off = 0
	}
	if off+n > size {
		n = size - off
	}
	if n <= 0 {
		return nil
	}
	b := make([]byte, n)
	k, _ := f.ReadAt(b, off)
	return b[:k]
}

// findPDFObjDict 按块扫全文件找最后一个 “num gen obj”，解析后面的字典（加密字典不能放在对象流里）。
func findPDFObjDict(f io.ReaderAt, size int64, num, gen string) map[string]any {
	re := regexp.MustCompile(`(?:^|[^0-9])` + num + `\s+` + gen + `\s+obj\b`)
	const chunk, overlap = 4 << 20, 64
	var last map[string]any
	for off := int64(0); off < size; off += chunk {
		b := readAtClamp(f, size, off, chunk+overlap+64<<10) // 多读一段，字典跨块也能解析
		lim := chunk + overlap
		if lim > len(b) {
			lim = len(b)
		}
		for _, m := range re.FindAllIndex(b[:lim], -1) {
			if off > 0 && m[0] < overlap {
				continue // 上一块已经看过
			}
			p := &pdfLex{b: b, i: m[1]}
			if d, ok := p.value().(map[string]any); ok {
				last = d
			}
		}
	}
	return last
}

// ---------- 极简 PDF 对象读取（只为加密字典） ----------

type pdfName string

type pdfRef struct{}

type pdfLex struct {
	b     []byte
	i     int
	depth int
}

func isPDFWS(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' || c == 0
}

func isPDFDelim(c byte) bool {
	return isPDFWS(c) || bytes.IndexByte([]byte("()<>[]{}/%"), c) >= 0
}

func (p *pdfLex) skip() {
	for p.i < len(p.b) {
		c := p.b[p.i]
		if isPDFWS(c) {
			p.i++
		} else if c == '%' {
			for p.i < len(p.b) && p.b[p.i] != '\n' && p.b[p.i] != '\r' {
				p.i++
			}
		} else {
			return
		}
	}
}

func (p *pdfLex) word() string {
	s := p.i
	for p.i < len(p.b) && !isPDFDelim(p.b[p.i]) {
		p.i++
	}
	return string(p.b[s:p.i])
}

// value 读一个对象：字典 → map，数组 → []any，字符串 → []byte，数字 → int64/float64，名字 → pdfName；
// 引用 “n g R” → pdfRef{}。出错返回 nil。
func (p *pdfLex) value() any {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 32 {
		return nil
	}
	p.skip()
	if p.i >= len(p.b) {
		return nil
	}
	switch c := p.b[p.i]; {
	case c == '<' && p.i+1 < len(p.b) && p.b[p.i+1] == '<':
		p.i += 2
		d := map[string]any{}
		for {
			p.skip()
			if p.i+1 < len(p.b) && p.b[p.i] == '>' && p.b[p.i+1] == '>' {
				p.i += 2
				return d
			}
			k, ok := p.value().(pdfName)
			if !ok {
				return nil
			}
			d[string(k)] = p.value()
		}
	case c == '<':
		p.i++
		e := bytes.IndexByte(p.b[p.i:], '>')
		if e < 0 {
			return nil
		}
		h := bytes.Map(func(r rune) rune {
			if r < 128 && isPDFWS(byte(r)) {
				return -1
			}
			return r
		}, p.b[p.i:p.i+e])
		p.i += e + 1
		if len(h)%2 == 1 {
			h = append(h, '0')
		}
		out := make([]byte, len(h)/2)
		if _, err := hex.Decode(out, h); err != nil {
			return nil
		}
		return out
	case c == '(':
		return p.literal()
	case c == '[':
		p.i++
		var a []any
		for {
			p.skip()
			if p.i >= len(p.b) {
				return nil
			}
			if p.b[p.i] == ']' {
				p.i++
				return a
			}
			v := p.value()
			if v == nil {
				return nil
			}
			a = append(a, v)
		}
	case c == '/':
		p.i++
		return pdfName(p.word())
	default:
		w := p.word()
		if w == "" {
			return nil
		}
		if n, err := strconv.ParseInt(w, 10, 64); err == nil {
			// 引用 “n g R”
			save := p.i
			p.skip()
			g := p.word()
			p.skip()
			if _, err := strconv.Atoi(g); err == nil && p.i < len(p.b) && p.b[p.i] == 'R' && (p.i+1 >= len(p.b) || isPDFDelim(p.b[p.i+1])) {
				p.i++
				return pdfRef{}
			}
			p.i = save
			return n
		}
		if x, err := strconv.ParseFloat(w, 64); err == nil {
			return x
		}
		switch w {
		case "true":
			return true
		case "false":
			return false
		}
		return pdfName("") // null 等关键字
	}
}

func (p *pdfLex) literal() any {
	p.i++ // (
	var out []byte
	nest := 0
	for p.i < len(p.b) {
		c := p.b[p.i]
		p.i++
		switch c {
		case '(':
			nest++
			out = append(out, c)
		case ')':
			if nest == 0 {
				return out
			}
			nest--
			out = append(out, c)
		case '\\':
			if p.i >= len(p.b) {
				return nil
			}
			e := p.b[p.i]
			p.i++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r':
				if p.i < len(p.b) && p.b[p.i] == '\n' {
					p.i++
				}
			case '\n':
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for k := 0; k < 2 && p.i < len(p.b) && p.b[p.i] >= '0' && p.b[p.i] <= '7'; k++ {
						v = v*8 + int(p.b[p.i]-'0')
						p.i++
					}
					out = append(out, byte(v))
				} else {
					out = append(out, e)
				}
			}
		default:
			out = append(out, c)
		}
	}
	return nil
}

// ---------- 空用户密码校验（PDF 32000-1 §7.6.3，ISO 32000-2 §7.6.4.3.4） ----------

var pdfPasswordPad = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80, 0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

// pdfEmptyUserPassword 返回空用户密码能否通过；known=false 表示参数不全 / 不认识的版本。
func pdfEmptyUserPassword(enc map[string]any, id0 []byte) (ok, known bool) {
	R, _ := enc["R"].(int64)
	O, _ := enc["O"].([]byte)
	U, _ := enc["U"].([]byte)
	switch R {
	case 2, 3, 4:
		if len(O) < 32 || len(U) < 32 {
			return false, false
		}
		P, _ := enc["P"].(int64)
		n := 5
		if R >= 3 {
			l, _ := enc["Length"].(int64)
			if l == 0 {
				l = 40
			}
			if l%8 != 0 || l < 40 || l > 128 {
				return false, false
			}
			n = int(l / 8)
		}
		// 算法 2：空密码的文件密钥
		h := md5.New()
		h.Write(pdfPasswordPad)
		h.Write(O[:32])
		var pb [4]byte
		binary.LittleEndian.PutUint32(pb[:], uint32(P))
		h.Write(pb[:])
		h.Write(id0)
		if em, isB := enc["EncryptMetadata"].(bool); R >= 4 && isB && !em {
			h.Write([]byte{0xff, 0xff, 0xff, 0xff})
		}
		key := h.Sum(nil)
		if R >= 3 {
			for i := 0; i < 50; i++ {
				s := md5.Sum(key[:n])
				key = s[:]
			}
		}
		key = key[:n]
		if R == 2 { // 算法 4
			c, _ := rc4.NewCipher(key)
			out := make([]byte, 32)
			c.XORKeyStream(out, pdfPasswordPad)
			return bytes.Equal(out, U[:32]), true
		}
		// 算法 5
		h = md5.New()
		h.Write(pdfPasswordPad)
		h.Write(id0)
		out := h.Sum(nil)
		k2 := make([]byte, n)
		for i := 0; i < 20; i++ {
			for j := range key {
				k2[j] = key[j] ^ byte(i)
			}
			c, _ := rc4.NewCipher(k2)
			c.XORKeyStream(out, out)
		}
		return bytes.Equal(out[:16], U[:16]), true
	case 5:
		if len(U) < 40 {
			return false, false
		}
		s := sha256.Sum256(U[32:40]) // 空密码 + 验证盐
		return bytes.Equal(s[:], U[:32]), true
	case 6:
		if len(U) < 40 {
			return false, false
		}
		return bytes.Equal(pdfHash2B(nil, U[32:40], nil), U[:32]), true
	}
	return false, false
}

// pdfHash2B 是 ISO 32000-2 的算法 2.B。
func pdfHash2B(pw, salt, udata []byte) []byte {
	h := sha256.New()
	h.Write(pw)
	h.Write(salt)
	h.Write(udata)
	k := h.Sum(nil)
	for round := 0; ; round++ {
		one := make([]byte, 0, len(pw)+len(k)+len(udata))
		one = append(append(append(one, pw...), k...), udata...)
		k1 := bytes.Repeat(one, 64)
		blk, _ := aes.NewCipher(k[:16])
		e := make([]byte, len(k1))
		cipher.NewCBCEncrypter(blk, k[16:32]).CryptBlocks(e, k1)
		sum := 0
		for _, c := range e[:16] {
			sum += int(c)
		}
		var hh hash.Hash
		switch sum % 3 {
		case 0:
			hh = sha256.New()
		case 1:
			hh = sha512.New384()
		default:
			hh = sha512.New()
		}
		hh.Write(e)
		k = hh.Sum(nil)
		if round >= 63 && int(e[len(e)-1]) <= round+1-32 {
			break
		}
	}
	return k[:32]
}
