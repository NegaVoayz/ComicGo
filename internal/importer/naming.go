package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"comicgo/internal/comic"
)

// archiveSuffixes lists known archive extensions, longest first, so DesiredName
// strips the whole recognized suffix ("x.tar.gz" -> "x.zip", not "x.tar").
var archiveSuffixes = []string{
	".tar.gz", ".tar", ".tgz",
	".cbz", ".cbr", ".cb7",
	".zip", ".rar", ".7z",
}

// DesiredName returns the base .zip filename to create for a source: an image
// folder keeps its own name; an archive keeps its name with the recognized
// archive suffix replaced by ".zip" (e.g. "Vol.1.cbr" -> "Vol.1.zip").
func DesiredName(source string, isDir bool) string {
	return DesiredStem(source, isDir) + ".zip"
}

// DesiredStem returns DesiredName's filename without the ".zip" extension: the
// folder's own name, or the archive name stripped of its recognized suffix.
func DesiredStem(source string, isDir bool) string {
	source = strings.TrimRight(source, `/\`)
	base := source
	if i := strings.LastIndexAny(source, `/\`); i >= 0 {
		base = source[i+1:]
	}
	stem := base
	if !isDir {
		lower := strings.ToLower(base)
		for _, suf := range archiveSuffixes {
			if strings.HasSuffix(lower, suf) {
				stem = base[:len(base)-len(suf)]
				break
			}
		}
	}
	if stem == "" || stem == "." || stem == string(filepath.Separator) {
		return "Comic"
	}
	return stem
}

// UniqueDest returns the first path in dir that does not already exist for the
// requested base name. On a collision it appends " (2)", " (3)", ... before the
// file extension. Comparison is case-insensitive, matching how the Windows
// filesystem treats "A.cbz" and "a.cbz" as the same file.
func UniqueDest(dir, base string) string {
	ext := filepath.Ext(base) // e.g. ".zip" or ".cbz"
	stem := strings.TrimSuffix(base, ext)
	lower := map[string]bool{}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			lower[strings.ToLower(e.Name())] = true
		}
	}
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s (%d)%s", stem, n, ext)
		}
		if lower[strings.ToLower(name)] {
			continue
		}
		cand := filepath.Join(dir, name)
		if _, err := os.Lstat(cand); err != nil { // not taken (or unreadable)
			return cand
		}
	}
}

// entryName is the destination zip name for page rank (1-based): a six-digit,
// zero-padded number plus the image's lowercased extension, so natural sort
// order equals rank order for any reader.
func entryName(rank int, ext string) string {
	return fmt.Sprintf("%06d%s", rank, strings.ToLower(ext))
}

// intQueue is a FIFO of ranks used to assign ranks to duplicate page names.
type intQueue struct{ items []int }

func (q *intQueue) push(v int) { q.items = append(q.items, v) }

func (q *intQueue) pop() int {
	v := q.items[0]
	q.items = q.items[1:]
	return v
}

// rankQueues sorts names naturally and returns, for each name, the queue of
// page ranks assigned to its occurrences (rank 1 = first page). Unique names
// get exactly one rank = their position in the natural order; a name repeated
// in the source consumes consecutive ranks, mirroring how comic.Open keeps
// duplicate entries adjacent.
func rankQueues(names []string) map[string]*intQueue {
	sorted := append([]string(nil), names...)
	sort.SliceStable(sorted, func(i, j int) bool { return comic.NaturalLess(sorted[i], sorted[j]) })
	m := make(map[string]*intQueue, len(sorted))
	for i, n := range sorted {
		q := m[n]
		if q == nil {
			q = &intQueue{}
			m[n] = q
		}
		q.push(i + 1)
	}
	return m
}
