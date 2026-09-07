// Package meta parses a comic archive's comment, which stores metadata as
// simple "key=value" lines (pure UTF-8).
package meta

import (
	"bufio"
	"strings"
)

// Parse turns comment text into a key/value map. Lines are trimmed; blank
// lines and lines without a separator are skipped. A comment line that is
// malformed but carries a key (e.g. "Title") is preserved with an empty value
// only if it has a trailing '='. The first '=' on a line is the separator so
// values may themselves contain '='.
func Parse(comment string) map[string]string {
	out := make(map[string]string)
	if comment == "" {
		return out
	}
	sc := bufio.NewScanner(strings.NewReader(comment))
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		if k != "" {
			out[k] = v
		}
	}
	return out
}
