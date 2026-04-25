package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/activitypub"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/internal/mediautil"
	"github.com/murlog-org/murlog/mention"
)

// isActivityPubRequest checks if the client wants ActivityPub JSON.
// クライアントが ActivityPub JSON を要求しているか判定する。
func isActivityPubRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/activity+json") ||
		strings.Contains(accept, "application/ld+json")
}

// handleActor serves the Actor JSON-LD or public profile HTML.
// Actor JSON-LD または公開プロフィール HTML を返す。
//
// Content Negotiation:
//   - Accept: application/activity+json → JSON-LD
//   - Otherwise → SSR HTML (profile + posts)
func (h *Handler) handleActor(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")

	// Remote actor acct (e.g. @bob@remote.example) → redirect to /my/users/ if logged in, else 404.
	// リモート Actor の acct (例: @bob@remote.example) → ログイン済みなら /my/users/ にリダイレクト、未ログインなら 404。
	if strings.Contains(username, "@") {
		if h.isHTTPAuthed(r) {
			http.Redirect(w, r, "/my/users/"+username, http.StatusSeeOther)
		} else {
			renderNotFound(w)
		}
		return
	}

	persona, err := h.store.GetPersonaByUsername(r.Context(), username)
	if err != nil || persona == nil {
		if isActivityPubRequest(r) {
			http.Error(w, "not found", http.StatusNotFound)
		} else {
			renderNotFound(w)
		}
		return
	}

	// Vary: Accept ensures CDN/proxies cache HTML and JSON-LD separately.
	// Vary: Accept で CDN/プロキシが HTML と JSON-LD を別々にキャッシュする。
	w.Header().Set("Vary", "Accept")

	if !isActivityPubRequest(r) {
		h.renderProfile(w, r, persona)
		return
	}

	base := h.baseURL(r)
	actor := activitypub.BuildLocalActor(persona, base, func(path string) string {
		return h.resolveMediaURL(base, path)
	})
	// AP summary should be HTML (other servers expect it).
	// AP の summary は HTML であるべき (他サーバーがそう期待する)。
	actor.Summary = h.formatBio(r.Context(), persona.Summary)

	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	w.Header().Set("Cache-Control", "max-age=300, public")
	json.NewEncoder(w).Encode(actor)
}

// handleInbox receives ActivityPub activities from remote servers.
// リモートサーバーからの ActivityPub Activity を受信する。
func (h *Handler) handleInbox(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")

	persona, err := h.store.GetPersonaByUsername(r.Context(), username)
	if err != nil || persona == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Read body for signature verification + JSON decode (limit 1 MB).
	// 署名検証と JSON デコードのためにボディを読む (上限 1 MB)。
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	r.Body.Close()
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Inbox POST requires Digest header. / Inbox POST には Digest ヘッダーが必須。
	if r.Header.Get("Digest") == "" {
		log.Printf("inbox: missing Digest header")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := activitypub.VerifyDigest(r, body); err != nil {
		log.Printf("inbox: digest verification failed: %v", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify HTTP Signature. / HTTP Signature を検証する。
	signerURI, err := h.verifyInboxSignature(r, body, persona)
	if err != nil {
		log.Printf("inbox: signature verification failed: %v", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var activity activitypub.Activity
	if err := json.Unmarshal(body, &activity); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Verify that the signer matches activity.actor (prevent actor spoofing).
	// 署名者と activity.actor の一致を検証 (Actor なりすまし防止)。
	if activity.Actor != signerURI {
		log.Printf("inbox: actor/signer mismatch: %s != %s", activity.Actor, signerURI)
		http.Error(w, "unauthorized: actor does not match signer", http.StatusUnauthorized)
		return
	}

	// Block check: reject activities from blocked actors/domains.
	// Block and Undo are always accepted (to process incoming Block/Undo Block).
	// ブロックチェック: ブロック済み Actor/ドメインからの Activity を拒否。
	// Block と Undo は常に受け付ける (受信 Block/Undo Block の処理のため)。
	if activity.Type != "Block" && activity.Type != "Undo" {
		if blocked, _ := h.store.IsBlocked(r.Context(), activity.Actor); blocked {
			log.Printf("inbox: blocked actor rejected: %s", activity.Actor)
			w.WriteHeader(http.StatusAccepted)
			return
		}
	}

	switch activity.Type {
	case "Follow":
		h.handleInboxFollow(w, r, persona, &activity)
	case "Undo":
		h.handleInboxUndo(w, r, persona, &activity)
	case "Create":
		h.handleInboxCreate(w, r, persona, &activity)
	case "Like":
		h.handleInboxLike(w, r, persona, &activity)
	case "Announce":
		h.handleInboxAnnounce(w, r, persona, &activity)
	case "Block":
		h.handleInboxBlock(w, r, persona, &activity)
	case "Delete":
		h.handleInboxDelete(w, r, persona, &activity)
	case "Update":
		h.handleInboxUpdate(w, r, persona, &activity)
	case "Accept":
		h.handleInboxAccept(w, r, persona, &activity)
	case "Reject":
		h.handleInboxReject(w, r, persona, &activity)
	default:
		// Unknown activity type — accept but ignore.
		// 未知の Activity タイプ — 受け入れるが無視する。
		w.WriteHeader(http.StatusAccepted)
	}
}

// handleInboxFollow processes an incoming Follow activity.
// Auto-accepts and enqueues Accept delivery.
// 受信した Follow Activity を処理する。自動承認し、Accept 配送をキューに追加。
func (h *Handler) handleInboxFollow(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, activity *activitypub.Activity) {
	ctx := r.Context()
	actorURI := activity.Actor
	now := time.Now()

	// Ensure the remote actor is cached for display (name, avatar, etc.).
	// 表示用にリモート Actor をキャッシュ (名前、アバター等)。
	h.ensureRemoteActorCached(ctx, actorURI, persona)

	// Create follower record. Approved immediately if persona is not locked.
	// フォロワーレコードを作成。ペルソナがロックされていなければ即承認。
	follower := &murlog.Follower{
		ID:        id.New(),
		PersonaID: persona.ID,
		ActorURI:  actorURI,
		Approved:  !persona.Locked,
		CreatedAt: now,
	}
	if err := h.store.CreateFollower(ctx, follower); err != nil {
		// May already exist (duplicate follow). / 重複フォローの可能性。
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Create follow notification. / フォロー通知を作成。
	h.store.CreateNotification(ctx, &murlog.Notification{
		ID:        id.New(),
		PersonaID: persona.ID,
		Type:      "follow",
		ActorURI:  actorURI,
		CreatedAt: now,
	})

	// If not locked, enqueue Accept delivery immediately.
	// ロックされていなければ、即座に Accept 配送をキューに追加。
	if !persona.Locked {
		job := murlog.NewJob(murlog.JobAcceptFollow, map[string]string{
				"persona_id":  persona.ID.String(),
				"activity_id": activity.ID,
				"actor_uri":   actorURI,
			})
		h.queue.Enqueue(ctx, job)
	}

	w.WriteHeader(http.StatusAccepted)
}

// handleInboxAccept processes an incoming Accept activity.
// Marks the corresponding outgoing Follow as accepted.
// 受信した Accept Activity を処理する。送信フォローを承認済みにする。
func (h *Handler) handleInboxAccept(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, activity *activitypub.Activity) {
	ctx := r.Context()

	// Match by the actor who sent the Accept (= the follow target).
	// Accept を送った Actor (= フォロー先) で照合。
	f, err := h.store.GetFollowByTarget(ctx, persona.ID, activity.Actor)
	if err == nil && !f.Accepted {
		f.Accepted = true
		h.store.UpdateFollow(ctx, f)
	}

	w.WriteHeader(http.StatusAccepted)
}

// handleInboxReject processes an incoming Reject activity.
// Removes the corresponding outgoing Follow (rejected by remote).
// 受信した Reject Activity を処理する。拒否されたフォローを削除する。
func (h *Handler) handleInboxReject(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, activity *activitypub.Activity) {
	ctx := r.Context()

	f, err := h.store.GetFollowByTarget(ctx, persona.ID, activity.Actor)
	if err == nil {
		h.store.DeleteFollow(ctx, f.ID)
	}

	w.WriteHeader(http.StatusAccepted)
}

// handleInboxUndo processes an incoming Undo activity.
// Handles Undo Follow, Undo Like, and Undo Announce.
// 受信した Undo Activity を処理する。Undo Follow / Like / Announce に対応。
func (h *Handler) handleInboxUndo(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, activity *activitypub.Activity) {
	ctx := r.Context()

	// Parse the inner object to determine what is being undone.
	// 内部オブジェクトをパースして何が取り消されるか判定する。
	inner, ok := activity.Object.(map[string]interface{})
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch inner["type"] {
	case "Follow":
		// Delete the follower by actor URI (O(1) via UNIQUE index).
		// Actor URI でフォロワーを削除 (UNIQUE インデックスで O(1))。
		h.store.DeleteFollowerByActorURI(ctx, persona.ID, activity.Actor)
		// Delete follow notification. / フォロー通知を削除。
		h.store.DeleteNotificationByActor(ctx, persona.ID, activity.Actor, "follow", id.ID{})
	case "Like":
		// Undo Like → delete favourite record and notification.
		// Undo Like → お気に入りレコードと通知を削除。
		if objectURI := extractObjectURI(inner); objectURI != "" {
			if post := h.resolveLocalPost(ctx, objectURI); post != nil {
				h.store.DeleteFavourite(ctx, post.ID, activity.Actor)
				h.store.DeleteNotificationByActor(ctx, persona.ID, activity.Actor, "favourite", post.ID)
			}
		}
	case "Announce":
		// Undo Announce → delete reblog record and notification.
		// Undo Announce → リブログレコードと通知を削除。
		if objectURI := extractObjectURI(inner); objectURI != "" {
			if post := h.resolveLocalPost(ctx, objectURI); post != nil {
				h.store.DeleteReblog(ctx, post.ID, activity.Actor)
				h.store.DeleteNotificationByActor(ctx, persona.ID, activity.Actor, "reblog", post.ID)
			}
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

// handleInboxLike processes an incoming Like activity.
// Creates a favourite record and a notification.
// 受信した Like Activity を処理する。お気に入りレコードと通知を作成。
func (h *Handler) handleInboxLike(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, activity *activitypub.Activity) {
	ctx := r.Context()

	// Extract the liked object URI. / いいねされたオブジェクトの URI を取得。
	objectURI := extractActivityObjectURI(activity)
	if objectURI == "" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Look up the local post by URI or path-embedded ID.
	// URI またはパス埋め込み ID でローカル投稿を検索。
	post := h.resolveLocalPost(ctx, objectURI)
	if post == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Ensure the remote actor is cached for display (name, avatar, etc.).
	// 表示用にリモート Actor をキャッシュ (名前、アバター等)。
	h.ensureRemoteActorCached(ctx, activity.Actor, persona)

	// Create favourite record (idempotent via UNIQUE constraint).
	// お気に入りレコードを作成 (UNIQUE 制約で冪等)。
	now := time.Now()
	h.store.CreateFavourite(ctx, &murlog.Favourite{
		ID:        id.New(),
		PostID:    post.ID,
		ActorURI:  activity.Actor,
		CreatedAt: now,
	})

	// Create notification. / 通知を作成。
	h.store.CreateNotification(ctx, &murlog.Notification{
		ID:        id.New(),
		PersonaID: post.PersonaID,
		Type:      "favourite",
		ActorURI:  activity.Actor,
		PostID:    post.ID,
		CreatedAt: now,
	})

	w.WriteHeader(http.StatusAccepted)
}

// handleInboxAnnounce processes an incoming Announce activity.
// Creates a reblog record and notification for local posts,
// or fetches and stores the remote Note for non-local posts.
// 受信した Announce Activity を処理する。
// ローカル投稿にはリブログレコードと通知を作成し、
// リモート投稿はフェッチしてタイムラインに保存する。
func (h *Handler) handleInboxAnnounce(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, activity *activitypub.Activity) {
	ctx := r.Context()

	// Extract the announced object URI. / リブログされたオブジェクトの URI を取得。
	objectURI := extractActivityObjectURI(activity)
	if objectURI == "" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Local post path: create reblog record + notification.
	// ローカル投稿パス: リブログレコード + 通知を作成。
	post := h.resolveLocalPost(ctx, objectURI)
	if post != nil {
		// Ensure the remote actor is cached for display (name, avatar, etc.).
		// 表示用にリモート Actor をキャッシュ (名前、アバター等)。
		h.ensureRemoteActorCached(ctx, activity.Actor, persona)

		now := time.Now()
		h.store.CreateReblog(ctx, &murlog.Reblog{
			ID:        id.New(),
			PostID:    post.ID,
			ActorURI:  activity.Actor,
			CreatedAt: now,
		})
		h.store.CreateNotification(ctx, &murlog.Notification{
			ID:        id.New(),
			PersonaID: post.PersonaID,
			Type:      "reblog",
			ActorURI:  activity.Actor,
			PostID:    post.ID,
			CreatedAt: now,
		})
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Non-local post: fetch the remote Note and store as a reblogged post.
	// リモート投稿: Note をフェッチしてリブログ投稿として保存。

	// Idempotent: skip if already received. / 冪等: 既に受信済みならスキップ。
	if _, err := h.store.GetPostByURI(ctx, objectURI); err == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Fetch the remote Note with signed HTTP request.
	// 署名付き HTTP リクエストでリモート Note を取得。
	base := h.baseURL(r)
	keyID := base + "/users/" + persona.Username + "#main-key"
	noteObj, err := activitypub.FetchNoteSigned(objectURI, keyID, persona.PrivateKeyPEM)
	if err != nil {
		log.Printf("inbox: リブログ元 Note の取得に失敗 / failed to fetch announced note: %s: %v", objectURI, err)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Store as a remote post with reblogged_by_uri.
	// reblogged_by_uri を設定してリモート投稿として保存。
	h.storeRemoteNote(r, persona, noteObj, activity.Actor, activity.Actor)

	w.WriteHeader(http.StatusAccepted)
}

// handleInboxBlock processes an incoming Block activity.
// Removes bidirectional follow relationships with the blocking actor.
// 受信した Block Activity を処理する。ブロック元 Actor との双方向フォロー関係を削除。
func (h *Handler) handleInboxBlock(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, activity *activitypub.Activity) {
	ctx := r.Context()

	// Delete follower record (remote → local). / フォロワーレコードを削除。
	h.store.DeleteFollowerByActorURI(ctx, persona.ID, activity.Actor)

	// Delete follow record (local → remote) if exists. / フォローレコードを削除。
	if f, err := h.store.GetFollowByTarget(ctx, persona.ID, activity.Actor); err == nil {
		h.store.DeleteFollow(ctx, f.ID)
	}

	w.WriteHeader(http.StatusAccepted)
}

// resolveLocalPost looks up a local post by its URI.
// First tries GetPostByURI (for posts with stored URI), then falls back
// to extracting the post ID from the URI path (/users/:name/posts/:id).
// URI からローカル投稿を検索する。
// まず GetPostByURI を試し、見つからなければ URI パスから投稿 ID を抽出してフォールバック。
func (h *Handler) resolveLocalPost(ctx context.Context, uri string) *murlog.Post {
	// Try direct URI lookup (works for remote posts stored with URI).
	// 直接 URI 検索 (URI 付きで保存されたリモート投稿用)。
	if post, err := h.store.GetPostByURI(ctx, uri); err == nil && post.Origin == "local" {
		return post
	}

	// Fall back: extract post ID from local URI path.
	// フォールバック: ローカル URI パスから投稿 ID を抽出。
	// Pattern: .../users/{username}/posts/{id}
	const postsSegment = "/posts/"
	idx := strings.LastIndex(uri, postsSegment)
	if idx < 0 {
		return nil
	}
	rawID := uri[idx+len(postsSegment):]
	postID, err := id.Parse(rawID)
	if err != nil {
		return nil
	}
	post, err := h.store.GetPost(ctx, postID)
	if err != nil || post.Origin != "local" {
		return nil
	}
	return post
}

// extractActivityObjectURI extracts the object URI from an Activity's Object field.
// Handles both string and map[string]interface{} forms.
// Activity の Object フィールドからオブジェクト URI を取得する。
// string と map の両形式に対応。
func extractActivityObjectURI(activity *activitypub.Activity) string {
	switch obj := activity.Object.(type) {
	case string:
		return obj
	case map[string]interface{}:
		uri, _ := obj["id"].(string)
		return uri
	}
	return ""
}

// extractObjectURI extracts the "object" field from an inner activity map.
// Handles both string and map[string]interface{} forms.
// 内部 Activity マップの "object" フィールドから URI を取得する。
// string と map の両形式に対応。
func extractObjectURI(inner map[string]interface{}) string {
	switch obj := inner["object"].(type) {
	case string:
		return obj
	case map[string]interface{}:
		uri, _ := obj["id"].(string)
		return uri
	}
	return ""
}

// handleInboxCreate processes an incoming Create activity (typically a Note).
// 受信した Create Activity (通常は Note) を処理する。
func (h *Handler) handleInboxCreate(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, activity *activitypub.Activity) {
	// Parse the inner object as a map to extract Note fields.
	// 内部オブジェクトを map としてパースして Note フィールドを取り出す。
	obj, ok := activity.Object.(map[string]interface{})
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	h.storeRemoteNote(r, persona, obj, activity.Actor, "")
	w.WriteHeader(http.StatusAccepted)
}

// storeRemoteNote parses a Note object map, sanitizes it, creates a Post record,
// and handles attachments, mentions, hashtags, and notifications.
// Note オブジェクトをパースし、サニタイズして Post レコードを作成。
// 添付・メンション・ハッシュタグ・通知を処理する。
func (h *Handler) storeRemoteNote(r *http.Request, persona *murlog.Persona, obj map[string]interface{}, activityActor string, rebloggedByURI string) *murlog.Post {
	ctx := r.Context()

	objType, _ := obj["type"].(string)
	if objType != "Note" {
		// Only handle Note for now. / 現時点では Note のみ処理。
		return nil
	}

	noteURI, _ := obj["id"].(string)
	if noteURI == "" {
		return nil
	}

	// Idempotent: skip if already received. / 冪等: 既に受信済みならスキップ。
	if _, err := h.store.GetPostByURI(ctx, noteURI); err == nil {
		return nil
	}

	// Sanitize and truncate remote content at storage time (defense in depth).
	// リモートコンテンツを保存時にサニタイズ・切り詰めする (多層防御)。
	rawContent, _ := obj["content"].(string)
	content := SanitizeContentHTML(rawContent)
	content = truncateRunes(content, MaxPostContentLength)
	actorURI, _ := obj["attributedTo"].(string)
	if actorURI == "" {
		actorURI = activityActor
	}

	published, _ := obj["published"].(string)
	pubTime := time.Now()
	if published != "" {
		if t, err := time.Parse(time.RFC3339, published); err == nil {
			pubTime = t
		}
	}

	// Parse and sanitize contentMap if present (max 50 languages).
	// contentMap があればパース・サニタイズ (最大50言語)。
	var contentMap map[string]string
	if cm, ok := obj["contentMap"].(map[string]interface{}); ok {
		contentMap = make(map[string]string, min(len(cm), maxContentMapLangs))
		for k, v := range cm {
			if len(contentMap) >= maxContentMapLangs {
				break
			}
			if s, ok := v.(string); ok {
				s = SanitizeContentHTML(s)
				if len([]rune(s)) > MaxPostContentLength {
					s = string([]rune(s)[:MaxPostContentLength])
				}
				contentMap[k] = s
			}
		}
	}

	// Parse inReplyTo if present. / inReplyTo があればパース。
	inReplyTo, _ := obj["inReplyTo"].(string)

	// Parse and sanitize summary (CW text) and sensitive flag.
	// summary (CW テキスト) をサニタイズし、sensitive フラグをパース。
	rawSummary, _ := obj["summary"].(string)
	summary := SanitizeContentHTML(rawSummary)
	sensitive, _ := obj["sensitive"].(bool)

	// Detect visibility from to/cc fields.
	// to/cc フィールドから公開範囲を判定。
	visibility := detectVisibility(obj)

	now := time.Now()
	post := &murlog.Post{
		ID:             id.New(),
		PersonaID:      persona.ID,
		Content:        content,
		ContentMap:     contentMap,
		Summary:        summary,
		Sensitive:      sensitive,
		Visibility:     visibility,
		Origin:         "remote",
		URI:            noteURI,
		ActorURI:       actorURI,
		InReplyToURI:   inReplyTo,
		RebloggedByURI: rebloggedByURI,
		CreatedAt:      pubTime,
		UpdatedAt:      now,
	}

	if err := h.store.CreatePost(ctx, post); err != nil {
		// May be a duplicate (race condition). / 重複かもしれない (競合)。
		return nil
	}

	// Track personas already notified to avoid duplicate notifications.
	// 重複通知を防ぐため、通知済みペルソナを追跡。
	notified := make(map[id.ID]bool)

	// Create mention notification if this is a reply to a local post.
	// ローカル投稿へのリプライなら mention 通知を生成。
	// ローカル投稿は URI カラムが空 (remote only) なので、GetPostByURI では見つからない。
	// inReplyTo URI からローカル投稿の ID をパースして GetPost で取得する。
	// Local posts have empty URI column (remote only), so GetPostByURI won't find them.
	// Parse the local post ID from the inReplyTo URI and use GetPost instead.
	if inReplyTo != "" {
		var parent *murlog.Post
		// まずリモート投稿として検索 / First try as a remote post
		parent, _ = h.store.GetPostByURI(ctx, inReplyTo)
		// ローカル投稿の場合: URI パターン {base}/users/{username}/posts/{id} からパース
		// For local posts: parse from URI pattern {base}/users/{username}/posts/{id}
		if parent == nil {
			base := h.baseURL(r)
			prefix := base + "/users/"
			if strings.HasPrefix(inReplyTo, prefix) {
				// "/users/{username}/posts/{id}" からIDを抽出
				// Extract ID from "/users/{username}/posts/{id}"
				parts := strings.Split(strings.TrimPrefix(inReplyTo, prefix), "/posts/")
				if len(parts) == 2 {
					if pid, err := id.Parse(parts[1]); err == nil {
						parent, _ = h.store.GetPost(ctx, pid)
					}
				}
			}
		}
		if parent != nil && parent.Origin == "local" {
			notified[parent.PersonaID] = true
			h.store.CreateNotification(ctx, &murlog.Notification{
				ID:        id.New(),
				PersonaID: parent.PersonaID,
				Type:      "mention",
				ActorURI:  actorURI,
				PostID:    post.ID,
				CreatedAt: now,
			})
		}
	}

	// Check tag array for direct mentions of local personas.
	// tag は配列または単一オブジェクトの場合がある (実装依存)。
	// tag 配列からローカルペルソナへの直接メンションを検出。
	var rawTags []interface{}
	if arr, ok := obj["tag"].([]interface{}); ok {
		if len(arr) > maxRemoteTags {
			arr = arr[:maxRemoteTags]
		}
		rawTags = arr
	} else if single, ok := obj["tag"].(map[string]interface{}); ok {
		// Single tag object (not wrapped in array).
		// 単一タグオブジェクト (配列でラップされていない場合)。
		rawTags = []interface{}{single}
	}
	if len(rawTags) > 0 {
		base := h.baseURL(r)
		personas, _ := h.store.ListPersonas(ctx)
		localActorURIs := make(map[string]*murlog.Persona, len(personas))
		for _, p := range personas {
			localActorURIs[base+"/users/"+p.Username] = p
		}

		var hashtags []string
		for _, rawTag := range rawTags {
			tag, ok := rawTag.(map[string]interface{})
			if !ok {
				continue
			}
			tagType, _ := tag["type"].(string)
			switch tagType {
			case "Mention":
				href, _ := tag["href"].(string)
				if href == "" {
					continue
				}
				if p, ok := localActorURIs[href]; ok && !notified[p.ID] {
					notified[p.ID] = true
					h.store.CreateNotification(ctx, &murlog.Notification{
						ID:        id.New(),
						PersonaID: p.ID,
						Type:      "mention",
						ActorURI:  actorURI,
						PostID:    post.ID,
						CreatedAt: now,
					})
				}
			case "Hashtag":
				name, _ := tag["name"].(string)
				name = strings.TrimPrefix(name, "#")
				if name != "" {
					hashtags = append(hashtags, strings.ToLower(name))
				}
			}
		}
		if len(hashtags) > 0 {
			post.SetHashtags(hashtags)
			h.store.UpdatePost(ctx, post)
		}
	}

	// Parse attachments from the Note (max 20). / Note から添付ファイルをパース (最大20)。
	if rawAtts, ok := obj["attachment"].([]interface{}); ok {
		attCount := 0
		for _, rawAtt := range rawAtts {
			if attCount >= maxRemoteAttachments {
				break
			}
			att, ok := rawAtt.(map[string]interface{})
			if !ok {
				continue
			}
			attType, _ := att["type"].(string)
			if attType != "Document" && attType != "Image" {
				continue
			}
			url, _ := att["url"].(string)
			if url == "" || !mention.IsSafeURL(url) {
				continue
			}
			mediaType, _ := att["mediaType"].(string)
			name, _ := att["name"].(string)
			width, _ := att["width"].(float64)
			height, _ := att["height"].(float64)

			a := &murlog.Attachment{
				ID:        id.New(),
				PostID:    post.ID,
				FilePath:  url, // リモート URL をそのまま保存 / store remote URL as-is
				MimeType:  mediaType,
				Alt:       name,
				Width:     int(width),
				Height:    int(height),
				CreatedAt: now,
			}
			h.store.CreateAttachment(ctx, a)
			attCount++
		}
	}

	// Cache remote actors for display (author and reblogger).
	// 表示用にリモート Actor をキャッシュ (投稿者とリブログ元)。
	h.ensureRemoteActorCached(ctx, post.ActorURI, persona)
	if rebloggedByURI != "" && rebloggedByURI != post.ActorURI {
		h.ensureRemoteActorCached(ctx, rebloggedByURI, persona)
	}

	return post
}

// ensureRemoteActorCached fetches and caches a remote Actor if not already cached.
// リモート Actor がキャッシュされていなければフェッチしてキャッシュする。
func (h *Handler) ensureRemoteActorCached(ctx context.Context, actorURI string, persona *murlog.Persona) {
	if actorURI == "" {
		return
	}
	// Skip if already cached with avatar. / アバター付きでキャッシュ済みならスキップ。
	if cached, err := h.store.GetRemoteActor(ctx, actorURI); err == nil && cached.AvatarURL != "" {
		return
	}

	base := h.baseURLFromCtx(ctx)
	keyID := base + "/users/" + persona.Username + "#main-key"
	actor, err := activitypub.FetchActorSigned(actorURI, keyID, persona.PrivateKeyPEM)
	if err != nil {
		log.Printf("inbox: Actor フェッチ失敗 / failed to fetch actor: %s: %v", actorURI, err)
		return
	}

	var acct string
	if actor.PreferredUsername != "" {
		if u, err := url.Parse(actor.ID); err == nil {
			acct = actor.PreferredUsername + "@" + u.Host
		}
	}

	ra := &murlog.RemoteActor{
		URI:          actor.ID,
		Username:     actor.PreferredUsername,
		DisplayName:  actor.Name,
		Summary:      actor.Summary,
		Inbox:        actor.Inbox,
		AvatarURL:    mediautil.ResolveActorIcon(actor),
		HeaderURL:    mediautil.ResolveActorHeader(actor),
		FieldsJSON:   murlog.MustJSON(activitypub.ResolveActorFields(actor)),
		Acct:         acct,
		FetchedAt:    time.Now(),
	}
	h.store.UpsertRemoteActor(ctx, ra)
}

// handleInboxUpdate processes an incoming Update activity (Note or Actor).
// 受信した Update Activity (Note または Actor) を処理する。
func (h *Handler) handleInboxUpdate(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, activity *activitypub.Activity) {
	ctx := r.Context()

	obj, ok := activity.Object.(map[string]interface{})
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	objType, _ := obj["type"].(string)
	switch objType {
	case "Note":
		h.handleInboxUpdateNote(ctx, obj)
	case "Person", "Service", "Application", "Organization", "Group":
		h.handleInboxUpdateActor(ctx, obj)
	}

	w.WriteHeader(http.StatusAccepted)
}

// handleInboxUpdateNote updates a previously received remote Note.
// 受信済みリモート Note を更新する。
func (h *Handler) handleInboxUpdateNote(ctx context.Context, obj map[string]interface{}) {
	noteURI, _ := obj["id"].(string)
	if noteURI == "" {
		return
	}

	post, err := h.store.GetPostByURI(ctx, noteURI)
	if err != nil || post.Origin != "remote" {
		return
	}

	// Sanitize updated content at storage time.
	// 更新コンテンツを保存時にサニタイズ。
	rawContent, _ := obj["content"].(string)
	post.Content = SanitizeContentHTML(rawContent)

	if cm, ok := obj["contentMap"].(map[string]interface{}); ok {
		contentMap := make(map[string]string, min(len(cm), maxContentMapLangs))
		for k, v := range cm {
			if len(contentMap) >= maxContentMapLangs {
				break
			}
			if s, ok := v.(string); ok {
				s = SanitizeContentHTML(s)
				if len([]rune(s)) > MaxPostContentLength {
					s = string([]rune(s)[:MaxPostContentLength])
				}
				contentMap[k] = s
			}
		}
		post.ContentMap = contentMap
	}

	post.UpdatedAt = time.Now()
	h.store.UpdatePost(ctx, post)
}

// handleInboxUpdateActor refreshes the cached remote Actor.
// キャッシュされたリモート Actor を更新する。
func (h *Handler) handleInboxUpdateActor(ctx context.Context, obj map[string]interface{}) {
	uri, _ := obj["id"].(string)
	if uri == "" {
		return
	}

	ra := &murlog.RemoteActor{
		URI:       uri,
		FetchedAt: time.Now(),
	}
	if v, ok := obj["preferredUsername"].(string); ok {
		ra.Username = v
		// Build acct from preferredUsername + URI host (e.g. "alice@mastodon.social").
		// preferredUsername + URI ホストから acct を組み立て。
		if u, err := url.Parse(uri); err == nil {
			ra.Acct = v + "@" + u.Host
		}
	}
	if v, ok := obj["name"].(string); ok {
		ra.DisplayName = truncateRunes(v, MaxDisplayNameLength)
	}
	if v, ok := obj["summary"].(string); ok {
		ra.Summary = truncateRunes(SanitizeContentHTML(v), MaxBioLength)
	}
	if v, ok := obj["inbox"].(string); ok {
		ra.Inbox = v
	}
	if icon, ok := obj["icon"].(map[string]interface{}); ok {
		if url, ok := icon["url"].(string); ok {
			ra.AvatarURL = url
		}
	}
	if image, ok := obj["image"].(map[string]interface{}); ok {
		if url, ok := image["url"].(string); ok {
			ra.HeaderURL = url
		}
	}
	if att, ok := obj["attachment"].([]interface{}); ok {
		var fields []murlog.CustomField
		for _, item := range att {
			if len(fields) >= maxRemoteActorFields {
				break
			}
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if t, _ := m["type"].(string); t != "PropertyValue" {
				continue
			}
			name, _ := m["name"].(string)
			value, _ := m["value"].(string)
			if name != "" {
				fields = append(fields, murlog.CustomField{Name: name, Value: value})
			}
		}
		ra.FieldsJSON = murlog.MustJSON(fields)
	}

	h.store.UpsertRemoteActor(ctx, ra)
}

// handleInboxDelete processes an incoming Delete activity.
// Deletes the corresponding remote post or removes the remote actor cache.
// 受信 Delete Activity を処理する。対応するリモート投稿を削除、またはリモート Actor キャッシュを除去。
func (h *Handler) handleInboxDelete(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, activity *activitypub.Activity) {
	ctx := r.Context()

	// Extract the deleted object URI.
	// 削除対象のオブジェクト URI を取り出す。
	var objectURI string
	switch obj := activity.Object.(type) {
	case string:
		objectURI = obj
	case map[string]interface{}:
		objectURI, _ = obj["id"].(string)
	}

	if objectURI == "" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// If the object URI matches the actor URI, this is an account deletion.
	// オブジェクト URI が Actor URI と一致する場合、アカウント削除。
	if objectURI == activity.Actor {
		h.store.DeleteFollowerByActorURI(ctx, persona.ID, activity.Actor)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Otherwise, treat as a post deletion — find and delete by URI.
	// それ以外は投稿削除として扱う — URI で検索して削除。
	post, err := h.store.GetPostByURI(ctx, objectURI)
	if err != nil || post == nil {
		// Not found — already deleted or never received.
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Only delete if it's a remote post belonging to this persona AND the sender is the author.
	// このペルソナに属するリモート投稿かつ送信者が投稿者本人のみ削除。
	if post.Origin == "remote" && post.PersonaID == persona.ID && post.ActorURI == activity.Actor {
		h.store.DeletePost(ctx, post.ID)
	}

	w.WriteHeader(http.StatusAccepted)
}

// verifyInboxSignature verifies the HTTP Signature on an incoming inbox request.
// Returns the verified signer's actor URI on success.
// 受信 Inbox リクエストの HTTP Signature を検証する。
// 成功時は署名者の Actor URI を返す。
func (h *Handler) verifyInboxSignature(r *http.Request, body []byte, persona *murlog.Persona) (string, error) {
	sigHeader := r.Header.Get("Signature")
	if sigHeader == "" {
		return "", fmt.Errorf("missing Signature header")
	}

	keyID, err := activitypub.ParseSignatureKeyID(sigHeader)
	if err != nil {
		return "", fmt.Errorf("invalid Signature header: %w", err)
	}

	// Derive actor URI from keyId.
	// keyId から Actor URI を導出。
	// Mastodon/Misskey: actor#main-key (フラグメント)
	// GoToSocial: actor/main-key (パスセグメント)
	actorURI := keyID
	if idx := strings.Index(keyID, "#"); idx >= 0 {
		actorURI = keyID[:idx]
	} else if strings.HasSuffix(keyID, "/main-key") {
		actorURI = strings.TrimSuffix(keyID, "/main-key")
	} else if strings.HasSuffix(keyID, "/publickey") {
		actorURI = strings.TrimSuffix(keyID, "/publickey")
	}

	// Fetch actor to get public key (signed request for GTS Authorized Fetch).
	// Actor の公開鍵を取得する（GTS の Authorized Fetch 対応で署名付きリクエスト）。
	base := h.baseURL(r)
	localKeyID := base + "/users/" + persona.Username + "#main-key"
	actor, err := activitypub.FetchActorSigned(actorURI, localKeyID, persona.PrivateKeyPEM)
	if err != nil {
		return "", fmt.Errorf("fetch actor %s: %w", actorURI, err)
	}

	if actor.PublicKey.PublicKeyPEM == "" {
		return "", fmt.Errorf("actor %s has no public key", actorURI)
	}

	// Rebuild request with body for digest verification.
	// ダイジェスト検証のためにボディ付きでリクエストを再構築。
	if body != nil {
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	}

	if err := activitypub.VerifyRequest(r, actor.PublicKey.PublicKeyPEM, body); err != nil {
		return "", fmt.Errorf("signature verification failed: %w", err)
	}

	return actorURI, nil
}

// handleOutbox serves the Outbox collection with real post data.
// Outbox コレクションを実データで返す。
func (h *Handler) handleOutbox(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	ctx := r.Context()

	persona, err := h.store.GetPersonaByUsername(ctx, username)
	if err != nil || persona == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	base := h.baseURL(r)
	actorURL := base + "/users/" + persona.Username
	postCount, _ := h.store.CountLocalPosts(ctx)

	// Fetch public local posts (up to 20 most recent).
	// 公開ローカル投稿を最新20件まで取得。
	posts, _ := h.store.ListPublicLocalPosts(ctx, persona.ID, id.Nil, 20)

	items := make([]interface{}, 0, len(posts))
	for _, post := range posts {
		// Reblog wrapper: emit as Announce activity.
		// リブログ wrapper は Announce Activity として出力。
		if !post.ReblogOfPostID.IsNil() {
			original, err := h.store.GetPost(ctx, post.ReblogOfPostID)
			if err != nil {
				continue
			}
			// Resolve the original post's URI.
			// 元投稿の URI を解決。
			objectURI := original.URI
			if objectURI == "" {
				objectURI = actorURL + "/posts/" + original.ID.String()
			}
			announceID := actorURL + "/posts/" + post.ID.String() + "/activity"
			items = append(items, activitypub.NewActivity(announceID, "Announce", actorURL, objectURI))
			continue
		}

		postURI := actorURL + "/posts/" + post.ID.String()

		to := []string{"https://www.w3.org/ns/activitystreams#Public"}
		cc := []string{actorURL + "/followers"}
		if post.Visibility == murlog.VisibilityUnlisted {
			to, cc = cc, to
		}

		// Render content for AP delivery.
		// AP 配送用にコンテンツをレンダリング。
		noteContent := post.Content
		noteContentMap := post.ContentMap
		if post.ContentType == murlog.ContentTypeText {
			noteContent = formatPostContent(post.Content, base)
			if len(post.ContentMap) > 0 {
				noteContentMap = make(map[string]string, len(post.ContentMap))
				for lang, text := range post.ContentMap {
					noteContentMap[lang] = formatPostContent(text, base)
				}
			}
			// Apply mention links. / メンションリンクを適用。
			if mentions := post.Mentions(); len(mentions) > 0 {
				resolved := make(map[string]mention.Resolved, len(mentions))
				for _, m := range mentions {
					resolved[m.Acct] = mention.Resolved{Acct: m.Acct, ActorURI: m.Href, ProfileURL: m.Href}
				}
				noteContent = mention.ReplaceWithHTML(noteContent, resolved)
				for lang, text := range noteContentMap {
					noteContentMap[lang] = mention.ReplaceWithHTML(text, resolved)
				}
			}
		}

		note := activitypub.Note{
			ID:           postURI,
			Type:         "Note",
			AttributedTo: actorURL,
			Content:      noteContent,
			ContentMap:   noteContentMap,
			Summary:      post.Summary,
			Sensitive:    post.Sensitive,
			Published:    post.CreatedAt.UTC().Format(time.RFC3339),
			To:           to,
			CC:           cc,
		}

		// Attach media if present. / メディアがあれば添付。
		atts, _ := h.store.ListAttachmentsByPost(ctx, post.ID)
		for _, a := range atts {
			note.Attachment = append(note.Attachment, activitypub.NoteAttachment{
				Type:      "Document",
				MediaType: a.MimeType,
				URL:       h.resolveMediaURL(base, a.FilePath),
				Name:      a.Alt,
				Width:     a.Width,
				Height:    a.Height,
			})
		}

		items = append(items, activitypub.NewActivity(postURI+"/activity", "Create", actorURL, note))
	}

	collection := activitypub.OrderedCollection(actorURL+"/outbox", postCount, items)

	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(collection)
}

// handleFollowersCollection serves the Followers collection with real data.
// Followers コレクションを実データで返す。
func (h *Handler) handleFollowersCollection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Vary", "Accept")
	// ブラウザからのアクセスは SPA にフォールバック。
	// Browser requests fall back to SPA.
	if !isActivityPubRequest(r) {
		h.serveSPA(w, r)
		return
	}

	username := r.PathValue("username")
	ctx := r.Context()

	persona, err := h.store.GetPersonaByUsername(ctx, username)
	if err != nil || persona == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	base := h.baseURL(r)
	actorURL := base + "/users/" + persona.Username

	// Always report totalItems; only expose orderedItems if show_follows is enabled.
	// totalItems は常に返す。orderedItems は show_follows 有効時のみ公開。
	followers, _ := h.store.ListFollowers(ctx, persona.ID)
	var items []string
	if persona.ShowFollows {
		items = make([]string, 0, len(followers))
		for _, f := range followers {
			items = append(items, f.ActorURI)
		}
	}

	collection := activitypub.OrderedCollection(actorURL+"/followers", len(followers), items)

	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(collection)
}

// handleFollowingCollection serves the Following collection with real data.
// Following コレクションを実データで返す。
func (h *Handler) handleFollowingCollection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Vary", "Accept")
	// ブラウザからのアクセスは SPA にフォールバック。
	// Browser requests fall back to SPA.
	if !isActivityPubRequest(r) {
		h.serveSPA(w, r)
		return
	}

	username := r.PathValue("username")
	ctx := r.Context()

	persona, err := h.store.GetPersonaByUsername(ctx, username)
	if err != nil || persona == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	base := h.baseURL(r)
	actorURL := base + "/users/" + persona.Username

	// Always report totalItems; only expose orderedItems if show_follows is enabled.
	// totalItems は常に返す。orderedItems は show_follows 有効時のみ公開。
	follows, _ := h.store.ListFollows(ctx, persona.ID)
	var items []string
	if persona.ShowFollows {
		items = make([]string, 0, len(follows))
		for _, f := range follows {
			items = append(items, f.TargetURI)
		}
	}

	collection := activitypub.OrderedCollection(actorURL+"/following", len(follows), items)

	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(collection)
}

// handleFeaturedCollection serves the Featured (pinned posts) collection.
// Featured (ピン留め投稿) コレクションを返す。
func (h *Handler) handleFeaturedCollection(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	persona, err := h.store.GetPersonaByUsername(r.Context(), username)
	if err != nil || persona == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	base := h.baseURL(r)
	actorURL := base + "/users/" + persona.Username

	var items []interface{}
	if pinnedPost, err := h.store.GetPinnedPost(r.Context(), persona.ID); err == nil && pinnedPost != nil {
		postURI := actorURL + "/posts/" + pinnedPost.ID.String()
		to := []string{"https://www.w3.org/ns/activitystreams#Public"}
		cc := []string{actorURL + "/followers"}
		note := map[string]interface{}{
			"id":           postURI,
			"type":         "Note",
			"attributedTo": actorURL,
			"content":      pinnedPost.Content,
			"published":    pinnedPost.CreatedAt.UTC().Format(time.RFC3339),
			"to":           to,
			"cc":           cc,
		}
		if len(pinnedPost.ContentMap) > 0 {
			note["contentMap"] = pinnedPost.ContentMap
		}
		if pinnedPost.Summary != "" {
			note["summary"] = pinnedPost.Summary
		}
		if pinnedPost.Sensitive {
			note["sensitive"] = true
		}
		items = append(items, note)
	}
	if items == nil {
		items = []interface{}{}
	}

	collection := activitypub.OrderedCollection(actorURL+"/collections/featured", len(items), items)

	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(collection)
}


// detectVisibility determines post visibility from ActivityPub to/cc fields.
// to/cc フィールドから投稿の公開範囲を判定する。
// Public in to → public, Public in cc → unlisted,
// followers collection in to/cc → followers, otherwise → direct.
func detectVisibility(obj map[string]interface{}) murlog.Visibility {
	const publicURI = "https://www.w3.org/ns/activitystreams#Public"
	toList := toStringSlice(obj["to"])
	ccList := toStringSlice(obj["cc"])
	for _, v := range toList {
		if v == publicURI {
			return murlog.VisibilityPublic
		}
	}
	for _, v := range ccList {
		if v == publicURI {
			return murlog.VisibilityUnlisted
		}
	}
	// No Public URI — check for followers collection to distinguish followers-only vs direct.
	// Public URI なし — フォロワーコレクションの有無で followers と direct を区別。
	for _, v := range append(toList, ccList...) {
		if strings.HasSuffix(v, "/followers") {
			return murlog.VisibilityFollowers
		}
	}
	return murlog.VisibilityDirect
}

// toStringSlice converts an interface{} (string or []interface{}) to []string.
// interface{} (string or []interface{}) を []string に変換する。
func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return []string{s}
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
