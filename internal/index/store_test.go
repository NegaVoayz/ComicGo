package index

import (
	"path/filepath"
	"testing"
)

func TestStoreLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	rec1 := Record{
		Path: filepath.Join("C:", "zips", "a.cbz"), Dir: filepath.Join("C:", "zips"),
		Filename: "a.cbz", FileSize: 100, MtimeNs: 111,
		PageCount: 5, CoverEntry: "p1.jpg", CommentRaw: "Title=Alpha",
		KV: map[string]string{"Title": "Alpha", "Genre": "Adventure"},
	}
	rec2 := Record{
		Path: filepath.Join("C:", "zips", "b.cbz"), Dir: filepath.Join("C:", "zips"),
		Filename: "b.cbz", FileSize: 200, MtimeNs: 222,
		PageCount: 8, CoverEntry: "p1.jpg", CommentRaw: "Title=Beta",
		KV: map[string]string{"Title": "Beta"},
	}

	id1, err := s.CommitComic(rec1)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != 1 {
		t.Errorf("id1 = %d", id1)
	}
	if _, err := s.CommitComic(rec2); err != nil {
		t.Fatal(err)
	}

	// Re-commit rec1 with a changed fingerprint and KV to test replace.
	rec1.MtimeNs = 999
	rec1.KV = map[string]string{"Title": "Alpha2"}
	if _, err := s.CommitComic(rec1); err != nil {
		t.Fatal(err)
	}

	rows, err := s.AllRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	var a, b *Row
	for i := range rows {
		if rows[i].Filename == "a.cbz" {
			a = &rows[i]
		} else {
			b = &rows[i]
		}
	}
	if a == nil || b == nil {
		t.Fatal("rows not found")
	}
	if a.KV["Title"] != "Alpha2" {
		t.Errorf("a KV replaced title = %q", a.KV["Title"])
	}
	if b.KV["Genre"] != "" {
		t.Error("b should not have Genre")
	}

	// fingerprint found & updated
	if size, mt, found, err := s.FindFingerprint(rec1.Path); err != nil || !found || size != 100 || mt != 999 {
		t.Errorf("FindFingerprint(a) = %d/%d/%v/%v", size, mt, found, err)
	}

	// progress + favorites
	if err := s.SetProgress(a.ID, 3, 5); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFavorite(a.ID, true); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.AllRows()
	for i := range rows {
		if rows[i].ID == a.ID {
			if rows[i].LastPage != 3 || !rows[i].Fav || rows[i].ReadAt == 0 {
				t.Errorf("a state = lastPage %d fav %v readAt %d", rows[i].LastPage, rows[i].Fav, rows[i].ReadAt)
			}
		}
	}
	if err := s.SetFavorite(a.ID, false); err != nil {
		t.Fatal(err)
	}

	// delete removes cascade
	if err := s.DeleteByPath(rec2.Path); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.AllRows()
	if len(rows) != 1 {
		t.Errorf("rows after delete = %d", len(rows))
	}
	if p, err := s.PathFromID(a.ID); err != nil || p != rec1.Path {
		t.Errorf("PathFromID = %q, %v", p, err)
	}
}

func TestIsWithin(t *testing.T) {
	dir := `C:\library\sub`
	if !IsWithin(dir, `C:\library\sub\a.cbz`) {
		t.Error("should be within")
	}
	if IsWithin(dir, `C:\library\other\a.cbz`) {
		t.Error("sibling should not be within")
	}
	if IsWithin(`C:\library`, `C:\library\a.cbz`) {
		// dir itself is a root; its files are within
	} else {
		t.Error("direct child should be within")
	}
}
