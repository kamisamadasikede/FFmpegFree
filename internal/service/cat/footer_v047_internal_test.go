package cat

import "testing"

// v0.47 §6.19.19.5 tokensPerSec = outputTokens / genSec。
func TestComputeTokensPerSec_V047(t *testing.T) {
	p := func(v int64) *int64 { return &v }
	cases := []struct {
		name         string
		out          int64
		total, first *int64
		want         float64 // 0 = 省略
	}{
		{"有首字：按 total−first", 2100, p(8400), p(900), 280},
		{"无首字：用 totalMs", 2000, p(4000), nil, 500},
		{"差=0：退回 totalMs", 100, p(1000), p(1000), 100},
		{"差<0：退回 totalMs", 100, p(1000), p(1500), 100},
		{"无 outputTokens：省略", 0, p(1000), p(100), 0},
		{"outputTokens<0：省略", -5, p(1000), p(100), 0},
		{"totalMs=0 且无首字：省略", 100, p(0), nil, 0},
		{"无 totalMs：省略", 100, nil, nil, 0},
	}
	for _, c := range cases {
		got := computeTokensPerSec(c.out, c.total, c.first)
		if c.want == 0 {
			if got != nil {
				t.Errorf("%s: want nil, got %v", c.name, *got)
			}
			continue
		}
		if got == nil || *got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}
