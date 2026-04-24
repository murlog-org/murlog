// Package handler provides the root HTTP handler and route registration for murlog.
// murlog のルート HTTP ハンドラとルーティング登録を提供するパッケージ。
package handler

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/config"
	"github.com/murlog-org/murlog/media"
	"github.com/murlog-org/murlog/queue"
	"github.com/murlog-org/murlog/store"
	"github.com/murlog-org/murlog/worker"
	"net"
)

// SetupPhase represents the current state of the setup wizard.
// セットアップウィザードの現在のフェーズ。
type SetupPhase int

const (
	SetupDone    SetupPhase = iota // Setup complete / セットアップ完了
	SetupStep1                     // murlog.ini not found / murlog.ini 未作成
	SetupStep2                     // toml exists, DB not initialized / toml あり、DB 未初期化
)

// DB settings keys used by handler.
// handler が使用する DB settings のキー。
const (
	SettingDomain            = "domain"
	SettingProtocol          = "protocol"
	SettingPasswordHash      = "password_hash"
	SettingSetupComplete     = "setup_complete"
	SettingLastPasswordReset = "last_password_reset_at"
	SettingWorkerSecret      = "worker_secret"
	SettingTOTPSecret        = "totp_secret"
	SettingRobotsNoIndex     = "robots_noindex"     // "true" = noindex / クローラーインデックス拒否
	SettingRobotsNoAI        = "robots_noai"         // "true" = noai / AI学習拒否
)

type Handler struct {
	cfg         *config.Config
	store       store.Store
	queue       queue.Queue
	worker      *worker.Worker
	media       media.Store
	mux         *http.ServeMux
	rpcMethods  map[string]methodDef
	homeTmpl    *template.Template
	profileTmpl *template.Template
	postTmpl    *template.Template
	tagTmpl     *template.Template
}

// New creates a new Handler with the given dependencies.
// 依存を受け取って新しい Handler を生成する。
func New(cfg *config.Config, s store.Store, q queue.Queue, w *worker.Worker, m media.Store) *Handler {
	h := &Handler{
		cfg:    cfg,
		store:  s,
		queue:  q,
		worker: w,
		media:  m,
		mux:    http.NewServeMux(),
	}
	h.rpcMethods = h.registerRPCMethods()
	h.registerRoutes()
	h.loadSSRTemplates()
	return h
}

// serverRoutePrefix lists URL prefixes handled by Go (not SPA).
// Go 側で処理する URL プレフィックス一覧 (SPA ではない)。
var serverRoutePrefixes = []string{
	"/admin/",
	"/api/",
	"/oauth/",
	"/users/",
	"/.well-known/",
	"/nodeinfo/",
	"/tags/",
}

// isServerRoute returns true if the path should be handled by Go, not SPA.
// Go が処理すべきパスなら true を返す。
func isServerRoute(path string) bool {
	if path == "/robots.txt" {
		return true
	}
	for _, p := range serverRoutePrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// ServeHTTP implements http.Handler.
// Setup guard → server routes (API/ActivityPub/Admin) → SPA fallback の順で処理。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// セキュリティヘッダを設定。 / Set security headers.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' https:; media-src 'self' https:")
	if h.isSecure(r) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	}

	path := r.URL.Path

	// Media files bypass setup guard and serve directly.
	// メディアファイルは setup guard をバイパスして直接配信。
	if strings.HasPrefix(path, "/media/") {
		h.serveMedia(w, r)
		return
	}

	// Static assets bypass setup guard (needed for SPA CSS/JS and PWA to load).
	// 静的アセットは setup guard をバイパス (SPA の CSS/JS と PWA のロードに必要)。
	if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/themes/") || strings.HasPrefix(path, "/locales/") ||
		path == "/dist/sw.js" || path == "/dist/manifest.webmanifest" || strings.HasPrefix(path, "/dist/icon-") {
		h.serveSPA(w, r)
		return
	}

	// Health check endpoint — bypass setup guard so monitoring works before setup.
	// ヘルスチェックエンドポイント — セットアップ前でも監視が動くよう guard をバイパス。
	if path == "/healthz" {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("ok"))
		return
	}

	// Setup guard: redirect to the appropriate setup step.
	// Only redirect HTML navigations — resource fetches (favicon, robots.txt, etc.)
	// must not trigger a redirect that overwrites the CSRF cookie.
	// セットアップガード: 適切なセットアップステップにリダイレクト。
	// HTML ナビゲーションのみリダイレクト — リソース取得 (favicon 等) はリダイレクトしない
	// (CSRF cookie の上書きを防ぐ)。
	if phase := h.setupPhase(); phase != SetupDone {
		if !strings.HasPrefix(path, "/admin/") {
			if !strings.Contains(r.Header.Get("Accept"), "text/html") {
				http.NotFound(w, r)
				return
			}
			switch phase {
			case SetupStep1:
				http.Redirect(w, r, "/admin/setup/server", http.StatusSeeOther)
			case SetupStep2:
				http.Redirect(w, r, "/admin/setup", http.StatusSeeOther)
			}
			return
		}
	}

	// Server routes: API, ActivityPub, Admin SSR — let mux handle.
	// サーバールート: API, ActivityPub, Admin SSR — mux に処理させる。
	if isServerRoute(path) {
		h.mux.ServeHTTP(w, r)
		return
	}

	// Home page: SSR with all personas.
	// ホームページ: 全ペルソナの SSR。
	if path == "/" {
		h.renderHome(w, r)
		return
	}

	// SPA fallback: serve static file or index.html.
	// SPA フォールバック: 静的ファイルまたは index.html を返す。
	h.serveSPA(w, r)
}

// serveSPA serves a static file from WebDir, or falls back to index.html.
// WebDir から静的ファイルを配信するか、index.html にフォールバックする。
func (h *Handler) serveSPA(w http.ResponseWriter, r *http.Request) {
	webDir := h.cfg.WebDir
	if webDir == "" {
		http.NotFound(w, r)
		return
	}

	// Try serving the exact file (e.g. /assets/index-xxx.js).
	// 正確なファイルを配信する (例: /assets/index-xxx.js)。
	filePath := filepath.Join(webDir, filepath.Clean(r.URL.Path))
	// Ensure resolved path stays within webDir (prevent path traversal).
	// 解決済みパスが webDir 内に留まることを確認 (パストラバーサル防止)。
	if !strings.HasPrefix(filePath, filepath.Clean(webDir)+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, filePath)
		return
	}

	// Fall back to index.html for SPA routing.
	// SPA ルーティングのため index.html にフォールバック。
	indexPath := filepath.Join(webDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, indexPath)
}

// setupPhase returns the current setup phase.
// 現在のセットアップフェーズを返す。
func (h *Handler) setupPhase() SetupPhase {
	if !config.Exists(h.cfg.Path) {
		return SetupStep1
	}
	if !h.isSetupComplete(nil) {
		return SetupStep2
	}
	return SetupDone
}

// domain returns the configured domain from DB settings.
// DB settings からドメインを取得する。
func (h *Handler) domain(r *http.Request) string {
	d, _ := h.store.GetSetting(r.Context(), SettingDomain)
	return d
}

// resolveProtocol returns the protocol, preferring MURLOG_PROTOCOL env var for local dev.
// プロトコルを返す。ローカル開発用に MURLOG_PROTOCOL 環境変数を優先。
func resolveProtocol(dbValue string) string {
	if env := os.Getenv("MURLOG_PROTOCOL"); env == "http" || env == "https" {
		return env
	}
	if dbValue == "http" {
		return "http"
	}
	return "https"
}

// protocol returns "https" or "http" from DB settings (default "https").
// MURLOG_PROTOCOL 環境変数で override 可能（ローカル開発用）。
func (h *Handler) protocol(r *http.Request) string {
	p, _ := h.store.GetSetting(r.Context(), SettingProtocol)
	return resolveProtocol(p)
}

// baseURL returns the base URL of this instance (e.g. "https://example.com").
// このインスタンスのベース URL を返す (例: "https://example.com")。
func (h *Handler) baseURL(r *http.Request) string {
	return h.protocol(r) + "://" + h.domain(r)
}

// baseURLFromCtx returns the base URL using context (for use in RPC handlers).
// ctx からベース URL を返す (RPC ハンドラ用)。
func (h *Handler) baseURLFromCtx(ctx context.Context) string {
	p, _ := h.store.GetSetting(ctx, SettingProtocol)
	p = resolveProtocol(p)
	d, _ := h.store.GetSetting(ctx, SettingDomain)
	return p + "://" + d
}

// primaryPersona returns the primary persona.
// プライマリペルソナを返す。
func (h *Handler) primaryPersona(ctx context.Context) (*murlog.Persona, error) {
	personas, err := h.store.ListPersonas(ctx)
	if err != nil || len(personas) == 0 {
		return nil, fmt.Errorf("no personas")
	}
	for _, p := range personas {
		if p.Primary {
			return p, nil
		}
	}
	return personas[0], nil
}

func (h *Handler) registerRoutes() {
	// Admin SSR (setup & password reset) / 管理 SSR (セットアップ・パスワードリセット)
	h.mux.HandleFunc("GET /admin/setup/server", h.handleStep1Form)
	h.mux.HandleFunc("POST /admin/setup/server", h.handleStep1Submit)
	h.mux.HandleFunc("GET /admin/setup", h.handleSetupForm)
	h.mux.HandleFunc("POST /admin/setup", h.handleSetupSubmit)
	h.mux.HandleFunc("GET /admin/reset", h.handleResetForm)
	h.mux.HandleFunc("POST /admin/reset", h.handleResetSubmit)

	// robots.txt — dynamic based on settings.
	// robots.txt — 設定に基づいて動的生成。
	h.mux.HandleFunc("GET /robots.txt", h.handleRobotsTxt)

	// Tag page / タグページ
	h.mux.HandleFunc("GET /tags/{tag}", h.handleTagPage)

	// WebFinger + NodeInfo discovery
	h.mux.HandleFunc("GET /.well-known/webfinger", h.handleWebFinger)
	h.mux.HandleFunc("GET /.well-known/nodeinfo", h.handleNodeInfoDiscovery)
	h.mux.HandleFunc("GET /nodeinfo/2.0", h.handleNodeInfo)

	// ActivityPub / Actor / Public pages
	h.mux.HandleFunc("GET /users/{username}/posts/{id}", h.handlePost)
	h.mux.HandleFunc("GET /users/{username}", h.handleActor)
	h.mux.HandleFunc("POST /users/{username}/inbox", h.handleInbox)
	h.mux.HandleFunc("GET /users/{username}/outbox", h.handleOutbox)
	h.mux.HandleFunc("GET /users/{username}/followers", h.handleFollowersCollection)
	h.mux.HandleFunc("GET /users/{username}/following", h.handleFollowingCollection)
	h.mux.HandleFunc("GET /users/{username}/collections/featured", h.handleFeaturedCollection)

	// murlog API v1 — JSON-RPC 2.0 / murlog 独自 API v1
	h.mux.HandleFunc("POST /api/mur/v1/rpc", h.handleRPC)

	// Media upload (multipart, separate from JSON-RPC).
	// メディアアップロード (multipart、JSON-RPC とは別)。
	h.mux.HandleFunc("POST /api/mur/v1/media", h.handleMediaUpload)

	// DB backup download. / DB バックアップダウンロード。
	h.mux.HandleFunc("GET /api/mur/v1/backup", h.handleBackup)

	// Worker tick — process pending jobs via HTTP.
	// ワーカー tick — HTTP 経由で pending ジョブを処理。
	h.mux.HandleFunc("POST /api/worker-tick", h.handleWorkerTick)
}

// clientIP returns the client IP address, respecting trusted_proxy config.
// trusted_proxy 設定に従ってクライアント IP を返す。
func (h *Handler) clientIP(r *http.Request) string {
	if h.cfg.TrustedProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For: client, proxy1, proxy2 — take the first.
			if idx := strings.IndexByte(xff, ','); idx > 0 {
				return strings.TrimSpace(xff[:idx])
			}
			return strings.TrimSpace(xff)
		}
	}
	// Fall back to RemoteAddr (strip port).
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

