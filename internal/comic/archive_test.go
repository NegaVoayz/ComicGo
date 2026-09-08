package comic

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"page1.jpg", "page2.jpg", true},
		{"page2.jpg", "page10.jpg", true}, // numeric, not lexicographic
		{"page10.jpg", "page9.jpg", false},
		{"1.jpg", "01.jpg", false}, // "1" == "01", keep stable -> not less
		{"page 1", "page 1a", true},
		{"a.jpg", "b.jpg", true},
		{"B.jpg", "a.jpg", false}, // case-insensitive: 'b' > 'a', so B.jpg is NOT less than a.jpg
		{"a.jpg", "B.jpg", true},  // case-insensitive compare: 'a' < 'b'
		{"page1", "page1", false},
		{"", "a", true},
		{"chapter10.jpg", "chapter2.jpg", false},
		{"1.jpg", "10.jpg", true},
	}
	for _, c := range cases {
		if got := NaturalLess(c.a, c.b); got != c.want {
			t.Errorf("NaturalLess(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestNaturalSortOrder(t *testing.T) {
	in := []string{"p10.jpg", "p2.jpg", "p1.jpg", "p1b.jpg"}
	sort.SliceStable(in, func(i, j int) bool { return NaturalLess(in[i], in[j]) })
	want := []string{"p1.jpg", "p1b.jpg", "p2.jpg", "p10.jpg"}
	if !reflect.DeepEqual(in, want) {
		t.Errorf("sorted = %v, want %v", in, want)
	}
}

// writeTestZip builds a zip at path with the given entry names (in order).
func writeTestZip(t *testing.T, path string, names []string, comment string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	zw.SetComment(comment)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("fakeimagebytes"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenPageMapNaturalOrder(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.cbz")
	// deliberately out of order in the central directory
	writeTestZip(t, p, []string{"p10.jpg", "folder/p2.png", "p1.jpg", "readme.txt", "p1b.webp"}, "Title=Demo")

	a, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	if a.PageCount() != 4 {
		t.Fatalf("PageCount = %d, want 4 (txt excluded)", a.PageCount())
	}
	got := make([]string, 0, 4)
	for i := 0; i < a.PageCount(); i++ {
		fe, err := a.PageEntry(i)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, fe.Name)
	}
	want := []string{"folder/p2.png", "p1.jpg", "p1b.webp", "p10.jpg"}
	// natural sort compares whole names including the folder prefix
	if !reflect.DeepEqual(got, want) {
		t.Errorf("page order = %v, want %v", got, want)
	}
	if a.CoverName() != want[0] {
		t.Errorf("cover = %q, want first image %q", a.CoverName(), want[0])
	}
}

func TestOpenEmptyArchive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.zip")
	writeTestZip(t, p, []string{"notes.txt"}, "")
	if _, err := Open(p); !errors.Is(err, ErrNotAComic) {
		t.Errorf("Open(no images) error = %v, want ErrNotAComic", err)
	}
}
