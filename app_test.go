package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"comicgo/internal/cache"
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

func TestSearchMatching(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	st, err := index.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	rows := []index.Record{
		{Path: `C:\s\a.cbz`, Dir: `C:\s`, Filename: "a.cbz", FileSize: 1, MtimeNs: 1,
			PageCount: 2, KV: map[string]string{"Title": "Alpha Space"}},
		{Path: `C:\s\z.cbz`, Dir: `C:\s`, Filename: "zeta.zip", FileSize: 2, MtimeNs: 2,
			PageCount: 3, KV: map[string]string{}},
		{Path: `C:\s\t.cbz`, Dir: `C:\s`, Filename: "t.cbz", FileSize: 3, MtimeNs: 3,
			PageCount: 1, KV: map[string]string{"Author": "Tanaka Yoshi"}},
	}
	for _, r := range rows {
		if _, err := st.CommitComic(r); err != nil {
			t.Fatal(err)
		}
	}
	a := NewApp()
	a.store = st

	cases := map[string]int{
		"alpha":  1, // Title value
		"space":  1, // Title value substring
		"zeta":   1, // filename (title fallback strips ext, filename still matches)
		"tanaka": 1, // metadata value
		"title":  0, // metadata key, not user-visible content
		"nope":   0,
		"":       3, // empty search = all
	}
	for q, want := range cases {
		if got := a.CountMatching(q, ""); got != want {
			t.Errorf("CountMatching(%q) = %d, want %d", q, got, want)
		}
	}
	// GetComics honors search too
	cards, err := a.GetComics(model.PageQuery{Limit: 10, Search: "tanaka"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].Filename != "t.cbz" {
		t.Errorf("search results = %+v", cards)
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

func TestParseSearchTerms(t *testing.T) {
	tests := []struct {
		in   string
		text []string
		tags []string
	}{
		{in: "", text: nil, tags: nil},
		{in: "alpha  beta", text: []string{"alpha", "beta"}, tags: nil},
		{in: `tag:ロリ`, text: nil, tags: []string{"ロリ"}},
		{in: `tag:"school girl" tanaka`, text: []string{"tanaka"}, tags: []string{"school girl"}},
		{in: `tag:"a b" tag:"c" x`, text: []string{"x"}, tags: []string{"a b", "c"}},
		{in: `"multi word" x`, text: []string{"multi word", "x"}, tags: nil},
		{in: `   `, text: nil, tags: nil},
	}
	for _, tt := range tests {
		q := parseSearch(tt.in)
		if !eqStrings(q.text, tt.text) || !eqStrings(q.tags, tt.tags) {
			t.Errorf("parseSearch(%q) = {text:%v tags:%v}, want {text:%v tags:%v}",
				tt.in, q.text, q.tags, tt.text, tt.tags)
		}
	}
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSearchTagTerms(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	st, err := index.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	rows := []index.Record{
		{Path: `C:\s\a.cbz`, Dir: `C:\s`, Filename: "a.cbz", FileSize: 1, MtimeNs: 1,
			PageCount: 2, KV: map[string]string{"Genre": "ロリ, 無修正"}},
		{Path: `C:\s\b.cbz`, Dir: `C:\s`, Filename: "b.cbz", FileSize: 2, MtimeNs: 2,
			PageCount: 2, KV: map[string]string{"Genre": "ロリコン, 無修正"}},
		{Path: `C:\s\c.cbz`, Dir: `C:\s`, Filename: "c.cbz", FileSize: 3, MtimeNs: 3,
			PageCount: 2, KV: map[string]string{"Genre": "ロリ巨乳, 学生"}},
		{Path: `C:\s\d.cbz`, Dir: `C:\s`, Filename: "d.cbz", FileSize: 4, MtimeNs: 4,
			PageCount: 2, KV: map[string]string{"Genre": "ロリ, 無修正", "Author": "Tanaka"}},
		{Path: `C:\s\e.cbz`, Dir: `C:\s`, Filename: "e.cbz", FileSize: 5, MtimeNs: 5,
			PageCount: 2, KV: map[string]string{"Genre": "school girl"}},
	}
	for _, r := range rows {
		if _, err := st.CommitComic(r); err != nil {
			t.Fatal(err)
		}
	}
	a := NewApp()
	a.store = st

	cases := map[string]int{
		"tag:ロリ":            2, // a, d — exact tag, NOT b (ロリコン) or c (ロリ巨乳)
		"tag:ロリ tag:無修正":    2, // a, d
		"tag:ロリ tag:学生":     0, // no comic has both
		`tag:"school girl"`: 1, // e — quoted tag with a space
		"tag:ロリコン":          1, // b
		"ロリ":                4, // plain text still substrings ロリコン/ロリ巨乳
		"ロリ tag:学生":         1, // c: text ロリ (in ロリ巨乳) AND tag 学生
		"tanaka tag:ロリ":     1, // d: text term in Author value AND tag
	}
	for q, want := range cases {
		if got := a.CountMatching(q, ""); got != want {
			t.Errorf("CountMatching(%q) = %d, want %d", q, got, want)
		}
	}

	// the same query drives the paged list
	cards, err := a.GetComics(model.PageQuery{Limit: 10, Search: "tag:ロリ tag:無修正"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 {
		t.Fatalf("GetComics(tags) = %d cards, want 2", len(cards))
	}
}

func TestCardTags(t *testing.T) {
	tests := []struct {
		name  string
		genre string
		want  []string
	}{
		{name: "empty", genre: "", want: nil},
		{name: "splits and trims", genre: "ロリ, 無修正, 中国翻訳", want: []string{"ロリ", "無修正", "中国翻訳"}},
		{name: "dedupes", genre: "ロリ, ロリ, 中出", want: []string{"ロリ", "中出"}},
		{name: "caps at six", genre: "a, b, c, d, e, f, g, h", want: []string{"a", "b", "c", "d", "e", "f"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model.Meta{KV: map[string]string{"Genre": tt.genre}}
			got := cardTags(m)
			if len(got) != len(tt.want) {
				t.Fatalf("cardTags = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("cardTags = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
