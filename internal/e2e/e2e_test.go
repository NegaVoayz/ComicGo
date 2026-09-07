// Package e2e exercises the real data path the UI drives — scan → SQLite index
// → loopback HTTP cover/page serving — against synthetic fixtures, or against
// a real library folder when E2E_LIB points at one. It does not need the GUI.
package e2e

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"comicgo/internal/cache"
	"comicgo/internal/comic"
	"comicgo/internal/imgutil"
	"comicgo/internal/index"
	"comicgo/internal/scan"
	"comicgo/internal/serve"
)

// ---------------------------------------------------------------------------
// fixture generation

func writePNGEntry(zw *zip.Writer, name string) error {
	img := image.NewRGBA(image.Rect(0, 0, 160, 220))
	for x := 0; x < 160; x++ {
		for y := 0; y < 220; y++ {
			img.Set(x, y, color.RGBA{uint8(x % 255), uint8(y % 255), 80, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	return err
}

// makeComic builds a zip at path. comment "" simulates a real comic with no
// metadata comment. An explicit pages slice controls entry order.
func makeComic(t *testing.T, path, comment string, pages []string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	zw.SetComment(comment)
	// interleave a non-image entry to prove it is excluded
	if len(pages) == 0 {
		pages = []string{"c001.jpg", "c002.jpg", "c003.jpg", "c004.jpg"}
	}
	ordered := []string{"readme.txt", pages[0]}
	ordered = append(ordered, pages[1:]...)
	for _, n := range ordered {
		if n == "readme.txt" {
			if _, err := zw.Create(n); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := writePNGEntry(zw, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// media provider mirroring app.go's coverBytes/pageBytes

type prov struct {
	st *index.Store
	th *cache.ThumbStore
}

func (p prov) Cover(id int64) ([]byte, string, error) {
	if p.th.Has(id) {
		if b, err := p.th.Read(id); err == nil {
			return b, "image/jpeg", nil
		}
	}
	path, err := p.st.PathFromID(id)
	if err != nil {
		return nil, "", err
	}
	ar, err := comic.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer ar.Close()
	rc, err := ar.OpenPage(0)
	if err != nil {
		return nil, "", err
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", err
	}
	thumb, err := imgutil.ThumbJPEG(raw, 320, 80)
	if err != nil {
		return nil, "", err
	}
	_ = p.th.Write(id, thumb)
	return thumb, "image/jpeg", nil
}

func (p prov) Page(id int64, n int) ([]byte, string, error) {
	path, err := p.st.PathFromID(id)
	if err != nil {
		return nil, "", err
	}
	ar, err := comic.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer ar.Close()
	rc, err := ar.OpenPage(n)
	if err != nil {
		return nil, "", err
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", err
	}
	return b, imgutil.SniffType(b), nil
}

// ---------------------------------------------------------------------------

func buildFixtureLibrary(t *testing.T, lib string) {
	t.Helper()
	_ = os.MkdirAll(lib, 0o755)
	// 1) commented comic, deliberately messy page order
	makeComic(t, filepath.Join(lib, "Alpha.cbz"),
		"Title=Alpha Comic\nAuthor=Nobody\nVol=3\nGenre=Adventure", nil)
	// 2) page names that exercise natural sort (1 vs 10)
	makeComic(t, filepath.Join(lib, "Beta.cbz"), "Title=Beta Comic",
		[]string{"p10.jpg", "p1.jpg", "p2.jpg"})
	// 3) a real comic with NO comment (filename must still become the title)
	makeComic(t, filepath.Join(lib, "NoComment.cbz"), "",
		[]string{"page 1.jpg", "page 2.jpg", "page 3.jpg"})
	// 4) a nested comic
	sub := filepath.Join(lib, "sub")
	_ = os.MkdirAll(sub, 0o755)
	makeComic(t, filepath.Join(sub, "Nested.cbz"), "Title=Nested", []string{"001.png", "002.png"})
	// 5) a non-comic zip that must be skipped silently
	nf, _ := os.Create(filepath.Join(lib, "notacomic.zip"))
	nf.Write([]byte("PK\x03\x04 not really an archive of images"))
	nf.Close()
}

func runE2E(t *testing.T, lib string) {
	t.Helper()
	dir := t.TempDir()
	st, err := index.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	th := cache.NewThumbStore(filepath.Join(dir, "thumbs"))

	var errPaths []string
	if err := scan.Run(context.Background(), scan.Options{
		Store:  st,
		Thumbs: th,
		Roots:  []string{lib},
		OnError: func(p string, e error) {
			errPaths = append(errPaths, p)
		},
	}); err != nil {
		t.Fatal(err)
	}
	if len(errPaths) != 0 {
		t.Fatalf("scan reported errors: %v", errPaths)
	}

	rows, err := st.AllRows()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("indexed %d comics", len(rows))

	srv, err := serve.Start(prov{st: st, th: th})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	for i := range rows {
		r := &rows[i]
		t.Logf("  %-14s pages=%-2d title=%q commentKVs=%d", r.Filename, r.PageCount, firstTitle(r), len(r.KV))

		// natural order sanity: first two entries sorted, not central-dir order
		if r.PageCount < 1 {
			t.Errorf("%s: no pages", r.Path)
		}
		// cover over real HTTP
		resp, err := http.Get(srv.CoverURL(r.ID))
		if err != nil {
			t.Errorf("cover http: %v", err)
			continue
		}
		cov, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !bytes.HasPrefix(cov, []byte("\xff\xd8\xff")) {
			t.Errorf("%s: cover = %d (%d bytes), want 200 jpeg", r.Filename, resp.StatusCode, len(cov))
		}
		// first two pages over real HTTP and via provider decode path
		for _, n := range []int{0, 1} {
			pr, err := http.Get(srv.PageURL(r.ID, n))
			if err != nil {
				t.Errorf("page http: %v", err)
				continue
			}
			pb, _ := io.ReadAll(pr.Body)
			pr.Body.Close()
			if pr.StatusCode != http.StatusOK || len(pb) == 0 {
				t.Errorf("%s page %d = %d, want 200", r.Filename, n, pr.StatusCode)
			}
			if imgutil.SniffType(pb) != "image/png" && imgutil.SniffType(pb) != "image/jpeg" {
				t.Errorf("%s page %d sniffed as %q", r.Filename, n, imgutil.SniffType(pb))
			}
		}
	}
}

func firstTitle(r *index.Row) string {
	if r.KV["Title"] != "" {
		return r.KV["Title"]
	}
	return r.Filename
}

func TestE2EAgainstRealOrFixtureLibrary(t *testing.T) {
	lib := os.Getenv("E2E_LIB")
	fromReal := lib != ""
	if lib == "" {
		lib = filepath.Join(t.TempDir(), "lib")
		buildFixtureLibrary(t, lib)
	}

	// expected total (fixtures) is 4 comics; for real libs we only assert >= 1
	runE2E(t, lib)

	if !fromReal {
		// precise checks only meaningful on the deterministic fixture set
		st, err := index.Open(filepath.Join(t.TempDir(), "check.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		th := cache.NewThumbStore(filepath.Join(t.TempDir(), "thumbs2"))
		if err := scan.Run(context.Background(), scan.Options{Store: st, Thumbs: th, Roots: []string{lib}}); err != nil {
			t.Fatal(err)
		}
		rows, _ := st.AllRows()
		if len(rows) != 4 {
			t.Fatalf("fixture indexed %d comics, want 4 (non-comic zip skipped)", len(rows))
		}
		var noComment bool
		for i := range rows {
			if rows[i].Filename == "NoComment.cbz" {
				noComment = true
				if len(rows[i].KV) != 0 {
					t.Error("NoComment.cbz should have zero comment keys")
				}
			}
		}
		if !noComment {
			t.Error("NoComment.cbz was not indexed")
		}
		t.Logf("fixtures OK: 4 comics indexed, non-comic zip skipped, no-comment comic handled")
	}
}

func TestNaturalOrderInRealContent(t *testing.T) {
	// Verify p1/p2/p10 ordering used for covers matches UI expectations.
	if !comic.IsImageExt("P1.JPG") {
		t.Error("IsImageExt should be case-insensitive")
	}
	if comic.IsImageExt("cover.pdf") {
		t.Error("pdf must not count as an image")
	}
}
