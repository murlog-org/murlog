package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

// blockJSON is the API representation of a Block.
// ブロックの API 表現。
type blockJSON struct {
	ID        string `json:"id"`
	ActorURI  string `json:"actor_uri"`
	CreatedAt string `json:"created_at"`
}

// domainBlockJSON is the API representation of a DomainBlock.
// ドメインブロックの API 表現。
type domainBlockJSON struct {
	ID        string `json:"id"`
	Domain    string `json:"domain"`
	CreatedAt string `json:"created_at"`
}

// blocks.list — list all actor blocks.
// 全アクターブロックを返す。
func (h *Handler) rpcBlocksList(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	blocks, err := h.store.ListBlocks(ctx)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	out := make([]blockJSON, len(blocks))
	for i, b := range blocks {
		out[i] = blockJSON{
			ID:        b.ID.String(),
			ActorURI:  b.ActorURI,
			CreatedAt: b.CreatedAt.Format(time.RFC3339),
		}
	}
	return out, nil
}

type blockCreateParams struct {
	ActorURI string `json:"actor_uri"`
}

// blocks.create — block a remote actor.
// リモート Actor をブロック → フォロー双方向削除 → Block Activity 配送。
func (h *Handler) rpcBlocksCreate(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[blockCreateParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.ActorURI == "" {
		return nil, newRPCErr(codeInvalidParams, "actor_uri is required")
	}

	now := time.Now()
	b := &murlog.Block{
		ID:        id.New(),
		ActorURI:  req.ActorURI,
		CreatedAt: now,
	}
	if err := h.store.CreateBlock(ctx, b); err != nil {
		return nil, newRPCErr(codeConflict, "already blocked")
	}

	// Delete bidirectional follow relationships (all personas).
	// 双方向フォロー関係を削除 (全ペルソナ)。
	personas, _ := h.store.ListPersonas(ctx)
	for _, p := range personas {
		h.store.DeleteFollowerByActorURI(ctx, p.ID, req.ActorURI)
		if f, err := h.store.GetFollowByTarget(ctx, p.ID, req.ActorURI); err == nil {
			h.store.DeleteFollow(ctx, f.ID)
		}
	}

	// Enqueue Block Activity delivery.
	// Block Activity 配送をキューに追加。
	personaID := h.primaryPersonaID(ctx)
	if !personaID.IsNil() {
		job := murlog.NewJob(murlog.JobSendBlock, map[string]string{
				"persona_id":       personaID.String(),
				"target_actor_uri": req.ActorURI,
			})
		h.queue.Enqueue(ctx, job)
	}

	return blockJSON{
		ID:        b.ID.String(),
		ActorURI:  b.ActorURI,
		CreatedAt: b.CreatedAt.Format(time.RFC3339),
	}, nil
}

type blockDeleteParams struct {
	ActorURI string `json:"actor_uri"`
}

// blocks.delete — unblock a remote actor.
// リモート Actor のブロックを解除 → Undo Block 配送。
func (h *Handler) rpcBlocksDelete(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[blockDeleteParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.ActorURI == "" {
		return nil, newRPCErr(codeInvalidParams, "actor_uri is required")
	}

	if err := h.store.DeleteBlock(ctx, req.ActorURI); err != nil {
		return nil, newRPCErr(codeNotFound, "block not found")
	}

	// Enqueue Undo Block delivery.
	// Undo Block 配送をキューに追加。
	personaID := h.primaryPersonaID(ctx)
	if !personaID.IsNil() {
		job := murlog.NewJob(murlog.JobSendUndoBlock, map[string]string{
				"persona_id":       personaID.String(),
				"target_actor_uri": req.ActorURI,
			})
		h.queue.Enqueue(ctx, job)
	}

	return statusOK, nil
}

// domain_blocks.list — list all domain blocks.
// 全ドメインブロックを返す。
func (h *Handler) rpcDomainBlocksList(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	blocks, err := h.store.ListDomainBlocks(ctx)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	out := make([]domainBlockJSON, len(blocks))
	for i, b := range blocks {
		out[i] = domainBlockJSON{
			ID:        b.ID.String(),
			Domain:    b.Domain,
			CreatedAt: b.CreatedAt.Format(time.RFC3339),
		}
	}
	return out, nil
}

type domainBlockCreateParams struct {
	Domain string `json:"domain"`
}

// domain_blocks.create — block a remote domain.
// リモートドメインをブロック → そのドメインの全フォロー関係を一括削除。
func (h *Handler) rpcDomainBlocksCreate(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[domainBlockCreateParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.Domain == "" {
		return nil, newRPCErr(codeInvalidParams, "domain is required")
	}

	now := time.Now()
	db := &murlog.DomainBlock{
		ID:        id.New(),
		Domain:    req.Domain,
		CreatedAt: now,
	}
	if err := h.store.CreateDomainBlock(ctx, db); err != nil {
		return nil, newRPCErr(codeConflict, "already blocked")
	}

	// Delete all follow relationships with the blocked domain (silent, no Activity delivery).
	// ブロックドメインとの全フォロー関係を削除 (サイレント、Activity 配送なし)。
	h.store.DeleteFollowsByTargetDomain(ctx, req.Domain)
	h.store.DeleteFollowersByActorDomain(ctx, req.Domain)

	// Cancel pending/failed queue jobs targeting the blocked domain.
	// ブロックドメイン宛ての pending/failed キュージョブをキャンセル。
	h.queue.CancelByDomain(ctx, req.Domain)

	return domainBlockJSON{
		ID:        db.ID.String(),
		Domain:    db.Domain,
		CreatedAt: db.CreatedAt.Format(time.RFC3339),
	}, nil
}

type domainBlockDeleteParams struct {
	Domain string `json:"domain"`
}

// domain_blocks.delete — unblock a remote domain.
// リモートドメインのブロックを解除。
func (h *Handler) rpcDomainBlocksDelete(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[domainBlockDeleteParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.Domain == "" {
		return nil, newRPCErr(codeInvalidParams, "domain is required")
	}

	if err := h.store.DeleteDomainBlock(ctx, req.Domain); err != nil {
		return nil, newRPCErr(codeNotFound, "domain block not found")
	}

	return statusOK, nil
}

// primaryPersonaID returns the primary persona's ID.
// プライマリペルソナの ID を返す。
func (h *Handler) primaryPersonaID(ctx context.Context) id.ID {
	personas, err := h.store.ListPersonas(ctx)
	if err != nil || len(personas) == 0 {
		return id.Nil
	}
	return personas[0].ID
}
