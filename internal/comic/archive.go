// Package comic reads comic archives (ZIP/CBZ). An archive is opened once and
// kept in a small LRU so page reads seek to and inflate only the requested
// member rather than decompressing the whole archive.
package comic

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// IsImageExt reports whether a zip entry name looks like a readable image.
func IsImageExt(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

// ErrNotAComic is returned when an archive holds no decodable image entries.
var ErrNotAComic = errors.New("archive contains no image entries")

// naturalLess implements an alphanumeric (numeric-aware) comparison so that
// page1, page2, page10 sort in the natural order. It returns a < b.
//
// Both inputs are split into alternating digit / non-digit tokens. Digit
// tokens compare by numeric value (leading zeros stripped); other tokens
// compare case-insensitively. If the token streams are equal up to the end of
// the shorter string, the shorter string is less.
func naturalLess(a, b string) bool {
	ta, tb := tokens(a), tokens(b)
	for i := 0; i < len(ta) && i < len(tb); i++ {
		ca, cb := ta[i], tb[i]
		if ca.num && cb.num {
			// numeric compare (leading zeros already stripped in tokens)
			if len(ca.text) != len(cb.text) {
				return len(ca.text) < len(cb.text)
			}
			if ca.text != cb.text {
				return ca.text < cb.text
			}
		} else if !ca.num && !cb.num {
			l, r := lowerStr(ca.text), lowerStr(cb.text)
			if l != r {
				return l < r
			}
			if ca.text != cb.text { // stable tie-break for mixed case
				return ca.text < cb.text
			}
		} else {
			// a digit sorts before a non-digit at the same position
			return ca.num
		}
	}
	// all compared tokens equal; the shorter string (fewer tokens) is less
	return len(ta) < len(tb)
}

type token struct {
	text string
	num  bool
}

func tokens(s string) []token {
	var out []token
	i := 0
	for i < len(s) {
		isDigit := s[i] >= '0' && s[i] <= '9'
		j := i
		for j < len(s) && (s[j] >= '0' && s[j] <= '9') == isDigit {
			j++
		}
		t := token{text: s[i:j], num: isDigit}
		if isDigit {
			t.text = strings.TrimLeft(t.text, "0")
			if t.text == "" {
				t.text = "0"
			}
		}
		out = append(out, t)
		i = j
	}
	return out
}

func lowerStr(s string) string {
	return strings.ToLower(s)
}

// Archive is an open comic plus its ordered page map.
type Archive struct {
	Path       string
	rc         *zip.ReadCloser
	pages      []*zip.File // image entries in natural filename order
	coverName  string
	comment    string
	hasNonUTF8 bool
}

// Open opens path, builds the page map and returns an *Archive.
// The caller must Close it.
func Open(path string) (*Archive, error) {
	rc, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	pages := make([]*zip.File, 0)
	nonUTF8 := false
	for _, f := range rc.File {
		if IsImageExt(f.Name) {
			pages = append(pages, f)
		}
		if !utf8.ValidString(f.Name) {
			nonUTF8 = true
		}
	}
	sort.Slice(pages, func(i, j int) bool { return naturalLess(pages[i].Name, pages[j].Name) })
	if len(pages) == 0 {
		rc.Close()
		return nil, fmt.Errorf("%w: %s", ErrNotAComic, path)
	}
	if !utf8.ValidString(rc.Comment) {
		nonUTF8 = true
	}
	return &Archive{Path: path, rc: rc, pages: pages, coverName: pages[0].Name,
		comment: rc.Comment, hasNonUTF8: nonUTF8}, nil
}

// Comment returns the raw archive comment.
func (a *Archive) Comment() string { return a.comment }

// HasNonUTF8 reports whether any entry name or the comment was not valid UTF-8.
func (a *Archive) HasNonUTF8() bool { return a.hasNonUTF8 }

// PageCount returns the number of readable pages.
func (a *Archive) PageCount() int { return len(a.pages) }

// CoverName returns the zip entry used as the cover (the first image).
func (a *Archive) CoverName() string { return a.coverName }

// PageEntry returns the *zip.File for a 0-based page index.
func (a *Archive) PageEntry(n int) (*zip.File, error) {
	if n < 0 || n >= len(a.pages) {
		return nil, fmt.Errorf("page %d out of range [0,%d)", n, len(a.pages))
	}
	return a.pages[n], nil
}

// OpenPage streams the decoded bytes of page n. The caller is responsible for
// closing the returned io.ReadCloser.
func (a *Archive) OpenPage(n int) (io.ReadCloser, error) {
	f, err := a.PageEntry(n)
	if err != nil {
		return nil, err
	}
	return f.Open()
}

// Close releases the underlying file handle.
func (a *Archive) Close() error {
	if a.rc != nil {
		return a.rc.Close()
	}
	return nil
}

// entryOrderedPageNames returns the natural-ordered image entry names for tests.
func entryOrderedPageNames(pages []*zip.File) []string {
	out := make([]string, 0, len(pages))
	for _, p := range pages {
		out = append(out, p.Name)
	}
	return out
}

// stat returns the (size, mtimeNs) fingerprint used for change detection.
func stat(path string) (size int64, mtimeNs int64, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	return fi.Size(), fi.ModTime().UnixNano(), nil
}
