package application

import (
	"path/filepath"
	"runtime"
	"testing"

	"comicgo/internal/model"
)

func TestImportDestinationMustBeInsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	a := &App{settings: model.Settings{LibraryRoots: []string{root}}}
	if _, err := a.ImportSources([]string{filepath.Join(root, "book.cbz")}, outside); err == nil {
		t.Fatal("ImportSources accepted a destination outside library roots")
	}
}

func TestSamePathUsesPlatformIdentity(t *testing.T) {
	root := t.TempDir()
	if !samePath(root, filepath.Join(root, ".")) {
		t.Fatal("same directory with a normalized suffix was not recognized")
	}
	if samePath(root, filepath.Join(root, "..")) {
		t.Fatal("different directories were treated as identical")
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		if samePath(filepath.Join(root, "Comics"), filepath.Join(root, "comics")) {
			t.Fatal("case-distinct Unix paths were treated as identical")
		}
	}
}
