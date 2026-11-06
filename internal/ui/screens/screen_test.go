package screens

import (
	. "comicgo/internal/ui"

	"image"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
)

func TestTagQueryEscaping(t *testing.T) {
	u := renderer(&UI{
		Search: `space  opera`,
		Tags:   []string{`quote " slash \`},
	})
	got := u.query()
	want := `space  opera tag:"quote \" slash \\"`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestStatsCardRendererRegistered(t *testing.T) {
	if cardRenderers["stats"] == nil {
		t.Fatal("stats Gio renderer is not registered")
	}
}

func TestRuleMenusAreExclusiveAndToggleClosed(t *testing.T) {
	first := &SectionState{MenuKind: "filter"}
	second := &SectionState{}
	u := renderer(&UI{Sections: []*SectionState{first, second}})

	u.toggleRuleMenu(second, "sort", image.Pt(100, 100))
	if first.MenuKind != "" || second.MenuKind != "sort" {
		t.Fatalf("open menus = %q, %q; want empty, sort", first.MenuKind, second.MenuKind)
	}
	u.toggleRuleMenu(second, "sort", image.Pt(100, 100))
	if second.MenuKind != "" {
		t.Fatalf("sort menu remained open after toggle: %q", second.MenuKind)
	}
}

func TestSearchSyncPreservesSourceCaret(t *testing.T) {
	source := &SectionState{}
	source.Search.SetText("ka")
	source.Search.SetCaret(2, 2)
	other := &SectionState{}
	u := renderer(&UI{Sections: []*SectionState{source, other}})

	u.syncSearchEditors("ka", source)

	if _, col := source.Search.CaretPos(); col != 2 {
		t.Fatalf("source caret column = %d, want 2", col)
	}
	if got := other.Search.Text(); got != "ka" {
		t.Fatalf("other search text = %q, want %q", got, "ka")
	}
}

func TestReaderAcceptsOnlyVisibleSpreadSlots(t *testing.T) {
	r := ReaderState{Id: 7, Page: 2, PageCount: 5, TwoPage: true}
	if !r.AcceptsPage(7, 0, 2) {
		t.Fatal("reader rejected the leading visible page")
	}
	if !r.AcceptsPage(7, 1, 3) {
		t.Fatal("reader rejected the trailing visible page")
	}
	if r.AcceptsPage(7, 1, 4) {
		t.Fatal("reader accepted a stale page for the second slot")
	}
	if r.AcceptsPage(8, 0, 2) {
		t.Fatal("reader accepted a page for another comic")
	}
}

func TestReaderKeyEventsReachReaderHandler(t *testing.T) {
	ui := renderer(&UI{})
	ui.Screen = ScreenReader
	router := new(input.Router)
	ops := new(op.Ops)

	frame := func() {
		gtx := layout.Context{
			Ops:         ops,
			Source:      router.Source(),
			Constraints: layout.Exact(image.Pt(100, 100)),
		}
		ui.handleReaderKeyEvents(gtx)
		router.Frame(ops)
		ops.Reset()
	}

	frame()
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()

	if ui.Screen != ScreenDetail {
		t.Fatalf("screen = %v after Escape, want detail", ui.Screen)
	}
}

func TestReaderFooterControlsUseExactHitBounds(t *testing.T) {
	u := renderer(NewUI(nil))
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Source:      new(input.Router).Source(),
		Constraints: layout.Exact(image.Pt(800, 54)),
	}
	var button, first, second widget.Clickable
	if got := u.readerFooterButton(gtx, &button, "Go", 42, ButtonStyle{}); got.Size != image.Pt(42, 32) {
		t.Fatalf("footer button size = %v, want 42x32", got.Size)
	}
	if got := u.readerSegmentControl(gtx, 132, &first, "1 page", true, &second, "2 pages", false); got.Size != image.Pt(132, 32) {
		t.Fatalf("segment control size = %v, want 132x32", got.Size)
	}
}
