package doc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
)

// previewCache 是引擎生成的 PDF 缓存（契约 6.12.32.3）：键 = sha256(path_key + size + mtime_ns + engine)，上限 1 GiB LRU。
type previewCache struct {
	dir string
	mu  sync.Mutex
	idx map[string]*cacheEntry // key hex → entry
}

type cacheEntry struct {
	Key      string `json:"key"`
	Path     string `json:"path"` // 绝对路径 .pdf
	Size     int64  `json:"size"`
	LastOpen int64  `json:"lastOpen"` // unix ms
	pinned   bool   // 正在被预览，不删
}

const maxCacheBytes int64 = 1 << 30 // 1 GiB

func newPreviewCache(dataRoot string) *previewCache {
	dir := previewCacheDir(dataRoot)
	_ = os.MkdirAll(dir, 0o755)
	c := &previewCache{dir: dir, idx: map[string]*cacheEntry{}}
	c.load()
	c.cleanupOrphans()
	c.evict()
	return c
}

func previewCacheDir(dataRoot string) string {
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "FFmpegFree", "cache", "preview")
		}
	}
	return filepath.Join(dataRoot, "cache", "preview")
}

func cacheKey(pathKey string, size, mtimeNs int64, engine string) string {
	h := sha256.Sum256([]byte(pathKey + "\n" + i64toa(size) + "\n" + i64toa(mtimeNs) + "\n" + engine))
	return hex.EncodeToString(h[:])
}

func i64toa(n int64) string {
	if n == 0 {
		return "0"
	}
	var neg bool
	if n < 0 {
		neg, n = true, -n
	}
	var b [32]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func (c *previewCache) indexPath() string { return filepath.Join(c.dir, "index.json") }

func (c *previewCache) load() {
	b, err := os.ReadFile(c.indexPath())
	if err != nil {
		return
	}
	var list []cacheEntry
	if json.Unmarshal(b, &list) != nil {
		return
	}
	for i := range list {
		e := list[i]
		if e.Key == "" || e.Path == "" {
			continue
		}
		cp := e
		c.idx[e.Key] = &cp
	}
}

func (c *previewCache) save() {
	list := make([]cacheEntry, 0, len(c.idx))
	for _, e := range c.idx {
		list = append(list, cacheEntry{Key: e.Key, Path: e.Path, Size: e.Size, LastOpen: e.LastOpen})
	}
	b, _ := json.Marshal(list)
	_ = os.WriteFile(c.indexPath(), b, 0o644)
}

func (c *previewCache) cleanupOrphans() {
	ents, _ := os.ReadDir(c.dir)
	known := map[string]bool{}
	for _, e := range c.idx {
		known[filepath.Base(e.Path)] = true
	}
	for _, e := range ents {
		name := e.Name()
		if name == "index.json" {
			continue
		}
		if stringsHasSuffix(name, ".part") {
			_ = os.Remove(filepath.Join(c.dir, name))
			continue
		}
		if !known[name] {
			_ = os.Remove(filepath.Join(c.dir, name))
		}
	}
	for k, e := range c.idx {
		if _, err := os.Stat(e.Path); err != nil {
			delete(c.idx, k)
		}
	}
	c.save()
}

func stringsHasSuffix(s, suf string) bool {
	return len(s) >= len(suf) && s[len(s)-len(suf):] == suf
}

// Lookup 按引擎顺序找缓存命中。
func (c *previewCache) Lookup(pathKey string, size, mtimeNs int64, engineIDs []string) (pdfPath, engine string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, eng := range engineIDs {
		k := cacheKey(pathKey, size, mtimeNs, eng)
		if e, hit := c.idx[k]; hit {
			if _, err := os.Stat(e.Path); err != nil {
				delete(c.idx, k)
				continue
			}
			e.LastOpen = time.Now().UnixMilli()
			c.save()
			return e.Path, eng, true
		}
	}
	return "", "", false
}

func (c *previewCache) PartPath(key string) string  { return filepath.Join(c.dir, key+".pdf.part") }
func (c *previewCache) FinalPath(key string) string { return filepath.Join(c.dir, key+".pdf") }

func (c *previewCache) Commit(key, engine string, size int64) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	part := c.PartPath(key)
	final := c.FinalPath(key)
	if err := os.Rename(part, final); err != nil {
		return "", err
	}
	c.idx[key] = &cacheEntry{Key: key, Path: final, Size: size, LastOpen: time.Now().UnixMilli()}
	c.evictLocked()
	c.save()
	return final, nil
}

func (c *previewCache) Pin(path string, pin bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.idx {
		if e.Path == path {
			e.pinned = pin
		}
	}
}

func (c *previewCache) evict() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictLocked()
	c.save()
}

func (c *previewCache) evictLocked() {
	var total int64
	list := make([]*cacheEntry, 0, len(c.idx))
	for _, e := range c.idx {
		total += e.Size
		list = append(list, e)
	}
	if total <= maxCacheBytes {
		return
	}
	sort.Slice(list, func(i, j int) bool { return list[i].LastOpen < list[j].LastOpen })
	for _, e := range list {
		if total <= maxCacheBytes {
			break
		}
		if e.pinned {
			continue
		}
		_ = os.Remove(e.Path)
		delete(c.idx, e.Key)
		total -= e.Size
	}
}

func (c *previewCache) RemoveByPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.idx {
		if e.Path == path || filepath.Base(e.Path) == filepath.Base(path) {
			_ = os.Remove(e.Path)
			delete(c.idx, k)
		}
	}
	c.save()
}
