package comic

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type testEntry struct {
	name  string
	data  string
	store bool // use the Store method instead of Deflate
}

// writeTestCbz writes a zip with the given comment and entries (mix of Store
// and Deflate), in order.
func writeTestCbz(t *testing.T, path, comment string, entries []testEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	if err := zw.SetComment(comment); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		method := zip.Deflate
		if e.store {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: method})
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
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// entrySnapshot records the names, methods and raw content of an archive so a
// test can assert a rewrite left every entry byte-for-byte identical.
type entrySnapshot struct {
	name   string
	method uint16
	raw    []byte
}

func snapshotEntries(t *testing.T, path string) []entrySnapshot {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var out []entrySnapshot
	for _, f := range zr.File {
		r, err := f.OpenRaw()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, entrySnapshot{name: f.Name, method: f.Method, raw: b})
	}
	return out
}

func TestRewriteCommentChangesComment(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "vol.cbz")
	writeTestCbz(t, src, "Title=Old\nAuthor=Prev", []testEntry{
		{name: "readme.txt", data: "some notes here that deflate nicely", store: false},
		{name: "p10.jpg", data: "page-ten-" + string(make([]byte, 2048)), store: false},
		{name: "p1.png", data: "page-one", store: true},
		{name: "p2.jpg", data: "page-two " + string(make([]byte, 4096)), store: false},
	})
	before := snapshotEntries(t, src)

	want := "Title=蕾佳是華麗的\nAuthor=ぐすたふ\nGenre=comedy, 中文\nSource=magnet:?xt=urn:btih:abc&dn=x"
	if err := RewriteComment(src, want); err != nil {
		t.Fatalf("RewriteComment: %v", err)
	}

	if _, err := os.Stat(src + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file left behind after rewrite")
	}

	ar, err := Open(src)
	if err != nil {
		t.Fatalf("Open after rewrite: %v", err)
	}
	if got := ar.Comment(); got != want {
		t.Errorf("comment = %q, want %q", got, want)
	}
	if ar.PageCount() != 3 {
		t.Errorf("page count = %d, want 3", ar.PageCount())
	}
	ar.Close()

	// Every entry must be raw-identical (names, methods, compressed bytes) —
	// proof the rewrite copied members instead of recompressing them.
	after := snapshotEntries(t, src)
	if len(after) != len(before) {
		t.Fatalf("entries = %d, want %d", len(after), len(before))
	}
	for i := range before {
		if after[i].name != before[i].name || after[i].method != before[i].method {
			t.Errorf("entry %d: got (%q, method %d), want (%q, method %d)",
				i, after[i].name, after[i].method, before[i].name, before[i].method)
		}
		if string(after[i].raw) != string(before[i].raw) {
			t.Errorf("entry %d %q content changed after rewrite", i, before[i].name)
		}
	}
}

func TestRewriteCommentClearsComment(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.cbz")
	writeTestCbz(t, src, "Title=Old", []testEntry{{name: "p1.jpg", data: "x", store: true}})

	if err := RewriteComment(src, ""); err != nil {
		t.Fatalf("RewriteComment: %v", err)
	}
	ar, err := Open(src)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ar.Close()
	if ar.Comment() != "" {
		t.Errorf("comment = %q, want empty", ar.Comment())
	}
}

func TestRewriteCommentOversizedLeavesOriginalUntouched(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.cbz")
	const orig = "Title=Keep me"
	writeTestCbz(t, src, orig, []testEntry{{name: "p1.jpg", data: "page", store: true}})

	huge := make([]byte, maxCommentLen+10)
	if err := RewriteComment(src, string(huge)); err == nil {
		t.Fatal("expected too-long error")
	}
	if _, err := os.Stat(src + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file left behind after failed rewrite")
	}
	ar, err := Open(src)
	if err != nil {
		t.Fatalf("Open after failed rewrite: %v", err)
	}
	defer ar.Close()
	if ar.Comment() != orig {
		t.Errorf("comment = %q, want %q (original must survive)", ar.Comment(), orig)
	}
	if ar.PageCount() != 1 {
		t.Errorf("page count = %d, want 1", ar.PageCount())
	}
}

func TestRewriteCommentMissingFile(t *testing.T) {
	if err := RewriteComment(filepath.Join(t.TempDir(), "nope.cbz"), "Title=X"); err == nil {
		t.Fatal("expected error for missing file")
	}
}
