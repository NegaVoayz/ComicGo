package main

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"comicgo/internal/imgutil"
	"comicgo/internal/model"

	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

const readerPageSlots = 2

type readerState struct {
	id          int64
	title       string
	page        int
	pageCount   int
	direction   string
	twoPage     bool
	images      [readerPageSlots]image.Image
	loading     [readerPageSlots]bool
	errors      [readerPageSlots]string
	pageFocused bool

	back       widget.Clickable
	left       widget.Clickable
	right      widget.Clickable
	goButton   widget.Clickable
	onePage    widget.Clickable
	twoPages   widget.Clickable
	ltr        widget.Clickable
	rtl        widget.Clickable
	pageEditor widget.Editor
	stageClick gesture.Click
}

func (r *readerState) reset() {
	*r = readerState{}
}

func (r *readerState) visiblePageCount() int {
	if r.page < 0 || r.page >= r.pageCount {
		return 0
	}
	if r.twoPage && r.page+1 < r.pageCount {
		return 2
	}
	return 1
}

func (r *readerState) pageAt(slot int) int {
	if slot < 0 || slot >= r.visiblePageCount() {
		return -1
	}
	return r.page + slot
}

func (r *readerState) acceptsPage(id int64, slot, page int) bool {
	return id == r.id && slot >= 0 && slot < readerPageSlots && page == r.pageAt(slot)
}

func (r *readerState) progressPage() int {
	if r.pageCount <= 0 {
		return 0
	}
	return minInt(r.page+r.visiblePageCount()-1, r.pageCount-1)
}

func readerPageStep(twoPage bool) int {
	if twoPage {
		return 2
	}
	return 1
}

func normalizeReaderPage(page, pageCount int, twoPage bool) int {
	if pageCount <= 0 {
		return 0
	}
	page = maxInt(0, minInt(page, pageCount-1))
	if twoPage {
		page -= page % readerPageStep(true)
	}
	return page
}

func readerLastViewStart(pageCount int, twoPage bool) int {
	return normalizeReaderPage(pageCount-1, pageCount, twoPage)
}

func readerCanGoNext(page, pageCount int, twoPage bool) bool {
	return page >= 0 && page+readerPageStep(twoPage) < pageCount
}

func readerCanMove(page, pageCount int, twoPage bool, delta int) bool {
	if delta < 0 {
		return page > 0
	}
	return readerCanGoNext(page, pageCount, twoPage)
}

func readerSpatialPageDelta(side int, direction string) int {
	if side < 0 {
		if direction == model.ReadingDirectionRTL {
			return 1
		}
		return -1
	}
	if side > 0 {
		if direction == model.ReadingDirectionRTL {
			return -1
		}
		return 1
	}
	return 0
}

func readerSidePageDelta(x, width int, direction string) int {
	if x < width*28/100 {
		return readerSpatialPageDelta(-1, direction)
	}
	if x > width*72/100 {
		return readerSpatialPageDelta(1, direction)
	}
	return 0
}

func readerKeyPageDelta(name key.Name, direction string) int {
	switch name {
	case key.NameRightArrow:
		if direction == model.ReadingDirectionRTL {
			return -1
		}
		return 1
	case key.NameLeftArrow:
		if direction == model.ReadingDirectionRTL {
			return 1
		}
		return -1
	case key.NameDownArrow, key.NamePageDown, key.NameSpace:
		return 1
	case key.NameUpArrow, key.NamePageUp:
		return -1
	default:
		return 0
	}
}

func (u *appUI) startReader() {
	m := u.detail.detail.Meta
	if m.PageCount <= 0 {
		return
	}
	direction := model.NormalizeReadingDirection(u.settings.ReadingDirection)
	twoPage := u.settings.TwoPageSpread
	page := normalizeReaderPage(m.LastReadPage, m.PageCount, twoPage)
	u.reader = readerState{
		id:        m.ID,
		title:     m.Title(),
		page:      page,
		pageCount: m.PageCount,
		direction: direction,
		twoPage:   twoPage,
	}
	u.reader.pageEditor.SingleLine = true
	u.reader.pageEditor.SetText(strconv.Itoa(page + 1))
	u.screen = screenReader
	u.loadReaderView(page)
}

func (u *appUI) requestReaderPage(page, slot int) {
	if page < 0 || page >= u.reader.pageCount || slot < 0 || slot >= readerPageSlots {
		return
	}
	id := u.reader.id
	u.reader.loading[slot] = true
	go func() {
		b, _, err := u.backend.pageBytes(id, page)
		if err != nil {
			u.post(pageLoadedEvent{id: id, page: page, slot: slot, err: err})
			return
		}
		img, _, err := imgutil.Decode(b)
		u.post(pageLoadedEvent{id: id, page: page, slot: slot, img: img, err: err})
	}()
}

func (u *appUI) warmReaderPage(page int) {
	if page < 0 || page >= u.reader.pageCount {
		return
	}
	id := u.reader.id
	go func() {
		_, _, _ = u.backend.pageBytes(id, page)
	}()
}

func (u *appUI) loadReaderView(page int) {
	page = normalizeReaderPage(page, u.reader.pageCount, u.reader.twoPage)
	u.reader.page = page
	u.reader.images = [readerPageSlots]image.Image{}
	u.reader.loading = [readerPageSlots]bool{}
	u.reader.errors = [readerPageSlots]string{}
	u.reader.pageEditor.SetText(strconv.Itoa(page + 1))
	for slot := 0; slot < u.reader.visiblePageCount(); slot++ {
		u.requestReaderPage(u.reader.pageAt(slot), slot)
	}

	step := readerPageStep(u.reader.twoPage)
	next := page + step
	if next < u.reader.pageCount {
		u.warmReaderPage(next)
		if u.reader.twoPage {
			u.warmReaderPage(next + 1)
		}
	}
	previous := page - step
	if previous >= 0 {
		u.warmReaderPage(previous)
		if u.reader.twoPage {
			u.warmReaderPage(previous + 1)
		}
	}
}

func (u *appUI) goToReaderPage(page int) {
	if u.reader.pageCount <= 0 {
		return
	}
	page = normalizeReaderPage(page, u.reader.pageCount, u.reader.twoPage)
	if page == u.reader.page && u.reader.images[0] != nil {
		return
	}
	u.loadReaderView(page)
	u.scheduleProgressSave()
	u.invalidate()
}

func (u *appUI) setReaderTwoPage(twoPage bool) {
	if u.reader.twoPage == twoPage {
		return
	}
	oldTwoPage := u.reader.twoPage
	oldSettings := u.settings.TwoPageSpread
	u.reader.twoPage = twoPage
	u.settings.TwoPageSpread = twoPage
	if err := u.backend.SetReaderPreferences(u.reader.direction, twoPage); err != nil {
		u.reader.twoPage = oldTwoPage
		u.settings.TwoPageSpread = oldSettings
		u.toast("Save reader settings: "+err.Error(), toastError)
		return
	}
	oldPage := u.reader.page
	page := normalizeReaderPage(oldPage, u.reader.pageCount, twoPage)
	u.loadReaderView(page)
	if page != oldPage {
		u.scheduleProgressSave()
	}
	u.invalidate()
}

func (u *appUI) setReaderDirection(direction string) {
	direction = model.NormalizeReadingDirection(direction)
	if u.reader.direction == direction {
		return
	}
	oldDirection := u.reader.direction
	oldSettings := u.settings.ReadingDirection
	u.reader.direction = direction
	u.settings.ReadingDirection = direction
	if err := u.backend.SetReaderPreferences(direction, u.reader.twoPage); err != nil {
		u.reader.direction = oldDirection
		u.settings.ReadingDirection = oldSettings
		u.toast("Save reader settings: "+err.Error(), toastError)
		return
	}
	u.invalidate()
}

func (u *appUI) scheduleProgressSave() {
	id, page, count, title := u.reader.id, u.reader.progressPage(), u.reader.pageCount, u.reader.title
	go func() {
		if err := u.backend.SetProgress(id, page, count); err != nil {
			u.post(scanErrorEvent{path: title, err: err})
			return
		}
		u.post(layoutChangedEvent{})
	}()
}

func (u *appUI) layoutReader(gtx layout.Context) layout.Dimensions {
	if u.reader.back.Clicked(gtx) {
		u.closeReader()
		return layout.Dimensions{}
	}
	if u.reader.left.Clicked(gtx) {
		u.moveReaderSide(-1)
	}
	if u.reader.right.Clicked(gtx) {
		u.moveReaderSide(1)
	}
	if u.reader.onePage.Clicked(gtx) {
		u.setReaderTwoPage(false)
	}
	if u.reader.twoPages.Clicked(gtx) {
		u.setReaderTwoPage(true)
	}
	if u.reader.ltr.Clicked(gtx) {
		u.setReaderDirection(model.ReadingDirectionLTR)
	}
	if u.reader.rtl.Clicked(gtx) {
		u.setReaderDirection(model.ReadingDirectionRTL)
	}
	if u.reader.goButton.Clicked(gtx) {
		if n, err := strconv.Atoi(strings.TrimSpace(u.reader.pageEditor.Text())); err == nil {
			u.goToReaderPage(n - 1)
		} else {
			u.reader.pageEditor.SetText(strconv.Itoa(u.reader.page + 1))
		}
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
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

func (u *appUI) moveReaderSide(side int) {
	delta := readerSpatialPageDelta(side, u.reader.direction)
	if !readerCanMove(u.reader.page, u.reader.pageCount, u.reader.twoPage, delta) {
		return
	}
	pageDelta := readerPageStep(u.reader.twoPage)
	if delta < 0 {
		pageDelta = -pageDelta
	}
	u.goToReaderPage(u.reader.page + pageDelta)
}

func (u *appUI) layoutReaderStage(gtx layout.Context) layout.Dimensions {
	for {
		ev, ok := u.reader.stageClick.Update(gtx.Source)
		if !ok {
			break
		}
		if ev.Kind != gesture.KindClick {
			continue
		}
		if delta := readerSidePageDelta(ev.Position.X, gtx.Constraints.Max.X, u.reader.direction); delta != 0 {
			pages := readerPageStep(u.reader.twoPage)
			if delta < 0 {
				pages = -pages
			}
			u.goToReaderPage(u.reader.page + pages)
		}
	}

	stage := gtx
	stage.Constraints.Min = stage.Constraints.Max
	paint.Fill(stage.Ops, colorReaderBG)
	switch {
	case u.reader.visiblePageCount() == 0:
		layout.Center.Layout(stage, func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, "No page selected.", unit.Sp(14), colDim, 1)
		})
	case u.reader.visiblePageCount() == 1:
		layout.Center.Layout(stage, func(gtx layout.Context) layout.Dimensions {
			return u.layoutReaderPage(stage, 0)
		})
	default:
		order := [readerPageSlots]int{0, 1}
		if u.reader.direction == model.ReadingDirectionRTL {
			order = [readerPageSlots]int{1, 0}
		}
		layout.Flex{Axis: layout.Horizontal}.Layout(stage,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return u.layoutReaderPage(gtx, order[0])
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return u.layoutReaderPage(gtx, order[1])
			}),
		)
	}
	rect := image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
	defer clip.Rect(rect).Push(gtx.Ops).Pop()
	u.reader.stageClick.Add(gtx.Ops)
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (u *appUI) layoutReaderPage(gtx layout.Context, slot int) layout.Dimensions {
	if slot < 0 || slot >= u.reader.visiblePageCount() {
		return layout.Dimensions{}
	}
	switch {
	case u.reader.errors[slot] != "":
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, "Could not load this page.\n"+u.reader.errors[slot], unit.Sp(14), colFav, 3)
		})
	case u.reader.images[slot] == nil:
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, "Loading page...", unit.Sp(14), colDim, 1)
		})
	default:
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return widget.Image{
				Src:   paint.NewImageOp(u.reader.images[slot]),
				Fit:   widget.Contain,
				Scale: 1 / gtx.Metric.PxPerDp,
			}.Layout(gtx)
		})
	}
}

var colorReaderBG = color.NRGBA{R: 0x0c, G: 0x0d, B: 0x10, A: 0xff}

func (u *appUI) readerPageSummary() string {
	if u.reader.pageCount <= 0 {
		return "page 0 / 0"
	}
	lastPage := minInt(u.reader.page+u.reader.visiblePageCount(), u.reader.pageCount)
	progress := lastPage * 100 / u.reader.pageCount
	if u.reader.twoPage && lastPage > u.reader.page+1 {
		return fmt.Sprintf("pages %d-%d / %d - %d%%", u.reader.page+1, lastPage, u.reader.pageCount, progress)
	}
	return fmt.Sprintf("page %d / %d - %d%%", u.reader.page+1, u.reader.pageCount, progress)
}

func (u *appUI) layoutReaderControls(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(54))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	defer clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, colBGSoft)
	paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())
	u.reader.pageFocused = gtx.Focused(&u.reader.pageEditor)
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Gap: 8, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.readerFooterButton(gtx, &u.reader.back, "< Library", 84, buttonStyle{})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, u.reader.title, unit.Sp(13), colText, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, u.readerPageSummary(), unit.Sp(11), colDim, 1)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(56)
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				gtx.Constraints.Min.Y = gtx.Dp(34)
				gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
				return u.editor(gtx, &u.reader.pageEditor, "Page", 34)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.readerFooterButton(gtx, &u.reader.goButton, "Go", 42, buttonStyle{})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.readerSegmentControl(gtx, unit.Dp(132),
					&u.reader.onePage, "1 page", !u.reader.twoPage,
					&u.reader.twoPages, "2 pages", u.reader.twoPage,
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.readerSegmentControl(gtx, unit.Dp(112),
					&u.reader.ltr, "LTR", u.reader.direction == model.ReadingDirectionLTR,
					&u.reader.rtl, "RTL", u.reader.direction == model.ReadingDirectionRTL,
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				leftDelta := readerSpatialPageDelta(-1, u.reader.direction)
				return u.readerFooterButton(gtx, &u.reader.left, "<", 40, buttonStyle{
					disabled: !readerCanMove(u.reader.page, u.reader.pageCount, u.reader.twoPage, leftDelta),
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				rightDelta := readerSpatialPageDelta(1, u.reader.direction)
				return u.readerFooterButton(gtx, &u.reader.right, ">", 40, buttonStyle{
					disabled: !readerCanMove(u.reader.page, u.reader.pageCount, u.reader.twoPage, rightDelta),
				})
			}),
		)
	})
}

func (u *appUI) readerFooterButton(gtx layout.Context, click *widget.Clickable, text string, width unit.Dp, style buttonStyle) layout.Dimensions {
	size := image.Pt(gtx.Dp(width), gtx.Dp(unit.Dp(32)))
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	style.centered = true
	return u.button(gtx, click, text, style)
}

func (u *appUI) readerSegmentControl(gtx layout.Context, width unit.Dp, first *widget.Clickable, firstLabel string, firstActive bool, second *widget.Clickable, secondLabel string, secondActive bool) layout.Dimensions {
	size := image.Pt(gtx.Dp(width), gtx.Dp(unit.Dp(32)))
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	paint.FillShape(gtx.Ops, colLine, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 7, NW: 7, SE: 7, SW: 7}.Op(gtx.Ops))
	inner := image.Rect(1, 1, size.X-1, size.Y-1)
	paint.FillShape(gtx.Ops, colBGSoft, clip.RRect{Rect: inner, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
	defer clip.RRect{Rect: inner, NE: 6, NW: 6, SE: 6, SW: 6}.Push(gtx.Ops).Pop()
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.readerSegment(gtx, first, firstLabel, firstActive, true)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.readerSegment(gtx, second, secondLabel, secondActive, false)
		}),
	)
}

func (u *appUI) readerSegment(gtx layout.Context, click *widget.Clickable, text string, active, first bool) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Max
		defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
		bg := color.NRGBA{}
		switch {
		case active:
			bg = colAccentBG
		case click.Hovered():
			bg = colPanel2
		}
		if bg.A != 0 {
			paint.Fill(gtx.Ops, bg)
		}
		if !first {
			paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: image.Pt(1, size.Y)}.Op())
		}
		labelColor := colDim
		if active {
			labelColor = colAccent
		}
		labelGtx := gtx
		labelGtx.Constraints = layout.Exact(size)
		layout.Center.Layout(labelGtx, func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, text, unit.Sp(11), labelColor, 1)
		})
		return layout.Dimensions{Size: size}
	})
}

func (u *appUI) handleReaderKey(ev key.Event) bool {
	if u.screen != screenReader || u.reader.pageFocused {
		return false
	}
	if delta := readerKeyPageDelta(ev.Name, u.reader.direction); delta != 0 {
		if delta < 0 {
			u.goToReaderPage(u.reader.page - readerPageStep(u.reader.twoPage))
		} else {
			u.goToReaderPage(u.reader.page + readerPageStep(u.reader.twoPage))
		}
		return true
	}
	switch ev.Name {
	case key.NameHome:
		u.goToReaderPage(0)
		return true
	case key.NameEnd:
		u.goToReaderPage(readerLastViewStart(u.reader.pageCount, u.reader.twoPage))
		return true
	}
	return false
}

func (u *appUI) handleReaderKeyEvents(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: key.NameLeftArrow},
			key.Filter{Name: key.NameRightArrow},
			key.Filter{Name: key.NameUpArrow},
			key.Filter{Name: key.NameDownArrow},
			key.Filter{Name: key.NamePageUp},
			key.Filter{Name: key.NamePageDown},
			key.Filter{Name: key.NameSpace},
			key.Filter{Name: key.NameHome},
			key.Filter{Name: key.NameEnd},
			key.Filter{Name: key.NameEscape},
		)
		if !ok {
			return
		}
		if ke, ok := ev.(key.Event); ok {
			u.handleKey(ke)
		}
	}
}
