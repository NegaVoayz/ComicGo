package main

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"comicgo/internal/imgutil"
	"comicgo/internal/model"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/explorer"
)

var (
	colBG       = color.NRGBA{R: 0x16, G: 0x18, B: 0x1d, A: 0xff}
	colBGSoft   = color.NRGBA{R: 0x1d, G: 0x20, B: 0x27, A: 0xff}
	colPanel    = color.NRGBA{R: 0x22, G: 0x26, B: 0x2e, A: 0xff}
	colPanel2   = color.NRGBA{R: 0x2a, G: 0x2f, B: 0x38, A: 0xff}
	colLine     = color.NRGBA{R: 0x34, G: 0x3a, B: 0x45, A: 0xff}
	colText     = color.NRGBA{R: 0xe8, G: 0xea, B: 0xf0, A: 0xff}
	colDim      = color.NRGBA{R: 0x9a, G: 0xa2, B: 0xb0, A: 0xff}
	colAccent   = color.NRGBA{R: 0xe0, G: 0xa4, B: 0x58, A: 0xff}
	colAccentBG = color.NRGBA{R: 0x3a, G: 0x2f, B: 0x23, A: 0xff}
	colFav      = color.NRGBA{R: 0xd9, G: 0x6c, B: 0x6c, A: 0xff}
	colOK       = color.NRGBA{R: 0x6f, G: 0xbf, B: 0x8f, A: 0xff}
	colBlack50  = color.NRGBA{A: 0x80}
)

type screenMode uint8

const (
	screenShelf screenMode = iota
	screenDetail
	screenReader
)

type toastKind uint8

const (
	toastInfo toastKind = iota
	toastOK
	toastError
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
	about        widget.Clickable
	menuClose    widget.Clickable
	rootRemove   []widget.Clickable
}

type appUI struct {
	ops     op.Ops
	win     *app.Window
	backend *App
	dialog  *explorer.Explorer
	events  chan any
	theme   *material.Theme

	screen screenMode
	fatal  error
	press  struct {
		tag       struct{}
		position  image.Point
		hasOrigin bool
	}

	settings    model.Settings
	layoutDoc   model.LayoutDoc
	filterRules []model.RuleInfo
	sortRules   []model.RuleInfo
	sections    []*sectionState
	top         topbarState
	shelfList   layout.List
	welcomeAdd  widget.Clickable

	search string
	tags   []string

	scanning       bool
	scanDone       int
	scanTotal      int
	importing      bool
	importDone     int
	importTotal    int
	importName     string
	importMenu     bool
	dragOver       bool
	dialogBusy     bool
	dropTarget     any
	dropHWND       uintptr
	oleInitialized bool

	aboutOpen     bool
	about         model.About
	removeRoot    string
	aboutClose    widget.Clickable
	aboutBackdrop widget.Clickable
	removeNo      widget.Clickable
	removeYes     widget.Clickable

	refresh int
	indexed int
	toasts  []toastItem
	nextID  int

	covers       map[int64]image.Image
	coverLoading map[int64]bool
	coverFailed  map[int64]bool
	coverOrder   []int64
	coverLimit   int

	detail detailState
	reader readerState

	dialogImportSources []string
}

type uiBackendSink struct{ ui *appUI }

type scanStartedEvent struct{}
type scanProgressEvent struct{ p model.ScanProgress }
type scanDoneEvent struct{ err error }
type scanErrorEvent struct {
	path string
	err  error
}
type importProgressEvent struct{ p model.ImportProgress }
type layoutChangedEvent struct{}
type settingsChangedEvent struct{}
type favoriteChangedEvent struct {
	id  int64
	fav bool
	err error
}
type progressChangedEvent struct {
	id   int64
	page int
	err  error
}
type dropStateEvent struct{ over bool }
type filesDroppedEvent struct{ paths []string }
type toastExpiredEvent struct{ id int }

type sectionCountEvent struct {
	index    int
	revision int
	count    int
}

type indexedCountEvent struct{ count int }

type sectionCardsEvent struct {
	index    int
	revision int
	start    int
	cards    []model.ComicCard
	err      error
}

type coverLoadedEvent struct {
	id  int64
	img image.Image
	err error
}

type detailLoadedEvent struct {
	id     int64
	detail model.ComicDetail
	err    error
}

type detailSavedEvent struct {
	id     int64
	detail model.ComicDetail
	err    error
}

type pageLoadedEvent struct {
	id   int64
	page int
	img  image.Image
	err  error
}

type importFinishedEvent struct {
	out []model.ImportOutcome
	err error
}

type dialogPathEvent struct {
	kind string
	path string
	err  error
}

func (s uiBackendSink) ScanStarted() { s.ui.post(scanStartedEvent{}) }
func (s uiBackendSink) ScanProgress(p model.ScanProgress) {
	s.ui.post(scanProgressEvent{p: p})
}
func (s uiBackendSink) ScanDone(err error) { s.ui.post(scanDoneEvent{err: err}) }
func (s uiBackendSink) ScanError(path string, err error) {
	s.ui.post(scanErrorEvent{path: path, err: err})
}
func (s uiBackendSink) ImportProgress(p model.ImportProgress) {
	s.ui.post(importProgressEvent{p: p})
}
func (s uiBackendSink) LayoutChanged() { s.ui.post(layoutChangedEvent{}) }

func newAppUI(backend *App) *appUI {
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
	ui := &appUI{
		backend:      backend,
		events:       make(chan any, 512),
		theme:        theme,
		screen:       screenShelf,
		covers:       make(map[int64]image.Image),
		coverLoading: make(map[int64]bool),
		coverFailed:  make(map[int64]bool),
		coverLimit:   256,
	}
	ui.shelfList.Axis = layout.Vertical
	return ui
}

func (u *appUI) run() error {
	u.win = new(app.Window)
	windowOptions(u.win)
	u.dialog = explorer.NewExplorer(u.win)
	u.backend.SetEventSink(uiBackendSink{ui: u})

	if err := u.backend.Start(); err != nil {
		u.fatal = err
		log.Printf("start ComicGo: %v", err)
	} else {
		u.loadBackendState()
		if len(u.settings.LibraryRoots) > 0 {
			if err := u.backend.StartScan(); err != nil {
				u.toast(err.Error(), toastError)
			}
		}
	}
	defer u.backend.Close()
	defer u.detachDrop()

	for {
		ev := u.win.Event()
		u.dialog.ListenEvents(ev)
		switch ev := ev.(type) {
		case app.FrameEvent:
			for {
				select {
				case queued := <-u.events:
					u.handleUIEvent(queued)
				default:
					gtx := app.NewContext(&u.ops, ev)
					u.layout(gtx)
					ev.Frame(gtx.Ops)
					goto nextEvent
				}
			}
		case app.Win32ViewEvent:
			if ev.Valid() {
				u.attachDrop(ev.HWND)
			} else {
				u.detachDrop()
			}
		case key.Event:
			u.handleKey(ev)
		case app.DestroyEvent:
			u.saveReaderProgressSync()
			return ev.Err
		}
	nextEvent:
	}
}

func (u *appUI) post(ev any) {
	select {
	case u.events <- ev:
	default:
		go func() { u.events <- ev }()
	}
	if u.win != nil {
		u.win.Invalidate()
	}
}

func (u *appUI) loadBackendState() {
	u.settings = u.backend.GetSettings()
	u.layoutDoc = u.backend.GetLayout()
	all := u.backend.ListRules()
	u.filterRules = u.filterRules[:0]
	u.sortRules = u.sortRules[:0]
	for _, rule := range all {
		switch rule.Kind {
		case "filter":
			u.filterRules = append(u.filterRules, rule)
		case "sort":
			u.sortRules = append(u.sortRules, rule)
		}
	}
	u.resetSections()
	u.top.rootRemove = make([]widget.Clickable, len(u.settings.LibraryRoots))
	u.refreshShelf()
}

func (u *appUI) refreshShelf() {
	u.refresh++
	go func() { u.post(indexedCountEvent{count: u.backend.CountComics("")}) }()
	for i := range u.sections {
		s := u.sections[i]
		s.cards = make(map[int]*cardState)
		s.pending = make(map[int]bool)
		s.total = 0
		u.requestSectionCount(i)
	}
}

func (u *appUI) handleUIEvent(ev any) {
	switch ev := ev.(type) {
	case scanStartedEvent:
		u.scanning = true
		u.scanDone = 0
		u.scanTotal = 0
	case scanProgressEvent:
		u.scanDone = ev.p.Done
		u.scanTotal = ev.p.Total
	case scanDoneEvent:
		u.scanning = false
		if ev.err != nil {
			u.toast("Scan: "+ev.err.Error(), toastError)
		}
		u.layoutDoc = u.backend.GetLayout()
		u.refreshShelf()
	case scanErrorEvent:
		u.toast(fmt.Sprintf("Scan: %s: %v", filepath.Base(ev.path), ev.err), toastError)
	case importProgressEvent:
		u.importing = true
		u.importDone = ev.p.Done
		u.importTotal = ev.p.Total
		u.importName = ev.p.Name
	case layoutChangedEvent:
		u.layoutDoc = u.backend.GetLayout()
		u.resetSections()
		u.refreshShelf()
	case settingsChangedEvent:
		u.settings = u.backend.GetSettings()
		u.top.rootRemove = make([]widget.Clickable, len(u.settings.LibraryRoots))
		u.refreshShelf()
	case favoriteChangedEvent:
		if ev.err != nil {
			u.toast(ev.err.Error(), toastError)
			return
		}
		u.updateCard(ev.id, func(card *model.ComicCard) { card.Fav = ev.fav })
		for i := range u.sections {
			u.requestSectionCount(i)
		}
	case progressChangedEvent:
		if ev.err == nil {
			u.updateCard(ev.id, func(card *model.ComicCard) { card.LastPage = ev.page })
		}
	case dropStateEvent:
		u.dragOver = ev.over
	case filesDroppedEvent:
		u.dragOver = false
		if !u.importing {
			u.beginImport(ev.paths)
		}
	case toastExpiredEvent:
		u.dismissToast(ev.id)
	case sectionCountEvent:
		if ev.index < 0 || ev.index >= len(u.sections) || ev.revision != u.refresh {
			return
		}
		u.sections[ev.index].total = ev.count
	case indexedCountEvent:
		u.indexed = ev.count
	case sectionCardsEvent:
		if ev.index < 0 || ev.index >= len(u.sections) || ev.revision != u.refresh {
			return
		}
		s := u.sections[ev.index]
		delete(s.pending, ev.start)
		if ev.err != nil {
			u.toast("Load shelf: "+ev.err.Error(), toastError)
			return
		}
		for i, card := range ev.cards {
			if _, exists := s.cards[ev.start+i]; !exists {
				s.cards[ev.start+i] = newCardState(card)
			}
		}
	case coverLoadedEvent:
		delete(u.coverLoading, ev.id)
		if ev.err != nil {
			u.coverFailed[ev.id] = true
			return
		}
		u.cacheCover(ev.id, ev.img)
	case detailLoadedEvent:
		if u.detail.id != ev.id {
			return
		}
		u.detail.loading = false
		if ev.err != nil {
			u.detail.err = ev.err.Error()
			return
		}
		u.detail.detail = ev.detail
		u.detail.title = ev.detail.Meta.Title()
		u.detail.loaded = true
		u.requestHero(ev.id)
	case detailSavedEvent:
		u.dialogBusy = false
		if ev.err != nil {
			u.detail.saveErr = ev.err.Error()
			u.toast(ev.err.Error(), toastError)
			return
		}
		u.detail.detail = ev.detail
		u.detail.title = ev.detail.Meta.Title()
		u.detail.editOpen = false
		u.detail.saveErr = ""
		u.toast("Metadata saved", toastOK)
		u.refreshShelf()
	case heroLoadedEvent:
		if u.detail.id != ev.id {
			return
		}
		u.detail.heroLoading = false
		if ev.err == nil {
			u.detail.hero = ev.img
		}
	case pageLoadedEvent:
		if u.reader.id != ev.id || u.reader.page != ev.page {
			return
		}
		u.reader.loading = false
		if ev.err != nil {
			u.reader.err = ev.err.Error()
			return
		}
		u.reader.img = ev.img
		u.reader.err = ""
	case importFinishedEvent:
		u.importing = false
		u.importDone = 0
		u.importTotal = 0
		u.importName = ""
		if ev.err != nil {
			u.toast(ev.err.Error(), toastError)
		}
		if len(ev.out) == 1 {
			u.toast(fmt.Sprintf("Imported %s - %d pages", filepath.Base(ev.out[0].Path), ev.out[0].PageCount), toastOK)
		} else if len(ev.out) > 1 {
			u.toast(fmt.Sprintf("Imported %d comics into the library", len(ev.out)), toastOK)
		}
		if len(ev.out) > 0 {
			u.refreshShelf()
		}
	case dialogPathEvent:
		u.dialogBusy = false
		if ev.err != nil && !isDialogCancel(ev.err) {
			u.toast(ev.err.Error(), toastError)
			return
		}
		if ev.path == "" {
			return
		}
		u.handleDialogPath(ev.kind, ev.path)
	}
}

func isDialogCancel(err error) bool {
	return err == nil || err == explorer.ErrUserDecline
}

func (u *appUI) toast(msg string, kind toastKind) {
	if strings.TrimSpace(msg) == "" {
		return
	}
	u.nextID++
	id := u.nextID
	dur := 3200 * time.Millisecond
	if kind == toastError {
		dur = 6 * time.Second
	}
	u.toasts = append(u.toasts, toastItem{id: id, text: msg, kind: kind, expires: time.Now().Add(dur)})
	if len(u.toasts) > 5 {
		u.toasts = u.toasts[len(u.toasts)-5:]
	}
	time.AfterFunc(dur, func() { u.post(toastExpiredEvent{id: id}) })
	u.invalidate()
}

func (u *appUI) updateCard(id int64, update func(*model.ComicCard)) {
	for _, s := range u.sections {
		for _, card := range s.cards {
			if card.card.ID == id {
				update(&card.card)
			}
		}
	}
	u.invalidate()
}

func (u *appUI) invalidate() {
	if u.win != nil {
		u.win.Invalidate()
	}
}

func (u *appUI) dismissToast(id int) {
	out := u.toasts[:0]
	for _, t := range u.toasts {
		if t.id != id {
			out = append(out, t)
		}
	}
	u.toasts = out
}

func (u *appUI) layout(gtx layout.Context) {
	paint.Fill(gtx.Ops, colBG)
	if u.fatal != nil {
		u.layoutFatal(gtx)
		return
	}
	switch u.screen {
	case screenShelf:
		u.layoutShelf(gtx)
	case screenDetail:
		u.layoutDetail(gtx)
	case screenReader:
		u.layoutReader(gtx)
	}
	if u.importMenu {
		u.layoutImportMenu(gtx)
	}
	if u.aboutOpen {
		u.layoutAbout(gtx)
	}
	if u.removeRoot != "" {
		u.layoutRemoveConfirm(gtx)
	}
	if u.screen == screenDetail && u.detail.editOpen {
		u.layoutMetadataEditor(gtx)
	}
	if u.dragOver {
		u.layoutDropOverlay(gtx)
	}
	u.layoutToasts(gtx)
	u.trackPointerPress(gtx)
}

func (u *appUI) trackPointerPress(gtx layout.Context) {
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &u.press.tag)
	area.Pop()
	pass.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &u.press.tag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if press, ok := ev.(pointer.Event); ok && press.Kind == pointer.Press {
			u.press.position = press.Position.Round()
			u.press.hasOrigin = true
		}
	}
}

func (u *appUI) layoutFatal(gtx layout.Context) {
	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.modal(gtx, unit.Dp(500), func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: 12}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, "ComicGo could not start", unit.Sp(22), colText, 1)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, u.fatal.Error(), unit.Sp(14), colDim, 5)
				}),
			)
		})
	})
}

func (u *appUI) layoutShelfBase(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.layoutTopbar(gtx) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return u.layoutShelfBody(gtx) }),
	)
}

func (u *appUI) layoutTopbar(gtx layout.Context) layout.Dimensions {
	if len(u.top.rootRemove) != len(u.settings.LibraryRoots) {
		u.top.rootRemove = make([]widget.Clickable, len(u.settings.LibraryRoots))
	}
	for i := range u.settings.LibraryRoots {
		if u.top.rootRemove[i].Clicked(gtx) {
			u.removeRoot = u.settings.LibraryRoots[i]
		}
	}
	if u.top.add.Clicked(gtx) {
		u.chooseFolder("library")
	}
	if u.top.scan.Clicked(gtx) {
		if err := u.backend.StartScan(); err != nil {
			u.toast(err.Error(), toastError)
		}
	}
	if u.top.stop.Clicked(gtx) {
		u.backend.CancelScan()
	}
	if u.top.importToggle.Clicked(gtx) {
		u.importMenu = !u.importMenu
	}
	if u.top.reloadLayout.Clicked(gtx) {
		if err := u.backend.ReloadLayout(); err != nil {
			u.toast("Reload layout: "+err.Error(), toastError)
		}
	}
	if u.top.about.Clicked(gtx) {
		u.about = u.backend.GetAbout()
		u.aboutOpen = true
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(52)
			gtx.Constraints.Max.Y = gtx.Dp(52)
			paint.Fill(gtx.Ops, colBGSoft)
			paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(52)
			gtx.Constraints.Max.Y = gtx.Dp(52)
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: 8}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							defer op.Offset(image.Pt(0, gtx.Dp(unit.Dp(1)))).Push(gtx.Ops).Pop()
							return u.label(gtx, "ComicGo", unit.Sp(17), colText, 1)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return u.layoutRoots(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if n := u.indexed; n > 0 {
								return u.indexedBadge(gtx, n)
							}
							return layout.Dimensions{}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if u.scanning {
								return u.layoutScanControls(gtx)
							}
							return layout.Flex{Axis: layout.Horizontal, Gap: 6}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.button(gtx, &u.top.importToggle, "Import", buttonStyle{primary: true, disabled: len(u.settings.LibraryRoots) == 0 || u.importing})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.button(gtx, &u.top.add, "+ Folder", buttonStyle{})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.button(gtx, &u.top.scan, "Scan", buttonStyle{disabled: len(u.settings.LibraryRoots) == 0})
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.button(gtx, &u.top.reloadLayout, "Layout", buttonStyle{})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.button(gtx, &u.top.about, "About", buttonStyle{})
						}),
					)
				})
			})
		}),
	)
}

func (u *appUI) layoutRoots(gtx layout.Context) layout.Dimensions {
	if len(u.settings.LibraryRoots) == 0 {
		return u.label(gtx, "No library selected", unit.Sp(12), colDim, 1)
	}
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	return layout.Flex{Axis: layout.Horizontal, Gap: 5, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Gap: 5, Alignment: layout.Middle}.Layout(gtx,
				func() []layout.FlexChild {
					children := make([]layout.FlexChild, 0, len(u.settings.LibraryRoots)*2)
					for i, root := range u.settings.LibraryRoots {
						i, root := i, root
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.layoutRootChip(gtx, filepath.Base(root), &u.top.rootRemove[i])
						}))
					}
					return children
				}()...)
		}),
	)
}

func (u *appUI) layoutScanControls(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Gap: 7, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Pt(gtx.Dp(120), gtx.Dp(7))
			gtx.Constraints.Max = gtx.Constraints.Min
			paint.FillShape(gtx.Ops, colPanel, clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Min}, NE: 4, NW: 4, SE: 4, SW: 4}.Op(gtx.Ops))
			if u.scanTotal > 0 {
				w := gtx.Constraints.Min.X * u.scanDone / u.scanTotal
				paint.FillShape(gtx.Ops, colAccent, clip.Rect{Max: image.Pt(w, gtx.Constraints.Min.Y)}.Op())
			}
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, fmt.Sprintf("%d/%d", u.scanDone, u.scanTotal), unit.Sp(11), colDim, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.button(gtx, &u.top.stop, "Stop", buttonStyle{})
		}),
	)
}

func (u *appUI) layoutImportMenu(gtx layout.Context) {
	if u.top.menuClose.Clicked(gtx) {
		u.importMenu = false
		return
	}
	if u.top.importFile.Clicked(gtx) {
		u.importMenu = false
		u.chooseImportFile()
	}
	if u.top.importFolder.Clicked(gtx) {
		u.importMenu = false
		u.chooseFolder("import-folder")
	}
	layout.Stack{Alignment: layout.NE}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return u.top.menuClose.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(55), Right: unit.Dp(150)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.panel(gtx, unit.Dp(210), func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: 4}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.button(gtx, &u.top.importFile, "From archive file", buttonStyle{plain: true})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.button(gtx, &u.top.importFolder, "From image folder", buttonStyle{plain: true})
						}),
					)
				})
			})
		}),
	)
}

func (u *appUI) chooseFolder(kind string) {
	if u.dialogBusy || u.dialog == nil {
		return
	}
	u.dialogBusy = true
	go func() {
		path, err := u.dialog.ChooseFolder()
		u.post(dialogPathEvent{kind: kind, path: path, err: err})
	}()
}

func (u *appUI) chooseImportFile() {
	if u.dialogBusy || u.dialog == nil {
		return
	}
	u.dialogBusy = true
	go func() {
		file, err := u.dialog.ChooseFile(".zip", ".cbz", ".rar", ".cbr", ".7z", ".cb7", ".tar", ".gz", ".tgz")
		if err != nil {
			u.post(dialogPathEvent{kind: "import-file", err: err})
			return
		}
		defer file.Close()
		path := ""
		if named, ok := file.(interface{ Name() string }); ok {
			path = named.Name()
		}
		u.post(dialogPathEvent{kind: "import-file", path: path})
	}()
}

func (u *appUI) handleDialogPath(kind, path string) {
	switch kind {
	case "library":
		if err := u.backend.AddLibrary(path); err != nil {
			u.toast(err.Error(), toastError)
			return
		}
		u.settings = u.backend.GetSettings()
		u.top.rootRemove = make([]widget.Clickable, len(u.settings.LibraryRoots))
		u.refreshShelf()
	case "import-file", "import-folder":
		u.beginImport([]string{path})
	case "import-destination":
		u.startImport(u.dialogImportSources, path)
	}
}

func (u *appUI) beginImport(sources []string) {
	roots := u.settings.LibraryRoots
	if len(roots) == 0 {
		u.toast("Add a library folder before importing", toastError)
		return
	}
	if len(roots) == 1 {
		u.startImport(sources, roots[0])
		return
	}
	u.dialogImportSources = sources
	u.chooseFolder("import-destination")
}

func (u *appUI) startImport(sources []string, destination string) {
	if len(sources) == 0 || u.importing {
		return
	}
	u.importing = true
	u.importName = filepath.Base(sources[0])
	go func() {
		out, err := u.backend.ImportSources(sources, destination)
		u.post(importFinishedEvent{out: out, err: err})
	}()
}

func (u *appUI) handleKey(ev key.Event) {
	if ev.State != key.Press {
		return
	}
	if u.handleReaderKey(ev) {
		return
	}
	if ev.Name == key.NameEscape {
		switch {
		case u.screen == screenReader:
			u.closeReader()
		case u.detail.editOpen:
			u.detail.editOpen = false
		case u.aboutOpen:
			u.aboutOpen = false
		case u.importMenu:
			u.importMenu = false
		case u.closeRuleMenu():
			// The filter or sort dropdown consumed Escape.
		case u.screen == screenDetail:
			u.screen = screenShelf
		}
	}
}

// ---- common widgets ----

type buttonStyle struct {
	primary  bool
	solid    bool
	disabled bool
	compact  bool
	plain    bool
	danger   bool
}

func (u *appUI) button(gtx layout.Context, click *widget.Clickable, label string, style buttonStyle) layout.Dimensions {
	bg := colPanel2
	fg := colText
	border := colLine
	if style.primary {
		bg = colAccentBG
		fg = colAccent
		border = colAccent
	}
	if style.solid {
		bg = colAccent
		fg = color.NRGBA{R: 0x14, G: 0x10, B: 0x0a, A: 0xff}
		border = colAccent
	}
	if style.plain {
		bg = color.NRGBA{}
		border = color.NRGBA{}
	}
	if style.danger {
		fg = colFav
	}
	if style.disabled {
		gtx = gtx.Disabled()
		fg = color.NRGBA{R: fg.R, G: fg.G, B: fg.B, A: 0x80}
	}
	if click.Hovered() && !style.disabled {
		border = colAccent
	}
	inset := layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(9), Right: unit.Dp(9)}
	if style.compact {
		inset = layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(3), Right: unit.Dp(3)}
	}
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				if size.X == 0 || size.Y == 0 {
					size = image.Pt(maxInt(size.X, gtx.Dp(unit.Dp(len(label)+4))), maxInt(size.Y, gtx.Dp(26)))
				}
				gtx.Constraints.Min = size
				if border.A != 0 {
					paint.FillShape(gtx.Ops, border, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
					inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
					if bg.A != 0 {
						paint.FillShape(gtx.Ops, bg, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
					}
				} else if bg.A != 0 {
					paint.FillShape(gtx.Ops, bg, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
				}
				return layout.Dimensions{Size: size}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.labelColor(gtx, label, unit.Sp(12), fg, 1, font.Normal)
				})
			}),
		)
	})
}

func (u *appUI) label(gtx layout.Context, s string, size unit.Sp, col color.NRGBA, maxLines int) layout.Dimensions {
	return u.labelColor(gtx, s, size, col, maxLines, font.Normal)
}

func (u *appUI) labelColor(gtx layout.Context, s string, size unit.Sp, col color.NRGBA, maxLines int, weight font.Weight) layout.Dimensions {
	l := material.Label(u.theme, size, s)
	l.Color = col
	l.MaxLines = maxLines
	l.Truncator = "..."
	l.Font.Weight = weight
	return l.Layout(gtx)
}

func (u *appUI) editor(gtx layout.Context, editor *widget.Editor, hint string, height unit.Dp) layout.Dimensions {
	if editor.SingleLine {
		gtx.Constraints.Min.Y = gtx.Dp(height)
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Max.X, maxInt(gtx.Constraints.Min.Y, gtx.Dp(height)))
			paint.FillShape(gtx.Ops, colLine, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			paint.FillShape(gtx.Ops, colBGSoft, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			inset := layout.UniformInset(unit.Dp(7))
			if editor.SingleLine {
				inset = layout.Inset{Top: unit.Dp(9), Bottom: unit.Dp(5), Left: unit.Dp(9), Right: unit.Dp(9)}
			}
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				e := material.Editor(u.theme, editor, hint)
				e.TextSize = 13
				e.Color = colText
				e.HintColor = colDim
				e.SelectionColor = colAccentBG
				return e.Layout(gtx)
			})
		}),
	)
}

func (u *appUI) panel(gtx layout.Context, width unit.Dp, content layout.Widget) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Dp(width)
	gtx.Constraints.Max.X = gtx.Dp(width)
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Max.X, maxInt(gtx.Constraints.Min.Y, gtx.Dp(40)))
			paint.FillShape(gtx.Ops, colLine, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 8, NW: 8, SE: 8, SW: 8}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			paint.FillShape(gtx.Ops, colPanel, clip.RRect{Rect: inner, NE: 7, NW: 7, SE: 7, SW: 7}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.UniformInset(unit.Dp(12)).Layout(gtx, content)
		}),
	)
}

func (u *appUI) modal(gtx layout.Context, width unit.Dp, content layout.Widget) layout.Dimensions {
	return u.panel(gtx, width, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.UniformInset(unit.Dp(18)).Layout(gtx, content)
	})
}

func (u *appUI) indexedBadge(gtx layout.Context, count int) layout.Dimensions {
	height := gtx.Dp(unit.Dp(26))
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Min.X, height)
			paint.FillShape(gtx.Ops, colAccent, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 10, NW: 10, SE: 10, SW: 10}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			paint.FillShape(gtx.Ops, colAccentBG, clip.RRect{Rect: inner, NE: 9, NW: 9, SE: 9, SW: 9}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = height
			gtx.Constraints.Max.Y = height
			return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(9), Right: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, fmt.Sprintf("%d indexed", count), unit.Sp(12), colAccent, 1)
			})
		}),
	)
}

func (u *appUI) layoutRootChip(gtx layout.Context, name string, remove *widget.Clickable) layout.Dimensions {
	height := gtx.Dp(unit.Dp(28))
	closeSize := gtx.Dp(unit.Dp(26))
	gtx.Constraints.Max.X = minInt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(230)))
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Min.X, height)
			border := colLine
			if remove.Hovered() {
				border = colFav
			}
			paint.FillShape(gtx.Ops, border, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 6, NW: 6, SE: 6, SW: 6}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			paint.FillShape(gtx.Ops, colPanel, clip.RRect{Rect: inner, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			gtx.Constraints.Max.Y = height
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Max.X = maxInt(0, gtx.Constraints.Max.X-closeSize)
						return u.label(gtx, name, unit.Sp(12), colDim, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.rootRemoveButton(gtx, remove, closeSize)
					}),
				)
			})
		}),
	)
}

func (u *appUI) rootRemoveButton(gtx layout.Context, click *widget.Clickable, size int) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = gtx.Constraints.Min
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if click.Hovered() {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 0xd9, G: 0x6c, B: 0x6c, A: 0x24}, clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Min}, NE: 5, NW: 5, SE: 5, SW: 5}.Op(gtx.Ops))
		}
		col := colDim
		if click.Hovered() {
			col = colFav
		}
		inset := float32(gtx.Dp(unit.Dp(7)))
		max := float32(size) - inset
		var path clip.Path
		path.Begin(gtx.Ops)
		path.MoveTo(f32.Pt(inset, inset))
		path.LineTo(f32.Pt(max, max))
		path.MoveTo(f32.Pt(max, inset))
		path.LineTo(f32.Pt(inset, max))
		paint.FillShape(gtx.Ops, col, clip.Stroke{
			Path:  path.End(),
			Width: float32(maxInt(1, gtx.Dp(unit.Dp(1.4)))),
		}.Op())
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

func (u *appUI) layoutAbout(gtx layout.Context) {
	if u.aboutClose.Clicked(gtx) {
		u.aboutOpen = false
	}
	if u.aboutBackdrop.Clicked(gtx) {
		u.aboutOpen = false
		return
	}
	layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.Fill(gtx.Ops, colBlack50)
			u.aboutBackdrop.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.modal(gtx, unit.Dp(460), func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: 10}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.labelColor(gtx, "ComicGo", unit.Sp(20), colText, 1, font.Bold)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, "Version "+u.about.Version, unit.Sp(13), colDim, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, fmt.Sprintf("Comics indexed: %d", u.about.ComicCount), unit.Sp(13), colText, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, "Renderer: Gio", unit.Sp(13), colText, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, "Data folder: "+u.about.ConfigDir, unit.Sp(12), colDim, 3)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.button(gtx, &u.aboutClose, "Close", buttonStyle{})
						}),
					)
				})
			})
		}),
	)
}

func (u *appUI) layoutRemoveConfirm(gtx layout.Context) {
	if u.removeNo.Clicked(gtx) {
		u.removeRoot = ""
		return
	}
	if u.removeYes.Clicked(gtx) {
		root := u.removeRoot
		u.removeRoot = ""
		go func() {
			if err := u.backend.RemoveLibrary(root); err != nil {
				u.post(scanErrorEvent{path: root, err: err})
			}
			u.post(settingsChangedEvent{})
		}()
	}
	layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.Fill(gtx.Ops, colBlack50)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.modal(gtx, unit.Dp(520), func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: 12}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.labelColor(gtx, "Remove library folder?", unit.Sp(19), colText, 1, font.Bold)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.label(gtx, u.removeRoot+"\n\nFiles will not be deleted. Its comics will be removed from the index.", unit.Sp(13), colDim, 4)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Gap: 8}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &u.removeNo, "Cancel", buttonStyle{}) }),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.button(gtx, &u.removeYes, "Remove", buttonStyle{danger: true})
								}),
							)
						}),
					)
				})
			})
		}),
	)
}

func (u *appUI) layoutDropOverlay(gtx layout.Context) {
	paint.Fill(gtx.Ops, color.NRGBA{R: 0x11, G: 0x13, B: 0x18, A: 0xd8})
	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.panel(gtx, unit.Dp(420), func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.labelColor(gtx, "Drop archives or image folders to import", unit.Sp(18), colAccent, 2, font.Bold)
			})
		})
	})
}

func (u *appUI) layoutToasts(gtx layout.Context) {
	layout.Stack{Alignment: layout.SE}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = minInt(gtx.Constraints.Max.X, gtx.Dp(420))
			return layout.Inset{Right: unit.Dp(16), Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Gap: 7}.Layout(gtx, func() []layout.FlexChild {
					children := make([]layout.FlexChild, 0, len(u.toasts))
					for _, t := range u.toasts {
						t := t
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							border := colLine
							fg := colText
							if t.kind == toastError {
								border, fg = colFav, color.NRGBA{R: 0xff, G: 0xd9, B: 0xd9, A: 0xff}
							} else if t.kind == toastOK {
								border, fg = colOK, color.NRGBA{R: 0xd6, G: 0xf2, B: 0xe0, A: 0xff}
							}
							return u.toastBox(gtx, t.text, border, fg)
						}))
					}
					return children
				}()...)
			})
		}),
	)
}

func (u *appUI) toastBox(gtx layout.Context, text string, border, fg color.NRGBA) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(38))
			paint.FillShape(gtx.Ops, border, clip.RRect{Rect: image.Rectangle{Max: size}, NE: 8, NW: 8, SE: 8, SW: 8}.Op(gtx.Ops))
			inner := image.Rect(1, 1, maxInt(1, size.X-1), maxInt(1, size.Y-1))
			paint.FillShape(gtx.Ops, colPanel2, clip.RRect{Rect: inner, NE: 7, NW: 7, SE: 7, SW: 7}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(9)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, text, unit.Sp(13), fg, 3)
			})
		}),
	)
}

func (u *appUI) cacheCover(id int64, img image.Image) {
	if _, ok := u.covers[id]; !ok {
		u.coverOrder = append(u.coverOrder, id)
	}
	u.covers[id] = img
	delete(u.coverFailed, id)
	for len(u.coverOrder) > u.coverLimit {
		old := u.coverOrder[0]
		u.coverOrder = u.coverOrder[1:]
		delete(u.covers, old)
	}
}

func (u *appUI) requestCover(id int64) {
	if _, ok := u.covers[id]; ok || u.coverLoading[id] || u.coverFailed[id] {
		return
	}
	u.coverLoading[id] = true
	go func() {
		b, _, err := u.backend.coverBytes(id)
		if err != nil {
			u.post(coverLoadedEvent{id: id, err: err})
			return
		}
		img, _, err := imgutil.Decode(b)
		u.post(coverLoadedEvent{id: id, img: img, err: err})
	}()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clipboardText(text string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(text))
}

func appendClipboard(gtx layout.Context, text string) {
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: clipboardText(text)})
}

func (u *appUI) closeReader() {
	u.saveReaderProgress()
	u.screen = screenDetail
	u.reader.reset()
}

func (u *appUI) saveReaderProgress() {
	if u.reader.id == 0 || u.reader.pageCount <= 0 {
		return
	}
	id, page, count := u.reader.id, u.reader.page, u.reader.pageCount
	go func() {
		if err := u.backend.SetProgress(id, page, count); err != nil {
			log.Printf("save reading progress: %v", err)
			u.post(progressChangedEvent{id: id, page: page, err: err})
			return
		}
		u.post(progressChangedEvent{id: id, page: page})
	}()
}

func (u *appUI) saveReaderProgressSync() {
	if u.reader.id == 0 || u.reader.pageCount <= 0 {
		return
	}
	if err := u.backend.SetProgress(u.reader.id, u.reader.page, u.reader.pageCount); err != nil {
		log.Printf("save reading progress on close: %v", err)
	}
}
