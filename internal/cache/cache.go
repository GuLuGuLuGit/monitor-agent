package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Kind 缓存类型
type Kind string

const (
	KindMetrics Kind = "metrics"
	KindLogs    Kind = "logs"
)

// Entry 单条缓存（通用 JSON）
type Entry struct {
	Kind string          `json:"kind"`
	Time time.Time       `json:"time"`
	Body json.RawMessage `json:"body"`
}

// Cache 本地文件缓存，用于离线时暂存
type Cache struct {
	dir       string
	maxBytes  int64
	mu        sync.Mutex
	written   int64
	indexFile string
}

// New 创建缓存目录
func New(dir string, maxSizeMB int) (*Cache, error) {
	if maxSizeMB <= 0 {
		maxSizeMB = 50
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	indexFile := filepath.Join(dir, "index.json")
	c := &Cache{
		dir:       dir,
		maxBytes:  int64(maxSizeMB) * 1024 * 1024,
		indexFile: indexFile,
	}
	_ = c.loadIndex()
	return c, nil
}

func (c *Cache) loadIndex() error {
	data, err := os.ReadFile(c.indexFile)
	if err != nil {
		return nil
	}
	var idx struct {
		Written int64 `json:"written"`
	}
	_ = json.Unmarshal(data, &idx)
	c.written = idx.Written
	return nil
}

func (c *Cache) saveIndex() error {
	data, _ := json.Marshal(map[string]int64{"written": c.written})
	return os.WriteFile(c.indexFile, data, 0600)
}

// Push 写入一条缓存；超过 maxBytes 时丢弃最旧文件
func (c *Cache) Push(kind Kind, body interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	entry := Entry{
		Kind: string(kind),
		Time: time.Now().UTC(),
		Body: raw,
	}
	entryData, _ := json.Marshal(entry)
	size := int64(len(entryData))

	for c.written+size > c.maxBytes {
		if !c.dropOldest() {
			return fmt.Errorf("cache full and no file to drop")
		}
	}

	name := fmt.Sprintf("%s_%d.json", kind, time.Now().UnixNano())
	fpath := filepath.Join(c.dir, name)
	if err := os.WriteFile(fpath, entryData, 0600); err != nil {
		return err
	}
	c.written += size
	return c.saveIndex()
}

// PopMetrics 弹出若干条 metrics 缓存（供上报后删除）
func (c *Cache) PopMetrics(max int) ([][]byte, error) {
	return c.popKind(KindMetrics, max)
}

// PopLogs 弹出若干条 logs 缓存
func (c *Cache) PopLogs(max int) ([][]byte, error) {
	return c.popKind(KindLogs, max)
}

func (c *Cache) popKind(kind Kind, max int) ([][]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, err
	}
	var out [][]byte
	for _, e := range entries {
		if e.IsDir() || e.Name() == "index.json" {
			continue
		}
		if len(out) >= max {
			break
		}
		fname := e.Name()
		prefix := string(kind) + "_"
		if len(fname) < len(prefix)+1 || fname[:len(prefix)] != prefix {
			continue
		}
		fpath := filepath.Join(c.dir, fname)
		data, err := os.ReadFile(fpath)
		if err != nil {
			continue
		}
		var entry Entry
		if json.Unmarshal(data, &entry) != nil || entry.Kind != string(kind) {
			continue
		}
		out = append(out, entry.Body)
		_ = os.Remove(fpath)
		c.written -= int64(len(data))
	}
	_ = c.saveIndex()
	return out, nil
}

func (c *Cache) dropOldest() bool {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return false
	}
	var oldest string
	var oldestMod time.Time
	for _, e := range entries {
		if e.IsDir() || e.Name() == "index.json" {
			continue
		}
		fpath := filepath.Join(c.dir, e.Name())
		info, err := os.Stat(fpath)
		if err != nil {
			continue
		}
		if oldest == "" || info.ModTime().Before(oldestMod) {
			oldest = fpath
			oldestMod = info.ModTime()
		}
	}
	if oldest == "" {
		return false
	}
	info, _ := os.Stat(oldest)
	_ = os.Remove(oldest)
	c.written -= info.Size()
	return true
}
