package meta

import (
	"sort"
	"strings"
)

// Build renders a key/value map back into comic-comment text: one "Key=Value"
// line per entry, in a deterministic order. It is the inverse of Parse — the
// well-known display keys come first (Title, Author, Genre, Source) and every
// remaining key follows in sorted order. Empty-valued keys are omitted (they
// carry no meaning), and any newline inside a value is collapsed to a space so
// a comment stays a single line per key. An empty map renders to "".
func Build(kv map[string]string) string {
	if len(kv) == 0 {
		return ""
	}
	var lines []string
	emit := func(k, v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if strings.ContainsAny(v, "\r\n") {
			v = strings.Map(func(r rune) rune {
				if r == '\r' || r == '\n' {
					return ' '
				}
				return r
			}, v)
		}
		lines = append(lines, k+"="+v)
	}
	for _, k := range knownKeys {
		if v, ok := kv[k]; ok {
			emit(k, v)
		}
	}
	var rest []string
	for k := range kv {
		if !knownKeySet[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		emit(k, kv[k])
	}
	return strings.Join(lines, "\n")
}

// CleanTags normalizes a comma-separated tag list: it splits on the same
// separators Parse/Build readers expect (`,`, `、`, `，`), trims each tag,
// drops empties and case-insensitive duplicates, and re-joins with ", ". The
// importer's tag handling follows the same rules, so this keeps edited Genre
// comments canonical.
func CleanTags(s string) string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '、' || r == '，'
	}) {
		t := strings.TrimSpace(part)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return strings.Join(out, ", ")
}

// knownKeys is the display order Build emits these in; knownKeySet lets it find
// which remaining keys to sort.
var knownKeys = []string{"Title", "Author", "Genre", "Source"}

var knownKeySet = func() map[string]bool {
	m := make(map[string]bool, len(knownKeys))
	for _, k := range knownKeys {
		m[k] = true
	}
	return m
}()
