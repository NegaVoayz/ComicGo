package screens

import (
	"comicgo/internal/model"
	"comicgo/internal/plugins"
	. "comicgo/internal/ui"
	"fmt"
	"image"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

func (u *Renderer) layoutFavoriteButton(gtx layout.Context, click *widget.Clickable, favorite bool) layout.Dimensions {
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

func (u *Renderer) toggleFavorite(card *CardState) {
	id := card.Card.ID
	go func() {
		fav, err := u.Backend.ToggleFavorite(id)
		if err != nil {
			u.Post(FavoriteChangedEvent{Id: id, Err: err})
			return
		}
		u.Post(FavoriteChangedEvent{Id: id, Fav: fav})
	}()
}

func (u *Renderer) addTag(tag string) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return
	}
	for _, existing := range u.Tags {
		if strings.EqualFold(existing, tag) {
			return
		}
	}
	u.setTags(append(append([]string(nil), u.Tags...), tag))
}

func (u *Renderer) addAuthor(author string) {
	author = strings.TrimSpace(author)
	if author == "" {
		return
	}
	for _, existing := range u.Authors {
		if strings.EqualFold(existing, author) {
			return
		}
	}
	u.setFilters(u.Tags, append(append([]string(nil), u.Authors...), author))
}

func (u *Renderer) removeTag(tag string) {
	out := make([]string, 0, len(u.Tags))
	for _, t := range u.Tags {
		if t != tag {
			out = append(out, t)
		}
	}
	u.setTags(out)
}

func (u *Renderer) removeAuthor(author string) {
	out := make([]string, 0, len(u.Authors))
	for _, a := range u.Authors {
		if a != author {
			out = append(out, a)
		}
	}
	u.setFilters(u.Tags, out)
}

func (u *Renderer) setTags(tags []string) {
	u.setFilters(tags, u.Authors)
}

func (u *Renderer) clearFilters() {
	u.setFilters(nil, nil)
}

func (u *Renderer) setFilters(tags, authors []string) {
	u.Tags = tags
	u.Authors = authors
	for _, s := range u.Sections {
		s.TagClicks = make([]widget.Clickable, len(tags))
		s.AuthorClicks = make([]widget.Clickable, len(authors))
	}
	u.RefreshShelfQuery()
	u.Invalidate()
}

func (u *Renderer) setSearch(text string, source *SectionState) {
	u.syncSearchEditors(text, source)
	for _, s := range u.Sections {
		s.SearchPending = true
	}
	u.RefreshShelfQuery()
	u.Invalidate()
}

func (u *Renderer) syncSectionSearch(s *SectionState) {
	text := s.Search.Text()
	if text == s.LastSearch {
		return
	}
	s.LastSearch = text
	if text != u.Search {
		u.setSearch(text, s)
	}
}

func (u *Renderer) syncSearchEditors(text string, source *SectionState) {
	u.Search = text
	for _, s := range u.Sections {
		if s != source {
			s.Search.SetText(text)
		}
		s.LastSearch = text
	}
}

func (u *Renderer) query() string {
	parts := []string{}
	if q := strings.TrimSpace(u.Search); q != "" {
		parts = append(parts, q)
	}
	for _, tag := range u.Tags {
		tag = strings.ReplaceAll(tag, `\`, `\\`)
		tag = strings.ReplaceAll(tag, `"`, `\"`)
		parts = append(parts, `tag:"`+tag+`"`)
	}
	for _, author := range u.Authors {
		author = strings.ReplaceAll(author, `\`, `\\`)
		author = strings.ReplaceAll(author, `"`, `\"`)
		parts = append(parts, `author:"`+author+`"`)
	}
	return strings.Join(parts, " ")
}

func (u *Renderer) requestSectionCount(index int) {
	if index < 0 || index >= len(u.Sections) {
		return
	}
	s := u.Sections[index]
	revision := u.Refresh
	q, filter := u.query(), s.FilterRuleID
	go func() {
		n := u.Backend.CountMatching(q, filter)
		u.Post(SectionCountEvent{Index: index, Revision: revision, Count: n})
	}()
}

func (u *Renderer) toggleRuleMenu(s *SectionState, kind string, anchor image.Point) {
	if s.MenuKind == kind {
		s.MenuKind = ""
		return
	}
	for _, other := range u.Sections {
		other.MenuKind = ""
	}
	s.MenuKind = kind
	s.MenuAnchor = anchor
	s.MenuClicks = nil
}

func (u *Renderer) ruleOptions(s *SectionState) []model.RuleInfo {
	options := []model.RuleInfo{{ID: "", Label: "All"}}
	if s.MenuKind == "sort" {
		options = []model.RuleInfo{{ID: "", Label: "Default"}}
		options = append(options, u.SortRules...)
	} else {
		options = append(options, u.FilterRules...)
	}
	return options
}

func (u *Renderer) handleRuleMenu(gtx layout.Context, s *SectionState) {
	if s.MenuKind == "" {
		return
	}
	options := u.ruleOptions(s)
	if len(s.MenuClicks) != len(options) {
		s.MenuClicks = make([]widget.Clickable, len(options))
	}
	for i := range options {
		if s.MenuClicks[i].Clicked(gtx) {
			kind, id, sectionID := s.MenuKind, options[i].ID, s.ID
			s.MenuKind = ""
			go func() {
				if err := u.Backend.UpdateSectionRule(sectionID, kind, id); err != nil {
					u.Post(ScanErrorEvent{Path: sectionID, Err: err})
				}
			}()
			return
		}
	}
}

func (u *Renderer) openRuleSection() *SectionState {
	for _, s := range u.Sections {
		if s.MenuKind != "" {
			return s
		}
	}
	return nil
}

func (u *Renderer) layoutPluginCardSection(gtx layout.Context, section model.LayoutSection) layout.Dimensions {
	data := u.Backend.CardData(section.CardID)
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
				return u.LabelColor(gtx, title, unit.Sp(16), colText, 1, font.Bold)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if render == nil {
					return u.Label(gtx, fmt.Sprintf("Unknown card %q - check the Gio card renderer registry.", section.CardID), unit.Sp(13), colDim, 2)
				}
				return render(u, gtx, data)
			}),
		)
	})
}

func layoutStatsCard(u *Renderer, gtx layout.Context, data map[string]any) layout.Dimensions {
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
			return u.Panel(gtx, unit.Dp(150), func(gtx layout.Context) layout.Dimensions {
				value := "-"
				if v, ok := data[item.key]; ok {
					value = fmt.Sprint(v)
				}
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Gap: 2}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.LabelColor(gtx, value, unit.Sp(22), colText, 1, font.Bold)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.Label(gtx, item.label, unit.Sp(12), colDim, 1)
					}),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Gap: 12}.Layout(gtx, children...)
}

// ---- cover crop and grid helpers ----
