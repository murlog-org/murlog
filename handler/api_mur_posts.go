package handler

import (
	"context"
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/hashtag"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/mention"
)

// postJSON is the API representation of a Post.
// Post の API レスポンス表現。
type postJSON struct {
	ID             string            `json:"id"`
	PersonaID      string            `json:"persona_id"`
	Content        string            `json:"content"`                   // rendered HTML / 表示用 HTML
	ContentSource  string            `json:"content_source,omitempty"`  // plain text for editing (text type only) / 編集用プレーンテキスト
	ContentMap     map[string]string `json:"content_map,omitempty"`
	Summary        string            `json:"summary,omitempty"`
	Sensitive      bool              `json:"sensitive,omitempty"`
	Visibility     string            `json:"visibility"`
	Origin         string            `json:"origin"`
	ActorURI       string            `json:"actor_uri,omitempty"`
	ActorAcct      string            `json:"actor_acct,omitempty"`      // @user@host for remote actors / リモート Actor の @user@host
	ActorName      string            `json:"actor_name,omitempty"`      // display name for post author / 投稿者の表示名
	ActorAvatarURL       string            `json:"actor_avatar_url,omitempty"` // remote actor avatar / リモート Actor のアバター
	RebloggedByURI       string            `json:"reblogged_by_uri,omitempty"`       // リブログ元の Actor URI / Actor URI of reblogger
	RebloggedByAcct      string            `json:"reblogged_by_acct,omitempty"`      // リブログ元の @user@host / @user@host of reblogger
	RebloggedByName      string            `json:"reblogged_by_name,omitempty"`      // リブログ元の表示名 / display name of reblogger
	RebloggedByAvatarURL string            `json:"reblogged_by_avatar_url,omitempty"` // リブログ元のアバター / avatar of reblogger
	InReplyToURI         string            `json:"in_reply_to_uri,omitempty"`
	InReplyToPost  *postJSON         `json:"in_reply_to_post,omitempty"`
	Attachments    []attachmentJSON  `json:"attachments,omitempty"`
	Pinned         bool              `json:"pinned,omitempty"`
	FavouritesCount int              `json:"favourites_count"`
	ReblogsCount    int              `json:"reblogs_count"`
	Favourited      bool             `json:"favourited"`
	Reblogged       bool             `json:"reblogged"`
	Reblog         *postJSON         `json:"reblog,omitempty"`          // nested original post for reblog wrapper / リブログ wrapper の元投稿
	URL            string            `json:"url,omitempty"`             // パーマリンク URL / permalink URL
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
}

func visibilityString(v murlog.Visibility) string {
	switch v {
	case murlog.VisibilityUnlisted:
		return "unlisted"
	case murlog.VisibilityFollowers:
		return "followers"
	case murlog.VisibilityDirect:
		return "direct"
	default:
		return "public"
	}
}

func parseVisibility(s string) murlog.Visibility {
	switch s {
	case "unlisted":
		return murlog.VisibilityUnlisted
	case "followers":
		return murlog.VisibilityFollowers
	case "direct":
		return murlog.VisibilityDirect
	default:
		return murlog.VisibilityPublic
	}
}

func toPostJSON(p *murlog.Post) postJSON {
	return postJSON{
		ID:           p.ID.String(),
		PersonaID:    p.PersonaID.String(),
		Content:      p.Content,
		ContentMap:   p.ContentMap,
		Summary:      p.Summary,
		Sensitive:    p.Sensitive,
		Visibility:   visibilityString(p.Visibility),
		Origin:       p.Origin,
		ActorURI:       p.ActorURI,
		RebloggedByURI: p.RebloggedByURI,
		InReplyToURI:   p.InReplyToURI,
		CreatedAt:    p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    p.UpdatedAt.Format(time.RFC3339),
	}
}

// enrichPostJSON loads attachments and interaction state for a single post.
// Fetches persona per call — use enrichPostJSONList for batch optimization.
// 単一投稿の添付ファイルといいね・リブログ状態を読み込む。
// ペルソナを毎回取得する — バッチ最適化には enrichPostJSONList を使う。
func (h *Handler) enrichPostJSON(ctx context.Context, p *murlog.Post) postJSON {
	persona, _ := h.store.GetPersona(ctx, p.PersonaID)
	atts, _ := h.store.ListAttachmentsByPost(ctx, p.ID)
	pj := h.enrichPostJSONWith(ctx, p, persona, atts)
	countsMap, _ := h.store.PostInteractionCounts(ctx, []id.ID{p.ID})
	if c, ok := countsMap[p.ID]; ok {
		pj.FavouritesCount = c.Favourites
		pj.ReblogsCount = c.Reblogs
	}
	// Single-post path: fetch parent individually.
	// 単体パス: 親投稿を個別取得。
	if p.InReplyToURI != "" {
		if parent := h.resolvePostByURI(ctx, p.InReplyToURI); parent != nil {
			pp := toPostJSON(parent)
			pj.InReplyToPost = &pp
		}
	}
	return pj
}

// enrichPostJSONWith enriches a post with pre-fetched persona and attachments.
// 事前取得済みのペルソナと添付ファイルで投稿を enrichする。
func (h *Handler) enrichPostJSONWith(ctx context.Context, p *murlog.Post, persona *murlog.Persona, atts []*murlog.Attachment) postJSON {
	pj := toPostJSON(p)
	base := h.baseURLFromCtx(ctx)

	// Render content based on content_type.
	// content_type に基づいてコンテンツをレンダリング。
	if p.ContentType == murlog.ContentTypeText {
		pj.ContentSource = p.Content
		pj.Content = formatPostContent(p.Content, base)
		if len(p.ContentMap) > 0 {
			rendered := make(map[string]string, len(p.ContentMap))
			for lang, text := range p.ContentMap {
				rendered[lang] = formatPostContent(text, base)
			}
			pj.ContentMap = rendered
		}
		// Apply mention links from stored mentions_json.
		// 保存済み mentions_json からメンションリンクを適用。
		if mentions := p.Mentions(); len(mentions) > 0 {
			resolved := make(map[string]mention.Resolved, len(mentions))
			for _, m := range mentions {
				resolved[m.Acct] = mention.Resolved{
					Acct:       m.Acct,
					ActorURI:   m.Href,
					ProfileURL: m.Href,
				}
			}
			pj.Content = mention.ReplaceWithHTML(pj.Content, resolved)
			for lang, text := range pj.ContentMap {
				pj.ContentMap[lang] = mention.ReplaceWithHTML(text, resolved)
			}
		}
	}

	if len(atts) > 0 {
		pj.Attachments = make([]attachmentJSON, len(atts))
		for i, a := range atts {
			pj.Attachments[i] = h.toAttachmentJSON(base, a)
		}
	}

	// Set permalink URL — all posts use internal /my/posts/{id}.
	// Public pages construct their own URLs via mapPostsForTemplate / toPostDataList.
	// パーマリンク URL — 全投稿を内部パス /my/posts/{id} に統一。
	// 公開ページは mapPostsForTemplate / toPostDataList で独自に URL を構築する。
	pj.URL = base + "/my/posts/" + p.ID.String()

	// Resolve actor acct, display name and avatar for display.
	// 表示用に Actor の acct・表示名・アバターを解決。
	if p.ActorURI != "" {
		ra, _ := h.store.GetRemoteActor(ctx, p.ActorURI)
		if ra != nil {
			pj.ActorAcct = formatAcct(ra)
			pj.ActorName = ra.DisplayName
			pj.ActorAvatarURL = ra.AvatarURL
		}
	} else if persona != nil {
		// ローカル投稿: ペルソナの acct・表示名・アバターを設定。
		// Local post: set persona acct, display name and avatar.
		pj.ActorAcct = "@" + persona.Username
		pj.ActorName = persona.DisplayName
		if persona.AvatarPath != "" {
			pj.ActorAvatarURL = h.resolveMediaURL(base, persona.AvatarPath)
		}
	}

	// Resolve reblogger actor info for display.
	// 表示用にリブログ元 Actor 情報を解決。
	if p.RebloggedByURI != "" {
		ra, _ := h.store.GetRemoteActor(ctx, p.RebloggedByURI)
		if ra != nil {
			pj.RebloggedByAcct = formatAcct(ra)
			pj.RebloggedByName = ra.DisplayName
			pj.RebloggedByAvatarURL = ra.AvatarURL
		}
	}

	// Resolve reblog wrapper: load and nest the original post.
	// リブログ wrapper の場合、元投稿をロードしてネストする。
	if !p.ReblogOfPostID.IsNil() {
		if original, err := h.store.GetPost(ctx, p.ReblogOfPostID); err == nil {
			rpj := h.enrichPostJSON(ctx, original)
			pj.Reblog = &rpj
		}
	}

	// Check if this post is pinned. / この投稿がピン留めされているか判定。
	if persona != nil && !persona.PinnedPostID.IsNil() && persona.PinnedPostID == p.ID {
		pj.Pinned = true
	}

	// Check if the local user has favourited/reblogged this post (EXISTS query).
	// ローカルユーザーがこの投稿をいいね・リブログしているか判定 (EXISTS クエリ)。
	if persona != nil {
		localActorURI := base + "/users/" + persona.Username
		pj.Favourited, _ = h.store.HasFavourited(ctx, p.ID, localActorURI)
		pj.Reblogged, _ = h.store.HasReblogged(ctx, p.ID, localActorURI)
	}

	return pj
}

// enrichPostJSONList batch-loads attachments and caches persona for a list of posts.
// 投稿リストの添付ファイルをバッチ取得し、ペルソナをキャッシュして読み込む。
func (h *Handler) enrichPostJSONList(ctx context.Context, posts []*murlog.Post) []postJSON {
	if len(posts) == 0 {
		return []postJSON{}
	}

	// Batch fetch attachments. / 添付ファイルをバッチ取得。
	postIDs := make([]id.ID, len(posts))
	for i, p := range posts {
		postIDs[i] = p.ID
	}
	attsMap, _ := h.store.ListAttachmentsByPosts(ctx, postIDs)
	countsMap, _ := h.store.PostInteractionCounts(ctx, postIDs)

	// Batch fetch parent posts for replies. / リプライの親投稿をバッチ取得。
	var replyURIs []string
	for _, p := range posts {
		if p.InReplyToURI != "" {
			replyURIs = append(replyURIs, p.InReplyToURI)
		}
	}
	parentsMap, _ := h.store.GetPostsByURIs(ctx, replyURIs)

	// Cache personas to avoid repeated queries. / ペルソナをキャッシュして重複クエリを回避。
	personaCache := make(map[id.ID]*murlog.Persona)

	out := make([]postJSON, len(posts))
	for i, p := range posts {
		persona, ok := personaCache[p.PersonaID]
		if !ok {
			persona, _ = h.store.GetPersona(ctx, p.PersonaID)
			personaCache[p.PersonaID] = persona
		}
		out[i] = h.enrichPostJSONWith(ctx, p, persona, attsMap[p.ID])
		if c, ok := countsMap[p.ID]; ok {
			out[i].FavouritesCount = c.Favourites
			out[i].ReblogsCount = c.Reblogs
		}
		// Embed parent post from batch-loaded map, with local fallback.
		// バッチ取得済みの親投稿を埋め込む。ローカル投稿はフォールバック。
		if p.InReplyToURI != "" {
			var parent *murlog.Post
			if parentsMap != nil {
				parent = parentsMap[p.InReplyToURI]
			}
			if parent == nil {
				parent = h.resolvePostByURI(ctx, p.InReplyToURI)
			}
			if parent != nil {
				pp := h.enrichPostJSON(ctx, parent)
				out[i].InReplyToPost = &pp
			}
		}
	}
	return out
}

// listPublicPosts fetches public local posts with pinned-post promotion.
// Shared by RPC (posts.list public_only) and SSR (renderProfile, renderHome).
// 公開ローカル投稿を取得し、ピン留め投稿を先頭に移動する。
// RPC (posts.list public_only) と SSR (renderProfile, renderHome) の共通内部 API。
func (h *Handler) listPublicPosts(ctx context.Context, personaID id.ID, cursor id.ID, limit int) ([]postJSON, error) {
	posts, err := h.store.ListPublicLocalPosts(ctx, personaID, cursor, limit)
	if err != nil {
		return nil, err
	}

	// Move pinned post to the top on first page.
	// 初回ページでピン留め投稿を先頭に移動。
	if cursor.IsNil() {
		persona, _ := h.store.GetPersona(ctx, personaID)
		if persona != nil && !persona.PinnedPostID.IsNil() {
			found := -1
			for i, p := range posts {
				if p.ID == persona.PinnedPostID {
					found = i
					break
				}
			}
			if found > 0 {
				pinned := posts[found]
				posts = append([]*murlog.Post{pinned}, append(posts[:found], posts[found+1:]...)...)
			} else if found < 0 {
				if pinned, err := h.store.GetPost(ctx, persona.PinnedPostID); err == nil {
					posts = append([]*murlog.Post{pinned}, posts...)
				}
			}
		}
	}

	return h.enrichPostJSONList(ctx, posts), nil
}

// listPostsParams is the params for posts.list.
type listPostsParams struct {
	PersonaID  string `json:"persona_id,omitempty"`
	Username   string `json:"username,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	PublicOnly bool   `json:"public_only,omitempty"`
}

// rpcPostsList handles posts.list.
// 投稿一覧を返す。未認証の場合は公開投稿のみ。
func (h *Handler) rpcPostsList(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[listPostsParams](params)
	if rErr != nil {
		return nil, rErr
	}

	// Resolve persona by ID or username.
	// ID または username でペルソナを解決。
	var personaID id.ID
	if req.Username != "" {
		p, err := h.store.GetPersonaByUsername(ctx, req.Username)
		if err != nil || p == nil {
			return nil, newRPCErr(codeNotFound, "persona not found")
		}
		personaID = p.ID
	} else {
		var rErr *rpcErr
		personaID, rErr = h.resolvePersonaID(ctx, req.PersonaID)
		if rErr != nil {
			return nil, rErr
		}
	}

	// Parse cursor. / カーソルをパース。
	var cursor id.ID
	if req.Cursor != "" {
		c, err := id.Parse(req.Cursor)
		if err == nil {
			cursor = c
		}
	}

	// Clamp limit. / limit をクランプ。
	limit := clampLimit(req.Limit, 20, 100)

	// Public-only mode: unauthenticated or explicitly requested.
	// 公開のみモード: 未認証またはリクエストで明示指定。
	if !h.isRPCAuthed(ctx) || req.PublicOnly {
		result, err := h.listPublicPosts(ctx, personaID, cursor, limit)
		if err != nil {
			return nil, newRPCErr(codeInternalError, "internal error")
		}
		return result, nil
	}

	posts, err := h.store.ListPostsByPersona(ctx, personaID, cursor, limit)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	return h.enrichPostJSONList(ctx, posts), nil
}

// rpcPostsGet handles posts.get.
// 指定投稿を返す。未認証の場合は公開投稿のみ。
func (h *Handler) rpcPostsGet(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[getParams](params)
	if rErr != nil {
		return nil, rErr
	}

	pid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	p, err := h.store.GetPost(ctx, pid)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "post not found")
	}

	// Unauthenticated: block non-public posts.
	// 未認証: 非公開投稿はブロック。
	if !h.isRPCAuthed(ctx) && p.Visibility != 0 {
		return nil, newRPCErr(codeNotFound, "post not found")
	}

	return h.enrichPostJSON(ctx, p), nil
}

// maxAttachmentsPerPost is the maximum number of attachments per post.
// 1投稿あたりの最大添付数。
const maxAttachmentsPerPost = 4

// MaxPostContentLength is the maximum character count for post content (Misskey-compatible).
// 投稿本文の最大文字数 (Misskey 準拠)。
const MaxPostContentLength = 3000

// MaxDisplayNameLength is the maximum character count for display names.
// 表示名の最大文字数。
const MaxDisplayNameLength = 100

// MaxBioLength is the maximum character count for profile bios/summaries.
// プロフィール bio/summary の最大文字数。
const MaxBioLength = 3000

// Remote data array limits — prevent resource exhaustion from crafted ActivityPub objects.
// リモートデータ配列上限 — 細工された AP オブジェクトからのリソース枯渇を防止。
const (
	maxContentMapLangs    = 50  // 言語バリアント数 / language variants in contentMap
	maxRemoteTags         = 100 // tag (Mention/Hashtag) 数 / tags per Note
	maxRemoteAttachments  = 20  // 添付ファイル数 / attachments per Note
	maxRemoteActorFields  = 20  // Actor カスタムフィールド数 / PropertyValue fields per Actor
)

// truncateRunes truncates a string to n runes without suffix.
// 文字列を n ルーンで切り詰める (サフィックスなし)。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// createPostParams is the params for posts.create.
type createPostParams struct {
	PersonaID   string            `json:"persona_id,omitempty"`
	Content     string            `json:"content"`
	ContentMap  map[string]string `json:"content_map,omitempty"`
	Summary     string            `json:"summary,omitempty"`     // CW text / CW テキスト
	Sensitive   bool              `json:"sensitive,omitempty"`   // sensitive media / センシティブメディア
	Visibility  string            `json:"visibility,omitempty"`
	InReplyTo   string            `json:"in_reply_to,omitempty"` // parent post ID / リプライ先投稿 ID
	Attachments []string          `json:"attachments,omitempty"` // attachment IDs / 添付ファイル ID
}

// rpcPostsCreate handles posts.create.
// 新しい投稿を作成する。
func (h *Handler) rpcPostsCreate(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[createPostParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.Content == "" {
		return nil, newRPCErr(codeInvalidParams, "content is required")
	}
	if len([]rune(req.Content)) > MaxPostContentLength {
		return nil, newRPCErr(codeInvalidParams, "content too long (max 3000 characters)")
	}
	if len(req.Attachments) > maxAttachmentsPerPost {
		return nil, newRPCErr(codeInvalidParams, "too many attachments")
	}

	// Resolve persona. / ペルソナを特定。
	var personaID id.ID
	if req.PersonaID != "" {
		pid, err := id.Parse(req.PersonaID)
		if err != nil {
			return nil, newRPCErr(codeInvalidParams, "invalid persona_id")
		}
		if _, err := h.store.GetPersona(ctx, pid); err != nil {
			return nil, newRPCErr(codeNotFound, "persona not found")
		}
		personaID = pid
	} else {
		personas, err := h.store.ListPersonas(ctx)
		if err != nil || len(personas) == 0 {
			return nil, newRPCErr(codeInternalError, "internal error")
		}
		personaID = personas[0].ID
	}

	// Resolve reply target. / リプライ先を解決。
	var inReplyToURI string
	if req.InReplyTo != "" {
		replyID, err := id.Parse(req.InReplyTo)
		if err != nil {
			return nil, newRPCErr(codeInvalidParams, "invalid in_reply_to")
		}
		parent, err := h.store.GetPost(ctx, replyID)
		if err != nil {
			return nil, newRPCErr(codeNotFound, "reply target not found")
		}
		if parent.Origin == "remote" {
			// リモート投稿にはURIがある / Remote posts have a URI
			inReplyToURI = parent.URI
		} else {
			// ローカル投稿はURIを構築 / Construct URI for local posts
			base := h.baseURLFromCtx(ctx)
			parentPersona, err := h.store.GetPersona(ctx, parent.PersonaID)
			if err != nil {
				return nil, newRPCErr(codeInternalError, "internal error")
			}
			inReplyToURI = base + "/users/" + parentPersona.Username + "/posts/" + parent.ID.String()
		}
	}

	visibility := parseVisibility(req.Visibility)
	now := time.Now()

	// Store plain text as-is. HTML rendering is done at read time based on content_type.
	// プレーンテキストをそのまま保存。HTML 変換は content_type に基づき読み出し時に行う。
	tags := hashtag.ParseHashtags(req.Content)

	post := &murlog.Post{
		ID:           id.New(),
		PersonaID:    personaID,
		Content:      req.Content,
		ContentType:  murlog.ContentTypeText,
		ContentMap:   req.ContentMap,
		Summary:      req.Summary,
		Sensitive:    req.Sensitive,
		Visibility:   visibility,
		InReplyToURI: inReplyToURI,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if len(tags) > 0 {
		post.SetHashtags(tags)
	}

	if err := h.store.CreatePost(ctx, post); err != nil {
		return nil, newRPCErr(codeInternalError, "create post failed")
	}

	// Attach uploaded media if provided. / アップロード済みメディアを紐づけ。
	if len(req.Attachments) > 0 {
		attIDs := make([]id.ID, 0, len(req.Attachments))
		for _, s := range req.Attachments {
			aid, err := id.Parse(s)
			if err != nil {
				continue
			}
			attIDs = append(attIDs, aid)
		}
		h.store.AttachToPost(ctx, attIDs, post.ID)
	}

	// Enqueue delivery to followers for all visibility levels.
	// 全公開範囲でフォロワーへの配送をキューに追加。
	if visibility == murlog.VisibilityPublic || visibility == murlog.VisibilityUnlisted || visibility == murlog.VisibilityFollowers {
		job := murlog.NewJob(murlog.JobDeliverPost, map[string]string{"post_id": post.ID.String()})
		h.queue.Enqueue(ctx, job)
	}

	return h.enrichPostJSON(ctx, post), nil
}

// updatePostParams is the params for posts.update.
type updatePostParams struct {
	ID          string            `json:"id"`
	Content     *string           `json:"content,omitempty"`
	ContentMap  map[string]string `json:"content_map,omitempty"`
	Summary     *string           `json:"summary,omitempty"`     // CW text / CW テキスト
	Sensitive   *bool             `json:"sensitive,omitempty"`   // sensitive media / センシティブメディア
	Visibility  *string           `json:"visibility,omitempty"`
	Attachments []string          `json:"attachments,omitempty"` // attachment IDs / 添付ファイル ID
}

// rpcPostsUpdate handles posts.update.
// 指定投稿を更新する。
func (h *Handler) rpcPostsUpdate(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[updatePostParams](params)
	if rErr != nil {
		return nil, rErr
	}

	pid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	post, err := h.store.GetPost(ctx, pid)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "post not found")
	}

	if req.Content != nil {
		if len([]rune(*req.Content)) > MaxPostContentLength {
			return nil, newRPCErr(codeInvalidParams, "content too long")
		}
		post.Content = *req.Content
		tags := hashtag.ParseHashtags(*req.Content)
		if len(tags) > 0 {
			post.SetHashtags(tags)
		}
	}
	if req.ContentMap != nil {
		post.ContentMap = req.ContentMap
	}
	if req.Summary != nil {
		post.Summary = *req.Summary
	}
	if req.Sensitive != nil {
		post.Sensitive = *req.Sensitive
	}
	if req.Visibility != nil {
		post.Visibility = parseVisibility(*req.Visibility)
	}
	post.UpdatedAt = time.Now()

	if err := h.store.UpdatePost(ctx, post); err != nil {
		return nil, newRPCErr(codeInternalError, "update post failed")
	}

	// Attach new media if provided. / 新しいメディアがあれば紐づけ。
	if len(req.Attachments) > 0 {
		if len(req.Attachments) > maxAttachmentsPerPost {
			return nil, newRPCErr(codeInvalidParams, "too many attachments")
		}
		attIDs := make([]id.ID, 0, len(req.Attachments))
		for _, s := range req.Attachments {
			aid, err := id.Parse(s)
			if err != nil {
				continue
			}
			attIDs = append(attIDs, aid)
		}
		h.store.AttachToPost(ctx, attIDs, post.ID)
	}

	// Enqueue Update Activity delivery to followers.
	// フォロワーへ Update Activity を配送するジョブを enqueue。
	if post.Visibility == murlog.VisibilityPublic || post.Visibility == murlog.VisibilityUnlisted || post.Visibility == murlog.VisibilityFollowers {
		job := murlog.NewJob(murlog.JobUpdatePost, map[string]string{"post_id": post.ID.String()})
		h.queue.Enqueue(ctx, job)
	}

	return h.enrichPostJSON(ctx, post), nil
}

// rpcPostsDelete handles posts.delete.
// 指定投稿を削除する。
func (h *Handler) rpcPostsDelete(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[getParams](params)
	if rErr != nil {
		return nil, rErr
	}

	pid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	post, err := h.store.GetPost(ctx, pid)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "post not found")
	}

	// Clear pin if deleting the pinned post. / ピン留め投稿を削除する場合はピンを解除。
	persona, _ := h.store.GetPersona(ctx, post.PersonaID)
	if persona != nil && persona.PinnedPostID == pid {
		h.store.UnpinPost(ctx, post.PersonaID)
	}

	if err := h.store.DeletePost(ctx, pid); err != nil {
		return nil, newRPCErr(codeInternalError, "delete post failed")
	}

	// Enqueue Delete delivery to followers (local posts only).
	// フォロワーへの Delete 配送をキューに追加 (ローカル投稿のみ)。
	if post.Origin == "local" {
		job := murlog.NewJob(murlog.JobDeliverDelete, map[string]string{
				"persona_id": post.PersonaID.String(),
				"post_id":    pid.String(),
			})
		h.queue.Enqueue(ctx, job)
	}

	return statusOK, nil
}

// maxThreadAncestors is the maximum number of ancestors to fetch in a thread.
// スレッドで遡る祖先の最大数。
const maxThreadAncestors = 20

// resolvePostByURI finds a post by its URI, falling back to local URI path parsing.
// Local posts don't store their URI in the DB, so we extract the post ID from
// the URI path (e.g., /users/alice/posts/{id}) as a fallback.
// URI で投稿を検索し、失敗時はローカル URI パスから投稿 ID を抽出して取得する。
// ローカル投稿は DB に URI を保存しないため、URI パスからの ID 抽出がフォールバック。
func (h *Handler) resolvePostByURI(ctx context.Context, uri string) *murlog.Post {
	// Try direct URI lookup (works for remote posts with stored URI).
	// 直接 URI 検索 (URI 付きで保存されたリモート投稿用)。
	if post, err := h.store.GetPostByURI(ctx, uri); err == nil {
		return post
	}
	// Fall back: extract post ID from local URI path.
	// フォールバック: ローカル URI パスから投稿 ID を抽出。
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
	if err != nil {
		return nil
	}
	return post
}

// threadJSON is the API response for posts.get_thread.
// posts.get_thread の API レスポンス。
type threadJSON struct {
	Ancestors   []postJSON `json:"ancestors"`
	Post        postJSON   `json:"post"`
	Descendants []postJSON `json:"descendants"`
}

// rpcPostsGetThread handles posts.get_thread (public).
// 指定投稿のスレッド（祖先・子孫）を返す。
func (h *Handler) rpcPostsGetThread(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[getParams](params)
	if rErr != nil {
		return nil, rErr
	}

	pid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	post, err := h.store.GetPost(ctx, pid)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "post not found")
	}

	// Fetch ancestors by following in_reply_to_uri chain.
	// in_reply_to_uri チェーンを辿って祖先を取得。
	var ancestors []postJSON
	current := post
	for i := 0; i < maxThreadAncestors && current.InReplyToURI != ""; i++ {
		parent := h.resolvePostByURI(ctx, current.InReplyToURI)
		if parent == nil {
			break // 未取得の投稿は辿れない / Can't follow unfetched posts
		}
		ancestors = append(ancestors, h.enrichPostJSON(ctx, parent))
		current = parent
	}
	// Reverse so oldest ancestor comes first.
	// 最古の祖先が先頭に来るよう反転。
	for i, j := 0, len(ancestors)-1; i < j; i, j = i+1, j-1 {
		ancestors[i], ancestors[j] = ancestors[j], ancestors[i]
	}

	// Fetch descendants (direct replies, flat list, chronological).
	// 子孫を取得（直接の返信、フラットリスト、時系列順）。
	base := h.baseURLFromCtx(ctx)
	var postURI string
	if post.Origin == "remote" {
		postURI = post.URI
	} else {
		persona, err := h.store.GetPersona(ctx, post.PersonaID)
		if err != nil {
			return nil, newRPCErr(codeInternalError, "internal error")
		}
		postURI = base + "/users/" + persona.Username + "/posts/" + post.ID.String()
	}

	replies, err := h.store.ListReplies(ctx, postURI, id.ID{}, 100)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	if ancestors == nil {
		ancestors = []postJSON{}
	}
	return threadJSON{
		Ancestors:   ancestors,
		Post:        h.enrichPostJSON(ctx, post),
		Descendants: h.enrichPostJSONList(ctx, replies),
	}, nil
}

// rpcPostsPin handles posts.pin.
// 投稿をピン留めする (ペルソナごとに最大1件)。
func (h *Handler) rpcPostsPin(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[getParams](params)
	if rErr != nil {
		return nil, rErr
	}

	pid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	post, err := h.store.GetPost(ctx, pid)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "post not found")
	}
	if post.Origin != "local" {
		return nil, newRPCErr(codeInvalidParams, "can only pin local posts")
	}
	if post.Visibility != murlog.VisibilityPublic {
		return nil, newRPCErr(codeInvalidParams, "can only pin public posts")
	}

	if err := h.store.PinPost(ctx, post.PersonaID, post.ID); err != nil {
		return nil, newRPCErr(codeInternalError, "pin failed")
	}

	return h.enrichPostJSON(ctx, post), nil
}

// rpcPostsUnpin handles posts.unpin.
// ピン留めを解除する。
func (h *Handler) rpcPostsUnpin(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	personaID, rErr := h.resolvePersonaID(ctx, "")
	if rErr != nil {
		return nil, rErr
	}

	if err := h.store.UnpinPost(ctx, personaID); err != nil {
		return nil, newRPCErr(codeInternalError, "unpin failed")
	}

	return statusOK, nil
}

// rpcPostsListByTag handles posts.list_by_tag.
// 指定ハッシュタグを含む公開投稿を返す。
func (h *Handler) rpcPostsListByTag(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	type tagParams struct {
		Tag    string `json:"tag"`
		Cursor string `json:"cursor,omitempty"`
		Limit  int    `json:"limit,omitempty"`
	}
	req, rErr := parseParams[tagParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.Tag == "" {
		return nil, newRPCErr(codeInvalidParams, "tag is required")
	}
	limit := clampLimit(req.Limit, 20, 40)

	cursor := id.Nil
	if req.Cursor != "" {
		var err error
		cursor, err = id.Parse(req.Cursor)
		if err != nil {
			return nil, newRPCErr(codeInvalidParams, "invalid cursor")
		}
	}

	// Unauthenticated: local posts only. Authenticated: include remote posts.
	// 未認証: ローカル投稿のみ。認証済み: リモート投稿も含む。
	localOnly := !h.isRPCAuthed(ctx)
	posts, err := h.store.ListPostsByHashtag(ctx, req.Tag, cursor, limit, localOnly)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	return h.enrichPostJSONList(ctx, posts), nil
}

// rpcPostsListByActor handles posts.list_by_actor (authenticated only).
// ローカル DB からリモート Actor の投稿を返す。
func (h *Handler) rpcPostsListByActor(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	type actorParams struct {
		ActorURI string `json:"actor_uri"`
		Cursor   string `json:"cursor,omitempty"`
		Limit    int    `json:"limit,omitempty"`
	}
	req, rErr := parseParams[actorParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.ActorURI == "" {
		return nil, newRPCErr(codeInvalidParams, "actor_uri is required")
	}
	limit := clampLimit(req.Limit, 20, 40)

	cursor := id.Nil
	if req.Cursor != "" {
		var err error
		cursor, err = id.Parse(req.Cursor)
		if err != nil {
			return nil, newRPCErr(codeInvalidParams, "invalid cursor")
		}
	}

	posts, err := h.store.ListPostsByActorURI(ctx, req.ActorURI, cursor, limit)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	return h.enrichPostJSONList(ctx, posts), nil
}

// urlRe matches bare URLs in plain text (not already inside an <a> tag).
// プレーンテキスト中の生 URL にマッチする（<a> タグ内は除外）。
var urlRe = regexp.MustCompile(`https?://[^\s<>"]+`)

// autoLinkURLs converts bare URLs in plain text to <a> HTML links.
// プレーンテキスト中の生 URL を <a> タグに変換する。
func autoLinkURLsMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return m
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = autoLinkURLs(v)
	}
	return out
}

// formatPostContent converts plain text to HTML for post content.
// Escapes HTML, auto-links URLs and hashtags, wraps in <p> with <br> for newlines.
// プレーンテキストを投稿用 HTML に変換する。
// HTML エスケープ、URL・ハッシュタグの自動リンク化、<p> で囲み改行は <br> に変換。
// formatPostContent converts plain text to HTML for post content.
// Auto-links URLs and hashtags (which handle their own escaping),
// then converts remaining newlines to <br> and wraps in <p>.
// プレーンテキストを投稿用 HTML に変換する。
// URL・ハッシュタグは各関数内でエスケープ処理、改行は <br> に変換して <p> で囲む。
func formatPostContent(text string, baseURL string) string {
	escaped := html.EscapeString(text)
	linked := autoLinkURLsEscaped(escaped)
	withTags := hashtag.ReplaceWithHTML(linked, baseURL)
	withBreaks := strings.ReplaceAll(withTags, "\n", "<br>")
	return "<p>" + withBreaks + "</p>"
}

// autoLinkURLsEscaped converts bare URLs in already-escaped text to <a> links.
// No additional escaping is applied since the input is pre-escaped.
// エスケープ済みテキスト中の生 URL を <a> タグに変換する。
func autoLinkURLsEscaped(text string) string {
	return urlRe.ReplaceAllStringFunc(text, func(rawURL string) string {
		trimmed := strings.TrimRight(rawURL, ".,;:!?)」』】）")
		suffix := rawURL[len(trimmed):]
		return `<a href="` + trimmed + `" rel="nofollow noopener" target="_blank">` + trimmed + `</a>` + suffix
	})
}

// autoLinkURLs converts bare URLs in plain text to <a> HTML links.
// プレーンテキスト中の生 URL を <a> タグに変換する。
func autoLinkURLs(text string) string {
	return urlRe.ReplaceAllStringFunc(text, func(rawURL string) string {
		trimmed := strings.TrimRight(rawURL, ".,;:!?)」』】）")
		suffix := rawURL[len(trimmed):]
		escaped := html.EscapeString(trimmed)
		return `<a href="` + escaped + `" rel="nofollow noopener" target="_blank">` + escaped + `</a>` + html.EscapeString(suffix)
	})
}
