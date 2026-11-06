package ui

import (
	"errors"
	"image"
	"path/filepath"
	"strings"
	"testing"

	"comicgo/internal/model"

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
	if !LandscapeLeftCrop(1600, 500) {
		t.Fatal("full landscape cover should crop from the left")
	}
	if LandscapeLeftCrop(500, 700) {
		t.Fatal("portrait cover should stay centered")
	}
	if LandscapeLeftCrop(500, 500) {
		t.Fatal("square cover should stay centered")
	}
}

func TestGridGeometry(t *testing.T) {
	cols, cardW, coverH, rowH := GridGeometry(1200, 150, 14, 14)
	if cols < 2 {
		t.Fatalf("cols = %d, want at least 2", cols)
	}
	if cardW < 80 || coverH != cardW*4/3 {
		t.Fatalf("card geometry = %d x %d", cardW, coverH)
	}
	if rowH != coverH+CaptionH+14 {
		t.Fatalf("row height = %d", rowH)
	}
}

func TestGridViewportHeightHugsShortLibrary(t *testing.T) {
	short := GridViewportHeight(700, 1200, 3, 150, 14, 14)
	_, _, _, rowH := GridGeometry(1200, 150, 14, 14)
	if short != rowH {
		t.Fatalf("short grid height = %d, want one padded row %d", short, rowH)
	}

	long := GridViewportHeight(700, 1200, 21, 150, 14, 14)
	if long != 700 {
		t.Fatalf("long grid height = %d, want viewport height 700", long)
	}
}

func TestVisibleRows(t *testing.T) {
	start, end := VisibleRows(10, 4, 30, 3)
	if start != 7 || end != 17 {
		t.Fatalf("VisibleRows = %d..%d, want 7..17", start, end)
	}
	start, end = VisibleRows(0, 1, 0, 3)
	if start != 0 || end != -1 {
		t.Fatalf("empty VisibleRows = %d..%d", start, end)
	}
}

func TestLoadedPageFromPreviousQueryNeedsReload(t *testing.T) {
	s := &SectionState{
		Cards:         map[int]*CardState{0: {}},
		CardRevisions: map[int]int{0: 3},
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
		if got := NormalizeReaderPage(tc.page, tc.count, tc.twoPage); got != tc.want {
			t.Errorf("NormalizeReaderPage(%d, %d, %v) = %d, want %d", tc.page, tc.count, tc.twoPage, got, tc.want)
		}
	}

	if got := ReaderLastViewStart(10, true); got != 8 {
		t.Fatalf("last spread starts at %d, want 8", got)
	}
	if ReaderCanGoNext(8, 10, true) {
		t.Fatal("the final spread was treated as having a next view")
	}
	if !ReaderCanGoNext(6, 10, true) {
		t.Fatal("the penultimate spread was treated as final")
	}
	if got := (&ReaderState{Page: 8, PageCount: 10, TwoPage: true}).ProgressPage(); got != 9 {
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
			if got := ReaderSidePageDelta(tc.x, 100, tc.direction); got != tc.want {
				t.Fatalf("page delta = %d, want %d", got, tc.want)
			}
		})
	}
	if got := ReaderSpatialPageDelta(-1, model.ReadingDirectionRTL); got != 1 {
		t.Fatalf("RTL left button delta = %d, want 1", got)
	}
	if got := ReaderSpatialPageDelta(1, model.ReadingDirectionRTL); got != -1 {
		t.Fatalf("RTL right button delta = %d, want -1", got)
	}
}

func TestReaderDirectionControlsArrowKeys(t *testing.T) {
	if got := ReaderKeyPageDelta(key.NameRightArrow, model.ReadingDirectionLTR); got != 1 {
		t.Fatalf("LTR right arrow delta = %d, want 1", got)
	}
	if got := ReaderKeyPageDelta(key.NameRightArrow, model.ReadingDirectionRTL); got != -1 {
		t.Fatalf("RTL right arrow delta = %d, want -1", got)
	}
	if got := ReaderKeyPageDelta(key.NameLeftArrow, model.ReadingDirectionRTL); got != 1 {
		t.Fatalf("RTL left arrow delta = %d, want 1", got)
	}
	if got := ReaderKeyPageDelta(key.NameDownArrow, model.ReadingDirectionRTL); got != 1 {
		t.Fatalf("down arrow delta = %d, want 1", got)
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
	u.Settings.LibraryRoots = []string{filepath.Join(t.TempDir(), "Comics")}
	u.Top.rootRemove = make([]widget.Clickable, 1)
	u.Indexed = 318

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
	u.Settings.LibraryRoots = []string{filepath.Join(t.TempDir(), "Comics")}
	u.Top.rootRemove = make([]widget.Clickable, 1)
	u.Indexed = 318

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
		if u.Top.importToggle.Hovered() {
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
	if !u.ImportMenu {
		t.Fatal("Import click did not open the import menu")
	}
	if u.ImportMenuArmed {
		t.Fatal("Import menu became interactive in the click frame")
	}
	frame()
	if !u.ImportMenuArmed {
		t.Fatal("Import menu did not become visible on the following frame")
	}
}

func TestEditorClickPastTextMovesCaretToEnd(t *testing.T) {
	u := NewUI(nil)
	editor := widget.Editor{SingleLine: true}
	editor.SetText("abc")
	router := new(input.Router)
	ops := new(op.Ops)
	frame := func() {
		gtx := layout.Context{
			Ops: ops, Source: router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(300, 36)),
		}
		u.Editor(gtx, &editor, "", 36)
		router.Frame(ops)
		ops.Reset()
	}
	frame()
	router.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(220, 18),
	})
	frame()
	if _, col := editor.CaretPos(); col != 3 {
		t.Fatalf("caret column = %d after right-side click, want 3", col)
	}
}

func TestPointerShieldBlocksUnderlyingHover(t *testing.T) {
	u := NewUI(nil)
	router, ops := new(input.Router), new(op.Ops)
	var under, shield widget.Clickable
	frame := func() {
		gtx := layout.Context{Ops: ops, Source: router.Source(), Constraints: layout.Exact(image.Pt(100, 100))}
		under.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
		u.BlockPointer(gtx, &shield)
		router.Frame(ops)
		ops.Reset()
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(50, 50)})
	frame()
	if under.Hovered() {
		t.Fatal("pointer shield allowed hover to reach the underlying control")
	}
}
