package screens

import (
	"comicgo/internal/model"
	. "comicgo/internal/ui"
	"errors"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
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

func (u *Renderer) authorOptions() []string {
	options := make([]string, 0, len(u.Detail.AuthorValues)+1)
	seen := map[string]bool{}
	add := func(author string) {
		author = strings.TrimSpace(author)
		key := strings.ToLower(author)
		if author == "" || seen[key] {
			return
		}
		seen[key] = true
		options = append(options, author)
	}
	add(u.Detail.AuthorEditor.Text())
	for _, author := range u.Detail.AuthorValues {
		add(author)
	}
	return options
}

func (u *Renderer) tagOptions() []string {
	selected := make(map[string]bool, len(u.Detail.Tags))
	for _, tag := range u.Detail.Tags {
		selected[strings.ToLower(tag)] = true
	}
	options := make([]string, 0, len(u.Detail.TagValues))
	for _, tag := range u.Detail.TagValues {
		if !selected[strings.ToLower(tag)] {
			options = append(options, tag)
		}
	}
	return options
}

func (u *Renderer) addDraftTags(raw string) bool {
	changed := false
	seen := make(map[string]bool, len(u.Detail.Tags))
	for _, tag := range u.Detail.Tags {
		seen[strings.ToLower(tag)] = true
	}
	for _, tag := range SplitTags(raw) {
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		u.Detail.Tags = append(u.Detail.Tags, tag)
		changed = true
	}
	return changed
}

func (u *Renderer) removeDraftTag(index int) {
	if index < 0 || index >= len(u.Detail.Tags) {
		return
	}
	u.Detail.Tags = append(u.Detail.Tags[:index], u.Detail.Tags[index+1:]...)
}

func (u *Renderer) commitDraftTags() []string {
	tags := append([]string(nil), u.Detail.Tags...)
	u.Detail.Tags = tags
	u.addDraftTags(u.Detail.TagsEditor.Text())
	u.Detail.TagsEditor.SetText("")
	return u.Detail.Tags
}

func (u *Renderer) layoutMetadataEditor(gtx layout.Context) {
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
		u.Detail.PickerKind = "author"
		u.Detail.AuthorOpen = true
		u.Detail.AuthorCenter = u.pressedControlCenter(&u.Detail.AuthorMenu, gtx.Dp(unit.Dp(38)))
		u.Detail.AuthorAnchor = u.Press.Position
		u.Invalidate()
	}
	if u.Detail.TagMenu.Clicked(gtx) {
		u.Detail.PickerKind = "tag"
		u.Detail.AuthorOpen = true
		u.Detail.AuthorCenter = u.pressedControlCenter(&u.Detail.TagMenu, gtx.Dp(unit.Dp(38)))
		u.Detail.AuthorAnchor = u.Press.Position
		u.Invalidate()
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.LabelColor(gtx, "Edit metadata", unit.Sp(19), colText, 1, font.Bold)
		}),
	}
	children = append(children, u.metadataField("Title", "", &u.Detail.NameEditor)...)
	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.LabelColor(gtx, "AUTHOR - SINGLE SELECT", unit.Sp(11), colAccent, 1, font.Normal)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.layoutAuthorSelector(gtx)
		}),
	)
	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.Label(gtx, "TAGS", unit.Sp(11), colDim, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.layoutTagsSelector(gtx)
		}),
	)
	children = append(children, u.metadataField("Source", "", &u.Detail.SourceEditor)...)
	if u.Detail.SaveErr != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.Label(gtx, u.Detail.SaveErr, unit.Sp(12), colFav, 2)
		}))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
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
	}))

	layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.Fill(gtx.Ops, colBlack50)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.Modal(gtx, unit.Dp(560), func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: 7}.Layout(gtx, children...)
				})
			})
		}),
	)
	if u.Detail.AuthorOpen {
		u.layoutAuthorMenu(gtx, options)
	}
}

func (u *Renderer) layoutAuthorSelector(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Gap: 6, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.Editor(gtx, &u.Detail.AuthorEditor, "Choose or type one author", 36)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.valueMenuButton(gtx, &u.Detail.AuthorMenu, u.Detail.PickerKind == "author")
		}),
	)
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
				return u.Button(gtx, &u.Detail.TagAdd, "Add", ButtonStyle{Disabled: strings.TrimSpace(u.Detail.TagsEditor.Text()) == ""})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.valueMenuButton(gtx, &u.Detail.TagMenu, u.Detail.PickerKind == "tag")
			}),
		)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: 7}.Layout(gtx, children...)
}
