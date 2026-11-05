package main

import (
	"image"
	"path/filepath"
	"testing"

	"comicgo/internal/model"
)

func TestLandscapeLeftCrop(t *testing.T) {
	if !landscapeLeftCrop(1600, 500) {
		t.Fatal("full landscape cover should crop from the left")
	}
	if landscapeLeftCrop(500, 700) {
		t.Fatal("portrait cover should stay centered")
	}
	if landscapeLeftCrop(500, 500) {
		t.Fatal("square cover should stay centered")
	}
}

func TestGridGeometry(t *testing.T) {
	cols, cardW, coverH, rowH := gridGeometry(1200, 150, 14, 14)
	if cols < 2 {
		t.Fatalf("cols = %d, want at least 2", cols)
	}
	if cardW < 80 || coverH != cardW*4/3 {
		t.Fatalf("card geometry = %d x %d", cardW, coverH)
	}
	if rowH != coverH+captionH+14 {
		t.Fatalf("row height = %d", rowH)
	}
}

func TestGridViewportHeightHugsShortLibrary(t *testing.T) {
	short := gridViewportHeight(700, 1200, 3, 150, 14, 14)
	_, _, _, rowH := gridGeometry(1200, 150, 14, 14)
	if short != rowH {
		t.Fatalf("short grid height = %d, want one padded row %d", short, rowH)
	}

	long := gridViewportHeight(700, 1200, 21, 150, 14, 14)
	if long != 700 {
		t.Fatalf("long grid height = %d, want viewport height 700", long)
	}
}

func TestVisibleRows(t *testing.T) {
	start, end := visibleRows(10, 4, 30, 3)
	if start != 7 || end != 17 {
		t.Fatalf("visibleRows = %d..%d, want 7..17", start, end)
	}
	start, end = visibleRows(0, 1, 0, 3)
	if start != 0 || end != -1 {
		t.Fatalf("empty visibleRows = %d..%d", start, end)
	}
}

func TestTagQueryEscaping(t *testing.T) {
	u := &appUI{
		search: `space  opera`,
		tags:   []string{`quote " slash \`},
	}
	got := u.query()
	want := `space  opera tag:"quote \" slash \\"`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestImportDestinationMustBeInsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	a := &App{settings: model.Settings{LibraryRoots: []string{root}}}
	if _, err := a.ImportSources([]string{filepath.Join(root, "book.cbz")}, outside); err == nil {
		t.Fatal("ImportSources accepted a destination outside library roots")
	}
}

func TestStatsCardRendererRegistered(t *testing.T) {
	if cardRenderers["stats"] == nil {
		t.Fatal("stats Gio renderer is not registered")
	}
}

func TestRuleMenusAreExclusiveAndToggleClosed(t *testing.T) {
	first := &sectionState{menuKind: "filter"}
	second := &sectionState{}
	u := &appUI{sections: []*sectionState{first, second}}

	u.toggleRuleMenu(second, "sort", image.Pt(100, 100))
	if first.menuKind != "" || second.menuKind != "sort" {
		t.Fatalf("open menus = %q, %q; want empty, sort", first.menuKind, second.menuKind)
	}
	u.toggleRuleMenu(second, "sort", image.Pt(100, 100))
	if second.menuKind != "" {
		t.Fatalf("sort menu remained open after toggle: %q", second.menuKind)
	}
}
