// Package archive exposes the streaming page sources accepted by the
// importer. It owns archive-format details so the importer package can focus on
// naming and writing the output comic.
package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Page is one image to write into the destination archive.
type Page struct {
	Entry string
	Open  func() (io.ReadCloser, error)
}

// Source yields image pages in write order.
type Source interface {
	Next() (*Page, error)
	Total() int
	Close() error
}

type source struct {
	next  func() (*Page, error)
	total func() int
	close func() error
}

func (s *source) Next() (*Page, error) { return s.next() }
func (s *source) Total() int           { return s.total() }
func (s *source) Close() error         { return s.close() }

// Open returns a reader for an image directory or supported archive.
func Open(path string) (Source, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var src Source
	if fi.IsDir() {
		src, err = openDir(path)
	} else {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".zip", ".cbz":
			src, err = openZip(path)
		case ".rar", ".cbr":
			src, err = openRar(path)
		case ".7z", ".cb7":
			src, err = openSevenZip(path)
		case ".tar", ".gz", ".tgz":
			ext := filepath.Ext(path)
			src, err = openTar(path, strings.EqualFold(ext, ".gz") || strings.EqualFold(ext, ".tgz"))
		default:
			return nil, fmt.Errorf("unsupported source %q: not a comic archive or image folder", path)
		}
	}
	if err != nil {
		return nil, err
	}
	return &source{next: src.Next, total: src.Total, close: src.Close}, nil
}
