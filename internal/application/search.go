package application

import (
	"strings"

	"comicgo/internal/model"
)

// ListAuthors returns the distinct authors currently present in the index.
func (a *App) ListAuthors() ([]string, error) {
	if a.store == nil {
		return nil, nil
	}
	return a.store.AuthorValues()
}

// ListTags returns every distinct Genre tag currently present in the index.
func (a *App) ListTags() ([]string, error) {
	if a.store == nil {
		return nil, nil
	}
	return a.store.TagValues()
}

type searchQuery struct {
	text    []string // lower-cased substrings, all must occur somewhere
	tags    []string // whole-tag matches (case-insensitive), all must be present
	authors []string // exact author matches (case-insensitive), all must be present
}

// parseSearch turns a search box string into structured terms. Plain words are
// text substrings (case-insensitive, anywhere in title/file/metadata); a term
// of the form tag:name matches a whole tag exactly (ロリ does not match ロリコン).
// tag:"a b" quotes a tag that contains spaces. Double quotes group any text; a
// backslash escapes the next character inside a quoted group.
func parseSearch(s string) searchQuery {
	var q searchQuery
	for _, tok := range splitSearchTerms(s) {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if v, ok := strings.CutPrefix(tok, "tag:"); ok {
			v = strings.TrimSpace(v)
			if v != "" {
				q.tags = append(q.tags, v)
			}
		} else if v, ok := strings.CutPrefix(tok, "author:"); ok {
			v = strings.TrimSpace(v)
			if v != "" {
				q.authors = append(q.authors, v)
			}
		} else {
			q.text = append(q.text, strings.ToLower(tok))
		}
	}
	return q
}

// splitSearchTerms splits s on whitespace but keeps double-quoted groups whole,
// so tag:"a b" stays a single term. Quote characters are dropped; a backslash
// escapes the next character inside a quoted group.
func splitSearchTerms(s string) []string {
	var terms []string
	var b strings.Builder
	inQ, esc := false, false
	flush := func() {
		if b.Len() > 0 {
			terms = append(terms, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		if esc {
			b.WriteRune(r)
			esc = false
			continue
		}
		switch r {
		case '\\':
			if inQ {
				esc = true
			} else {
				b.WriteRune(r)
			}
		case '"':
			inQ = !inQ
		case ' ', '\t', '\r', '\n':
			if inQ {
				b.WriteRune(r)
			} else {
				flush()
			}
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return terms
}

// selection returns the comics matching an optional search query and filter
// rule. An empty query selects everything (still honouring the filter).
func (a *App) selection(search, filterRuleID string) []model.Meta {
	list := a.filtered(filterRuleID)
	q := parseSearch(search)
	if len(q.text) == 0 && len(q.tags) == 0 && len(q.authors) == 0 {
		return list
	}
	out := make([]model.Meta, 0, len(list))
	for _, m := range list {
		if searchQueryMatch(m, q) {
			out = append(out, m)
		}
	}
	return out
}

func searchQueryMatch(m model.Meta, q searchQuery) bool {
	for _, needle := range q.text {
		if !textFieldMatch(m, needle) {
			return false
		}
	}
	for _, tag := range q.tags {
		if !metaHasTag(m, tag) {
			return false
		}
	}
	for _, author := range q.authors {
		if !strings.EqualFold(strings.TrimSpace(m.KV["Author"]), author) {
			return false
		}
	}
	return true
}

// textFieldMatch reports whether needle occurs (case-insensitively) in the
// title, file name, or any metadata value. Metadata field names are not user
// content and must not make broad terms such as "a" match every comic.
func textFieldMatch(m model.Meta, needle string) bool {
	if strings.Contains(strings.ToLower(m.Title()), needle) ||
		strings.Contains(strings.ToLower(m.Filename), needle) {
		return true
	}
	for _, v := range m.KV {
		if strings.Contains(strings.ToLower(v), needle) {
			return true
		}
	}
	return false
}

// metaHasTag reports whether the comic carries tag as one of its Genre comment
// tags, compared exactly and case-insensitively. Genre is the list the UI shows
// as tag chips (Author is a separate key, not a searchable tag).
func metaHasTag(m model.Meta, tag string) bool {
	for _, part := range strings.Split(m.KV["Genre"], ",") {
		if strings.EqualFold(strings.TrimSpace(part), tag) {
			return true
		}
	}
	return false
}

// cardTagLimit bounds how many comment tags travel with each shelf card, so a
// big tagged library does not bloat every page payload.
const cardTagLimit = 6

// cardTags returns the comic's Genre comment tags (the content labels we
// imported), trimmed and de-duplicated, for the card caption. The author is
// surfaced separately on its own caption line, not as one of these chips.
func cardTags(m model.Meta) []string {
	genre := strings.TrimSpace(m.KV["Genre"])
	if genre == "" {
		return nil
	}
	seen := make(map[string]bool, cardTagLimit)
	out := make([]string, 0, cardTagLimit)
	for _, part := range strings.Split(genre, ",") {
		t := strings.TrimSpace(part)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) == cardTagLimit {
			break
		}
	}
	return out
}

// GetComics returns one paged slice of cards matching the query.
