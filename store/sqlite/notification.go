//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

func (s *sqliteStore) ListNotifications(ctx context.Context, personaID id.ID, cursor id.ID, limit int) ([]*murlog.Notification, error) {
	var rows *sql.Rows
	var err error

	if cursor.IsNil() {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, persona_id, type, actor_uri, post_id, read, created_at
			FROM notifications WHERE persona_id = ?
			ORDER BY id DESC LIMIT ?`,
			personaID.Bytes(), limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, persona_id, type, actor_uri, post_id, read, created_at
			FROM notifications WHERE persona_id = ? AND id < ?
			ORDER BY id DESC LIMIT ?`,
			personaID.Bytes(), cursor.Bytes(), limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []*murlog.Notification
	for rows.Next() {
		var n murlog.Notification
		var rawID, rawPersonaID []byte
		var rawPostID []byte
		var readInt int
		var createdAt string
		if err := rows.Scan(&rawID, &rawPersonaID, &n.Type, &n.ActorURI, &rawPostID, &readInt, &createdAt); err != nil {
			return nil, err
		}
		var sh scanHelper
		n.ID = sh.scanID(rawID)
		n.PersonaID = sh.scanID(rawPersonaID)
		if rawPostID != nil {
			n.PostID = sh.scanID(rawPostID)
		}
		n.Read = readInt != 0
		n.CreatedAt = sh.parseTime(createdAt)
		if sh.err != nil {
			return nil, sh.err
		}
		notifications = append(notifications, &n)
	}
	return notifications, rows.Err()
}

func (s *sqliteStore) CountUnreadNotifications(ctx context.Context, personaID id.ID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM notifications WHERE persona_id = ? AND read = 0`,
		personaID.Bytes()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}

func (s *sqliteStore) CreateNotification(ctx context.Context, n *murlog.Notification) error {
	var postID interface{}
	if !n.PostID.IsNil() {
		postID = n.PostID.Bytes()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notifications (id, persona_id, type, actor_uri, post_id, read, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.ID.Bytes(), n.PersonaID.Bytes(), n.Type, n.ActorURI, postID, n.Read, formatTime(n.CreatedAt))
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}
	return nil
}

func (s *sqliteStore) MarkNotificationRead(ctx context.Context, nid id.ID) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notifications SET read = 1 WHERE id = ?`, nid.Bytes())
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	return nil
}

func (s *sqliteStore) MarkAllNotificationsRead(ctx context.Context, personaID id.ID) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notifications SET read = 1 WHERE persona_id = ? AND read = 0`, personaID.Bytes())
	if err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}
	return nil
}

func (s *sqliteStore) DeleteNotification(ctx context.Context, nid id.ID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM notifications WHERE id = ?`, nid.Bytes())
	if err != nil {
		return fmt.Errorf("delete notification: %w", err)
	}
	return nil
}

func (s *sqliteStore) DeleteNotificationByActor(ctx context.Context, personaID id.ID, actorURI string, notifType string, postID id.ID) error {
	if postID.IsNil() {
		// Follow 通知など post_id なし / Follow notifications without post_id
		_, err := s.db.ExecContext(ctx, `
			DELETE FROM notifications WHERE persona_id = ? AND actor_uri = ? AND type = ? AND post_id IS NULL`,
			personaID.Bytes(), actorURI, notifType)
		if err != nil {
			return fmt.Errorf("delete notification by actor: %w", err)
		}
	} else {
		_, err := s.db.ExecContext(ctx, `
			DELETE FROM notifications WHERE persona_id = ? AND actor_uri = ? AND type = ? AND post_id = ?`,
			personaID.Bytes(), actorURI, notifType, postID.Bytes())
		if err != nil {
			return fmt.Errorf("delete notification by actor: %w", err)
		}
	}
	return nil
}

func (s *sqliteStore) CleanupNotifications(ctx context.Context, olderThan time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM notifications WHERE read = 1 AND created_at < ?`,
		formatTime(olderThan))
	if err != nil {
		return fmt.Errorf("cleanup notifications: %w", err)
	}
	return nil
}
