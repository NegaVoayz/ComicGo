package windowing

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

	"comicgo/internal/ui/style"
)

func (u *Controller) LayoutMoveArea(gtx layout.Context, content layout.Widget) layout.Dimensions {
	return u.decorations.LayoutMove(gtx, func(gtx layout.Context) layout.Dimensions {
		return content(gtx)
	})
}

// LayoutControls draws and updates the platform window controls.
func (u *Controller) LayoutControls(gtx layout.Context, height int) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, height)
	gtx.Constraints = layout.Exact(size)
	scale := gtx.Dp(unit.Dp(1))
	u.regions = captionHitRegions(size.X, height, scale)
	if u.backend != nil {
		defer u.backend.setCaptionRegions(u.regions)
	}
	u.decorations.Maximized = u.maximized
	if actions := u.decorations.Update(gtx); actions != 0 {
		u.lastAction = actions
		u.performWindowAction(actions)
	}

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
		buttonHeight = min(height, buttonHeight)
		start := size.X - 3*buttonWidth
		u.layoutWindowsCaptionButton(gtx, chromeMinimize, system.ActionMinimize, start, buttonTop, buttonWidth, buttonHeight)
		u.layoutWindowsCaptionButton(gtx, chromeMaximize, system.ActionMaximize, start+buttonWidth, buttonTop, buttonWidth, buttonHeight)
		u.layoutWindowsCaptionButton(gtx, chromeClose, system.ActionClose, start+2*buttonWidth, buttonTop, buttonWidth, buttonHeight)
	}

	return layout.Dimensions{Size: size}
}

func (u *Controller) layoutWindowsCaptionButton(gtx layout.Context, part chromePart, action system.Action, x, y, buttonWidth, buttonHeight int) {
	child := gtx
	child.Constraints = layout.Exact(image.Pt(buttonWidth, buttonHeight))
	offset := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	defer offset.Pop()

	click := u.decorations.Clickable(action)
	click.Layout(child, func(gtx layout.Context) layout.Dimensions {
		hovered := click.Hovered() || u.hover == part
		bg := caseHoverBackground(part, hovered)
		if bg.A != 0 {
			paint.Fill(gtx.Ops, bg)
		}
		icon := style.Dim
		if hovered && u.focused {
			icon = style.Text
		}
		drawCaptionGlyph(gtx.Ops, part, icon, gtx.Constraints.Max, u.maximized)
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
	return style.Panel2
}

func (u *Controller) layoutMacCaptionButton(gtx layout.Context, part chromePart, action system.Action, x, height int) {
	child := gtx
	child.Constraints = layout.Exact(image.Pt(22, height))
	offset := op.Offset(image.Pt(x-gtx.Dp(unit.Dp(10)), 0)).Push(gtx.Ops)
	defer offset.Pop()

	click := u.decorations.Clickable(action)
	click.Layout(child, func(gtx layout.Context) layout.Dimensions {
		d := gtx.Dp(unit.Dp(12))
		y := (height - d) / 2
		margin := gtx.Dp(unit.Dp(10))
		r := image.Rect(margin, y, margin+d, y+d)
		fill := color.NRGBA{R: 0x69, G: 0x70, B: 0x7b, A: 0xff}
		if u.focused {
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
	glyph := max(6, size.Y/7)
	if part == chromeMaximize && maximized {
		geometry := captionRestoreGeometry(cx, cy, glyph)
		var bracket clip.Path
		bracket.Begin(ops)
		bracket.MoveTo(f32.Pt(float32(geometry.BracketStart.X), float32(geometry.BracketStart.Y)))
		bracket.LineTo(f32.Pt(float32(geometry.BracketCorner.X), float32(geometry.BracketCorner.Y)))
		bracket.LineTo(f32.Pt(float32(geometry.BracketEnd.X), float32(geometry.BracketEnd.Y)))
		paint.FillShape(ops, col, clip.Stroke{Path: bracket.End(), Width: 1}.Op())

		var square clip.Path
		square.Begin(ops)
		addRoundedRectPath(&square, geometry.Square, geometry.Radius)
		paint.FillShape(ops, col, clip.Stroke{Path: square.End(), Width: 1}.Op())
		return
	}
	var path clip.Path
	path.Begin(ops)
	switch part {
	case chromeMinimize:
		path.MoveTo(f32.Pt(float32(cx-glyph), float32(cy)))
		path.LineTo(f32.Pt(float32(cx+glyph), float32(cy)))
	case chromeMaximize:
		addRectPath(&path, image.Rect(cx-glyph, cy-glyph, cx+glyph, cy+glyph))
	case chromeClose:
		path.MoveTo(f32.Pt(float32(cx-glyph), float32(cy-glyph)))
		path.LineTo(f32.Pt(float32(cx+glyph), float32(cy+glyph)))
		path.MoveTo(f32.Pt(float32(cx+glyph), float32(cy-glyph)))
		path.LineTo(f32.Pt(float32(cx-glyph), float32(cy+glyph)))
	}
	paint.FillShape(ops, col, clip.Stroke{Path: path.End(), Width: 1}.Op())
}

type restoreGlyphGeometry struct {
	Square        image.Rectangle
	Radius        int
	BracketStart  image.Point
	BracketCorner image.Point
	BracketEnd    image.Point
}

func captionRestoreGeometry(cx, cy, half int) restoreGlyphGeometry {
	side := 2*half - 2
	squareCenter := image.Pt(cx-1, cy+1)
	square := image.Rectangle{
		Min: squareCenter.Sub(image.Pt(side/2, side/2)),
		Max: image.Pt(squareCenter.X-side/2+side, squareCenter.Y-side/2+side),
	}
	return restoreGlyphGeometry{
		Square:        square,
		Radius:        max(1, half/4),
		BracketStart:  image.Pt(cx-half/2, cy-half-1),
		BracketCorner: image.Pt(cx+half+1, cy-half-1),
		BracketEnd:    image.Pt(cx+half+1, cy+half/2),
	}
}

func addRectPath(path *clip.Path, rect image.Rectangle) {
	path.MoveTo(f32.Pt(float32(rect.Min.X), float32(rect.Min.Y)))
	path.LineTo(f32.Pt(float32(rect.Max.X), float32(rect.Min.Y)))
	path.LineTo(f32.Pt(float32(rect.Max.X), float32(rect.Max.Y)))
	path.LineTo(f32.Pt(float32(rect.Min.X), float32(rect.Max.Y)))
	path.Close()
}

func addRoundedRectPath(path *clip.Path, rect image.Rectangle, radius int) {
	if radius <= 0 {
		addRectPath(path, rect)
		return
	}
	r := min(radius, min(rect.Dx(), rect.Dy())/2)
	path.MoveTo(f32.Pt(float32(rect.Min.X+r), float32(rect.Min.Y)))
	path.LineTo(f32.Pt(float32(rect.Max.X-r), float32(rect.Min.Y)))
	path.QuadTo(
		f32.Pt(float32(rect.Max.X), float32(rect.Min.Y)),
		f32.Pt(float32(rect.Max.X), float32(rect.Min.Y+r)),
	)
	path.LineTo(f32.Pt(float32(rect.Max.X), float32(rect.Max.Y-r)))
	path.QuadTo(
		f32.Pt(float32(rect.Max.X), float32(rect.Max.Y)),
		f32.Pt(float32(rect.Max.X-r), float32(rect.Max.Y)),
	)
	path.LineTo(f32.Pt(float32(rect.Min.X+r), float32(rect.Max.Y)))
	path.QuadTo(
		f32.Pt(float32(rect.Min.X), float32(rect.Max.Y)),
		f32.Pt(float32(rect.Min.X), float32(rect.Max.Y-r)),
	)
	path.LineTo(f32.Pt(float32(rect.Min.X), float32(rect.Min.Y+r)))
	path.QuadTo(
		f32.Pt(float32(rect.Min.X), float32(rect.Min.Y)),
		f32.Pt(float32(rect.Min.X+r), float32(rect.Min.Y)),
	)
	path.Close()
}
