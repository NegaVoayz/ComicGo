package screens

import (
	"comicgo/internal/model"
	. "comicgo/internal/ui"
	"comicgo/internal/ui/style"
	"fmt"
	"image"
	"image/color"

	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

func (u *Renderer) layoutReaderStage(gtx layout.Context) layout.Dimensions {
	for {
		ev, ok := u.Reader.StageClick.Update(gtx.Source)
		if !ok {
			break
		}
		if ev.Kind != gesture.KindClick {
			continue
		}
		if delta := ReaderSidePageDelta(ev.Position.X, gtx.Constraints.Max.X, u.Reader.Direction); delta != 0 {
			pages := ReaderPageStep(u.Reader.TwoPage)
			if delta < 0 {
				pages = -pages
			}
			u.goToReaderPage(u.Reader.Page + pages)
		}
	}

	stage := gtx
	stage.Constraints.Min = stage.Constraints.Max
	paint.Fill(stage.Ops, colorReaderBG)
	switch {
	case u.Reader.VisiblePageCount() == 0:
		layout.Center.Layout(stage, func(gtx layout.Context) layout.Dimensions {
			return u.Label(gtx, "No page selected.", unit.Sp(14), colDim, 1)
		})
	case u.Reader.VisiblePageCount() == 1:
		layout.Center.Layout(stage, func(gtx layout.Context) layout.Dimensions {
			return u.layoutReaderPage(stage, 0)
		})
	default:
		order := [ReaderPageSlots]int{0, 1}
		if u.Reader.Direction == model.ReadingDirectionRTL {
			order = [ReaderPageSlots]int{1, 0}
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
	u.Reader.StageClick.Add(gtx.Ops)
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (u *Renderer) layoutReaderPage(gtx layout.Context, slot int) layout.Dimensions {
	if slot < 0 || slot >= u.Reader.VisiblePageCount() {
		return layout.Dimensions{}
	}
	switch {
	case u.Reader.Errors[slot] != "":
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return u.Label(gtx, "Could not load this page.\n"+u.Reader.Errors[slot], unit.Sp(14), colFav, 3)
		})
	case u.Reader.Images[slot] == nil:
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return u.Label(gtx, "Loading page...", unit.Sp(14), colDim, 1)
		})
	default:
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return widget.Image{
				Src:   paint.NewImageOp(u.Reader.Images[slot]),
				Fit:   widget.Contain,
				Scale: 1 / gtx.Metric.PxPerDp,
			}.Layout(gtx)
		})
	}
}

var colorReaderBG = style.ReaderBG

func (u *Renderer) readerPageSummary() string {
	if u.Reader.PageCount <= 0 {
		return "page 0 / 0"
	}
	lastPage := min(u.Reader.Page+u.Reader.VisiblePageCount(), u.Reader.PageCount)
	progress := lastPage * 100 / u.Reader.PageCount
	if u.Reader.TwoPage && lastPage > u.Reader.Page+1 {
		return fmt.Sprintf("pages %d-%d / %d - %d%%", u.Reader.Page+1, lastPage, u.Reader.PageCount, progress)
	}
	return fmt.Sprintf("page %d / %d - %d%%", u.Reader.Page+1, u.Reader.PageCount, progress)
}

func (u *Renderer) layoutReaderControls(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(54))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	defer clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, colBGSoft)
	paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())
	u.Reader.PageFocused = gtx.Focused(&u.Reader.PageEditor)
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Gap: 8, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.readerFooterButton(gtx, &u.Reader.Back, "< Library", 84, ButtonStyle{})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{Alignment: layout.W}.Layout(gtx,
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						defer op.Offset(image.Pt(0, gtx.Dp(unit.Dp(6)))).Push(gtx.Ops).Pop()
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.Label(gtx, u.Reader.Title, unit.Sp(13), colText, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.Label(gtx, u.readerPageSummary(), unit.Sp(11), colDim, 1)
							}),
						)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(56)
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				gtx.Constraints.Min.Y = gtx.Dp(34)
				gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
				return u.Editor(gtx, &u.Reader.PageEditor, "Page", 34)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.readerFooterButton(gtx, &u.Reader.GoButton, "Go", 42, ButtonStyle{})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.readerSegmentControl(gtx, unit.Dp(132),
					&u.Reader.OnePage, "1 page", !u.Reader.TwoPage,
					&u.Reader.TwoPages, "2 pages", u.Reader.TwoPage,
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.readerSegmentControl(gtx, unit.Dp(112),
					&u.Reader.Ltr, "LTR", u.Reader.Direction == model.ReadingDirectionLTR,
					&u.Reader.Rtl, "RTL", u.Reader.Direction == model.ReadingDirectionRTL,
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				leftDelta := ReaderSpatialPageDelta(-1, u.Reader.Direction)
				return u.readerFooterButton(gtx, &u.Reader.Left, "<", 40, ButtonStyle{
					Disabled: !ReaderCanMove(u.Reader.Page, u.Reader.PageCount, u.Reader.TwoPage, leftDelta),
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				rightDelta := ReaderSpatialPageDelta(1, u.Reader.Direction)
				return u.readerFooterButton(gtx, &u.Reader.Right, ">", 40, ButtonStyle{
					Disabled: !ReaderCanMove(u.Reader.Page, u.Reader.PageCount, u.Reader.TwoPage, rightDelta),
				})
			}),
		)
	})
}

func (u *Renderer) readerFooterButton(gtx layout.Context, click *widget.Clickable, text string, width unit.Dp, style ButtonStyle) layout.Dimensions {
	size := image.Pt(gtx.Dp(width), gtx.Dp(unit.Dp(32)))
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	style.Centered = true
	return u.Button(gtx, click, text, style)
}

func (u *Renderer) readerSegmentControl(gtx layout.Context, width unit.Dp, first *widget.Clickable, firstLabel string, firstActive bool, second *widget.Clickable, secondLabel string, secondActive bool) layout.Dimensions {
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

func (u *Renderer) readerSegment(gtx layout.Context, click *widget.Clickable, text string, active, first bool) layout.Dimensions {
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
			return u.Label(gtx, text, unit.Sp(11), labelColor, 1)
		})
		return layout.Dimensions{Size: size}
	})
}

func (u *Renderer) handleReaderKey(ev key.Event) bool {
	if u.Screen != ScreenReader || u.Reader.PageFocused {
		return false
	}
	if delta := ReaderKeyPageDelta(ev.Name, u.Reader.Direction); delta != 0 {
		if delta < 0 {
			u.goToReaderPage(u.Reader.Page - ReaderPageStep(u.Reader.TwoPage))
		} else {
			u.goToReaderPage(u.Reader.Page + ReaderPageStep(u.Reader.TwoPage))
		}
		return true
	}
	switch ev.Name {
	case key.NameHome:
		u.goToReaderPage(0)
		return true
	case key.NameEnd:
		u.goToReaderPage(ReaderLastViewStart(u.Reader.PageCount, u.Reader.TwoPage))
		return true
	}
	return false
}

func (u *Renderer) handleReaderKeyEvents(gtx layout.Context) {
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
			u.HandleKey(ke)
		}
	}
}
