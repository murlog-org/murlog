package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/activitypub"
	"github.com/murlog-org/murlog/id"
)

// rpcPersonasList handles personas.list.
// ペルソナ一覧を返す。
func (h *Handler) rpcPersonasList(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	personas, err := h.store.ListPersonas(ctx)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	base := h.baseURLFromCtx(ctx)
	out := make([]accountJSON, len(personas))
	for i, p := range personas {
		out[i] = h.toAccountFromPersona(ctx, base, p)
	}
	return out, nil
}

// getParams holds an "id" field used by several RPC methods.
type getParams struct {
	ID string `json:"id"`
}

// rpcPersonasGet handles personas.get.
// 指定ペルソナを返す。
func (h *Handler) rpcPersonasGet(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[getParams](params)
	if rErr != nil {
		return nil, rErr
	}

	pid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	p, err := h.store.GetPersona(ctx, pid)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "persona not found")
	}

	base := h.baseURLFromCtx(ctx)
	return h.toAccountFromPersona(ctx, base, p), nil
}

// createPersonaParams is the params for personas.create.
type createPersonaParams struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Summary     string `json:"summary"`
}

// rpcPersonasCreate handles personas.create.
// 新しいペルソナを作成する。
func (h *Handler) rpcPersonasCreate(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[createPersonaParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if err := murlog.ValidateUsername(req.Username); err != nil {
		return nil, newRPCErr(codeInvalidParams, err.Error())
	}

	// Check for duplicate username. / ユーザー名の重複チェック。
	if existing, _ := h.store.GetPersonaByUsername(ctx, req.Username); existing != nil {
		return nil, newRPCErrData(codeConflict, "conflict", map[string]string{"field": "username", "reason": "already taken"})
	}

	// Generate RSA key pair. / RSA 鍵ペアを生成。
	pubPEM, privPEM, err := activitypub.GenerateKeyPair()
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	now := time.Now()
	p := &murlog.Persona{
		ID:            id.New(),
		Username:      req.Username,
		DisplayName:   req.DisplayName,
		Summary:       req.Summary,
		PublicKeyPEM:  pubPEM,
		PrivateKeyPEM: privPEM,
		Primary:       false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := h.store.CreatePersona(ctx, p); err != nil {
		return nil, newRPCErr(codeInternalError, "create persona failed")
	}

	base := h.baseURLFromCtx(ctx)
	return h.toAccountFromPersona(ctx, base, p), nil
}

// updatePersonaParams is the params for personas.update.
// maxCustomFields is the maximum number of custom fields per persona.
// ペルソナごとのカスタムフィールド最大数。
const maxCustomFields = 4

type updatePersonaParams struct {
	ID          string                `json:"id"`
	DisplayName *string               `json:"display_name,omitempty"`
	Summary     *string               `json:"summary,omitempty"`
	Fields      *[]murlog.CustomField `json:"fields,omitempty"`
	AvatarPath  *string               `json:"avatar_path,omitempty"`
	HeaderPath  *string               `json:"header_path,omitempty"`
	Locked      *bool                 `json:"locked,omitempty"`
	ShowFollows  *bool                 `json:"show_follows,omitempty"`
	Discoverable *bool                 `json:"discoverable,omitempty"`
}

// rpcPersonasUpdate handles personas.update.
// 指定ペルソナを更新する。
func (h *Handler) rpcPersonasUpdate(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[updatePersonaParams](params)
	if rErr != nil {
		return nil, rErr
	}

	pid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	p, err := h.store.GetPersona(ctx, pid)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "persona not found")
	}

	if req.DisplayName != nil {
		p.DisplayName = *req.DisplayName
	}
	if req.Summary != nil {
		p.Summary = *req.Summary
	}
	if req.Fields != nil {
		if len(*req.Fields) > maxCustomFields {
			return nil, newRPCErr(codeInvalidParams, "too many fields (max 4)")
		}
		p.SetFields(*req.Fields)
	}
	if req.AvatarPath != nil {
		p.AvatarPath = *req.AvatarPath
	}
	if req.HeaderPath != nil {
		p.HeaderPath = *req.HeaderPath
	}
	if req.Locked != nil {
		p.Locked = *req.Locked
	}
	if req.ShowFollows != nil {
		p.ShowFollows = *req.ShowFollows
	}
	if req.Discoverable != nil {
		p.Discoverable = *req.Discoverable
	}
	p.UpdatedAt = time.Now()

	if err := h.store.UpdatePersona(ctx, p); err != nil {
		return nil, newRPCErr(codeInternalError, "update persona failed")
	}

	// Enqueue Update Actor delivery to followers.
	// フォロワーへの Update Actor 配送をキューに登録。
	h.queue.Enqueue(ctx, murlog.NewJob(murlog.JobUpdateActor, map[string]string{"persona_id": p.ID.String()}))

	base := h.baseURLFromCtx(ctx)
	return h.toAccountFromPersona(ctx, base, p), nil
}
