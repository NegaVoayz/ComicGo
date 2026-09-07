// Package plugins defines the compile-time plugin contracts. A plugin is just
// a Go package that implements one of these interfaces and is imported once in
// registry.go — no scripting, no hot loading. Adding a rule = one package +
// one import line, then recompile.
package plugins

import "comicgo/internal/model"

// Rule is the common base for filter and sort rules.
type Rule interface {
	// ID is a stable machine identifier referenced from layout.json and by the
	// UI (e.g. "bytitle"). Never change it once shipped.
	ID() string
	// Label is the human-readable name shown in dropdowns (e.g. "Title (A-Z)").
	Label() string
}

// FilterRule decides whether a comic belongs in the current selection.
type FilterRule interface {
	Rule
	Keep(m model.Meta) bool
}

// SortRule orders a selection. Less must implement a strict weak ordering
// (the same contract as sort.Slice) so pagination is deterministic.
type SortRule interface {
	Rule
	Less(a, b model.Meta) bool
}

// Card declares a plugin UI card that can be placed into a layout section.
// Cards only declare *static* spec here; dynamic payloads (e.g. live counts)
// are supplied by the app's CardData binding keyed by the same ID.
type Card interface {
	ID() string
	Render() model.CardSpec
}
