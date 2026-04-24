//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

const attachmentColumns = `id, post_id, file_path, mime_type, alt, width, height, size, created_at`

func (s *sqliteStore) GetAttachment(ctx context.Context, aid id.ID) (*murlog.Attachment, error) {
	return scanAttachment(s.db.QueryRowContext(ctx, `
		SELECT `+attachmentColumns+`
		FROM attachments WHERE id = ?`, aid.Bytes()))
}

func (s *sqliteStore) ListAttachmentsByPost(ctx context.Context, postID id.ID) ([]*murlog.Attachment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+attachmentColumns+`
		FROM attachments WHERE post_id = ?
		ORDER BY created_at ASC`, postID.Bytes())
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanAttachment)
}

// ListAttachmentsByPosts returns attachments grouped by post ID (batch query).
// 投稿 ID ごとにグループ化した添付ファイルを返す (バッチクエリ)。
func (s *sqliteStore) ListAttachmentsByPosts(ctx context.Context, postIDs []id.ID) (map[id.ID][]*murlog.Attachment, error) {
	if len(postIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(postIDs))
	args := make([]any, len(postIDs))
	for i, pid := range postIDs {
		placeholders[i] = "?"
		args[i] = pid.Bytes()
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+attachmentColumns+`
		FROM attachments WHERE post_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY created_at ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[id.ID][]*murlog.Attachment)
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		result[a.PostID] = append(result[a.PostID], a)
	}
	return result, rows.Err()
}

func (s *sqliteStore) CreateAttachment(ctx context.Context, a *murlog.Attachment) error {
	var postID any
	if !a.PostID.IsNil() {
		postID = a.PostID.Bytes()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO attachments (id, post_id, file_path, mime_type, alt, width, height, size, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID.Bytes(), postID, a.FilePath, a.MimeType, a.Alt,
		a.Width, a.Height, a.Size, formatTime(a.CreatedAt))
	if err != nil {
		return fmt.Errorf("create attachment: %w", err)
	}
	return nil
}

// AttachToPost links orphaned attachments to a post. Already-attached attachments are ignored.
// 孤立した添付ファイルを投稿に紐づける。既に紐づいているものは無視。
func (s *sqliteStore) AttachToPost(ctx context.Context, attachmentIDs []id.ID, postID id.ID) error {
	if len(attachmentIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(attachmentIDs))
	args := make([]any, 0, len(attachmentIDs)+1)
	args = append(args, postID.Bytes())
	for i, aid := range attachmentIDs {
		placeholders[i] = "?"
		args = append(args, aid.Bytes())
	}
	query := `UPDATE attachments SET post_id = ? WHERE id IN (` + strings.Join(placeholders, ",") + `) AND post_id IS NULL`
	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("attach to post: %w", err)
	}
	return nil
}

func (s *sqliteStore) DeleteAttachment(ctx context.Context, aid id.ID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM attachments WHERE id = ?`, aid.Bytes())
	if err != nil {
		return fmt.Errorf("delete attachment: %w", err)
	}
	return nil
}

// ListOrphanAttachments returns unattached attachments older than the given time.
// 指定時刻より古い未紐づけの添付ファイルを返す。
func (s *sqliteStore) ListOrphanAttachments(ctx context.Context, olderThan time.Time) ([]*murlog.Attachment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+attachmentColumns+`
		FROM attachments WHERE post_id IS NULL AND created_at < ?
		ORDER BY created_at ASC`, formatTime(olderThan))
	if err != nil {
		return nil, err
	}
	return scanRows(rows, scanAttachment)
}

// scanAttachment scans an attachment from sql.Row or sql.Rows.
// sql.Row / sql.Rows から添付ファイルをスキャンする。
func scanAttachment(sc scanner) (*murlog.Attachment, error) {
	var a murlog.Attachment
	var rawID, rawPostID []byte
	var createdAt string
	err := sc.Scan(&rawID, &rawPostID, &a.FilePath, &a.MimeType, &a.Alt,
		&a.Width, &a.Height, &a.Size, &createdAt)
	if err != nil {
		return nil, err
	}
	var sh scanHelper
	a.ID = sh.scanID(rawID)
	a.PostID = sh.scanID(rawPostID)
	a.CreatedAt = sh.parseTime(createdAt)
	if sh.err != nil {
		return nil, sh.err
	}
	return &a, nil
}
