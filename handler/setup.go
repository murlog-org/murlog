package handler

import (
	"context"
	"crypto/subtle"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/activitypub"
	"github.com/murlog-org/murlog/config"
	"github.com/murlog-org/murlog/i18n"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/queue/sqlqueue"
	"github.com/murlog-org/murlog/store"
	"golang.org/x/crypto/bcrypt"
)

// adminCSS is the shared CSS for admin SSR pages.
// 管理 SSR ページ共通の CSS。
const adminCSS = `
  :root {
    --ink: #1d1d1f;
    --paper: #ffffff;
    --secondary: #f5f5f7;
    --accent: #3a7ca5;
    --accent-soft: #eef4f8;
    --muted: #86868b;
    --border: #d2d2d7;
    --danger: #dc2626;
    --radius: 6px;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --ink: #e5e5e7;
      --paper: #161618;
      --secondary: #1c1c1e;
      --accent: #5aaddb;
      --accent-soft: #1a2a36;
      --muted: #7c7c80;
      --border: #303034;
      --danger: #ef4444;
    }
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Helvetica Neue", "Hiragino Sans", "Noto Sans JP", system-ui, sans-serif;
    background: var(--secondary);
    color: var(--ink);
    line-height: 1.6;
    -webkit-font-smoothing: antialiased;
  }
  .screen { max-width: 480px; margin: 48px auto; padding: 0 16px; }
  .screen h2 {
    font-size: 18px; font-weight: 600;
    margin-bottom: 20px; padding-bottom: 10px;
    border-bottom: 1px solid var(--border);
  }
  .card {
    background: var(--paper); border: 1px solid var(--border);
    border-radius: var(--radius); padding: 20px;
  }
  .welcome { font-size: 14px; color: var(--muted); margin-bottom: 16px; }
  h3 { font-size: 13px; font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: 0.05em; margin: 20px 0 8px; }
  h3:first-child { margin-top: 0; }
  .input-group { margin-bottom: 12px; }
  .input-group label { display: block; font-size: 13px; font-weight: 600; margin-bottom: 4px; }
  .input-group input {
    width: 100%; padding: 8px 10px;
    border: 1px solid var(--border); border-radius: var(--radius);
    font-size: 14px; font-family: inherit;
    background: var(--paper); color: var(--ink);
    transition: border-color 0.15s;
  }
  .input-group input:focus { outline: none; border-color: var(--accent); }
  .actions { text-align: right; margin-top: 20px; }
  .btn {
    display: inline-block; padding: 8px 24px; border: none;
    border-radius: var(--radius); cursor: pointer;
    font-size: 14px; font-family: inherit; font-weight: 500;
    background: var(--accent); color: #fff;
    transition: opacity 0.15s;
  }
  .btn:hover { opacity: 0.85; }
  .error {
    background: #fef2f2; border: 1px solid var(--danger); color: var(--danger);
    padding: 10px 12px; border-radius: var(--radius);
    font-size: 13px; margin-bottom: 16px;
  }
  @media (prefers-color-scheme: dark) {
    .error { background: #2a1618; }
  }
  .logo { text-align: center; margin-bottom: 8px; }
  .hint { font-size: 12px; color: var(--muted); margin-top: 2px; }
`

const adminLogoSVG = `<svg viewBox="0 0 240 240" width="40" height="40" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
  <circle cx="56" cy="52" r="8" fill="currentColor" opacity="0.35"/>
  <circle cx="120" cy="52" r="8" fill="currentColor" opacity="0.35"/>
  <circle cx="184" cy="52" r="8" fill="currentColor" opacity="0.35"/>
  <circle cx="56" cy="90" r="10" fill="currentColor" opacity="0.5"/>
  <circle cx="88" cy="90" r="10" fill="currentColor" opacity="0.5"/>
  <circle cx="120" cy="90" r="10" fill="currentColor" opacity="0.5"/>
  <circle cx="152" cy="90" r="10" fill="currentColor" opacity="0.5"/>
  <circle cx="184" cy="90" r="10" fill="currentColor" opacity="0.5"/>
  <circle cx="56" cy="132" r="12" fill="currentColor" opacity="0.7"/>
  <circle cx="120" cy="132" r="12" fill="currentColor" opacity="0.7"/>
  <circle cx="184" cy="132" r="12" fill="currentColor" opacity="0.7"/>
  <circle cx="56" cy="176" r="14" fill="currentColor"/>
  <circle cx="120" cy="176" r="14" fill="currentColor"/>
  <circle cx="184" cy="176" r="14" fill="currentColor"/>
  <circle cx="184" cy="214" r="16" fill="var(--accent)"/>
</svg>`

// ──────────────────────────────────────────────
// Step 1: Server configuration (murlog.ini)
// ──────────────────────────────────────────────

// step1Data is the template data for the server config form.
// サーバー設定フォームのテンプレートデータ。
// setupTokenEnv is the environment variable for optional setup authentication.
// セットアップ認証用のオプション環境変数。
const setupTokenEnv = "MURLOG_SETUP_TOKEN"

// setupTokenFile is the file for optional setup authentication (CGI environments).
// セットアップ認証用のオプションファイル (CGI 環境向け)。
const setupTokenFile = "murlog.setup-token"

// getSetupToken returns the setup token from env var or file, or empty if neither exists.
// 環境変数 → ファイルの順でセットアップトークンを取得。どちらもなければ空文字。
func getSetupToken() string {
	if v := os.Getenv(setupTokenEnv); v != "" {
		return strings.TrimSpace(v)
	}
	if data, err := os.ReadFile(setupTokenFile); err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			return token
		}
	}
	return ""
}

type step1Data struct {
	T            func(string) string
	Lang         string
	Error        string
	CSRFToken    string
	RequireToken bool
	DataDir   string
	MediaPath string
}

var step1Tmpl = template.Must(template.New("step1").Parse(`<!DOCTYPE html>
<html lang="{{.Lang}}">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>murlog — {{call .T "setup.step1.title"}}</title>
<style>` + adminCSS + `</style>
</head>
<body>
<div class="screen">
  <div class="logo">` + adminLogoSVG + `</div>
  <h2>{{call .T "setup.step1.title"}}</h2>
  <div class="card">
    <p class="welcome">{{call .T "setup.step1.welcome"}}</p>
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
    <form method="POST" action="/admin/setup/server">
      <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
      {{if .RequireToken}}
      <div class="input-group">
        <label for="setup_token">Setup Token</label>
        <input type="password" id="setup_token" name="setup_token" required>
        <div class="hint">MURLOG_SETUP_TOKEN or murlog.setup-token</div>
      </div>
      {{end}}
      <h3>{{call .T "setup.step1.storage"}}</h3>
      <div class="input-group">
        <label for="data_dir">{{call .T "setup.step1.data_dir"}}</label>
        <input type="text" id="data_dir" name="data_dir" value="{{.DataDir}}" required>
        <div class="hint">e.g. ./data</div>
      </div>
      <div class="input-group">
        <label for="media_path">{{call .T "setup.step1.media_path"}}</label>
        <input type="text" id="media_path" name="media_path" value="{{.MediaPath}}" required>
        <div class="hint">e.g. ./media</div>
      </div>
      <div class="actions">
        <button class="btn" type="submit">{{call .T "setup.step1.submit"}}</button>
      </div>
    </form>
  </div>
</div>
</body>
</html>`))

// handleStep1Form serves the server configuration form (Step 1).
// サーバー設定フォームを返す (Step 1)。
func (h *Handler) handleStep1Form(w http.ResponseWriter, r *http.Request) {
	if config.Exists(h.cfg.Path) {
		http.Redirect(w, r, "/admin/setup", http.StatusSeeOther)
		return
	}
	lang := i18n.DetectLang(r)
	defaults := config.Defaults()
	csrfToken := h.setCSRFToken(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	step1Tmpl.Execute(w, &step1Data{
		T:            func(key string) string { return i18n.T(lang, key) },
		Lang:         lang,
		CSRFToken:    csrfToken,
		RequireToken: getSetupToken() != "",
		DataDir:   defaults.DataDir,
		MediaPath: defaults.MediaPath,
	})
}

// handleStep1Submit processes the server configuration form (Step 1).
// サーバー設定フォームの送信を処理する (Step 1)。
func (h *Handler) handleStep1Submit(w http.ResponseWriter, r *http.Request) {
	if config.Exists(h.cfg.Path) {
		http.Redirect(w, r, "/admin/setup", http.StatusSeeOther)
		return
	}
	if !validateCSRFToken(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	if token := getSetupToken(); token != "" {
		if subtle.ConstantTimeCompare([]byte(r.FormValue("setup_token")), []byte(token)) != 1 {
			http.Error(w, "invalid setup token", http.StatusForbidden)
			return
		}
	}

	lang := i18n.DetectLang(r)
	dataDir := r.FormValue("data_dir")
	mediaPath := r.FormValue("media_path")

	if dataDir == "" || mediaPath == "" {
		csrfToken := h.setCSRFToken(w, r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		step1Tmpl.Execute(w, &step1Data{
			T:            func(key string) string { return i18n.T(lang, key) },
			Lang:         lang,
			Error:        i18n.T(lang, "setup.step1.err.required"),
			CSRFToken:    csrfToken,
			RequireToken: getSetupToken() != "",
			DataDir:      dataDir,
			MediaPath:    mediaPath,
		})
		return
	}

	// Update config and save to disk. / 設定を更新してディスクに保存。
	oldDataDir := h.cfg.DataDir
	h.cfg.DataDir = dataDir
	h.cfg.MediaPath = mediaPath
	if err := h.cfg.Save(); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		step1Tmpl.Execute(w, &step1Data{
			T:         func(key string) string { return i18n.T(lang, key) },
			Lang:      lang,
			Error:     i18n.T(lang, "setup.step1.err.save"),
			DataDir:   dataDir,
			MediaPath: mediaPath,
		})
		return
	}

	// Resolve to absolute path for accurate comparison and usage.
	// 正確な比較と利用のため絶対パスに変換。
	absDir, _ := filepath.Abs(dataDir)
	if absDir != "" {
		h.cfg.DataDir = absDir
	}

	// Reconnect store if data dir actually changed.
	// データディレクトリが実際に変わった場合のみ再接続。
	if h.cfg.DataDir != oldDataDir {
		// Ensure directory exists. / ディレクトリの存在を保証する。
		os.MkdirAll(h.cfg.DataDir, 0755)
		newStore, err := store.Open(h.cfg.DBDriver, h.cfg.MainDBPath())
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			step1Tmpl.Execute(w, &step1Data{
				T:         func(key string) string { return i18n.T(lang, key) },
				Lang:      lang,
				Error:     i18n.T(lang, "setup.step1.err.save"),
				DataDir:   dataDir,
				MediaPath: mediaPath,
			})
			return
		}
		// Migrate the new DB. / 新しい DB をマイグレーション。
		if err := newStore.Migrate(r.Context()); err != nil {
			newStore.Close()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			step1Tmpl.Execute(w, &step1Data{
				T:         func(key string) string { return i18n.T(lang, key) },
				Lang:      lang,
				Error:     i18n.T(lang, "setup.err.migration"),
				DataDir:   dataDir,
				MediaPath: mediaPath,
			})
			return
		}
		h.store.Close()
		h.store = newStore
		h.queue = sqlqueue.New(newStore.DB())
	}

	http.Redirect(w, r, "/admin/setup", http.StatusSeeOther)
}

// ──────────────────────────────────────────────
// Step 2: Site setup (DB initialization)
// ──────────────────────────────────────────────

// setupData is the template data for the setup form.
// セットアップフォームのテンプレートデータ。
type setupData struct {
	T            func(string) string
	Lang         string
	Error        string
	CSRFToken    string
	RequireToken bool
	Domain       string
	Username     string
	DisplayName  string
}

var setupTmpl = template.Must(template.New("setup").Parse(`<!DOCTYPE html>
<html lang="{{.Lang}}">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>murlog — {{call .T "setup.title"}}</title>
<style>` + adminCSS + `</style>
</head>
<body>
<div class="screen">
  <div class="logo">` + adminLogoSVG + `</div>
  <h2>{{call .T "setup.title"}}</h2>
  <div class="card">
    <p class="welcome">{{call .T "setup.welcome"}}</p>
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
    <form method="POST" action="/admin/setup">
      <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
      {{if .RequireToken}}
      <div class="input-group">
        <label for="setup_token">Setup Token</label>
        <input type="password" id="setup_token" name="setup_token" required>
        <div class="hint">MURLOG_SETUP_TOKEN or murlog.setup-token</div>
      </div>
      {{end}}
      <h3>{{call .T "setup.server"}}</h3>
      <div class="input-group">
        <label for="domain">{{call .T "setup.domain"}}</label>
        <input type="text" id="domain" name="domain" placeholder="example.com" value="{{.Domain}}" required>
      </div>

      <h3>{{call .T "setup.persona"}}</h3>
      <div class="input-group">
        <label for="username">{{call .T "setup.username"}}</label>
        <input type="text" id="username" name="username" placeholder="main" value="{{.Username}}" required>
      </div>
      <div class="input-group">
        <label for="display_name">{{call .T "setup.display_name"}}</label>
        <input type="text" id="display_name" name="display_name" value="{{.DisplayName}}">
      </div>

      <h3>{{call .T "setup.auth"}}</h3>
      <div class="input-group">
        <label for="password">{{call .T "setup.password"}}</label>
        <input type="password" id="password" name="password" required>
      </div>
      <div class="input-group">
        <label for="password_confirm">{{call .T "setup.password_confirm"}}</label>
        <input type="password" id="password_confirm" name="password_confirm" required>
      </div>

      <div class="actions">
        <button class="btn" type="submit">{{call .T "setup.submit"}}</button>
      </div>
    </form>
  </div>
</div>
</body>
</html>`))

// handleSetupForm serves the initial setup form.
// 初期セットアップフォームを返す。
func (h *Handler) handleSetupForm(w http.ResponseWriter, r *http.Request) {
	if h.isSetupComplete(r) {
		http.Redirect(w, r, "/my/", http.StatusSeeOther)
		return
	}
	lang := i18n.DetectLang(r)
	csrfToken := h.setCSRFToken(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	setupTmpl.Execute(w, &setupData{
		T:            func(key string) string { return i18n.T(lang, key) },
		Lang:         lang,
		CSRFToken:    csrfToken,
		RequireToken: getSetupToken() != "",
		Domain:       guessHost(r),
	})
}

// guessHost extracts the hostname from the request, stripping port.
// リクエストからホスト名を取り出す (ポートは除去)。
func guessHost(r *http.Request) string {
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	// Strip port. / ポートを除去。
	if i := strings.LastIndex(host, ":"); i != -1 {
		return host[:i]
	}
	return host
}

// renderSetupError re-renders the setup form with an error message and preserved input.
// エラーメッセージと入力値を保持してセットアップフォームを再表示する。
func (h *Handler) renderSetupError(w http.ResponseWriter, r *http.Request, errKey string) {
	lang := i18n.DetectLang(r)
	csrfToken := h.setCSRFToken(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	setupTmpl.Execute(w, &setupData{
		T:            func(key string) string { return i18n.T(lang, key) },
		Lang:         lang,
		Error:        i18n.T(lang, errKey),
		CSRFToken:    csrfToken,
		RequireToken: getSetupToken() != "",
		Domain:       r.FormValue("domain"),
		Username:     r.FormValue("username"),
		DisplayName:  r.FormValue("display_name"),
	})
}

// handleSetupSubmit processes the initial setup form.
// 初期セットアップフォームの送信を処理する。
func (h *Handler) handleSetupSubmit(w http.ResponseWriter, r *http.Request) {
	if h.isSetupComplete(r) {
		http.Redirect(w, r, "/my/", http.StatusSeeOther)
		return
	}

	if !validateCSRFToken(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	if token := getSetupToken(); token != "" {
		if subtle.ConstantTimeCompare([]byte(r.FormValue("setup_token")), []byte(token)) != 1 {
			http.Error(w, "invalid setup token", http.StatusForbidden)
			return
		}
	}

	ctx := r.Context()
	domain := r.FormValue("domain")
	username := r.FormValue("username")
	displayName := r.FormValue("display_name")
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	// Validate inputs. / 入力を検証。
	if domain == "" || username == "" || password == "" {
		h.renderSetupError(w, r, "setup.err.required")
		return
	}
	if err := murlog.ValidateUsername(username); err != nil {
		h.renderSetupError(w, r, err.Error())
		return
	}
	if password != confirm {
		h.renderSetupError(w, r, "setup.err.password_mismatch")
		return
	}
	if err := murlog.ValidatePassword(password); err != nil {
		h.renderSetupError(w, r, err.Error())
		return
	}

	// Generate key pair for the persona. / ペルソナ用の鍵ペアを生成。
	pubKey, privKey, err := activitypub.GenerateKeyPair()
	if err != nil {
		h.renderSetupError(w, r, "setup.err.keygen")
		return
	}

	// Create the primary persona. / プライマリペルソナを作成。
	now := time.Now()
	persona := &murlog.Persona{
		ID:            id.New(),
		Username:      username,
		DisplayName:   displayName,
		PublicKeyPEM:  pubKey,
		PrivateKeyPEM: privKey,
		Primary:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := h.store.CreatePersona(ctx, persona); err != nil {
		h.renderSetupError(w, r, "setup.err.persona")
		return
	}

	// Hash and store password. / パスワードをハッシュして保存。
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		h.renderSetupError(w, r, "setup.err.internal")
		return
	}

	// Save settings. / 設定を保存。
	protocol := r.FormValue("protocol")
	if protocol != "http" {
		protocol = "https"
	}
	settings := map[string]string{
		SettingDomain:        domain,
		SettingProtocol:      protocol,
		SettingPasswordHash:  string(hash),
		SettingSetupComplete: "true",
	}
	for k, v := range settings {
		if err := h.store.SetSetting(ctx, k, v); err != nil {
			h.renderSetupError(w, r, "setup.err.settings")
			return
		}
	}

	http.Redirect(w, r, "/my/login", http.StatusSeeOther)
}

// isSetupComplete checks if the initial setup has been completed.
// 初期セットアップが完了しているか確認する。
func (h *Handler) isSetupComplete(r *http.Request) bool {
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	v, _ := h.store.GetSetting(ctx, SettingSetupComplete)
	return v == "true"
}
