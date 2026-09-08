package importer

import (
	"io"
	"path/filepath"

	"comicgo/internal/comic"

	"github.com/nwaples/rardecode"
)

// rarSrc streams pages from a .rar/.cbr in archive order. RAR files cannot be
// pre-listed without decompressing their solid blocks, so this is a single
// forward pass and the page count is not known up front (progress reports
// total 0). Real-world .cbr files are stored in page order, which is what this
// preserves. Next() automatically discards skipped entries' data, including
// across solid blocks.
type rarSrc struct {
	rc *rardecode.ReadCloser
	n  int
}

func openRar(path string) (*rarSrc, error) {
	rc, err := rardecode.OpenReader(path, "")
	if err != nil {
		return nil, err
	}
	return &rarSrc{rc: rc}, nil
}

func (s *rarSrc) total() int { return 0 }

func (s *rarSrc) next() (*page, error) {
	for {
		hdr, err := s.rc.Next()
		if err != nil { // io.EOF included
			return nil, err
		}
		if hdr.IsDir || !comic.IsImageExt(hdr.Name) {
			continue // Next() discards this entry's data
		}
		s.n++
		rank := s.n
		r := s.rc
		return &page{
			entry: entryName(rank, filepath.Ext(hdr.Name)),
			open: func() (io.ReadCloser, error) {
				// The rardecode reader is bounded to the current file's size
				// and returns io.EOF at its end; io.Copy then naturally
				// advances us to the next entry for the following Next().
				return io.NopCloser(r), nil
			},
		}, nil
	}
}

func (s *rarSrc) close() error {
	if s.rc == nil {
		return nil
	}
	return s.rc.Close()
}
