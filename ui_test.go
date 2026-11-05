package main

import (
	"image"
	"path/filepath"
	"testing"

	"comicgo/internal/model"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
)

func TestLandscapeLeftCrop(t *testing.T) {
	if !landscapeLeftCrop(1600, 500) {
		t.Fatal("full landscape cover should crop from the left")
	}
	if landscapeLeftCrop(500, 700) {
		t.Fatal("portrait cover should stay centered")
	}
	if landscapeLeftCrop(500, 500) {
		t.Fatal("square cover should stay centered")
	}
}

func TestGridGeometry(t *testing.T) {
	cols, cardW, coverH, rowH := gridGeometry(1200, 150, 14, 14)
	if cols < 2 {
		t.Fatalf("cols = %d, want at least 2", cols)
	}
	if cardW < 80 || coverH != cardW*4/3 {
		t.Fatalf("card geometry = %d x %d", cardW, coverH)
	}
	if rowH != coverH+captionH+14 {
		t.Fatalf("row height = %d", rowH)
	}
}

func TestGridViewportHeightHugsShortLibrary(t *testing.T) {
	short := gridViewportHeight(700, 1200, 3, 150, 14, 14)
	_, _, _, rowH := gridGeometry(1200, 150, 14, 14)
	if short != rowH {
		t.Fatalf("short grid height = %d, want one padded row %d", short, rowH)
	}

	long := gridViewportHeight(700, 1200, 21, 150, 14, 14)
	if long != 700 {
		t.Fatalf("long grid height = %d, want viewport height 700", long)
	}
}

func TestVisibleRows(t *testing.T) {
	start, end := visibleRows(10, 4, 30, 3)
	if start != 7 || end != 17 {
		t.Fatalf("visibleRows = %d..%d, want 7..17", start, end)
	}
	start, end = visibleRows(0, 1, 0, 3)
	if start != 0 || end != -1 {
		t.Fatalf("empty visibleRows = %d..%d", start, end)
	}
}

func TestTagQueryEscaping(t *testing.T) {
	u := &appUI{
		search: `space  opera`,
		tags:   []string{`quote " slash \`},
	}
	got := u.query()
	want := `space  opera tag:"quote \" slash \\"`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestImportDestinationMustBeInsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	a := &App{settings: model.Settings{LibraryRoots: []string{root}}}
	if _, err := a.ImportSources([]string{filepath.Join(root, "book.cbz")}, outside); err == nil {
		t.Fatal("ImportSources accepted a destination outside library roots")
	}
}

func TestStatsCardRendererRegistered(t *testing.T) {
	if cardRenderers["stats"] == nil {
		t.Fatal("stats Gio renderer is not registered")
	}
}

func TestRuleMenusAreExclusiveAndToggleClosed(t *testing.T) {
	first := &sectionState{menuKind: "filter"}
	second := &sectionState{}
	u := &appUI{sections: []*sectionState{first, second}}

	u.toggleRuleMenu(second, "sort", image.Pt(100, 100))
	if first.menuKind != "" || second.menuKind != "sort" {
		t.Fatalf("open menus = %q, %q; want empty, sort", first.menuKind, second.menuKind)
	}
	u.toggleRuleMenu(second, "sort", image.Pt(100, 100))
	if second.menuKind != "" {
		t.Fatalf("sort menu remained open after toggle: %q", second.menuKind)
	}
}

func TestSearchSyncPreservesSourceCaret(t *testing.T) {
	source := &sectionState{}
	source.search.SetText("ka")
	source.search.SetCaret(2, 2)
	other := &sectionState{}
	u := &appUI{sections: []*sectionState{source, other}}

	u.syncSearchEditors("ka", source)

	if _, col := source.search.CaretPos(); col != 2 {
		t.Fatalf("source caret column = %d, want 2", col)
	}
	if got := other.search.Text(); got != "ka" {
		t.Fatalf("other search text = %q, want %q", got, "ka")
	}
}

func TestLoadedPageFromPreviousQueryNeedsReload(t *testing.T) {
	s := &sectionState{
		cards:         map[int]*cardState{0: {}},
		cardRevisions: map[int]int{0: 3},
	}

	if !s.pageNeedsLoad(0, 1, 4) {
		t.Fatal("page loaded for an older query was treated as current")
	}
	if s.pageNeedsLoad(0, 1, 3) {
		t.Fatal("current, complete page was unnecessarily reloaded")
	}
}

func TestReaderSpreadPageNormalization(t *testing.T) {
	tests := []struct {
		page, count int
		twoPage     bool
		want        int
	}{
		{page: 0, count: 10, twoPage: true, want: 0},
		{page: 1, count: 10, twoPage: true, want: 0},
		{page: 2, count: 10, twoPage: true, want: 2},
		{page: 9, count: 10, twoPage: true, want: 8},
		{page: 9, count: 10, twoPage: false, want: 9},
		{page: -3, count: 10, twoPage: false, want: 0},
		{page: 30, count: 10, twoPage: true, want: 8},
	}
	for _, tc := range tests {
		if got := normalizeReaderPage(tc.page, tc.count, tc.twoPage); got != tc.want {
			t.Errorf("normalizeReaderPage(%d, %d, %v) = %d, want %d", tc.page, tc.count, tc.twoPage, got, tc.want)
		}
	}

	if got := readerLastViewStart(10, true); got != 8 {
		t.Fatalf("last spread starts at %d, want 8", got)
	}
	if readerCanGoNext(8, 10, true) {
		t.Fatal("the final spread was treated as having a next view")
	}
	if !readerCanGoNext(6, 10, true) {
		t.Fatal("the penultimate spread was treated as final")
	}
	if got := (&readerState{page: 8, pageCount: 10, twoPage: true}).progressPage(); got != 9 {
		t.Fatalf("final spread progress page = %d, want 9", got)
	}
}

func TestReaderDirectionControlsPagingSides(t *testing.T) {
	tests := []struct {
		name      string
		x         int
		direction string
		want      int
	}{
		{name: "LTR left is previous", x: 10, direction: model.ReadingDirectionLTR, want: -1},
		{name: "LTR right is next", x: 90, direction: model.ReadingDirectionLTR, want: 1},
		{name: "RTL left is next", x: 10, direction: model.ReadingDirectionRTL, want: 1},
		{name: "RTL right is previous", x: 90, direction: model.ReadingDirectionRTL, want: -1},
		{name: "center does nothing", x: 50, direction: model.ReadingDirectionRTL, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := readerSidePageDelta(tc.x, 100, tc.direction); got != tc.want {
				t.Fatalf("page delta = %d, want %d", got, tc.want)
			}
		})
	}
	if got := readerSpatialPageDelta(-1, model.ReadingDirectionRTL); got != 1 {
		t.Fatalf("RTL left button delta = %d, want 1", got)
	}
	if got := readerSpatialPageDelta(1, model.ReadingDirectionRTL); got != -1 {
		t.Fatalf("RTL right button delta = %d, want -1", got)
	}
}

func TestReaderDirectionControlsArrowKeys(t *testing.T) {
	if got := readerKeyPageDelta(key.NameRightArrow, model.ReadingDirectionLTR); got != 1 {
		t.Fatalf("LTR right arrow delta = %d, want 1", got)
	}
	if got := readerKeyPageDelta(key.NameRightArrow, model.ReadingDirectionRTL); got != -1 {
		t.Fatalf("RTL right arrow delta = %d, want -1", got)
	}
	if got := readerKeyPageDelta(key.NameLeftArrow, model.ReadingDirectionRTL); got != 1 {
		t.Fatalf("RTL left arrow delta = %d, want 1", got)
	}
	if got := readerKeyPageDelta(key.NameDownArrow, model.ReadingDirectionRTL); got != 1 {
		t.Fatalf("down arrow delta = %d, want 1", got)
	}
}

func TestReaderAcceptsOnlyVisibleSpreadSlots(t *testing.T) {
	r := readerState{id: 7, page: 2, pageCount: 5, twoPage: true}
	if !r.acceptsPage(7, 0, 2) {
		t.Fatal("reader rejected the leading visible page")
	}
	if !r.acceptsPage(7, 1, 3) {
		t.Fatal("reader rejected the trailing visible page")
	}
	if r.acceptsPage(7, 1, 4) {
		t.Fatal("reader accepted a stale page for the second slot")
	}
	if r.acceptsPage(8, 0, 2) {
		t.Fatal("reader accepted a page for another comic")
	}
}

func TestFrameKeyEventsReachGlobalHandler(t *testing.T) {
	var u appUI
	u.screen = screenDetail
	router := new(input.Router)
	ops := new(op.Ops)

	frame := func() {
		gtx := layout.Context{
			Ops:         ops,
			Source:      router.Source(),
			Constraints: layout.Exact(image.Pt(100, 100)),
		}
		u.handleKeyEvents(gtx)
		router.Frame(ops)
		ops.Reset()
	}

	frame()
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()

	if u.screen != screenShelf {
		t.Fatalf("screen = %v after Escape, want shelf", u.screen)
	}
}
