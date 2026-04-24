//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

// Session / セッション

func (s *sqliteStore) GetSession(ctx context.Context, tokenHash string) (*murlog.Session, error) {
	var sess murlog.Session
	var rawID []byte
	var expiresAt, createdAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, token_hash, expires_at, created_at
		FROM sessions WHERE token_hash = ?`, tokenHash).
		Scan(&rawID, &sess.TokenHash, &expiresAt, &createdAt)
	if err != nil {
		return nil, err
	}
	var sh scanHelper
	sess.ID = sh.scanID(rawID)
	sess.ExpiresAt = sh.parseTime(expiresAt)
	sess.CreatedAt = sh.parseTime(createdAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return &sess, nil
}

func (s *sqliteStore) CreateSession(ctx context.Context, sess *murlog.Session) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?)`,
		sess.ID.Bytes(), sess.TokenHash, formatTime(sess.ExpiresAt), formatTime(sess.CreatedAt))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *sqliteStore) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes all sessions past their expiry.
// 期限切れセッションをすべて削除する。
func (s *sqliteStore) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, formatTime(now()))
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

// APIToken / API トークン

func (s *sqliteStore) GetAPIToken(ctx context.Context, tokenHash string) (*murlog.APIToken, error) {
	var t murlog.APIToken
	var rawID []byte
	var rawAppID []byte
	var expiresAt sql.NullString
	var createdAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, token_hash, app_id, scopes, expires_at, created_at
		FROM api_tokens WHERE token_hash = ?`, tokenHash).
		Scan(&rawID, &t.Name, &t.TokenHash, &rawAppID, &t.Scopes, &expiresAt, &createdAt)
	if err != nil {
		return nil, err
	}
	var sh scanHelper
	t.ID = sh.scanID(rawID)
	if rawAppID != nil {
		t.AppID = sh.scanID(rawAppID)
	}
	if expiresAt.Valid {
		t.ExpiresAt = sh.parseTime(expiresAt.String)
	}
	t.CreatedAt = sh.parseTime(createdAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return &t, nil
}

func (s *sqliteStore) CreateAPIToken(ctx context.Context, t *murlog.APIToken) error {
	var appID interface{}
	if !t.AppID.IsNil() {
		appID = t.AppID.Bytes()
	}
	var expiresAt interface{}
	if !t.ExpiresAt.IsZero() {
		expiresAt = formatTime(t.ExpiresAt)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_tokens (id, name, token_hash, app_id, scopes, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID.Bytes(), t.Name, t.TokenHash, appID, t.Scopes, expiresAt, formatTime(t.CreatedAt))
	if err != nil {
		return fmt.Errorf("create api token: %w", err)
	}
	return nil
}

// DeleteExpiredAPITokens removes API tokens that have expired.
// 期限切れの API トークンを削除する。
func (s *sqliteStore) DeleteExpiredAPITokens(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at < ?`,
		formatTime(now()))
	if err != nil {
		return fmt.Errorf("delete expired api tokens: %w", err)
	}
	return nil
}

func (s *sqliteStore) DeleteAPIToken(ctx context.Context, tid id.ID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, tid.Bytes())
	if err != nil {
		return fmt.Errorf("delete api token: %w", err)
	}
	return nil
}
