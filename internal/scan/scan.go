// Package scan walks library folders, parsing new/changed archives and
// pruning removed ones. It is resumable: unchanged archives are skipped by
// their (size, mtime) fingerprint, so an interrupted run only re-does files
// that had not yet been committed.
package scan

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"comicgo/internal/cache"
	"comicgo/internal/comic"
	"comicgo/internal/imgutil"
	"comicgo/internal/index"
	"comicgo/internal/meta"
	"comicgo/internal/model"
)

const (
	thumbMaxDim  = 320
	thumbQuality = 80
)

// Options configures one scan pass.
type Options struct {
	Store      *index.Store
	Thumbs     *cache.ThumbStore
	Roots      []string
	Force      bool // ignore fingerprints and re-parse everything
	OnProgress func(model.ScanProgress)
	OnError    func(path string, err error)
}

type jobItem struct {
	path  string
	size  int64
	mtime int64
}

type parseResult struct {
	rec   index.Record
	thumb []byte
	err   error
}

func isComicExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip", ".cbz":
		return true
	}
	return false
}

// Run scans every root. It returns after all archives are handled or ctx is
// cancelled.
func Run(ctx context.Context, o Options) error {
	if len(o.Roots) == 0 {
		return nil
	}

	// 1) enumerate archives and split into skip / work lists
	var work []jobItem
	seen := map[string]struct{}{}
	for _, root := range o.Roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !isComicExt(p) {
				return nil
			}
			seen[p] = struct{}{}
			fi, serr := os.Stat(p)
			if serr != nil {
				return nil
			}
			item := jobItem{path: p, size: fi.Size(), mtime: fi.ModTime().UnixNano()}
			// fingerprint skip
			if sz, mt, found, ferr := o.Store.FindFingerprint(p); ferr == nil && found && sz == item.size && mt == item.mtime && !o.Force {
				return nil // unchanged: nothing to do
			}
			work = append(work, item)
			return nil
		})
	}
	total := len(seen)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// 2) parse the work list on a worker pool; a single writer commits results
	completed := total - len(work) // files skipped by fingerprint
	if len(work) > 0 {
		jobs := make(chan jobItem)
		results := make(chan parseResult, 64)
		ctxCancel := ctx

		workers := runtime.NumCPU()
		if workers > len(work) {
			workers = len(work)
		}
		var wg sync.WaitGroup
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer wg.Done()
				for it := range jobs {
					rec, thumb, err := parseOne(it)
					select {
					case <-ctxCancel.Done():
						return
					case results <- parseResult{rec: rec, thumb: thumb, err: err}:
					}
				}
			}()
		}
		go func() {
			defer close(jobs)
			for _, it := range work {
				select {
				case <-ctxCancel.Done():
					return
				case jobs <- it:
				}
			}
		}()
		go func() {
			wg.Wait()
			close(results)
		}()

		for res := range results {
			if res.err != nil {
				if errors.Is(res.err, comic.ErrNotAComic) {
					// a valid archive with no images (or one that can no longer
					// be read): drop any stale row + thumbnail, silently.
					if id, ok, _ := o.Store.IDFromPath(res.rec.Path); ok {
						o.Thumbs.Remove(id)
					}
					_ = o.Store.DeleteByPath(res.rec.Path)
				}
				// Other parse failures (corrupt/non-zip with a .zip name) are
				// ignored: the file simply is not indexed.
			} else {
				id, err := o.Store.CommitComic(res.rec)
				if err != nil {
					if o.OnError != nil {
						o.OnError(res.rec.Path, err)
					}
				} else if len(res.thumb) > 0 {
					_ = o.Thumbs.Write(id, res.thumb)
				}
			}
			completed++
			emit(o, total, completed, res.rec.Path)
			if ctx.Err() != nil {
				break
			}
		}
	}
	emit(o, total, total, "")

	// 3) prune rows whose files no longer exist under any root
	return pruneRemoved(o, seen)
}

func parseOne(it jobItem) (index.Record, []byte, error) {
	rec := index.Record{Path: it.path, FileSize: it.size, MtimeNs: it.mtime}
	a, err := comic.Open(it.path)
	if err != nil {
		rec.Path = it.path // keep path so ErrNotAComic results can prune
		return rec, nil, err
	}
	defer a.Close()

	comment := sanitize(a.Comment())
	kv := meta.Parse(comment)
	rec = index.Record{
		Path:       it.path,
		Dir:        filepath.Dir(it.path),
		Filename:   filepath.Base(it.path),
		FileSize:   it.size,
		MtimeNs:    it.mtime,
		PageCount:  a.PageCount(),
		CoverEntry: sanitize(a.CoverName()),
		CommentRaw: comment,
		HasNonUTF8: a.HasNonUTF8(),
		KV:         kv,
	}
	return rec, thumbOf(a), nil
}

func thumbOf(a *comic.Archive) []byte {
	rc, err := a.OpenPage(0)
	if err != nil {
		return nil
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil
	}
	thumb, err := imgutil.ThumbJPEG(b, thumbMaxDim, thumbQuality)
	if err != nil {
		return nil
	}
	return thumb
}

func pruneRemoved(o Options, seen map[string]struct{}) error {
	paths, err := o.Store.ListPaths()
	if err != nil {
		return err
	}
	for _, p := range paths {
		underRoot := false
		for _, root := range o.Roots {
			if index.IsWithin(root, p) {
				underRoot = true
				break
			}
		}
		if !underRoot {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		if id, ok, _ := o.Store.IDFromPath(p); ok {
			o.Thumbs.Remove(id)
		}
		_ = o.Store.DeleteByPath(p)
	}
	return nil
}

func emit(o Options, total, done int, path string) {
	if o.OnProgress != nil {
		o.OnProgress(model.ScanProgress{Done: done, Total: total, Path: path})
	}
}

func sanitize(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "�")
}
