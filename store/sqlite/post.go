//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

const postColumns = `id, persona_id, content, content_type, content_map, visibility, origin, uri, actor_uri, in_reply_to_uri, mentions_json, hashtags_json, reblogged_by_uri, reblog_of_post_id, summary, sensitive, created_at, updated_at`

func (s *sqliteStore) GetPost(ctx context.Context, pid id.ID) (*murlog.Post, error) {
	return scanPost(s.db.QueryRowContext(ctx, `
		SELECT `+postColumns+`
		FROM posts WHERE id = ?`, pid.Bytes()))
}

func (s *sqliteStore) GetPostByURI(ctx context.Context, uri string) (*murlog.Post, error) {
	return scanPost(s.db.QueryRowContext(ctx, `
		SELECT `+postColumns+`
		FROM posts WHERE uri = ?`, uri))
}

// GetPostsByURIs returns posts keyed by URI (batch query).
// URI をキーとした投稿マップを返す (バッチクエリ)。
func (s *sqliteStore) GetPostsByURIs(ctx context.Context, uris []string) (map[string]*murlog.Post, error) {
	if len(uris) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(uris))
	args := make([]any, len(uris))
	for i, u := range uris {
		placeholders[i] = "?"
		args[i] = u
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+postColumns+`
		FROM posts WHERE uri IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*murlog.Post)
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		if p.URI != "" {
			result[p.URI] = p
		}
	}
	return result, rows.Err()
}

// ListPostsByPersona returns posts ordered by UUIDv7 id (= receive order).
// Remote posts appear by arrival time, not original published time.
// UUIDv7 id 順（= 受信順）で投稿を返す。リモート投稿は元の投稿時刻ではなく到着順。
func (s *sqliteStore) ListPostsByPersona(ctx context.Context, personaID id.ID, cursor id.ID, limit int) ([]*murlog.Post, error) {
	var rows *sql.Rows
	var err error

	// Exclude direct messages from timeline.
	// タイムラインからダイレクトメッセージを除外。
	directVis := int(murlog.VisibilityDirect)
	if cursor.IsNil() {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+postColumns+`
			FROM posts WHERE persona_id = ? AND visibility != ?
			ORDER BY id DESC LIMIT ?`,
			personaID.Bytes(), directVis, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+postColumns+`
			FROM posts WHERE persona_id = ? AND visibility != ? AND id < ?
			ORDER BY id DESC LIMIT ?`,
			personaID.Bytes(), directVis, cursor.Bytes(), limit)
	}
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanPost)
}

// ListPublicLocalPosts returns public, locally-authored posts for a persona.
// 公開かつローカルの投稿をペルソナ別に取得する。
func (s *sqliteStore) ListPublicLocalPosts(ctx context.Context, personaID id.ID, cursor id.ID, limit int) ([]*murlog.Post, error) {
	var rows *sql.Rows
	var err error

	if cursor.IsNil() {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+postColumns+`
			FROM posts WHERE persona_id = ? AND origin = 'local' AND visibility = 0
			ORDER BY id DESC LIMIT ?`,
			personaID.Bytes(), limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+postColumns+`
			FROM posts WHERE persona_id = ? AND origin = 'local' AND visibility = 0 AND id < ?
			ORDER BY id DESC LIMIT ?`,
			personaID.Bytes(), cursor.Bytes(), limit)
	}
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanPost)
}

func (s *sqliteStore) CreatePost(ctx context.Context, p *murlog.Post) error {
	if err := s.insertPost(ctx, p); err != nil {
		return err
	}
	// Refresh cached post counter. / 投稿数キャッシュを更新。
	origin := p.Origin
	if origin == "" {
		origin = "local"
	}
	if origin == "local" {
		s.refreshPostCount(ctx, p.PersonaID)
	}
	return nil
}

// CreatePostBulk inserts a post without refreshing cached counters.
// Call RefreshAllCounters after bulk insertion.
// キャッシュカウンターを更新せずに投稿を挿入する。バルク挿入後に RefreshAllCounters を呼ぶこと。
func (s *sqliteStore) CreatePostBulk(ctx context.Context, p *murlog.Post) error {
	return s.insertPost(ctx, p)
}

func (s *sqliteStore) insertPost(ctx context.Context, p *murlog.Post) error {
	origin := p.Origin
	if origin == "" {
		origin = "local"
	}
	contentMapJSON, err := marshalJSON(p.ContentMap)
	if err != nil {
		return err
	}
	contentType := p.ContentType
	if contentType == "" {
		contentType = murlog.ContentTypeHTML
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO posts (id, persona_id, content, content_type, content_map, visibility, origin, uri, actor_uri, in_reply_to_uri, mentions_json, hashtags_json, reblogged_by_uri, reblog_of_post_id, summary, sensitive, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID.Bytes(), p.PersonaID.Bytes(), p.Content, contentType, contentMapJSON,
		int(p.Visibility), origin, nullString(p.URI), nullString(p.ActorURI),
		nullString(p.InReplyToURI), nullString(p.MentionsJSON), p.HashtagsJSON,
		nullString(p.RebloggedByURI), nullBytes(p.ReblogOfPostID),
		p.Summary, boolToInt(p.Sensitive),
		formatTime(p.CreatedAt), formatTime(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create post: %w", err)
	}
	s.syncPostTags(ctx, p)
	return nil
}

func (s *sqliteStore) UpdatePost(ctx context.Context, p *murlog.Post) error {
	contentMapJSON, err := marshalJSON(p.ContentMap)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE posts SET content = ?, content_map = ?, visibility = ?, mentions_json = ?, hashtags_json = ?, summary = ?, sensitive = ?, updated_at = ?
		WHERE id = ?`,
		p.Content, contentMapJSON, int(p.Visibility),
		nullString(p.MentionsJSON), p.HashtagsJSON, p.Summary, boolToInt(p.Sensitive),
		formatTime(p.UpdatedAt), p.ID.Bytes())
	if err != nil {
		return fmt.Errorf("update post: %w", err)
	}
	s.syncPostTags(ctx, p)
	return nil
}

func (s *sqliteStore) DeletePost(ctx context.Context, pid id.ID) error {
	// Get persona_id before delete for counter refresh.
	// カウンター更新用に削除前に persona_id を取得。
	var rawPersonaID []byte
	var origin string
	s.db.QueryRowContext(ctx, `SELECT persona_id, origin FROM posts WHERE id = ?`, pid.Bytes()).Scan(&rawPersonaID, &origin)
	_, err := s.db.ExecContext(ctx, `DELETE FROM posts WHERE id = ?`, pid.Bytes())
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	// Refresh cached post counter. / 投稿数キャッシュを更新。
	if origin == "local" && len(rawPersonaID) > 0 {
		personaID, _ := id.FromBytes(rawPersonaID)
		s.refreshPostCount(ctx, personaID)
	}
	return nil
}

// DeleteReblogPost deletes the wrapper post created for a local reblog.
// ローカルリブログ用の wrapper post を削除する。
func (s *sqliteStore) DeleteReblogPost(ctx context.Context, personaID id.ID, reblogOfPostID id.ID) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM posts WHERE persona_id = ? AND reblog_of_post_id = ?`,
		personaID.Bytes(), reblogOfPostID.Bytes())
	if err != nil {
		return fmt.Errorf("delete reblog post: %w", err)
	}
	s.refreshPostCount(ctx, personaID)
	return nil
}

// refreshPostCount updates the cached post_count from actual COUNT.
// 実際の COUNT から post_count キャッシュを更新する。
func (s *sqliteStore) refreshPostCount(ctx context.Context, personaID id.ID) {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE personas SET post_count = (SELECT COUNT(*) FROM posts WHERE persona_id = ? AND origin = 'local')
		WHERE id = ?`, personaID.Bytes(), personaID.Bytes()); err != nil {
		log.Printf("refreshPostCount: %v", err)
	}
}

// syncPostTags syncs the post_tags table with the post's hashtags.
// post_tags テーブルを投稿のハッシュタグと同期する。
func (s *sqliteStore) syncPostTags(ctx context.Context, p *murlog.Post) {
	s.db.ExecContext(ctx, `DELETE FROM post_tags WHERE post_id = ?`, p.ID.Bytes())
	tags := p.Hashtags()
	for _, tag := range tags {
		s.db.ExecContext(ctx, `INSERT OR IGNORE INTO post_tags (post_id, tag) VALUES (?, ?)`,
			p.ID.Bytes(), strings.ToLower(tag))
	}
}

// nullString returns a sql.NullString for INSERT (empty string → NULL).
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullBytes returns nil for zero-value id.ID, otherwise the byte representation.
// ゼロ値の id.ID には nil を、それ以外はバイト表現を返す。
func nullBytes(i id.ID) any {
	if i.IsNil() {
		return nil
	}
	return i.Bytes()
}

// scanPost scans a post from sql.Row or sql.Rows.
// sql.Row / sql.Rows から投稿をスキャンする。
// ListReplies returns posts that are replies to the given URI, ordered by created_at ASC.
// 指定 URI へのリプライを時系列順で返す。
func (s *sqliteStore) ListReplies(ctx context.Context, inReplyToURI string, cursor id.ID, limit int) ([]*murlog.Post, error) {
	var rows *sql.Rows
	var err error

	if cursor.IsNil() {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+postColumns+`
			FROM posts WHERE in_reply_to_uri = ?
			ORDER BY created_at ASC LIMIT ?`,
			inReplyToURI, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+postColumns+`
			FROM posts WHERE in_reply_to_uri = ? AND id > ?
			ORDER BY created_at ASC LIMIT ?`,
			inReplyToURI, cursor.Bytes(), limit)
	}
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanPost)
}

// ListPostsByHashtag returns posts containing the given hashtag, newest first.
// Uses the post_tags join table for indexed lookup.
// localOnly=true restricts to local posts only (for public pages).
// 指定ハッシュタグを含む投稿を新しい順に返す。
// post_tags テーブルの JOIN でインデックス検索。
// localOnly=true はローカル投稿のみに限定 (公開ページ用)。
func (s *sqliteStore) ListPostsByHashtag(ctx context.Context, tag string, cursor id.ID, limit int, localOnly bool) ([]*murlog.Post, error) {
	originFilter := ""
	if localOnly {
		originFilter = " AND p.origin = 'local'"
	}
	var rows *sql.Rows
	var err error

	if cursor.IsNil() {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+prefixedPostColumns("p.")+`
			FROM posts p JOIN post_tags pt ON pt.post_id = p.id
			WHERE pt.tag = ? AND p.visibility IN (0, 1)`+originFilter+`
			ORDER BY p.id DESC LIMIT ?`,
			strings.ToLower(tag), limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+prefixedPostColumns("p.")+`
			FROM posts p JOIN post_tags pt ON pt.post_id = p.id
			WHERE pt.tag = ? AND p.visibility IN (0, 1)`+originFilter+` AND p.id < ?
			ORDER BY p.id DESC LIMIT ?`,
			strings.ToLower(tag), cursor.Bytes(), limit)
	}
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanPost)
}

// prefixedPostColumns returns postColumns with a table prefix (e.g. "p.").
// テーブルプレフィックス付きの postColumns を返す。
func prefixedPostColumns(prefix string) string {
	cols := strings.Split(postColumns, ", ")
	for i, c := range cols {
		cols[i] = prefix + c
	}
	return strings.Join(cols, ", ")
}

// ListPostsByActorURI returns posts by a remote actor URI, newest first.
// リモート Actor URI の投稿を新しい順に返す。
func (s *sqliteStore) ListPostsByActorURI(ctx context.Context, actorURI string, cursor id.ID, limit int) ([]*murlog.Post, error) {
	var rows *sql.Rows
	var err error

	// Exclude DM (visibility 3). / DM (visibility 3) を除外。
	if cursor.IsNil() {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+postColumns+`
			FROM posts WHERE actor_uri = ? AND visibility != 3
			ORDER BY id DESC LIMIT ?`,
			actorURI, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+postColumns+`
			FROM posts WHERE actor_uri = ? AND visibility != 3 AND id < ?
			ORDER BY id DESC LIMIT ?`,
			actorURI, cursor.Bytes(), limit)
	}
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanPost)
}

func scanPost(sc scanner) (*murlog.Post, error) {
	var p murlog.Post
	var rawID, rawPersonaID, rawReblogOfPostID []byte
	var contentMap string
	var visibility int
	var origin string
	var uri, actorURI, inReplyToURI, mentionsJSON, rebloggedByURI sql.NullString
	var sensitive int
	var createdAt, updatedAt string
	err := sc.Scan(&rawID, &rawPersonaID, &p.Content, &p.ContentType, &contentMap, &visibility,
		&origin, &uri, &actorURI, &inReplyToURI, &mentionsJSON, &p.HashtagsJSON,
		&rebloggedByURI, &rawReblogOfPostID, &p.Summary, &sensitive, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	var sh scanHelper
	p.ID = sh.scanID(rawID)
	p.PersonaID = sh.scanID(rawPersonaID)
	p.ReblogOfPostID = sh.scanID(rawReblogOfPostID)
	p.ContentMap = sh.unmarshalStringMap(contentMap)
	p.Visibility = murlog.Visibility(visibility)
	p.Origin = origin
	p.URI = uri.String
	p.ActorURI = actorURI.String
	p.InReplyToURI = inReplyToURI.String
	p.MentionsJSON = mentionsJSON.String
	p.RebloggedByURI = rebloggedByURI.String
	p.Sensitive = sensitive != 0
	p.CreatedAt = sh.parseTime(createdAt)
	p.UpdatedAt = sh.parseTime(updatedAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return &p, nil
}
