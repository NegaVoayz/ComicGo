package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"comicgo/internal/cache"
	"comicgo/internal/comic"
	"comicgo/internal/config"
	"comicgo/internal/imgutil"
	"comicgo/internal/index"
	"comicgo/internal/model"
	"comicgo/internal/plugins"
	"comicgo/internal/scan"
	"comicgo/internal/serve"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

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

	pageCache *cache.LRU
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
	return &App{metasDirty: true, pageCache: cache.NewLRU(8 << 20)}
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
}

// shutdown closes resources when the app exits.
func (a *App) shutdown(ctx context.Context) {
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

// RemoveLibrary removes a root without deleting files.
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
	a.startScan()
	return nil
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

// IsScanning reports whether a scan is in progress.
func (a *App) IsScanning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.scanCancel != nil
}

func (a *App) startScan() {
	a.mu.Lock()
	if a.scanCancel != nil {
		a.scanCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.scanCancel = cancel
	a.mu.Unlock()

	roots := append([]string(nil), a.settings.LibraryRoots...)
	runtime.EventsEmit(a.ctx, "scan:started", struct{}{})

	go func() {
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
	return len(a.filtered(filterRuleID))
}

// GetComics returns one paged slice of cards matching the query.
func (a *App) GetComics(q model.PageQuery) ([]model.ComicCard, error) {
	list := a.filtered(q.FilterRuleID)
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
	raw, err := openCoverRaw(path)
	if err != nil {
		return nil, "", err
	}
	thumb, err := imgutil.ThumbJPEG(raw, 320, 80)
	if err != nil {
		return nil, "", err
	}
	_ = a.thumbs.Write(id, thumb) // best-effort cache
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
		}
		a.metasDirty = false
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
