// Package id 生成契约约定的 ULID 字符串 ID：按时间有序，便于按创建顺序排序和分页。
package id

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

var (
	mu      sync.Mutex
	entropy = ulid.Monotonic(rand.Reader, 0)
)

// New 返回一个新的 ULID，同一毫秒内也保证单调递增。
func New() string {
	mu.Lock()
	defer mu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
