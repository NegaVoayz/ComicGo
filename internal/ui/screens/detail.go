package screens

import (
	"comicgo/internal/model"
	. "comicgo/internal/ui"
	"comicgo/internal/ui/metadata"
	"fmt"
	"image"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/outlay"
)

func (u *Renderer) layoutRuleMenuPanel(gtx layout.Context, s *SectionState, options []model.RuleInfo, width int) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	widthDp := unit.Dp(float32(width) / gtx.Metric.PxPerDp)
	return u.Panel(gtx, widthDp, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(options))
		for i, option := range options {
			i := i
			selected := s.FilterRuleID == option.ID
			if s.MenuKind == "sort" {
				selected = s.SortRuleID == option.ID
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				style := ButtonStyle{Plain: true, Centered: true}
				if selected {
					style = ButtonStyle{Primary: true, Centered: true}
				}
				return u.Button(gtx, &s.MenuClicks[i], option.Label, style)
			}))
		}
		return layout.Flex{Axis: layout.Vertical, Gap: 2}.Layout(gtx, children...)
	})
}

func (u *Renderer) layoutSectionError(gtx layout.Context, section model.LayoutSection) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(14), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.Label(gtx, fmt.Sprintf("Unsupported layout section %q (%s)", section.ID, section.Kind), unit.Sp(13), colDim, 2)
	})
}

type pluginCardRenderer func(*Renderer, layout.Context, map[string]any) layout.Dimensions

var cardRenderers = map[string]pluginCardRenderer{
	"stats": layoutStatsCard,
}

func (u *Renderer) layoutDetailMeta(gtx layout.Context, tags []string) layout.Dimensions {
	m := u.Detail.Detail.Meta
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.LabelColor(gtx, u.Detail.Title, unit.Sp(24), colText, 2, font.Bold)
		}),
	}
	if author := strings.TrimSpace(m.KV["Author"]); author != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.Button(gtx, &u.Detail.AuthorClick, "AUTHOR  "+author, ButtonStyle{Primary: true, Compact: true})
		}))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		progress := ""
		if m.PageCount > 0 && m.LastReadPage > 0 {
			progress = fmt.Sprintf(" - %d%% read", (m.LastReadPage+1)*100/m.PageCount)
		}
		return u.Label(gtx, fmt.Sprintf("%d pages%s", m.PageCount, progress), unit.Sp(13), colDim, 1)
	}))
	if len(tags) > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: 8}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.Label(gtx, "TAGS", unit.Sp(10), colDim, 1)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return outlay.FlowWrap{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, len(tags), func(gtx layout.Context, i int) layout.Dimensions {
						return layout.Inset{Right: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return u.Button(gtx, &u.Detail.TagClicks[i], tags[i], ButtonStyle{})
						})
					})
				}),
			)
		}))
	}
	if source := strings.TrimSpace(m.KV["Source"]); source != "" {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Gap: 8}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.Label(gtx, "SOURCE", unit.Sp(10), colDim, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Gap: 10, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return u.sourceField(gtx, source)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.Button(gtx, &u.Detail.Copy, "Copy", ButtonStyle{})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.Label(gtx, "Magnet link from the original download; paste it into a torrent client to re-fetch the file.", unit.Sp(12), colDim, 2)
					}),
				)
			}),
		)
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		label := "Open"
		if m.PageCount > 0 {
			if m.LastReadPage > 0 {
				label = fmt.Sprintf("Continue - page %d of %d", m.LastReadPage+1, m.PageCount)
			} else {
				label = fmt.Sprintf("Start reading - %d pages", m.PageCount)
			}
		}
		return u.Button(gtx, &u.Detail.Start, label, ButtonStyle{Solid: true, Disabled: m.PageCount <= 0})
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: 10}.Layout(gtx, children...)
}

func (u *Renderer) sourceField(gtx layout.Context, source string) layout.Dimensions {
	height := gtx.Dp(unit.Dp(34))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Max.X, height)
			paint.FillShape(gtx.Ops, colLine, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
			inner := image.Rect(1, 1, max(1, size.X-1), max(1, size.Y-1))
			paint.FillShape(gtx.Ops, colBG, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))

			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(9), Right: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.Label(gtx, source, unit.Sp(12), colDim, 1)
			})
		}),
	)
}

func SplitTags(s string) []string {
	return metadata.SplitTags(s)
}

func (u *Renderer) closePicker() {
	u.Detail.PickerKind = ""
	u.Detail.AuthorOpen = false
	u.Detail.PickerSuppressed = true
}

func (u *Renderer) openPicker(kind string, gtx layout.Context) {
	u.Detail.PickerKind = kind
	u.Detail.AuthorOpen = true
	u.Detail.PickerSuppressed = false
	contentWidth := gtx.Dp(unit.Dp(metadataContentWidth))
	u.Detail.PickerRight = (gtx.Constraints.Max.X + contentWidth) / 2
	if kind == "tag" {
		u.Detail.PickerRight -= u.Detail.TagAddWidth + gtx.Dp(unit.Dp(6))
	}
}

const (
	metadataContentWidth  = 500
	metadataFormTopOffset = 30
	metadataPickerColumns = 3
)

func pickerGridRows(total int) int {
	return max(1, (total+metadataPickerColumns-1)/metadataPickerColumns)
}

func (u *Renderer) valueMenuButton(gtx layout.Context, click *widget.Clickable, active bool) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(38)), gtx.Dp(unit.Dp(36)))
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		border := colLine
		if click.Hovered() || active {
			border = colAccent
		}
		paint.FillShape(gtx.Ops, border, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
		inner := image.Rect(1, 1, size.X-1, size.Y-1)
		paint.FillShape(gtx.Ops, colBGSoft, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
		cx, cy := float32(size.X/2), float32(size.Y/2)
		var path clip.Path
		path.Begin(gtx.Ops)
		path.MoveTo(f32.Pt(cx-4, cy-2))
		path.LineTo(f32.Pt(cx, cy+2))
		path.LineTo(f32.Pt(cx+4, cy-2))
		paint.FillShape(gtx.Ops, colText, clip.Stroke{Path: path.End(), Width: float32(max(1, gtx.Dp(unit.Dp(1.5))))}.Op())
		return layout.Dimensions{Size: size}
	})
}

func (u *Renderer) layoutPickerBackdrop(gtx layout.Context, modal image.Rectangle) {
	full := image.Rectangle{Max: gtx.Constraints.Max}
	areas := []struct {
		rect image.Rectangle
		pass bool
	}{
		{image.Rect(full.Min.X, full.Min.Y, full.Max.X, modal.Min.Y), false},
		{image.Rect(full.Min.X, modal.Max.Y, full.Max.X, full.Max.Y), false},
		{image.Rect(full.Min.X, modal.Min.Y, modal.Min.X, modal.Max.Y), false},
		{image.Rect(modal.Max.X, modal.Min.Y, full.Max.X, modal.Max.Y), false},
		{modal, true},
	}
	for _, area := range areas {
		if area.rect.Empty() {
			continue
		}
		areaGtx := gtx
		areaGtx.Constraints = layout.Exact(area.rect.Size())
		offset := op.Offset(area.rect.Min).Push(gtx.Ops)
		var pass pointer.PassStack
		if area.pass {
			pass = pointer.PassOp{}.Push(gtx.Ops)
		}
		u.Detail.AuthorClose.Layout(areaGtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: area.rect.Size()}
		})
		if area.pass {
			pass.Pop()
		}
		offset.Pop()
	}
}

func (u *Renderer) layoutAuthorMenu(gtx layout.Context, options []string) {
	y := u.Detail.PickerY + gtx.Dp(unit.Dp(4))
	available := max(gtx.Dp(unit.Dp(30)), gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(8))-y)
	macro := op.Record(gtx.Ops)
	menuDims := u.layoutAuthorMenuPanel(gtx, options, available)
	menu := macro.Stop()
	x := u.Detail.PickerRight - menuDims.Size.X
	if u.Detail.PickerRight == 0 {
		x = (gtx.Constraints.Max.X - menuDims.Size.X) / 2
	}
	x = max(gtx.Dp(unit.Dp(8)), min(x, gtx.Constraints.Max.X-menuDims.Size.X-gtx.Dp(unit.Dp(8))))
	y = max(gtx.Dp(unit.Dp(8)), y)
	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()
	menu.Add(gtx.Ops)
}

func (u *Renderer) layoutAuthorMenuPanel(gtx layout.Context, options []string, available int) layout.Dimensions {
	width := min(gtx.Dp(unit.Dp(460)), gtx.Constraints.Max.X-gtx.Dp(unit.Dp(16)))
	rowHeight := gtx.Dp(unit.Dp(30))
	rows := pickerGridRows(len(options))
	height := min(min(gtx.Dp(unit.Dp(240)), available), max(rowHeight, rows*rowHeight))
	widthDp := unit.Dp(float32(width) / gtx.Metric.PxPerDp)
	return u.Panel(gtx, widthDp, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = height
		gtx.Constraints.Max.Y = height
		if len(options) == 0 {
			message := "No authors indexed"
			if u.Detail.PickerKind == "tag" {
				message = "No more indexed tags"
			}
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.Label(gtx, message, unit.Sp(12), colDim, 1)
			})
		}
		u.Detail.AuthorList.Axis = layout.Vertical
		return u.Detail.AuthorList.Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
			children := make([]layout.FlexChild, 0, metadataPickerColumns)
			for col := 0; col < metadataPickerColumns; col++ {
				i := row*metadataPickerColumns + col
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if i >= len(options) {
						return layout.Dimensions{Size: image.Pt(0, rowHeight)}
					}
					style := ButtonStyle{Plain: true, Centered: true}
					if u.Detail.PickerKind == "author" && strings.EqualFold(options[i], u.Detail.AuthorEditor.Text()) {
						style = ButtonStyle{Primary: true, Centered: true}
					}
					gtx.Constraints.Min.Y = rowHeight
					gtx.Constraints.Max.Y = rowHeight
					return u.Button(gtx, &u.Detail.AuthorClicks[i], options[i], style)
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Gap: 2}.Layout(gtx, children...)
		})
	})
}
