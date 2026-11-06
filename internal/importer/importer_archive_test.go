package importer

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unicode/utf8"

	"comicgo/internal/importer/archive"
)

// ---- fixture builders ----

// writeZip creates a zip at path with the given (name -> data) entries in
func TestDesiredName(t *testing.T) {
	cases := []struct {
		source string
		isDir  bool
		want   string
	}{
		{"C:\\comics\\Vol.1.cbr", false, "Vol.1.zip"},
		{"C:\\comics\\x.tar.gz", false, "x.zip"},
		{"C:\\comics\\y.tar", false, "y.zip"},
		{"C:\\comics\\z.tgz", false, "z.zip"},
		{"C:\\comics\\a.cbz", false, "a.zip"},
		{"C:\\comics\\b.zip", false, "b.zip"},
		{"C:\\comics\\c.7z", false, "c.zip"},
		{"C:\\comics\\Some Folder", true, "Some Folder.zip"},
		{"C:\\comics\\noext", false, "noext.zip"},
	}
	for _, c := range cases {
		if got := DesiredName(c.source, c.isDir); got != c.want {
			t.Errorf("DesiredName(%q, %v) = %q, want %q", c.source, c.isDir, got, c.want)
		}
	}
}

func TestUniqueDest(t *testing.T) {
	dir := t.TempDir()
	// Not taken yet: returns the requested name verbatim.
	first := UniqueDest(dir, "Vol 1.zip")
	if filepath.Base(first) != "Vol 1.zip" {
		t.Fatalf("fresh dest = %q", first)
	}
	// Case-insensitive collision: "vol 1.zip" already exists.
	low := filepath.Join(dir, "vol 1.zip")
	if err := os.WriteFile(low, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := UniqueDest(dir, "Vol 1.zip")
	if filepath.Base(second) != "Vol 1 (2).zip" {
		t.Fatalf("collision dest = %q, want Vol 1 (2).zip", second)
	}
	// And a further collision steps to (3).
	if err := os.WriteFile(second, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	third := UniqueDest(dir, "Vol 1.zip")
	if filepath.Base(third) != "Vol 1 (3).zip" {
		t.Fatalf("second collision dest = %q, want Vol 1 (3).zip", third)
	}
}

func TestEntryName(t *testing.T) {
	if got := archive.EntryName(1, ".JPG"); got != "000001.jpg" {
		t.Errorf("EntryName(1,.JPG) = %q", got)
	}
	if got := archive.EntryName(12, ".png"); got != "000012.png" {
		t.Errorf("EntryName(12,.png) = %q", got)
	}
}

func TestNoImageSourceFailsAndCleansUp(t *testing.T) {
	dir := t.TempDir()

	folder := filepath.Join(dir, "texts")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	d1 := filepath.Join(dir, "empty-folder.zip")
	if _, err := Import(context.Background(), Options{Source: folder, Dest: d1}); err == nil {
		t.Fatal("expected error for image-less folder")
	}
	if _, err := os.Stat(d1); !os.IsNotExist(err) {
		t.Fatalf("partial dest left behind: %v", err)
	}

	zipPath := filepath.Join(dir, "noimg.cbz")
	writeZip(t, zipPath, []struct {
		name string
		data string
	}{{"readme.txt", "x"}})
	d2 := filepath.Join(dir, "noimg-out.zip")
	if _, err := Import(context.Background(), Options{Source: zipPath, Dest: d2}); err == nil {
		t.Fatal("expected error for image-less zip")
	}
	if _, err := os.Stat(d2); !os.IsNotExist(err) {
		t.Fatalf("partial dest left behind: %v", err)
	}
}

func TestUnsupportedSource(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(context.Background(), Options{Source: bad, Dest: filepath.Join(dir, "o.zip")}); err == nil {
		t.Fatal("expected unsupported-source error")
	}
}

func TestOutputStoreMethodAndUTF8(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.cbz")
	writeZip(t, src, scrambledEntries())
	dest := filepath.Join(dir, "out.zip")
	if _, err := Import(context.Background(), Options{Source: src, Dest: dest}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 3 {
		t.Fatalf("entries = %d, want 3", len(zr.File))
	}
	for _, f := range zr.File {
		if f.Method != zip.Store {
			t.Errorf("entry %s method = %d, want Store(%d)", f.Name, f.Method, zip.Store)
		}
		if !utf8.ValidString(f.Name) {
			t.Errorf("entry name %q not valid UTF-8", f.Name)
		}
	}
}

func TestProgressEvents(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.cbz")
	writeZip(t, src, scrambledEntries())
	var d []int
	_, err := Import(context.Background(), Options{
		Source: src,
		Dest:   filepath.Join(dir, "out.zip"),
		OnProgress: func(done, total int) {
			d = append(d, done)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, []int{1, 2, 3}) {
		t.Errorf("progress = %v, want [1 2 3]", d)
	}
}
