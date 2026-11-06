package application

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"comicgo/internal/comic"
	"comicgo/internal/importer"
	"comicgo/internal/index"
	"comicgo/internal/meta"
	"comicgo/internal/model"
	"comicgo/internal/plugins"
)

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
