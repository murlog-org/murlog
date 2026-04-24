//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/murlog-org/murlog"
)

func (s *sqliteStore) GetRemoteActor(ctx context.Context, uri string) (*murlog.RemoteActor, error) {
	var a murlog.RemoteActor
	var fetchedAt string
	var acct sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT uri, username, display_name, summary, inbox, avatar_url, header_url, featured_url, fields_json, acct, fetched_at
		FROM remote_actors WHERE uri = ?`, uri).
		Scan(&a.URI, &a.Username, &a.DisplayName, &a.Summary, &a.Inbox,
			&a.AvatarURL, &a.HeaderURL, &a.FeaturedURL, &a.FieldsJSON, &acct, &fetchedAt)
	if err != nil {
		return nil, err
	}
	a.Acct = acct.String
	var sh scanHelper
	a.FetchedAt = sh.parseTime(fetchedAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return &a, nil
}

// GetRemoteActorByAcct looks up a cached remote actor by acct (user@domain).
// acct (user@domain) でキャッシュ済みリモート Actor を検索する。
func (s *sqliteStore) GetRemoteActorByAcct(ctx context.Context, acct string) (*murlog.RemoteActor, error) {
	var a murlog.RemoteActor
	var fetchedAt string
	var acctVal sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT uri, username, display_name, summary, inbox, avatar_url, header_url, featured_url, fields_json, acct, fetched_at
		FROM remote_actors WHERE acct = ?`, acct).
		Scan(&a.URI, &a.Username, &a.DisplayName, &a.Summary, &a.Inbox,
			&a.AvatarURL, &a.HeaderURL, &a.FeaturedURL, &a.FieldsJSON, &acctVal, &fetchedAt)
	if err != nil {
		return nil, err
	}
	a.Acct = acctVal.String
	var sh scanHelper
	a.FetchedAt = sh.parseTime(fetchedAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return &a, nil
}

// UpsertRemoteActor inserts or updates a cached remote actor.
// キャッシュされたリモート Actor を挿入または更新する。
func (s *sqliteStore) UpsertRemoteActor(ctx context.Context, a *murlog.RemoteActor) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO remote_actors (uri, username, display_name, summary, inbox, avatar_url, header_url, featured_url, fields_json, acct, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uri) DO UPDATE SET
			username = excluded.username,
			display_name = excluded.display_name,
			summary = excluded.summary,
			inbox = excluded.inbox,
			avatar_url = excluded.avatar_url,
			header_url = excluded.header_url,
			featured_url = excluded.featured_url,
			fields_json = excluded.fields_json,
			acct = excluded.acct,
			fetched_at = excluded.fetched_at`,
		a.URI, a.Username, a.DisplayName, a.Summary, a.Inbox,
		a.AvatarURL, a.HeaderURL, a.FeaturedURL, a.FieldsJSON, nullString(a.Acct), formatTime(a.FetchedAt))
	if err != nil {
		return fmt.Errorf("upsert remote actor: %w", err)
	}
	return nil
}
