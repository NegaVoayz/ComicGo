// Package bymodtime sorts by the archive file modification time.
package bymodtime

import (
	"comicgo/internal/model"
)

// Newest puts recently modified archives first.
type Newest struct{}

func (Newest) ID() string    { return "bymodtime" }
func (Newest) Label() string { return "File date (newest)" }
func (Newest) Less(a, b model.Meta) bool {
	if a.UpdatedAt != b.UpdatedAt {
		return a.UpdatedAt > b.UpdatedAt
	}
	if a.Filename != b.Filename {
		return a.Filename < b.Filename
	}
	return a.ID < b.ID
}
