package application

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"strings"

	"comicgo/internal/comic"
	"comicgo/internal/config"
	"comicgo/internal/imgutil"
	"comicgo/internal/index"
	"comicgo/internal/model"
	"comicgo/internal/plugins"
)

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

// CoverBytes returns the first page and its media extension for a comic.
func (a *App) CoverBytes(id int64) ([]byte, string, error) {
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

// PageBytes returns one page and its media extension for a comic.
func (a *App) PageBytes(id int64, n int) ([]byte, string, error) {
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

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if goruntime.GOOS == "windows" {
		return strings.EqualFold(filepath.ToSlash(a), filepath.ToSlash(b))
	}
	if a == b {
		return true
	}
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	return aerr == nil && berr == nil && os.SameFile(ai, bi)
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
