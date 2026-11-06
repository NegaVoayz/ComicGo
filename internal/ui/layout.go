package ui

import (
	"image"
	"strings"
	"time"

	"comicgo/internal/model"
	"comicgo/internal/ui/windowing"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/explorer"
)

func isDialogCancel(err error) bool {
	return err == nil || err == explorer.ErrUserDecline
}

func dialogErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "org.freedesktop.portal.Desktop") {
		return "File picker unavailable. Install and start xdg-desktop-portal and a desktop backend such as xdg-desktop-portal-gtk."
	}
	return msg
}

func (u *UI) Toast(msg string, kind toastKind) {
	if strings.TrimSpace(msg) == "" {
		return
	}
	u.NextID++
	id := u.NextID
	dur := 3200 * time.Millisecond
	if kind == ToastError {
		dur = 6 * time.Second
	}
	u.Toasts = append(u.Toasts, toastItem{id: id, text: msg, kind: kind, expires: time.Now().Add(dur)})
	if len(u.Toasts) > 5 {
		u.Toasts = u.Toasts[len(u.Toasts)-5:]
	}
	time.AfterFunc(dur, func() { u.Post(ToastExpiredEvent{Id: id}) })
	u.Invalidate()
}

func (u *UI) updateCard(id int64, update func(*model.ComicCard)) {
	for _, s := range u.Sections {
		for _, card := range s.Cards {
			if card.Card.ID == id {
				update(&card.Card)
			}
		}
	}
	u.Invalidate()
}

func (u *UI) Invalidate() {
	if u.Win != nil {
		u.Win.Invalidate()
	}
}

func (u *UI) BlockPointer(gtx layout.Context, click *widget.Clickable) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
}

func (u *UI) dismissToast(id int) {
	out := u.Toasts[:0]
	for _, t := range u.Toasts {
		if t.id != id {
			out = append(out, t)
		}
	}
	u.Toasts = out
}

func (u *UI) layout(gtx layout.Context) {
	screen := u.Screen
	paint.Fill(gtx.Ops, colBG)
	u.Press.EditorHit = false
	u.FocusedEditor = nil
	if u.Fatal != nil {
		u.layoutFatal(gtx)
	} else {
		switch u.Screen {
		case ScreenShelf:
			screenHooks.LayoutShelf(u, gtx)
		case ScreenDetail:
			screenHooks.LayoutDetail(u, gtx)
		case ScreenReader:
			screenHooks.LayoutReader(u, gtx)
		}
	}
	if u.ImportMenu && u.ImportMenuArmed {
		u.layoutImportMenu(gtx)
	}
	if u.AboutOpen {
		u.layoutAbout(gtx)
	}
	if u.RemoveRoot != "" {
		u.layoutRemoveConfirm(gtx)
	}
	if u.Screen == ScreenDetail && u.Detail.EditOpen {
		screenHooks.LayoutMetadataEditor(u, gtx)
	}
	if u.DragOver {
		u.layoutDropOverlay(gtx)
	}
	u.layoutToasts(gtx)
	captionHeight := 52
	if u.Screen == ScreenReader {
		captionHeight = 36
	}
	u.Chrome.LayoutControls(gtx, gtx.Dp(unit.Dp(captionHeight)))
	u.trackPointerPress(gtx)
	if u.Screen != screen {
		u.Invalidate()
	}
}

func (u *UI) trackPointerPress(gtx layout.Context) {
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &u.Press.Tag)
	area.Pop()
	pass.Pop()
	hadPress := false
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &u.Press.Tag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if press, ok := ev.(pointer.Event); ok && press.Kind == pointer.Press {
			hadPress = true
			u.Press.Position = press.Position.Round()
			u.Press.HasOrigin = true
		}
	}
	if hadPress && !u.Press.EditorHit && u.FocusedEditor != nil {
		gtx.Execute(key.FocusCmd{})
	}
}

func (u *UI) layoutFatal(gtx layout.Context) {
	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.Modal(gtx, unit.Dp(500), func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: 12}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.Label(gtx, "ComicGo could not start", unit.Sp(22), colText, 1)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.Label(gtx, u.Fatal.Error(), unit.Sp(14), colDim, 5)
				}),
			)
		})
	})
}

func (u *UI) LayoutShelfBase(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.layoutTopbar(gtx) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return screenHooks.LayoutShelfBody(u, gtx)
		}),
	)
}

func (u *UI) layoutTopbar(gtx layout.Context) layout.Dimensions {
	if u.ImportMenu && !u.ImportMenuArmed {
		u.ImportMenuArmed = true
	}
	if len(u.Top.rootRemove) != len(u.Settings.LibraryRoots) {
		u.Top.rootRemove = make([]widget.Clickable, len(u.Settings.LibraryRoots))
	}
	for i := range u.Settings.LibraryRoots {
		if u.Top.rootRemove[i].Clicked(gtx) {
			u.RemoveRoot = u.Settings.LibraryRoots[i]
		}
	}
	if u.Top.add.Clicked(gtx) {
		u.ChooseFolder("library")
	}
	if u.Top.scan.Clicked(gtx) {
		if err := u.Backend.StartScan(); err != nil {
			u.Toast(err.Error(), ToastError)
		}
	}
	if u.Top.stop.Clicked(gtx) {
		u.Backend.CancelScan()
	}
	if u.Top.importToggle.Clicked(gtx) {
		if u.ImportMenu {
			u.closeImportMenu()
		} else {
			u.ImportMenu = true
			u.ImportMenuArmed = false
		}
	}
	if u.Top.reloadLayout.Clicked(gtx) {
		if err := u.Backend.ReloadLayout(); err != nil {
			u.Toast("Reload layout: "+err.Error(), ToastError)
		}
	}
	if u.Top.About.Clicked(gtx) {
		u.about = u.Backend.GetAbout()
		u.AboutOpen = true
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(52)
			gtx.Constraints.Max.Y = gtx.Dp(52)
			return u.Chrome.LayoutMoveArea(gtx, func(gtx layout.Context) layout.Dimensions {
				paint.Fill(gtx.Ops, colBGSoft)
				paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(52)
			gtx.Constraints.Max.Y = gtx.Dp(52)
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				inset := windowing.CaptionContentInsets(52)
				inset.Top = 0
				inset.Bottom = 0
				return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: 8}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							width := gtx.Dp(unit.Dp(112))
							gtx.Constraints.Min.X = width
							gtx.Constraints.Max.X = width
							gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
							return u.Chrome.LayoutMoveArea(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									defer op.Offset(image.Pt(0, gtx.Dp(unit.Dp(1)))).Push(gtx.Ops).Pop()
									return u.Label(gtx, "ComicGo", unit.Sp(17), colText, 1)
								})
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.layoutRoots(gtx)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return u.Chrome.LayoutMoveArea(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{Size: gtx.Constraints.Max}
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if n := u.Indexed; n > 0 {
								return u.indexedBadge(gtx, n)
							}
							return layout.Dimensions{}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if u.Scanning {
								return u.layoutScanControls(gtx)
							}
							return layout.Flex{Axis: layout.Horizontal, Gap: 6}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.layoutTopbarImportButton(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.Button(gtx, &u.Top.add, "+ Folder", ButtonStyle{})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.Button(gtx, &u.Top.scan, "Scan", ButtonStyle{Disabled: len(u.Settings.LibraryRoots) == 0})
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.Button(gtx, &u.Top.reloadLayout, "Layout", ButtonStyle{})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.Button(gtx, &u.Top.About, "About", ButtonStyle{})
						}),
					)
				})
			})
		}),
	)
}
