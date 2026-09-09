// Package importer converts any supported comic source (an archive file or an
// image folder) into a flat .zip. Every page is streamed straight into the
// output zip with the Store method (no re-compression), so memory stays
// bounded however large the source. Import is non-destructive: the source is
// never modified.
//
// Each destination entry is named by its page rank ("000001.jpg",
// "000002.png", ...) rather than its original path. comic.Open re-sorts
// entries with a natural (numeric-aware) order on read, so a rank-based name
// makes the reader's page order equal the intended order by construction and
// sidesteps folder prefixes, non-UTF-8 names and name collisions.
package importer

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"comicgo/internal/comic"
)

// Options configures one conversion.
type Options struct {
	Source string // archive path or image folder to read
	Dest   string // full destination .zip path (must already be unique)
	// Comment is written into the destination archive as its zip comment
	// (the "Key=Value" metadata format ComicGo reads). Empty for none.
	Comment string
	// OnProgress is called after each page is written. total is the page
	// count when the source could be pre-listed (zip/folder/tar/7z) and 0
	// when it could not (rar streams in archive order).
	OnProgress func(done, total int)
}

// Import converts Source into Dest and returns the number of pages written.
// Dest is verified with comic.Open before returning. On any error the partial
// Dest file is removed, so a failed import never leaves a broken .zip behind.
func Import(ctx context.Context, o Options) (int, error) {
	if o.Source == "" || o.Dest == "" {
		return 0, errors.New("importer: Source and Dest are required")
	}
	src, err := openSource(o.Source)
	if err != nil {
		return 0, err
	}
	defer src.close()
	return writePages(ctx, src, o.Dest, o.Comment, o.OnProgress)
}

// page is one image to write into the destination archive. open is called only
// when the writer is ready to stream that page's bytes (so sequential sources
// such as tar/rar stay positioned correctly between pages).
type page struct {
	entry string // destination zip entry name
	open  func() (io.ReadCloser, error)
}

// source yields image pages in write order. Because every entry name is a rank,
// the write order never affects the final reader order — it only constrains how
// much the source must buffer. Sources that can list cheaply pre-sort; sources
// that must stream (rar) yield in archive order.
type source interface {
	next() (*page, error) // returns (nil, io.EOF) when exhausted
	total() int           // page count if known up front, else 0
	close() error
}

// openSource dispatches to a page source based on what Source is: an image
// folder, or a supported archive extension.
func openSource(path string) (source, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return openDir(path)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip", ".cbz":
		return openZip(path)
	case ".rar", ".cbr":
		return openRar(path)
	case ".7z", ".cb7":
		return open7z(path)
	case ".tar", ".gz", ".tgz":
		// ".gz" and ".tgz" are gzip'd tar streams.
		return openTar(path, strings.EqualFold(filepath.Ext(path), ".gz") || strings.EqualFold(filepath.Ext(path), ".tgz"))
	}
	return nil, fmt.Errorf("unsupported source %q: not a comic archive or image folder", path)
}

// writePages streams every page of src into a new Store-method zip at dest,
// then verifies the result opens with comic.Open and holds the written count.
// On failure the partial dest is removed.
func writePages(ctx context.Context, src source, dest, comment string, onProgress func(int, int)) (int, error) {
	f, err := os.Create(dest)
	if err != nil {
		return 0, err
	}
	fail := func(werr error) (int, error) {
		_ = f.Close()
		_ = os.Remove(dest)
		return 0, werr
	}
	zw := zip.NewWriter(f)
	if comment != "" {
		zw.SetComment(comment)
	}
	written := 0
	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		p, err := src.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(fmt.Errorf("read source page %d: %w", written+1, err))
		}
		hdr := &zip.FileHeader{Name: p.entry, Method: zip.Store}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return fail(fmt.Errorf("create zip entry %s: %w", p.entry, err))
		}
		rc, err := p.open()
		if err != nil {
			return fail(fmt.Errorf("open source page %d (%s): %w", written+1, p.entry, err))
		}
		_, cpErr := io.Copy(w, rc)
		clErr := rc.Close()
		if cpErr != nil {
			return fail(fmt.Errorf("copy source page %d (%s): %w", written+1, p.entry, cpErr))
		}
		if clErr != nil {
			return fail(fmt.Errorf("close source page %d (%s): %w", written+1, p.entry, clErr))
		}
		written++
		if onProgress != nil {
			onProgress(written, src.total())
		}
	}
	if written == 0 {
		return fail(errors.New("source contains no image pages"))
	}
	if err := zw.Close(); err != nil {
		return fail(fmt.Errorf("finalize zip: %w", err))
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(dest)
		return 0, err
	}
	// Verification: the file must open as a comic holding exactly what we wrote.
	ar, err := comic.Open(dest)
	if err != nil {
		_ = os.Remove(dest)
		return written, fmt.Errorf("converted file failed to open: %w", err)
	}
	n := ar.PageCount()
	_ = ar.Close()
	if n != written {
		_ = os.Remove(dest)
		return written, fmt.Errorf("verification mismatch: wrote %d pages, archive holds %d", written, n)
	}
	return written, nil
}
