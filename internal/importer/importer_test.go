package importer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unicode/utf8"

	"comicgo/internal/comic"
)

// ---- fixture builders ----

// writeZip creates a zip at path with the given (name -> data) entries in
// creation order.
func writeZip(t *testing.T, path string, entries []struct {
	name string
	data string
}) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// writeTar creates a tar (gzipped when gz) with the given entries in order.
func writeTar(t *testing.T, path string, gz bool, entries []struct {
	name string
	data string
}) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var w io.Writer = f
	if gz {
		gw := gzip.NewWriter(f)
		defer gw.Close()
		w = gw
	}
	tw := tar.NewWriter(w)
	for _, e := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
}

// pageData returns the decoded content of page n (0-based) of a produced archive.
func pageData(t *testing.T, cbz string, n int) string {
	t.Helper()
	ar, err := comic.Open(cbz)
	if err != nil {
		t.Fatal(err)
	}
	defer ar.Close()
	if n >= ar.PageCount() {
		t.Fatalf("page %d out of range (count %d)", n, ar.PageCount())
	}
	rc, err := ar.OpenPage(n)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ---- tests ----

// naturalPageNames is the natural order (by entry name) the source pages are
// expected to rank in, ignoring the readme. Creation order in the zip is the
// reverse, proving the importer re-orders rather than trusting archive order.
var naturalPageNames = []string{"p1.jpg", "p2.png", "p10.jpg"}
var naturalPageData = []string{"page-1", "page-2", "page-10"}

func scrambledEntries() []struct {
	name string
	data string
} {
	return []struct {
		name string
		data string
	}{
		{"readme.txt", "ignore-me"},
		{"p10.jpg", "page-10"},
		{"p1.jpg", "page-1"},
		{"p2.png", "page-2"},
	}
}

func TestZipRoundtrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.cbz")
	writeZip(t, src, scrambledEntries())

	dest := filepath.Join(dir, "out.zip")
	n, err := Import(context.Background(), Options{Source: src, Dest: dest})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n != 3 {
		t.Fatalf("page count = %d, want 3", n)
	}
	ar, err := comic.Open(dest)
	if err != nil {
		t.Fatalf("comic.Open(dest): %v", err)
	}
	if ar.PageCount() != 3 {
		t.Fatalf("reader page count = %d, want 3", ar.PageCount())
	}
	ar.Close()
	for i, want := range naturalPageData {
		if got := pageData(t, dest, i); got != want {
			t.Errorf("page %d content = %q, want %q (%s)", i, got, want, naturalPageNames[i])
		}
	}
}

func TestFolderImport(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "album")
	if err := os.MkdirAll(filepath.Join(folder, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"notes.txt":      "junk",
		"img10.jpg":      "page-10",
		"img2.jpg":       "page-2",
		"img1.jpg":       "page-1",
		"sub/nested.png": "page-nested",
		"img1.txt":       "not an image",
	}
	for rel, data := range files {
		p := filepath.Join(folder, rel)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dest := filepath.Join(dir, "album.zip")
	if _, err := Import(context.Background(), Options{Source: folder, Dest: dest}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	// Natural order of relative paths: img1, img2, img10, sub/nested.
	want := []string{"page-1", "page-2", "page-10", "page-nested"}
	got := []string{}
	for i := range want {
		got = append(got, pageData(t, dest, i))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pages = %v, want %v", got, want)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("dest missing after import: %v", err)
	}
}

func TestTarRoundtrips(t *testing.T) {
	for _, gz := range []bool{false, true} {
		name := "tar"
		if gz {
			name = "tar.gz"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			// Reverse the natural order on purpose: archive order must not
			// decide the page order. (Start from all entries minus readme.)
			var rev []struct {
				name string
				data string
			}
			for _, e := range scrambledEntries() {
				if e.name == "readme.txt" {
					continue
				}
				rev = append(rev, e)
			}
			for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
				rev[i], rev[j] = rev[j], rev[i]
			}
			var src string
			if gz {
				src = filepath.Join(dir, "src.tar.gz")
			} else {
				src = filepath.Join(dir, "src.tar")
			}
			writeTar(t, src, gz, rev)
			dest := filepath.Join(dir, "out.zip")
			if _, err := Import(context.Background(), Options{Source: src, Dest: dest}); err != nil {
				t.Fatalf("Import: %v", err)
			}
			for i, want := range naturalPageData {
				if got := pageData(t, dest, i); got != want {
					t.Errorf("page %d content = %q, want %q", i, got, want)
				}
			}
		})
	}
}

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
	if got := entryName(1, ".JPG"); got != "000001.jpg" {
		t.Errorf("entryName(1,.JPG) = %q", got)
	}
	if got := entryName(12, ".png"); got != "000012.png" {
		t.Errorf("entryName(12,.png) = %q", got)
	}
}

func TestRankQueuesDuplicates(t *testing.T) {
	q := rankQueues([]string{"b.jpg", "a.jpg", "a.jpg"})
	if got := q["a.jpg"].pop(); got != 1 {
		t.Errorf("first a.jpg rank = %d, want 1", got)
	}
	if got := q["a.jpg"].pop(); got != 2 {
		t.Errorf("second a.jpg rank = %d, want 2", got)
	}
	if got := q["b.jpg"].pop(); got != 3 {
		t.Errorf("b.jpg rank = %d, want 3", got)
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
