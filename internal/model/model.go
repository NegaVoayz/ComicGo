// Package model defines the plain, JSON-serializable types that cross the
// Go <-> webview boundary and that plugins operate on. It must not import
// any other internal package so it can be a shared leaf dependency.
package model

import "encoding/json"

// Meta is the stable, read-only view of a single comic that plugin rules
// (filter/sort) see. Plugins never touch the database directly.
type Meta struct {
	ID           int64             `json:"id"`
	Path         string            `json:"path"`
	Filename     string            `json:"filename"`
	Dir          string            `json:"dir"`
	PageCount    int               `json:"pageCount"`
	FileSize     int64             `json:"fileSize"`
	AddedAt      int64             `json:"addedAt"`   // unix millis
	UpdatedAt    int64             `json:"updatedAt"` // unix millis (file mtime)
	LastReadPage int               `json:"lastReadPage"`
	ReadAt       int64             `json:"readAt"` // unix ms of last read, 0 if never
	Fav          bool              `json:"fav"`
	HasNonUTF8   bool              `json:"hasNonUtf8"`
	KV           map[string]string `json:"kv"` // parsed zip-comment "key=value" lines
}

// Val returns the value for a comment key, or "" if absent.
func (m Meta) Val(key string) string { return m.KV[key] }

// Title is the preferred display name: the "Title" comment key when present,
// otherwise the zip filename.
func (m Meta) Title() string {
	if t := m.KV["Title"]; t != "" {
		return t
	}
	return m.Filename
}

// ComicCard is a lightweight shelf row (no image bytes, no meta blob).
type ComicCard struct {
	ID         int64  `json:"id"`
	Filename   string `json:"filename"`
	Title      string `json:"title"`
	PageCount  int    `json:"pageCount"`
	Fav        bool   `json:"fav"`
	LastPage   int    `json:"lastPage"`
	HasNonUTF8 bool   `json:"hasNonUtf8"`
	CoverURL   string `json:"coverUrl"` // filled by the transport layer
}

// ComicDetail is the full view used by the info panel / open action.
type ComicDetail struct {
	Meta        Meta     `json:"meta"`
	CommentRaw  string   `json:"commentRaw"`
	AllMetaKeys []string `json:"allMetaKeys"`
}

// PageQuery describes one paged slice of the (filtered, sorted) shelf.
type PageQuery struct {
	Offset       int    `json:"offset"`
	Limit        int    `json:"limit"`
	FilterRuleID string `json:"filterRuleId"` // "" = no filter
	SortRuleID   string `json:"sortRuleId"`   // "" = default ordering (by title)
}

// RuleInfo describes a registered plugin rule for the UI dropdowns.
type RuleInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // "filter" | "sort"
}

// CoverRef is the transport-independent handle to a cover image.
type CoverRef struct {
	URL string `json:"url"` // http://.../cover/{id} or a data: URL
}

// PageRef is the transport-independent handle to a single reader page.
type PageRef struct {
	URL string `json:"url"`
}

// OpenResult is returned when a comic is opened for reading.
type OpenResult struct {
	ID        int64 `json:"id"`
	PageCount int   `json:"pageCount"`
}

// CardSpec describes one rendered plugin card.
type CardSpec struct {
	// Type is the frontend component key registered in pluginRegistry.ts.
	Type string `json:"type"`
	// Data is arbitrary JSON the plugin-authored Svelte component consumes.
	Data json.RawMessage `json:"data,omitempty"`
}

// Progress mirrors the reading_progress row.
type Progress struct {
	ComicID   int64 `json:"comicId"`
	LastPage  int   `json:"lastPage"`
	PageCount int   `json:"pageCount"`
}

// Settings is persisted in settings.json.
type Settings struct {
	LibraryRoots   []string `json:"libraryRoots"`
	MediaTransport string   `json:"mediaTransport"` // "http" | "blob"
	SchemaVersion  int      `json:"schemaVersion"`
}

// LayoutSection is one composable region of the shelf screen. The layout is a
// fixed app shell whose arrangement comes from layout.json (user-editable).
type LayoutSection struct {
	ID     string `json:"id"`     // stable id referenced by bindings
	Title  string `json:"title"`  // header shown above the section
	Kind   string `json:"kind"`   // "grid" | "card"
	CardID string `json:"cardId"` // Kind=="card": plugin card id, e.g. "stats"
	// grid tuning:
	FilterRuleID string `json:"filterRuleId,omitempty"`
	SortRuleID   string `json:"sortRuleId,omitempty"`
	PageSize     int    `json:"pageSize,omitempty"`
}

// LayoutDoc is the persisted, user-editable shelf layout.
type LayoutDoc struct {
	Version  int             `json:"version"`
	Sections []LayoutSection `json:"sections"`
}

// ScanProgress is emitted as an event during scanning.
type ScanProgress struct {
	Done  int    `json:"done"`
	Total int    `json:"total"`
	Path  string `json:"path"`
}

// ScanJob is the handle for a (possibly running) scan.
type ScanJob struct {
	ID     string `json:"id"`
	Status string `json:"status"` // running | done | cancelled | failed
	Done   int    `json:"done"`
	Total  int    `json:"total"`
}
