package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"comicgo/internal/cache"
	"comicgo/internal/comic"
	"comicgo/internal/config"
	"comicgo/internal/imgutil"
	"comicgo/internal/index"
	"comicgo/internal/model"
	"comicgo/internal/plugins"
	"comicgo/internal/scan"
	"comicgo/internal/serve"
	"comicgo/internal/watch"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Version is the human-facing app version.
const Version = "0.2.0"

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

// App is the single object bound to the frontend.
type App struct {
	ctx    context.Context
	mu     sync.Mutex
	paths  *config.Paths
	store  *index.Store
	thumbs *cache.ThumbStore
	media  *serve.Server

	settings model.Settings
	layout   model.LayoutDoc

	metas      []model.Meta
	metasDirty bool

	scanCancel context.CancelFunc
	scanDone   chan struct{}
	watcher    *watch.Watcher

	pageCache *cache.LRU

	// coverSem bounds concurrent cover decodes so transient memory stays low
	// while a fresh library generates thumbnails on demand.
	coverSem chan struct{}
	coverGen atomic.Uint32
}

// mediaProvider adapts App to serve.Provider without exposing provider methods
// as Wails bindings.
type mediaProvider struct{ a *App }

func (m mediaProvider) Cover(id int64) ([]byte, string, error) {
	return m.a.coverBytes(id)
}
func (m mediaProvider) Page(id int64, n int) ([]byte, string, error) {
	return m.a.pageBytes(id, n)
}

func NewApp() *App {
	return &App{metasDirty: true, pageCache: cache.NewLRU(8 << 20), coverSem: make(chan struct{}, 2)}
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

// startup is called once when the app boots.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	paths, err := config.Resolve()
	if err != nil {
		runtime.LogErrorf(ctx, "resolve config: %v", err)
		return
	}
	a.paths = paths

	s, err := config.LoadSettings()
	if err != nil {
		runtime.LogErrorf(ctx, "load settings: %v", err)
	}
	a.settings = s

	st, err := index.Open(paths.DB)
	if err != nil {
		runtime.LogErrorf(ctx, "open database: %v", err)
		return
	}
	a.store = st
	a.thumbs = cache.NewThumbStore(paths.Thumbs)
	ensureThumbGeneration(paths.Thumbs)

	// loopback media server (unless the user chose blob transport)
	if s.MediaTransport != "blob" {
		ms, err := serve.Start(mediaProvider{a})
		if err != nil {
			runtime.LogErrorf(ctx, "start media server: %v", err)
		} else {
			a.media = ms
		}
	}

	doc, err := config.LoadLayout()
	if err != nil {
		runtime.LogErrorf(ctx, "load layout: %v", err)
	}
	a.layout = doc
	a.syncWatcher()
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

// shutdown closes resources when the app exits.
func (a *App) shutdown(ctx context.Context) {
	if a.watcher != nil {
		a.watcher.Stop()
	}
	if a.media != nil {
		a.media.Close()
	}
	if a.store != nil {
		a.store.Close()
	}
}

// ---- library / lifecycle ----

// GetSettings returns the current settings.
func (a *App) GetSettings() model.Settings { return a.settings }

// AddLibrary persists a folder to the library roots and scans it.
func (a *App) AddLibrary(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("folder not found: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("not a folder: %s", dir)
	}
	for _, r := range a.settings.LibraryRoots {
		if strings.EqualFold(cleanPath(r), cleanPath(dir)) {
			return nil // already added
		}
	}
	a.settings.LibraryRoots = append(a.settings.LibraryRoots, dir)
	if err := config.SaveSettings(a.settings); err != nil {
		return err
	}
	a.startScan()
	a.syncWatcher()
	return nil
}

// PickLibrary opens a directory dialog and adds the chosen folder.
func (a *App) PickLibrary() (model.Settings, error) {
	if a.ctx == nil {
		return a.settings, fmt.Errorf("app not ready")
	}
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose your comics folder",
	})
	if err != nil {
		return a.settings, err
	}
	if dir == "" {
		return a.settings, nil // cancelled
	}
	return a.settings, a.AddLibrary(dir)
}

// RemoveLibrary removes a root without deleting files. Indexed comics under
// that folder are purged so they stop appearing on the shelf. It also stops
// any scan currently adding rows from that folder (otherwise commits that land
// after the purge would linger as orphans) and enforces the invariant that no
// indexed comic lives outside the remaining roots.
func (a *App) RemoveLibrary(dir string) error {
	var kept []string
	for _, r := range a.settings.LibraryRoots {
		if !strings.EqualFold(cleanPath(r), cleanPath(dir)) {
			kept = append(kept, r)
		}
	}
	a.settings.LibraryRoots = kept
	if err := config.SaveSettings(a.settings); err != nil {
		return err
	}
	a.CancelScan()                  // stop the scan that may still be adding this dir's rows
	a.waitScanDone(3 * time.Second) // ...and wait for it to actually exit
	a.pruneStaleRoots()
	if len(kept) > 0 {
		a.startScan() // on completion its sweep removes any straggler commits
	}
	a.syncWatcher()
	return nil
}

// pruneStaleRoots deletes any indexed comic that is not under a current
// library root (rows whose folder was removed mid-scan, etc.).
func (a *App) pruneStaleRoots() {
	paths, err := a.store.ListPaths()
	if err != nil {
		a.dbg("pruneStale list error: %v", err)
		return
	}
	removed := 0
	for _, p := range paths {
		keep := false
		for _, r := range a.settings.LibraryRoots {
			if index.IsWithin(r, p) {
				keep = true
				break
			}
		}
		if keep {
			continue
		}
		if id, ok, _ := a.store.IDFromPath(p); ok {
			a.thumbs.Remove(id)
		}
		if err := a.store.DeleteByPath(p); err == nil {
			removed++
		}
	}
	if removed > 0 {
		a.invalidate()
		a.dbg("pruned %d stale comic(s) outside library roots", removed)
	}
}

// purgeRoot deletes indexed rows (and thumbnails) for archives under dir.
func (a *App) purgeRoot(dir string) {
	paths, err := a.store.ListPaths()
	if err != nil {
		a.dbg("purgeRoot list error: %v", err)
		return
	}
	for _, p := range paths {
		if !index.IsWithin(dir, p) {
			continue
		}
		if id, ok, _ := a.store.IDFromPath(p); ok {
			a.thumbs.Remove(id)
		}
		if err := a.store.DeleteByPath(p); err != nil {
			a.dbg("purgeRoot delete %s: %v", p, err)
		}
	}
	a.invalidate()
}

// GetAbout returns info for the About dialog.
func (a *App) GetAbout() model.About {
	ab := model.About{Version: Version, MediaTransport: a.settings.MediaTransport}
	if a.paths != nil {
		ab.ConfigDir = a.paths.Root
	}
	ab.ComicCount = len(a.currentMetas())
	return ab
}

// StartScan begins an incremental scan of all library roots.
func (a *App) StartScan() error {
	if len(a.settings.LibraryRoots) == 0 {
		return fmt.Errorf("no library folder added yet")
	}
	a.startScan()
	return nil
}

// CancelScan stops an in-flight scan.
func (a *App) CancelScan() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.scanCancel != nil {
		a.scanCancel()
	}
}

// waitScanDone blocks until a running scan goroutine has fully exited (so no
// late commits land after a removal), or until the timeout elapses.
func (a *App) waitScanDone(timeout time.Duration) {
	a.mu.Lock()
	ch := a.scanDone
	a.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case <-ch:
	case <-time.After(timeout):
	}
}

// IsScanning reports whether a scan is in progress.
func (a *App) IsScanning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.scanCancel != nil
}

func (a *App) startScan() {
	done := make(chan struct{})
	a.mu.Lock()
	if a.scanCancel != nil {
		a.scanCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.scanCancel = cancel
	a.scanDone = done
	a.mu.Unlock()

	roots := append([]string(nil), a.settings.LibraryRoots...)
	a.dbg("startScan begin roots=%v", roots)
	runtime.EventsEmit(a.ctx, "scan:started", struct{}{})

	go func() {
		defer close(done)
		opts := scan.Options{
			Store:  a.store,
			Thumbs: a.thumbs,
			Roots:  roots,
			OnProgress: func(p model.ScanProgress) {
				runtime.EventsEmit(a.ctx, "scan:progress", p)
			},
			OnError: func(path string, err error) {
				runtime.EventsEmit(a.ctx, "scan:error", map[string]string{"path": path, "error": err.Error()})
			},
		}
		err := scan.Run(ctx, opts)
		a.mu.Lock()
		a.scanCancel = nil
		a.mu.Unlock()
		a.invalidate()
		// Enforce "no comic outside the current roots": removes any rows a
		// cancelled scan committed after the user removed their folder.
		a.pruneStaleRoots()
		if a.watcher != nil {
			a.watcher.Sync() // pick up newly created subfolders
		}
		a.dbg("scan finished err=%v", err)

		if err != nil {
			runtime.EventsEmit(a.ctx, "scan:done", map[string]any{"error": err.Error()})
		} else {
			runtime.EventsEmit(a.ctx, "scan:done", map[string]any{"ok": true})
		}
	}()
}

// ---- rules / query ----

// ListRules returns all registered filter and sort rules for dropdowns.
func (a *App) ListRules() []model.RuleInfo {
	var out []model.RuleInfo
	for _, r := range plugins.FilterRules {
		out = append(out, model.RuleInfo{ID: r.ID(), Label: r.Label(), Kind: "filter"})
	}
	for _, r := range plugins.SortRules {
		out = append(out, model.RuleInfo{ID: r.ID(), Label: r.Label(), Kind: "sort"})
	}
	return out
}

// CountComics returns the number of comics matching a filter ("" = all).
func (a *App) CountComics(filterRuleID string) int {
	n := len(a.filtered(filterRuleID))
	a.dbg("CountComics(filter=%q) -> %d", filterRuleID, n)
	return n
}

// CountMatching returns the number of comics matching a free-text query
// (optionally narrowed by a filter rule). Used by the search box.
func (a *App) CountMatching(search, filterRuleID string) int {
	return len(a.selection(search, filterRuleID))
}

// selection returns the comics matching an optional free-text search and an
// optional filter rule. Search is case-insensitive substring over title,
// filename and every metadata value/key.
func (a *App) selection(search, filterRuleID string) []model.Meta {
	list := a.filtered(filterRuleID)
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return list
	}
	out := make([]model.Meta, 0, len(list))
	for _, m := range list {
		if searchMetaMatch(m, needle) {
			out = append(out, m)
		}
	}
	return out
}

func searchMetaMatch(m model.Meta, needle string) bool {
	if strings.Contains(strings.ToLower(m.Title()), needle) ||
		strings.Contains(strings.ToLower(m.Filename), needle) {
		return true
	}
	for k, v := range m.KV {
		if strings.Contains(strings.ToLower(k), needle) ||
			strings.Contains(strings.ToLower(v), needle) {
			return true
		}
	}
	return false
}

// ReportFrontend lets the UI log diagnostics into the debug file (debugging aid).
func (a *App) ReportFrontend(msg string) {
	a.dbg("FRONTEND: %s", msg)
}

// GetComics returns one paged slice of cards matching the query.
func (a *App) GetComics(q model.PageQuery) ([]model.ComicCard, error) {
	list := a.selection(q.Search, q.FilterRuleID)
	if q.SortRuleID != "" {
		if sr := plugins.SortByID(q.SortRuleID); sr != nil {
			sort.SliceStable(list, func(i, j int) bool { return sr.Less(list[i], list[j]) })
		}
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	if q.Limit <= 0 {
		q.Limit = 200
	}
	start := q.Offset
	if start > len(list) {
		start = len(list)
	}
	end := start + q.Limit
	if end > len(list) {
		end = len(list)
	}
	cards := make([]model.ComicCard, 0, end-start)
	for _, m := range list[start:end] {
		cards = append(cards, model.ComicCard{
			ID: m.ID, Filename: m.Filename, Title: m.Title(),
			PageCount: m.PageCount, Fav: m.Fav, LastPage: m.LastReadPage,
			HasNonUTF8: m.HasNonUTF8,
			CoverURL:   a.shelfCoverURL(m.ID),
		})
	}
	a.dbg("GetComics(%+v) -> %d cards of %d matched", q, len(cards), len(list))
	return cards, nil
}

// shelfCoverURL pre-fills cover URLs only in http mode. In blob/data fallback
// mode it returns "" so the frontend lazily fetches each visible cover via
// GetCover instead of inlining data for a whole page of cards at once.
func (a *App) shelfCoverURL(id int64) string {
	if a.media != nil {
		return a.media.CoverURL(id)
	}
	return ""
}

// GetComicDetail returns full metadata for the info panel.
func (a *App) GetComicDetail(id int64) (model.ComicDetail, error) {
	rows, err := a.store.RowByID(id)
	if err != nil {
		return model.ComicDetail{}, err
	}
	m := rowToMeta(*rows)
	var keys []string
	for k := range m.KV {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return model.ComicDetail{Meta: m, CommentRaw: rows.CommentRaw, AllMetaKeys: keys}, nil
}

// GetCover returns a transport URL for a comic's cover thumbnail.
func (a *App) GetCover(id int64) model.CoverRef {
	return model.CoverRef{URL: a.coverURL(id)}
}

// GetPageRef returns a transport URL for one reader page.
func (a *App) GetPageRef(id int64, n int) model.PageRef {
	if a.media != nil {
		return model.PageRef{URL: a.media.PageURL(id, n)}
	}
	b, mime, err := a.pageBytes(id, n)
	if err != nil {
		return model.PageRef{}
	}
	return model.PageRef{URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)}
}

// ---- reader ----

// OpenComic returns the page count for a comic (used to size the reader).
func (a *App) OpenComic(id int64) (model.OpenResult, error) {
	if _, err := a.store.PathFromID(id); err != nil {
		return model.OpenResult{}, err
	}
	rows, err := a.store.RowByID(id)
	if err != nil {
		return model.OpenResult{}, err
	}
	return model.OpenResult{ID: id, PageCount: rows.PageCount}, nil
}

// ---- progress / favorites ----

// GetProgress returns the saved reading state of a comic.
func (a *App) GetProgress(id int64) model.Progress {
	rows, err := a.store.RowByID(id)
	if err != nil {
		return model.Progress{ComicID: id}
	}
	return model.Progress{ComicID: id, LastPage: rows.LastPage, PageCount: rows.PageCount}
}

// SetProgress records the current page and refreshes rule inputs.
func (a *App) SetProgress(id int64, lastPage, pageCount int) error {
	if err := a.store.SetProgress(id, lastPage, pageCount); err != nil {
		return err
	}
	a.invalidate()
	return nil
}

// ToggleFavorite flips the favorite mark and returns the new state.
func (a *App) ToggleFavorite(id int64) (bool, error) {
	cur, err := a.store.IsFavorite(id)
	if err != nil {
		return false, err
	}
	next := !cur
	if err := a.store.SetFavorite(id, next); err != nil {
		return false, err
	}
	a.invalidate()
	return next, nil
}

// GetFavorites returns the ids of favorited comics.
func (a *App) GetFavorites() ([]int64, error) {
	var out []int64
	for _, m := range a.currentMetas() {
		if m.Fav {
			out = append(out, m.ID)
		}
	}
	return out, nil
}

// ---- layout ----

// GetLayout returns the current layout document.
func (a *App) GetLayout() model.LayoutDoc { return a.layout }

// ReloadLayout re-reads layout.json from disk (after a hand edit).
func (a *App) ReloadLayout() error {
	doc, err := config.LoadLayout()
	if err != nil {
		return err
	}
	a.layout = doc
	runtime.EventsEmit(a.ctx, "layout:changed", struct{}{})
	return nil
}

// SaveLayout persists a layout document and reloads it.
func (a *App) SaveLayout(doc model.LayoutDoc) error {
	if err := config.SaveLayout(doc); err != nil {
		return err
	}
	a.layout = doc
	runtime.EventsEmit(a.ctx, "layout:changed", struct{}{})
	return nil
}

// UpdateSectionRule changes the filter/sort rule of one layout section.
// kind is "filter" or "sort".
func (a *App) UpdateSectionRule(sectionID, kind, ruleID string) error {
	doc := a.layout
	found := false
	for i := range doc.Sections {
		if doc.Sections[i].ID == sectionID {
			switch kind {
			case "filter":
				doc.Sections[i].FilterRuleID = ruleID
			case "sort":
				doc.Sections[i].SortRuleID = ruleID
			default:
				return fmt.Errorf("unknown rule kind %q", kind)
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("section %q not found", sectionID)
	}
	return a.SaveLayout(doc)
}

// CardData returns the dynamic payload for a plugin card section.
func (a *App) CardData(cardID string) map[string]any {
	if plugins.CardByID(cardID) == nil {
		return map[string]any{"missing": true}
	}
	metas := a.currentMetas()
	switch cardID {
	case "stats":
		var fav, unread, inProg int
		for _, m := range metas {
			if m.Fav {
				fav++
			}
			if m.LastReadPage == 0 {
				unread++
			}
			if m.LastReadPage > 0 && m.LastReadPage+1 < m.PageCount {
				inProg++
			}
		}
		return map[string]any{"total": len(metas), "favorites": fav, "unread": unread, "inProgress": inProg}
	}
	return map[string]any{}
}

// ---- media provider internals ----

func (a *App) coverURL(id int64) string {
	if a.media != nil {
		return a.media.CoverURL(id)
	}
	b, _, err := a.coverBytes(id)
	if err != nil {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b)
}

func (a *App) coverBytes(id int64) ([]byte, string, error) {
	if a.thumbs.Has(id) {
		if b, err := a.thumbs.Read(id); err == nil {
			return b, "image/jpeg", nil
		}
	}
	path, err := a.store.PathFromID(id)
	if err != nil {
		return nil, "", err
	}
	// Bounded decode: at most two cover generations run at once, so decoding a
	// fresh library's huge first pages never spikes memory.
	a.coverSem <- struct{}{}
	defer func() { <-a.coverSem }()
	raw, err := openCoverRaw(path)
	if err != nil {
		return nil, "", err
	}
	// 256px long edge keeps decoded-in-browser covers small (less memory per
	// scrolled image while remaining crisp at shelf sizes).
	thumb, err := imgutil.ThumbJPEG(raw, 256, 80)
	if err != nil {
		return nil, "", err
	}
	_ = a.thumbs.Write(id, thumb) // best-effort cache
	// Return freed memory to the OS now and then, so Task Manager doesn't show
	// the whole transient-decode peak as resident memory.
	if a.coverGen.Add(1)%30 == 0 {
		goruntime.GC()
		debug.FreeOSMemory()
	}
	return thumb, "image/jpeg", nil
}

func (a *App) pageBytes(id int64, n int) ([]byte, string, error) {
	key := fmt.Sprintf("%d:%d", id, n)
	if b, ok := a.pageCache.Get(key); ok {
		return b, imgutil.SniffType(b), nil
	}
	path, err := a.store.PathFromID(id)
	if err != nil {
		return nil, "", err
	}
	ar, err := comic.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer ar.Close()
	rc, err := ar.OpenPage(n)
	if err != nil {
		return nil, "", err
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", err
	}
	a.pageCache.Put(key, b)
	return b, imgutil.SniffType(b), nil
}

// ---- filtering helpers ----

func (a *App) filtered(filterRuleID string) []model.Meta {
	metas := a.currentMetas()
	out := make([]model.Meta, len(metas))
	copy(out, metas)
	if filterRuleID != "" {
		if fr := plugins.FilterByID(filterRuleID); fr != nil {
			kept := out[:0]
			for _, m := range out {
				if fr.Keep(m) {
					kept = append(kept, m)
				}
			}
			out = kept
		}
	}
	return out
}

func (a *App) currentMetas() []model.Meta {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.metasDirty {
		if rows, err := a.store.AllRows(); err == nil {
			metas := make([]model.Meta, 0, len(rows))
			for i := range rows {
				metas = append(metas, rowToMeta(rows[i]))
			}
			a.metas = metas
			a.dbg("currentMetas reload: %d rows (dirty was true)", len(rows))
		} else {
			a.dbg("currentMetas reload ERROR: %v", err)
		}
		a.metasDirty = false
	} else {
		a.dbg("currentMetas cache hit: %d metas (not dirty)", len(a.metas))
	}
	return a.metas
}

func (a *App) invalidate() {
	a.mu.Lock()
	a.metasDirty = true
	a.mu.Unlock()
}

func rowToMeta(r index.Row) model.Meta {
	return model.Meta{
		ID:           r.ID,
		Path:         r.Path,
		Filename:     r.Filename,
		Dir:          r.Dir,
		PageCount:    r.PageCount,
		FileSize:     r.FileSize,
		UpdatedAt:    r.MtimeNs / 1e6,
		LastReadPage: r.LastPage,
		ReadAt:       r.ReadAt,
		Fav:          r.Fav,
		HasNonUTF8:   r.HasNonUTF8,
		KV:           r.KV,
	}
}

func cleanPath(p string) string {
	return strings.ToLower(strings.ReplaceAll(p, "\\", "/"))
}

func openCoverRaw(path string) ([]byte, error) {
	ar, err := comic.Open(path)
	if err != nil {
		return nil, err
	}
	defer ar.Close()
	rc, err := ar.OpenPage(0)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
