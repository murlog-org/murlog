package sqlite

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/murlog-org/murlog"
)

// ListBlocks returns all actor blocks.
// 全アクターブロックを返す。
func (s *sqliteStore) ListBlocks(ctx context.Context) ([]*murlog.Block, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, actor_uri, created_at
		FROM blocks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []*murlog.Block
	for rows.Next() {
		b := &murlog.Block{}
		var rawID []byte
		var createdAt string
		if err := rows.Scan(&rawID, &b.ActorURI, &createdAt); err != nil {
			return nil, err
		}
		var sh scanHelper
		b.ID = sh.scanID(rawID)
		b.CreatedAt = sh.parseTime(createdAt)
		if sh.err != nil {
			return nil, sh.err
		}
		blocks = append(blocks, b)
	}
	return blocks, rows.Err()
}

// GetBlockByActorURI returns a block by actor URI.
// Actor URI でブロックを取得する。
func (s *sqliteStore) GetBlockByActorURI(ctx context.Context, actorURI string) (*murlog.Block, error) {
	b := &murlog.Block{}
	var rawID []byte
	var createdAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, actor_uri, created_at
		FROM blocks WHERE actor_uri = ?`, actorURI).Scan(&rawID, &b.ActorURI, &createdAt)
	if err != nil {
		return nil, err
	}
	var sh scanHelper
	b.ID = sh.scanID(rawID)
	b.CreatedAt = sh.parseTime(createdAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return b, nil
}

// CreateBlock creates an actor block.
// アクターブロックを作成する。
func (s *sqliteStore) CreateBlock(ctx context.Context, b *murlog.Block) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO blocks (id, actor_uri, created_at)
		VALUES (?, ?, ?)`,
		b.ID.Bytes(), b.ActorURI, b.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("create block: %w", err)
	}
	return nil
}

// DeleteBlock deletes an actor block by actor URI.
// Actor URI でアクターブロックを削除する。
func (s *sqliteStore) DeleteBlock(ctx context.Context, actorURI string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM blocks WHERE actor_uri = ?`, actorURI)
	if err != nil {
		return fmt.Errorf("delete block: %w", err)
	}
	return nil
}

// IsBlocked checks if an actor URI is blocked (actor block or domain block).
// Actor URI がブロック済みか判定する (アクターブロックまたはドメインブロック)。
func (s *sqliteStore) IsBlocked(ctx context.Context, actorURI string) (bool, error) {
	// Check actor block. / アクターブロックを確認。
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM blocks WHERE actor_uri = ?`, actorURI).Scan(&count)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}

	// Check domain block. / ドメインブロックを確認。
	domain := domainFromURI(actorURI)
	if domain == "" {
		return false, nil
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM domain_blocks WHERE domain = ?`, domain).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListDomainBlocks returns all domain blocks.
// 全ドメインブロックを返す。
func (s *sqliteStore) ListDomainBlocks(ctx context.Context) ([]*murlog.DomainBlock, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, domain, created_at
		FROM domain_blocks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []*murlog.DomainBlock
	for rows.Next() {
		b := &murlog.DomainBlock{}
		var rawID []byte
		var createdAt string
		if err := rows.Scan(&rawID, &b.Domain, &createdAt); err != nil {
			return nil, err
		}
		var sh scanHelper
		b.ID = sh.scanID(rawID)
		b.CreatedAt = sh.parseTime(createdAt)
		if sh.err != nil {
			return nil, sh.err
		}
		blocks = append(blocks, b)
	}
	return blocks, rows.Err()
}

// CreateDomainBlock creates a domain block.
// ドメインブロックを作成する。
func (s *sqliteStore) CreateDomainBlock(ctx context.Context, db *murlog.DomainBlock) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO domain_blocks (id, domain, created_at)
		VALUES (?, ?, ?)`,
		db.ID.Bytes(), db.Domain, db.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("create domain block: %w", err)
	}
	return nil
}

// DeleteDomainBlock deletes a domain block.
// ドメインブロックを削除する。
func (s *sqliteStore) DeleteDomainBlock(ctx context.Context, domain string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM domain_blocks WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("delete domain block: %w", err)
	}
	return nil
}

// IsDomainBlocked checks if a domain is blocked.
// ドメインがブロック済みか判定する。
func (s *sqliteStore) IsDomainBlocked(ctx context.Context, domain string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM domain_blocks WHERE domain = ?`, domain).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// domainFromURI extracts the host (without port) from a URI.
// URI からホスト部分 (ポートなし) を抽出する。
func domainFromURI(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname()
}
