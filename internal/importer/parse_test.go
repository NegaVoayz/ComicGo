package importer

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"comicgo/internal/comic"
)

func TestParseName(t *testing.T) {
	cases := []struct {
		in     string
		title  string
		author string
		tags   []string
	}{
		// Square brackets (author) + parentheses (tag) — the real downloader shape.
		{"[ぐすたふ] 蕾佳是華麗的 我的俏女傭 レイカは華麗な僕のメイド (中文)",
			"蕾佳是華麗的 我的俏女傭 レイカは華麗な僕のメイド", "ぐすたふ", []string{"中文"}},
		{"[Artist] Title [Chinese] [Digital]", "Title", "Artist", []string{"Chinese", "Digital"}},
		{"[Artist] Title", "Title", "Artist", nil},
		{"[Artist]", "", "Artist", nil},
		// Braces still work; groups of different delimiters may be mixed.
		{"{Alice}{Bob} The Adventure {fantasy}{comedy}", "The Adventure", "Alice Bob", []string{"fantasy", "comedy"}},
		{"{A}{B}NoSpaceTitle{T1}{T2}", "NoSpaceTitle", "A B", []string{"T1", "T2"}},
		{"{甲斐谷忍} 新宿スワン {おねショタ, 都市}", "新宿スワン", "甲斐谷忍", []string{"おねショタ", "都市"}},
		{"  [C]   Spaced   Out   (t1) {t2}  ", "Spaced Out", "C", []string{"t1", "t2"}},
		// Stray text after a tag stays in the title.
		{"[Circle] Title (drama) tail", "Title tail", "Circle", []string{"drama"}},
		// Names that do NOT start with an author group are not treated as
		// bracketed: parentheses that are part of a plain title must survive.
		{"NoBraces At All", "", "", nil},
		{"My Comic (Vol 3)", "", "", nil},
		{"蕾佳是華麗的 (中文)", "", "", nil},
		{"", "", "", nil},
	}
	for _, c := range cases {
		title, author, tags := ParseName(c.in)
		if title != c.title || author != c.author || !reflect.DeepEqual(tags, c.tags) {
			t.Errorf("ParseName(%q)\n  title  = %q (want %q)\n  author = %q (want %q)\n  tags   = %v (want %v)",
				c.in, title, c.title, author, c.author, tags, c.tags)
		}
	}
}

func TestBuildComment(t *testing.T) {
	cases := []struct {
		title, author string
		tags          []string
		want          string
	}{
		{"The Adventure", "Alice Bob", []string{"fantasy", "comedy"}, "Title=The Adventure\nAuthor=Alice Bob\nGenre=fantasy, comedy"},
		{"Title", "", nil, "Title=Title"},
		{"", "", nil, ""},
		{"Title", "Author", nil, "Title=Title\nAuthor=Author"},
		// case-insensitive tag dedupe, comma inside a group splits
		{"T", "A", []string{"Foo", "foo", "Bar"}, "Title=T\nAuthor=A\nGenre=Foo, Bar"},
	}
	for _, c := range cases {
		if got := BuildComment(c.title, c.author, c.tags); got != c.want {
			t.Errorf("BuildComment(%q,%q,%v) = %q, want %q", c.title, c.author, c.tags, got, c.want)
		}
	}
}

// A bracket-styled name parsed for metadata should produce a .cbz named after
// the clean title and whose comment round-trips through comic.Open.
func TestImportWithComment(t *testing.T) {
	dir := t.TempDir()
	// Use a real-style name ending in .zip to also exercise suffix stripping.
	folder := filepath.Join(dir, "[Alice] The Adventure (fantasy) (comedy).zip")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "p1.jpg"), []byte("page-1"), 0o644); err != nil {
		t.Fatal(err)
	}

	stem := DesiredStem(folder, true) // folder import: name kept as-is, no .zip strip
	title, author, tags := ParseName(stem)
	comment := BuildComment(title, author, tags)
	wantName := title + ".cbz"

	dest := filepath.Join(dir, wantName)
	if _, err := Import(context.Background(), Options{Source: folder, Dest: dest, Comment: comment}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	ar, err := comic.Open(dest)
	if err != nil {
		t.Fatalf("comic.Open: %v", err)
	}
	defer ar.Close()
	if ar.Comment() != comment {
		t.Errorf("comment = %q, want %q", ar.Comment(), comment)
	}
	if ar.PageCount() != 1 {
		t.Errorf("page count = %d, want 1", ar.PageCount())
	}
}
