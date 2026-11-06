package ui

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (u *UI) Button(gtx layout.Context, click *widget.Clickable, label string, style ButtonStyle) layout.Dimensions {
	bg := colPanel2
	fg := colText
	border := colLine
	if style.Primary {
		bg = colAccentBG
		fg = colAccent
		border = colAccent
	}
	if style.Solid {
		bg = colAccent
		fg = color.NRGBA{R: 0x14, G: 0x10, B: 0x0a, A: 0xff}
		border = colAccent
	}
	if style.Plain {
		bg = color.NRGBA{}
		border = color.NRGBA{}
	}
	if style.Danger {
		fg = colFav
	}
	if style.Disabled {
		gtx = gtx.Disabled()
		fg = color.NRGBA{R: fg.R, G: fg.G, B: fg.B, A: 0x80}
	}
	if click.Hovered() && !style.Disabled {
		border = colAccent
	}
	inset := layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(9), Right: unit.Dp(9)}
	if style.Compact {
		inset = layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(3), Right: unit.Dp(3)}
	}
	if style.Centered {
		return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Max.X, maxInt(gtx.Constraints.Min.Y, gtx.Dp(unit.Dp(28))))
			paint.FillShape(gtx.Ops, border, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			if bg.A != 0 {
				paint.FillShape(gtx.Ops, bg, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
			}
			labelGtx := gtx
			labelGtx.Constraints = layout.Exact(size)
			layout.Center.Layout(labelGtx, func(gtx layout.Context) layout.Dimensions {
				return u.LabelColor(gtx, label, unit.Sp(12), fg, 1, font.Normal)
			})
			return layout.Dimensions{Size: size}
		})
	}
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.W}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				if size.X == 0 || size.Y == 0 {
					size = image.Pt(maxInt(size.X, gtx.Dp(unit.Dp(len(label)+4))), maxInt(size.Y, gtx.Dp(26)))
				}
				gtx.Constraints.Min = size
				if border.A != 0 {
					paint.FillShape(gtx.Ops, border, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
					inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
					if bg.A != 0 {
						paint.FillShape(gtx.Ops, bg, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
					}
				} else if bg.A != 0 {
					paint.FillShape(gtx.Ops, bg, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
				}
				return layout.Dimensions{Size: size}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.LabelColor(gtx, label, unit.Sp(12), fg, 1, font.Normal)
				})
			}),
		)
	})
}

func (u *UI) Label(gtx layout.Context, s string, size unit.Sp, col color.NRGBA, maxLines int) layout.Dimensions {
	return u.LabelColor(gtx, s, size, col, maxLines, font.Normal)
}

func (u *UI) LabelColor(gtx layout.Context, s string, size unit.Sp, col color.NRGBA, maxLines int, weight font.Weight) layout.Dimensions {
	l := material.Label(u.Theme, size, s)
	l.Color = col
	l.MaxLines = maxLines
	l.Truncator = "..."
	l.Font.Weight = weight
	return l.Layout(gtx)
}

func (u *UI) Editor(gtx layout.Context, editor *widget.Editor, hint string, height unit.Dp) layout.Dimensions {
	if editor.SingleLine {
		gtx.Constraints.Min.Y = gtx.Dp(height)
	}
	focus := u.editorFocusTag(editor)
	if gtx.Focused(editor) {
		u.FocusedEditor = editor
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	hitSize := gtx.Constraints.Max
	if editor.SingleLine {
		hitSize.Y = minInt(hitSize.Y, maxInt(gtx.Constraints.Min.Y, gtx.Dp(height)))
	}
	area := clip.Rect{Max: hitSize}.Push(gtx.Ops)
	event.Op(gtx.Ops, &focus.tag)
	area.Pop()
	pass.Pop()
	setCaretEnd := false
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &focus.tag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if press, ok := ev.(pointer.Event); ok && press.Kind == pointer.Press {
			u.Press.EditorHit = true
			if editor == &u.Detail.AuthorEditor || editor == &u.Detail.TagsEditor {
				u.Detail.PickerSuppressed = false
			}
			if press.Position.X > float32(gtx.Dp(unit.Dp(9))+u.editorTextWidth(gtx, editor.Text())) {
				end := editor.Len()
				editor.SetCaret(end, end)
				setCaretEnd = true
			}
			gtx.Execute(key.FocusCmd{Tag: editor})
			u.Invalidate()
		}
	}
	dims := layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Max.X, maxInt(gtx.Constraints.Min.Y, gtx.Dp(height)))
			paint.FillShape(gtx.Ops, colLine, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			paint.FillShape(gtx.Ops, colBGSoft, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			inset := layout.UniformInset(unit.Dp(7))
			if editor.SingleLine {
				inset = layout.Inset{Top: unit.Dp(9), Bottom: unit.Dp(5), Left: unit.Dp(9), Right: unit.Dp(9)}
			}
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				e := material.Editor(u.Theme, editor, hint)
				e.TextSize = 13
				e.Color = colText
				e.HintColor = colDim
				e.SelectionColor = colAccentBG
				return e.Layout(gtx)
			})
		}),
	)
	if setCaretEnd {
		// The editor can clamp clicks beyond short text to column zero. Restore
		// the intentional end placement after its pointer handling.
		end := editor.Len()
		editor.SetCaret(end, end)
	}
	return dims
}

func (u *UI) editorFocusTag(editor *widget.Editor) *editorFocusState {
	if u.EditorFocus == nil {
		u.EditorFocus = make(map[*widget.Editor]*editorFocusState)
	}
	focus := u.EditorFocus[editor]
	if focus == nil {
		focus = &editorFocusState{}
		u.EditorFocus[editor] = focus
	}
	return focus
}

func (u *UI) Panel(gtx layout.Context, width unit.Dp, content layout.Widget) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Dp(width)
	gtx.Constraints.Max.X = gtx.Dp(width)
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Max.X, maxInt(gtx.Constraints.Min.Y, gtx.Dp(40)))
			paint.FillShape(gtx.Ops, colLine, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 8, NW: 8, SE: 8, SW: 8}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			paint.FillShape(gtx.Ops, colPanel, clip.RRect{Rect: inner, NE: 7, NW: 7, SE: 7, SW: 7}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.UniformInset(unit.Dp(12)).Layout(gtx, content)
		}),
	)
}

func (u *UI) Modal(gtx layout.Context, width unit.Dp, content layout.Widget) layout.Dimensions {
	return u.Panel(gtx, width, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.UniformInset(unit.Dp(18)).Layout(gtx, content)
	})
}

func (u *UI) indexedBadge(gtx layout.Context, count int) layout.Dimensions {
	height := gtx.Dp(unit.Dp(26))
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Min.X, height)
			paint.FillShape(gtx.Ops, colAccent, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 10, NW: 10, SE: 10, SW: 10}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			paint.FillShape(gtx.Ops, colAccentBG, clip.RRect{Rect: inner, NE: 9, NW: 9, SE: 9, SW: 9}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = height
			gtx.Constraints.Max.Y = height
			return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(9), Right: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.Label(gtx, fmt.Sprintf("%d indexed", count), unit.Sp(12), colAccent, 1)
			})
		}),
	)
}

func (u *UI) layoutRootChip(gtx layout.Context, name string, remove *widget.Clickable) layout.Dimensions {
	height := gtx.Dp(unit.Dp(28))
	closeSize := gtx.Dp(unit.Dp(26))
	gtx.Constraints.Max.X = minInt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(230)))
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Min.X, height)
			border := colLine
			if remove.Hovered() {
				border = colFav
			}
			paint.FillShape(gtx.Ops, border, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			paint.FillShape(gtx.Ops, colPanel, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			gtx.Constraints.Max.Y = height
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Max.X = maxInt(0, gtx.Constraints.Max.X-closeSize)
						return u.Chrome.LayoutMoveArea(gtx, func(gtx layout.Context) layout.Dimensions {
							return u.Label(gtx, name, unit.Sp(12), colDim, 1)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.rootRemoveButton(gtx, remove, closeSize)
					}),
				)
			})
		}),
	)
}

func (u *UI) rootRemoveButton(gtx layout.Context, click *widget.Clickable, size int) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = gtx.Constraints.Min
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if click.Hovered() {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 0xd9, G: 0x6c, B: 0x6c, A: 0x24}, clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Min}, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
		}
		col := colDim
		if click.Hovered() {
			col = colFav
		}
		inset := float32(gtx.Dp(unit.Dp(7)))
		max := float32(size) - inset
		var path clip.Path
		path.Begin(gtx.Ops)
		path.MoveTo(f32.Pt(inset, inset))
		path.LineTo(f32.Pt(max, max))
		path.MoveTo(f32.Pt(max, inset))
		path.LineTo(f32.Pt(inset, max))
		paint.FillShape(gtx.Ops, col, clip.Stroke{
			Path:  path.End(),
			Width: float32(maxInt(1, gtx.Dp(unit.Dp(1.4)))),
		}.Op())
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}
