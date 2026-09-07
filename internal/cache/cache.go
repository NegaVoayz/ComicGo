// Package cache provides the on-disk cover thumbnail store and the small
// in-memory LRUs that bound memory during scanning and reading.
package cache

import (
	"container/list"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// ThumbStore is the on-disk JPEG thumbnail directory (thumbs/<id>.jpg).
type ThumbStore struct {
	dir string
}

// NewThumbStore creates the store rooted at dir.
func NewThumbStore(dir string) *ThumbStore { return &ThumbStore{dir: dir} }

// Path returns the file path for a comic's thumbnail.
func (t *ThumbStore) Path(id int64) string {
	return filepath.Join(t.dir, strconv.FormatInt(id, 10)+".jpg")
}

// Has reports whether the thumbnail exists.
func (t *ThumbStore) Has(id int64) bool {
	fi, err := os.Stat(t.Path(id))
	return err == nil && !fi.IsDir()
}

// Read returns the thumbnail bytes, or an error if absent.
func (t *ThumbStore) Read(id int64) ([]byte, error) {
	return os.ReadFile(t.Path(id))
}

// Write stores the thumbnail bytes, creating the directory if needed.
func (t *ThumbStore) Write(id int64, b []byte) error {
	if err := os.MkdirAll(t.dir, 0o755); err != nil {
		return err
	}
	tmp := t.Path(id) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, t.Path(id))
}

// Remove deletes a comic's thumbnail if present.
func (t *ThumbStore) Remove(id int64) {
	_ = os.Remove(t.Path(id))
}

// LRU is a size-bounded byte cache used for decoded page payloads.
type LRU struct {
	mu       sync.Mutex
	maxBytes int64
	used     int64
	ll       *list.List
	items    map[string]*list.Element
}

type lruEntry struct {
	key   string
	value []byte
}

// NewLRU returns an LRU capped at maxBytes.
func NewLRU(maxBytes int64) *LRU {
	return &LRU{maxBytes: maxBytes, ll: list.New(), items: map[string]*list.Element{}}
}

// Get returns a cached value.
func (c *LRU) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*lruEntry).value, true
}

// Put stores a value, evicting least-recently-used entries over budget. Values
// larger than the whole budget are dropped rather than cached.
func (c *LRU) Put(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if int64(len(value)) > c.maxBytes {
		return
	}
	if el, ok := c.items[key]; ok {
		ent := el.Value.(*lruEntry)
		c.used += int64(len(value)) - int64(len(ent.value))
		ent.value = value
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&lruEntry{key: key, value: value})
	c.items[key] = el
	c.used += int64(len(value))
	for c.used > c.maxBytes && c.ll.Len() > 0 {
		back := c.ll.Back()
		ent := back.Value.(*lruEntry)
		c.ll.Remove(back)
		delete(c.items, ent.key)
		c.used -= int64(len(ent.value))
	}
}

// Len returns the number of cached entries (for tests/metrics).
func (c *LRU) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
