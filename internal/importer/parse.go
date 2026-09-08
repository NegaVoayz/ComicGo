package importer

import "strings"

// This file parses the bracketed filename convention used by comic downloaders:
//
//	[AUTHOR...] TITLE (TAG...)
//
// where the group delimiters may be square brackets, braces or parentheses —
// all three are treated the same. Example real name:
//
//	[ぐすたふ] 蕾佳是華麗的 我的俏女傭 レイカは華麗な僕のメイド (中文)
//
// Every group that appears before the title text contributes to the author;
// every group after the title text is one tag; the text outside the groups is
// the title. Groups may repeat on either side and need not be separated by
// anything ([A][B] Title {t1} (t2)).

const groupOpeners = "[{("

func groupCloser(open byte) byte {
	switch open {
	case '[':
		return ']'
	case '{':
		return '}'
	default: // '('
		return ')'
	}
}

// ParseName splits a file/folder name into its title, author and tags using
// the bracket convention. base is the source stem (no archive extension).
//
// A name only counts as bracketed when it BEGINS with an author group (after
// leading whitespace). Plain titles that merely contain parentheses — e.g.
// "My Comic (Vol 3)" — return empty metadata so the caller keeps the whole
// name as the title and writes no comment.
func ParseName(base string) (title, author string, tags []string) {
	if trimmed := strings.TrimSpace(base); trimmed == "" || strings.IndexByte(groupOpeners, trimmed[0]) < 0 {
		return "", "", nil
	}

	var authors, tagGroups []string
	var titleParts []string
	started := false

	for _, tok := range groupTokens(base) {
		if tok.group {
			if started {
				tagGroups = append(tagGroups, tok.s)
			} else {
				authors = append(authors, tok.s)
			}
			continue
		}
		if strings.TrimSpace(tok.s) != "" {
			started = true
		}
		if started {
			titleParts = append(titleParts, tok.s)
		}
	}

	if len(authors) > 0 {
		author = strings.Join(authors, " ")
	}
	title = strings.TrimSpace(strings.Join(titleParts, " "))
	title = strings.Join(strings.Fields(title), " ") // collapse any whitespace runs
	for _, g := range tagGroups {
		tags = append(tags, splitTagList(g)...)
	}
	return title, author, tags
}

// BuildComment renders parsed metadata as a comic-archive comment in the same
// "Key=Value" line format internal/meta.Parse reads (the keys the library
// already shows as author, tags and searchable title). Returns "" when nothing
// was parsed.
func BuildComment(title, author string, tags []string) string {
	var lines []string
	if t := strings.TrimSpace(title); t != "" {
		lines = append(lines, "Title="+t)
	}
	if a := strings.TrimSpace(author); a != "" {
		lines = append(lines, "Author="+a)
	}
	seen := map[string]bool{}
	var cleaned []string
	for _, g := range tags {
		for _, t := range splitTagList(g) {
			key := strings.ToLower(t)
			if t == "" || seen[key] {
				continue
			}
			seen[key] = true
			cleaned = append(cleaned, t)
		}
	}
	if len(cleaned) > 0 {
		lines = append(lines, "Genre="+strings.Join(cleaned, ", "))
	}
	return strings.Join(lines, "\n")
}

// splitTagList splits one group's content on the list separators commas
// (half- and full-width) that tag lists commonly use, trimming empties.
func splitTagList(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '、' || r == '，'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type nameTok struct {
	group bool
	s     string
}

// groupTokens splits a name into alternating {group} and plain-text runs,
// recognizing square brackets, braces and parentheses as group delimiters. The
// content of a group is trimmed; whitespace-only plain runs and empty group
// bodies are dropped (they only glue groups together).
func groupTokens(base string) []nameTok {
	var toks []nameTok
	i := 0
	for i < len(base) {
		pos := -1
		for j := i; j < len(base); j++ {
			if strings.IndexByte(groupOpeners, base[j]) >= 0 {
				pos = j
				break
			}
		}
		if pos < 0 {
			appendNameText(&toks, base[i:])
			break
		}
		appendNameText(&toks, base[i:pos])
		cc := groupCloser(base[pos])
		end := -1
		depth := 0
		for j := pos; j < len(base); j++ {
			if base[j] == base[pos] {
				depth++
			} else if base[j] == cc {
				depth--
				if depth == 0 {
					end = j
					break
				}
			}
		}
		if end < 0 {
			// unmatched opener: keep the rest as plain text so nothing is lost
			appendNameText(&toks, base[pos:])
			break
		}
		if content := strings.TrimSpace(base[pos+1 : end]); content != "" {
			toks = append(toks, nameTok{group: true, s: content})
		}
		i = end + 1
	}
	return toks
}

func appendNameText(toks *[]nameTok, s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	*toks = append(*toks, nameTok{s: s})
}
