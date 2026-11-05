package main

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"comicgo/internal/imgutil"
	"comicgo/internal/model"
	"comicgo/internal/plugins"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/outlay"
)

const (
	gridMinCard = 150
	gridGap     = 14
	gridPad     = 14
	rowsPerPage = 6
	fetchRows   = 3
	// Gio's label line boxes and compact tag buttons need more room than the
	// browser caption, which previously clipped the tag row under the next cover.
	captionH = 76
)

type sectionState struct {
	index int
	model.LayoutSection
	total       int
	cards       map[int]*cardState
	pending     map[int]bool
	list        layout.List
	search      widget.Editor
	lastSearch  string
	filterClick widget.Clickable
	sortClick   widget.Clickable
	sortWidth   int
	menuKind    string
	menuAnchor  image.Point
	menuClose   widget.Clickable
	menuClicks  []widget.Clickable
	tagClicks   []widget.Clickable
	clearTags   widget.Clickable
	lastFirst   int
}

type cardState struct {
	card      model.ComicCard
	open      widget.Clickable
	favorite  widget.Clickable
	tagClicks []widget.Clickable
}

func newCardState(card model.ComicCard) *cardState {
	return &cardState{
		card:      card,
		tagClicks: make([]widget.Clickable, len(card.Tags)),
	}
}

func (u *appUI) resetSections() {
	u.sections = u.sections[:0]
	gridIndex := 0
	for _, section := range u.layoutDoc.Sections {
		if section.Kind != "grid" {
			continue
		}
		s := &sectionState{
			index:         gridIndex,
			LayoutSection: section,
			cards:         make(map[int]*cardState),
			pending:       make(map[int]bool),
		}
		s.list.Axis = layout.Vertical
		s.search.SingleLine = true
		s.search.SetText(u.search)
		s.lastSearch = u.search
		u.sections = append(u.sections, s)
		gridIndex++
	}
}

func (u *appUI) layoutShelf(gtx layout.Context) {
	u.layoutShelfBase(gtx)
	u.layoutRuleOverlay(gtx)
}

func (u *appUI) layoutShelfBody(gtx layout.Context) layout.Dimensions {
	if len(u.settings.LibraryRoots) == 0 {
		return u.layoutWelcome(gtx)
	}
	viewportH := gtx.Constraints.Max.Y
	return u.shelfList.Layout(gtx, len(u.layoutDoc.Sections), func(gtx layout.Context, index int) layout.Dimensions {
		section := u.layoutDoc.Sections[index]
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

func (u *appUI) layoutWelcome(gtx layout.Context) layout.Dimensions {
	if u.welcomeAdd.Clicked(gtx) {
		u.chooseFolder("library")
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
						return u.labelColor(gtx, "CG", unit.Sp(24), colAccent, 1, font.Bold)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.labelColor(gtx, "Welcome to ComicGo", unit.Sp(25), colText, 1, font.Bold)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(550))
				return u.label(gtx, "Choose a folder of comic archives. Covers load lazily and metadata is read from each archive comment.", unit.Sp(14), colDim, 3)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.welcomeAdd, "Choose comics folder", buttonStyle{primary: true})
			}),
		)
	})
}

func (u *appUI) sectionByID(id string) *sectionState {
	for _, s := range u.sections {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (u *appUI) layoutSection(gtx layout.Context, s *sectionState, viewportH int) layout.Dimensions {
	if len(s.tagClicks) != len(u.tags) {
		s.tagClicks = make([]widget.Clickable, len(u.tags))
	}
	for i := range s.tagClicks {
		if s.tagClicks[i].Clicked(gtx) {
			u.removeTag(u.tags[i])
			return layout.Dimensions{}
		}
	}
	if s.clearTags.Clicked(gtx) {
		u.setTags(nil)
		return layout.Dimensions{}
	}
	if s.filterClick.Clicked(gtx) {
		u.toggleRuleMenu(s, "filter", u.press.position)
	}
	if s.sortClick.Clicked(gtx) {
		u.toggleRuleMenu(s, "sort", u.press.position)
	}
	u.handleRuleMenu(gtx, s)
	text := s.search.Text()
	if text != s.lastSearch {
		s.lastSearch = text
		if text != u.search {
			u.setSearch(text)
		}
	}

	section := layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(2), Left: unit.Dp(18), Right: unit.Dp(18)}
	return section.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: 8}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.layoutSectionHeader(gtx, s)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(u.tags) == 0 {
					return layout.Dimensions{}
				}
				return u.layoutTagBar(gtx, s)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if u.scanning && s.total == 0 {
					return u.label(gtx, "Scanning your library...", unit.Sp(14), colDim, 1)
				}
				h := maxInt(gtx.Dp(unit.Dp(300)), viewportH-gtx.Dp(unit.Dp(70)))
				return u.layoutGrid(gtx, s, h)
			}),
		)
	})
}

func (u *appUI) layoutSectionHeader(gtx layout.Context, s *sectionState) layout.Dimensions {
	filterLabel := "Filter: All"
	if id := s.FilterRuleID; id != "" {
		for _, r := range u.filterRules {
			if r.ID == id {
				filterLabel = "Filter: " + r.Label
				break
			}
		}
	}
	sortLabel := "Sort: Default"
	if id := s.SortRuleID; id != "" {
		for _, r := range u.sortRules {
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
							return u.labelColor(gtx, s.Title, unit.Sp(16), colText, 1, font.Bold)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, strconv.Itoa(s.total), unit.Sp(12), colDim, 1)
						}),
					)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return u.editor(gtx, &s.search, "Search this section...", 35)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.button(gtx, &s.filterClick, filterLabel, buttonStyle{primary: s.menuKind == "filter"})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := u.button(gtx, &s.sortClick, sortLabel, buttonStyle{primary: s.menuKind == "sort"})
					s.sortWidth = dims.Size.X
					return dims
				}),
			)
		}),
	}
	return layout.Flex{Axis: layout.Vertical, Gap: 5}.Layout(gtx, children...)
}

func (u *appUI) layoutTagBar(gtx layout.Context, s *sectionState) layout.Dimensions {
	return outlay.FlowWrap{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, len(u.tags)+2, func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Right: unit.Dp(5), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			switch {
			case i == 0:
				return u.label(gtx, "TAGS", unit.Sp(10), colDim, 1)
			case i <= len(u.tags):
				tag := u.tags[i-1]
				return u.layoutSearchTag(gtx, &s.tagClicks[i-1], tag)
			default:
				return u.button(gtx, &s.clearTags, "Clear", buttonStyle{plain: true, compact: true})
			}
		})
	})
}

func (u *appUI) layoutSearchTag(gtx layout.Context, remove *widget.Clickable, tag string) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			paint.FillShape(gtx.Ops, colAccent, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			paint.FillShape(gtx.Ops, colAccentBG, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(5), Right: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Gap: 3, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, tag, unit.Sp(12), colAccent, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.searchTagRemoveButton(gtx, remove)
					}),
				)
			})
		},
	)
}

func (u *appUI) searchTagRemoveButton(gtx layout.Context, click *widget.Clickable) layout.Dimensions {
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
			Width: float32(maxInt(1, gtx.Dp(unit.Dp(1.4)))),
		}.Op())
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

func (u *appUI) layoutGrid(gtx layout.Context, s *sectionState, height int) layout.Dimensions {
	height = gridViewportHeight(height, gtx.Constraints.Max.X, s.total, gtx.Dp(gridMinCard), gtx.Dp(gridGap), gtx.Dp(gridPad))
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

func (u *appUI) layoutGridContent(gtx layout.Context, s *sectionState) layout.Dimensions {
	paint.FillShape(gtx.Ops, colBG, clip.Rect{Max: gtx.Constraints.Max}.Op())
	cols, cardW, _, rowH := gridGeometry(gtx.Constraints.Max.X, gtx.Dp(gridMinCard), gtx.Dp(gridGap), gtx.Dp(gridPad))
	rows := 0
	if s.total > 0 {
		rows = (s.total + cols - 1) / cols
	}
	s.ensureVisible(u, rows, cols)
	dims := s.list.Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
		top := unit.Dp(0)
		if row == 0 {
			top = unit.Dp(gridPad)
		}
		bottom := unit.Dp(0)
		if row < rows-1 {
			bottom = unit.Dp(gridGap)
		}
		return layout.Inset{Top: top, Bottom: bottom, Left: unit.Dp(gridPad)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = cols*cardW + maxInt(0, cols-1)*gtx.Dp(gridGap)
			gtx.Constraints.Max.X = gtx.Constraints.Min.X
			children := make([]layout.FlexChild, 0, cols)
			for col := 0; col < cols; col++ {
				idx := row*cols + col
				if idx >= s.total {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = cardW
						gtx.Constraints.Max.X = cardW
						return layout.Dimensions{Size: image.Pt(cardW, rowH-gtx.Dp(gridGap))}
					}))
					continue
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = cardW
					gtx.Constraints.Max.X = cardW
					card := s.cards[idx]
					if card == nil {
						return u.layoutCardPlaceholder(gtx, cardW)
					}
					return u.layoutCard(gtx, card, cardW)
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Gap: gtx.Dp(gridGap)}.Layout(gtx, children...)
		})
	})
	s.ensureVisible(u, rows, cols)
	if s.lastFirst != s.list.Position.First {
		s.lastFirst = s.list.Position.First
		u.invalidate()
	}
	return dims
}

func (s *sectionState) ensureVisible(u *appUI, rows, cols int) {
	if rows <= 0 || cols <= 0 {
		return
	}
	first := maxInt(0, s.list.Position.First-fetchRows)
	count := maxInt(1, s.list.Position.Count)
	last := minInt(rows-1, s.list.Position.First+count+fetchRows)
	start := first * cols
	end := minInt(s.total-1, (last+1)*cols-1)
	pageSize := rowsPerPage * cols
	for p := (start / pageSize) * pageSize; p <= end; p += pageSize {
		if s.pending[p] {
			continue
		}
		missing := false
		limit := minInt(pageSize, s.total-p)
		for i := p; i < p+limit; i++ {
			if s.cards[i] == nil {
				missing = true
				break
			}
		}
		if !missing {
			continue
		}
		s.pending[p] = true
		revision := u.refresh
		index := s.index
		q := model.PageQuery{
			Offset:       p,
			Limit:        limit,
			FilterRuleID: s.FilterRuleID,
			SortRuleID:   s.SortRuleID,
			Search:       u.query(),
		}
		go func() {
			cards, err := u.backend.GetComics(q)
			u.post(sectionCardsEvent{index: index, revision: revision, start: p, cards: cards, err: err})
		}()
	}
}

func (u *appUI) layoutCardPlaceholder(gtx layout.Context, width int) layout.Dimensions {
	h := width * 4 / 3
	gtx.Constraints.Min = image.Pt(width, h+captionH)
	gtx.Constraints.Max = gtx.Constraints.Min
	paint.FillShape(gtx.Ops, colPanel, clip.RRect{Rect: image.Rect(0, 0, width, h), NE: 8, NW: 8, SE: 8, SW: 8}.Op(gtx.Ops))
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func (u *appUI) layoutCard(gtx layout.Context, card *cardState, width int) layout.Dimensions {
	tagClicked := false
	for i := range card.tagClicks {
		if i < len(card.card.Tags) && card.tagClicks[i].Clicked(gtx) {
			u.addTag(card.card.Tags[i])
			tagClicked = true
		}
	}
	favoriteClicked := card.favorite.Clicked(gtx)
	if card.open.Clicked(gtx) && !tagClicked && !favoriteClicked {
		u.openDetail(card.card)
	}
	if favoriteClicked {
		u.toggleFavorite(card)
	}
	coverH := width * 4 / 3
	gtx.Constraints.Min = image.Pt(width, coverH+captionH)
	gtx.Constraints.Max = gtx.Constraints.Min
	dims := card.open.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.layoutCardCover(gtx, card, width, coverH)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.layoutCardMeta(gtx, card, width, captionH)
			}),
		)
	})
	return dims
}

func (u *appUI) layoutCardCover(gtx layout.Context, card *cardState, width, height int) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	rect := image.Rect(0, 0, width, height)
	defer clip.RRect{Rect: rect, NE: 8, NW: 8, SE: 8, SW: 8}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, colPanel, clip.Rect{Max: image.Pt(width, height)}.Op())
	if img, ok := u.covers[card.card.ID]; ok {
		im := widget.Image{Src: paint.NewImageOp(img), Fit: widget.Cover, Position: layout.Center}
		if landscapeLeftCrop(img.Bounds().Dx(), img.Bounds().Dy()) {
			im.Position = layout.W
		}
		im.Layout(gtx)
	} else {
		u.requestCover(card.card.ID)
		initials := titleInitials(card.card.Title)
		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return u.labelColor(gtx, initials, unit.Sp(30), colDim, 1, font.Bold)
		})
	}
	if card.card.HasNonUTF8 {
		layout.NE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, "?", unit.Sp(12), colText, 1)
			})
		})
	}
	if card.card.PageCount > 0 && card.card.LastPage > 0 {
		progress := minInt(100, (card.card.LastPage+1)*100/card.card.PageCount)
		layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(5), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, fmt.Sprintf("%d%%", progress), unit.Sp(10), color.NRGBA{R: 0xff, G: 0xe9, B: 0xc9, A: 0xff}, 1)
			})
		})
	}
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func (u *appUI) layoutCardMeta(gtx layout.Context, card *cardState, width, height int) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	return layout.Flex{Axis: layout.Vertical, Gap: 1}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, card.card.Title, unit.Sp(13), colText, 1)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := "♡"
					if card.card.Fav {
						label = "♥"
					}
					return u.layoutFavoriteButton(gtx, &card.favorite, label == "♥")
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if card.card.Author == "" {
				return layout.Dimensions{}
			}
			return u.label(gtx, card.card.Author, unit.Sp(11), colDim, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{}
			for i, tag := range card.card.Tags {
				if i >= 3 {
					break
				}
				i, tag := i, tag
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.button(gtx, &card.tagClicks[i], tag, buttonStyle{plain: true, compact: true})
				}))
			}
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, fmt.Sprintf("%d pages", card.card.PageCount), unit.Sp(10), colDim, 1)
			}))
			return layout.Flex{Axis: layout.Horizontal, Gap: 4, Alignment: layout.Middle}.Layout(gtx, children...)
		}),
	)
}

func (u *appUI) layoutFavoriteButton(gtx layout.Context, click *widget.Clickable, favorite bool) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(22)), gtx.Dp(unit.Dp(22)))
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	col := colDim
	if favorite {
		col = colFav
	} else if click.Hovered() {
		col = colText
	}
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var path clip.Path
		path.Begin(gtx.Ops)
		w, h := float32(size.X), float32(size.Y)
		path.MoveTo(f32.Pt(w*.5, h*.86))
		path.CubeTo(f32.Pt(w*.12, h*.55), f32.Pt(w*.06, h*.22), f32.Pt(w*.28, h*.14))
		path.CubeTo(f32.Pt(w*.4, h*.09), f32.Pt(w*.48, h*.17), f32.Pt(w*.5, h*.25))
		path.CubeTo(f32.Pt(w*.52, h*.17), f32.Pt(w*.6, h*.09), f32.Pt(w*.72, h*.14))
		path.CubeTo(f32.Pt(w*.94, h*.22), f32.Pt(w*.88, h*.55), f32.Pt(w*.5, h*.86))
		path.Close()
		paint.FillShape(gtx.Ops, col, clip.Outline{Path: path.End()}.Op())
		return layout.Dimensions{Size: size}
	})
}

func (u *appUI) toggleFavorite(card *cardState) {
	id := card.card.ID
	go func() {
		fav, err := u.backend.ToggleFavorite(id)
		if err != nil {
			u.post(favoriteChangedEvent{id: id, err: err})
			return
		}
		u.post(favoriteChangedEvent{id: id, fav: fav})
	}()
}

func (u *appUI) addTag(tag string) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return
	}
	for _, existing := range u.tags {
		if strings.EqualFold(existing, tag) {
			return
		}
	}
	u.setTags(append(append([]string(nil), u.tags...), tag))
}

func (u *appUI) removeTag(tag string) {
	out := make([]string, 0, len(u.tags))
	for _, t := range u.tags {
		if t != tag {
			out = append(out, t)
		}
	}
	u.setTags(out)
}

func (u *appUI) setTags(tags []string) {
	u.tags = tags
	for _, s := range u.sections {
		s.tagClicks = make([]widget.Clickable, len(tags))
	}
	u.refreshShelf()
}

func (u *appUI) setSearch(text string) {
	u.search = text
	for _, s := range u.sections {
		s.search.SetText(text)
		s.lastSearch = text
	}
	u.refreshShelf()
}

func (u *appUI) query() string {
	parts := []string{}
	if q := strings.TrimSpace(u.search); q != "" {
		parts = append(parts, q)
	}
	for _, tag := range u.tags {
		tag = strings.ReplaceAll(tag, `\`, `\\`)
		tag = strings.ReplaceAll(tag, `"`, `\"`)
		parts = append(parts, `tag:"`+tag+`"`)
	}
	return strings.Join(parts, " ")
}

func (u *appUI) requestSectionCount(index int) {
	if index < 0 || index >= len(u.sections) {
		return
	}
	s := u.sections[index]
	revision := u.refresh
	q, filter := u.query(), s.FilterRuleID
	go func() {
		n := u.backend.CountMatching(q, filter)
		u.post(sectionCountEvent{index: index, revision: revision, count: n})
	}()
}

func (u *appUI) toggleRuleMenu(s *sectionState, kind string, anchor image.Point) {
	if s.menuKind == kind {
		s.menuKind = ""
		return
	}
	for _, other := range u.sections {
		other.menuKind = ""
	}
	s.menuKind = kind
	s.menuAnchor = anchor
	s.menuClicks = nil
}

func (u *appUI) ruleOptions(s *sectionState) []model.RuleInfo {
	options := []model.RuleInfo{{ID: "", Label: "All"}}
	if s.menuKind == "sort" {
		options = []model.RuleInfo{{ID: "", Label: "Default"}}
		options = append(options, u.sortRules...)
	} else {
		options = append(options, u.filterRules...)
	}
	return options
}

func (u *appUI) handleRuleMenu(gtx layout.Context, s *sectionState) {
	if s.menuKind == "" {
		return
	}
	options := u.ruleOptions(s)
	if len(s.menuClicks) != len(options) {
		s.menuClicks = make([]widget.Clickable, len(options))
	}
	for i := range options {
		if s.menuClicks[i].Clicked(gtx) {
			kind, id, sectionID := s.menuKind, options[i].ID, s.ID
			s.menuKind = ""
			go func() {
				if err := u.backend.UpdateSectionRule(sectionID, kind, id); err != nil {
					u.post(scanErrorEvent{path: sectionID, err: err})
				}
			}()
			return
		}
	}
}

func (u *appUI) openRuleSection() *sectionState {
	for _, s := range u.sections {
		if s.menuKind != "" {
			return s
		}
	}
	return nil
}

func (u *appUI) layoutRuleOverlay(gtx layout.Context) {
	s := u.openRuleSection()
	if s == nil {
		return
	}
	options := u.ruleOptions(s)
	if len(s.menuClicks) != len(options) {
		s.menuClicks = make([]widget.Clickable, len(options))
	}
	if s.menuClose.Clicked(gtx) {
		s.menuKind = ""
		return
	}

	menuWidth := u.ruleMenuWidth(gtx, options)
	macro := op.Record(gtx.Ops)
	menuDims := u.layoutRuleMenuPanel(gtx, s, options, menuWidth)
	menu := macro.Stop()
	right := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(18))
	if s.menuKind == "filter" {
		right = gtx.Constraints.Max.X - gtx.Dp(unit.Dp(18)) - s.sortWidth - gtx.Dp(unit.Dp(12))
	}
	top := s.menuAnchor.Y + gtx.Dp(unit.Dp(18))
	if !s.menuAnchorValid(gtx) {
		top = gtx.Dp(unit.Dp(52 + 14 + 35))
	}
	x := maxInt(gtx.Dp(unit.Dp(8)), right-menuDims.Size.X)
	y := maxInt(gtx.Dp(unit.Dp(52)), top)
	if y+menuDims.Size.Y > gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(8)) {
		y = maxInt(gtx.Dp(unit.Dp(52)), gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(8))-menuDims.Size.Y)
	}
	s.menuClose.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()
	menu.Add(gtx.Ops)
}

func (u *appUI) ruleMenuWidth(gtx layout.Context, options []model.RuleInfo) int {
	maxLabel := 0
	for _, option := range options {
		mgtx := gtx
		mgtx.Constraints.Min = image.Point{}
		mgtx.Constraints.Max.X = mgtx.Dp(unit.Dp(320))
		macro := op.Record(gtx.Ops)
		dims := u.label(mgtx, option.Label, unit.Sp(12), colText, 1)
		macro.Stop()
		maxLabel = maxInt(maxLabel, dims.Size.X)
	}
	return maxLabel + gtx.Dp(unit.Dp(42))
}

func (u *appUI) layoutRuleMenuPanel(gtx layout.Context, s *sectionState, options []model.RuleInfo, width int) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	widthDp := unit.Dp(float32(width) / gtx.Metric.PxPerDp)
	return u.panel(gtx, widthDp, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(options))
		for i, option := range options {
			i := i
			selected := s.FilterRuleID == option.ID
			if s.menuKind == "sort" {
				selected = s.SortRuleID == option.ID
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				style := buttonStyle{plain: true}
				if selected {
					style = buttonStyle{primary: true}
				}
				return u.button(gtx, &s.menuClicks[i], option.Label, style)
			}))
		}
		return layout.Flex{Axis: layout.Vertical, Gap: 2}.Layout(gtx, children...)
	})
}

func (s *sectionState) menuAnchorValid(gtx layout.Context) bool {
	return s.menuAnchor.X > 0 && s.menuAnchor.Y > gtx.Dp(unit.Dp(52))
}

func (u *appUI) closeRuleMenu() bool {
	for _, s := range u.sections {
		if s.menuKind != "" {
			s.menuKind = ""
			return true
		}
	}
	return false
}

func (u *appUI) layoutSectionError(gtx layout.Context, section model.LayoutSection) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(14), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.label(gtx, fmt.Sprintf("Unsupported layout section %q (%s)", section.ID, section.Kind), unit.Sp(13), colDim, 2)
	})
}

type pluginCardRenderer func(*appUI, layout.Context, map[string]any) layout.Dimensions

var cardRenderers = map[string]pluginCardRenderer{
	"stats": layoutStatsCard,
}

func (u *appUI) layoutPluginCardSection(gtx layout.Context, section model.LayoutSection) layout.Dimensions {
	data := u.backend.CardData(section.CardID)
	specType := ""
	if card := plugins.CardByID(section.CardID); card != nil {
		specType = card.Render().Type
	}
	render := cardRenderers[specType]
	return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(8), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: 8}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				title := section.Title
				if title == "" {
					title = section.CardID
				}
				return u.labelColor(gtx, title, unit.Sp(16), colText, 1, font.Bold)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if render == nil {
					return u.label(gtx, fmt.Sprintf("Unknown card %q - check the Gio card renderer registry.", section.CardID), unit.Sp(13), colDim, 2)
				}
				return render(u, gtx, data)
			}),
		)
	})
}

func layoutStatsCard(u *appUI, gtx layout.Context, data map[string]any) layout.Dimensions {
	tiles := []struct {
		key   string
		label string
	}{
		{"total", "Total"},
		{"favorites", "Favorites"},
		{"unread", "Unread"},
		{"inProgress", "In progress"},
	}
	children := make([]layout.FlexChild, 0, len(tiles))
	for _, item := range tiles {
		item := item
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.panel(gtx, unit.Dp(150), func(gtx layout.Context) layout.Dimensions {
				value := "-"
				if v, ok := data[item.key]; ok {
					value = fmt.Sprint(v)
				}
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Gap: 2}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.labelColor(gtx, value, unit.Sp(22), colText, 1, font.Bold)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, item.label, unit.Sp(12), colDim, 1)
					}),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Gap: 12}.Layout(gtx, children...)
}

// ---- cover crop and grid helpers ----

func landscapeLeftCrop(width, height int) bool { return height < width }

func gridGeometry(width, minCard, gap, pad int) (cols, cardWidth, coverHeight, rowHeight int) {
	if width <= 0 {
		return 1, minCard, minCard * 4 / 3, minCard*4/3 + captionH + gap
	}
	cols = maxInt(2, (width-pad+gap)/(minCard+gap))
	cardWidth = maxInt(80, (width-pad-gap*(cols-1))/cols)
	coverHeight = cardWidth * 4 / 3
	rowHeight = coverHeight + captionH + gap
	return
}

func gridViewportHeight(requested, width, total, minCard, gap, pad int) int {
	if total <= 0 {
		return requested
	}
	cols, _, _, rowH := gridGeometry(width, minCard, gap, pad)
	rows := (total + cols - 1) / cols
	contentH := rows*rowH - gap + pad
	return minInt(requested, maxInt(rowH, contentH))
}

func visibleRows(first, count, rows, pad int) (int, int) {
	if rows <= 0 {
		return 0, -1
	}
	start := maxInt(0, first-pad)
	end := minInt(rows-1, first+maxInt(count, 1)+pad)
	return start, end
}

func titleInitials(title string) string {
	words := strings.Fields(title)
	var b strings.Builder
	for i := 0; i < len(words) && i < 2; i++ {
		runes := []rune(words[i])
		if len(runes) > 0 {
			b.WriteRune(runes[0])
		}
	}
	if b.Len() == 0 {
		return "?"
	}
	return strings.ToUpper(b.String())
}

// ---- details ----

type detailState struct {
	id           int64
	title        string
	loading      bool
	loaded       bool
	err          string
	saveErr      string
	detail       model.ComicDetail
	hero         image.Image
	heroLoading  bool
	editOpen     bool
	nameEditor   widget.Editor
	authorEditor widget.Editor
	tagsEditor   widget.Editor
	sourceEditor widget.Editor
	back         widget.Clickable
	edit         widget.Clickable
	copy         widget.Clickable
	start        widget.Clickable
	save         widget.Clickable
	cancel       widget.Clickable
	tagClicks    []widget.Clickable
}

type heroLoadedEvent struct {
	id  int64
	img image.Image
	err error
}

func (u *appUI) openDetail(card model.ComicCard) {
	u.detail = detailState{
		id:           card.ID,
		title:        card.Title,
		loading:      true,
		nameEditor:   widget.Editor{SingleLine: true},
		authorEditor: widget.Editor{SingleLine: true},
		tagsEditor:   widget.Editor{SingleLine: true},
		sourceEditor: widget.Editor{SingleLine: true},
	}
	u.screen = screenDetail
	go func() {
		detail, err := u.backend.GetComicDetail(card.ID)
		u.post(detailLoadedEvent{id: card.ID, detail: detail, err: err})
	}()
}

func (u *appUI) requestHero(id int64) {
	if u.detail.hero != nil || u.detail.heroLoading {
		return
	}
	u.detail.heroLoading = true
	go func() {
		b, _, err := u.backend.pageBytes(id, 0)
		if err != nil {
			u.post(heroLoadedEvent{id: id, err: err})
			return
		}
		img, _, err := imgutil.Decode(b)
		u.post(heroLoadedEvent{id: id, img: img, err: err})
	}()
}

func (u *appUI) layoutDetail(gtx layout.Context) layout.Dimensions {
	if u.detail.back.Clicked(gtx) {
		u.screen = screenShelf
		u.detail = detailState{}
	}
	if u.detail.edit.Clicked(gtx) {
		u.openMetadataEditor()
	}
	if u.detail.copy.Clicked(gtx) {
		appendClipboard(gtx, u.detail.detail.Meta.KV["Source"])
		u.toast("Source copied", toastOK)
	}
	if u.detail.start.Clicked(gtx) {
		u.startReader()
	}
	tags := splitTags(u.detail.detail.Meta.KV["Genre"])
	if len(u.detail.tagClicks) != len(tags) {
		u.detail.tagClicks = make([]widget.Clickable, len(tags))
	}
	for i, tag := range tags {
		if u.detail.tagClicks[i].Clicked(gtx) {
			u.addTag(tag)
			u.screen = screenShelf
			u.detail = detailState{}
			return layout.Dimensions{}
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.layoutDetailTop(gtx) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if u.detail.loading {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, "Loading comic details...", unit.Sp(15), colDim, 1)
				})
			}
			if u.detail.err != "" {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, u.detail.err, unit.Sp(14), colFav, 3)
				})
			}
			return u.layoutDetailBody(gtx, tags)
		}),
	)
}

func (u *appUI) layoutDetailTop(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(52))
	paint.Fill(gtx.Ops, colBGSoft)
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Gap: 8, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.detail.back, "< Library", buttonStyle{})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.detail.edit, "Edit metadata", buttonStyle{})
			}),
		)
	})
}

func (u *appUI) layoutDetailBody(gtx layout.Context, tags []string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(28), Bottom: unit.Dp(28), Left: unit.Dp(34), Right: unit.Dp(34)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gap := minInt(gtx.Dp(unit.Dp(56)), maxInt(gtx.Dp(unit.Dp(24)), gtx.Constraints.Max.X/20))
					return layout.Flex{Axis: layout.Horizontal, Gap: gap, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							width := minInt(gtx.Dp(unit.Dp(340)), gtx.Constraints.Max.X*34/100)
							height := width * 4 / 3
							return u.layoutDetailHero(gtx, width, height)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Max.X = minInt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(680)))
							return u.layoutDetailMeta(gtx, tags)
						}),
					)
				})
			}),
		)
	})
}

func (u *appUI) layoutDetailHero(gtx layout.Context, width, height int) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	defer clip.RRect{Rect: image.Rect(0, 0, width, height), NE: 8, NW: 8, SE: 8, SW: 8}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, colPanel, clip.Rect{Max: image.Pt(width, height)}.Op())
	if u.detail.hero != nil {
		im := widget.Image{Src: paint.NewImageOp(u.detail.hero), Fit: widget.Cover, Position: layout.Center}
		if landscapeLeftCrop(u.detail.hero.Bounds().Dx(), u.detail.hero.Bounds().Dy()) {
			im.Position = layout.W
		}
		return im.Layout(gtx)
	}
	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.label(gtx, "Loading cover...", unit.Sp(14), colDim, 1)
	})
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func (u *appUI) layoutDetailMeta(gtx layout.Context, tags []string) layout.Dimensions {
	m := u.detail.detail.Meta
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.labelColor(gtx, u.detail.title, unit.Sp(24), colText, 2, font.Bold)
		}),
	}
	if author := strings.TrimSpace(m.KV["Author"]); author != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, author, unit.Sp(14), colAccent, 2)
		}))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		progress := ""
		if m.PageCount > 0 && m.LastReadPage > 0 {
			progress = fmt.Sprintf(" - %d%% read", (m.LastReadPage+1)*100/m.PageCount)
		}
		return u.label(gtx, fmt.Sprintf("%d pages%s", m.PageCount, progress), unit.Sp(13), colDim, 1)
	}))
	if len(tags) > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: 8}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, "TAGS", unit.Sp(10), colDim, 1)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return outlay.FlowWrap{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, len(tags), func(gtx layout.Context, i int) layout.Dimensions {
						return layout.Inset{Right: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return u.button(gtx, &u.detail.tagClicks[i], tags[i], buttonStyle{})
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
						return u.label(gtx, "SOURCE", unit.Sp(10), colDim, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Gap: 10, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return u.sourceField(gtx, source)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.button(gtx, &u.detail.copy, "Copy", buttonStyle{})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, "Magnet link from the original download; paste it into a torrent client to re-fetch the file.", unit.Sp(12), colDim, 2)
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
		return u.button(gtx, &u.detail.start, label, buttonStyle{solid: true, disabled: m.PageCount <= 0})
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: 10}.Layout(gtx, children...)
}

func (u *appUI) sourceField(gtx layout.Context, source string) layout.Dimensions {
	height := gtx.Dp(unit.Dp(34))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Max.X, height)
			paint.FillShape(gtx.Ops, colLine, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			paint.FillShape(gtx.Ops, colBG, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(9), Right: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, source, unit.Sp(12), colDim, 1)
			})
		}),
	)
}

func (u *appUI) openMetadataEditor() {
	m := u.detail.detail.Meta
	u.detail.nameEditor.SetText(m.Title())
	u.detail.authorEditor.SetText(m.KV["Author"])
	u.detail.tagsEditor.SetText(m.KV["Genre"])
	u.detail.sourceEditor.SetText(m.KV["Source"])
	u.detail.saveErr = ""
	u.detail.editOpen = true
}

func splitTags(s string) []string {
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

func (u *appUI) layoutMetadataEditor(gtx layout.Context) {
	if u.detail.cancel.Clicked(gtx) {
		u.detail.editOpen = false
	}
	if u.detail.save.Clicked(gtx) && !u.dialogBusy {
		edit := model.MetaEdit{
			Title:  u.detail.nameEditor.Text(),
			Author: u.detail.authorEditor.Text(),
			Genre:  u.detail.tagsEditor.Text(),
			Source: u.detail.sourceEditor.Text(),
		}
		id := u.detail.id
		u.dialogBusy = true
		go func() {
			detail, err := u.backend.UpdateComicMetadata(id, edit)
			u.post(detailSavedEvent{id: id, detail: detail, err: err})
		}()
	}
	layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.Fill(gtx.Ops, colBlack50)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.modal(gtx, unit.Dp(560), func(gtx layout.Context) layout.Dimensions {
					fields := []struct {
						label  string
						editor *widget.Editor
					}{
						{"Title", &u.detail.nameEditor},
						{"Author", &u.detail.authorEditor},
						{"Tags (comma-separated)", &u.detail.tagsEditor},
						{"Source", &u.detail.sourceEditor},
					}
					children := []layout.FlexChild{
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.labelColor(gtx, "Edit metadata", unit.Sp(19), colText, 1, font.Bold)
						}),
					}
					for _, field := range fields {
						field := field
						children = append(children,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, field.label, unit.Sp(11), colDim, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.editor(gtx, field.editor, field.label, 36)
							}),
						)
					}
					if u.detail.saveErr != "" {
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, u.detail.saveErr, unit.Sp(12), colFav, 2)
						}))
					}
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Gap: 8}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.button(gtx, &u.detail.cancel, "Cancel", buttonStyle{disabled: u.dialogBusy})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := "Save"
								if u.dialogBusy {
									label = "Saving..."
								}
								return u.button(gtx, &u.detail.save, label, buttonStyle{solid: true, disabled: u.dialogBusy})
							}),
						)
					}))
					return layout.Flex{Axis: layout.Vertical, Gap: 7}.Layout(gtx, children...)
				})
			})
		}),
	)
}
