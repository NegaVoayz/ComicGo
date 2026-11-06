package inputtest

import (
	"image"
	"testing"

	"comicgo/internal/ui"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

func TestEditorHitRegionStopsAtVisibleHeight(t *testing.T) {
	u := ui.NewUI(nil)
	first, second := widget.Editor{SingleLine: true}, widget.Editor{SingleLine: true}
	router, ops := new(input.Router), new(op.Ops)
	frame := func() {
		gtx := layout.Context{
			Ops: ops, Source: router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(300, 140)),
		}
		layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.Editor(gtx, &first, "", 36)
			}),
			layout.Rigid(func(layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(300, 60)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.Editor(gtx, &second, "", 36)
			}),
		)
		router.Frame(ops)
		ops.Reset()
	}
	frame()
	router.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(20, 60),
	})
	frame()
	frame()
	if u.FocusedEditor == &first {
		t.Fatal("clicking the gap below an editor focused it")
	}
}

func TestPointerShieldBlocksUnderlyingHover(t *testing.T) {
	u := ui.NewUI(nil)
	router, ops := new(input.Router), new(op.Ops)
	var under, shield widget.Clickable
	frame := func() {
		gtx := layout.Context{Ops: ops, Source: router.Source(), Constraints: layout.Exact(image.Pt(100, 100))}
		under.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
		u.BlockPointer(gtx, &shield)
		router.Frame(ops)
		ops.Reset()
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(50, 50)})
	frame()
	if under.Hovered() {
		t.Fatal("pointer shield allowed hover to reach the underlying control")
	}
}
