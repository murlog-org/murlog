//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

func (s *sqliteStore) ListReblogsByPost(ctx context.Context, postID id.ID) ([]*murlog.Reblog, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, post_id, actor_uri, created_at
		FROM reblogs WHERE post_id = ? ORDER BY created_at`,
		postID.Bytes())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reblogs []*murlog.Reblog
	for rows.Next() {
		var r murlog.Reblog
		var rawID, rawPostID []byte
		var createdAt string
		if err := rows.Scan(&rawID, &rawPostID, &r.ActorURI, &createdAt); err != nil {
			return nil, err
		}
		var sh scanHelper
		r.ID = sh.scanID(rawID)
		r.PostID = sh.scanID(rawPostID)
		r.CreatedAt = sh.parseTime(createdAt)
		if sh.err != nil {
			return nil, sh.err
		}
		reblogs = append(reblogs, &r)
	}
	return reblogs, rows.Err()
}

func (s *sqliteStore) HasReblogged(ctx context.Context, postID id.ID, actorURI string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM reblogs WHERE post_id = ? AND actor_uri = ? LIMIT 1`,
		postID.Bytes(), actorURI).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("has reblogged: %w", err)
	}
	return true, nil
}

func (s *sqliteStore) CreateReblog(ctx context.Context, r *murlog.Reblog) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO reblogs (id, post_id, actor_uri, created_at)
		VALUES (?, ?, ?, ?)`,
		r.ID.Bytes(), r.PostID.Bytes(), r.ActorURI, formatTime(r.CreatedAt))
	if err != nil {
		return fmt.Errorf("create reblog: %w", err)
	}
	return nil
}

func (s *sqliteStore) DeleteReblog(ctx context.Context, postID id.ID, actorURI string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM reblogs WHERE post_id = ? AND actor_uri = ?`,
		postID.Bytes(), actorURI)
	if err != nil {
		return fmt.Errorf("delete reblog: %w", err)
	}
	return nil
}
