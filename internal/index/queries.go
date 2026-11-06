// Package index owns the SQLite database: schema, migrations, and all query
// access. It uses modernc.org/sqlite (pure Go) with WAL and a busy timeout so
// concurrent reader connections and a single writer coexist safely.
package index

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func (s *Store) SetProgress(comicID int64, lastPage, pageCount int) error {
	_, err := s.db.ExecContext(context.Background(), `
INSERT INTO reading_progress(comic_id, last_page, page_count, updated_at)
VALUES(?,?,?,?)
ON CONFLICT(comic_id) DO UPDATE SET last_page=excluded.last_page,
  page_count=excluded.page_count, updated_at=excluded.updated_at`,
		comicID, lastPage, pageCount, time.Now().UnixMilli())
	return err
}

// IsFavorite reports whether a comic is favorited.
func (s *Store) IsFavorite(comicID int64) (bool, error) {
	var one int
	err := s.db.QueryRowContext(context.Background(),
		`SELECT 1 FROM favorites WHERE comic_id=?`, comicID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// SetFavorite marks (on=true) or clears a favorite.
func (s *Store) SetFavorite(comicID int64, on bool) error {
	ctx := context.Background()
	if on {
		_, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO favorites(comic_id, added_at) VALUES(?,?)`,
			comicID, time.Now().UnixMilli())
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM favorites WHERE comic_id=?`, comicID)
	return err
}

// IDFromPath resolves an indexed comic id from its path.
func (s *Store) IDFromPath(path string) (int64, bool, error) {
	var id int64
	err := s.db.QueryRowContext(context.Background(),
		`SELECT id FROM comics WHERE path=?`, path).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// PathFromID resolves a comic id to its archive path.
func (s *Store) PathFromID(id int64) (string, error) {
	var p string
	err := s.db.QueryRowContext(context.Background(),
		`SELECT path FROM comics WHERE id=?`, id).Scan(&p)
	return p, err
}

// RowByID loads one row including KV.
func (s *Store) RowByID(id int64) (*Row, error) {
	rows, err := s.AllRows()
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// IsWithin reports whether path is under dir (used to prune removed files).
func IsWithin(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}
