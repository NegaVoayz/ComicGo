package screens

import (
	"comicgo/internal/model"
	. "comicgo/internal/ui"
	"fmt"
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
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
			return u.Label(gtx, author, unit.Sp(14), colAccent, 2)
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

func (u *Renderer) openMetadataEditor() {
	m := u.Detail.Detail.Meta
	u.Detail.NameEditor.SetText(m.Title())
	u.Detail.AuthorEditor.SetText(m.KV["Author"])
	u.Detail.TagsEditor.SetText(m.KV["Genre"])
	u.Detail.SourceEditor.SetText(m.KV["Source"])
	u.Detail.SaveErr = ""
	u.Detail.EditOpen = true
}

func SplitTags(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" && !seen[part] {
			seen[part] = true
			out = append(out, part)
		}
	}
	return out
}

func (u *Renderer) layoutMetadataEditor(gtx layout.Context) {
	if u.Detail.Cancel.Clicked(gtx) {
		u.Detail.EditOpen = false
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
		go func() {
			detail, err := u.Backend.UpdateComicMetadata(id, edit)
			u.Post(DetailSavedEvent{Id: id, Detail: detail, Err: err})
		}()
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
					}{
						{"Title", &u.Detail.NameEditor},
						{"Author", &u.Detail.AuthorEditor},
						{"Tags (comma-separated)", &u.Detail.TagsEditor},
						{"Source", &u.Detail.SourceEditor},
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
}
