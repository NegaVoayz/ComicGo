package metadata

import (
	"slices"
	"strings"
	"testing"
)

func TestAuthorOptionsFilterAndDeduplicateTypedValue(t *testing.T) {
	got := AuthorOptions([]string{"Alpha", "alpha", "Current", "Beta"}, " current ", " current ")
	if !slices.Equal(got, []string{"current"}) {
		t.Fatalf("author options = %q, want current", got)
	}
}

func TestTagOptionsFilterTypedValueAndExcludeSelected(t *testing.T) {
	got := TagOptions([]string{"Alpha", "Beta", "Better", "Comedy"}, []string{"Beta"}, "be")
	if strings.Join(got, "|") != "Better" {
		t.Fatalf("tag options = %q, want Better", got)
	}
}
