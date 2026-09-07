// Package config loads and saves the user-editable settings.json and
// layout.json under the OS user-config directory, creating sensible defaults
// on first run.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"comicgo/internal/model"
)

// AppName is used to scope the per-user directory.
const AppName = "comicgo"

// DefaultMediaTransport is used unless overridden. "http" serves covers/pages
// from the loopback media server; "blob" uses data: URLs (fallback).
const DefaultMediaTransport = "http"

// SchemaVersion is stamped into settings and compared at startup.
const SchemaVersion = 1

// Dir returns the per-user app directory, creating it if needed.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, AppName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Paths bundles the fixed file locations under the app dir.
type Paths struct {
	Root   string // app dir
	DB     string // sqlite file
	Thumbs string // on-disk cover thumbnail cache
	Temp   string // scratch
	Config string // settings.json
	Layout string // layout.json
}

// Resolve computes all paths (creating directories).
func Resolve() (*Paths, error) {
	root, err := Dir()
	if err != nil {
		return nil, err
	}
	p := &Paths{
		Root:   root,
		DB:     filepath.Join(root, "comicgo.db"),
		Thumbs: filepath.Join(root, "thumbs"),
		Temp:   filepath.Join(root, "tmp"),
		Config: filepath.Join(root, "settings.json"),
		Layout: filepath.Join(root, "layout.json"),
	}
	for _, d := range []string{p.Thumbs, p.Temp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// LoadSettings reads settings.json, defaulting missing files/fields.
func LoadSettings() (model.Settings, error) {
	s := DefaultSettings()
	p, err := Resolve()
	if err != nil {
		return s, err
	}
	b, err := os.ReadFile(p.Config)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		// Corrupt settings should not brick the app; fall back to defaults.
		return DefaultSettings(), nil
	}
	if s.SchemaVersion != SchemaVersion {
		// On version change, keep roots/transport but re-stamp version.
		s.SchemaVersion = SchemaVersion
	}
	return s, nil
}

// DefaultSettings returns the baseline settings.
func DefaultSettings() model.Settings {
	return model.Settings{
		MediaTransport: DefaultMediaTransport,
		SchemaVersion:  SchemaVersion,
	}
}

// SaveSettings writes settings.json atomically.
func SaveSettings(s model.Settings) error {
	s.SchemaVersion = SchemaVersion
	p, err := Resolve()
	if err != nil {
		return err
	}
	return writeJSONAtomic(p.Config, s)
}

// LoadLayout reads layout.json, returning the default layout when absent.
func LoadLayout() (model.LayoutDoc, error) {
	p, err := Resolve()
	if err != nil {
		return model.LayoutDoc{}, err
	}
	b, err := os.ReadFile(p.Layout)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultLayout(), nil
		}
		return model.LayoutDoc{}, err
	}
	var doc model.LayoutDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return DefaultLayout(), nil // tolerate hand-edit mistakes
	}
	if len(doc.Sections) == 0 {
		return DefaultLayout(), nil
	}
	return doc, nil
}

// DefaultLayout is a single "all comics" grid section sorted by title.
func DefaultLayout() model.LayoutDoc {
	return model.LayoutDoc{
		Version: 1,
		Sections: []model.LayoutSection{
			{ID: "library", Title: "Library", Kind: "grid", SortRuleID: "bytitle", PageSize: 200},
		},
	}
}

// SaveLayout writes layout.json atomically.
func SaveLayout(doc model.LayoutDoc) error {
	doc.Version = 1
	p, err := Resolve()
	if err != nil {
		return err
	}
	return writeJSONAtomic(p.Layout, doc)
}

func writeJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
