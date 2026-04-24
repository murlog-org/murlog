package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

// notificationJSON is the API representation of a Notification.
type notificationJSON struct {
	ID               string `json:"id"`
	PersonaID        string `json:"persona_id"`
	Type             string `json:"type"`
	ActorURI         string `json:"actor_uri"`
	ActorDisplayName string `json:"actor_display_name,omitempty"` // 表示名 / display name
	ActorAcct        string `json:"actor_acct,omitempty"`         // @user@host
	ActorAvatarURL   string `json:"actor_avatar_url,omitempty"`   // アバター / avatar
	PostID           string `json:"post_id,omitempty"`
	PostContent      string `json:"post_content,omitempty"` // 投稿プレビュー / post preview
	Read             bool   `json:"read"`
	CreatedAt        string `json:"created_at"`
}

func (h *Handler) toNotificationJSON(ctx context.Context, n *murlog.Notification) notificationJSON {
	nj := notificationJSON{
		ID:        n.ID.String(),
		PersonaID: n.PersonaID.String(),
		Type:      n.Type,
		ActorURI:  n.ActorURI,
		Read:      n.Read,
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
	}
	if !n.PostID.IsNil() {
		nj.PostID = n.PostID.String()
	}

	// Enrich actor info from remote_actors cache.
	// remote_actors キャッシュから Actor 情報を付与。
	if ra, err := h.store.GetRemoteActor(ctx, n.ActorURI); err == nil && ra != nil {
		nj.ActorDisplayName = ra.DisplayName
		nj.ActorAvatarURL = ra.AvatarURL
		if ra.Acct != "" {
			nj.ActorAcct = "@" + ra.Acct
		}
	}

	// Enrich post content preview.
	// 投稿内容のプレビューを付与。
	if !n.PostID.IsNil() {
		if post, err := h.store.GetPost(ctx, n.PostID); err == nil {
			nj.PostContent = truncate(stripHTML(post.Content), 200)
		}
	}
	return nj
}

type notificationsListParams struct {
	PersonaID string `json:"persona_id,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

func (h *Handler) rpcNotificationsList(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[notificationsListParams](params)
	if rErr != nil {
		return nil, rErr
	}

	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}

	var cursor id.ID
	if req.Cursor != "" {
		c, err := id.Parse(req.Cursor)
		if err == nil {
			cursor = c
		}
	}

	limit := clampLimit(req.Limit, 20, 100)

	notifications, err := h.store.ListNotifications(ctx, personaID, cursor, limit)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	out := make([]notificationJSON, len(notifications))
	for i, n := range notifications {
		out[i] = h.toNotificationJSON(ctx, n)
	}
	return out, nil
}

type notificationReadParams struct {
	ID string `json:"id"`
}

func (h *Handler) rpcNotificationsRead(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[notificationReadParams](params)
	if rErr != nil {
		return nil, rErr
	}

	nid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	if err := h.store.MarkNotificationRead(ctx, nid); err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	return statusOK, nil
}

func (h *Handler) rpcNotificationsReadAll(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[notificationsListParams](params)
	if rErr != nil {
		return nil, rErr
	}

	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}

	if err := h.store.MarkAllNotificationsRead(ctx, personaID); err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	return statusOK, nil
}

func (h *Handler) rpcNotificationsCountUnread(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[notificationsListParams](params)
	if rErr != nil {
		return nil, rErr
	}

	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}

	count, err := h.store.CountUnreadNotifications(ctx, personaID)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	return map[string]int{"count": count}, nil
}

func (h *Handler) rpcNotificationsDelete(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[notificationReadParams](params)
	if rErr != nil {
		return nil, rErr
	}

	nid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	if err := h.store.DeleteNotification(ctx, nid); err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	return statusOK, nil
}

// notifications.poll — returns notifications since a given ID.
// CGI 環境でのポーリング用。since 以降の通知を返す。
type notificationsPollParams struct {
	PersonaID string `json:"persona_id,omitempty"`
	Since     string `json:"since,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

func (h *Handler) rpcNotificationsPoll(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[notificationsPollParams](params)
	if rErr != nil {
		return nil, rErr
	}

	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}

	limit := clampLimit(req.Limit, 20, 100)

	// "since" is a notification ID — we want items AFTER it.
	// ListNotifications uses cursor as "before" — poll needs "after".
	// Fetch all recent and filter client-side for simplicity.
	// 一人用サーバーなので量は少ない。全件取得して since 以降をフィルタ。
	all, err := h.store.ListNotifications(ctx, personaID, id.ID{}, limit+100)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	var sinceID id.ID
	if req.Since != "" {
		s, err := id.Parse(req.Since)
		if err == nil {
			sinceID = s
		}
	}

	var out []notificationJSON
	for _, n := range all {
		if !sinceID.IsNil() && n.ID.String() <= sinceID.String() {
			break // UUIDv7 is time-ordered, results are DESC
		}
		out = append(out, h.toNotificationJSON(ctx, n))
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		out = []notificationJSON{}
	}
	return out, nil
}
