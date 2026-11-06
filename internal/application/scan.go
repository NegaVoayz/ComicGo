package application

import (
	"context"
	"fmt"
	"time"

	"comicgo/internal/model"
	"comicgo/internal/plugins"
	"comicgo/internal/scan"
)

func (a *App) GetAbout() model.About {
	ab := model.About{Version: Version}
	if a.paths != nil {
		ab.ConfigDir = a.paths.Root
	}
	ab.ComicCount = len(a.currentMetas())
	return ab
}

// StartScan begins an incremental scan of all library roots.
func (a *App) StartScan() error {
	if len(a.settings.LibraryRoots) == 0 {
		return fmt.Errorf("no library folder added yet")
	}
	a.startScan()
	return nil
}

// CancelScan stops an in-flight scan.
func (a *App) CancelScan() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.scanCancel != nil {
		a.scanCancel()
	}
}

// waitScanDone blocks until a running scan goroutine has fully exited (so no
// late commits land after a removal), or until the timeout elapses.
func (a *App) waitScanDone(timeout time.Duration) {
	a.mu.Lock()
	ch := a.scanDoneCh
	a.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case <-ch:
	case <-time.After(timeout):
	}
}

// IsScanning reports whether a scan is in progress.
func (a *App) IsScanning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.scanCancel != nil
}

func (a *App) startScan() {
	done := make(chan struct{})
	a.mu.Lock()
	if a.scanCancel != nil {
		a.scanCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.scanCancel = cancel
	a.scanDoneCh = done
	a.mu.Unlock()

	roots := append([]string(nil), a.settings.LibraryRoots...)
	a.dbg("startScan begin roots=%v", roots)
	a.scanStarted()

	go func() {
		defer close(done)
		opts := scan.Options{
			Store:  a.store,
			Thumbs: a.thumbs,
			Roots:  roots,
			OnProgress: func(p model.ScanProgress) {
				a.scanProgress(p)
			},
			OnError: func(path string, err error) {
				a.scanError(path, err)
			},
		}
		err := scan.Run(ctx, opts)
		a.mu.Lock()
		a.scanCancel = nil
		a.mu.Unlock()
		a.invalidate()
		// Enforce "no comic outside the current roots": removes any rows a
		// cancelled scan committed after the user removed their folder.
		a.pruneStaleRoots()
		if a.watcher != nil {
			a.watcher.Sync() // pick up newly created subfolders
		}
		a.dbg("scan finished err=%v", err)

		a.emitScanDone(err)
	}()
}

// ---- rules / query ----

// ListRules returns all registered filter and sort rules for dropdowns.
func (a *App) ListRules() []model.RuleInfo {
	var out []model.RuleInfo
	for _, r := range plugins.FilterRules {
		out = append(out, model.RuleInfo{ID: r.ID(), Label: r.Label(), Kind: "filter"})
	}
	for _, r := range plugins.SortRules {
		out = append(out, model.RuleInfo{ID: r.ID(), Label: r.Label(), Kind: "sort"})
	}
	return out
}

// CountComics returns the number of comics matching a filter ("" = all).
func (a *App) CountComics(filterRuleID string) int {
	n := len(a.filtered(filterRuleID))
	a.dbg("CountComics(filter=%q) -> %d", filterRuleID, n)
	return n
}

// CountMatching returns the number of comics matching a search query
// (optionally narrowed by a filter rule). The query may hold plain words and
// tag terms (tag:...); see parseSearch. Count and list go through the same
// parser so they can never disagree.
func (a *App) CountMatching(search, filterRuleID string) int {
	return len(a.selection(search, filterRuleID))
}

// searchQuery is a parsed search box query: text substrings and exact whole-tag
// matches. Every term must match (logical AND): all text terms must occur
// somewhere and all tags must be present on the comic.
