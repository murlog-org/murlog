package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/murlog-org/murlog"
)

// deadThreshold is the minimum failure count to consider a domain dead.
// ドメインを dead とみなす最小失敗回数。
const deadThreshold = 10

// deadWindow is the time window for dead domain detection.
// If the last failure is older than this, the domain is considered recovered.
// dead ドメイン判定の時間窓。最後の失敗がこれより古ければ回復とみなす。
const deadWindow = 1 * time.Hour

// ListDomainFailures returns all domain failure records.
// 全ドメイン失敗レコードを返す。
func (s *sqliteStore) ListDomainFailures(ctx context.Context) ([]*murlog.DomainFailure, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT domain, failure_count, last_error, first_failure_at, last_failure_at
		FROM domain_failures ORDER BY last_failure_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*murlog.DomainFailure
	for rows.Next() {
		var f murlog.DomainFailure
		var firstAt, lastAt string
		if err := rows.Scan(&f.Domain, &f.FailureCount, &f.LastError, &firstAt, &lastAt); err != nil {
			return nil, err
		}
		var sh scanHelper
		f.FirstFailureAt = sh.parseTime(firstAt)
		f.LastFailureAt = sh.parseTime(lastAt)
		if sh.err != nil {
			return nil, sh.err
		}
		results = append(results, &f)
	}
	return results, rows.Err()
}

// IncrementDomainFailure increments the failure count for a domain.
// ドメインの失敗カウントをインクリメントする。
func (s *sqliteStore) IncrementDomainFailure(ctx context.Context, domain, lastError string) error {
	now := formatTime(time.Now())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO domain_failures (domain, failure_count, last_error, first_failure_at, last_failure_at)
		VALUES (?, 1, ?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			failure_count = failure_count + 1,
			last_error = excluded.last_error,
			last_failure_at = excluded.last_failure_at`,
		domain, lastError, now, now)
	return err
}

// ResetDomainFailure resets the failure count for a domain (on successful delivery).
// ドメインの失敗カウントをリセットする（配送成功時）。
func (s *sqliteStore) ResetDomainFailure(ctx context.Context, domain string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM domain_failures WHERE domain = ?`, domain)
	return err
}

// IsDomainDead returns true if a domain has failed too many times recently.
// ドメインが最近多数の失敗をしていれば true を返す。
func (s *sqliteStore) IsDomainDead(ctx context.Context, domain string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT failure_count FROM domain_failures
		WHERE domain = ? AND last_failure_at > ?`,
		domain, formatTime(time.Now().Add(-deadWindow))).Scan(&count)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("is domain dead: %w", err)
	}
	return count >= deadThreshold, nil
}
