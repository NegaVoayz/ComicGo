// Package stats provides a small "library stats" card that demonstrates the
// plugin Card surface. Its dynamic counts are filled in by the app's CardData
// binding; only the static spec is declared here.
package stats

import "comicgo/internal/model"

// Card shows library totals.
type Card struct{}

func (Card) ID() string { return "stats" }
func (Card) Render() model.CardSpec {
	return model.CardSpec{Type: "stats"}
}

// Counts is the dynamic payload a stats card receives.
type Counts struct {
	Total      int `json:"total"`
	Favorites  int `json:"favorites"`
	Unread     int `json:"unread"`
	InProgress int `json:"inProgress"`
}
