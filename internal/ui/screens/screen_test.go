package screens

import (
	"comicgo/internal/model"
	. "comicgo/internal/ui"

	"image"
	"slices"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

func clickHovered(t *testing.T, router *input.Router, ops *op.Ops, frame func(), bounds image.Rectangle, step int, hovered func() bool) {
	t.Helper()
	frame()
	var position f32.Point
	for y := bounds.Min.Y; y < bounds.Max.Y && position == (f32.Point{}); y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			probe := f32.Pt(float32(x), float32(y))
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: probe})
			frame()
			if hovered() {
				position = probe
				break
			}
		}
	}
	if position == (f32.Point{}) {
		t.Fatal("could not locate hover target")
	}
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
	frame()
	frame()
}

func TestTagQueryEscaping(t *testing.T) {
	u := renderer(&UI{
		Search:  `space  opera`,
		Tags:    []string{`quote " slash \`},
		Authors: []string{`author " slash \`},
	})
	got := u.query()
	want := `space  opera tag:"quote \" slash \\" author:"author \" slash \\"`
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

func TestAuthorOptionsFilterAndDeduplicateTypedValue(t *testing.T) {
	ui := NewUI(nil)
	ui.Detail.AuthorEditor.SetText(" current ")
	ui.Detail.AuthorValues = []string{"Alpha", "alpha", "Current", "Beta"}
	got := renderer(ui).authorOptions()
	want := []string{"current"}
	if !slices.Equal(got, want) {
		t.Fatalf("author options = %q, want %q", got, want)
	}
}

func TestTagOptionsFilterTypedValueAndExcludeSelected(t *testing.T) {
	ui := NewUI(nil)
	ui.Detail.TagsEditor.SetText("be")
	ui.Detail.Tags = []string{"Beta"}
	ui.Detail.TagValues = []string{"Alpha", "Beta", "Better", "Comedy"}
	got := renderer(ui).tagOptions()
	if strings.Join(got, "|") != "Better" {
		t.Fatalf("tag options = %q, want Better", got)
	}
}

func TestMetadataPickerUsesThreeColumns(t *testing.T) {
	if got := pickerGridRows(7); got != 3 {
		t.Fatalf("picker rows for 7 values = %d, want 3", got)
	}
}

func TestMetadataPickerOpensForFocusedEditor(t *testing.T) {
	ui := NewUI(nil)
	ui.Detail.EditOpen = true
	router := new(input.Router)
	ops := new(op.Ops)
	frame := func(focus *widget.Editor) {
		gtx := layout.Context{
			Ops: ops, Source: router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1200, 820)),
		}
		if focus != nil {
			gtx.Execute(key.FocusCmd{Tag: focus})
		}
		renderer(ui).layoutMetadataEditor(gtx)
		router.Frame(ops)
		ops.Reset()
	}
	frame(&ui.Detail.AuthorEditor)
	frame(nil)
	if !ui.Detail.AuthorOpen || ui.Detail.PickerKind != "author" {
		t.Fatalf("author focus opened picker %q (open=%v)", ui.Detail.PickerKind, ui.Detail.AuthorOpen)
	}
	frame(&ui.Detail.TagsEditor)
	frame(nil)
	if !ui.Detail.AuthorOpen || ui.Detail.PickerKind != "tag" || ui.Detail.PickerRight >= 850 {
		t.Fatalf("tag focus opened picker %q (open=%v)", ui.Detail.PickerKind, ui.Detail.AuthorOpen)
	}
}

func TestAuthorChipFiltersInsteadOfOpeningDetail(t *testing.T) {
	ui := NewUI(nil)
	const id = int64(7)
	ui.Covers[id] = image.NewRGBA(image.Rect(0, 0, 1, 1))
	card := NewCardState(model.ComicCard{
		ID: id, Title: "Example", Author: "Artist", PageCount: 1,
	})
	router := new(input.Router)
	ops := new(op.Ops)
	frame := func() {
		gtx := layout.Context{
			Ops: ops, Source: router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(200, 342)),
		}
		renderer(ui).layoutCard(gtx, card, 200)
		router.Frame(ops)
		ops.Reset()
	}
	clickHovered(t, router, ops, frame, image.Rect(0, 270, 200, 342), 2, card.AuthorClick.Hovered)
	if len(ui.Authors) != 1 || ui.Authors[0] != "Artist" {
		t.Fatalf("author filters = %q, want Artist", ui.Authors)
	}
	if ui.Screen != ScreenShelf || ui.Detail.Id != 0 {
		t.Fatalf("author click opened detail screen: screen=%v detail=%d", ui.Screen, ui.Detail.Id)
	}
}

func TestMetadataTagsAddRemoveAndDeduplicate(t *testing.T) {
	ui := NewUI(nil)
	ui.Detail.Tags = []string{"Alpha"}
	r := renderer(ui)
	if !r.addDraftTags("alpha, Beta、beta，Gamma") {
		t.Fatal("new tags were not added")
	}
	want := []string{"Alpha", "Beta", "Gamma"}
	if !slices.Equal(ui.Detail.Tags, want) {
		t.Fatalf("tags = %q, want %q", ui.Detail.Tags, want)
	}
	r.removeDraftTag(1)
	if strings.Join(ui.Detail.Tags, "|") != "Alpha|Gamma" {
		t.Fatalf("tags after removal = %q", ui.Detail.Tags)
	}
}

func TestMetadataTagOptionsExcludeSelectedCaseInsensitively(t *testing.T) {
	ui := NewUI(nil)
	ui.Detail.Tags = []string{"Alpha", "gamma"}
	ui.Detail.TagValues = []string{"alpha", "Beta", "Gamma", "Delta"}
	got := renderer(ui).tagOptions()
	if strings.Join(got, "|") != "Beta|Delta" {
		t.Fatalf("tag options = %q, want Beta|Delta", got)
	}
}

func TestDetailAuthorChipFiltersInsteadOfStayingOnDetail(t *testing.T) {
	ui := NewUI(nil)
	ui.Screen = ScreenDetail
	ui.Detail.Id = 9
	ui.Detail.Loading = false
	ui.Detail.Detail.Meta.KV = map[string]string{"Author": "Tanaka Yoshi"}
	ui.Detail.Detail.Meta.PageCount = 1
	router := new(input.Router)
	ops := new(op.Ops)
	frame := func() {
		gtx := layout.Context{
			Ops: ops, Source: router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1200, 820)),
		}
		renderer(ui).layoutDetail(gtx)
		router.Frame(ops)
		ops.Reset()
	}
	clickHovered(t, router, ops, frame, image.Rect(560, 80, 1100, 600), 4, ui.Detail.AuthorClick.Hovered)
	if strings.Join(ui.Authors, "|") != "Tanaka Yoshi" {
		t.Fatalf("author filters = %q, want Tanaka Yoshi", ui.Authors)
	}
	if ui.Screen != ScreenShelf {
		t.Fatalf("screen = %v after author click, want shelf", ui.Screen)
	}
}
