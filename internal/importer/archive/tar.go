package archive

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"

	"comicgo/internal/comic"
)

// tarSrc reads pages from a .tar (or gzip'd .tar.gz/.tgz). Plain tar files
// cannot be indexed without walking them, so this uses two streaming passes:
// the first collects every image name, the second streams the archive again in
// its own order writing each image under the rank it earned in the natural
// sort. Non-image entries are skipped; tar.Reader discards unread entry data
// on the following Next().
type tarSrc struct {
	path  string
	gz    bool
	rank  map[string]*intQueue
	count int
	f     *os.File
	tr    *tar.Reader
	gr    *gzip.Reader
}

func openTar(path string, gz bool) (*tarSrc, error) {
	var names []string
	err := scanTar(path, gz, func(hdr *tar.Header) {
		if hdr.Size > 0 && comic.IsImageExt(hdr.Name) {
			names = append(names, hdr.Name)
		}
	})
	if err != nil {
		return nil, err
	}
	s := &tarSrc{path: path, gz: gz, rank: rankQueues(names), count: len(names)}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	s.f = f
	if gz {
		gr, err := gzip.NewReader(f)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		s.gr = gr
		s.tr = tar.NewReader(gr)
	} else {
		s.tr = tar.NewReader(f)
	}
	return s, nil
}

// scanTar streams a tar (decompressing gz) and calls fn for every file header.
func scanTar(path string, gz bool, fn func(*tar.Header)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader = f
	if gz {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gr.Close()
		r = gr
	}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		fn(hdr)
	}
}

func (s *tarSrc) Total() int { return s.count }

func (s *tarSrc) Next() (*Page, error) {
	for {
		hdr, err := s.tr.Next()
		if err != nil { // io.EOF included
			return nil, err
		}
		if hdr.Typeflag == tar.TypeDir || hdr.Size == 0 || !comic.IsImageExt(hdr.Name) {
			continue
		}
		q := s.rank[hdr.Name]
		if q == nil {
			continue
		}
		rank := q.pop()
		size := hdr.Size
		return &Page{
			Entry: EntryName(rank, filepath.Ext(hdr.Name)),
			Open: func() (io.ReadCloser, error) {
				// Bound this page to exactly its tar body so io.Copy stops at
				// the entry end and the next Next() starts at the next header.
				return io.NopCloser(io.LimitReader(s.tr, size)), nil
			},
		}, nil
	}
}

func (s *tarSrc) Close() error {
	var err error
	if s.gr != nil {
		err = s.gr.Close()
	}
	if s.f != nil {
		if cerr := s.f.Close(); err == nil {
			err = cerr
		}
	}
	return err
}
