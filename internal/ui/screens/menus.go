package screens

import (
	"comicgo/internal/imgutil"
	"comicgo/internal/model"
	. "comicgo/internal/ui"
	"comicgo/internal/ui/windowing"
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

func (u *Renderer) openDetail(card model.ComicCard) {
	u.Detail = DetailState{
		Id:           card.ID,
		Title:        card.Title,
		Loading:      true,
		NameEditor:   widget.Editor{SingleLine: true},
		AuthorEditor: widget.Editor{SingleLine: true},
		TagsEditor:   widget.Editor{SingleLine: true},
		SourceEditor: widget.Editor{SingleLine: true},
	}
	u.Screen = ScreenDetail
	go func() {
		detail, err := u.Backend.GetComicDetail(card.ID)
		u.Post(DetailLoadedEvent{Id: card.ID, Detail: detail, Err: err})
	}()
}

func (u *Renderer) requestHero(id int64) {
	if u.Detail.Hero != nil || u.Detail.HeroLoading {
		return
	}
	u.Detail.HeroLoading = true
	go func() {
		b, _, err := u.Backend.PageBytes(id, 0)
		if err != nil {
			u.Post(HeroLoadedEvent{Id: id, Err: err})
			return
		}
		img, _, err := imgutil.Decode(b)
		u.Post(HeroLoadedEvent{Id: id, Img: img, Err: err})
	}()
}

func (u *Renderer) layoutDetail(gtx layout.Context) layout.Dimensions {
	if u.Detail.Back.Clicked(gtx) {
		u.Screen = ScreenShelf
		u.Detail = DetailState{}
	}
	if u.Detail.Edit.Clicked(gtx) {
		u.openMetadataEditor()
	}
	if u.Detail.Copy.Clicked(gtx) {
		AppendClipboard(gtx, u.Detail.Detail.Meta.KV["Source"])
		u.Toast("Source copied", ToastOK)
	}
	if u.Detail.Start.Clicked(gtx) {
		u.startReader()
	}
	author := strings.TrimSpace(u.Detail.Detail.Meta.KV["Author"])
	if author != "" && u.Detail.AuthorClick.Clicked(gtx) {
		u.addAuthor(author)
		u.Screen = ScreenShelf
		u.Detail = DetailState{}
		return layout.Dimensions{}
	}
	tags := SplitTags(u.Detail.Detail.Meta.KV["Genre"])
	if len(u.Detail.TagClicks) != len(tags) {
		u.Detail.TagClicks = make([]widget.Clickable, len(tags))
	}
	for i, tag := range tags {
		if u.Detail.TagClicks[i].Clicked(gtx) {
			u.addTag(tag)
			u.Screen = ScreenShelf
			u.Detail = DetailState{}
			return layout.Dimensions{}
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.layoutDetailTop(gtx) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if u.Detail.Loading {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.Label(gtx, "Loading comic details...", unit.Sp(15), colDim, 1)
				})
			}
			if u.Detail.Err != "" {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.Label(gtx, u.Detail.Err, unit.Sp(14), colFav, 3)
				})
			}
			return u.layoutDetailBody(gtx, tags)
		}),
	)
}

func (u *Renderer) layoutDetailTop(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(52))
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(52))
			gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
			return u.Chrome.LayoutMoveArea(gtx, func(gtx layout.Context) layout.Dimensions {
				paint.Fill(gtx.Ops, colBGSoft)
				paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(52))
			gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
			inset := windowing.CaptionContentInsets(52)
			inset.Top = 0
			inset.Bottom = 0
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Gap: 8, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.Button(gtx, &u.Detail.Back, "< Library", ButtonStyle{})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return u.Chrome.LayoutMoveArea(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: gtx.Constraints.Max}
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.Button(gtx, &u.Detail.Edit, "Edit metadata", ButtonStyle{})
					}),
				)
			})
		}),
	)
}

func (u *Renderer) layoutDetailBody(gtx layout.Context, tags []string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(28), Bottom: unit.Dp(28), Left: unit.Dp(34), Right: unit.Dp(34)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gap := min(gtx.Dp(unit.Dp(56)), max(gtx.Dp(unit.Dp(24)), gtx.Constraints.Max.X/20))
					return layout.Flex{Axis: layout.Horizontal, Gap: gap, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							width := min(gtx.Dp(unit.Dp(340)), gtx.Constraints.Max.X*34/100)
							height := width * 4 / 3
							return u.layoutDetailHero(gtx, width, height)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(680)))
							return u.layoutDetailMeta(gtx, tags)
						}),
					)
				})
			}),
		)
	})
}

func (u *Renderer) layoutDetailHero(gtx layout.Context, width, height int) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	defer clip.RRect{Rect: image.Rect(0, 0, width, height), NE: 8, NW: 8, SE: 8, SW: 8}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, colPanel, clip.Rect{Max: image.Pt(width, height)}.Op())
	if u.Detail.Hero != nil {
		im := widget.Image{Src: paint.NewImageOp(u.Detail.Hero), Fit: widget.Cover, Position: layout.Center}
		if LandscapeLeftCrop(u.Detail.Hero.Bounds().Dx(), u.Detail.Hero.Bounds().Dy()) {
			im.Position = layout.W
		}
		return im.Layout(gtx)
	}
	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.Label(gtx, "Loading cover...", unit.Sp(14), colDim, 1)
	})
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func (u *Renderer) layoutRuleOverlay(gtx layout.Context) {
	s := u.openRuleSection()
	if s == nil {
		return
	}
	options := u.ruleOptions(s)
	if len(s.MenuClicks) != len(options) {
		s.MenuClicks = make([]widget.Clickable, len(options))
	}
	if s.MenuClose.Clicked(gtx) {
		s.MenuKind = ""
		return
	}

	menuWidth := u.ruleMenuWidth(gtx, options)
	macro := op.Record(gtx.Ops)
	menuDims := u.layoutRuleMenuPanel(gtx, s, options, menuWidth)
	menu := macro.Stop()
	right := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(18))
	triggerWidth := s.SortWidth
	if s.MenuKind == "filter" {
		right = gtx.Constraints.Max.X - gtx.Dp(unit.Dp(18)) - s.SortWidth - gtx.Dp(unit.Dp(12))
		triggerWidth = s.FilterWidth
	}
	top := s.MenuAnchor.Y + gtx.Dp(unit.Dp(18))
	if !s.MenuAnchorValid(gtx) {
		top = gtx.Dp(unit.Dp(52 + 14 + 35))
	}
	x := s.MenuCenter - menuDims.Size.X/2
	if s.MenuCenter == 0 {
		x = right - triggerWidth/2 - menuDims.Size.X/2
	}
	x = min(x, gtx.Constraints.Max.X-gtx.Dp(unit.Dp(8))-menuDims.Size.X)
	x = max(gtx.Dp(unit.Dp(8)), x)
	y := max(gtx.Dp(unit.Dp(52)), top)
	if y+menuDims.Size.Y > gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(8)) {
		y = max(gtx.Dp(unit.Dp(52)), gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(8))-menuDims.Size.Y)
	}
	s.MenuClose.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()
	menu.Add(gtx.Ops)
}

func (u *Renderer) pressedControlCenter(click *widget.Clickable, width int) int {
	history := click.History()
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Cancelled {
			continue
		}
		return u.Press.Position.X - history[i].Position.X + width/2
	}
	return 0
}

func (u *Renderer) ruleMenuWidth(gtx layout.Context, options []model.RuleInfo) int {
	maxLabel := 0
	for _, option := range options {
		mgtx := gtx
		mgtx.Constraints.Min = image.Point{}
		mgtx.Constraints.Max.X = mgtx.Dp(unit.Dp(320))
		macro := op.Record(gtx.Ops)
		dims := u.Label(mgtx, option.Label, unit.Sp(12), colText, 1)
		macro.Stop()
		maxLabel = max(maxLabel, dims.Size.X)
	}
	return maxLabel + gtx.Dp(unit.Dp(42))
}
