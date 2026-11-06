package screens

import (
	. "comicgo/internal/ui"
	"image"
	"strconv"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/outlay"
)

func (u *Renderer) resetSections() {
	u.Sections = u.Sections[:0]
	gridIndex := 0
	for _, section := range u.LayoutDoc.Sections {
		if section.Kind != "grid" {
			continue
		}
		s := &SectionState{
			Index:         gridIndex,
			LayoutSection: section,
			Cards:         make(map[int]*CardState),
			Pending:       make(map[int]bool),
			CardRevisions: make(map[int]int),
		}
		s.List.Axis = layout.Vertical
		s.Search.SingleLine = true
		s.Search.SetText(u.Search)
		s.LastSearch = u.Search
		u.Sections = append(u.Sections, s)
		gridIndex++
	}
}

func (u *Renderer) layoutShelf(gtx layout.Context) {
	u.LayoutShelfBase(gtx)
	u.layoutRuleOverlay(gtx)
}

func (u *Renderer) layoutShelfBody(gtx layout.Context) layout.Dimensions {
	if len(u.Settings.LibraryRoots) == 0 {
		return u.layoutWelcome(gtx)
	}
	viewportH := gtx.Constraints.Max.Y
	return u.ShelfList.Layout(gtx, len(u.LayoutDoc.Sections), func(gtx layout.Context, index int) layout.Dimensions {
		section := u.LayoutDoc.Sections[index]
		switch section.Kind {
		case "grid":
			if s := u.sectionByID(section.ID); s != nil {
				return u.layoutSection(gtx, s, viewportH)
			}
			return u.layoutSectionError(gtx, section)
		case "card":
			return u.layoutPluginCardSection(gtx, section)
		default:
			return u.layoutSectionError(gtx, section)
		}
	})
}

func (u *Renderer) layoutWelcome(gtx layout.Context) layout.Dimensions {
	if u.WelcomeAdd.Clicked(gtx) {
		u.ChooseFolder("library")
	}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Gap: 10}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Dp(unit.Dp(58))
				return layout.Stack{Alignment: layout.Center}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min = image.Pt(size, size)
						gtx.Constraints.Max = gtx.Constraints.Min
						paint.FillShape(gtx.Ops, colAccentBG, clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Min}, NE: 12, NW: 12, SE: 12, SW: 12}.Op(gtx.Ops))
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return u.LabelColor(gtx, "CG", unit.Sp(24), colAccent, 1, font.Bold)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.LabelColor(gtx, "Welcome to ComicGo", unit.Sp(25), colText, 1, font.Bold)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(550))
				return u.Label(gtx, "Choose a folder of comic archives. Covers load lazily and metadata is read from each archive comment.", unit.Sp(14), colDim, 3)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.Button(gtx, &u.WelcomeAdd, "Choose comics folder", ButtonStyle{Primary: true})
			}),
		)
	})
}

func (u *Renderer) sectionByID(id string) *SectionState {
	for _, s := range u.Sections {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (u *Renderer) layoutSection(gtx layout.Context, s *SectionState, viewportH int) layout.Dimensions {
	if len(s.TagClicks) != len(u.Tags) {
		s.TagClicks = make([]widget.Clickable, len(u.Tags))
	}
	for i := range s.TagClicks {
		if s.TagClicks[i].Clicked(gtx) {
			u.removeTag(u.Tags[i])
			return layout.Dimensions{}
		}
	}
	if s.ClearTags.Clicked(gtx) {
		u.setTags(nil)
		return layout.Dimensions{}
	}
	if s.FilterClick.Clicked(gtx) {
		s.MenuCenter = u.pressedControlCenter(&s.FilterClick, s.FilterWidth)
		u.toggleRuleMenu(s, "filter", u.Press.Position)
	}
	if s.SortClick.Clicked(gtx) {
		s.MenuCenter = u.pressedControlCenter(&s.SortClick, s.SortWidth)
		u.toggleRuleMenu(s, "sort", u.Press.Position)
	}
	u.handleRuleMenu(gtx, s)

	section := layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(2), Left: unit.Dp(18), Right: unit.Dp(18)}
	return section.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: 8}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := u.layoutSectionHeader(gtx, s)
				u.syncSectionSearch(s)
				return dims
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(u.Tags) == 0 {
					return layout.Dimensions{}
				}
				return u.layoutTagBar(gtx, s)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if u.Scanning && s.Total == 0 {
					return u.Label(gtx, "Scanning your library...", unit.Sp(14), colDim, 1)
				}
				h := max(gtx.Dp(unit.Dp(300)), viewportH-gtx.Dp(unit.Dp(70)))
				return u.layoutGrid(gtx, s, h)
			}),
		)
	})
}

func (u *Renderer) layoutSectionHeader(gtx layout.Context, s *SectionState) layout.Dimensions {
	filterLabel := "Filter: All"
	if id := s.FilterRuleID; id != "" {
		for _, r := range u.FilterRules {
			if r.ID == id {
				filterLabel = "Filter: " + r.Label
				break
			}
		}
	}
	sortLabel := "Sort: Default"
	if id := s.SortRuleID; id != "" {
		for _, r := range u.SortRules {
			if r.ID == id {
				sortLabel = "Sort: " + r.Label
				break
			}
		}
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: 12}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline, Gap: 8}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.LabelColor(gtx, s.Title, unit.Sp(16), colText, 1, font.Bold)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							width := gtx.Dp(unit.Dp(CountWidth))
							gtx.Constraints.Min.X = width
							gtx.Constraints.Max.X = width
							value := strconv.Itoa(s.Total)
							color := colDim
							if s.SearchPending {
								value = "..."
								color = colAccent
							}
							label := material.Label(u.Theme, unit.Sp(12), value)
							label.Alignment = text.End
							label.Color = color
							label.MaxLines = 1
							dims := label.Layout(gtx)
							dims.Size.X = width
							return dims
						}),
					)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return u.Editor(gtx, &s.Search, "Search this section...", 35)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := u.Button(gtx, &s.FilterClick, filterLabel, ButtonStyle{Primary: s.MenuKind == "filter"})
					s.FilterWidth = dims.Size.X
					return dims
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := u.Button(gtx, &s.SortClick, sortLabel, ButtonStyle{Primary: s.MenuKind == "sort"})
					s.SortWidth = dims.Size.X
					return dims
				}),
			)
		}),
	}
	return layout.Flex{Axis: layout.Vertical, Gap: 5}.Layout(gtx, children...)
}

func (u *Renderer) layoutTagBar(gtx layout.Context, s *SectionState) layout.Dimensions {
	return outlay.FlowWrap{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, len(u.Tags)+2, func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Right: unit.Dp(5), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			switch {
			case i == 0:
				return u.Label(gtx, "TAGS", unit.Sp(10), colDim, 1)
			case i <= len(u.Tags):
				tag := u.Tags[i-1]
				return u.layoutSearchTag(gtx, &s.TagClicks[i-1], tag)
			default:
				return u.Button(gtx, &s.ClearTags, "Clear", ButtonStyle{Plain: true, Compact: true})
			}
		})
	})
}
