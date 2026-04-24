package handler

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

// followJSON is the API representation of a Follow.
type followJSON struct {
	ID          string `json:"id"`
	PersonaID   string `json:"persona_id"`
	TargetURI   string `json:"target_uri"`
	Acct        string `json:"acct,omitempty"`
	DisplayName string `json:"display_name,omitempty"` // 表示名 / display name
	AvatarURL   string `json:"avatar_url,omitempty"`   // アバター / avatar
	Summary     string `json:"summary,omitempty"`      // プロフィール / bio
	Accepted    bool   `json:"accepted"`
	CreatedAt   string `json:"created_at"`
}

func (h *Handler) toFollowJSON(ctx context.Context, f *murlog.Follow) followJSON {
	fj := followJSON{
		ID:        f.ID.String(),
		PersonaID: f.PersonaID.String(),
		TargetURI: f.TargetURI,
		Accepted:  f.Accepted,
		CreatedAt: f.CreatedAt.Format(time.RFC3339),
	}
	if ra := h.resolveRemoteActorByURI(ctx, f.TargetURI); ra != nil {
		fj.Acct = formatAcct(ra)
		fj.DisplayName = ra.DisplayName
		fj.AvatarURL = ra.AvatarURL
		fj.Summary = ra.Summary
	}
	return fj
}

// followerJSON is the API representation of a Follower.
type followerJSON struct {
	ID          string `json:"id"`
	PersonaID   string `json:"persona_id"`
	ActorURI    string `json:"actor_uri"`
	Acct        string `json:"acct,omitempty"`
	DisplayName string `json:"display_name,omitempty"` // 表示名 / display name
	AvatarURL   string `json:"avatar_url,omitempty"`   // アバター / avatar
	Summary     string `json:"summary,omitempty"`      // プロフィール / bio
	CreatedAt   string `json:"created_at"`
}

func (h *Handler) toFollowerJSON(ctx context.Context, f *murlog.Follower) followerJSON {
	fj := followerJSON{
		ID:        f.ID.String(),
		PersonaID: f.PersonaID.String(),
		ActorURI:  f.ActorURI,
		CreatedAt: f.CreatedAt.Format(time.RFC3339),
	}
	if ra := h.resolveRemoteActorByURI(ctx, f.ActorURI); ra != nil {
		fj.Acct = formatAcct(ra)
		fj.DisplayName = ra.DisplayName
		fj.AvatarURL = ra.AvatarURL
		fj.Summary = ra.Summary
	}
	return fj
}

// resolveRemoteActorByURI returns the cached remote actor for a given URI.
// 指定 URI のキャッシュ済みリモート Actor を返す。
func (h *Handler) resolveRemoteActorByURI(ctx context.Context, actorURI string) *murlog.RemoteActor {
	ra, err := h.store.GetRemoteActor(ctx, actorURI)
	if err != nil || ra == nil {
		return nil
	}
	return ra
}

// formatAcct formats a RemoteActor as @user@host.
// RemoteActor を @user@host 形式にフォーマットする。
func formatAcct(ra *murlog.RemoteActor) string {
	if ra.Acct != "" {
		return "@" + ra.Acct
	}
	if ra.Username != "" {
		if u, err := url.Parse(ra.URI); err == nil {
			return "@" + ra.Username + "@" + u.Host
		}
	}
	return ""
}

// --- follows ---

type followsCheckParams struct {
	TargetURI string `json:"target_uri"`
}

// rpcFollowsCheck checks if the current user follows a target actor.
// 現在のユーザーが対象 Actor をフォローしているか確認する。
func (h *Handler) rpcFollowsCheck(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[followsCheckParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.TargetURI == "" {
		return nil, newRPCErr(codeInvalidParams, "target_uri is required")
	}

	personaID, rErr := h.resolvePersonaID(ctx, "")
	if rErr != nil {
		return nil, rErr
	}

	f, err := h.store.GetFollowByTarget(ctx, personaID, req.TargetURI)
	if err != nil || f == nil {
		return nil, nil
	}
	return h.toFollowJSON(ctx, f), nil
}

type followsListParams struct {
	PersonaID string `json:"persona_id,omitempty"`
}

func (h *Handler) rpcFollowsList(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[followsListParams](params)
	if rErr != nil {
		return nil, rErr
	}

	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}

	// 未認証かつ show_follows 無効なら空リストを返す。
	// Return empty list if unauthenticated and show_follows is disabled.
	if !h.isRPCAuthed(ctx) {
		persona, err := h.store.GetPersona(ctx, personaID)
		if err != nil || !persona.ShowFollows {
			return []followJSON{}, nil
		}
	}

	follows, err := h.store.ListFollows(ctx, personaID)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	out := make([]followJSON, len(follows))
	for i, f := range follows {
		out[i] = h.toFollowJSON(ctx, f)
	}
	return out, nil
}

type followsCreateParams struct {
	PersonaID string `json:"persona_id,omitempty"`
	TargetURI string `json:"target_uri"`
}

func (h *Handler) rpcFollowsCreate(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[followsCreateParams](params)
	if rErr != nil {
		return nil, rErr
	}

	if req.TargetURI == "" {
		return nil, newRPCErr(codeInvalidParams, "target_uri is required")
	}

	// Reject follow if the target is blocked.
	// ブロック済みアクターへのフォローを拒否。
	if blocked, _ := h.store.IsBlocked(ctx, req.TargetURI); blocked {
		return nil, newRPCErr(codeConflict, "actor is blocked")
	}

	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}

	now := time.Now()
	f := &murlog.Follow{
		ID:        id.New(),
		PersonaID: personaID,
		TargetURI: req.TargetURI,
		Accepted:  false,
		CreatedAt: now,
	}
	if err := h.store.CreateFollow(ctx, f); err != nil {
		return nil, newRPCErr(codeConflict, "already following")
	}

	// Enqueue follow request delivery to remote actor.
	// リモート Actor にフォローリクエストを配送するジョブを追加。
	job := murlog.NewJob(murlog.JobSendFollow, map[string]string{
			"follow_id": f.ID.String(),
		})
	h.queue.Enqueue(ctx, job)

	return h.toFollowJSON(ctx, f), nil
}

type followsDeleteParams struct {
	ID string `json:"id"`
}

func (h *Handler) rpcFollowsDelete(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[followsDeleteParams](params)
	if rErr != nil {
		return nil, rErr
	}

	fid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	// Get follow before deleting (need target_uri for Undo delivery).
	// 削除前にフォロー情報を取得 (Undo 配送に target_uri が必要)。
	follow, err := h.store.GetFollow(ctx, fid)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "not found")
	}

	if err := h.store.DeleteFollow(ctx, fid); err != nil {
		return nil, newRPCErr(codeInternalError, "delete follow failed")
	}

	// Enqueue Undo Follow delivery to remote actor.
	// リモート Actor への Undo Follow 配送をキューに追加。
	job := murlog.NewJob(murlog.JobSendUndoFollow, map[string]string{
			"persona_id": follow.PersonaID.String(),
			"follow_id":  follow.ID.String(),
			"target_uri": follow.TargetURI,
		})
	h.queue.Enqueue(ctx, job)

	return statusOK, nil
}

// --- followers ---

type followersListParams struct {
	PersonaID string `json:"persona_id,omitempty"`
}

func (h *Handler) rpcFollowersList(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[followersListParams](params)
	if rErr != nil {
		return nil, rErr
	}

	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}

	// 未認証かつ show_follows 無効なら空リストを返す。
	// Return empty list if unauthenticated and show_follows is disabled.
	if !h.isRPCAuthed(ctx) {
		persona, err := h.store.GetPersona(ctx, personaID)
		if err != nil || !persona.ShowFollows {
			return []followerJSON{}, nil
		}
	}

	followers, err := h.store.ListFollowers(ctx, personaID)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	out := make([]followerJSON, len(followers))
	for i, f := range followers {
		out[i] = h.toFollowerJSON(ctx, f)
	}
	return out, nil
}

type followersDeleteParams struct {
	ID string `json:"id"`
}

func (h *Handler) rpcFollowersDelete(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[followersDeleteParams](params)
	if rErr != nil {
		return nil, rErr
	}

	fid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	if err := h.store.DeleteFollower(ctx, fid); err != nil {
		return nil, newRPCErr(codeNotFound, "not found")
	}
	return statusOK, nil
}

// rpcFollowersPending returns unapproved follow requests.
// 未承認のフォローリクエスト一覧を返す。
func (h *Handler) rpcFollowersPending(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[followersListParams](params)
	if rErr != nil {
		return nil, rErr
	}
	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}
	followers, err := h.store.ListPendingFollowers(ctx, personaID)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	out := make([]followerJSON, len(followers))
	for i, f := range followers {
		out[i] = h.toFollowerJSON(ctx, f)
	}
	return out, nil
}

type followerActionParams struct {
	ID         string `json:"id"`
	ActivityID string `json:"activity_id,omitempty"`
}

// rpcFollowersApprove approves a pending follow request and enqueues Accept delivery.
// 保留中のフォローリクエストを承認し、Accept 配送をキューに追加する。
func (h *Handler) rpcFollowersApprove(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[followerActionParams](params)
	if rErr != nil {
		return nil, rErr
	}
	fid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}
	f, err := h.store.GetFollower(ctx, fid)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "follower not found")
	}
	if err := h.store.ApproveFollower(ctx, fid); err != nil {
		return nil, newRPCErr(codeInternalError, "approve failed")
	}
	// Enqueue Accept delivery.
	// Accept 配送をキューに追加。
	persona, _ := h.store.GetPersona(ctx, f.PersonaID)
	if persona != nil {
		job := murlog.NewJob(murlog.JobAcceptFollow, map[string]string{
				"persona_id":  persona.ID.String(),
				"activity_id": req.ActivityID,
				"actor_uri":   f.ActorURI,
			})
		h.queue.Enqueue(ctx, job)
	}
	return statusOK, nil
}

// rpcFollowersReject rejects a pending follow request and enqueues Reject delivery.
// 保留中のフォローリクエストを拒否し、Reject 配送をキューに追加する。
func (h *Handler) rpcFollowersReject(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[followerActionParams](params)
	if rErr != nil {
		return nil, rErr
	}
	fid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}
	f, err := h.store.GetFollower(ctx, fid)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "follower not found")
	}
	if err := h.store.DeleteFollower(ctx, fid); err != nil {
		return nil, newRPCErr(codeInternalError, "reject failed")
	}
	// Enqueue Reject delivery.
	// Reject 配送をキューに追加。
	persona, _ := h.store.GetPersona(ctx, f.PersonaID)
	if persona != nil {
		job := murlog.NewJob(murlog.JobRejectFollow, map[string]string{
				"persona_id":  persona.ID.String(),
				"activity_id": req.ActivityID,
				"actor_uri":   f.ActorURI,
			})
		h.queue.Enqueue(ctx, job)
	}
	return statusOK, nil
}

// --- helpers ---

// resolvePersonaID parses a persona_id string, defaulting to primary persona.
func (h *Handler) resolvePersonaID(ctx context.Context, raw string) (id.ID, *rpcErr) {
	if raw != "" {
		pid, err := id.Parse(raw)
		if err != nil {
			return id.ID{}, newRPCErr(codeInvalidParams, "invalid persona_id")
		}
		return pid, nil
	}
	personas, err := h.store.ListPersonas(ctx)
	if err != nil || len(personas) == 0 {
		return id.ID{}, newRPCErr(codeInternalError, "internal error")
	}
	return personas[0].ID, nil
}

// statusOK is a reusable success response.
var statusOK = map[string]string{"status": "ok"}
