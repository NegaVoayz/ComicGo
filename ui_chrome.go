package main

import (
	"image"
	"image/color"
	"runtime"

	"gioui.org/f32"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

const (
	captionButtonWidthDp  = 56
	captionButtonHeightDp = 40
	captionButtonTopDp    = 2
	macCaptionAreaDp      = 74
)

type chromePart uint8

const (
	chromeNone chromePart = iota
	chromeMinimize
	chromeMaximize
	chromeClose
)

type chromeHitRegions struct {
	Minimize image.Rectangle
	Maximize image.Rectangle
	Close    image.Rectangle
	Caption  image.Rectangle
}

type windowChromeState struct {
	decorations widget.Decorations
	maximized   bool
	focused     bool
	hover       chromePart
	regions     chromeHitRegions
}

type windowChromeBackend interface {
	handlePlatformEvent(ev any) bool
	setCaptionRegions(chromeHitRegions)
	close()
}

type chromeActionEvent struct {
	part chromePart
}

type chromeHoverEvent struct {
	part chromePart
}

func captionHitRegions(width, height, scale int) chromeHitRegions {
	if scale < 1 {
		scale = 1
	}
	caption := image.Rect(0, 0, width, height)
	if runtime.GOOS == "darwin" {
		cy := height / 2
		d := 12 * scale
		top, bottom := cy-d, cy+d
		return chromeHitRegions{
			Close:    image.Rect(8*scale, top, 32*scale, bottom),
			Minimize: image.Rect(30*scale, top, 54*scale, bottom),
			Maximize: image.Rect(52*scale, top, 76*scale, bottom),
			Caption:  caption,
		}
	}
	right := width
	button := captionButtonWidthDp * scale
	buttonHeight := captionButtonHeightDp * scale
	top := captionButtonTopDp * scale
	bottom := minInt(height, buttonHeight)
	maximize := image.Rect(right-2*button, top, right-button, bottom)
	close := image.Rect(right-button, top, right, bottom)
	minimize := image.Rect(right-3*button, top, right-2*button, bottom)
	return chromeHitRegions{Minimize: minimize, Maximize: maximize, Close: close, Caption: caption}
}

func (r chromeHitRegions) at(p image.Point) chromePart {
	switch {
	case p.In(r.Minimize):
		return chromeMinimize
	case p.In(r.Maximize):
		return chromeMaximize
	case p.In(r.Close):
		return chromeClose
	default:
		return chromeNone
	}
}

func chromeAction(part chromePart, maximized bool) system.Action {
	switch part {
	case chromeMinimize:
		return system.ActionMinimize
	case chromeMaximize:
		if maximized {
			return system.ActionUnmaximize
		}
		return system.ActionMaximize
	case chromeClose:
		return system.ActionClose
	default:
		return 0
	}
}

func captionContentInsets(height int) layout.Inset {
	if runtime.GOOS == "darwin" {
		return layout.Inset{Left: unit.Dp(macCaptionAreaDp), Right: unit.Dp(8)}
	}
	return layout.Inset{
		Left:  unit.Dp(8),
		Right: unit.Dp(3*captionButtonWidthDp + 8),
	}
}

func (u *appUI) layoutCaptionMoveArea(gtx layout.Context) layout.Dimensions {
	return u.chrome.decorations.LayoutMove(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
}

func (u *appUI) layoutCaptionControls(gtx layout.Context, height int) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, height)
	gtx.Constraints = layout.Exact(size)
	scale := gtx.Dp(unit.Dp(1))
	u.chrome.regions = captionHitRegions(size.X, height, scale)
	u.chrome.decorations.Maximized = u.chrome.maximized

	if runtime.GOOS == "darwin" {
		controls := []struct {
			part   chromePart
			action system.Action
			x      int
		}{
			{chromeClose, system.ActionClose, gtx.Dp(unit.Dp(20))},
			{chromeMinimize, system.ActionMinimize, gtx.Dp(unit.Dp(42))},
			{chromeMaximize, system.ActionMaximize, gtx.Dp(unit.Dp(64))},
		}
		for _, control := range controls {
			u.layoutMacCaptionButton(gtx, control.part, control.action, control.x, height)
		}
	} else {
		buttonWidth := captionButtonWidthDp * scale
		buttonHeight := captionButtonHeightDp * scale
		buttonTop := captionButtonTopDp * scale
		buttonHeight = minInt(height, buttonHeight)
		start := size.X - 3*buttonWidth
		u.layoutWindowsCaptionButton(gtx, chromeMinimize, system.ActionMinimize, start, buttonTop, buttonWidth, buttonHeight)
		u.layoutWindowsCaptionButton(gtx, chromeMaximize, system.ActionMaximize, start+buttonWidth, buttonTop, buttonWidth, buttonHeight)
		u.layoutWindowsCaptionButton(gtx, chromeClose, system.ActionClose, start+2*buttonWidth, buttonTop, buttonWidth, buttonHeight)
	}

	if actions := u.chrome.decorations.Update(gtx); actions != 0 {
		u.win.Perform(actions)
	}
	return layout.Dimensions{Size: size}
}

func (u *appUI) layoutWindowsCaptionButton(gtx layout.Context, part chromePart, action system.Action, x, y, buttonWidth, buttonHeight int) {
	child := gtx
	child.Constraints = layout.Exact(image.Pt(buttonWidth, buttonHeight))
	offset := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	defer offset.Pop()

	click := u.chrome.decorations.Clickable(action)
	click.Layout(child, func(gtx layout.Context) layout.Dimensions {
		hovered := click.Hovered() || u.chrome.hover == part
		bg := caseHoverBackground(part, hovered)
		if bg.A != 0 {
			paint.Fill(gtx.Ops, bg)
		}
		icon := colDim
		if hovered && u.chrome.focused {
			icon = colText
		}
		drawCaptionGlyph(gtx.Ops, part, icon, gtx.Constraints.Max, u.chrome.maximized)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
}

func caseHoverBackground(part chromePart, hovered bool) (col color.NRGBA) {
	if !hovered {
		return color.NRGBA{}
	}
	if part == chromeClose {
		return color.NRGBA{R: 0xc4, G: 0x3d, B: 0x45, A: 0xff}
	}
	return colPanel2
}

func (u *appUI) layoutMacCaptionButton(gtx layout.Context, part chromePart, action system.Action, x, height int) {
	child := gtx
	child.Constraints = layout.Exact(image.Pt(22, height))
	offset := op.Offset(image.Pt(x-gtx.Dp(unit.Dp(10)), 0)).Push(gtx.Ops)
	defer offset.Pop()

	click := u.chrome.decorations.Clickable(action)
	click.Layout(child, func(gtx layout.Context) layout.Dimensions {
		d := gtx.Dp(unit.Dp(12))
		y := (height - d) / 2
		margin := gtx.Dp(unit.Dp(10))
		r := image.Rect(margin, y, margin+d, y+d)
		fill := color.NRGBA{R: 0x69, G: 0x70, B: 0x7b, A: 0xff}
		if u.chrome.focused {
			switch part {
			case chromeClose:
				fill = color.NRGBA{R: 0xff, G: 0x5f, B: 0x57, A: 0xff}
			case chromeMinimize:
				fill = color.NRGBA{R: 0xfe, G: 0xbc, B: 0x2e, A: 0xff}
			case chromeMaximize:
				fill = color.NRGBA{R: 0x28, G: 0xc8, B: 0x40, A: 0xff}
			}
		}
		paint.FillShape(gtx.Ops, fill, clip.Ellipse(r).Op(gtx.Ops))
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
}

func drawCaptionGlyph(ops *op.Ops, part chromePart, col color.NRGBA, size image.Point, maximized bool) {
	cx := size.X / 2
	cy := size.Y / 2
	glyph := maxInt(6, size.Y/7)
	var path clip.Path
	path.Begin(ops)
	switch part {
	case chromeMinimize:
		path.MoveTo(f32.Pt(float32(cx-glyph), float32(cy)))
		path.LineTo(f32.Pt(float32(cx+glyph), float32(cy)))
	case chromeMaximize:
		if maximized {
			inner := glyph - 2
			path.MoveTo(f32.Pt(float32(cx-inner), float32(cy-glyph)))
			path.LineTo(f32.Pt(float32(cx+glyph), float32(cy-glyph)))
			path.LineTo(f32.Pt(float32(cx+glyph), float32(cy+inner)))
			path.MoveTo(f32.Pt(float32(cx-glyph), float32(cy-inner)))
			path.LineTo(f32.Pt(float32(cx+inner), float32(cy-inner)))
			path.LineTo(f32.Pt(float32(cx+inner), float32(cy+glyph)))
		} else {
			path.MoveTo(f32.Pt(float32(cx-glyph), float32(cy-glyph)))
			path.LineTo(f32.Pt(float32(cx+glyph), float32(cy-glyph)))
			path.LineTo(f32.Pt(float32(cx+glyph), float32(cy+glyph)))
			path.LineTo(f32.Pt(float32(cx-glyph), float32(cy+glyph)))
			path.Close()
		}
	case chromeClose:
		path.MoveTo(f32.Pt(float32(cx-glyph), float32(cy-glyph)))
		path.LineTo(f32.Pt(float32(cx+glyph), float32(cy+glyph)))
		path.MoveTo(f32.Pt(float32(cx+glyph), float32(cy-glyph)))
		path.LineTo(f32.Pt(float32(cx-glyph), float32(cy+glyph)))
	}
	paint.FillShape(ops, col, clip.Stroke{Path: path.End(), Width: 1}.Op())
}
