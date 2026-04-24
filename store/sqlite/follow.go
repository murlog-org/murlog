//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

// Follow (local -> remote) / フォロー (ローカル → リモート)

func scanFollow(sc scanner) (*murlog.Follow, error) {
	var f murlog.Follow
	var rawID, rawPersonaID []byte
	var accepted int
	var createdAt string
	if err := sc.Scan(&rawID, &rawPersonaID, &f.TargetURI, &accepted, &createdAt); err != nil {
		return nil, err
	}
	var sh scanHelper
	f.ID = sh.scanID(rawID)
	f.PersonaID = sh.scanID(rawPersonaID)
	f.Accepted = accepted != 0
	f.CreatedAt = sh.parseTime(createdAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return &f, nil
}

func (s *sqliteStore) GetFollow(ctx context.Context, fid id.ID) (*murlog.Follow, error) {
	return scanFollow(s.db.QueryRowContext(ctx, `
		SELECT id, persona_id, target_uri, accepted, created_at
		FROM follows WHERE id = ?`, fid.Bytes()))
}

func (s *sqliteStore) GetFollowByTarget(ctx context.Context, personaID id.ID, targetURI string) (*murlog.Follow, error) {
	return scanFollow(s.db.QueryRowContext(ctx, `
		SELECT id, persona_id, target_uri, accepted, created_at
		FROM follows WHERE persona_id = ? AND target_uri = ?`,
		personaID.Bytes(), targetURI))
}

func (s *sqliteStore) ListFollows(ctx context.Context, personaID id.ID) ([]*murlog.Follow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, persona_id, target_uri, accepted, created_at
		FROM follows WHERE persona_id = ? ORDER BY created_at DESC`,
		personaID.Bytes())
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanFollow)
}

func (s *sqliteStore) CreateFollow(ctx context.Context, f *murlog.Follow) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO follows (id, persona_id, target_uri, accepted, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		f.ID.Bytes(), f.PersonaID.Bytes(), f.TargetURI, f.Accepted, formatTime(f.CreatedAt))
	if err != nil {
		return fmt.Errorf("create follow: %w", err)
	}
	s.refreshFollowingCount(ctx, f.PersonaID)
	return nil
}

func (s *sqliteStore) UpdateFollow(ctx context.Context, f *murlog.Follow) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE follows SET accepted = ? WHERE id = ?`,
		f.Accepted, f.ID.Bytes())
	if err != nil {
		return fmt.Errorf("update follow: %w", err)
	}
	return nil
}

func (s *sqliteStore) DeleteFollow(ctx context.Context, fid id.ID) error {
	var rawPersonaID []byte
	s.db.QueryRowContext(ctx, `SELECT persona_id FROM follows WHERE id = ?`, fid.Bytes()).Scan(&rawPersonaID)
	_, err := s.db.ExecContext(ctx, `DELETE FROM follows WHERE id = ?`, fid.Bytes())
	if err != nil {
		return fmt.Errorf("delete follow: %w", err)
	}
	if len(rawPersonaID) > 0 {
		personaID, _ := id.FromBytes(rawPersonaID)
		s.refreshFollowingCount(ctx, personaID)
	}
	return nil
}

// DeleteFollowsByTargetDomain deletes all follows whose target_uri belongs to the given domain.
// 指定ドメインに属する全フォローを削除する。
func (s *sqliteStore) DeleteFollowsByTargetDomain(ctx context.Context, domain string) error {
	if !isValidDomain(domain) {
		return fmt.Errorf("delete follows by target domain: invalid domain %q", domain)
	}
	pattern := "%://" + domain + "/%"
	_, err := s.db.ExecContext(ctx, `DELETE FROM follows WHERE target_uri LIKE ?`, pattern)
	if err != nil {
		return fmt.Errorf("delete follows by target domain: %w", err)
	}
	s.refreshAllFollowingCounts(ctx)
	return nil
}

// Follower (remote -> local) / フォロワー (リモート → ローカル)

func scanFollower(sc scanner) (*murlog.Follower, error) {
	var f murlog.Follower
	var rawID, rawPersonaID []byte
	var approved int
	var createdAt string
	if err := sc.Scan(&rawID, &rawPersonaID, &f.ActorURI, &approved, &createdAt); err != nil {
		return nil, err
	}
	var sh scanHelper
	f.ID = sh.scanID(rawID)
	f.PersonaID = sh.scanID(rawPersonaID)
	f.Approved = approved != 0
	f.CreatedAt = sh.parseTime(createdAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return &f, nil
}

func (s *sqliteStore) GetFollower(ctx context.Context, fid id.ID) (*murlog.Follower, error) {
	return scanFollower(s.db.QueryRowContext(ctx, `
		SELECT id, persona_id, actor_uri, approved, created_at
		FROM followers WHERE id = ?`, fid.Bytes()))
}

func (s *sqliteStore) ListFollowers(ctx context.Context, personaID id.ID) ([]*murlog.Follower, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, persona_id, actor_uri, approved, created_at
		FROM followers WHERE persona_id = ? AND approved = 1 ORDER BY created_at DESC`,
		personaID.Bytes())
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanFollower)
}

func (s *sqliteStore) ListFollowersPaged(ctx context.Context, personaID id.ID, cursor id.ID, limit int) ([]*murlog.Follower, error) {
	var rows *sql.Rows
	var err error

	if cursor.IsNil() {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, persona_id, actor_uri, approved, created_at
			FROM followers WHERE persona_id = ? AND approved = 1
			ORDER BY id ASC LIMIT ?`,
			personaID.Bytes(), limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, persona_id, actor_uri, approved, created_at
			FROM followers WHERE persona_id = ? AND approved = 1 AND id > ?
			ORDER BY id ASC LIMIT ?`,
			personaID.Bytes(), cursor.Bytes(), limit)
	}
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanFollower)
}

func (s *sqliteStore) CreateFollower(ctx context.Context, f *murlog.Follower) error {
	if err := s.insertFollower(ctx, f); err != nil {
		return err
	}
	s.refreshFollowersCount(ctx, f.PersonaID)
	return nil
}

// CreateFollowerBulk inserts a follower without refreshing cached counters.
// キャッシュカウンターを更新せずにフォロワーを挿入する。
func (s *sqliteStore) CreateFollowerBulk(ctx context.Context, f *murlog.Follower) error {
	return s.insertFollower(ctx, f)
}

func (s *sqliteStore) insertFollower(ctx context.Context, f *murlog.Follower) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO followers (id, persona_id, actor_uri, approved, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		f.ID.Bytes(), f.PersonaID.Bytes(), f.ActorURI, f.Approved, formatTime(f.CreatedAt))
	if err != nil {
		return fmt.Errorf("create follower: %w", err)
	}
	return nil
}

// ListPendingFollowers returns unapproved followers for a persona.
// 未承認のフォロワー一覧を返す。
func (s *sqliteStore) ListPendingFollowers(ctx context.Context, personaID id.ID) ([]*murlog.Follower, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, persona_id, actor_uri, approved, created_at
		FROM followers WHERE persona_id = ? AND approved = 0 ORDER BY created_at`,
		personaID.Bytes())
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanFollower)
}

// ApproveFollower sets a follower's approved flag to true.
// フォロワーの承認フラグを true にする。
func (s *sqliteStore) ApproveFollower(ctx context.Context, fid id.ID) error {
	_, err := s.db.ExecContext(ctx, `UPDATE followers SET approved = 1 WHERE id = ?`, fid.Bytes())
	if err != nil {
		return fmt.Errorf("approve follower: %w", err)
	}
	// Refresh counter after approval. / 承認後にカウンターを更新。
	var rawPersonaID []byte
	s.db.QueryRowContext(ctx, `SELECT persona_id FROM followers WHERE id = ?`, fid.Bytes()).Scan(&rawPersonaID)
	if len(rawPersonaID) > 0 {
		personaID, _ := id.FromBytes(rawPersonaID)
		s.refreshFollowersCount(ctx, personaID)
	}
	return nil
}

func (s *sqliteStore) DeleteFollower(ctx context.Context, fid id.ID) error {
	// Get persona_id before delete for counter refresh.
	var rawPersonaID []byte
	s.db.QueryRowContext(ctx, `SELECT persona_id FROM followers WHERE id = ?`, fid.Bytes()).Scan(&rawPersonaID)
	_, err := s.db.ExecContext(ctx, `DELETE FROM followers WHERE id = ?`, fid.Bytes())
	if err != nil {
		return fmt.Errorf("delete follower: %w", err)
	}
	if len(rawPersonaID) > 0 {
		personaID, _ := id.FromBytes(rawPersonaID)
		s.refreshFollowersCount(ctx, personaID)
	}
	return nil
}

// DeleteFollowerByActorURI deletes a follower by persona and actor URI (O(1) via UNIQUE index).
// ペルソナと Actor URI でフォロワーを削除する (UNIQUE インデックスで O(1))。
func (s *sqliteStore) DeleteFollowerByActorURI(ctx context.Context, personaID id.ID, actorURI string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM followers WHERE persona_id = ? AND actor_uri = ?`,
		personaID.Bytes(), actorURI)
	if err != nil {
		return fmt.Errorf("delete follower by actor uri: %w", err)
	}
	s.refreshFollowersCount(ctx, personaID)
	return nil
}

// DeleteFollowersByActorDomain deletes all followers whose actor_uri belongs to the given domain.
// 指定ドメインに属する全フォロワーを削除する。
func (s *sqliteStore) DeleteFollowersByActorDomain(ctx context.Context, domain string) error {
	if !isValidDomain(domain) {
		return fmt.Errorf("delete followers by actor domain: invalid domain %q", domain)
	}
	pattern := "%://" + domain + "/%"
	_, err := s.db.ExecContext(ctx, `DELETE FROM followers WHERE actor_uri LIKE ?`, pattern)
	if err != nil {
		return fmt.Errorf("delete followers by actor domain: %w", err)
	}
	// Refresh all persona counters. / 全ペルソナのカウンターを更新。
	s.refreshAllFollowersCounts(ctx)
	return nil
}

// isValidDomain checks that a domain contains only DNS-safe characters.
// LIKE メタ文字 (%, _) の混入を防ぐ。
// ドメインが DNS セーフな文字のみを含むか検証する。
// --- Counter refresh helpers / カウンターリフレッシュヘルパー ---

func (s *sqliteStore) refreshFollowersCount(ctx context.Context, personaID id.ID) {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE personas SET followers_count = (SELECT COUNT(*) FROM followers WHERE persona_id = ? AND approved = 1)
		WHERE id = ?`, personaID.Bytes(), personaID.Bytes()); err != nil {
		log.Printf("refreshFollowersCount: %v", err)
	}
}

func (s *sqliteStore) refreshFollowingCount(ctx context.Context, personaID id.ID) {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE personas SET following_count = (SELECT COUNT(*) FROM follows WHERE persona_id = ?)
		WHERE id = ?`, personaID.Bytes(), personaID.Bytes()); err != nil {
		log.Printf("refreshFollowingCount: %v", err)
	}
}

func (s *sqliteStore) refreshAllFollowersCounts(ctx context.Context) {
	s.db.ExecContext(ctx, `
		UPDATE personas SET followers_count = (SELECT COUNT(*) FROM followers WHERE followers.persona_id = personas.id AND approved = 1)`)
}

func (s *sqliteStore) refreshAllFollowingCounts(ctx context.Context) {
	s.db.ExecContext(ctx, `
		UPDATE personas SET following_count = (SELECT COUNT(*) FROM follows WHERE follows.persona_id = personas.id)`)
}

// RefreshAllCounters recalculates all cached counters from actual data.
// Used after bulk operations (seed, migration).
// 全キャッシュカウンターを実データから再計算する。バルク操作後に使用。
func (s *sqliteStore) RefreshAllCounters(ctx context.Context) {
	s.db.ExecContext(ctx, `
		UPDATE personas SET
			post_count = (SELECT COUNT(*) FROM posts WHERE posts.persona_id = personas.id AND posts.origin = 'local'),
			followers_count = (SELECT COUNT(*) FROM followers WHERE followers.persona_id = personas.id AND followers.approved = 1),
			following_count = (SELECT COUNT(*) FROM follows WHERE follows.persona_id = personas.id)`)
}

func isValidDomain(domain string) bool {
	if domain == "" {
		return false
	}
	for _, c := range domain {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.') {
			return false
		}
	}
	return true
}
