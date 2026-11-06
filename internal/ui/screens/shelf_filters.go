package screens

import (
	. "comicgo/internal/ui"
	"fmt"
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

func (u *Renderer) layoutSearchTag(gtx layout.Context, remove *widget.Clickable, tag string) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			paint.FillShape(gtx.Ops, colAccent, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
			inner := image.Rect(1, 1, max(1, size.X-1), max(1, size.Y-1))
			paint.FillShape(gtx.Ops, colAccentBG, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(5), Right: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Gap: 3, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.Label(gtx, tag, unit.Sp(12), colAccent, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.searchTagRemoveButton(gtx, remove)
					}),
				)
			})
		},
	)
}

func (u *Renderer) searchTagRemoveButton(gtx layout.Context, click *widget.Clickable) layout.Dimensions {
	size := gtx.Dp(unit.Dp(18))
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = gtx.Constraints.Min
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if click.Hovered() {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 0xd9, G: 0x6c, B: 0x6c, A: 0x24}, clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Min}, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
		}
		col := colAccent
		if click.Hovered() {

			col = colFav
		}
		center := float32(size) / 2
		radius := float32(gtx.Dp(unit.Dp(4)))
		var path clip.Path
		path.Begin(gtx.Ops)
		path.MoveTo(f32.Pt(center-radius, center-radius))
		path.LineTo(f32.Pt(center+radius, center+radius))
		path.MoveTo(f32.Pt(center+radius, center-radius))
		path.LineTo(f32.Pt(center-radius, center+radius))
		paint.FillShape(gtx.Ops, col, clip.Stroke{
			Path:  path.End(),
			Width: float32(max(1, gtx.Dp(unit.Dp(1.4)))),
		}.Op())
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

func (u *Renderer) layoutGrid(gtx layout.Context, s *SectionState, height int) layout.Dimensions {
	height = GridViewportHeight(height, gtx.Constraints.Max.X, s.Total, gtx.Dp(GridMinCard), gtx.Dp(GridGap), gtx.Dp(GridPad))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	paint.FillShape(gtx.Ops, colLine, clip.RRect{
		Rect: image.Rect(0, 0, gtx.Constraints.Max.X, height),
		NE:   8,
		NW:   8,
	}.Op(gtx.Ops))
	return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.layoutGridContent(gtx, s)
	})
}

func (u *Renderer) layoutGridContent(gtx layout.Context, s *SectionState) layout.Dimensions {
	paint.FillShape(gtx.Ops, colBG, clip.Rect{Max: gtx.Constraints.Max}.Op())
	cols, cardW, _, rowH := GridGeometry(gtx.Constraints.Max.X, gtx.Dp(GridMinCard), gtx.Dp(GridGap), gtx.Dp(GridPad))
	rows := 0
	if s.Total > 0 {
		rows = (s.Total + cols - 1) / cols
	}
	s.EnsureVisible(u.UI, rows, cols)
	dims := s.List.Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
		top := unit.Dp(0)
		if row == 0 {
			top = unit.Dp(GridPad)
		}
		bottom := unit.Dp(0)
		if row < rows-1 {
			bottom = unit.Dp(GridGap)
		}
		return layout.Inset{Top: top, Bottom: bottom, Left: unit.Dp(GridPad)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = cols*cardW + max(0, cols-1)*gtx.Dp(GridGap)
			gtx.Constraints.Max.X = gtx.Constraints.Min.X
			children := make([]layout.FlexChild, 0, cols)
			for col := 0; col < cols; col++ {
				idx := row*cols + col
				if idx >= s.Total {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = cardW
						gtx.Constraints.Max.X = cardW
						return layout.Dimensions{Size: image.Pt(cardW, rowH-gtx.Dp(GridGap))}
					}))
					continue
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = cardW
					gtx.Constraints.Max.X = cardW
					card := s.Cards[idx]
					if card == nil {
						return u.layoutCardPlaceholder(gtx, cardW)
					}
					return u.layoutCard(gtx, card, cardW)
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Gap: gtx.Dp(GridGap)}.Layout(gtx, children...)
		})
	})
	s.EnsureVisible(u.UI, rows, cols)
	if s.LastFirst != s.List.Position.First {
		s.LastFirst = s.List.Position.First
		u.Invalidate()
	}
	return dims
}

func (u *Renderer) layoutCardPlaceholder(gtx layout.Context, width int) layout.Dimensions {
	h := width * 4 / 3
	gtx.Constraints.Min = image.Pt(width, h+CaptionH)
	gtx.Constraints.Max = gtx.Constraints.Min
	paint.FillShape(gtx.Ops, colPanel, clip.RRect{Rect: image.Rect(0, 0, width, h), NE: 8, NW: 8, SE: 8, SW: 8}.Op(gtx.Ops))
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func (u *Renderer) layoutCard(gtx layout.Context, card *CardState, width int) layout.Dimensions {
	tagClicked := false
	for i := range card.TagClicks {
		if i < len(card.Card.Tags) && card.TagClicks[i].Clicked(gtx) {
			u.addTag(card.Card.Tags[i])
			tagClicked = true
		}
	}
	favoriteClicked := card.Favorite.Clicked(gtx)
	if card.Open.Clicked(gtx) && !tagClicked && !favoriteClicked {
		u.openDetail(card.Card)
	}
	if favoriteClicked {
		u.toggleFavorite(card)
	}
	coverH := width * 4 / 3
	gtx.Constraints.Min = image.Pt(width, coverH+CaptionH)
	gtx.Constraints.Max = gtx.Constraints.Min
	dims := card.Open.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.layoutCardCover(gtx, card, width, coverH)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.layoutCardMeta(gtx, card, width, CaptionH)
			}),
		)
	})
	return dims
}

func (u *Renderer) layoutCardCover(gtx layout.Context, card *CardState, width, height int) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	rect := image.Rect(0, 0, width, height)
	defer clip.RRect{Rect: rect, NE: 8, NW: 8, SE: 8, SW: 8}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, colPanel, clip.Rect{Max: image.Pt(width, height)}.Op())
	if img, ok := u.Covers[card.Card.ID]; ok {
		im := widget.Image{Src: paint.NewImageOp(img), Fit: widget.Cover, Position: layout.Center}
		if LandscapeLeftCrop(img.Bounds().Dx(), img.Bounds().Dy()) {
			im.Position = layout.W
		}
		im.Layout(gtx)
	} else {
		u.RequestCover(card.Card.ID)
		initials := TitleInitials(card.Card.Title)
		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return u.LabelColor(gtx, initials, unit.Sp(30), colDim, 1, font.Bold)
		})
	}
	if card.Card.HasNonUTF8 {
		layout.NE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.Label(gtx, "?", unit.Sp(12), colText, 1)
			})
		})
	}
	if card.Card.PageCount > 0 && card.Card.LastPage > 0 {
		progress := min(100, (card.Card.LastPage+1)*100/card.Card.PageCount)
		layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(5), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.Label(gtx, fmt.Sprintf("%d%%", progress), unit.Sp(10), color.NRGBA{R: 0xff, G: 0xe9, B: 0xc9, A: 0xff}, 1)
			})
		})
	}
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func (u *Renderer) layoutCardMeta(gtx layout.Context, card *CardState, width, height int) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	return layout.Flex{Axis: layout.Vertical, Gap: 1}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return u.Label(gtx, card.Card.Title, unit.Sp(13), colText, 1)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := "♡"
					if card.Card.Fav {
						label = "♥"
					}
					return u.layoutFavoriteButton(gtx, &card.Favorite, label == "♥")
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if card.Card.Author == "" {
				return layout.Dimensions{}
			}
			return u.Label(gtx, card.Card.Author, unit.Sp(11), colDim, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{}
			for i, tag := range card.Card.Tags {
				if i >= 3 {
					break
				}
				i, tag := i, tag
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.Button(gtx, &card.TagClicks[i], tag, ButtonStyle{Plain: true, Compact: true})
				}))
			}
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return u.Label(gtx, fmt.Sprintf("%d pages", card.Card.PageCount), unit.Sp(10), colDim, 1)
			}))
			return layout.Flex{Axis: layout.Horizontal, Gap: 4, Alignment: layout.Middle}.Layout(gtx, children...)
		}),
	)
}
