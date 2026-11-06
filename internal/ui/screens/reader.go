package screens

import (
	"comicgo/internal/imgutil"
	"comicgo/internal/model"
	. "comicgo/internal/ui"
	"comicgo/internal/ui/windowing"
	"image"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func (u *Renderer) startReader() {
	m := u.Detail.Detail.Meta
	if m.PageCount <= 0 {
		return
	}
	direction := model.NormalizeReadingDirection(u.Settings.ReadingDirection)
	twoPage := u.Settings.TwoPageSpread
	page := NormalizeReaderPage(m.LastReadPage, m.PageCount, twoPage)
	u.Reader = ReaderState{
		Id:        m.ID,
		Title:     m.Title(),
		Page:      page,
		PageCount: m.PageCount,
		Direction: direction,
		TwoPage:   twoPage,
	}
	u.Reader.PageEditor.SingleLine = true
	u.Reader.PageEditor.SetText(strconv.Itoa(page + 1))
	u.Screen = ScreenReader
	u.loadReaderView(page)
}

func (u *Renderer) requestReaderPage(page, slot int) {
	if page < 0 || page >= u.Reader.PageCount || slot < 0 || slot >= ReaderPageSlots {
		return
	}
	id := u.Reader.Id
	u.Reader.Loading[slot] = true
	go func() {
		b, _, err := u.Backend.PageBytes(id, page)
		if err != nil {
			u.Post(PageLoadedEvent{Id: id, Page: page, Slot: slot, Err: err})
			return
		}
		img, _, err := imgutil.Decode(b)
		u.Post(PageLoadedEvent{Id: id, Page: page, Slot: slot, Img: img, Err: err})
	}()
}

func (u *Renderer) warmReaderPage(page int) {
	if page < 0 || page >= u.Reader.PageCount {
		return
	}
	id := u.Reader.Id
	go func() {
		_, _, _ = u.Backend.PageBytes(id, page)
	}()
}

func (u *Renderer) loadReaderView(page int) {
	page = NormalizeReaderPage(page, u.Reader.PageCount, u.Reader.TwoPage)
	u.Reader.Page = page
	u.Reader.Images = [ReaderPageSlots]image.Image{}
	u.Reader.Loading = [ReaderPageSlots]bool{}
	u.Reader.Errors = [ReaderPageSlots]string{}
	u.Reader.PageEditor.SetText(strconv.Itoa(page + 1))
	for slot := 0; slot < u.Reader.VisiblePageCount(); slot++ {
		u.requestReaderPage(u.Reader.PageAt(slot), slot)
	}

	step := ReaderPageStep(u.Reader.TwoPage)
	next := page + step
	if next < u.Reader.PageCount {
		u.warmReaderPage(next)
		if u.Reader.TwoPage {
			u.warmReaderPage(next + 1)
		}
	}
	previous := page - step
	if previous >= 0 {
		u.warmReaderPage(previous)
		if u.Reader.TwoPage {
			u.warmReaderPage(previous + 1)
		}
	}
}

func (u *Renderer) goToReaderPage(page int) {
	if u.Reader.PageCount <= 0 {
		return
	}
	page = NormalizeReaderPage(page, u.Reader.PageCount, u.Reader.TwoPage)
	if page == u.Reader.Page && u.Reader.Images[0] != nil {
		return
	}
	u.loadReaderView(page)
	u.scheduleProgressSave()
	u.Invalidate()
}

func (u *Renderer) setReaderTwoPage(twoPage bool) {
	if u.Reader.TwoPage == twoPage {
		return
	}
	oldTwoPage := u.Reader.TwoPage
	oldSettings := u.Settings.TwoPageSpread
	u.Reader.TwoPage = twoPage
	u.Settings.TwoPageSpread = twoPage
	if err := u.Backend.SetReaderPreferences(u.Reader.Direction, twoPage); err != nil {
		u.Reader.TwoPage = oldTwoPage
		u.Settings.TwoPageSpread = oldSettings
		u.Toast("Save reader settings: "+err.Error(), ToastError)
		return
	}
	oldPage := u.Reader.Page
	page := NormalizeReaderPage(oldPage, u.Reader.PageCount, twoPage)
	u.loadReaderView(page)
	if page != oldPage {
		u.scheduleProgressSave()
	}
	u.Invalidate()
}

func (u *Renderer) setReaderDirection(direction string) {
	direction = model.NormalizeReadingDirection(direction)
	if u.Reader.Direction == direction {
		return
	}
	oldDirection := u.Reader.Direction
	oldSettings := u.Settings.ReadingDirection
	u.Reader.Direction = direction
	u.Settings.ReadingDirection = direction
	if err := u.Backend.SetReaderPreferences(direction, u.Reader.TwoPage); err != nil {
		u.Reader.Direction = oldDirection
		u.Settings.ReadingDirection = oldSettings
		u.Toast("Save reader settings: "+err.Error(), ToastError)
		return
	}
	u.Invalidate()
}

func (u *Renderer) scheduleProgressSave() {
	id, page, count, title := u.Reader.Id, u.Reader.ProgressPage(), u.Reader.PageCount, u.Reader.Title
	go func() {
		if err := u.Backend.SetProgress(id, page, count); err != nil {
			u.Post(ScanErrorEvent{Path: title, Err: err})
			return
		}
		u.Post(LayoutChangedEvent{})
	}()
}

func (u *Renderer) layoutReader(gtx layout.Context) layout.Dimensions {
	if u.Reader.Back.Clicked(gtx) {
		u.CloseReader()
		return layout.Dimensions{}
	}
	if u.Reader.Left.Clicked(gtx) {
		u.moveReaderSide(-1)
	}
	if u.Reader.Right.Clicked(gtx) {
		u.moveReaderSide(1)
	}
	if u.Reader.OnePage.Clicked(gtx) {
		u.setReaderTwoPage(false)
	}
	if u.Reader.TwoPages.Clicked(gtx) {
		u.setReaderTwoPage(true)
	}
	if u.Reader.Ltr.Clicked(gtx) {
		u.setReaderDirection(model.ReadingDirectionLTR)
	}
	if u.Reader.Rtl.Clicked(gtx) {
		u.setReaderDirection(model.ReadingDirectionRTL)
	}
	if u.Reader.GoButton.Clicked(gtx) {
		if n, err := strconv.Atoi(strings.TrimSpace(u.Reader.PageEditor.Text())); err == nil {
			u.goToReaderPage(n - 1)
		} else {
			u.Reader.PageEditor.SetText(strconv.Itoa(u.Reader.Page + 1))
		}
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.layoutReaderCaption(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.layoutReaderStage(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.layoutReaderControls(gtx)
		}),
	)
	u.handleReaderKeyEvents(gtx)
	return dims
}

func (u *Renderer) layoutReaderCaption(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(36))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	defer clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, colBGSoft)
	paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())
	inset := windowing.CaptionContentInsets(36)
	inset.Top = 0
	inset.Bottom = 0
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.Chrome.LayoutMoveArea(gtx, func(gtx layout.Context) layout.Dimensions {
			layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.Label(gtx, "ComicGo  /  "+u.Reader.Title, unit.Sp(12), colDim, 1)
			})
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
	})
}

func (u *Renderer) moveReaderSide(side int) {
	delta := ReaderSpatialPageDelta(side, u.Reader.Direction)
	if !ReaderCanMove(u.Reader.Page, u.Reader.PageCount, u.Reader.TwoPage, delta) {
		return
	}
	pageDelta := ReaderPageStep(u.Reader.TwoPage)
	if delta < 0 {
		pageDelta = -pageDelta
	}
	u.goToReaderPage(u.Reader.Page + pageDelta)
}
