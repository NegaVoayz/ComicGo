package ui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"strings"

	"comicgo/internal/imgutil"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func (u *UI) layoutAbout(gtx layout.Context) {
	if u.AboutClose.Clicked(gtx) {
		u.AboutOpen = false
	}
	if u.AboutBackdrop.Clicked(gtx) {
		u.AboutOpen = false
		return
	}
	layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.Fill(gtx.Ops, colBlack50)
			u.AboutBackdrop.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.Modal(gtx, unit.Dp(460), func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: 10}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.LabelColor(gtx, "ComicGo", unit.Sp(20), colText, 1, font.Bold)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.Label(gtx, "Version "+u.about.Version, unit.Sp(13), colDim, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.Label(gtx, fmt.Sprintf("Comics indexed: %d", u.about.ComicCount), unit.Sp(13), colText, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.Label(gtx, "Renderer: Gio", unit.Sp(13), colText, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.Label(gtx, "Data folder: "+u.about.ConfigDir, unit.Sp(12), colDim, 3)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.Button(gtx, &u.AboutClose, "Close", ButtonStyle{})
						}),
					)
				})
			})
		}),
	)
}

func (u *UI) layoutRemoveConfirm(gtx layout.Context) {
	if u.RemoveNo.Clicked(gtx) {
		u.RemoveRoot = ""
		return
	}
	if u.RemoveYes.Clicked(gtx) {
		root := u.RemoveRoot
		u.RemoveRoot = ""
		go func() {
			if err := u.Backend.RemoveLibrary(root); err != nil {
				u.Post(ScanErrorEvent{Path: root, Err: err})
			}
			u.Post(SettingsChangedEvent{})
		}()
	}
	layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.Fill(gtx.Ops, colBlack50)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.Modal(gtx, unit.Dp(520), func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: 12}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.LabelColor(gtx, "Remove library folder?", unit.Sp(19), colText, 1, font.Bold)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.Label(gtx, u.RemoveRoot+"\n\nFiles will not be deleted. Its comics will be removed from the index.", unit.Sp(13), colDim, 4)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Gap: 8}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.Button(gtx, &u.RemoveNo, "Cancel", ButtonStyle{}) }),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return u.Button(gtx, &u.RemoveYes, "Remove", ButtonStyle{Danger: true})
								}),
							)
						}),
					)
				})
			})
		}),
	)
}

func (u *UI) layoutDropOverlay(gtx layout.Context) {
	paint.Fill(gtx.Ops, color.NRGBA{R: 0x11, G: 0x13, B: 0x18, A: 0xd8})
	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.Panel(gtx, unit.Dp(420), func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.LabelColor(gtx, "Drop archives or image folders to import", unit.Sp(18), colAccent, 2, font.Bold)
			})
		})
	})
}

func (u *UI) layoutToasts(gtx layout.Context) {
	layout.Stack{Alignment: layout.SE}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = minInt(gtx.Constraints.Max.X, gtx.Dp(420))
			return layout.Inset{Right: unit.Dp(16), Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Gap: 7}.Layout(gtx, func() []layout.FlexChild {
					children := make([]layout.FlexChild, 0, len(u.Toasts))
					for _, t := range u.Toasts {
						t := t
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							border := colLine
							fg := colText
							if t.kind == ToastError {
								border, fg = colFav, color.NRGBA{R: 0xff, G: 0xd9, B: 0xd9, A: 0xff}
							} else if t.kind == ToastOK {
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

func (u *UI) toastBox(gtx layout.Context, text string, border, fg color.NRGBA) layout.Dimensions {
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
				return u.Label(gtx, text, unit.Sp(13), fg, 3)
			})
		}),
	)
}

func (u *UI) cacheCover(id int64, img image.Image) {
	if _, ok := u.Covers[id]; !ok {
		u.CoverOrder = append(u.CoverOrder, id)
	}
	u.Covers[id] = img
	delete(u.CoverFailed, id)
	for len(u.CoverOrder) > u.CoverLimit {
		old := u.CoverOrder[0]
		u.CoverOrder = u.CoverOrder[1:]
		delete(u.Covers, old)
	}
}

func (u *UI) RequestCover(id int64) {
	if _, ok := u.Covers[id]; ok || u.CoverLoading[id] || u.CoverFailed[id] {
		return
	}
	u.CoverLoading[id] = true
	go func() {
		b, _, err := u.Backend.CoverBytes(id)
		if err != nil {
			u.Post(CoverLoadedEvent{Id: id, Err: err})
			return
		}
		img, _, err := imgutil.Decode(b)
		u.Post(CoverLoadedEvent{Id: id, Img: img, Err: err})
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

func AppendClipboard(gtx layout.Context, text string) {
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: clipboardText(text)})
}

func (u *UI) CloseReader() {
	u.saveReaderProgress()
	u.Screen = ScreenDetail
	u.Reader.reset()
}

func (u *UI) saveReaderProgress() {
	if u.Reader.Id == 0 || u.Reader.PageCount <= 0 {
		return
	}
	id, page, count := u.Reader.Id, u.Reader.ProgressPage(), u.Reader.PageCount
	go func() {
		if err := u.Backend.SetProgress(id, page, count); err != nil {
			log.Printf("save reading progress: %v", err)
			u.Post(ProgressChangedEvent{Id: id, Page: page, Err: err})
			return
		}
		u.Post(ProgressChangedEvent{Id: id, Page: page})
	}()
}

func (u *UI) saveReaderProgressSync() {
	if u.Reader.Id == 0 || u.Reader.PageCount <= 0 {
		return
	}
	if err := u.Backend.SetProgress(u.Reader.Id, u.Reader.ProgressPage(), u.Reader.PageCount); err != nil {
		log.Printf("save reading progress on close: %v", err)
	}
}

// ScreenHooks lets the independent screen package render and manipulate UI
// state without making the event loop depend on that package directly.
type ScreenHooks interface {
	ResetSections(*UI)
	Query(*UI) string
	RequestSectionCount(*UI, int)
	RequestHero(*UI, int64)
	LayoutShelf(*UI, layout.Context) layout.Dimensions
	LayoutShelfBody(*UI, layout.Context) layout.Dimensions
	LayoutDetail(*UI, layout.Context) layout.Dimensions
	LayoutReader(*UI, layout.Context) layout.Dimensions
	HandleReaderKey(*UI, key.Event) bool
	LayoutMetadataEditor(*UI, layout.Context)
}

type noScreenHooks struct{}

func (noScreenHooks) ResetSections(*UI)            {}
func (noScreenHooks) Query(*UI) string             { return "" }
func (noScreenHooks) RequestSectionCount(*UI, int) {}
func (noScreenHooks) RequestHero(*UI, int64)       {}
func (noScreenHooks) LayoutShelf(*UI, layout.Context) layout.Dimensions {
	return layout.Dimensions{}
}
func (noScreenHooks) LayoutShelfBody(*UI, layout.Context) layout.Dimensions {
	return layout.Dimensions{}
}
func (noScreenHooks) LayoutDetail(*UI, layout.Context) layout.Dimensions {
	return layout.Dimensions{}
}
func (noScreenHooks) LayoutReader(*UI, layout.Context) layout.Dimensions {
	return layout.Dimensions{}
}
func (noScreenHooks) HandleReaderKey(*UI, key.Event) bool      { return false }
func (noScreenHooks) LayoutMetadataEditor(*UI, layout.Context) {}

var screenHooks ScreenHooks = noScreenHooks{}

// SetScreenHooks installs the optional screen renderer.
func SetScreenHooks(hooks ScreenHooks) { screenHooks = hooks }
