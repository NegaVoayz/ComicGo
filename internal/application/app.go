package application

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"comicgo/internal/cache"
	"comicgo/internal/config"
	"comicgo/internal/index"
	"comicgo/internal/model"
	"comicgo/internal/watch"
)

// Version is the human-facing app version.
const Version = "0.1.0"

// thumbGen marks the current cover thumbnail format. Bump it (and let
// ensureThumbGeneration clear the cache) whenever thumb dimensions change so
// stale-size covers are regenerated on demand.
const thumbGen = "2"

// ensureThumbGeneration drops all cached thumbnails once when the format
// generation changes, forcing lazy regeneration at the new size.
func ensureThumbGeneration(dir string) {
	marker := filepath.Join(dir, ".gen")
	if b, err := os.ReadFile(marker); err == nil && string(b) == thumbGen {
		return
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".jpg") {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	_ = os.WriteFile(marker, []byte(thumbGen), 0o644)
}

// AppEventSink receives backend state changes that the GUI should render.
// Implementations must not block; background import/scan workers call these
// methods directly.
type AppEventSink interface {
	ScanStarted()
	ScanProgress(model.ScanProgress)
	ScanDone(error)
	ScanError(path string, err error)
	ImportProgress(model.ImportProgress)
	LayoutChanged()
}

// App owns the library, database, watcher, scans, and imports.
type App struct {
	mu       sync.Mutex
	importMu sync.Mutex // serializes comic imports / metadata edits (one file rewrite at a time)
	events   AppEventSink
	paths    *config.Paths
	store    *index.Store
	thumbs   *cache.ThumbStore

	settings model.Settings
	layout   model.LayoutDoc

	metas      []model.Meta
	metasDirty bool

	scanCancel context.CancelFunc
	scanDoneCh chan struct{}
	watcher    *watch.Watcher

	pageCache *cache.LRU

	// coverSem bounds concurrent cover decodes so transient memory stays bounded
	// while a fresh library generates thumbnails on demand. Its width is every
	// CPU Go can use (see coverWorkers) so cold cover fills saturate the machine.
	coverSem chan struct{}
	coverGen atomic.Uint32
}

// SetEventSink installs the GUI listener. It must be called before Start.
func (a *App) SetEventSink(sink AppEventSink) {
	a.events = sink
}

func (a *App) scanStarted() {
	if a.events != nil {
		a.events.ScanStarted()
	}
}

func (a *App) scanProgress(p model.ScanProgress) {
	if a.events != nil {
		a.events.ScanProgress(p)
	}
}

func (a *App) emitScanDone(err error) {
	if a.events != nil {
		a.events.ScanDone(err)
	}
}

func (a *App) scanError(path string, err error) {
	if a.events != nil {
		a.events.ScanError(path, err)
	}
}

func (a *App) importProgress(p model.ImportProgress) {
	if a.events != nil {
		a.events.ImportProgress(p)
	}
}

func (a *App) layoutChanged() {
	if a.events != nil {
		a.events.LayoutChanged()
	}
}

// coverWorkers returns how many cover thumbnails may be decoded in parallel:
// every CPU Go can use, so a cold library fill saturates the machine. Each
// in-flight decode can transiently hold a full-size page in memory (up to
// imgutil.MaxPixels, ~400 MB worst case), so on a memory-constrained machine
// set COMICGO_COVER_WORKERS=N to cap the width without recompiling.
func coverWorkers() int {
	if s := os.Getenv("COMICGO_COVER_WORKERS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return goruntime.GOMAXPROCS(0)
}

func NewApp() *App {
	return &App{metasDirty: true, pageCache: cache.NewLRU(8 << 20), coverSem: make(chan struct{}, coverWorkers())}
}

// dbg appends one line to <appdir>/tmp/debug.log when COMICGO_DEBUG=1.
func (a *App) dbg(format string, args ...any) {
	if os.Getenv("COMICGO_DEBUG") == "" || a == nil || a.paths == nil {
		return
	}
	line := time.Now().Format("15:04:05.000") + " " + fmt.Sprintf(format, args...) + "\n"
	if f, err := os.OpenFile(filepath.Join(a.paths.Temp, "debug.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		_, _ = f.WriteString(line)
		_ = f.Close()
	}
}

// Start initializes user configuration and starts watching configured roots.
func (a *App) Start() error {
	paths, err := config.Resolve()
	if err != nil {
		return fmt.Errorf("resolve config: %w", err)
	}
	a.paths = paths

	s, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	a.settings = s

	st, err := index.Open(paths.DB)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	a.store = st
	a.thumbs = cache.NewThumbStore(paths.Thumbs)
	ensureThumbGeneration(paths.Thumbs)

	doc, err := config.LoadLayout()
	if err != nil {
		return fmt.Errorf("load layout: %w", err)
	}
	a.layout = doc
	a.syncWatcher()
	return nil
}

// syncWatcher restarts the folder watcher for the current library roots.
func (a *App) syncWatcher() {
	if a.watcher != nil {
		a.watcher.Stop()
		a.watcher = nil
	}
	roots := append([]string(nil), a.settings.LibraryRoots...)
	if len(roots) == 0 {
		return
	}
	w, err := watch.New(roots, func() { a.startScan() })
	if err != nil {
		a.dbg("watcher start error: %v", err)
		return
	}
	a.watcher = w
	a.dbg("watcher started on %d root(s)", len(roots))
}

// Close releases resources when the app exits.
func (a *App) Close() {
	if a.watcher != nil {
		a.watcher.Stop()
		a.watcher = nil
	}
	if a.store != nil {
		if err := a.store.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
		a.store = nil
	}
}

// ---- library / lifecycle ----

// GetSettings returns the current settings.
func (a *App) GetSettings() model.Settings { return a.settings }
