package main

import (
	"path/filepath"
	"testing"

	"comicgo/internal/cache"
	"comicgo/internal/index"
	"comicgo/internal/model"
)

func TestAppCountQueryProgressFav(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	st, err := index.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	id, err := st.CommitComic(index.Record{
		Path: filepath.Join(`C:\lib`, "a.cbz"), Dir: `C:\lib`, Filename: "a.cbz",
		FileSize: 10, MtimeNs: 1, PageCount: 5, CommentRaw: "Title=Alpha",
		KV: map[string]string{"Title": "Alpha"},
	})
	if err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	a.store = st

	if n := a.CountComics(""); n != 1 {
		t.Fatalf("CountComics(all) = %d, want 1", n)
	}
	cards, err := a.GetComics(model.PageQuery{Offset: 0, Limit: 10, SortRuleID: "bytitle"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 {
		t.Fatalf("GetComics returned %d cards, want 1", len(cards))
	}
	if cards[0].Title != "Alpha" {
		t.Errorf("card title = %q", cards[0].Title)
	}

	// progress + favorite filter round-trip
	if err := a.SetProgress(id, 2, 5); err != nil {
		t.Fatal(err)
	}
	if n := a.CountComics("unread"); n != 0 {
		t.Errorf("unread count = %d, want 0 after reading p3", n)
	}
	on, err := a.ToggleFavorite(id)
	if err != nil || !on {
		t.Fatalf("toggle fav = %v, %v", on, err)
	}
	if n := a.CountComics("favorites"); n != 1 {
		t.Errorf("favorites count = %d, want 1", n)
	}
	if p := a.GetProgress(id); p.LastPage != 2 {
		t.Errorf("progress lastPage = %d, want 2", p.LastPage)
	}
}

func TestPurgeRootOnlyAffectsTargetDir(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	st, err := index.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	dirA := filepath.Join(`C:\libA`)
	dirB := filepath.Join(`C:\libB`)
	for _, d := range []string{dirA, dirB} {
		if _, err := st.CommitComic(index.Record{
			Path: filepath.Join(d, "c.cbz"), Dir: d, Filename: "c.cbz",
			FileSize: 10, MtimeNs: 1, PageCount: 2, KV: map[string]string{},
		}); err != nil {
			t.Fatal(err)
		}
	}

	a := NewApp()
	a.store = st
	a.thumbs = cache.NewThumbStore(filepath.Join(t.TempDir(), "thumbs"))

	a.purgeRoot(dirA)

	rows, err := st.AllRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("after purging dirA rows = %d, want 1 (dirB untouched)", len(rows))
	}
	if rows[0].Dir != dirB {
		t.Errorf("remaining row is in %q, want dirB %q", rows[0].Dir, dirB)
	}
	if n := a.CountComics(""); n != 1 {
		t.Errorf("CountComics after purge = %d, want 1", n)
	}
}
