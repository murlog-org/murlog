package handler

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

// interactionJSON is the unified API representation of a favourite or reblog.
// お気に入り・リブログの統一 API 表現。
type interactionJSON struct {
	ID        string `json:"id"`
	PostID    string `json:"post_id"`
	ActorURI  string `json:"actor_uri"`
	CreatedAt string `json:"created_at"`
}

func interactionFromFavourite(f *murlog.Favourite) interactionJSON {
	return interactionJSON{
		ID:        f.ID.String(),
		PostID:    f.PostID.String(),
		ActorURI:  f.ActorURI,
		CreatedAt: f.CreatedAt.Format(time.RFC3339),
	}
}

func interactionFromReblog(r *murlog.Reblog) interactionJSON {
	return interactionJSON{
		ID:        r.ID.String(),
		PostID:    r.PostID.String(),
		ActorURI:  r.ActorURI,
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
	}
}

// interactionParams is used for favourites and reblogs create/delete.
// お気に入りとリブログの create/delete 共通パラメータ。
type interactionParams struct {
	PersonaID string `json:"persona_id,omitempty"`
	PostID    string `json:"post_id"`
}

// rpcFavouritesCreate creates a favourite (like) on a post and enqueues delivery.
// 投稿にお気に入り (Like) を作成し、配送をキューに追加。
func (h *Handler) rpcFavouritesCreate(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[interactionParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.PostID == "" {
		return nil, newRPCErr(codeInvalidParams, "post_id is required")
	}

	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}

	postID, err := id.Parse(req.PostID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid post_id")
	}

	post, err := h.store.GetPost(ctx, postID)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "post not found")
	}

	persona, err := h.store.GetPersona(ctx, personaID)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	base := h.baseURLFromCtx(ctx)
	localActorURI := base + "/users/" + persona.Username

	now := time.Now()
	fav := &murlog.Favourite{
		ID:        id.New(),
		PostID:    post.ID,
		ActorURI:  localActorURI,
		CreatedAt: now,
	}
	if err := h.store.CreateFavourite(ctx, fav); err != nil {
		return nil, newRPCErr(codeConflict, "already favourited")
	}

	// Enqueue Like delivery for remote posts.
	// リモート投稿の場合、Like 配送をキューに追加。
	if post.Origin == "remote" && post.ActorURI != "" {
		job := murlog.NewJob(murlog.JobSendLike, map[string]string{
				"persona_id":       personaID.String(),
				"post_uri":         post.URI,
				"target_actor_uri": post.ActorURI,
			})
		h.queue.Enqueue(ctx, job)
	}

	return interactionFromFavourite(fav), nil
}

// rpcFavouritesDelete removes a favourite (like) and enqueues Undo delivery.
// お気に入り (Like) を削除し、Undo 配送をキューに追加。
func (h *Handler) rpcFavouritesDelete(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[interactionParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.PostID == "" {
		return nil, newRPCErr(codeInvalidParams, "post_id is required")
	}

	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}

	postID, err := id.Parse(req.PostID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid post_id")
	}

	post, err := h.store.GetPost(ctx, postID)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "post not found")
	}

	persona, err := h.store.GetPersona(ctx, personaID)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	base := h.baseURLFromCtx(ctx)
	localActorURI := base + "/users/" + persona.Username

	if err := h.store.DeleteFavourite(ctx, post.ID, localActorURI); err != nil {
		return nil, newRPCErr(codeNotFound, "favourite not found")
	}

	// Enqueue Undo Like delivery for remote posts.
	// リモート投稿の場合、Undo Like 配送をキューに追加。
	if post.Origin == "remote" && post.ActorURI != "" {
		job := murlog.NewJob(murlog.JobSendUndoLike, map[string]string{
				"persona_id":       personaID.String(),
				"post_uri":         post.URI,
				"target_actor_uri": post.ActorURI,
			})
		h.queue.Enqueue(ctx, job)
	}

	return statusOK, nil
}

// rpcReblogsCreate creates a reblog (reblog) on a post and enqueues delivery.
// 投稿にリブログ (リブログ) を作成し、配送をキューに追加。
func (h *Handler) rpcReblogsCreate(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[interactionParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.PostID == "" {
		return nil, newRPCErr(codeInvalidParams, "post_id is required")
	}

	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}

	postID, err := id.Parse(req.PostID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid post_id")
	}

	post, err := h.store.GetPost(ctx, postID)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "post not found")
	}

	persona, err := h.store.GetPersona(ctx, personaID)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	base := h.baseURLFromCtx(ctx)
	localActorURI := base + "/users/" + persona.Username

	// Resolve post URI: remote posts have URI, local posts construct it.
	// 投稿 URI を解決: リモートは URI あり、ローカルは構築。
	postURI := post.URI
	if postURI == "" {
		postURI = base + "/users/" + persona.Username + "/posts/" + post.ID.String()
	}

	now := time.Now()
	reblog := &murlog.Reblog{
		ID:        id.New(),
		PostID:    post.ID,
		ActorURI:  localActorURI,
		CreatedAt: now,
	}
	if err := h.store.CreateReblog(ctx, reblog); err != nil {
		return nil, newRPCErr(codeConflict, "already reblogged")
	}

	// Create a wrapper post for the profile timeline (empty content, references original).
	// プロフィールタイムライン用に wrapper post を作成 (内容は空、元投稿を参照)。
	wrapperPost := &murlog.Post{
		ID:             id.New(),
		PersonaID:      persona.ID,
		Visibility:     murlog.VisibilityPublic,
		Origin:         "local",
		ReblogOfPostID: post.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := h.store.CreatePost(ctx, wrapperPost); err != nil {
		log.Printf("reblogs.create: wrapper post の作成に失敗 / failed to create wrapper post: %v", err)
	}

	// Enqueue Announce delivery (to followers + post author).
	// Announce 配送をキューに追加 (フォロワー + 投稿者)。
	targetActorURI := post.ActorURI
	if targetActorURI == "" {
		targetActorURI = localActorURI
	}

	job := murlog.NewJob(murlog.JobSendAnnounce, map[string]string{
			"persona_id":       personaID.String(),
			"post_uri":         postURI,
			"target_actor_uri": targetActorURI,
		})
	h.queue.Enqueue(ctx, job)

	return interactionFromReblog(reblog), nil
}

// rpcReblogsDelete removes a reblog (reblog) and enqueues Undo delivery.
// リブログ (リブログ) を削除し、Undo 配送をキューに追加。
func (h *Handler) rpcReblogsDelete(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[interactionParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.PostID == "" {
		return nil, newRPCErr(codeInvalidParams, "post_id is required")
	}

	personaID, rErr := h.resolvePersonaID(ctx, req.PersonaID)
	if rErr != nil {
		return nil, rErr
	}

	postID, err := id.Parse(req.PostID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid post_id")
	}

	post, err := h.store.GetPost(ctx, postID)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "post not found")
	}

	persona, err := h.store.GetPersona(ctx, personaID)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	base := h.baseURLFromCtx(ctx)
	localActorURI := base + "/users/" + persona.Username

	if err := h.store.DeleteReblog(ctx, post.ID, localActorURI); err != nil {
		return nil, newRPCErr(codeNotFound, "reblog not found")
	}

	// Delete the wrapper post from the profile timeline.
	// プロフィールタイムラインから wrapper post を削除。
	if err := h.store.DeleteReblogPost(ctx, persona.ID, post.ID); err != nil {
		log.Printf("reblogs.delete: wrapper post の削除に失敗 / failed to delete wrapper post: %v", err)
	}

	// Resolve post URI. / 投稿 URI を解決。
	postURI := post.URI
	if postURI == "" {
		postURI = base + "/users/" + persona.Username + "/posts/" + post.ID.String()
	}

	targetActorURI := post.ActorURI
	if targetActorURI == "" {
		targetActorURI = localActorURI
	}

	job := murlog.NewJob(murlog.JobSendUndoAnnounce, map[string]string{
			"persona_id":       personaID.String(),
			"post_uri":         postURI,
			"target_actor_uri": targetActorURI,
		})
	h.queue.Enqueue(ctx, job)

	return statusOK, nil
}
