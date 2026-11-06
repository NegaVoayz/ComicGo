package ui

import (
	"image"
	"log"
	"os"
	"time"

	"comicgo/internal/application"
	"comicgo/internal/model"
	"comicgo/internal/ui/style"
	"comicgo/internal/ui/windowing"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/explorer"
)

// App is the backend owned by the desktop UI.
type App = application.App

// Run starts the Gio application and blocks until the process exits.
func Run(backend *application.App) {
	go func() {
		if err := newAppUI(backend).run(); err != nil {
			log.Printf("ComicGo stopped: %v", err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func windowOptions(window *app.Window) {
	window.Option(
		app.Title("ComicGo"),
		app.Size(unit.Dp(1200), unit.Dp(820)),
		app.MinSize(unit.Dp(900), unit.Dp(600)),
		app.Decorated(false),
	)
}

var (
	colBG       = style.BG
	colBGSoft   = style.BGSoft
	colPanel    = style.Panel
	colPanel2   = style.Panel2
	colLine     = style.Line
	colText     = style.Text
	colDim      = style.Dim
	colAccent   = style.Accent
	colAccentBG = style.AccentBG
	colFav      = style.Favorite
	colOK       = style.OK
	colBlack50  = style.BlackHalf
)

type ScreenMode uint8

const (
	ScreenShelf ScreenMode = iota
	ScreenDetail
	ScreenReader
)

type toastKind uint8

const (
	toastInfo toastKind = iota
	ToastOK
	ToastError
)

type toastItem struct {
	id      int
	text    string
	kind    toastKind
	expires time.Time
}

type topbarState struct {
	add          widget.Clickable
	scan         widget.Clickable
	stop         widget.Clickable
	importToggle widget.Clickable
	importFile   widget.Clickable
	importFolder widget.Clickable
	reloadLayout widget.Clickable
	About        widget.Clickable
	menuClose    widget.Clickable
	rootRemove   []widget.Clickable
}

type editorFocusState struct {
	tag byte
}

type UI struct {
	Ops     op.Ops
	Win     *app.Window
	Backend *App
	Dialog  *explorer.Explorer
	Events  chan any
	Theme   *material.Theme

	Screen ScreenMode
	Fatal  error
	Press  struct {
		Tag       struct{}
		Position  image.Point
		HasOrigin bool
		EditorHit bool
	}
	EditorFocus   map[*widget.Editor]*editorFocusState
	FocusedEditor *widget.Editor

	Settings    model.Settings
	LayoutDoc   model.LayoutDoc
	FilterRules []model.RuleInfo
	SortRules   []model.RuleInfo
	Sections    []*SectionState
	Top         topbarState
	ShelfList   layout.List
	WelcomeAdd  widget.Clickable

	Search  string
	Tags    []string
	Authors []string

	Scanning        bool
	ScanDone        int
	ScanTotal       int
	Importing       bool
	ImportDone      int
	ImportTotal     int
	ImportName      string
	ImportMenu      bool
	ImportMenuArmed bool
	DragOver        bool
	DialogBusy      bool

	Chrome *windowing.Controller

	AboutOpen     bool
	about         model.About
	RemoveRoot    string
	AboutClose    widget.Clickable
	AboutBackdrop widget.Clickable
	RemoveNo      widget.Clickable
	RemoveYes     widget.Clickable

	Refresh int
	Indexed int
	Toasts  []toastItem
	NextID  int

	Covers       map[int64]image.Image
	CoverLoading map[int64]bool
	CoverFailed  map[int64]bool
	CoverOrder   []int64
	CoverLimit   int

	Detail DetailState
	Reader ReaderState

	DialogImportSources []string
}

type uiBackendSink struct{ ui *UI }

func (s uiBackendSink) ScanStarted() { s.ui.Post(ScanStartedEvent{}) }
func (s uiBackendSink) ScanProgress(p model.ScanProgress) {
	s.ui.Post(ScanProgressEvent{P: p})
}
func (s uiBackendSink) ScanDone(err error) { s.ui.Post(ScanDoneEvent{err: err}) }
func (s uiBackendSink) ScanError(path string, err error) {
	s.ui.Post(ScanErrorEvent{Path: path, Err: err})
}
func (s uiBackendSink) ImportProgress(p model.ImportProgress) {
	s.ui.Post(ImportProgressEvent{P: p})
}
func (s uiBackendSink) LayoutChanged() { s.ui.Post(LayoutChangedEvent{}) }

func newAppUI(backend *App) *UI {
	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	theme.Palette = material.Palette{
		Bg:         colBG,
		Fg:         colText,
		ContrastBg: colAccent,
		ContrastFg: colBG,
	}
	theme.TextSize = unit.Sp(14)
	theme.FingerSize = unit.Dp(24)
	ui := &UI{
		Backend:      backend,
		Events:       make(chan any, 512),
		Theme:        theme,
		Screen:       ScreenShelf,
		Covers:       make(map[int64]image.Image),
		CoverLoading: make(map[int64]bool),
		CoverFailed:  make(map[int64]bool),
		CoverLimit:   256,
	}
	ui.ShelfList.Axis = layout.Vertical
	ui.Chrome = windowing.New(nil, ui.Post)
	return ui
}

// NewUI creates the desktop UI for embedding and tests.
func NewUI(backend *App) *UI { return newAppUI(backend) }

func (u *UI) editorTextWidth(gtx layout.Context, text string) int {
	measure := gtx
	measure.Constraints.Min = image.Point{}
	measure.Constraints.Max.X = 1 << 20
	macro := op.Record(measure.Ops)
	dims := u.Label(measure, text, unit.Sp(13), colText, 1)
	macro.Stop()
	return dims.Size.X
}
