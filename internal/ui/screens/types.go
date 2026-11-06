package screens

import (
	. "comicgo/internal/ui"
	"comicgo/internal/ui/style"

	"gioui.org/io/key"
	"gioui.org/layout"
)

type Renderer struct{ *UI }

var (
	colDim      = style.Dim
	colFav      = style.Favorite
	colText     = style.Text
	colLine     = style.Line
	colPanel    = style.Panel
	colPanel2   = style.Panel2
	colBGSoft   = style.BGSoft
	colAccent   = style.Accent
	colAccentBG = style.AccentBG
	colBG       = style.BG
	colBlack50  = style.BlackHalf
)

type hooks struct{}

func init() { SetScreenHooks(hooks{}) }

func renderer(ui *UI) *Renderer { return &Renderer{UI: ui} }

func (hooks) ResetSections(ui *UI) { renderer(ui).resetSections() }
func (hooks) Query(ui *UI) string  { return renderer(ui).query() }
func (hooks) RequestSectionCount(ui *UI, index int) {
	renderer(ui).requestSectionCount(index)
}
func (hooks) RequestHero(ui *UI, id int64) { renderer(ui).requestHero(id) }
func (hooks) LayoutShelf(ui *UI, gtx layout.Context) layout.Dimensions {
	renderer(ui).layoutShelf(gtx)
	return layout.Dimensions{Size: gtx.Constraints.Max}
}
func (hooks) LayoutShelfBody(ui *UI, gtx layout.Context) layout.Dimensions {
	return renderer(ui).layoutShelfBody(gtx)
}
func (hooks) LayoutDetail(ui *UI, gtx layout.Context) layout.Dimensions {
	return renderer(ui).layoutDetail(gtx)
}
func (hooks) LayoutReader(ui *UI, gtx layout.Context) layout.Dimensions {
	return renderer(ui).layoutReader(gtx)
}
func (hooks) HandleReaderKey(ui *UI, event key.Event) bool {
	return renderer(ui).handleReaderKey(event)
}
func (hooks) LayoutMetadataEditor(ui *UI, gtx layout.Context) {
	renderer(ui).layoutMetadataEditor(gtx)
}
