// Package mark provides selection filters based on reading/favorite marks.
package mark

import (
	"comicgo/internal/model"
)

// Favorites keeps favorited comics.
type Favorites struct{}

func (Favorites) ID() string             { return "favorites" }
func (Favorites) Label() string          { return "Favorites" }
func (Favorites) Keep(m model.Meta) bool { return m.Fav }

// Unread keeps comics that have never been opened.
type Unread struct{}

func (Unread) ID() string             { return "unread" }
func (Unread) Label() string          { return "Unread" }
func (Unread) Keep(m model.Meta) bool { return m.LastReadPage == 0 }

// InProgress keeps comics started but not finished.
type InProgress struct{}

func (InProgress) ID() string { return "inprogress" }
func (InProgress) Label() string {
	return "In progress"
}
func (InProgress) Keep(m model.Meta) bool {
	return m.LastReadPage > 0 && m.LastReadPage+1 < m.PageCount
}
