// Package watch watches library roots and fires a debounced callback when
// comic files are added, removed, renamed or changed, so the app can rescan.
// Directories are watched recursively (re-synced after each scan for new
// subfolders).
package watch

import (
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounceDelay = 1500 * time.Millisecond

// Watcher reports filesystem changes under a set of root directories.
type Watcher struct {
	fw       *fsnotify.Watcher
	roots    []string
	onChange func()

	mu     sync.Mutex
	dirs   map[string]bool
	timer  *time.Timer
	closed bool
	done   chan struct{}
}

// New creates a watcher for roots. onChange is debounced and called on the
// watcher's own goroutine.
func New(roots []string, onChange func()) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		fw:       fw,
		roots:    roots,
		onChange: onChange,
		dirs:     map[string]bool{},
		done:     make(chan struct{}),
	}
	w.syncDirs()
	go w.loop()
	return w, nil
}

// Sync adds watches for directories that appeared since the last scan (new
// subfolders inside a root).
func (w *Watcher) Sync() {
	w.syncDirs()
}

func (w *Watcher) syncDirs() {
	for _, root := range w.roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			w.addDir(p)
			return nil
		})
	}
}

func (w *Watcher) addDir(p string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.dirs[p] {
		return
	}
	if err := w.fw.Add(p); err == nil {
		w.dirs[p] = true
	}
}

func (w *Watcher) loop() {
	for {
		select {
		case ev, ok := <-w.fw.Events:
			if !ok {
				return
			}
			w.onEvent(ev)
		case _, ok := <-w.fw.Errors:
			if !ok {
				return
			}
		case <-w.done:
			return
		}
	}
}

func (w *Watcher) onEvent(ev fsnotify.Event) {
	if ev.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename|fsnotify.Write) == 0 {
		return
	}
	// ignore churn on our own temp/thumb files (they live outside roots anyway)
	if strings.HasSuffix(ev.Name, ".tmp") {
		return
	}
	ext := strings.ToLower(filepath.Ext(ev.Name))
	interesting := ext == ".zip" || ext == ".cbz" || ev.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0
	if !interesting {
		return
	}
	w.debounce()
}

func (w *Watcher) debounce() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(debounceDelay, func() {
		w.mu.Lock()
		w.timer = nil
		w.mu.Unlock()
		if w.onChange != nil {
			w.onChange()
		}
	})
}

// Stop releases all watches and stops the loop.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	if w.timer != nil {
		w.timer.Stop()
	}
	close(w.done)
	w.mu.Unlock()
	_ = w.fw.Close()
}
