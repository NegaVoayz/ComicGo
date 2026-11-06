package windowing

import (
	"image"
	"runtime"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
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

// Controller owns the client-rendered window chrome state.
type Controller struct {
	decorations    widget.Decorations
	mode           app.WindowMode
	restoreMode    app.WindowMode
	restorePending bool
	metric         unit.Metric
	windowedSize   image.Point
	maximized      bool
	focused        bool
	hover          chromePart
	regions        chromeHitRegions
	lastAction     system.Action
	post           func(any)
	win            *app.Window
	backend        windowChromeBackend
}

type windowChromeBackend interface {
	handlePlatformEvent(ev any) bool
	setCaptionRegions(chromeHitRegions)
	close()
}

// ActionEvent is posted when a platform caption button is invoked.
type ActionEvent struct {
	Part chromePart
}

// HoverEvent is posted when the native caption hover state changes.
type HoverEvent struct {
	Part chromePart
}

// DropStateEvent reports whether files are hovering over the window.
type DropStateEvent struct{ Over bool }

// FilesDroppedEvent carries the paths released over the window.
type FilesDroppedEvent struct{ Paths []string }

// New creates a window controller attached to the given native window.
func New(win *app.Window, post func(any)) *Controller {
	controller := &Controller{win: win, post: post}
	controller.backend = newWindowChromeBackend(post)
	return controller
}

// AttachWindow updates the native window after the event loop creates it.
func (u *Controller) AttachWindow(win *app.Window) { u.win = win }

// SetMetric records the current display scale for later size restoration.
func (u *Controller) SetMetric(metric unit.Metric) { u.metric = metric }

// Close releases platform hooks owned by the controller.
func (u *Controller) Close() {
	if u.backend != nil {
		u.backend.close()
		u.backend = nil
	}
}

// HandlePlatformEvent forwards native platform events to the backend.
func (u *Controller) HandlePlatformEvent(event any) bool {
	return u.backend != nil && u.backend.handlePlatformEvent(event)
}

// HandleEvent consumes controller-owned events.
func (u *Controller) HandleEvent(event any) bool {
	switch event := event.(type) {
	case ActionEvent:
		u.performWindowAction(chromeAction(event.Part, u.maximized))
		return true
	case HoverEvent:
		u.hover = event.Part
		return true
	default:
		return false
	}
}

// ApplyConfig synchronizes native window state.
func (u *Controller) ApplyConfig(config app.Config) {
	if config.Mode == app.Minimized {
		if u.mode != app.Minimized {
			u.restoreMode = u.mode
		}
	} else if config.Mode == app.Windowed && !u.restorePending && config.Size.X > 0 && config.Size.Y > 0 {
		u.windowedSize = config.Size
	}
	u.mode = config.Mode
	u.maximized = config.Mode == app.Maximized
	u.focused = config.Focused
	u.FinishMinimizedRestore()
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
	bottom := min(height, buttonHeight)
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

func (u *Controller) performWindowAction(actions system.Action) {
	unmaximize := actions&system.ActionUnmaximize != 0
	switch {
	case actions&system.ActionMinimize != 0:
		if u.mode != app.Minimized {
			u.restoreMode = u.mode
		}
		u.mode = app.Minimized
	case actions&system.ActionMaximize != 0:
		u.mode = app.Maximized
		u.maximized = true
	case actions&system.ActionUnmaximize != 0:
		u.mode = app.Windowed
		u.maximized = false
	case actions&system.ActionFullscreen != 0:
		u.mode = app.Fullscreen
		u.maximized = false
	}
	if u.win != nil {
		u.win.Perform(actions)
		if runtime.GOOS == "linux" && unmaximize {
			u.restoreWindowedSize()
		}
	}
}

func (u *Controller) restoreWindowedSize() {
	size := u.windowedSize
	if u.win == nil || size.X <= 0 || size.Y <= 0 {
		return
	}
	u.win.Option(app.Size(
		u.metric.PxToDp(size.X),
		u.metric.PxToDp(size.Y),
	))
}

// recoverMinimizedWindow repairs Gio's stale Linux window mode. Wayland and
// X11 compositors restore a minimized window without changing app.Config.Mode,
// leaving it at Minimized. Gio then refuses title-bar moves and ignores further
// minimize/maximize transitions.
// RecoverMinimizedWindow repairs stale minimized mode after a Linux resume.
func (u *Controller) RecoverMinimizedWindow() {
	if runtime.GOOS != "linux" || u.mode != app.Minimized {
		return
	}
	u.restorePending = u.restoreMode == app.Maximized
	u.mode = app.Windowed
	u.maximized = false
	if u.win != nil {
		u.win.Option(app.Windowed.Option())
	}
}

// FinishMinimizedRestore reapplies maximization after a compositor restore.
func (u *Controller) FinishMinimizedRestore() {
	if !u.restorePending || u.mode != app.Windowed {
		return
	}
	u.restorePending = false
	if u.win != nil {
		u.win.Option(app.Maximized.Option())
	}
}

// CaptionContentInsets returns space reserved for platform controls.
func CaptionContentInsets(height int) layout.Inset {
	if runtime.GOOS == "darwin" {
		return layout.Inset{Left: unit.Dp(macCaptionAreaDp), Right: unit.Dp(8)}
	}
	return layout.Inset{
		Left:  unit.Dp(8),
		Right: unit.Dp(3*captionButtonWidthDp + 8),
	}
}

// LayoutMoveArea makes inert caption space draggable and lays out content
// inside the draggable region.
