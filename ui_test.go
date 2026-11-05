package main

import (
	"errors"
	"image"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"comicgo/internal/model"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
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

func TestReaderKeyEventsReachReaderHandler(t *testing.T) {
	var u appUI
	u.screen = screenReader
	router := new(input.Router)
	ops := new(op.Ops)

	frame := func() {
		gtx := layout.Context{
			Ops:         ops,
			Source:      router.Source(),
			Constraints: layout.Exact(image.Pt(100, 100)),
		}
		u.handleReaderKeyEvents(gtx)
		router.Frame(ops)
		ops.Reset()
	}

	frame()
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()

	if u.screen != screenDetail {
		t.Fatalf("screen = %v after Escape, want detail", u.screen)
	}
}

func TestReaderFooterControlsUseExactHitBounds(t *testing.T) {
	u := newAppUI(nil)
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Source:      new(input.Router).Source(),
		Constraints: layout.Exact(image.Pt(800, 54)),
	}
	var button, first, second widget.Clickable
	if got := u.readerFooterButton(gtx, &button, "Go", 42, buttonStyle{}); got.Size != image.Pt(42, 32) {
		t.Fatalf("footer button size = %v, want 42x32", got.Size)
	}
	if got := u.readerSegmentControl(gtx, 132, &first, "1 page", true, &second, "2 pages", false); got.Size != image.Pt(132, 32) {
		t.Fatalf("segment control size = %v, want 132x32", got.Size)
	}
}

func TestCaptionControlHitRegions(t *testing.T) {
	regions := captionHitRegions(1200, 52, 1)
	tests := []struct {
		name string
		part chromePart
	}{
		{name: "minimize", part: chromeMinimize},
		{name: "maximize", part: chromeMaximize},
		{name: "close", part: chromeClose},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rect := chromeRegionForPart(regions, tc.part)
			if rect.Empty() {
				t.Fatalf("%s hit region is empty", tc.name)
			}
			if rect.Dy() >= rect.Dx() {
				t.Fatalf("%s control is %v; want landscape proportions", tc.name, rect.Size())
			}
			if rect.Min.Y != captionButtonTopDp {
				t.Fatalf("%s control starts at y=%d; want %d", tc.name, rect.Min.Y, captionButtonTopDp)
			}
			center := rect.Min.Add(image.Pt(rect.Dx()/2, rect.Dy()/2))
			if got := regions.at(center); got != tc.part {
				t.Fatalf("hit at %v = %v, want %v", center, got, tc.part)
			}
		})
	}
	if got := regions.at(image.Pt(600, 20)); got != chromeNone {
		t.Fatalf("content hit = %v, want chromeNone", got)
	}
	if !image.Pt(600, 20).In(regions.Caption) {
		t.Fatal("caption context-menu region does not cover the title strip")
	}
}

func chromeRegionForPart(regions chromeHitRegions, part chromePart) image.Rectangle {
	switch part {
	case chromeMinimize:
		return regions.Minimize
	case chromeMaximize:
		return regions.Maximize
	case chromeClose:
		return regions.Close
	default:
		return image.Rectangle{}
	}
}

func TestChromeActionTracksMaximizedState(t *testing.T) {
	if got := chromeAction(chromeMaximize, false); got != system.ActionMaximize {
		t.Fatalf("windowed maximize action = %v", got)
	}
	if got := chromeAction(chromeMaximize, true); got != system.ActionUnmaximize {
		t.Fatalf("maximized maximize action = %v", got)
	}
	if got := chromeAction(chromeClose, false); got != system.ActionClose {
		t.Fatalf("close action = %v", got)
	}
}

func TestRestoreGlyphMatchesWindowsGeometry(t *testing.T) {
	geometry := captionRestoreGeometry(20, 20, 6)
	if geometry.Square.Empty() || geometry.Square.Dx() != geometry.Square.Dy() {
		t.Fatalf("restore square = %v; want a non-empty square", geometry.Square)
	}
	if geometry.Radius <= 0 || geometry.Radius*2 >= geometry.Square.Dx() {
		t.Fatalf("restore square radius = %d for size %v", geometry.Radius, geometry.Square.Size())
	}
	if geometry.BracketCorner.X <= geometry.Square.Max.X || geometry.BracketCorner.Y >= geometry.Square.Min.Y {
		t.Fatalf("restore bracket corner %v is not outside the square top-right", geometry.BracketCorner)
	}
	if geometry.BracketStart.Y != geometry.BracketCorner.Y || geometry.BracketEnd.X != geometry.BracketCorner.X {
		t.Fatalf("restore bracket is not a right angle: %+v", geometry)
	}
	if gap := geometry.Square.Min.Y - geometry.BracketStart.Y; gap < 2 {
		t.Fatalf("restore top gap = %d; want at least 2 for a visible 1px gap", gap)
	}
	if gap := geometry.BracketCorner.X - geometry.Square.Max.X; gap < 2 {
		t.Fatalf("restore right gap = %d; want at least 2 for a visible 1px gap", gap)
	}
}

func TestWindowConfigUpdatesChromeState(t *testing.T) {
	var u appUI
	u.applyWindowConfig(app.Config{Mode: app.Maximized, Focused: true})
	if !u.chrome.maximized || !u.chrome.focused {
		t.Fatalf("maximized chrome = %+v", u.chrome)
	}
	u.applyWindowConfig(app.Config{Mode: app.Windowed, Focused: false})
	if u.chrome.maximized || u.chrome.focused {
		t.Fatalf("restored chrome = %+v", u.chrome)
	}
}

func TestSamePathUsesPlatformIdentity(t *testing.T) {
	root := t.TempDir()
	if !samePath(root, filepath.Join(root, ".")) {
		t.Fatal("same directory with a normalized suffix was not recognized")
	}
	if samePath(root, filepath.Join(root, "..")) {
		t.Fatal("different directories were treated as identical")
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		if samePath(filepath.Join(root, "Comics"), filepath.Join(root, "comics")) {
			t.Fatal("case-distinct Unix paths were treated as identical")
		}
	}
}

func TestDialogPortalErrorIsActionable(t *testing.T) {
	err := errors.New("The name org.freedesktop.portal.Desktop was not provided by any .service files")
	msg := dialogErrorMessage(err)
	if !strings.Contains(msg, "xdg-desktop-portal") {
		t.Fatalf("portal error message = %q", msg)
	}
}

func TestTopBarDragDoesNotCoverControls(t *testing.T) {
	u := newAppUI(nil)
	u.settings.LibraryRoots = []string{filepath.Join(t.TempDir(), "Comics")}
	u.top.rootRemove = make([]widget.Clickable, 1)
	u.indexed = 318

	router := new(input.Router)
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Source:      router.Source(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1200, 52)),
	}
	u.layoutTopbar(gtx)
	router.Frame(ops)

	if action, ok := router.ActionAt(f32.Pt(500, 26)); !ok || action != system.ActionMove {
		t.Fatalf("empty title bar action = %v, %v; want ActionMove", action, ok)
	}
	for _, x := range []float32{800, 1000, 1150} {
		if action, ok := router.ActionAt(f32.Pt(x, 48)); !ok || action != system.ActionMove {
			t.Fatalf("right title bar action at x=%v = %v, %v; want ActionMove", x, action, ok)
		}
	}
	if action, ok := router.ActionAt(f32.Pt(748, 26)); ok && action == system.ActionMove {
		t.Fatalf("Import button action = %v, %v; title bar move must not cover it", action, ok)
	}
}

func TestTopBarImportButtonStillClicks(t *testing.T) {
	u := newAppUI(nil)
	u.settings.LibraryRoots = []string{filepath.Join(t.TempDir(), "Comics")}
	u.top.rootRemove = make([]widget.Clickable, 1)
	u.indexed = 318

	router := new(input.Router)
	ops := new(op.Ops)
	frame := func() {
		gtx := layout.Context{
			Ops:         ops,
			Source:      router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1200, 52)),
		}
		u.layoutTopbar(gtx)
		router.Frame(ops)
		ops.Reset()
	}
	frame()
	var position f32.Point
	for x := 0; x < 1200; x += 2 {
		probe := f32.Pt(float32(x), 26)
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: probe})
		frame()
		if u.top.importToggle.Hovered() {
			position = probe
			break
		}
	}
	if position == (f32.Point{}) {
		t.Fatal("could not locate the Import button")
	}
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
	frame()
	if !u.importMenu {
		t.Fatal("Import click did not open the import menu")
	}
	if u.importMenuArmed {
		t.Fatal("Import menu became interactive in the click frame")
	}
	frame()
	if !u.importMenuArmed {
		t.Fatal("Import menu did not become visible on the following frame")
	}
}
