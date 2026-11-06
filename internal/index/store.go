// Package index owns the SQLite database: schema, migrations, and all query
// access. It uses modernc.org/sqlite (pure Go) with WAL and a busy timeout so
// concurrent reader connections and a single writer coexist safely.
package index

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Row is a comic plus its parsed comment keys and reading state, ready to be
// converted into a model.Meta or served to the UI.
type Row struct {
	ID         int64
	Path       string
	Dir        string
	Filename   string
	FileSize   int64
	MtimeNs    int64
	PageCount  int
	CoverEntry string
	CommentRaw string
	HasNonUTF8 bool
	KV         map[string]string
	Fav        bool
	LastPage   int
	ReadAt     int64
}

// Record is what the scanner commits for one archive.
type Record struct {
	Path       string
	Dir        string
	Filename   string
	FileSize   int64
	MtimeNs    int64
	PageCount  int
	CoverEntry string
	CommentRaw string
	HasNonUTF8 bool
	KV         map[string]string
}

// Store wraps the SQLite handle.
type Store struct {
	db *sql.DB
}

const schemaVersion = 1

const schema = `
CREATE TABLE IF NOT EXISTS comics (
  id            INTEGER PRIMARY KEY,
  path          TEXT    NOT NULL UNIQUE,
  dir_path      TEXT    NOT NULL,
  filename      TEXT    NOT NULL,
  file_size     INTEGER NOT NULL,
  mtime_ns      INTEGER NOT NULL,
  page_count    INTEGER NOT NULL DEFAULT 0,
  cover_entry   TEXT,
  comment_raw   TEXT    NOT NULL DEFAULT '',
  has_nonutf8   INTEGER NOT NULL DEFAULT 0,
  added_at      INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_comics_dir ON comics(dir_path);

CREATE TABLE IF NOT EXISTS comic_meta (
  comic_id  INTEGER NOT NULL REFERENCES comics(id) ON DELETE CASCADE,
  key       TEXT    NOT NULL,
  value     TEXT    NOT NULL,
  PRIMARY KEY (comic_id, key)
);
CREATE INDEX IF NOT EXISTS idx_meta_kv ON comic_meta(key, value);

CREATE TABLE IF NOT EXISTS favorites (
  comic_id  INTEGER PRIMARY KEY REFERENCES comics(id) ON DELETE CASCADE,
  added_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS reading_progress (
  comic_id   INTEGER PRIMARY KEY REFERENCES comics(id) ON DELETE CASCADE,
  last_page  INTEGER NOT NULL DEFAULT 0,
  page_count INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS scan_state (
  path       TEXT PRIMARY KEY,
  state      TEXT NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS schema_meta (
  k TEXT PRIMARY KEY,
  v INTEGER NOT NULL
);`

// Open opens (creating) the database at path and runs migrations.
func Open(dbPath string) (*Store, error) {
	dsn := "file:" + dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc may open lazily; force a connection to surface DSN errors now.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return err
	}
	var v int
	row := tx.QueryRowContext(ctx, `SELECT v FROM schema_meta WHERE k='version'`)
	if err := row.Scan(&v); err == sql.ErrNoRows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_meta(k,v) VALUES('version', ?)`, schemaVersion); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if v > schemaVersion {
		return fmt.Errorf("database schema %d is newer than supported %d", v, schemaVersion)
	}
	return tx.Commit()
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// FindFingerprint returns the stored (size, mtimeNs) for path, if indexed.
func (s *Store) FindFingerprint(path string) (size, mtimeNs int64, found bool, err error) {
	row := s.db.QueryRowContext(context.Background(),
		`SELECT file_size, mtime_ns FROM comics WHERE path=?`, path)
	err = row.Scan(&size, &mtimeNs)
	if err == sql.ErrNoRows {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	return size, mtimeNs, true, nil
}

// CommitComic upserts a comic row and replaces its comment-metadata keys in
// one transaction.
func (s *Store) CommitComic(rec Record) (int64, error) {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
INSERT INTO comics(path, dir_path, filename, file_size, mtime_ns, page_count, cover_entry, comment_raw, has_nonutf8, added_at, updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(path) DO UPDATE SET
  dir_path=excluded.dir_path, filename=excluded.filename, file_size=excluded.file_size,
  mtime_ns=excluded.mtime_ns, page_count=excluded.page_count, cover_entry=excluded.cover_entry,
  comment_raw=excluded.comment_raw, has_nonutf8=excluded.has_nonutf8, updated_at=excluded.updated_at`,
		rec.Path, rec.Dir, rec.Filename, rec.FileSize, rec.MtimeNs,
		rec.PageCount, rec.CoverEntry, rec.CommentRaw, boolToInt(rec.HasNonUTF8), now, now)
	if err != nil {
		return 0, err
	}

	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM comics WHERE path=?`, rec.Path).Scan(&id); err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM comic_meta WHERE comic_id=?`, id); err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO comic_meta(comic_id,key,value) VALUES(?,?,?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for k, v := range rec.KV {
		if _, err := stmt.ExecContext(ctx, id, k, v); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// DeleteByPath removes a comic and cascades its meta/fav/progress.
func (s *Store) DeleteByPath(path string) error {
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM comics WHERE path=?`, path)
	return err
}

// MovePath re-keys an indexed comic from oldPath to newPath — the row that
// follows when the archive file is renamed on disk. The id is untouched, so
// the comic_meta keys, favorites and reading progress all survive the move.
func (s *Store) MovePath(oldPath, newPath string) error {
	res, err := s.db.ExecContext(context.Background(),
		`UPDATE comics SET path=?, dir_path=?, filename=? WHERE path=?`,
		newPath, filepath.Dir(newPath), filepath.Base(newPath), oldPath)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListPaths returns every indexed comic path.
func (s *Store) ListPaths() ([]string, error) {
	rows, err := s.db.QueryContext(context.Background(), `SELECT path FROM comics`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AllRows loads every comic joined with favorites/progress plus its comment
// keys. This is the full candidate set for in-memory filter/sort.
