//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"fmt"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

// OAuthApp / OAuth アプリ

func (s *sqliteStore) GetOAuthApp(ctx context.Context, clientID string) (*murlog.OAuthApp, error) {
	var app murlog.OAuthApp
	var rawID []byte
	var createdAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, client_id, client_secret, name, redirect_uri, scopes, created_at
		FROM oauth_apps WHERE client_id = ?`, clientID).
		Scan(&rawID, &app.ClientID, &app.ClientSecret, &app.Name,
			&app.RedirectURI, &app.Scopes, &createdAt)
	if err != nil {
		return nil, err
	}
	var sh scanHelper
	app.ID = sh.scanID(rawID)
	app.CreatedAt = sh.parseTime(createdAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return &app, nil
}

func (s *sqliteStore) CreateOAuthApp(ctx context.Context, app *murlog.OAuthApp) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO oauth_apps (id, client_id, client_secret, name, redirect_uri, scopes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		app.ID.Bytes(), app.ClientID, app.ClientSecret, app.Name,
		app.RedirectURI, app.Scopes, formatTime(app.CreatedAt))
	if err != nil {
		return fmt.Errorf("create oauth app: %w", err)
	}
	return nil
}

// OAuthCode / OAuth 認可コード

func (s *sqliteStore) GetOAuthCode(ctx context.Context, code string) (*murlog.OAuthCode, error) {
	var c murlog.OAuthCode
	var rawID, rawAppID []byte
	var expiresAt, createdAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, app_id, code, redirect_uri, scopes, code_challenge, expires_at, created_at
		FROM oauth_codes WHERE code = ?`, code).
		Scan(&rawID, &rawAppID, &c.Code, &c.RedirectURI, &c.Scopes,
			&c.CodeChallenge, &expiresAt, &createdAt)
	if err != nil {
		return nil, err
	}
	var sh scanHelper
	c.ID = sh.scanID(rawID)
	c.AppID = sh.scanID(rawAppID)
	c.ExpiresAt = sh.parseTime(expiresAt)
	c.CreatedAt = sh.parseTime(createdAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return &c, nil
}

func (s *sqliteStore) CreateOAuthCode(ctx context.Context, c *murlog.OAuthCode) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO oauth_codes (id, app_id, code, redirect_uri, scopes, code_challenge, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID.Bytes(), c.AppID.Bytes(), c.Code, c.RedirectURI, c.Scopes,
		c.CodeChallenge, formatTime(c.ExpiresAt), formatTime(c.CreatedAt))
	if err != nil {
		return fmt.Errorf("create oauth code: %w", err)
	}
	return nil
}

func (s *sqliteStore) DeleteOAuthCode(ctx context.Context, code string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_codes WHERE code = ?`, code)
	if err != nil {
		return fmt.Errorf("delete oauth code: %w", err)
	}
	return nil
}

// DeleteAPITokensByApp deletes all tokens issued via a specific OAuth app.
// 特定の OAuth アプリ経由で発行された全トークンを削除する。
func (s *sqliteStore) DeleteAPITokensByApp(ctx context.Context, appID id.ID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE app_id = ?`, appID.Bytes())
	if err != nil {
		return fmt.Errorf("delete api tokens by app: %w", err)
	}
	return nil
}
