package ui

import (
	"image"

	"comicgo/internal/model"

	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/widget"
)

const ReaderPageSlots = 2

type ReaderState struct {
	Id          int64
	Title       string
	Page        int
	PageCount   int
	Direction   string
	TwoPage     bool
	Images      [ReaderPageSlots]image.Image
	Loading     [ReaderPageSlots]bool
	Errors      [ReaderPageSlots]string
	PageFocused bool

	Back       widget.Clickable
	Left       widget.Clickable
	Right      widget.Clickable
	GoButton   widget.Clickable
	OnePage    widget.Clickable
	TwoPages   widget.Clickable
	Ltr        widget.Clickable
	Rtl        widget.Clickable
	PageEditor widget.Editor
	StageClick gesture.Click
}

func (r *ReaderState) reset() {
	*r = ReaderState{}
}

func (r *ReaderState) VisiblePageCount() int {
	if r.Page < 0 || r.Page >= r.PageCount {
		return 0
	}
	if r.TwoPage && r.Page+1 < r.PageCount {
		return 2
	}
	return 1
}

func (r *ReaderState) PageAt(slot int) int {
	if slot < 0 || slot >= r.VisiblePageCount() {
		return -1
	}
	return r.Page + slot
}

func (r *ReaderState) AcceptsPage(id int64, slot, page int) bool {
	return id == r.Id && slot >= 0 && slot < ReaderPageSlots && page == r.PageAt(slot)
}

func (r *ReaderState) ProgressPage() int {
	if r.PageCount <= 0 {
		return 0
	}
	return minInt(r.Page+r.VisiblePageCount()-1, r.PageCount-1)
}

func ReaderPageStep(twoPage bool) int {
	if twoPage {
		return 2
	}
	return 1
}

func NormalizeReaderPage(page, pageCount int, twoPage bool) int {
	if pageCount <= 0 {
		return 0
	}
	page = maxInt(0, minInt(page, pageCount-1))
	if twoPage {
		page -= page % ReaderPageStep(true)
	}
	return page
}

func ReaderLastViewStart(pageCount int, twoPage bool) int {
	return NormalizeReaderPage(pageCount-1, pageCount, twoPage)
}

func ReaderCanGoNext(page, pageCount int, twoPage bool) bool {
	return page >= 0 && page+ReaderPageStep(twoPage) < pageCount
}

func ReaderCanMove(page, pageCount int, twoPage bool, delta int) bool {
	if delta < 0 {
		return page > 0
	}
	return ReaderCanGoNext(page, pageCount, twoPage)
}

func ReaderSpatialPageDelta(side int, direction string) int {
	if side < 0 {
		if direction == model.ReadingDirectionRTL {
			return 1
		}
		return -1
	}
	if side > 0 {
		if direction == model.ReadingDirectionRTL {
			return -1
		}
		return 1
	}
	return 0
}

func ReaderSidePageDelta(x, width int, direction string) int {
	if x < width*28/100 {
		return ReaderSpatialPageDelta(-1, direction)
	}
	if x > width*72/100 {
		return ReaderSpatialPageDelta(1, direction)
	}
	return 0
}

func ReaderKeyPageDelta(name key.Name, direction string) int {
	switch name {
	case key.NameRightArrow:
		if direction == model.ReadingDirectionRTL {
			return -1
		}
		return 1
	case key.NameLeftArrow:
		if direction == model.ReadingDirectionRTL {
			return 1
		}
		return -1
	case key.NameDownArrow, key.NamePageDown, key.NameSpace:
		return 1
	case key.NameUpArrow, key.NamePageUp:
		return -1
	default:
		return 0
	}
}
