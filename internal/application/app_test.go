package application

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"comicgo/internal/comic"
	"comicgo/internal/index"
	"comicgo/internal/meta"
	"comicgo/internal/model"
)

// writeImageCbz writes a minimal real cbz (two image entries, so comic.Open sees
// 2 pages with cover p1.jpg) carrying the given comment.
func writeImageCbz(t *testing.T, path, comment string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	if err := zw.SetComment(comment); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"p1.jpg", "p2.jpg"} {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, "fake-page-"+n); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// newMetaEditApp seeds one real comic file in a fresh dir + DB and returns the
// app (with store injected), the dir and the comic id.
func newMetaEditApp(t *testing.T, filename, comment string) (*App, string, int64) {
	t.Helper()
	st, err := index.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	dir := t.TempDir()
	path := filepath.Join(dir, filename)
	writeImageCbz(t, path, comment)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	ar, err := comic.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.CommitComic(index.Record{
		Path: path, Dir: dir, Filename: filename,
		FileSize: fi.Size(), MtimeNs: fi.ModTime().UnixNano(),
		PageCount: ar.PageCount(), CoverEntry: ar.CoverName(), CommentRaw: comment,
		KV: meta.Parse(comment),
	})
	ar.Close()
	if err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	a.store = st
	return a, dir, id
}

func readComment(t *testing.T, path string) string {
	t.Helper()
	ar, err := comic.Open(path)
	if err != nil {
		t.Fatalf("comic.Open(%q): %v", path, err)
	}
	defer ar.Close()
	return ar.Comment()
}

func TestUpdateMetadataOnlyNoRename(t *testing.T) {
	// File already named after the title, so a metadata edit must not rename it.
	a, dir, id := newMetaEditApp(t, "Alpha.cbz", "Title=Alpha\nAuthor=Old\nGenre=a\nSource=old-src")

	if err := a.store.SetFavorite(id, true); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetProgress(id, 1, 2); err != nil {
		t.Fatal(err)
	}

	got, err := a.UpdateComicMetadata(id, model.MetaEdit{
		Title: "Alpha", Author: "New Author", Genre: " x, X , y, ", Source: "",
	})
	if err != nil {
		t.Fatalf("UpdateComicMetadata: %v", err)
	}
	path := filepath.Join(dir, "Alpha.cbz")
	if p, _ := a.store.PathFromID(id); p != path {
		t.Errorf("path unchanged = %q, want %q", p, path)
	}
	kv := meta.Parse(readComment(t, path))
	if kv["Title"] != "Alpha" || kv["Author"] != "New Author" || kv["Genre"] != "x, y" {
		t.Errorf("comment kv = %v", kv)
	}
	if _, ok := kv["Source"]; ok {
		t.Errorf("Source should have been cleared, kv = %v", kv)
	}
	// Id, favorite and progress survive untouched.
	row, err := a.store.RowByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != id || !row.Fav || row.LastPage != 1 {
		t.Errorf("row after edit = id %d fav %v lastPage %d", row.ID, row.Fav, row.LastPage)
	}
	if got.Meta.Title() != "Alpha" {
		t.Errorf("returned detail title = %q", got.Meta.Title())
	}
}

func TestUpdateMetadataRenamesFileKeepsState(t *testing.T) {
	a, dir, id := newMetaEditApp(t, "Old Name.cbz", "Title=Old Name\nAuthor=Artist")

	if err := a.store.SetFavorite(id, true); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetProgress(id, 1, 2); err != nil {
		t.Fatal(err)
	}

	_, err := a.UpdateComicMetadata(id, model.MetaEdit{
		Title: "New Title", Author: "", Genre: "comedy, COMEDY", Source: "magnet:?xt=x",
	})
	if err != nil {
		t.Fatalf("UpdateComicMetadata: %v", err)
	}
	old := filepath.Join(dir, "Old Name.cbz")
	newPath := filepath.Join(dir, "New Title.cbz")
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("new file missing: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old file still present")
	}
	if p, err := a.store.PathFromID(id); err != nil || p != newPath {
		t.Errorf("PathFromID = %q, %v; want %q", p, err, newPath)
	}
	if _, _, found, _ := a.store.FindFingerprint(old); found {
		t.Error("old path still indexed")
	}
	if _, err := os.Stat(newPath + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file left behind")
	}

	kv := meta.Parse(readComment(t, newPath))
	if kv["Title"] != "New Title" || kv["Genre"] != "comedy" || kv["Source"] != "magnet:?xt=x" {
		t.Errorf("comment kv = %v", kv)
	}
	if _, ok := kv["Author"]; ok {
		t.Errorf("Author should have been cleared, kv = %v", kv)
	}
	row, err := a.store.RowByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != id || !row.Fav || row.LastPage != 1 {
		t.Errorf("row after rename = id %d fav %v lastPage %d (favorites/progress must survive)", row.ID, row.Fav, row.LastPage)
	}
}

func TestUpdateMetadataRenameCollisionPicksSuffix(t *testing.T) {
	a, dir, id := newMetaEditApp(t, "First.cbz", "Title=First")

	// Another real comic already occupies "New Title.cbz".
	writeImageCbz(t, filepath.Join(dir, "New Title.cbz"), "Title=Occupied")

	_, err := a.UpdateComicMetadata(id, model.MetaEdit{Title: "New Title"})
	if err != nil {
		t.Fatalf("UpdateComicMetadata: %v", err)
	}
	want := filepath.Join(dir, "New Title (2).cbz")
	if p, _ := a.store.PathFromID(id); p != want {
		t.Errorf("PathFromID = %q, want %q", p, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file %q: %v", want, err)
	}
}

func TestUpdateMetadataClearsAndPreservesUnknownKeys(t *testing.T) {
	a, dir, id := newMetaEditApp(t, "Keep.cbz", "Title=Keep\nCustom=PreserveMe\nAuthor=A")

	_, err := a.UpdateComicMetadata(id, model.MetaEdit{Author: ""})
	if err != nil {
		t.Fatalf("UpdateComicMetadata: %v", err)
	}
	kv := meta.Parse(readComment(t, filepath.Join(dir, "Keep.cbz")))
	if kv["Custom"] != "PreserveMe" {
		t.Errorf("unknown key dropped: kv = %v", kv)
	}
	if _, ok := kv["Author"]; ok {
		t.Errorf("cleared Author still present: kv = %v", kv)
	}
}

func TestUpdateMetadataUnchangedTitleDoesNotRename(t *testing.T) {
	// File name and comment Title disagree, but the form is saved with the Title
	// left at its current value: metadata updates, the file does not move.
	a, dir, id := newMetaEditApp(t, "zz-file.cbz", "Title=Long Display Name\nAuthor=A")

	_, err := a.UpdateComicMetadata(id, model.MetaEdit{Title: "Long Display Name", Author: "B"})
	if err != nil {
		t.Fatalf("UpdateComicMetadata: %v", err)
	}
	if p, _ := a.store.PathFromID(id); p != filepath.Join(dir, "zz-file.cbz") {
		t.Errorf("path moved on an unchanged title: %q", p)
	}
	if _, err := os.Stat(filepath.Join(dir, "Long Display Name.cbz")); !os.IsNotExist(err) {
		t.Error("a renamed file should not exist")
	}
}

func TestUpdateMetadataIllegalTitleSkipsRename(t *testing.T) {
	a, dir, id := newMetaEditApp(t, "Boring.cbz", "Title=Boring")

	// A title made of only illegal characters has no safe file name → metadata
	// is updated but the file is left alone.
	_, err := a.UpdateComicMetadata(id, model.MetaEdit{Title: `???:::`})
	if err != nil {
		t.Fatalf("UpdateComicMetadata: %v", err)
	}
	if p, _ := a.store.PathFromID(id); p != filepath.Join(dir, "Boring.cbz") {
		t.Errorf("path changed despite unusable title: %q", p)
	}
	kv := meta.Parse(readComment(t, filepath.Join(dir, "Boring.cbz")))
	if kv["Title"] != `???:::` {
		t.Errorf("title comment = %q, want the raw title kept", kv["Title"])
	}
}
