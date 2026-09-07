// Package bytitle provides title (A-Z / Z-A) sort rules.
package bytitle

import (
	"strings"

	"comicgo/internal/model"
)

// Asc sorts by display title ascending (case-insensitive), falling back to
// filename and id so the order is total and deterministic.
type Asc struct{}

func (Asc) ID() string    { return "bytitle" }
func (Asc) Label() string { return "Title (A–Z)" }
func (Asc) Less(a, b model.Meta) bool {
	ta, tb := strings.ToLower(a.Title()), strings.ToLower(b.Title())
	if ta != tb {
		return ta < tb
	}
	if a.Filename != b.Filename {
		return a.Filename < b.Filename
	}
	return a.ID < b.ID
}

// Desc is the reverse of Asc.
type Desc struct{}

func (Desc) ID() string    { return "bytitle-desc" }
func (Desc) Label() string { return "Title (Z–A)" }
func (Desc) Less(a, b model.Meta) bool {
	return Asc{}.Less(b, a)
}
