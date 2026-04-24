// Package handler — JSON-RPC 2.0 dispatcher.
// JSON-RPC 2.0 ディスパッチャー。
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// JSON-RPC 2.0 error codes.
// JSON-RPC 2.0 エラーコード。
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603

	// Application-defined errors (-32000 to -32099).
	// アプリ固有エラー。
	codeUnauthorized = -32000
	codeNotFound     = -32001
	codeConflict     = -32002
)

// rpcRequest is a JSON-RPC 2.0 request object.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      any             `json:"id,omitempty"`
}

// rpcResponse is a JSON-RPC 2.0 response object.
type rpcResponse struct {
	JSONRPC string   `json:"jsonrpc"`
	Result  any      `json:"result,omitempty"`
	Error   *rpcErr  `json:"error,omitempty"`
	ID      any      `json:"id"`
}

// rpcErr is a JSON-RPC 2.0 error object.
type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func newRPCErr(code int, msg string) *rpcErr {
	return &rpcErr{Code: code, Message: msg}
}

func newRPCErrData(code int, msg string, data any) *rpcErr {
	return &rpcErr{Code: code, Message: msg, Data: data}
}

// rpcMethod is the signature for JSON-RPC method handlers.
// JSON-RPC メソッドハンドラのシグネチャ。
type rpcMethod func(ctx context.Context, params json.RawMessage) (any, *rpcErr)

// methodDef defines a registered JSON-RPC method.
type methodDef struct {
	handler rpcMethod
	public  bool // true = no auth required / 認証不要
}

// context keys for passing http.ResponseWriter and *http.Request.
type rpcCtxKey int

const (
	ctxKeyWriter  rpcCtxKey = iota
	ctxKeyRequest
)

func rpcWriter(ctx context.Context) http.ResponseWriter {
	w, _ := ctx.Value(ctxKeyWriter).(http.ResponseWriter)
	return w
}

func rpcRequest_(ctx context.Context) *http.Request {
	r, _ := ctx.Value(ctxKeyRequest).(*http.Request)
	return r
}

// registerRPCMethods sets up the method dispatch table.
// メソッドディスパッチテーブルを構築する。
func (h *Handler) registerRPCMethods() map[string]methodDef {
	return map[string]methodDef{
		// Auth (public)
		"auth.login":           {handler: h.rpcAuthLogin, public: true},
		"auth.logout":          {handler: h.rpcAuthLogout, public: false},
		"auth.change_password": {handler: h.rpcAuthChangePassword, public: false},

		// Personas
		"personas.list":   {handler: h.rpcPersonasList, public: true},
		"personas.get":    {handler: h.rpcPersonasGet, public: true},
		"personas.create": {handler: h.rpcPersonasCreate, public: false},
		"personas.update": {handler: h.rpcPersonasUpdate, public: false},

		// Posts
		"posts.list":        {handler: h.rpcPostsList, public: true},
		"posts.list_by_tag": {handler: h.rpcPostsListByTag, public: true},
		"posts.get":        {handler: h.rpcPostsGet, public: true},
		"posts.get_thread": {handler: h.rpcPostsGetThread, public: true},
		"posts.create":     {handler: h.rpcPostsCreate, public: false},
		"posts.update":     {handler: h.rpcPostsUpdate, public: false},
		"posts.delete":     {handler: h.rpcPostsDelete, public: false},
		"posts.pin":        {handler: h.rpcPostsPin, public: false},
		"posts.unpin":      {handler: h.rpcPostsUnpin, public: false},

		// Timeline
		"timeline.home": {handler: h.rpcTimelineHome, public: false},

		// Actors
		"actors.lookup":    {handler: h.rpcActorsLookup, public: false},
		"actors.outbox":    {handler: h.rpcActorsOutbox, public: false},
		"actors.following": {handler: h.rpcActorsFollowing, public: false},
		"actors.followers": {handler: h.rpcActorsFollowers, public: false},
		"actors.featured":  {handler: h.rpcActorsFeatured, public: false},

		// Follows / Followers
		"follows.list":    {handler: h.rpcFollowsList, public: true},
		"follows.check":   {handler: h.rpcFollowsCheck, public: false},
		"follows.create":  {handler: h.rpcFollowsCreate, public: false},
		"follows.delete":  {handler: h.rpcFollowsDelete, public: false},
		"followers.list":    {handler: h.rpcFollowersList, public: true},
		"followers.pending": {handler: h.rpcFollowersPending, public: false},
		"followers.approve": {handler: h.rpcFollowersApprove, public: false},
		"followers.reject":  {handler: h.rpcFollowersReject, public: false},
		"followers.delete":  {handler: h.rpcFollowersDelete, public: false},

		// Media
		"media.delete": {handler: h.rpcMediaDelete, public: false},

		// Favourites / お気に入り
		"favourites.create": {handler: h.rpcFavouritesCreate, public: false},
		"favourites.delete": {handler: h.rpcFavouritesDelete, public: false},

		// Reblogs / リブログ
		"reblogs.create": {handler: h.rpcReblogsCreate, public: false},
		"reblogs.delete": {handler: h.rpcReblogsDelete, public: false},

		// Blocks / ブロック
		"blocks.list":         {handler: h.rpcBlocksList, public: false},
		"blocks.create":       {handler: h.rpcBlocksCreate, public: false},
		"blocks.delete":       {handler: h.rpcBlocksDelete, public: false},
		"domain_blocks.list":   {handler: h.rpcDomainBlocksList, public: false},
		"domain_blocks.create": {handler: h.rpcDomainBlocksCreate, public: false},
		"domain_blocks.delete": {handler: h.rpcDomainBlocksDelete, public: false},

		// Notifications
		"notifications.list":         {handler: h.rpcNotificationsList, public: false},
		"notifications.count_unread": {handler: h.rpcNotificationsCountUnread, public: false},
		"notifications.read":         {handler: h.rpcNotificationsRead, public: false},
		"notifications.read_all":     {handler: h.rpcNotificationsReadAll, public: false},
		"notifications.delete":       {handler: h.rpcNotificationsDelete, public: false},
		"notifications.poll":         {handler: h.rpcNotificationsPoll, public: false},

		// Domains / ドメイン配送失敗
		"domains.list_failures": {handler: h.rpcDomainsListFailures, public: false},
		"domains.reset_failure": {handler: h.rpcDomainsResetFailure, public: false},

		// Links / リンク
		"links.preview": {handler: h.rpcLinksPreview, public: true},

		// Queue / キュー
		// TOTP / 二要素認証
		"totp.setup":   {handler: h.rpcTOTPSetup, public: false},
		"totp.verify":  {handler: h.rpcTOTPVerify, public: false},
		"totp.disable": {handler: h.rpcTOTPDisable, public: false},
		"totp.status":  {handler: h.rpcTOTPStatus, public: true},

		"queue.stats": {handler: h.rpcQueueStats, public: false},
		"queue.list":  {handler: h.rpcQueueList, public: false},
		"queue.retry":   {handler: h.rpcQueueRetry, public: false},
		"queue.dismiss": {handler: h.rpcQueueDismiss, public: false},
		"queue.tick":    {handler: h.rpcQueueTick, public: false},
		"queue.vacuum":  {handler: h.rpcQueueVacuum, public: false},

		"site.get_settings":    {handler: h.rpcSiteGetSettings, public: false},
		"site.update_settings": {handler: h.rpcSiteUpdateSettings, public: false},
	}
}

// handleRPC handles POST /api/mur/v1/rpc.
// JSON-RPC 2.0 のリクエストを処理する。バッチリクエスト対応。
func (h *Handler) handleRPC(w http.ResponseWriter, r *http.Request) {
	// Limit request body to 1 MB. / リクエストボディを 1 MB に制限。
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	r.Body.Close()
	if err != nil {
		writeRPCError(w, nil, codeParseError, "parse error")
		return
	}

	// Detect batch (starts with '[') or single request.
	// バッチ (先頭が '[') か単一リクエストかを判定。
	body = trimLeft(body)
	if len(body) == 0 {
		writeRPCError(w, nil, codeInvalidRequest, "empty request")
		return
	}

	ctx := context.WithValue(r.Context(), ctxKeyWriter, w)
	ctx = context.WithValue(ctx, ctxKeyRequest, r)

	if body[0] == '[' {
		h.handleRPCBatch(ctx, w, body)
	} else {
		h.handleRPCSingle(ctx, w, body)
	}
}

func (h *Handler) handleRPCSingle(ctx context.Context, w http.ResponseWriter, body []byte) {
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPCError(w, nil, codeParseError, "parse error")
		return
	}

	resp := h.dispatchRPC(ctx, &req)

	// Notification (no id) → no response.
	// 通知 (id なし) → レスポンスなし。
	if req.ID == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleRPCBatch(ctx context.Context, w http.ResponseWriter, body []byte) {
	var reqs []rpcRequest
	if err := json.Unmarshal(body, &reqs); err != nil {
		writeRPCError(w, nil, codeParseError, "parse error")
		return
	}

	if len(reqs) == 0 {
		writeRPCError(w, nil, codeInvalidRequest, "empty batch")
		return
	}

	// Limit batch size to prevent resource exhaustion.
	// バッチサイズを制限してリソース枯渇を防ぐ。
	const maxBatchSize = 100
	if len(reqs) > maxBatchSize {
		writeRPCError(w, nil, codeInvalidRequest, "batch too large")
		return
	}

	var responses []rpcResponse
	for i := range reqs {
		resp := h.dispatchRPC(ctx, &reqs[i])
		// Skip notifications (no id).
		if reqs[i].ID != nil {
			responses = append(responses, resp)
		}
	}

	if len(responses) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(responses)
}

func (h *Handler) dispatchRPC(ctx context.Context, req *rpcRequest) rpcResponse {
	if req.JSONRPC != "2.0" {
		return rpcResponse{JSONRPC: "2.0", Error: newRPCErr(codeInvalidRequest, "invalid jsonrpc version"), ID: req.ID}
	}

	def, ok := h.rpcMethods[req.Method]
	if !ok {
		return rpcResponse{JSONRPC: "2.0", Error: newRPCErr(codeMethodNotFound, "method not found"), ID: req.ID}
	}

	// Auth check for non-public methods.
	// 非公開メソッドの認証チェック。
	if !def.public {
		if !h.isRPCAuthed(ctx) {
			return rpcResponse{JSONRPC: "2.0", Error: newRPCErr(codeUnauthorized, "unauthorized"), ID: req.ID}
		}
	}

	result, rpcError := def.handler(ctx, req.Params)
	if rpcError != nil {
		return rpcResponse{JSONRPC: "2.0", Error: rpcError, ID: req.ID}
	}
	return rpcResponse{JSONRPC: "2.0", Result: result, ID: req.ID}
}

// isRPCAuthed checks authentication via session cookie or bearer token (RPC context).
// セッション Cookie または Bearer トークンで認証を確認する (RPC コンテキスト用)。
func (h *Handler) isRPCAuthed(ctx context.Context) bool {
	r := rpcRequest_(ctx)
	if r == nil {
		return false
	}
	return h.isHTTPAuthed(r)
}

// isHTTPAuthed checks authentication via session cookie or bearer token.
// セッション Cookie または Bearer トークンで認証を確認する。
func (h *Handler) isHTTPAuthed(r *http.Request) bool {
	ctx := r.Context()

	// Check session cookie. / セッション Cookie を確認。
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		hash := hashToken(cookie.Value)
		sess, err := h.store.GetSession(ctx, hash)
		if err == nil && sess.ExpiresAt.After(time.Now()) {
			return true
		}
	}

	// Check Bearer token (with expiry check). / Bearer トークンを確認 (期限チェック付き)。
	if token := extractBearerToken(r); token != "" {
		hash := hashToken(token)
		if tok, err := h.store.GetAPIToken(ctx, hash); err == nil {
			if tok.ExpiresAt.IsZero() || tok.ExpiresAt.After(time.Now()) {
				return true
			}
		}
	}

	return false
}

// writeRPCError writes a JSON-RPC error response for protocol-level errors.
func writeRPCError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		Error:   newRPCErr(code, msg),
		ID:      id,
	})
}

// trimLeft skips leading whitespace.
func trimLeft(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\n' || b[0] == '\r') {
		b = b[1:]
	}
	return b
}

// parseParams is a helper to decode JSON-RPC params into a struct.
// JSON-RPC の params を構造体にデコードするヘルパー。
func parseParams[T any](params json.RawMessage) (T, *rpcErr) {
	var v T
	if len(params) == 0 || string(params) == "null" {
		return v, nil
	}
	if err := json.Unmarshal(params, &v); err != nil {
		return v, newRPCErr(codeInvalidParams, "invalid params")
	}
	return v, nil
}

// clampLimit constrains a limit value to [1, max], defaulting to def if <= 0.
// limit 値を [1, max] の範囲に制限する。0 以下の場合は def を使う。
func clampLimit(val, def, max int) int {
	if val < 1 {
		return def
	}
	if val > max {
		return max
	}
	return val
}
