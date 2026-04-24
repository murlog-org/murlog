package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/activitypub"
	"github.com/murlog-org/murlog/internal/mediautil"
)

// remoteActorJSON / toRemoteActorJSON は accountJSON / toAccountFromRemoteActor に統合済み。
// Unified into accountJSON / toAccountFromRemoteActor in account_json.go.

type actorsLookupParams struct {
	Acct string `json:"acct"` // "user@domain" or "@user@domain"
}

// resolveRemoteActor resolves an acct to a RemoteActor via cache or WebFinger+fetch.
// acct をキャッシュまたは WebFinger+fetch で RemoteActor に解決する。
func (h *Handler) resolveRemoteActor(ctx context.Context, acct string) (*murlog.RemoteActor, error) {
	// 1. WebFinger → Actor URI.
	actorURI, err := activitypub.LookupWebFinger(acct)
	if err != nil {
		return nil, err
	}

	// 2. Always fetch fresh data (user-initiated lookup).
	// 常に最新データを取得する (ユーザー起点のルックアップ)。

	// 3. Fetch actor with HTTP Signature.
	// 署名付きで Actor を取得。
	persona, err := h.primaryPersona(ctx)
	if err != nil {
		return nil, err
	}

	base := h.baseURLFromCtx(ctx)
	keyID := base + "/users/" + persona.Username + "#main-key"

	actor, err := activitypub.FetchActorSigned(actorURI, keyID, persona.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}

	// 4. Cache the actor.
	// Actor をキャッシュ。
	ra := &murlog.RemoteActor{
		URI:          actor.ID,
		Username:     actor.PreferredUsername,
		DisplayName:  actor.Name,
		Summary:      actor.Summary,
		Inbox:        actor.Inbox,
		AvatarURL:    mediautil.ResolveActorIcon(actor),
		HeaderURL:    mediautil.ResolveActorHeader(actor),
		FeaturedURL:  actor.Featured,
		FieldsJSON:   murlog.MustJSON(activitypub.ResolveActorFields(actor)),
		Acct:         acct,
		FetchedAt:    time.Now(),
	}
	h.store.UpsertRemoteActor(ctx, ra)
	return ra, nil
}

// rpcActorsLookup resolves an acct address to a remote actor.
// acct アドレスをリモート Actor に解決する。
func (h *Handler) rpcActorsLookup(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[actorsLookupParams](params)
	if rErr != nil {
		return nil, rErr
	}

	acct := strings.TrimPrefix(req.Acct, "@")
	if acct == "" || !strings.Contains(acct, "@") {
		return nil, newRPCErr(codeInvalidParams, "acct must be user@domain")
	}

	ra, err := h.resolveRemoteActor(ctx, acct)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "could not resolve: "+err.Error())
	}
	return toAccountFromRemoteActor(ra), nil
}

// outboxPostJSON is the API representation of a remote post from an outbox.
// outbox から取得したリモート投稿の API 表現。
type outboxPostJSON struct {
	URI         string              `json:"uri"`
	Content     string              `json:"content"`
	Published   string              `json:"published"`
	Summary     string              `json:"summary,omitempty"`
	Attachments []attachmentJSON  `json:"attachments,omitempty"`
	Favourited  bool                `json:"favourited,omitempty"`
	Reblogged   bool                `json:"reblogged,omitempty"`
	LocalID     string              `json:"local_id,omitempty"` // ローカル DB の投稿 ID (Inbox 受信済みの場合) / local post ID if received via Inbox
}

type actorsOutboxParams struct {
	Acct   string `json:"acct"`             // "user@domain" or "@user@domain"
	Cursor string `json:"cursor,omitempty"` // 次ページ URL (前回レスポンスの next) / next page URL
	Limit  int    `json:"limit,omitempty"`  // default 20, max 40
}

// outboxResult wraps outbox posts with pagination cursor.
// ページネーションカーソル付きの outbox 投稿レスポンス。
type outboxResult struct {
	Posts []outboxPostJSON `json:"posts"`
	Next  string           `json:"next,omitempty"`
}

// rpcActorsOutbox fetches a remote actor's outbox and returns recent posts.
// リモート Actor の outbox をフェッチして最新の投稿を返す。
func (h *Handler) rpcActorsOutbox(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[actorsOutboxParams](params)
	if rErr != nil {
		return nil, rErr
	}

	acct := strings.TrimPrefix(req.Acct, "@")
	if acct == "" || !strings.Contains(acct, "@") {
		return nil, newRPCErr(codeInvalidParams, "acct must be user@domain")
	}

	limit := clampLimit(req.Limit, 20, 40)

	persona, err := h.primaryPersona(ctx)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	base := h.baseURLFromCtx(ctx)
	keyID := base + "/users/" + persona.Username + "#main-key"

	// cursor がある場合はそのページを直接フェッチ。
	// If cursor is provided, fetch that page directly.
	var fetchURL string
	if req.Cursor != "" {
		// M1: cursor URL のホストが acct のドメインと一致するか検証 (SSRF 防止)。
		// M1: Validate cursor URL host matches acct domain to prevent SSRF.
		acctDomain := acct[strings.Index(acct, "@")+1:]
		if err := validateCursorHost(req.Cursor, acctDomain); err != nil {
			return nil, newRPCErr(codeInvalidParams, err.Error())
		}
		fetchURL = req.Cursor
	} else {
		actorURI, err := activitypub.LookupWebFinger(acct)
		if err != nil {
			return nil, newRPCErr(codeNotFound, "could not resolve: "+err.Error())
		}
		actor, err := activitypub.FetchActorSigned(actorURI, keyID, persona.PrivateKeyPEM)
		if err != nil {
			return nil, newRPCErr(codeNotFound, "could not fetch actor: "+err.Error())
		}
		fetchURL = actor.Outbox
	}
	if fetchURL == "" {
		return outboxResult{Posts: []outboxPostJSON{}}, nil
	}

	// Fetch outbox collection page.
	// outbox コレクションページをフェッチ。
	page, err := activitypub.FetchCollectionSigned(fetchURL, keyID, persona.PrivateKeyPEM, limit)
	if err != nil {
		return outboxResult{Posts: []outboxPostJSON{}}, nil
	}

	// Extract Note content from activities or objects.
	// Activity またはオブジェクトから Note コンテンツを抽出。
	var posts []outboxPostJSON
	for _, item := range page.Items {
		note := extractNote(item)
		if note == nil {
			continue
		}
		uri, _ := note["id"].(string)
		content, _ := note["content"].(string)
		published, _ := note["published"].(string)
		summary, _ := note["summary"].(string)
		if content == "" {
			continue
		}
		post := outboxPostJSON{
			URI:       uri,
			Content:   SanitizeContentHTML(content),
			Published: published,
			Summary:   summary,
		}

		// Extract attachments from Note. / Note から添付ファイルを抽出。
		if rawAtts, ok := note["attachment"].([]interface{}); ok {
			for _, rawAtt := range rawAtts {
				att, ok := rawAtt.(map[string]interface{})
				if !ok {
					continue
				}
				attURL, _ := att["url"].(string)
				if attURL == "" {
					continue
				}
				post.Attachments = append(post.Attachments, attachmentJSON{
					URL:      attURL,
					Alt:      strVal(att, "name"),
					MimeType: strVal(att, "mediaType"),
				})
			}
		}

		posts = append(posts, post)
	}
	// Enrich with local interaction state (favourited/reblogged).
	// ローカルのいいね・リブログ状態を付与。
	if len(posts) > 0 && h.isRPCAuthed(ctx) {
		uris := make([]string, len(posts))
		for i, p := range posts {
			uris[i] = p.URI
		}
		postsMap, _ := h.store.GetPostsByURIs(ctx, uris)
		if len(postsMap) > 0 {
			localActorURI := base + "/users/" + persona.Username
			for i, p := range posts {
				if local, ok := postsMap[p.URI]; ok {
					posts[i].LocalID = local.ID.String()
					posts[i].Favourited, _ = h.store.HasFavourited(ctx, local.ID, localActorURI)
					posts[i].Reblogged, _ = h.store.HasReblogged(ctx, local.ID, localActorURI)
				}
			}
		}
	}

	return outboxResult{Posts: posts, Next: page.Next}, nil
}

// strVal extracts a string value from a map.
// map から文字列値を取り出す。
func strVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

// extractNote extracts a Note from an Activity or a direct Note object.
// Activity または直接の Note オブジェクトから Note を抽出する。
func extractNote(item map[string]interface{}) map[string]interface{} {
	itemType, _ := item["type"].(string)
	if itemType == "Note" {
		return item
	}
	if itemType == "Create" {
		if obj, ok := item["object"].(map[string]interface{}); ok {
			if objType, _ := obj["type"].(string); objType == "Note" {
				return obj
			}
		}
	}
	return nil
}

type actorsCollectionParams struct {
	Acct   string `json:"acct"`             // "user@domain" or "@user@domain"
	Cursor string `json:"cursor,omitempty"` // 次ページ URL (前回レスポンスの next) / next page URL from previous response
}

// remoteCollectionResult wraps follow/follower items with pagination cursor.
// ページネーションカーソル付きのフォロー/フォロワー一覧レスポンス。
type remoteCollectionResult struct {
	Items []accountJSON `json:"items"`
	Next  string        `json:"next,omitempty"`  // 次ページ URL / next page URL
	Total int           `json:"total,omitempty"` // コレクション全体の件数 / total items in collection
}

// rpcActorsFollowing fetches a remote actor's following collection.
// リモート Actor の following コレクションをフェッチする。
func (h *Handler) rpcActorsFollowing(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	return h.fetchRemoteCollection(ctx, params, "following")
}

// rpcActorsFollowers fetches a remote actor's followers collection.
// リモート Actor の followers コレクションをフェッチする。
func (h *Handler) rpcActorsFollowers(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	return h.fetchRemoteCollection(ctx, params, "followers")
}

func (h *Handler) fetchRemoteCollection(ctx context.Context, params json.RawMessage, which string) (any, *rpcErr) {
	req, rErr := parseParams[actorsCollectionParams](params)
	if rErr != nil {
		return nil, rErr
	}

	acct := strings.TrimPrefix(req.Acct, "@")
	if acct == "" || !strings.Contains(acct, "@") {
		return nil, newRPCErr(codeInvalidParams, "acct must be user@domain")
	}

	persona, err := h.primaryPersona(ctx)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	base := h.baseURLFromCtx(ctx)
	keyID := base + "/users/" + persona.Username + "#main-key"

	// cursor がある場合はそのページを直接フェッチ。
	// If cursor is provided, fetch that page directly.
	var fetchURL string
	if req.Cursor != "" {
		// M1: cursor URL のホストが acct のドメインと一致するか検証 (SSRF 防止)。
		// M1: Validate cursor URL host matches acct domain to prevent SSRF.
		acctDomain := acct[strings.Index(acct, "@")+1:]
		if err := validateCursorHost(req.Cursor, acctDomain); err != nil {
			return nil, newRPCErr(codeInvalidParams, err.Error())
		}
		fetchURL = req.Cursor
	} else {
		actorURI, err := activitypub.LookupWebFinger(acct)
		if err != nil {
			return nil, newRPCErr(codeNotFound, "could not resolve: "+err.Error())
		}
		actor, err := activitypub.FetchActorSigned(actorURI, keyID, persona.PrivateKeyPEM)
		if err != nil {
			return nil, newRPCErr(codeNotFound, "could not fetch actor: "+err.Error())
		}
		if which == "following" {
			fetchURL = actor.Following
		} else {
			fetchURL = actor.Followers
		}
	}
	if fetchURL == "" {
		return remoteCollectionResult{Items: []accountJSON{}}, nil
	}

	page, err := activitypub.FetchCollectionSigned(fetchURL, keyID, persona.PrivateKeyPEM, 0)
	if err != nil {
		return remoteCollectionResult{Items: []accountJSON{}}, nil
	}

	// Build response with actor info from inline objects or local cache.
	// インライン Actor オブジェクトまたはローカルキャッシュから Actor 情報を付与。
	out := make([]accountJSON, 0, len(page.Items))
	for _, item := range page.Items {
		uri, _ := item["id"].(string)
		if uri == "" {
			continue
		}
		rj := accountJSON{URI: uri}

		// Try inline actor fields first. / まずインライン Actor フィールドを試す。
		if name, ok := item["name"].(string); ok && name != "" {
			rj.DisplayName = name
			if username, ok := item["preferredUsername"].(string); ok {
				if u, err := url.Parse(uri); err == nil {
					rj.Acct = "@" + username + "@" + u.Host
				}
			}
			if icon, ok := item["icon"].(map[string]interface{}); ok {
				rj.AvatarURL, _ = icon["url"].(string)
			}
			rj.Summary, _ = item["summary"].(string)
		} else if ra := h.resolveRemoteActorByURI(ctx, uri); ra != nil {
			// Fallback to local cache. / ローカルキャッシュにフォールバック。
			rj.Acct = formatAcct(ra)
			rj.DisplayName = ra.DisplayName
			rj.AvatarURL = ra.AvatarURL
			rj.Summary = ra.Summary
		} else {
			// Best-effort: derive acct from URI + enqueue background fetch.
			// ベストエフォート: URI から acct を推測 + バックグラウンドフェッチをキューに追加。
			if u, err := url.Parse(uri); err == nil {
				parts := strings.Split(u.Path, "/")
				if len(parts) > 0 {
					username := parts[len(parts)-1]
					rj.Acct = "@" + username + "@" + u.Host
				}
			}
			h.queue.Enqueue(ctx, murlog.NewJob(murlog.JobFetchRemoteActor, map[string]string{"actor_uri": uri}))
		}
		out = append(out, rj)
	}

	total := 0
	if page.TotalItems >= 0 {
		total = page.TotalItems
	}
	return remoteCollectionResult{Items: out, Next: page.Next, Total: total}, nil
}

// validateCursorHost checks that a cursor URL's host matches the expected domain.
// cursor URL のホストが期待されるドメインと一致するか検証する。
func validateCursorHost(cursorURL, expectedDomain string) error {
	u, err := url.Parse(cursorURL)
	if err != nil {
		return fmt.Errorf("invalid cursor URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("invalid cursor URL scheme")
	}
	if !strings.EqualFold(u.Hostname(), expectedDomain) {
		return fmt.Errorf("cursor host mismatch")
	}
	return nil
}


// rpcActorsFeatured fetches a remote actor's Featured (pinned) collection.
// リモート Actor の Featured (ピン留め) コレクションを取得する。
func (h *Handler) rpcActorsFeatured(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	type featuredParams struct {
		FeaturedURL string `json:"featured_url"`
	}
	req, rErr := parseParams[featuredParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.FeaturedURL == "" {
		return []outboxPostJSON{}, nil
	}

	persona, rErr := h.resolvePersonaID(ctx, "")
	if rErr != nil {
		return []outboxPostJSON{}, nil
	}
	p, err := h.store.GetPersona(ctx, persona)
	if err != nil {
		return []outboxPostJSON{}, nil
	}
	base := h.baseURLFromCtx(ctx)
	keyID := base + "/users/" + p.Username + "#main-key"

	page, err := activitypub.FetchCollectionSigned(req.FeaturedURL, keyID, p.PrivateKeyPEM, 0)
	if err != nil {
		return []outboxPostJSON{}, nil
	}

	// Limit featured items to prevent excessive individual fetches.
	// 個別フェッチの過多を防ぐため featured アイテム数を制限。
	const maxFeaturedItems = 10
	items := page.Items
	if len(items) > maxFeaturedItems {
		items = items[:maxFeaturedItems]
	}
	var posts []outboxPostJSON
	for _, item := range items {
		note := extractNote(item)
		if note == nil {
			// Item may be a URI reference (e.g. Misskey). Fetch the Note individually.
			// URI 参照のみの場合 (Misskey 等)。Note を個別にフェッチする。
			if uri, _ := item["id"].(string); uri != "" {
				fetched, err := activitypub.FetchNoteSigned(uri, keyID, p.PrivateKeyPEM)
				if err == nil {
					note = fetched
				}
			}
			if note == nil {
				continue
			}
		}
		uri, _ := note["id"].(string)
		content, _ := note["content"].(string)
		published, _ := note["published"].(string)
		if content == "" {
			continue
		}
		post := outboxPostJSON{
			URI:       uri,
			Content:   SanitizeContentHTML(content),
			Published: published,
		}
		if rawAtts, ok := note["attachment"].([]interface{}); ok {
			for _, rawAtt := range rawAtts {
				att, ok := rawAtt.(map[string]interface{})
				if !ok {
					continue
				}
				attURL, _ := att["url"].(string)
				if attURL == "" {
					continue
				}
				post.Attachments = append(post.Attachments, attachmentJSON{
					URL:      attURL,
					Alt:      strVal(att, "name"),
					MimeType: strVal(att, "mediaType"),
				})
			}
		}
		posts = append(posts, post)
	}
	return posts, nil
}
