package ui

import (
	"fmt"
	"image"
	"path/filepath"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

func (u *UI) layoutRoots(gtx layout.Context) layout.Dimensions {
	if len(u.Settings.LibraryRoots) == 0 {
		return u.Chrome.LayoutMoveArea(gtx, func(gtx layout.Context) layout.Dimensions {
			return u.Label(gtx, "No library selected", unit.Sp(12), colDim, 1)
		})
	}
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	return layout.Flex{Axis: layout.Horizontal, Gap: 5, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Gap: 5, Alignment: layout.Middle}.Layout(gtx,
				func() []layout.FlexChild {
					children := make([]layout.FlexChild, 0, len(u.Settings.LibraryRoots)*2)
					for i, root := range u.Settings.LibraryRoots {
						i, root := i, root
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.layoutRootChip(gtx, filepath.Base(root), &u.Top.rootRemove[i])
						}))
					}
					return children
				}()...)
		}),
	)
}

func (u *UI) layoutScanControls(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Gap: 7, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Pt(gtx.Dp(120), gtx.Dp(7))
			gtx.Constraints.Max = gtx.Constraints.Min
			paint.FillShape(gtx.Ops, colPanel, clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Min}, NE: 4, NW: 4, SE: 4, SW: 4}.Op(gtx.Ops))
			if u.ScanTotal > 0 {
				w := gtx.Constraints.Min.X * u.ScanDone / u.ScanTotal
				paint.FillShape(gtx.Ops, colAccent, clip.Rect{Max: image.Pt(w, gtx.Constraints.Min.Y)}.Op())
			}
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.Label(gtx, fmt.Sprintf("%d/%d", u.ScanDone, u.ScanTotal), unit.Sp(11), colDim, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.Button(gtx, &u.Top.stop, "Stop", ButtonStyle{})
		}),
	)
}

func (u *UI) layoutImportMenu(gtx layout.Context) {
	if u.Top.menuClose.Clicked(gtx) {
		u.closeImportMenu()
		return
	}
	u.Top.menuClose.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
}

func (u *UI) layoutTopbarImportButton(gtx layout.Context) layout.Dimensions {
	dims := u.Button(gtx, &u.Top.importToggle, "Import", ButtonStyle{
		Primary:  true,
		Disabled: len(u.Settings.LibraryRoots) == 0 || u.Importing,
	})
	if !u.ImportMenu || !u.ImportMenuArmed {
		return dims
	}
	record := op.Record(gtx.Ops)
	buttonTop := (gtx.Dp(unit.Dp(52)) - dims.Size.Y) / 2
	panelTop := gtx.Dp(unit.Dp(55)) - buttonTop
	op.Offset(image.Pt(0, panelTop)).Add(gtx.Ops)
	panelGtx := gtx
	panelGtx.Constraints = layout.Constraints{
		Max: image.Pt(gtx.Dp(unit.Dp(220)), gtx.Dp(unit.Dp(120))),
	}
	u.layoutImportPanel(panelGtx)
	op.Defer(gtx.Ops, record.Stop())
	return dims
}

func (u *UI) layoutImportPanel(gtx layout.Context) layout.Dimensions {
	if u.Top.importFile.Clicked(gtx) {
		u.closeImportMenu()
		u.chooseImportFile()
	}
	if u.Top.importFolder.Clicked(gtx) {
		u.closeImportMenu()
		u.ChooseFolder("import-folder")
	}
	return u.Panel(gtx, unit.Dp(210), func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: 4}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.Button(gtx, &u.Top.importFile, "From archive file", ButtonStyle{Plain: true})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.Button(gtx, &u.Top.importFolder, "From image folder", ButtonStyle{Plain: true})
			}),
		)
	})
}

func (u *UI) ChooseFolder(kind string) {
	if u.DialogBusy || u.Dialog == nil {
		return
	}
	u.DialogBusy = true
	go func() {
		path, err := u.Dialog.ChooseFolder()
		u.Post(DialogPathEvent{Kind: kind, Path: path, Err: err})
	}()
}

func (u *UI) chooseImportFile() {
	if u.DialogBusy || u.Dialog == nil {
		return
	}
	u.DialogBusy = true
	go func() {
		file, err := u.Dialog.ChooseFile(".zip", ".cbz", ".rar", ".cbr", ".7z", ".cb7", ".tar", ".gz", ".tgz")
		if err != nil {
			u.Post(DialogPathEvent{Kind: "import-file", Err: err})
			return
		}
		defer file.Close()
		path := ""
		if named, ok := file.(interface{ Name() string }); ok {
			path = named.Name()
		}
		u.Post(DialogPathEvent{Kind: "import-file", Path: path})
	}()
}

func (u *UI) handleDialogPath(kind, path string) {
	switch kind {
	case "library":
		if err := u.Backend.AddLibrary(path); err != nil {
			u.Toast(err.Error(), ToastError)
			return
		}
		u.Settings = u.Backend.GetSettings()
		u.Top.rootRemove = make([]widget.Clickable, len(u.Settings.LibraryRoots))
		u.refreshShelf()
	case "import-file", "import-folder":
		u.beginImport([]string{path})
	case "import-destination":
		u.startImport(u.DialogImportSources, path)
	}
}

func (u *UI) beginImport(sources []string) {
	roots := u.Settings.LibraryRoots
	if len(roots) == 0 {
		u.Toast("Add a library folder before importing", ToastError)
		return
	}
	if len(roots) == 1 {
		u.startImport(sources, roots[0])
		return
	}
	u.DialogImportSources = sources
	u.ChooseFolder("import-destination")
}

func (u *UI) startImport(sources []string, destination string) {
	if len(sources) == 0 || u.Importing {
		return
	}
	u.Importing = true
	u.ImportName = filepath.Base(sources[0])
	go func() {
		out, err := u.Backend.ImportSources(sources, destination)
		u.Post(ImportFinishedEvent{Out: out, Err: err})
	}()
}

func (u *UI) HandleKey(ev key.Event) {
	if ev.State != key.Press {
		return
	}
	if screenHooks.HandleReaderKey(u, ev) {
		return
	}
	if ev.Name == key.NameEscape {
		switch {
		case u.Screen == ScreenReader:
			u.CloseReader()
		case u.Detail.AuthorOpen:
			u.Detail.AuthorOpen = false
			u.Detail.PickerKind = ""
			u.Detail.PickerSuppressed = true
			u.Invalidate()
		case u.Detail.EditOpen:
			u.Detail.EditOpen = false
			u.Detail.AuthorOpen = false
			u.Invalidate()
		case u.AboutOpen:
			u.AboutOpen = false
		case u.ImportMenu:
			u.closeImportMenu()
		case u.closeRuleMenu():
			// The filter or sort dropdown consumed Escape.
		case u.Screen == ScreenDetail:
			u.Screen = ScreenShelf
		}
	}
}

func (u *UI) closeImportMenu() {
	u.ImportMenu = false
	u.ImportMenuArmed = false
}

// ---- common widgets ----

type ButtonStyle struct {
	Primary  bool
	Solid    bool
	Disabled bool
	Compact  bool
	Plain    bool
	Danger   bool
	Centered bool
}
