package doc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"unicode/utf16"
)

// 一个最小的 sfnt（TrueType）读取器：只读 fpdf 之外我们需要的两样东西——
// cmap（某个字符字体有没有字形，用于选字体和统计缺字）和 name 表（测试断言字体名不含 Source）。

var errNotTrueType = errors.New("不是 TrueType 轮廓的 .ttf（.otf / .ttc / 损坏）")

type sfnt struct {
	data   []byte
	tables map[string][]byte
	cmap   cmapTable
}

// parseSfnt 解析字体：必须是单一字体（不是 ttcf 集合）、sfnt 版本 0x00010000 或 'true'、有 glyf（TrueType 轮廓）、
// 没有 CFF（.otf）、没有 fvar（可变字体，fpdf 不能正确处理）。
func parseSfnt(data []byte) (*sfnt, error) {
	if len(data) < 12 {
		return nil, errNotTrueType
	}
	switch tag := string(data[:4]); tag {
	case "\x00\x01\x00\x00", "true":
	default:
		return nil, errNotTrueType
	}
	n := int(binary.BigEndian.Uint16(data[4:6]))
	if n == 0 || 12+16*n > len(data) {
		return nil, errNotTrueType
	}
	f := &sfnt{data: data, tables: make(map[string][]byte, n)}
	for i := 0; i < n; i++ {
		rec := data[12+16*i : 12+16*i+16]
		off, ln := int(binary.BigEndian.Uint32(rec[8:12])), int(binary.BigEndian.Uint32(rec[12:16]))
		if off < 0 || ln < 0 || off > len(data) || ln > len(data)-off {
			return nil, errNotTrueType
		}
		f.tables[string(rec[:4])] = data[off : off+ln]
	}
	for _, need := range []string{"glyf", "loca", "head", "cmap"} {
		if _, ok := f.tables[need]; !ok {
			return nil, errNotTrueType
		}
	}
	if _, ok := f.tables["CFF "]; ok {
		return nil, errNotTrueType
	}
	if _, ok := f.tables["fvar"]; ok {
		return nil, fmt.Errorf("可变字体不受支持（fvar）")
	}
	c, err := parseCmap(f.tables["cmap"])
	if err != nil {
		return nil, err
	}
	f.cmap = c
	return f, nil
}

// cmapTable 是一个 Unicode cmap 子表（format 4 或 12）。
type cmapTable struct {
	format int
	b      []byte
}

func parseCmap(t []byte) (cmapTable, error) {
	if len(t) < 4 {
		return cmapTable{}, errors.New("cmap 表损坏")
	}
	n := int(binary.BigEndian.Uint16(t[2:4]))
	var best cmapTable
	bestRank := -1
	for i := 0; i < n && 4+8*i+8 <= len(t); i++ {
		pid := binary.BigEndian.Uint16(t[4+8*i:])
		eid := binary.BigEndian.Uint16(t[6+8*i:])
		off := int(binary.BigEndian.Uint32(t[8+8*i:]))
		if off < 0 || off+4 > len(t) {
			continue
		}
		format := int(binary.BigEndian.Uint16(t[off:]))
		rank := -1
		switch {
		case pid == 3 && eid == 10 && format == 12:
			rank = 3
		case pid == 0 && format == 12:
			rank = 2
		case pid == 3 && eid == 1 && format == 4:
			rank = 1
		case pid == 0 && format == 4:
			rank = 0
		}
		if rank > bestRank {
			bestRank, best = rank, cmapTable{format: format, b: t[off:]}
		}
	}
	if bestRank < 0 {
		return cmapTable{}, errors.New("字体没有 Unicode cmap")
	}
	return best, nil
}

// Has 判断 r 在字体里是否有真正的字形（映射到 .notdef 即 glyph 0 算没有）。
func (c cmapTable) Has(r rune) bool {
	if r < 0 || r > 0x10FFFF {
		return false
	}
	switch c.format {
	case 12:
		return c.has12(uint32(r))
	case 4:
		return r <= 0xFFFF && c.has4(uint16(r))
	}
	return false
}

func (c cmapTable) has12(r uint32) bool {
	b := c.b
	if len(b) < 16 {
		return false
	}
	n := int(binary.BigEndian.Uint32(b[12:16]))
	if n < 0 || 16+12*n > len(b) {
		return false
	}
	i := sort.Search(n, func(i int) bool { return binary.BigEndian.Uint32(b[16+12*i+4:]) >= r })
	if i >= n {
		return false
	}
	g := b[16+12*i:]
	start, sg := binary.BigEndian.Uint32(g[0:]), binary.BigEndian.Uint32(g[8:])
	return r >= start && sg+(r-start) != 0
}

func (c cmapTable) has4(r uint16) bool {
	b := c.b
	if len(b) < 14 {
		return false
	}
	segX2 := int(binary.BigEndian.Uint16(b[6:8]))
	seg := segX2 / 2
	endO, startO := 14, 14+segX2+2
	deltaO, rangeO := startO+segX2, startO+2*segX2
	if rangeO+segX2 > len(b) {
		return false
	}
	i := sort.Search(seg, func(i int) bool { return binary.BigEndian.Uint16(b[endO+2*i:]) >= r })
	if i >= seg {
		return false
	}
	start := binary.BigEndian.Uint16(b[startO+2*i:])
	if r < start {
		return false
	}
	delta := binary.BigEndian.Uint16(b[deltaO+2*i:])
	ro := binary.BigEndian.Uint16(b[rangeO+2*i:])
	if ro == 0 {
		return r+delta != 0 // uint16 回绕
	}
	idx := rangeO + 2*i + int(ro) + 2*int(r-start)
	if idx+2 > len(b) {
		return false
	}
	g := binary.BigEndian.Uint16(b[idx:])
	return g != 0 && g+delta != 0
}

// nameRecord 是 name 表的一条记录。
type nameRecord struct {
	PlatformID, EncodingID, LanguageID, NameID uint16
	Value                                      string
}

// names 解析 name 表（Windows 平台记录按 UTF-16BE，Mac 记录按单字节近似）。
func (f *sfnt) names() []nameRecord {
	t := f.tables["name"]
	if len(t) < 6 {
		return nil
	}
	count := int(binary.BigEndian.Uint16(t[2:4]))
	strOff := int(binary.BigEndian.Uint16(t[4:6]))
	var out []nameRecord
	for i := 0; i < count && 6+12*i+12 <= len(t); i++ {
		r := t[6+12*i:]
		rec := nameRecord{
			PlatformID: binary.BigEndian.Uint16(r[0:]), EncodingID: binary.BigEndian.Uint16(r[2:]),
			LanguageID: binary.BigEndian.Uint16(r[4:]), NameID: binary.BigEndian.Uint16(r[6:]),
		}
		ln, off := int(binary.BigEndian.Uint16(r[8:])), int(binary.BigEndian.Uint16(r[10:]))
		if strOff+off+ln > len(t) {
			continue
		}
		raw := t[strOff+off : strOff+off+ln]
		if rec.PlatformID == 3 || rec.PlatformID == 0 {
			u := make([]uint16, len(raw)/2)
			for j := range u {
				u[j] = binary.BigEndian.Uint16(raw[2*j:])
			}
			rec.Value = string(utf16.Decode(u))
		} else {
			rec.Value = string(raw)
		}
		out = append(out, rec)
	}
	return out
}
