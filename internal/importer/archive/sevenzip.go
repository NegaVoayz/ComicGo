package archive

import (
	"io"
	"path/filepath"

	"comicgo/internal/comic"

	"github.com/bodgit/sevenzip"
)

// szSrc streams pages from a .7z/.cb7. The 7z header lists every file cheaply,
// so ranks are pre-computed from a natural sort of the names. Files are then
// read in archive order in one forward pass: sevenzip caches each folder's
// decompressor across reads, which keeps solid archives linear rather than
// quadratic.
type sevenZipSource struct {
	rc    *sevenzip.ReadCloser
	files []*sevenzip.File // image entries, archive order
	rank  map[string]*intQueue
	i     int
}

func openSevenZip(path string) (*sevenZipSource, error) {
	rc, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	var files []*sevenzip.File
	var names []string
	for _, f := range rc.File {
		if f.FileInfo().IsDir() || !comic.IsImageExt(f.Name) {
			continue
		}
		files = append(files, f)
		names = append(names, f.Name)
	}
	return &sevenZipSource{rc: rc, files: files, rank: rankQueues(names)}, nil
}

func (s *sevenZipSource) Total() int { return len(s.files) }

func (s *sevenZipSource) Next() (*Page, error) {
	if s.i >= len(s.files) {
		return nil, io.EOF
	}
	f := s.files[s.i]
	s.i++
	rank := s.rank[f.Name].pop()
	f7 := f
	return &Page{
		Entry: EntryName(rank, filepath.Ext(f.Name)),
		Open:  f7.Open,
	}, nil
}

func (s *sevenZipSource) Close() error {
	if s.rc == nil {
		return nil
	}
	return s.rc.Close()
}
