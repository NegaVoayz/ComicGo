package screens

import (
	"comicgo/internal/model"
	. "comicgo/internal/ui"
	"image"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

func (u *Renderer) openMetadataEditor() {
	m := u.Detail.Detail.Meta
	u.Detail.NameEditor.SetText(m.Title())
	u.Detail.AuthorEditor.SetText(m.KV["Author"])
	u.Detail.TagsEditor.SetText(m.KV["Genre"])
	u.Detail.SourceEditor.SetText(m.KV["Source"])
	u.Detail.SaveErr = ""
	u.Detail.AuthorOpen = false
	u.Detail.AuthorValues = nil
	u.Detail.AuthorList = layout.List{Axis: layout.Vertical}
	u.Detail.EditOpen = true
	id := u.Detail.Id
	go func() {
		authors, err := u.Backend.ListAuthors()
		u.Post(AuthorsLoadedEvent{Id: id, Authors: authors, Err: err})
	}()
}

func (u *Renderer) handleAuthorsLoaded(ev AuthorsLoadedEvent) {
	if u.Detail.Id != ev.Id || !u.Detail.EditOpen {
		return
	}
	if ev.Err != nil {
		u.Toast("Load authors: "+ev.Err.Error(), ToastError)
		return
	}
	u.Detail.AuthorValues = append(u.Detail.AuthorValues[:0], ev.Authors...)
}

func (u *Renderer) authorOptions() []string {
	options := make([]string, 0, len(u.Detail.AuthorValues)+1)
	seen := map[string]bool{}
	add := func(author string) {
		author = strings.TrimSpace(author)
		key := strings.ToLower(author)
		if author == "" || seen[key] {
			return
		}
		seen[key] = true
		options = append(options, author)
	}
	add(u.Detail.AuthorEditor.Text())
	for _, author := range u.Detail.AuthorValues {
		add(author)
	}
	return options
}

func (u *Renderer) layoutMetadataEditor(gtx layout.Context) {
	if u.Detail.Cancel.Clicked(gtx) {
		u.Detail.EditOpen = false
		u.Detail.AuthorOpen = false
		u.Invalidate()
	}
	if u.Detail.Save.Clicked(gtx) && !u.DialogBusy {
		edit := model.MetaEdit{
			Title:  u.Detail.NameEditor.Text(),
			Author: u.Detail.AuthorEditor.Text(),
			Genre:  u.Detail.TagsEditor.Text(),
			Source: u.Detail.SourceEditor.Text(),
		}
		id := u.Detail.Id
		u.DialogBusy = true
		u.Detail.AuthorOpen = false
		go func() {
			detail, err := u.Backend.UpdateComicMetadata(id, edit)
			u.Post(DetailSavedEvent{Id: id, Detail: detail, Err: err})
		}()
	}
	options := u.authorOptions()
	if len(u.Detail.AuthorClicks) != len(options) {
		u.Detail.AuthorClicks = make([]widget.Clickable, len(options))
	}
	selected := false
	for i, author := range options {
		if u.Detail.AuthorClicks[i].Clicked(gtx) {
			u.Detail.AuthorEditor.SetText(author)
			u.Detail.AuthorOpen = false
			u.Invalidate()
			selected = true
			break
		}
	}
	if !selected && u.Detail.AuthorOpen && u.Detail.AuthorClose.Clicked(gtx) {
		u.Detail.AuthorOpen = false
		u.Invalidate()
	}
	if u.Detail.AuthorMenu.Clicked(gtx) {
		u.Detail.AuthorOpen = !u.Detail.AuthorOpen
		u.Detail.AuthorCenter = u.pressedControlCenter(&u.Detail.AuthorMenu, gtx.Dp(unit.Dp(38)))
		u.Detail.AuthorAnchor = u.Press.Position
		u.Invalidate()
	}

	layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.Fill(gtx.Ops, colBlack50)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.Modal(gtx, unit.Dp(560), func(gtx layout.Context) layout.Dimensions {
					fields := []struct {
						label  string
						editor *widget.Editor
						author bool
					}{
						{"Title", &u.Detail.NameEditor, false},
						{"Author", &u.Detail.AuthorEditor, true},
						{"Tags (comma-separated)", &u.Detail.TagsEditor, false},
						{"Source", &u.Detail.SourceEditor, false},
					}
					children := []layout.FlexChild{
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.LabelColor(gtx, "Edit metadata", unit.Sp(19), colText, 1, font.Bold)
						}),
					}
					for _, field := range fields {
						field := field
						children = append(children,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.Label(gtx, field.label, unit.Sp(11), colDim, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if field.author {
									return u.layoutAuthorSelector(gtx)
								}
								return u.Editor(gtx, field.editor, field.label, 36)
							}),
						)
					}
					if u.Detail.SaveErr != "" {
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.Label(gtx, u.Detail.SaveErr, unit.Sp(12), colFav, 2)
						}))
					}
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Gap: 8}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.Button(gtx, &u.Detail.Cancel, "Cancel", ButtonStyle{Disabled: u.DialogBusy})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := "Save"
								if u.DialogBusy {
									label = "Saving..."
								}
								return u.Button(gtx, &u.Detail.Save, label, ButtonStyle{Solid: true, Disabled: u.DialogBusy})
							}),
						)
					}))
					return layout.Flex{Axis: layout.Vertical, Gap: 7}.Layout(gtx, children...)
				})
			})
		}),
	)
	if u.Detail.AuthorOpen {
		u.layoutAuthorMenu(gtx, options)
	}
}

func (u *Renderer) layoutAuthorSelector(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Gap: 6, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.Editor(gtx, &u.Detail.AuthorEditor, "Author", 36)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.authorMenuButton(gtx)
		}),
	)
}

func (u *Renderer) authorMenuButton(gtx layout.Context) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(38)), gtx.Dp(unit.Dp(36)))
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	return u.Detail.AuthorMenu.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		border := colLine
		if u.Detail.AuthorMenu.Hovered() || u.Detail.AuthorOpen {
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
		paint.FillShape(gtx.Ops, colText, clip.Stroke{
			Path: path.End(), Width: float32(max(1, gtx.Dp(unit.Dp(1.5)))),
		}.Op())
		return layout.Dimensions{Size: size}
	})
}

func (u *Renderer) layoutAuthorMenu(gtx layout.Context, options []string) {
	macro := op.Record(gtx.Ops)
	menuDims := u.layoutAuthorMenuPanel(gtx, options)
	menu := macro.Stop()
	u.Detail.AuthorClose.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	x := u.Detail.AuthorCenter - menuDims.Size.X/2
	if u.Detail.AuthorCenter == 0 {
		x = (gtx.Constraints.Max.X - menuDims.Size.X) / 2
	}
	x = max(gtx.Dp(unit.Dp(8)), min(x, gtx.Constraints.Max.X-menuDims.Size.X-gtx.Dp(unit.Dp(8))))
	y := u.Detail.AuthorAnchor.Y + gtx.Dp(unit.Dp(18))
	if y+menuDims.Size.Y > gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(8)) {
		y = u.Detail.AuthorAnchor.Y - menuDims.Size.Y - gtx.Dp(unit.Dp(8))
	}
	y = max(gtx.Dp(unit.Dp(8)), y)
	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()
	menu.Add(gtx.Ops)
}

func (u *Renderer) layoutAuthorMenuPanel(gtx layout.Context, options []string) layout.Dimensions {
	width := min(gtx.Dp(unit.Dp(300)), gtx.Constraints.Max.X-gtx.Dp(unit.Dp(16)))
	rowHeight := gtx.Dp(unit.Dp(30))
	height := min(gtx.Dp(unit.Dp(240)), max(rowHeight, len(options)*rowHeight))
	widthDp := unit.Dp(float32(width) / gtx.Metric.PxPerDp)
	return u.Panel(gtx, widthDp, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = height
		gtx.Constraints.Max.Y = height
		if len(options) == 0 {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.Label(gtx, "No authors indexed", unit.Sp(12), colDim, 1)
			})
		}
		u.Detail.AuthorList.Axis = layout.Vertical
		return u.Detail.AuthorList.Layout(gtx, len(options), func(gtx layout.Context, i int) layout.Dimensions {
			style := ButtonStyle{Plain: true}
			if strings.EqualFold(options[i], u.Detail.AuthorEditor.Text()) {
				style = ButtonStyle{Primary: true}
			}
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return u.Button(gtx, &u.Detail.AuthorClicks[i], options[i], style)
		})
	})
}
