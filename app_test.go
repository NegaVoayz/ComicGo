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
