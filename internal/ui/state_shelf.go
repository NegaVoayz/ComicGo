package ui

import (
	"image"
	"strings"

	Model "comicgo/internal/model"
	model "comicgo/internal/model"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
)

const (
	GridMinCard = 150
	GridGap     = 14
	GridPad     = 14
	RowsPerPage = 6
	FetchRows   = 3
	CountWidth  = 34
	// Gio's label line boxes and compact tag buttons need more room than the
	// browser caption, which previously clipped the tag row under the next cover.
	CaptionH = 76
)

type SectionState struct {
	Index int
	Model.LayoutSection
	Total         int
	Cards         map[int]*CardState
	Pending       map[int]bool
	List          layout.List
	Search        widget.Editor
	LastSearch    string
	SearchPending bool
	CardRevisions map[int]int
	FilterClick   widget.Clickable
	FilterWidth   int
	SortClick     widget.Clickable
	SortWidth     int
	MenuKind      string
	MenuAnchor    image.Point
	MenuCenter    int
	MenuClose     widget.Clickable
	MenuClicks    []widget.Clickable
	TagClicks     []widget.Clickable
	ClearTags     widget.Clickable
	LastFirst     int
}

type CardState struct {
	Card      Model.ComicCard
	Open      widget.Clickable
	Favorite  widget.Clickable
	TagClicks []widget.Clickable
}

func NewCardState(card Model.ComicCard) *CardState {
	return &CardState{
		Card:      card,
		TagClicks: make([]widget.Clickable, len(card.Tags)),
	}
}

func (s *SectionState) EnsureVisible(u *UI, rows, cols int) {
	if rows <= 0 || cols <= 0 {
		return
	}
	first := maxInt(0, s.List.Position.First-FetchRows)
	count := maxInt(1, s.List.Position.Count)
	last := minInt(rows-1, s.List.Position.First+count+FetchRows)
	start := first * cols
	end := minInt(s.Total-1, (last+1)*cols-1)
	pageSize := RowsPerPage * cols
	for p := (start / pageSize) * pageSize; p <= end; p += pageSize {
		if s.Pending[p] {
			continue
		}
		limit := minInt(pageSize, s.Total-p)
		if !s.pageNeedsLoad(p, limit, u.Refresh) {
			continue
		}
		s.Pending[p] = true
		revision := u.Refresh
		index := s.Index
		q := Model.PageQuery{
			Offset:       p,
			Limit:        limit,
			FilterRuleID: s.FilterRuleID,
			SortRuleID:   s.SortRuleID,
			Search:       screenHooks.Query(u),
		}
		go func() {
			cards, err := u.Backend.GetComics(q)
			u.Post(SectionCardsEvent{Index: index, Revision: revision, Start: p, Limit: limit, Cards: cards, Err: err})
		}()
	}
}

func (s *SectionState) pageNeedsLoad(page, limit, revision int) bool {
	for i := page; i < page+limit; i++ {
		if s.Cards[i] == nil || s.CardRevisions[i] != revision {
			return true
		}
	}
	return false
}

func (s *SectionState) MenuAnchorValid(gtx layout.Context) bool {
	return s.MenuAnchor.X > 0 && s.MenuAnchor.Y > gtx.Dp(unit.Dp(52))
}

func (u *UI) closeRuleMenu() bool {
	for _, s := range u.Sections {
		if s.MenuKind != "" {
			s.MenuKind = ""
			return true
		}
	}
	return false
}

func LandscapeLeftCrop(width, height int) bool { return height < width }

func GridGeometry(width, minCard, gap, pad int) (cols, cardWidth, coverHeight, rowHeight int) {
	if width <= 0 {
		return 1, minCard, minCard * 4 / 3, minCard*4/3 + CaptionH + gap
	}
	cols = maxInt(2, (width-pad+gap)/(minCard+gap))
	cardWidth = maxInt(80, (width-pad-gap*(cols-1))/cols)
	coverHeight = cardWidth * 4 / 3
	rowHeight = coverHeight + CaptionH + gap
	return
}

func GridViewportHeight(requested, width, total, minCard, gap, pad int) int {
	if total <= 0 {
		return requested
	}
	cols, _, _, rowH := GridGeometry(width, minCard, gap, pad)
	rows := (total + cols - 1) / cols
	contentH := rows*rowH - gap + pad
	return minInt(requested, maxInt(rowH, contentH))
}

func VisibleRows(first, count, rows, pad int) (int, int) {
	if rows <= 0 {
		return 0, -1
	}
	start := maxInt(0, first-pad)
	end := minInt(rows-1, first+maxInt(count, 1)+pad)
	return start, end
}

func TitleInitials(title string) string {
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

type DetailState struct {
	Id           int64
	Title        string
	Loading      bool
	Loaded       bool
	Err          string
	SaveErr      string
	Detail       Model.ComicDetail
	Hero         image.Image
	HeroLoading  bool
	EditOpen     bool
	NameEditor   widget.Editor
	AuthorEditor widget.Editor
	TagsEditor   widget.Editor
	SourceEditor widget.Editor
	AuthorMenu   widget.Clickable
	AuthorClose  widget.Clickable
	AuthorClicks []widget.Clickable
	AuthorList   layout.List
	AuthorOpen   bool
	AuthorCenter int
	AuthorAnchor image.Point
	AuthorValues []string
	Back         widget.Clickable
	Edit         widget.Clickable
	Copy         widget.Clickable
	Start        widget.Clickable
	Save         widget.Clickable
	Cancel       widget.Clickable
	TagClicks    []widget.Clickable
}

type HeroLoadedEvent struct {
	Id  int64
	Img image.Image
	Err error
}

type ScanStartedEvent struct{}
type ScanProgressEvent struct{ P model.ScanProgress }
type ScanDoneEvent struct{ err error }
type ScanErrorEvent struct {
	Path string
	Err  error
}
type ImportProgressEvent struct{ P model.ImportProgress }
type LayoutChangedEvent struct{}
type SettingsChangedEvent struct{}
type FavoriteChangedEvent struct {
	Id  int64
	Fav bool
	Err error
}
type ProgressChangedEvent struct {
	Id   int64
	Page int
	Err  error
}
type ToastExpiredEvent struct{ Id int }

type SectionCountEvent struct {
	Index    int
	Revision int
	Count    int
}

type IndexedCountEvent struct{ Count int }

type SectionCardsEvent struct {
	Index    int
	Revision int
	Start    int
	Limit    int
	Cards    []model.ComicCard
	Err      error
}

type CoverLoadedEvent struct {
	Id  int64
	Img image.Image
	Err error
}

type DetailLoadedEvent struct {
	Id     int64
	Detail model.ComicDetail
	Err    error
}

type DetailSavedEvent struct {
	Id     int64
	Detail model.ComicDetail
	Err    error
}

type AuthorsLoadedEvent struct {
	Id      int64
	Authors []string
	Err     error
}

type PageLoadedEvent struct {
	Id   int64
	Page int
	Slot int
	Img  image.Image
	Err  error
}

type ImportFinishedEvent struct {
	Out []model.ImportOutcome
	Err error
}

type DialogPathEvent struct {
	Kind string
	Path string
	Err  error
}
