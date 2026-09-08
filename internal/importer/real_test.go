package importer

// Real-world RAR and 7z validation. There is no pure-Go encoder for either
// format, so these tests cannot build their own fixtures; they import a real
// file the user points at and skip when the env var is absent:
//
//	COMICGO_TEST_CBR=<path to a .cbr/.rar>
//	COMICGO_TEST_CB7=<path to a .cb7/.7z>
//
// Each asserts the produced .cbz opens with comic.Open and has > 0 pages.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"comicgo/internal/comic"
)

func TestRealRAR(t *testing.T) {
	path := os.Getenv("COMICGO_TEST_CBR")
	if path == "" {
		t.Skip("set COMICGO_TEST_CBR to a real .cbr/.rar file to run")
	}
	runRealImport(t, path)
}

func TestReal7Z(t *testing.T) {
	path := os.Getenv("COMICGO_TEST_CB7")
	if path == "" {
		t.Skip("set COMICGO_TEST_CB7 to a real .cb7/.7z file to run")
	}
	runRealImport(t, path)
}

func runRealImport(t *testing.T, src string) {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "out.cbz")
	n, err := Import(context.Background(), Options{Source: src, Dest: dest})
	if err != nil {
		t.Fatalf("Import(%s): %v", src, err)
	}
	if n == 0 {
		t.Fatalf("Import(%s): page count 0", src)
	}
	ar, err := comic.Open(dest)
	if err != nil {
		t.Fatalf("comic.Open(%s): %v", dest, err)
	}
	defer ar.Close()
	if ar.PageCount() != n {
		t.Fatalf("verified %d pages, wrote %d", ar.PageCount(), n)
	}
}
