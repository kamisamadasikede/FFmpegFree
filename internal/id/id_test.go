package id

import "testing"

func TestNewIsMonotonic(t *testing.T) {
	prev := New()
	for i := 0; i < 1000; i++ {
		cur := New()
		if len(cur) != 26 {
			t.Fatalf("ULID 长度应为 26: %s", cur)
		}
		if cur <= prev {
			t.Fatalf("ID 应单调递增: %s <= %s", cur, prev)
		}
		prev = cur
	}
}
