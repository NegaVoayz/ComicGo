package importer

import (
	"archive/zip"
	"io"
	"path/filepath"
	"sort"

	"comicgo/internal/comic"
)

// zipSrc reads pages from an existing .zip/.cbz. The zip central directory
// lists entries without touching their data, so entries are cheaply re-ordered
// into natural name order before streaming.
type zipSrc struct {
	rc    *zip.ReadCloser
	files []*zip.File
	i     int
}

func openZip(path string) (*zipSrc, error) {
	rc, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	var files []*zip.File
	for _, f := range rc.File {
		if comic.IsImageExt(f.Name) {
			files = append(files, f)
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return comic.NaturalLess(files[i].Name, files[j].Name) })
	return &zipSrc{rc: rc, files: files}, nil
}

func (s *zipSrc) total() int { return len(s.files) }

func (s *zipSrc) next() (*page, error) {
	if s.i >= len(s.files) {
		return nil, io.EOF
	}
	f := s.files[s.i]
	s.i++
	rank := s.i
	return &page{
		entry: entryName(rank, filepath.Ext(f.Name)),
		open:  f.Open,
	}, nil
}

func (s *zipSrc) close() error {
	if s.rc == nil {
		return nil
	}
	return s.rc.Close()
}
