package scan

import (
	"archive/zip"
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"comicgo/internal/cache"
	"comicgo/internal/imgutil"
	"comicgo/internal/index"
)

// makeComic writes a zip containing a few png pages + a key=value comment.
func makeComic(t *testing.T, path, title string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	zw.SetComment("Title=" + title)
	for _, name := range []string{"001.png", "002.png", "003.png"} {
		img := image.NewRGBA(image.Rect(0, 0, 200, 200))
		img.Set(0, 0, color.White)
		b, err := imgutil.EncodePNG(img)
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func runOnce(t *testing.T, st *index.Store, thumbs *cache.ThumbStore, lib string, force bool) []error {
	t.Helper()
	var errs []error
	opts := Options{
		Store:  st,
		Thumbs: thumbs,
		Roots:  []string{lib},
		Force:  force,
		OnError: func(p string, e error) {
			errs = append(errs, e)
		},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	return errs
}

func TestScanAddSkipPrune(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib")
	_ = os.MkdirAll(lib, 0o755)

	makeComic(t, filepath.Join(lib, "a.cbz"), "Alpha")
	makeComic(t, filepath.Join(lib, "b.cbz"), "Beta")

	st, err := index.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	thumbs := cache.NewThumbStore(filepath.Join(dir, "thumbs"))

	if errs := runOnce(t, st, thumbs, lib, false); len(errs) != 0 {
		t.Fatalf("first scan errors: %v", errs)
	}
	rows, _ := st.AllRows()
	if len(rows) != 2 {
		t.Fatalf("after first scan rows = %d, want 2", len(rows))
	}
	if !thumbs.Has(rows[0].ID) || !thumbs.Has(rows[1].ID) {
		t.Error("thumbnails were not written")
	}
	if rows[0].KV["Title"] == "" {
		t.Error("comment metadata not parsed")
	}
	if rows[0].PageCount != 3 {
		t.Errorf("pageCount = %d, want 3", rows[0].PageCount)
	}

	// add a comic + rescan (a and b unchanged -> skipped by fingerprint)
	makeComic(t, filepath.Join(lib, "c.cbz"), "Gamma")
	if errs := runOnce(t, st, thumbs, lib, false); len(errs) != 0 {
		t.Fatalf("second scan errors: %v", errs)
	}
	rows, _ = st.AllRows()
	if len(rows) != 3 {
		t.Fatalf("after add rows = %d, want 3", len(rows))
	}

	// remove a comic + rescan (prune)
	if err := os.Remove(filepath.Join(lib, "b.cbz")); err != nil {
		t.Fatal(err)
	}
	if errs := runOnce(t, st, thumbs, lib, false); len(errs) != 0 {
		t.Fatalf("third scan errors: %v", errs)
	}
	rows, _ = st.AllRows()
	if len(rows) != 2 {
		t.Fatalf("after prune rows = %d, want 2", len(rows))
	}
}

func TestScanNonComicZipIgnored(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib")
	_ = os.MkdirAll(lib, 0o755)
	makeComic(t, filepath.Join(lib, "a.cbz"), "Alpha")
	// a zip that is not a comic
	f, _ := os.Create(filepath.Join(lib, "junk.zip"))
	f.Write([]byte("PK\x03\x04 not a real archive of images"))
	f.Close()

	st, err := index.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	thumbs := cache.NewThumbStore(filepath.Join(dir, "thumbs"))
	if errs := runOnce(t, st, thumbs, lib, false); len(errs) != 0 {
		t.Fatalf("scan errors: %v", errs)
	}
	rows, _ := st.AllRows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
}

func TestScanCancelledNoDeadlock(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib")
	_ = os.MkdirAll(lib, 0o755)
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		makeComic(t, filepath.Join(lib, name+".cbz"), "X")
	}
	st, err := index.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	thumbs := cache.NewThumbStore(filepath.Join(dir, "thumbs"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before any work
	if err := Run(ctx, Options{Store: st, Thumbs: thumbs, Roots: []string{lib}}); err == nil {
		t.Error("expected cancellation error")
	}
	rows, _ := st.AllRows()
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}
