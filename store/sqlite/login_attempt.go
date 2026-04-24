//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GetLoginAttempt returns the failure count and lock expiry for an IP.
// IP の失敗回数とロック期限を返す。
func (s *sqliteStore) GetLoginAttempt(ctx context.Context, ip string) (int, time.Time, error) {
	var failCount int
	var lockedUntil string
	err := s.db.QueryRowContext(ctx, `
		SELECT fail_count, locked_until FROM login_attempts WHERE ip = ?`, ip).
		Scan(&failCount, &lockedUntil)
	if err == sql.ErrNoRows {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	var t time.Time
	if lockedUntil != "" {
		var sh scanHelper
		t = sh.parseTime(lockedUntil)
		if sh.err != nil {
			return 0, time.Time{}, sh.err
		}
	}
	return failCount, t, nil
}

// RecordLoginFailure increments the failure count and sets the lock expiry.
// 失敗回数をインクリメントし、ロック期限を設定する。
func (s *sqliteStore) RecordLoginFailure(ctx context.Context, ip string, lockedUntil time.Time) error {
	lu := ""
	if !lockedUntil.IsZero() {
		lu = formatTime(lockedUntil)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO login_attempts (ip, fail_count, locked_until)
		VALUES (?, 1, ?)
		ON CONFLICT(ip) DO UPDATE SET
			fail_count = fail_count + 1,
			locked_until = excluded.locked_until`,
		ip, lu)
	if err != nil {
		return fmt.Errorf("record login failure: %w", err)
	}
	return nil
}

// ClearLoginAttempts removes the record for an IP (on successful login).
// ログイン成功時に IP のレコードを削除する。
func (s *sqliteStore) ClearLoginAttempts(ctx context.Context, ip string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_attempts WHERE ip = ?`, ip)
	if err != nil {
		return fmt.Errorf("clear login attempts: %w", err)
	}
	return nil
}
