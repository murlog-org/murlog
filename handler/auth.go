package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"time"
)

const (
	// sessionCookieName is the name of the session cookie.
	// セッション Cookie の名前。
	sessionCookieName = "murlog_session"

	// sessionDuration is how long a session lasts.
	// セッションの有効期間。
	sessionDuration = 14 * 24 * time.Hour
)

// generateToken creates a cryptographically random token (32 bytes, hex encoded).
// 暗号学的に安全なランダムトークンを生成する (32バイト、hex エンコード)。
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken returns the SHA-256 hex digest of a token.
// トークンの SHA-256 hex ダイジェストを返す。
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// isSecure returns true when the connection should be treated as HTTPS.
// Uses the protocol DB setting so that Secure cookies work behind reverse proxies.
// リバースプロキシ背後でも Secure Cookie が有効になるよう protocol 設定を参照する。
func (h *Handler) isSecure(r *http.Request) bool {
	return h.protocol(r) == "https"
}

// setCSRFToken generates a CSRF token, sets it as a cookie, and returns it.
// Double Submit Cookie pattern: token is placed in both a cookie and a hidden form field.
// CSRF トークンを生成し Cookie にセットして返す。
// Double Submit Cookie パターン: Cookie と hidden フィールドの両方にトークンを配置。
func (h *Handler) setCSRFToken(w http.ResponseWriter, r *http.Request) string {
	token, _ := generateToken()
	http.SetCookie(w, &http.Cookie{
		Name:     "_csrf",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isSecure(r),
		SameSite: http.SameSiteStrictMode,
	})
	return token
}

// validateCSRFToken checks that the form field matches the cookie value.
// フォームフィールドと Cookie の値が一致するか検証する。
func validateCSRFToken(r *http.Request) bool {
	cookie, err := r.Cookie("_csrf")
	if err != nil || cookie.Value == "" {
		return false
	}
	formToken := r.FormValue("_csrf")
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(formToken)) == 1
}

// extractBearerToken extracts the token from "Authorization: Bearer <token>".
// "Authorization: Bearer <token>" からトークンを取り出す。
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}
