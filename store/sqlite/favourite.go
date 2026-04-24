//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

func (s *sqliteStore) ListFavouritesByPost(ctx context.Context, postID id.ID) ([]*murlog.Favourite, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, post_id, actor_uri, created_at
		FROM favourites WHERE post_id = ? ORDER BY created_at`,
		postID.Bytes())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favourites []*murlog.Favourite
	for rows.Next() {
		var f murlog.Favourite
		var rawID, rawPostID []byte
		var createdAt string
		if err := rows.Scan(&rawID, &rawPostID, &f.ActorURI, &createdAt); err != nil {
			return nil, err
		}
		var sh scanHelper
		f.ID = sh.scanID(rawID)
		f.PostID = sh.scanID(rawPostID)
		f.CreatedAt = sh.parseTime(createdAt)
		if sh.err != nil {
			return nil, sh.err
		}
		favourites = append(favourites, &f)
	}
	return favourites, rows.Err()
}

func (s *sqliteStore) HasFavourited(ctx context.Context, postID id.ID, actorURI string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM favourites WHERE post_id = ? AND actor_uri = ? LIMIT 1`,
		postID.Bytes(), actorURI).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("has favourited: %w", err)
	}
	return true, nil
}

func (s *sqliteStore) CreateFavourite(ctx context.Context, f *murlog.Favourite) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO favourites (id, post_id, actor_uri, created_at)
		VALUES (?, ?, ?, ?)`,
		f.ID.Bytes(), f.PostID.Bytes(), f.ActorURI, formatTime(f.CreatedAt))
	if err != nil {
		return fmt.Errorf("create favourite: %w", err)
	}
	return nil
}

func (s *sqliteStore) DeleteFavourite(ctx context.Context, postID id.ID, actorURI string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM favourites WHERE post_id = ? AND actor_uri = ?`,
		postID.Bytes(), actorURI)
	if err != nil {
		return fmt.Errorf("delete favourite: %w", err)
	}
	return nil
}
