// Package index owns the SQLite database: schema, migrations, and all query
// access. It uses modernc.org/sqlite (pure Go) with WAL and a busy timeout so
// concurrent reader connections and a single writer coexist safely.
package index

import (
	"context"

	_ "modernc.org/sqlite"
)

// Row is a comic plus its parsed comment keys and reading state, ready to be
func (s *Store) AllRows() ([]Row, error) {
	ctx := context.Background()
	q := `
SELECT c.id, c.path, c.dir_path, c.filename, c.file_size, c.mtime_ns,
       c.page_count, c.comment_raw, c.has_nonutf8,
       COALESCE(f.comic_id, 0) > 0 AS fav,
       COALESCE(p.last_page, 0),
       COALESCE(p.updated_at, 0)
FROM comics c
LEFT JOIN favorites f        ON f.comic_id = c.id
LEFT JOIN reading_progress p ON p.comic_id = c.id
ORDER BY c.id`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Read all comic rows first, then attach their comment keys in a second pass.
	var out []Row
	metaByID := map[int64]map[string]string{}
	for rows.Next() {
		var r Row
		var fav int
		if err := rows.Scan(&r.ID, &r.Path, &r.Dir, &r.Filename, &r.FileSize,
			&r.MtimeNs, &r.PageCount, &r.CommentRaw, &r.HasNonUTF8, &fav, &r.LastPage, &r.ReadAt); err != nil {
			return nil, err
		}
		r.Fav = fav > 0
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	mrows, err := s.db.QueryContext(ctx, `SELECT comic_id, key, value FROM comic_meta`)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var cid int64
		var k, v string
		if err := mrows.Scan(&cid, &k, &v); err != nil {
			return nil, err
		}
		m, ok := metaByID[cid]
		if !ok {
			m = map[string]string{}
			metaByID[cid] = m
		}
		m[k] = v
	}
	if err := mrows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].KV = metaByID[out[i].ID]
		if out[i].KV == nil {
			out[i].KV = map[string]string{}
		}
	}
	return out, nil
}

// SetProgress upserts the last-read page for a comic.
