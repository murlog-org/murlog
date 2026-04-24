package handler

import (
	"context"
	"encoding/json"

	"github.com/murlog-org/murlog/id"
)

// timelineParams is the params for timeline.home.
type timelineParams struct {
	PersonaID string `json:"persona_id,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// rpcTimelineHome handles timeline.home.
// ホームタイムライン — ローカル投稿 + リモート投稿を統合して返す。
func (h *Handler) rpcTimelineHome(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[timelineParams](params)
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

	// ListPostsByPersona returns both local and remote posts for the persona,
	// ordered by created_at DESC — this IS the home timeline.
	// ローカル投稿 + リモート投稿を created_at DESC で返す = ホームタイムライン。
	posts, err := h.store.ListPostsByPersona(ctx, personaID, cursor, limit)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	return h.enrichPostJSONList(ctx, posts), nil
}
