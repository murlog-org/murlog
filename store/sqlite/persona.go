//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

func (s *sqliteStore) GetPersona(ctx context.Context, pid id.ID) (*murlog.Persona, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, display_name, summary, avatar_path, header_path,
		       public_key_pem, private_key_pem, is_primary, locked, show_follows, discoverable, pinned_post_id, fields_json,
		       post_count, followers_count, following_count, created_at, updated_at
		FROM personas WHERE id = ?`, pid.Bytes())
	return scanPersona(row)
}

func (s *sqliteStore) GetPersonaByUsername(ctx context.Context, username string) (*murlog.Persona, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, display_name, summary, avatar_path, header_path,
		       public_key_pem, private_key_pem, is_primary, locked, show_follows, discoverable, pinned_post_id, fields_json,
		       post_count, followers_count, following_count, created_at, updated_at
		FROM personas WHERE username = ?`, username)
	return scanPersona(row)
}


func (s *sqliteStore) ListPersonas(ctx context.Context) ([]*murlog.Persona, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, display_name, summary, avatar_path, header_path,
		       public_key_pem, private_key_pem, is_primary, locked, show_follows, discoverable, pinned_post_id, fields_json,
		       post_count, followers_count, following_count, created_at, updated_at
		FROM personas ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanPersona)
}

func (s *sqliteStore) CreatePersona(ctx context.Context, p *murlog.Persona) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO personas (id, username, display_name, summary, avatar_path, header_path,
		                      public_key_pem, private_key_pem, is_primary, locked, show_follows, discoverable, pinned_post_id, fields_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID.Bytes(), p.Username, p.DisplayName, p.Summary, p.AvatarPath, p.HeaderPath,
		p.PublicKeyPEM, p.PrivateKeyPEM, p.Primary, p.Locked, p.ShowFollows, p.Discoverable, nilableIDBytes(p.PinnedPostID),
		p.FieldsJSON,
		formatTime(p.CreatedAt), formatTime(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create persona: %w", err)
	}
	return nil
}

func (s *sqliteStore) UpdatePersona(ctx context.Context, p *murlog.Persona) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE personas SET display_name = ?, summary = ?, avatar_path = ?, header_path = ?,
		                    locked = ?, show_follows = ?, discoverable = ?, fields_json = ?, updated_at = ?
		WHERE id = ?`,
		p.DisplayName, p.Summary, p.AvatarPath, p.HeaderPath,
		p.Locked, p.ShowFollows, p.Discoverable, p.FieldsJSON,
		formatTime(p.UpdatedAt), p.ID.Bytes())
	if err != nil {
		return fmt.Errorf("update persona: %w", err)
	}
	return nil
}

// scanPersona scans a persona from sql.Row or sql.Rows.
// sql.Row / sql.Rows からペルソナをスキャンする。
func scanPersona(sc scanner) (*murlog.Persona, error) {
	var p murlog.Persona
	var rawID, rawPinnedPostID []byte
	var isPrimary, isLocked, showFollows, discoverable int
	var createdAt, updatedAt string
	err := sc.Scan(&rawID, &p.Username, &p.DisplayName, &p.Summary, &p.AvatarPath, &p.HeaderPath,
		&p.PublicKeyPEM, &p.PrivateKeyPEM, &isPrimary, &isLocked, &showFollows, &discoverable, &rawPinnedPostID, &p.FieldsJSON,
		&p.PostCount, &p.FollowersCount, &p.FollowingCount, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	var sh scanHelper
	p.ID = sh.scanID(rawID)
	p.Primary = isPrimary != 0
	p.Locked = isLocked != 0
	p.ShowFollows = showFollows != 0
	p.Discoverable = discoverable != 0
	if len(rawPinnedPostID) > 0 {
		p.PinnedPostID = sh.scanID(rawPinnedPostID)
	}
	p.CreatedAt = sh.parseTime(createdAt)
	p.UpdatedAt = sh.parseTime(updatedAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return &p, nil
}

// nilableIDBytes returns nil for zero ID, otherwise the byte representation.
// ゼロ ID なら nil、それ以外はバイト表現を返す。
func nilableIDBytes(id id.ID) interface{} {
	if id.IsNil() {
		return nil
	}
	return id.Bytes()
}

// PinPost sets the pinned post for a persona (max 1, replaces previous).
// ペルソナのピン留め投稿を設定する (最大1件、前のピンを置き換え)。
func (s *sqliteStore) PinPost(ctx context.Context, personaID id.ID, postID id.ID) error {
	_, err := s.db.ExecContext(ctx, `UPDATE personas SET pinned_post_id = ? WHERE id = ?`,
		postID.Bytes(), personaID.Bytes())
	if err != nil {
		return fmt.Errorf("pin post: %w", err)
	}
	return nil
}

// UnpinPost clears the pinned post for a persona.
// ペルソナのピン留めを解除する。
func (s *sqliteStore) UnpinPost(ctx context.Context, personaID id.ID) error {
	_, err := s.db.ExecContext(ctx, `UPDATE personas SET pinned_post_id = NULL WHERE id = ?`,
		personaID.Bytes())
	if err != nil {
		return fmt.Errorf("unpin post: %w", err)
	}
	return nil
}

// GetPinnedPost returns the pinned post for a persona, or nil if none.
// ペルソナのピン留め投稿を返す。なければ nil。
func (s *sqliteStore) GetPinnedPost(ctx context.Context, personaID id.ID) (*murlog.Post, error) {
	var rawPinnedID []byte
	err := s.db.QueryRowContext(ctx, `SELECT pinned_post_id FROM personas WHERE id = ?`,
		personaID.Bytes()).Scan(&rawPinnedID)
	if err == sql.ErrNoRows || (err == nil && len(rawPinnedID) == 0) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get pinned post: %w", err)
	}
	var sh scanHelper
	postID := sh.scanID(rawPinnedID)
	if sh.err != nil {
		return nil, sh.err
	}
	if postID.IsNil() {
		return nil, nil
	}
	return s.GetPost(ctx, postID)
}
