package meta

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildOrderAndContent(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]string
		want string
	}{
		{"empty", map[string]string{}, ""},
		{"only empty value dropped", map[string]string{"Title": "", "Author": "A"}, "Author=A"},
		{
			"known keys first, unknowns sorted after",
			map[string]string{"Genre": "x, y", "B": "2", "Title": "T", "A": "1", "Source": "z"},
			"Title=T\nGenre=x, y\nSource=z\nA=1\nB=2",
		},
		{"equals in value survives", map[string]string{"Title": "a=b"}, "Title=a=b"},
		{"newline in value collapses", map[string]string{"Author": "a\nb"}, "Author=a b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Build(c.in); got != c.want {
				t.Errorf("Build(%v)\n got  %q\n want %q", c.in, got, c.want)
			}
		})
	}
}

func TestBuildParseRoundTrip(t *testing.T) {
	// Values that are non-empty survive the round trip; empty-valued keys are
	// dropped by Build, so they are not expected back.
	kv := map[string]string{
		"Title":  "蕾佳是華麗的 僕のメイド",
		"Author": "ぐすたふ",
		"Genre":  "comedy, 中文",
		"Source": "magnet:?xt=urn:btih:abc&dn=x",
		"Custom": "kept=as=is",
	}
	got := Parse(Build(kv))
	if !reflect.DeepEqual(got, kv) {
		t.Errorf("Parse(Build(kv))\n got  %v\n want %v", got, kv)
	}
}

func TestCleanTags(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"foo", "foo"},
		{" foo , Foo, bar、x，x, , ", "foo, bar, x"}, // trim, case-insens dedupe, multi separators
		{"one, two", "one, two"},
	}
	for _, c := range cases {
		if got := CleanTags(c.in); got != c.want {
			t.Errorf("CleanTags(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := CleanTags("a,b"); !strings.Contains(got, ", ") {
		t.Errorf("CleanTags join separator wrong: %q", got)
	}
}
