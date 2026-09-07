// Package bylastread sorts by reading recency (most recently opened first).
package bylastread

import (
	"comicgo/internal/model"
)

// Recent puts most recently read comics first; never-read comics sort last
// (by title) so they do not interleave among read history.
type Recent struct{}

func (Recent) ID() string    { return "bylastread" }
func (Recent) Label() string { return "Recently read" }
func (Recent) Less(a, b model.Meta) bool {
	ra, rb := a.ReadAt, b.ReadAt
	switch {
	case ra == 0 && rb == 0:
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		return a.ID < b.ID
	case ra == 0:
		return false // a is unread, b is read -> a not less
	case rb == 0:
		return true // b is unread -> a (read) less
	default:
		if ra != rb {
			return ra > rb
		}
		return a.ID < b.ID
	}
}
