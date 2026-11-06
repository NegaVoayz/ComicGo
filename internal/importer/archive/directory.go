package archive

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"comicgo/internal/comic"
)

// dirSrc reads every image file under a folder (recursively) as the pages of
// one comic. Files are ordered by their path relative to the folder using the
// same natural order the reader applies, so chapter subfolders keep their
// intended sequence.
type dirSrc struct {
	root  string
	names []string // relative paths, natural order
	i     int
}

func openDir(root string) (*dirSrc, error) {
	var names []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !comic.IsImageExt(d.Name()) {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		names = append(names, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(names, func(i, j int) bool { return comic.NaturalLess(names[i], names[j]) })
	return &dirSrc{root: root, names: names}, nil
}

func (s *dirSrc) Total() int { return len(s.names) }

func (s *dirSrc) Next() (*Page, error) {
	if s.i >= len(s.names) {
		return nil, io.EOF
	}
	rel := s.names[s.i]
	s.i++
	rank := s.i
	abs := filepath.Join(s.root, rel)
	return &Page{
		Entry: EntryName(rank, filepath.Ext(rel)),
		Open: func() (io.ReadCloser, error) {
			return os.Open(abs)
		},
	}, nil
}

func (s *dirSrc) Close() error { return nil }
