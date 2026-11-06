package windowing

import (
	"image"
	"runtime"
	"testing"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestCaptionControlHitRegions(t *testing.T) {
	regions := captionHitRegions(1200, 52, 1)
	tests := []struct {
		name string
		part chromePart
	}{
		{name: "minimize", part: chromeMinimize},
		{name: "maximize", part: chromeMaximize},
		{name: "close", part: chromeClose},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rect := chromeRegionForPart(regions, tc.part)
			if rect.Empty() {
				t.Fatalf("%s hit region is empty", tc.name)
			}
			if rect.Dy() >= rect.Dx() {
				t.Fatalf("%s control is %v; want landscape proportions", tc.name, rect.Size())
			}
			if rect.Min.Y != captionButtonTopDp {
				t.Fatalf("%s control starts at y=%d; want %d", tc.name, rect.Min.Y, captionButtonTopDp)
			}
			center := rect.Min.Add(image.Pt(rect.Dx()/2, rect.Dy()/2))
			if got := regions.at(center); got != tc.part {
				t.Fatalf("hit at %v = %v, want %v", center, got, tc.part)
			}
		})
	}
	if got := regions.at(image.Pt(600, 20)); got != chromeNone {
		t.Fatalf("content hit = %v, want chromeNone", got)
	}
	if !image.Pt(600, 20).In(regions.Caption) {
		t.Fatal("caption context-menu region does not cover the title strip")
	}
}

func chromeRegionForPart(regions chromeHitRegions, part chromePart) image.Rectangle {
	switch part {
	case chromeMinimize:
		return regions.Minimize
	case chromeMaximize:
		return regions.Maximize
	case chromeClose:
		return regions.Close
	default:
		return image.Rectangle{}
	}
}

func TestChromeActionTracksMaximizedState(t *testing.T) {
	if got := chromeAction(chromeMaximize, false); got != system.ActionMaximize {
		t.Fatalf("windowed maximize action = %v", got)
	}
	if got := chromeAction(chromeMaximize, true); got != system.ActionUnmaximize {
		t.Fatalf("maximized maximize action = %v", got)
	}
	if got := chromeAction(chromeClose, false); got != system.ActionClose {
		t.Fatalf("close action = %v", got)
	}
}

func TestCaptionControlsReceiveClicksAboveDragSurface(t *testing.T) {
	u := New(nil, func(any) {})
	router := new(input.Router)
	ops := new(op.Ops)
	frame := func() {
		gtx := layout.Context{
			Ops:         ops,
			Source:      router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1200, 52)),
		}

		u.LayoutControls(gtx, 52)
		router.Frame(ops)
		ops.Reset()
	}
	frame()

	regions := captionHitRegions(1200, 52, 1)
	tests := []struct {
		name string
		part chromePart
		want system.Action
	}{
		{name: "minimize", part: chromeMinimize, want: system.ActionMinimize},
		{name: "maximize", part: chromeMaximize, want: system.ActionMaximize},
		{name: "close", part: chromeClose, want: system.ActionClose},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rect := chromeRegionForPart(regions, tc.part)
			position := f32.Pt(float32(rect.Min.X+rect.Dx()/2), float32(rect.Min.Y+rect.Dy()/2))
			u.lastAction = 0
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: position})
			frame()
			if !u.decorations.Clickable(chromeAction(tc.part, false)).Hovered() {
				t.Fatalf("control at %v is not hovered", position)
			}
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: position})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
			frame()
			if u.lastAction != tc.want {
				t.Fatalf("action = %v, want %v", u.lastAction, tc.want)
			}
		})
	}
}

func TestRestoreGlyphMatchesWindowsGeometry(t *testing.T) {
	geometry := captionRestoreGeometry(20, 20, 6)
	if geometry.Square.Empty() || geometry.Square.Dx() != geometry.Square.Dy() {
		t.Fatalf("restore square = %v; want a non-empty square", geometry.Square)
	}
	if geometry.Radius <= 0 || geometry.Radius*2 >= geometry.Square.Dx() {
		t.Fatalf("restore square radius = %d for size %v", geometry.Radius, geometry.Square.Size())
	}
	if geometry.BracketCorner.X <= geometry.Square.Max.X || geometry.BracketCorner.Y >= geometry.Square.Min.Y {
		t.Fatalf("restore bracket corner %v is not outside the square top-right", geometry.BracketCorner)
	}
	if geometry.BracketStart.Y != geometry.BracketCorner.Y || geometry.BracketEnd.X != geometry.BracketCorner.X {
		t.Fatalf("restore bracket is not a right angle: %+v", geometry)
	}
	if geometry.BracketCorner != image.Pt(27, 13) {
		t.Fatalf("restore bracket corner = %v; want 27,13", geometry.BracketCorner)
	}
	if gap := geometry.Square.Min.Y - geometry.BracketStart.Y; gap != 3 {
		t.Fatalf("restore top clearance = %d; want 3", gap)
	}
	if gap := geometry.BracketCorner.X - geometry.Square.Max.X; gap != 3 {
		t.Fatalf("restore right clearance = %d; want 3", gap)
	}
}

func TestWindowConfigUpdatesChromeState(t *testing.T) {
	var u Controller
	u.ApplyConfig(app.Config{Mode: app.Maximized, Focused: true})
	if !u.maximized || !u.focused {
		t.Fatalf("maximized chrome = %+v", u)
	}
	u.ApplyConfig(app.Config{Mode: app.Windowed, Focused: false})
	if u.maximized || u.focused {
		t.Fatalf("restored chrome = %+v", u)
	}
}

func TestMinimizedWindowPreservesRestoreMode(t *testing.T) {
	var u Controller
	windowedSize := image.Pt(960, 688)
	u.ApplyConfig(app.Config{Mode: app.Windowed, Size: windowedSize, Focused: true})
	u.ApplyConfig(app.Config{Mode: app.Maximized, Size: image.Pt(2048, 1104), Focused: true})
	u.ApplyConfig(app.Config{Mode: app.Minimized, Focused: false})
	if u.mode != app.Minimized {
		t.Fatalf("minimized mode = %v; want %v", u.mode, app.Minimized)
	}
	if u.restoreMode != app.Maximized {
		t.Fatalf("restore mode = %v; want %v", u.restoreMode, app.Maximized)
	}
	if u.windowedSize != windowedSize {
		t.Fatalf("windowed size = %v; want %v", u.windowedSize, windowedSize)
	}

	u.RecoverMinimizedWindow()
	if runtime.GOOS != "linux" {
		if u.mode != app.Minimized || u.restorePending {
			t.Fatalf("non-Linux recovery changed chrome: %+v", u)
		}
		return
	}
	if u.mode != app.Windowed || !u.restorePending {
		t.Fatalf("recovered chrome = %+v; want pending maximized restore", u)
	}
	u.ApplyConfig(app.Config{Mode: app.Windowed, Size: image.Pt(2048, 1104), Focused: true})
	if u.restorePending {
		t.Fatalf("maximize restore remained pending after windowed config: %+v", u)
	}
	if u.windowedSize != windowedSize {
		t.Fatalf("restore sync overwrote windowed size with %v; want %v", u.windowedSize, windowedSize)
	}
	u.ApplyConfig(app.Config{Mode: app.Maximized, Focused: true})
	if !u.maximized || u.mode != app.Maximized {
		t.Fatalf("maximized restore did not complete: %+v", u)
	}
}
