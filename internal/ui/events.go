package ui

import (
	"fmt"
	"log"
	"path/filepath"

	"comicgo/internal/model"
	"comicgo/internal/ui/windowing"

	"gioui.org/app"
	"gioui.org/widget"
	"gioui.org/x/explorer"
)

func (u *UI) run() error {
	u.Win = new(app.Window)
	windowOptions(u.Win)
	u.Chrome.AttachWindow(u.Win)
	u.Dialog = explorer.NewExplorer(u.Win)
	u.Backend.SetEventSink(uiBackendSink{ui: u})

	if err := u.Backend.Start(); err != nil {
		u.Fatal = err
		log.Printf("start ComicGo: %v", err)
	} else {
		u.loadBackendState()
		if len(u.Settings.LibraryRoots) > 0 {
			if err := u.Backend.StartScan(); err != nil {
				u.Toast(err.Error(), ToastError)
			}
		}
	}
	defer u.Backend.Close()
	defer u.Chrome.Close()

	for {
		ev := u.Win.Event()
		u.Dialog.ListenEvents(ev)
		if u.Chrome.HandlePlatformEvent(ev) {
			continue
		}
		switch ev := ev.(type) {
		case app.FrameEvent:
			u.Chrome.SetMetric(ev.Metric)
			u.Chrome.RecoverMinimizedWindow()
			for {
				select {
				case queued := <-u.Events:
					u.handleUIEvent(queued)
				default:
					gtx := app.NewContext(&u.Ops, ev)
					u.layout(gtx)
					ev.Frame(gtx.Ops)
					goto nextEvent
				}
			}
		case app.ConfigEvent:
			u.Chrome.ApplyConfig(ev.Config)
		case app.DestroyEvent:
			u.saveReaderProgressSync()
			return ev.Err
		}
	nextEvent:
	}
}

func (u *UI) applyWindowConfig(config app.Config) {
	u.Chrome.ApplyConfig(config)
}

func (u *UI) Post(ev any) {
	select {
	case u.Events <- ev:
	default:
		go func() { u.Events <- ev }()
	}
	if u.Win != nil {
		u.Win.Invalidate()
	}
}

func (u *UI) loadBackendState() {
	u.Settings = u.Backend.GetSettings()
	u.LayoutDoc = u.Backend.GetLayout()
	all := u.Backend.ListRules()
	u.FilterRules = u.FilterRules[:0]
	u.SortRules = u.SortRules[:0]
	for _, rule := range all {
		switch rule.Kind {
		case "filter":
			u.FilterRules = append(u.FilterRules, rule)
		case "sort":
			u.SortRules = append(u.SortRules, rule)
		}
	}
	screenHooks.ResetSections(u)
	u.Top.rootRemove = make([]widget.Clickable, len(u.Settings.LibraryRoots))
	u.refreshShelf()
}

func (u *UI) refreshShelf() {
	go func() { u.Post(IndexedCountEvent{Count: u.Backend.CountComics("")}) }()
	u.RefreshShelfQuery()
}

func (u *UI) RefreshShelfQuery() {
	u.Refresh++
	for i := range u.Sections {
		s := u.Sections[i]
		s.Pending = make(map[int]bool)
		screenHooks.RequestSectionCount(u, i)
	}
}

func (u *UI) handleUIEvent(ev any) {
	switch ev := ev.(type) {
	case ScanStartedEvent:
		u.Scanning = true
		u.ScanDone = 0
		u.ScanTotal = 0
	case ScanProgressEvent:
		u.ScanDone = ev.P.Done
		u.ScanTotal = ev.P.Total
	case ScanDoneEvent:
		u.Scanning = false
		if ev.err != nil {
			u.Toast("Scan: "+ev.err.Error(), ToastError)
		}
		u.LayoutDoc = u.Backend.GetLayout()
		u.refreshShelf()
	case ScanErrorEvent:
		u.Toast(fmt.Sprintf("Scan: %s: %v", filepath.Base(ev.Path), ev.Err), ToastError)
	case ImportProgressEvent:
		u.Importing = true
		u.ImportDone = ev.P.Done
		u.ImportTotal = ev.P.Total
		u.ImportName = ev.P.Name
	case LayoutChangedEvent:
		u.LayoutDoc = u.Backend.GetLayout()
		screenHooks.ResetSections(u)
		u.refreshShelf()
	case SettingsChangedEvent:
		u.Settings = u.Backend.GetSettings()
		u.Top.rootRemove = make([]widget.Clickable, len(u.Settings.LibraryRoots))
		u.refreshShelf()
	case FavoriteChangedEvent:
		if ev.Err != nil {
			u.Toast(ev.Err.Error(), ToastError)
			return
		}
		u.updateCard(ev.Id, func(card *model.ComicCard) { card.Fav = ev.Fav })
		for i := range u.Sections {
			screenHooks.RequestSectionCount(u, i)
		}
	case ProgressChangedEvent:
		if ev.Err == nil {
			u.updateCard(ev.Id, func(card *model.ComicCard) { card.LastPage = ev.Page })
		}
	case windowing.DropStateEvent:
		u.DragOver = ev.Over
	case windowing.FilesDroppedEvent:
		u.DragOver = false
		if !u.Importing {
			u.beginImport(ev.Paths)
		}
	case ToastExpiredEvent:
		u.dismissToast(ev.Id)
	case SectionCountEvent:
		if ev.Index < 0 || ev.Index >= len(u.Sections) || ev.Revision != u.Refresh {
			return
		}
		s := u.Sections[ev.Index]
		s.Total = ev.Count
		if ev.Count == 0 {
			s.SearchPending = false
		}
	case IndexedCountEvent:
		u.Indexed = ev.Count
	case SectionCardsEvent:
		if ev.Index < 0 || ev.Index >= len(u.Sections) || ev.Revision != u.Refresh {
			return
		}
		s := u.Sections[ev.Index]
		delete(s.Pending, ev.Start)
		if ev.Err != nil {
			if ev.Start == 0 {
				s.SearchPending = false
			}
			u.Toast("Load shelf: "+ev.Err.Error(), ToastError)
			return
		}
		for i, card := range ev.Cards {
			s.Cards[ev.Start+i] = NewCardState(card)
			s.CardRevisions[ev.Start+i] = ev.Revision
		}
		for i := ev.Start + len(ev.Cards); i < ev.Start+ev.Limit; i++ {
			delete(s.Cards, i)
			delete(s.CardRevisions, i)
		}
		if ev.Start == 0 {
			s.SearchPending = false
		}
	case CoverLoadedEvent:
		delete(u.CoverLoading, ev.Id)
		if ev.Err != nil {
			u.CoverFailed[ev.Id] = true
			return
		}
		u.cacheCover(ev.Id, ev.Img)
	case DetailLoadedEvent:
		if u.Detail.Id != ev.Id {
			return
		}
		u.Detail.Loading = false
		if ev.Err != nil {
			u.Detail.Err = ev.Err.Error()
			return
		}
		u.Detail.Detail = ev.Detail
		u.Detail.Title = ev.Detail.Meta.Title()
		u.Detail.Loaded = true
		screenHooks.RequestHero(u, ev.Id)
	case AuthorsLoadedEvent:
		screenHooks.HandleAuthorsLoaded(u, ev)
	case DetailSavedEvent:
		u.DialogBusy = false
		if ev.Err != nil {
			u.Detail.SaveErr = ev.Err.Error()
			u.Toast(ev.Err.Error(), ToastError)
			return
		}
		u.Detail.Detail = ev.Detail
		u.Detail.Title = ev.Detail.Meta.Title()
		u.Detail.EditOpen = false
		u.Detail.SaveErr = ""
		u.Toast("Metadata saved", ToastOK)
		u.refreshShelf()
	case HeroLoadedEvent:
		if u.Detail.Id != ev.Id {
			return
		}
		u.Detail.HeroLoading = false
		if ev.Err == nil {
			u.Detail.Hero = ev.Img
		}
	case PageLoadedEvent:
		if !u.Reader.AcceptsPage(ev.Id, ev.Slot, ev.Page) {
			return
		}
		u.Reader.Loading[ev.Slot] = false
		if ev.Err != nil {
			u.Reader.Errors[ev.Slot] = ev.Err.Error()
			return
		}
		u.Reader.Images[ev.Slot] = ev.Img
		u.Reader.Errors[ev.Slot] = ""
	case ImportFinishedEvent:
		u.Importing = false
		u.ImportDone = 0
		u.ImportTotal = 0
		u.ImportName = ""
		if ev.Err != nil {
			u.Toast(ev.Err.Error(), ToastError)
		}
		if len(ev.Out) == 1 {
			u.Toast(fmt.Sprintf("Imported %s - %d pages", filepath.Base(ev.Out[0].Path), ev.Out[0].PageCount), ToastOK)
		} else if len(ev.Out) > 1 {
			u.Toast(fmt.Sprintf("Imported %d comics into the library", len(ev.Out)), ToastOK)
		}
		if len(ev.Out) > 0 {
			u.refreshShelf()
		}
	case DialogPathEvent:
		u.DialogBusy = false
		if ev.Err != nil && !isDialogCancel(ev.Err) {
			u.Toast(dialogErrorMessage(ev.Err), ToastError)
			return
		}
		if ev.Path == "" {
			return
		}
		u.handleDialogPath(ev.Kind, ev.Path)
	default:
		if !u.Chrome.HandleEvent(ev) {
			log.Printf("unhandled UI event %T", ev)
		}
	}
}
