package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"comicgo/internal/cache"
	"comicgo/internal/comic"
	"comicgo/internal/config"
	"comicgo/internal/imgutil"
	"comicgo/internal/importer"
	"comicgo/internal/index"
	"comicgo/internal/meta"
	"comicgo/internal/model"
	"comicgo/internal/plugins"
	"comicgo/internal/scan"
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

// ---- comic import ----

// ImportSources converts every supported archive or image folder into a .zip
// under destination, then starts an index scan. The GUI owns source and
// destination dialogs; this method only validates and performs the work.
func (a *App) ImportSources(sources []string, destination string) ([]model.ImportOutcome, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	if len(a.settings.LibraryRoots) == 0 {
		return nil, fmt.Errorf("add a library folder before importing comics")
	}
	validDest := false
	for _, root := range a.settings.LibraryRoots {
		if index.IsWithin(root, destination) {
			validDest = true
			break
		}
	}
	if !validDest {
		return nil, fmt.Errorf("import folder is not inside a library folder: %s", destination)
	}
	if !a.importMu.TryLock() {
		return nil, errors.New("an import is already running")
	}
	defer a.importMu.Unlock()

	var imported []model.ImportOutcome
	var failures []string
	for _, src := range sources {
		out, err := a.importOne(src, destination)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if out != nil {
			imported = append(imported, *out)
		}
	}
	if len(imported) == 0 {
		if len(failures) > 0 {
			return nil, errors.New(strings.Join(failures, "; "))
		}
		return nil, errors.New("nothing to import: drop comic archives or image folders")
	}
	a.startScan() // the new .zip files are now complete; index them
	if len(failures) > 0 {
		return imported, errors.New(strings.Join(failures, "; "))
	}
	return imported, nil
}

// importOne converts a single source into a unique .zip under destDir, emitting
// import:progress events while it streams. Returns (nil, nil) for a source this
// app cannot import (a missing path or a file that is not an archive), so batch
// drops can skip such noise without failing the rest.
func (a *App) importOne(source, destDir string) (*model.ImportOutcome, error) {
	fi, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // a stale dropped path
		}
		return nil, err
	}
	isDir := fi.IsDir()
	if !isDir && !isImportableExt(source) {
		return nil, nil // not an archive we read; skip silently
	}
	// A "brace" source name ({Author...} Title {Tag...}) becomes a .zip named
	// after the clean title, with the parsed metadata written into the archive
	// comment (Title/Author/Genre) so the shelf shows author + tag chips and tag
	// search works. Plain names import unchanged (no comment).
	stem := importer.DesiredStem(source, isDir)
	title, author, tags := importer.ParseName(stem)
	base := stem + ".zip"
	comment := ""
	if title != "" && (author != "" || len(tags) > 0) {
		base = title + ".zip"
		comment = importer.BuildComment(title, author, tags)
	}
	dest := importer.UniqueDest(destDir, base)
	pages, err := importer.Import(context.Background(), importer.Options{
		Source:  source,
		Dest:    dest,
		Comment: comment,
		OnProgress: func(done, total int) {
			a.importProgress(model.ImportProgress{
				Done: done, Total: total, Name: filepath.Base(dest),
			})
		},
	})
	if err != nil {
		return nil, err
	}
	kind := "file"
	if isDir {
		kind = "folder"
	}
	return &model.ImportOutcome{Path: dest, PageCount: pages, Source: source, Kind: kind}, nil
}

// isImportableExt reports whether a file path is an archive kind the importer
// can convert (used to skip non-archive files dropped alongside real ones).
func isImportableExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip", ".cbz", ".rar", ".cbr", ".7z", ".cb7", ".tar", ".gz", ".tgz":
		return true
	}
	return false
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
	ab := model.About{Version: Version}
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
	ch := a.scanDoneCh
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
	a.scanDoneCh = done
	a.mu.Unlock()

	roots := append([]string(nil), a.settings.LibraryRoots...)
	a.dbg("startScan begin roots=%v", roots)
	a.scanStarted()

	go func() {
		defer close(done)
		opts := scan.Options{
			Store:  a.store,
			Thumbs: a.thumbs,
			Roots:  roots,
			OnProgress: func(p model.ScanProgress) {
				a.scanProgress(p)
			},
			OnError: func(path string, err error) {
				a.scanError(path, err)
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

		a.emitScanDone(err)
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

// CountMatching returns the number of comics matching a search query
// (optionally narrowed by a filter rule). The query may hold plain words and
// tag terms (tag:...); see parseSearch. Count and list go through the same
// parser so they can never disagree.
func (a *App) CountMatching(search, filterRuleID string) int {
	return len(a.selection(search, filterRuleID))
}

// searchQuery is a parsed search box query: text substrings and exact whole-tag
// matches. Every term must match (logical AND): all text terms must occur
// somewhere and all tags must be present on the comic.
type searchQuery struct {
	text []string // lower-cased substrings, all must occur somewhere
	tags []string // whole-tag matches (case-insensitive), all must be present
}

// parseSearch turns a search box string into structured terms. Plain words are
// text substrings (case-insensitive, anywhere in title/file/metadata); a term
// of the form tag:name matches a whole tag exactly (ロリ does not match ロリコン).
// tag:"a b" quotes a tag that contains spaces. Double quotes group any text; a
// backslash escapes the next character inside a quoted group.
func parseSearch(s string) searchQuery {
	var q searchQuery
	for _, tok := range splitSearchTerms(s) {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if v, ok := strings.CutPrefix(tok, "tag:"); ok {
			v = strings.TrimSpace(v)
			if v != "" {
				q.tags = append(q.tags, v)
			}
		} else {
			q.text = append(q.text, strings.ToLower(tok))
		}
	}
	return q
}

// splitSearchTerms splits s on whitespace but keeps double-quoted groups whole,
// so tag:"a b" stays a single term. Quote characters are dropped; a backslash
// escapes the next character inside a quoted group.
func splitSearchTerms(s string) []string {
	var terms []string
	var b strings.Builder
	inQ, esc := false, false
	flush := func() {
		if b.Len() > 0 {
			terms = append(terms, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		if esc {
			b.WriteRune(r)
			esc = false
			continue
		}
		switch r {
		case '\\':
			if inQ {
				esc = true
			} else {
				b.WriteRune(r)
			}
		case '"':
			inQ = !inQ
		case ' ', '\t', '\r', '\n':
			if inQ {
				b.WriteRune(r)
			} else {
				flush()
			}
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return terms
}

// selection returns the comics matching an optional search query and filter
// rule. An empty query selects everything (still honouring the filter).
func (a *App) selection(search, filterRuleID string) []model.Meta {
	list := a.filtered(filterRuleID)
	q := parseSearch(search)
	if len(q.text) == 0 && len(q.tags) == 0 {
		return list
	}
	out := make([]model.Meta, 0, len(list))
	for _, m := range list {
		if searchQueryMatch(m, q) {
			out = append(out, m)
		}
	}
	return out
}

func searchQueryMatch(m model.Meta, q searchQuery) bool {
	for _, needle := range q.text {
		if !textFieldMatch(m, needle) {
			return false
		}
	}
	for _, tag := range q.tags {
		if !metaHasTag(m, tag) {
			return false
		}
	}
	return true
}

// textFieldMatch reports whether needle occurs (case-insensitively) in the
// title, file name, or any metadata key/value.
func textFieldMatch(m model.Meta, needle string) bool {
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

// metaHasTag reports whether the comic carries tag as one of its Genre comment
// tags, compared exactly and case-insensitively. Genre is the list the UI shows
// as tag chips (Author is a separate key, not a searchable tag).
func metaHasTag(m model.Meta, tag string) bool {
	for _, part := range strings.Split(m.KV["Genre"], ",") {
		if strings.EqualFold(strings.TrimSpace(part), tag) {
			return true
		}
	}
	return false
}

// cardTagLimit bounds how many comment tags travel with each shelf card, so a
// big tagged library does not bloat every page payload.
const cardTagLimit = 6

// cardTags returns the comic's Genre comment tags (the content labels we
// imported), trimmed and de-duplicated, for the card caption. The author is
// surfaced separately on its own caption line, not as one of these chips.
func cardTags(m model.Meta) []string {
	genre := strings.TrimSpace(m.KV["Genre"])
	if genre == "" {
		return nil
	}
	seen := make(map[string]bool, cardTagLimit)
	out := make([]string, 0, cardTagLimit)
	for _, part := range strings.Split(genre, ",") {
		t := strings.TrimSpace(part)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) == cardTagLimit {
			break
		}
	}
	return out
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
			Author:    strings.TrimSpace(m.KV["Author"]),
			Tags:      cardTags(m),
			PageCount: m.PageCount, Fav: m.Fav, LastPage: m.LastReadPage,
			HasNonUTF8: m.HasNonUTF8,
		})
	}
	a.dbg("GetComics(%+v) -> %d cards of %d matched", q, len(cards), len(list))
	return cards, nil
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

// ---- comic metadata ----

// UpdateComicMetadata persists an edit to one comic's comment metadata
// (title/author/genre/source) by rewriting the archive comment. When the new
// Title differs from the current file name, the archive itself is renamed to
// match (collision-safe) and the index row follows the file so favorites and
// reading progress survive the rename. Returns the refreshed detail.
func (a *App) UpdateComicMetadata(id int64, edit model.MetaEdit) (model.ComicDetail, error) {
	if a.store == nil {
		return model.ComicDetail{}, errors.New("app not ready")
	}
	// Serialized against imports: both rewrite files under a library root and
	// must not interleave.
	a.importMu.Lock()
	defer a.importMu.Unlock()

	path, err := a.store.PathFromID(id)
	if err != nil {
		return model.ComicDetail{}, err
	}

	// Build the new comment from the live file (never the possibly-stale cached
	// row) so concurrent edits to the same archive are not clobbered, and take
	// the page facts from the same open.
	ar, err := comic.Open(path)
	if err != nil {
		return model.ComicDetail{}, err
	}
	kv := meta.Parse(ar.Comment())
	pageCount, coverEntry, hasNonUTF8 := ar.PageCount(), ar.CoverName(), ar.HasNonUTF8()
	if err := ar.Close(); err != nil {
		return model.ComicDetail{}, err
	}
	oldComment := meta.Build(kv)
	priorTitle := kv["Title"] // pre-edit value, for the rename decision below

	title := strings.TrimSpace(edit.Title)
	setField := func(key, value string) {
		if value != "" {
			kv[key] = value
		} else {
			delete(kv, key)
		}
	}
	setField("Title", title)
	setField("Author", strings.TrimSpace(edit.Author))
	setField("Genre", meta.CleanTags(edit.Genre))
	setField("Source", strings.TrimSpace(edit.Source))
	comment := meta.Build(kv)

	// Rename the archive only when the Title field itself changed to a real,
	// safe name — a plain Save of an unchanged form never moves the file. The
	// reference is the previous comment Title, or the file stem when unset (the
	// value the edit form was pre-filled with).
	newPath := path
	if title != "" {
		prior := priorTitle
		if prior == "" {
			prior = model.StripArchiveExt(filepath.Base(path))
		}
		if safe := safeComicName(title); safe != "" && !strings.EqualFold(safe, prior) {
			newPath = importer.UniqueDest(filepath.Dir(path), safe+filepath.Ext(path))
		}
	}
	renamed := newPath != path

	if comment == oldComment && !renamed {
		return a.GetComicDetail(id) // nothing changed
	}

	// Rename the file first, then move the index row, so a failure later in the
	// edit can never drop the row's id (favorites and progress live on it).
	if renamed {
		if err := os.Rename(path, newPath); err != nil {
			return model.ComicDetail{}, fmt.Errorf("rename comic to %q: %w", filepath.Base(newPath), err)
		}
		if err := a.store.MovePath(path, newPath); err != nil {
			return model.ComicDetail{}, fmt.Errorf("move index row after rename: %w", err)
		}
	}

	if err := comic.RewriteComment(newPath, comment); err != nil {
		return model.ComicDetail{}, fmt.Errorf("write metadata into %q: %w", filepath.Base(newPath), err)
	}

	fi, err := os.Stat(newPath)
	if err != nil {
		return model.ComicDetail{}, err
	}
	if _, err := a.store.CommitComic(index.Record{
		Path:       newPath,
		Dir:        filepath.Dir(newPath),
		Filename:   filepath.Base(newPath),
		FileSize:   fi.Size(),
		MtimeNs:    fi.ModTime().UnixNano(),
		PageCount:  pageCount,
		CoverEntry: coverEntry,
		CommentRaw: comment,
		HasNonUTF8: hasNonUTF8,
		KV:         kv,
	}); err != nil {
		return model.ComicDetail{}, fmt.Errorf("commit updated metadata: %w", err)
	}

	a.invalidate()
	return a.GetComicDetail(id)
}

// safeComicName turns a user-entered title into something usable as a Windows
// file name: illegal characters are replaced with spaces, whitespace runs
// collapse, and trailing dots/spaces are trimmed (Windows ignores them). It
// returns "" when nothing usable remains, in which case callers skip the rename.
func safeComicName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimRight(strings.Join(strings.Fields(b.String()), " "), ". ")
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
	a.layoutChanged()
	return nil
}

// SaveLayout persists a layout document and reloads it.
func (a *App) SaveLayout(doc model.LayoutDoc) error {
	if err := config.SaveLayout(doc); err != nil {
		return err
	}
	a.layout = doc
	a.layoutChanged()
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

// ---- image provider internals ----

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
