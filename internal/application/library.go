package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"comicgo/internal/config"
	"comicgo/internal/importer"
	"comicgo/internal/index"
	"comicgo/internal/model"
)

// SetReaderPreferences persists the page layout and reading direction used by
// the reader. Unknown directions fall back to left-to-right.
func (a *App) SetReaderPreferences(direction string, twoPage bool) error {
	direction = model.NormalizeReadingDirection(direction)
	a.mu.Lock()
	defer a.mu.Unlock()

	oldDirection := a.settings.ReadingDirection
	oldTwoPage := a.settings.TwoPageSpread
	a.settings.ReadingDirection = direction
	a.settings.TwoPageSpread = twoPage
	if err := config.SaveSettings(a.settings); err != nil {
		a.settings.ReadingDirection = oldDirection
		a.settings.TwoPageSpread = oldTwoPage
		return err
	}
	return nil
}

// AddLibrary persists a folder to the library roots and scans it.
func (a *App) AddLibrary(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("folder not found: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("not a folder: %s", dir)
	}
	for _, r := range a.settings.LibraryRoots {
		if samePath(r, dir) {
			return nil // already added
		}
	}
	a.settings.LibraryRoots = append(a.settings.LibraryRoots, dir)
	if err := config.SaveSettings(a.settings); err != nil {
		return err
	}
	a.startScan()
	a.syncWatcher()
	return nil
}

// RemoveLibrary removes a root without deleting files. Indexed comics under
// that folder are purged so they stop appearing on the shelf. It also stops
// any scan currently adding rows from that folder (otherwise commits that land
// after the purge would linger as orphans) and enforces the invariant that no
// indexed comic lives outside the remaining roots.
func (a *App) RemoveLibrary(dir string) error {
	var kept []string
	for _, r := range a.settings.LibraryRoots {
		if !samePath(r, dir) {
			kept = append(kept, r)
		}
	}
	a.settings.LibraryRoots = kept
	if err := config.SaveSettings(a.settings); err != nil {
		return err
	}
	a.CancelScan()                  // stop the scan that may still be adding this dir's rows
	a.waitScanDone(3 * time.Second) // ...and wait for it to actually exit
	a.pruneStaleRoots()
	if len(kept) > 0 {
		a.startScan() // on completion its sweep removes any straggler commits
	}
	a.syncWatcher()
	return nil
}

// ---- comic import ----

// ImportSources converts every supported archive or image folder into a .zip
// under destination, then starts an index scan. The GUI owns source and
// destination dialogs; this method only validates and performs the work.
func (a *App) ImportSources(sources []string, destination string) ([]model.ImportOutcome, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	if len(a.settings.LibraryRoots) == 0 {
		return nil, fmt.Errorf("add a library folder before importing comics")
	}
	validDest := false
	for _, root := range a.settings.LibraryRoots {
		if index.IsWithin(root, destination) {
			validDest = true
			break
		}
	}
	if !validDest {
		return nil, fmt.Errorf("import folder is not inside a library folder: %s", destination)
	}
	if !a.importMu.TryLock() {
		return nil, errors.New("an import is already running")
	}
	defer a.importMu.Unlock()

	var imported []model.ImportOutcome
	var failures []string
	for _, src := range sources {
		out, err := a.importOne(src, destination)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if out != nil {
			imported = append(imported, *out)
		}
	}
	if len(imported) == 0 {
		if len(failures) > 0 {
			return nil, errors.New(strings.Join(failures, "; "))
		}
		return nil, errors.New("nothing to import: drop comic archives or image folders")
	}
	a.startScan() // the new .zip files are now complete; index them
	if len(failures) > 0 {
		return imported, errors.New(strings.Join(failures, "; "))
	}
	return imported, nil
}

// importOne converts a single source into a unique .zip under destDir, emitting
// import:progress events while it streams. Returns (nil, nil) for a source this
// app cannot import (a missing path or a file that is not an archive), so batch
// drops can skip such noise without failing the rest.
func (a *App) importOne(source, destDir string) (*model.ImportOutcome, error) {
	fi, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // a stale dropped path
		}
		return nil, err
	}
	isDir := fi.IsDir()
	if !isDir && !isImportableExt(source) {
		return nil, nil // not an archive we read; skip silently
	}
	// A "brace" source name ({Author...} Title {Tag...}) becomes a .zip named
	// after the clean title, with the parsed metadata written into the archive
	// comment (Title/Author/Genre) so the shelf shows author + tag chips and tag
	// search works. Plain names import unchanged (no comment).
	stem := importer.DesiredStem(source, isDir)
	title, author, tags := importer.ParseName(stem)
	base := stem + ".zip"
	comment := ""
	if title != "" && (author != "" || len(tags) > 0) {
		base = title + ".zip"
		comment = importer.BuildComment(title, author, tags)
	}
	dest := importer.UniqueDest(destDir, base)
	pages, err := importer.Import(context.Background(), importer.Options{
		Source:  source,
		Dest:    dest,
		Comment: comment,
		OnProgress: func(done, total int) {
			a.importProgress(model.ImportProgress{
				Done: done, Total: total, Name: filepath.Base(dest),
			})
		},
	})
	if err != nil {
		return nil, err
	}
	kind := "file"
	if isDir {
		kind = "folder"
	}
	return &model.ImportOutcome{Path: dest, PageCount: pages, Source: source, Kind: kind}, nil
}

// isImportableExt reports whether a file path is an archive kind the importer
// can convert (used to skip non-archive files dropped alongside real ones).
func isImportableExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip", ".cbz", ".rar", ".cbr", ".7z", ".cb7", ".tar", ".gz", ".tgz":
		return true
	}
	return false
}

// pruneStaleRoots deletes any indexed comic that is not under a current
// library root (rows whose folder was removed mid-scan, etc.).
func (a *App) pruneStaleRoots() {
	paths, err := a.store.ListPaths()
	if err != nil {
		a.dbg("pruneStale list error: %v", err)
		return
	}
	removed := 0
	for _, p := range paths {
		keep := false
		for _, r := range a.settings.LibraryRoots {
			if index.IsWithin(r, p) {
				keep = true
				break
			}
		}
		if keep {
			continue
		}
		if id, ok, _ := a.store.IDFromPath(p); ok {
			a.thumbs.Remove(id)
		}
		if err := a.store.DeleteByPath(p); err == nil {
			removed++
		}
	}
	if removed > 0 {
		a.invalidate()
		a.dbg("pruned %d stale comic(s) outside library roots", removed)
	}
}

// purgeRoot deletes indexed rows (and thumbnails) for archives under dir.
func (a *App) purgeRoot(dir string) {
	paths, err := a.store.ListPaths()
	if err != nil {
		a.dbg("purgeRoot list error: %v", err)
		return
	}
	for _, p := range paths {
		if !index.IsWithin(dir, p) {
			continue
		}
		if id, ok, _ := a.store.IDFromPath(p); ok {
			a.thumbs.Remove(id)
		}
		if err := a.store.DeleteByPath(p); err != nil {
			a.dbg("purgeRoot delete %s: %v", p, err)
		}
	}
	a.invalidate()
}

// GetAbout returns info for the About dialog.
