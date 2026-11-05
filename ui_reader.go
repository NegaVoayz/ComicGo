package main

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"comicgo/internal/imgutil"

	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

type readerState struct {
	id          int64
	title       string
	page        int
	pageCount   int
	img         image.Image
	loading     bool
	err         string
	pageFocused bool
	back        widget.Clickable
	prev        widget.Clickable
	next        widget.Clickable
	goButton    widget.Clickable
	pageEditor  widget.Editor
	stageClick  gesture.Click
}

func (r *readerState) reset() {
	*r = readerState{}
}

func (u *appUI) startReader() {
	m := u.detail.detail.Meta
	if m.PageCount <= 0 {
		return
	}
	page := maxInt(0, minInt(m.LastReadPage, m.PageCount-1))
	u.reader = readerState{
		id:        m.ID,
		title:     m.Title(),
		page:      page,
		pageCount: m.PageCount,
		loading:   true,
	}
	u.reader.pageEditor.SingleLine = true
	u.reader.pageEditor.SetText(strconv.Itoa(page + 1))
	u.screen = screenReader
	u.requestReaderPage(page)
}

func (u *appUI) requestReaderPage(page int) {
	if page < 0 || page >= u.reader.pageCount {
		return
	}
	id := u.reader.id
	u.reader.loading = true
	go func() {
		b, _, err := u.backend.pageBytes(id, page)
		if err != nil {
			u.post(pageLoadedEvent{id: id, page: page, err: err})
			return
		}
		img, _, err := imgutil.Decode(b)
		u.post(pageLoadedEvent{id: id, page: page, img: img, err: err})
	}()
	u.warmReaderPage(page + 1)
	u.warmReaderPage(page - 1)
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

func (u *appUI) goToReaderPage(page int) {
	if u.reader.pageCount <= 0 {
		return
	}
	page = maxInt(0, minInt(page, u.reader.pageCount-1))
	if page == u.reader.page && u.reader.img != nil {
		return
	}
	u.reader.page = page
	u.reader.pageEditor.SetText(strconv.Itoa(page + 1))
	u.requestReaderPage(page)
	u.scheduleProgressSave()
	u.invalidate()
}

func (u *appUI) scheduleProgressSave() {
	id, page, count := u.reader.id, u.reader.page, u.reader.pageCount
	go func() {
		if err := u.backend.SetProgress(id, page, count); err != nil {
			u.post(scanErrorEvent{path: u.reader.title, err: err})
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
	if u.reader.prev.Clicked(gtx) {
		u.goToReaderPage(u.reader.page - 1)
	}
	if u.reader.next.Clicked(gtx) {
		u.goToReaderPage(u.reader.page + 1)
	}
	if u.reader.goButton.Clicked(gtx) {
		if n, err := strconv.Atoi(strings.TrimSpace(u.reader.pageEditor.Text())); err == nil {
			u.goToReaderPage(n - 1)
		} else {
			u.reader.pageEditor.SetText(strconv.Itoa(u.reader.page + 1))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.layoutReaderStage(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.layoutReaderControls(gtx)
		}),
	)
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
		if ev.Position.X < gtx.Constraints.Max.X*28/100 {
			u.goToReaderPage(u.reader.page - 1)
		} else if ev.Position.X > gtx.Constraints.Max.X*72/100 {
			u.goToReaderPage(u.reader.page + 1)
		}
	}
	paint.Fill(gtx.Ops, colorReaderBG)
	stage := gtx
	stage.Constraints.Min = stage.Constraints.Max
	dims := layout.Dimensions{Size: stage.Constraints.Max}
	switch {
	case u.reader.err != "":
		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, "Could not load this page.\n"+u.reader.err, unit.Sp(14), colFav, 3)
		})
	case u.reader.img == nil:
		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, "Loading page...", unit.Sp(14), colDim, 1)
		})
	default:
		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return widget.Image{
				Src:   paint.NewImageOp(u.reader.img),
				Fit:   widget.Contain,
				Scale: 1 / gtx.Metric.PxPerDp,
			}.Layout(gtx)
		})
	}
	rect := image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
	defer clip.Rect(rect).Push(gtx.Ops).Pop()
	u.reader.stageClick.Add(gtx.Ops)
	return dims
}

var colorReaderBG = color.NRGBA{R: 0x0c, G: 0x0d, B: 0x10, A: 0xff}

func (u *appUI) layoutReaderControls(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(54))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	defer clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, colBGSoft)
	paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())
	u.reader.pageFocused = gtx.Focused(&u.reader.pageEditor)
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		progress := 0
		if u.reader.pageCount > 0 {
			progress = (u.reader.page + 1) * 100 / u.reader.pageCount
		}
		return layout.Flex{Axis: layout.Horizontal, Gap: 10, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.reader.back, "< Library", buttonStyle{})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, u.reader.title, unit.Sp(13), colText, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, fmt.Sprintf("page %d / %d - %d%%", u.reader.page+1, u.reader.pageCount, progress), unit.Sp(11), colDim, 1)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(62)
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				return u.editor(gtx, &u.reader.pageEditor, "Page", 34)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.reader.goButton, "Go", buttonStyle{})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.reader.prev, "<", buttonStyle{disabled: u.reader.page <= 0})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.reader.next, ">", buttonStyle{disabled: u.reader.page >= u.reader.pageCount-1})
			}),
		)
	})
}

func (u *appUI) handleReaderKey(ev key.Event) bool {
	if u.screen != screenReader || u.reader.pageFocused {
		return false
	}
	switch ev.Name {
	case key.NameRightArrow, key.NameDownArrow, key.NamePageDown, key.NameSpace:
		u.goToReaderPage(u.reader.page + 1)
		return true
	case key.NameLeftArrow, key.NameUpArrow, key.NamePageUp:
		u.goToReaderPage(u.reader.page - 1)
		return true
	case key.NameHome:
		u.goToReaderPage(0)
		return true
	case key.NameEnd:
		u.goToReaderPage(u.reader.pageCount - 1)
		return true
	}
	return false
}
