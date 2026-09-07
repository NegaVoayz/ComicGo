package model

import "testing"

func TestMetaTitleStripsArchiveExt(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"深海ニテ沈ム.zip", "深海ニテ沈ム"},
		{"Some Comic.CBZ", "Some Comic"},
		{"a.b.c.zip", "a.b.c"},
		{"noext", "noext"},
		{"nested/Comic.cbz", "Comic"},
	}
	for _, c := range cases {
		m := Meta{Filename: c.in, KV: map[string]string{}}
		if got := m.Title(); got != c.want {
			t.Errorf("Title(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMetaTitlePrefersCommentKey(t *testing.T) {
	m := Meta{Filename: "x.cbz", KV: map[string]string{"Title": "Real Title"}}
	if got := m.Title(); got != "Real Title" {
		t.Errorf("Title = %q, want comment key", got)
	}
}
