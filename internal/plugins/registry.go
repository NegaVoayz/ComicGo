package plugins

import (
	"comicgo/internal/plugins/builtin/bylastread"
	"comicgo/internal/plugins/builtin/bymodtime"
	"comicgo/internal/plugins/builtin/bytitle"
	"comicgo/internal/plugins/builtin/mark"
	"comicgo/internal/plugins/builtin/stats"
	// ---- add a plugin by importing its package above and appending it to the
	//      list below. That is the entire wiring step. ----
)

// FilterRules are all registered selection rules, shown in the UI.
var FilterRules = []FilterRule{
	mark.Favorites{},
	mark.Unread{},
	mark.InProgress{},
}

// SortRules are all registered ordering rules, shown in the UI.
var SortRules = []SortRule{
	bytitle.Asc{},
	bytitle.Desc{},
	bymodtime.Newest{},
	bylastread.Recent{},
}

// Cards are all registered plugin cards available to layout.json.
var Cards = []Card{
	stats.Card{},
}

// FilterByID finds a registered filter rule, or nil.
func FilterByID(id string) FilterRule {
	for _, r := range FilterRules {
		if r.ID() == id {
			return r
		}
	}
	return nil
}

// SortByID finds a registered sort rule, or nil.
func SortByID(id string) SortRule {
	for _, r := range SortRules {
		if r.ID() == id {
			return r
		}
	}
	return nil
}

// CardByID finds a registered card, or nil.
func CardByID(id string) Card {
	for _, c := range Cards {
		if c.ID() == id {
			return c
		}
	}
	return nil
}
