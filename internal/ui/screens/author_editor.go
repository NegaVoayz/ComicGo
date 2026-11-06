package screens

import (
	"comicgo/internal/model"
	. "comicgo/internal/ui"
	"comicgo/internal/ui/metadata"
	"errors"
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/outlay"
)

func (u *Renderer) openMetadataEditor() {
	m := u.Detail.Detail.Meta
	u.Detail.NameEditor.SetText(m.Title())
	u.Detail.AuthorEditor.SetText(m.KV["Author"])
	u.Detail.TagsEditor.SetText("")
	u.Detail.SourceEditor.SetText(m.KV["Source"])
	u.Detail.Tags = SplitTags(m.KV["Genre"])
	u.Detail.TagRemove = nil
	u.Detail.SaveErr = ""
	u.closePicker()
	u.Detail.PickerSuppressed = false
	u.Detail.AuthorValues = nil
	u.Detail.TagValues = nil
	u.Detail.AuthorList = layout.List{Axis: layout.Vertical}
	u.Detail.EditOpen = true
	id := u.Detail.Id
	go func() {
		authors, authorErr := u.Backend.ListAuthors()
		tags, tagErr := u.Backend.ListTags()
		u.Post(AuthorsLoadedEvent{
			Id: id, Authors: authors, Tags: tags, Err: errors.Join(authorErr, tagErr),
		})
	}()
}

func (u *Renderer) handleAuthorsLoaded(ev AuthorsLoadedEvent) {
	if u.Detail.Id != ev.Id || !u.Detail.EditOpen {
		return
	}
	if ev.Err != nil {
		u.Toast("Load metadata choices: "+ev.Err.Error(), ToastError)
		return
	}
	u.Detail.AuthorValues = append(u.Detail.AuthorValues[:0], ev.Authors...)
	u.Detail.TagValues = append(u.Detail.TagValues[:0], ev.Tags...)
}

func (u *Renderer) addDraftTags(raw string) bool {
	tags, changed := metadata.AddTags(u.Detail.Tags, raw)
	u.Detail.Tags = tags
	return changed
}

func (u *Renderer) removeDraftTag(index int) {
	u.Detail.Tags = metadata.RemoveTag(u.Detail.Tags, index)
}

func (u *Renderer) commitDraftTags() []string {
	tags := append([]string(nil), u.Detail.Tags...)
	u.Detail.Tags = tags
	u.addDraftTags(u.Detail.TagsEditor.Text())
	u.Detail.TagsEditor.SetText("")
	return u.Detail.Tags
}

func (u *Renderer) layoutMetadataEditor(gtx layout.Context) {
	authorFocused := gtx.Focused(&u.Detail.AuthorEditor)
	tagFocused := gtx.Focused(&u.Detail.TagsEditor)
	if !authorFocused && !tagFocused {
		u.Detail.PickerSuppressed = false
	}
	if !u.Detail.PickerSuppressed {
		switch {
		case authorFocused && (u.Detail.PickerKind != "author" || !u.Detail.AuthorOpen):
			u.openPicker("author", gtx)
		case tagFocused && (u.Detail.PickerKind != "tag" || !u.Detail.AuthorOpen):
			u.openPicker("tag", gtx)
		}
	}
	if u.Detail.Cancel.Clicked(gtx) {
		u.Detail.EditOpen = false
		u.closePicker()
		u.Invalidate()
	}
	if u.Detail.TagAdd.Clicked(gtx) {
		if u.addDraftTags(u.Detail.TagsEditor.Text()) {
			u.Detail.TagsEditor.SetText("")
		}
		u.closePicker()
		u.Invalidate()
	}
	openedKind, wasOpen := u.Detail.PickerKind, u.Detail.AuthorOpen
	if u.Detail.Save.Clicked(gtx) && !u.DialogBusy {
		edit := model.MetaEdit{
			Title:  u.Detail.NameEditor.Text(),
			Author: u.Detail.AuthorEditor.Text(),
			Genre:  strings.Join(u.commitDraftTags(), ", "),
			Source: u.Detail.SourceEditor.Text(),
		}
		id := u.Detail.Id
		u.DialogBusy = true
		u.closePicker()
		go func() {
			detail, err := u.Backend.UpdateComicMetadata(id, edit)
			u.Post(DetailSavedEvent{Id: id, Detail: detail, Err: err})
		}()
	}

	options := u.authorOptions()
	if u.Detail.PickerKind == "tag" {
		options = u.tagOptions()
	}
	if len(u.Detail.AuthorClicks) != len(options) {
		u.Detail.AuthorClicks = make([]widget.Clickable, len(options))
	}
	selected := false
	for i, value := range options {
		if u.Detail.AuthorClicks[i].Clicked(gtx) {
			if u.Detail.PickerKind == "tag" {
				u.addDraftTags(value)
			} else {
				u.Detail.AuthorEditor.SetText(value)
			}
			u.closePicker()
			u.Invalidate()
			selected = true
			break
		}
	}
	if !selected && u.Detail.AuthorOpen && u.Detail.AuthorClose.Clicked(gtx) {
		u.closePicker()
		u.Invalidate()
	}
	if u.Detail.AuthorMenu.Clicked(gtx) {
		if wasOpen && openedKind == "author" {
			u.closePicker()
		} else {
			u.openPicker("author", gtx)
		}
	}
	if u.Detail.TagMenu.Clicked(gtx) {
		if wasOpen && openedKind == "tag" {
			u.closePicker()
		} else {
			u.openPicker("tag", gtx)
		}
	}

	paint.Fill(gtx.Ops, colBlack50)
	u.BlockPointer(gtx, &u.Detail.ModalShield)
	var authorBottom, tagBottom int
	modalGtx := gtx
	modalGtx.Constraints.Min = image.Point{}
	macro := op.Record(gtx.Ops)
	modalDims := u.Modal(modalGtx, unit.Dp(560), func(gtx layout.Context) layout.Dimensions {
		return u.layoutMetadataForm(gtx, &authorBottom, &tagBottom)
	})
	modal := macro.Stop()
	x := (gtx.Constraints.Max.X - modalDims.Size.X) / 2
	y := (gtx.Constraints.Max.Y - modalDims.Size.Y) / 2
	modalOffset := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	modal.Add(gtx.Ops)
	modalOffset.Pop()
	contentTop := y + gtx.Dp(unit.Dp(metadataFormTopOffset))
	u.Detail.PickerRight = (gtx.Constraints.Max.X + gtx.Dp(unit.Dp(metadataContentWidth))) / 2
	switch u.Detail.PickerKind {
	case "author":
		u.Detail.PickerY = contentTop + authorBottom
	case "tag":
		u.Detail.PickerY = contentTop + tagBottom
		u.Detail.PickerRight -= u.Detail.TagAddWidth + gtx.Dp(unit.Dp(6))
	}
	if u.Detail.AuthorOpen {
		u.layoutPickerBackdrop(gtx, image.Rect(x, y, x+modalDims.Size.X, y+modalDims.Size.Y))
		u.layoutAuthorMenu(gtx, options)
	}
}

func (u *Renderer) layoutMetadataForm(gtx layout.Context, authorBottom, tagBottom *int) layout.Dimensions {
	var cursor int
	add := func(content layout.Widget, capture func(int)) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := content(gtx)
			cursor += dims.Size.Y
			if capture != nil {
				capture(cursor)
			}
			cursor += gtx.Dp(unit.Dp(7))
			return dims
		})
	}
	children := []layout.FlexChild{
		add(func(gtx layout.Context) layout.Dimensions {
			return u.LabelColor(gtx, "Edit metadata", unit.Sp(19), colText, 1, font.Bold)
		}, nil),
	}
	field := func(text, hint string, editor *widget.Editor) {
		children = append(children,
			add(func(gtx layout.Context) layout.Dimensions {
				return u.Label(gtx, text, unit.Sp(11), colDim, 1)
			}, nil),
			add(func(gtx layout.Context) layout.Dimensions {
				return u.Editor(gtx, editor, hint, 36)
			}, nil),
		)
	}
	field("Title", "", &u.Detail.NameEditor)
	children = append(children,
		add(func(gtx layout.Context) layout.Dimensions {
			return u.LabelColor(gtx, "AUTHOR - SINGLE SELECT", unit.Sp(11), colAccent, 1, font.Normal)
		}, nil),
		add(func(gtx layout.Context) layout.Dimensions {
			return u.layoutAuthorSelector(gtx)
		}, func(bottom int) { *authorBottom = bottom }),
		add(func(gtx layout.Context) layout.Dimensions {
			return u.Label(gtx, "TAGS", unit.Sp(11), colDim, 1)
		}, nil),
		add(func(gtx layout.Context) layout.Dimensions {
			return u.layoutTagsSelector(gtx)
		}, func(bottom int) { *tagBottom = bottom }),
	)
	field("Source", "", &u.Detail.SourceEditor)
	if u.Detail.SaveErr != "" {
		children = append(children, add(func(gtx layout.Context) layout.Dimensions {
			return u.Label(gtx, u.Detail.SaveErr, unit.Sp(12), colFav, 2)
		}, nil))
	}
	children = append(children, add(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Gap: 8}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.Button(gtx, &u.Detail.Cancel, "Cancel", ButtonStyle{Disabled: u.DialogBusy})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := "Save"
				if u.DialogBusy {
					label = "Saving..."
				}
				return u.Button(gtx, &u.Detail.Save, label, ButtonStyle{Solid: true, Disabled: u.DialogBusy})
			}),
		)
	}, nil))
	return layout.Flex{Axis: layout.Vertical, Gap: 7}.Layout(gtx, children...)
}

func (u *Renderer) layoutTagsSelector(gtx layout.Context) layout.Dimensions {
	if len(u.Detail.TagRemove) != len(u.Detail.Tags) {
		u.Detail.TagRemove = make([]widget.Clickable, len(u.Detail.Tags))
	}
	for i := range u.Detail.TagRemove {
		if u.Detail.TagRemove[i].Clicked(gtx) {
			u.removeDraftTag(i)
			u.Invalidate()
			return layout.Dimensions{}
		}
	}
	children := []layout.FlexChild{}
	if len(u.Detail.Tags) > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return outlay.FlowWrap{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, len(u.Detail.Tags), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(5), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.layoutSearchTag(gtx, &u.Detail.TagRemove[i], u.Detail.Tags[i])
				})
			})
		}))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Gap: 6, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return u.Editor(gtx, &u.Detail.TagsEditor, "Add a tag...", 36)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.valueMenuButton(gtx, &u.Detail.TagMenu, u.Detail.PickerKind == "tag")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := u.Button(gtx, &u.Detail.TagAdd, "Add", ButtonStyle{Disabled: strings.TrimSpace(u.Detail.TagsEditor.Text()) == ""})
				u.Detail.TagAddWidth = dims.Size.X
				return dims
			}),
		)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: 7}.Layout(gtx, children...)
}
