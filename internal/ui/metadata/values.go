// Package metadata contains pure helpers for the comic metadata editor.
package metadata

import "strings"

func SplitTags(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '、' || r == '，'
	}) {
		part = strings.TrimSpace(part)
		key := strings.ToLower(part)
		if part != "" && !seen[key] {
			seen[key] = true
			out = append(out, part)
		}
	}
	return out
}

func AuthorOptions(values []string, current, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	options := make([]string, 0, len(values)+1)
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] || (query != "" && !strings.Contains(key, query)) {
			return
		}
		seen[key] = true
		options = append(options, value)
	}
	add(current)
	for _, value := range values {
		add(value)
	}
	return options
}

func TagOptions(values, selected []string, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	chosen := make(map[string]bool, len(selected))
	for _, tag := range selected {
		chosen[strings.ToLower(tag)] = true
	}
	options := make([]string, 0, len(values))
	for _, tag := range values {
		key := strings.ToLower(tag)
		if chosen[key] || (query != "" && !strings.Contains(key, query)) {
			continue
		}
		options = append(options, tag)
	}
	return options
}

func AddTags(existing []string, raw string) ([]string, bool) {
	tags := append([]string(nil), existing...)
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		seen[strings.ToLower(tag)] = true
	}
	changed := false
	for _, tag := range SplitTags(raw) {
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		tags = append(tags, tag)
		changed = true
	}
	return tags, changed
}

func RemoveTag(tags []string, index int) []string {
	if index < 0 || index >= len(tags) {
		return tags
	}
	out := append([]string(nil), tags...)
	return append(out[:index], out[index+1:]...)
}
