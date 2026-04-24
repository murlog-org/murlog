package handler

import (
	"crypto/subtle"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/murlog-org/murlog"
	"golang.org/x/crypto/bcrypt"
)

const (
	// resetFileName is the file that triggers password reset.
	// パスワードリセットをトリガーするファイル名。
	resetFileName = "murlog.reset"
)

// resetFilePath returns the path to the reset file (next to the DB file).
// リセットファイルのパスを返す (DB ファイルと同じディレクトリ)。
func (h *Handler) resetFilePath() string {
	return filepath.Join(h.cfg.DataDir, resetFileName)
}

// isResetMode checks if password reset mode is active.
// パスワードリセットモードが有効か確認する。
//
// Conditions:
//   - murlog.reset file exists
//   - File mtime > DB's last_password_reset_at
//   - File contains a non-empty token
//
// 条件:
//   - murlog.reset ファイルが存在する
//   - ファイルの mtime > DB の last_password_reset_at
//   - ファイルに空でないトークンが含まれている
func (h *Handler) isResetMode(r *http.Request) bool {
	info, err := os.Stat(h.resetFilePath())
	if err != nil {
		return false
	}

	// Check mtime against last reset. / mtime を最終リセット日時と比較。
	lastReset, _ := h.store.GetSetting(r.Context(), SettingLastPasswordReset)
	if lastReset != "" {
		lastResetTime, err := time.Parse(time.RFC3339, lastReset)
		if err == nil && !info.ModTime().After(lastResetTime) {
			return false
		}
	}

	// Check file has content (token). / ファイルにトークンが含まれているか確認。
	token, _ := h.readResetToken()
	return token != ""
}

// readResetToken reads and trims the token from the reset file.
// リセットファイルからトークンを読み取り、トリムする。
func (h *Handler) readResetToken() (string, error) {
	data, err := os.ReadFile(h.resetFilePath())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// handleResetForm serves the password reset form.
// パスワードリセットフォームを返す。
func (h *Handler) handleResetForm(w http.ResponseWriter, r *http.Request) {
	if !h.isResetMode(r) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Verify token in query matches file. / クエリのトークンがファイルと一致するか確認。
	queryToken := r.URL.Query().Get("token")
	fileToken, _ := h.readResetToken()
	if queryToken == "" || subtle.ConstantTimeCompare([]byte(queryToken), []byte(fileToken)) != 1 {
		http.Error(w, "invalid token", http.StatusForbidden)
		return
	}

	csrfToken := h.setCSRFToken(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	escapedToken := template.HTMLEscapeString(fileToken)
	escapedCSRF := template.HTMLEscapeString(csrfToken)
	w.Write([]byte(`<form method="POST" action="/admin/reset">
		<input type="hidden" name="token" value="` + escapedToken + `">
		<input type="hidden" name="_csrf" value="` + escapedCSRF + `">
		<input type="password" name="password" placeholder="New password" required>
		<input type="password" name="password_confirm" placeholder="Confirm" required>
		<button type="submit">Reset Password</button>
	</form>`))
}

// handleResetSubmit processes the password reset form.
// パスワードリセットフォームの送信を処理する。
func (h *Handler) handleResetSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.isResetMode(r) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if !validateCSRFToken(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	// Verify token. / トークンを検証。
	formToken := r.FormValue("token")
	fileToken, _ := h.readResetToken()
	if formToken == "" || subtle.ConstantTimeCompare([]byte(formToken), []byte(fileToken)) != 1 {
		http.Error(w, "invalid token", http.StatusForbidden)
		return
	}

	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")
	if password == "" || password != confirm {
		http.Error(w, "passwords do not match", http.StatusBadRequest)
		return
	}
	if err := murlog.ValidatePassword(password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Hash and store new password. / 新パスワードをハッシュして保存。
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	if err := h.store.SetSetting(ctx, SettingPasswordHash, string(hash)); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Record reset time (invalidates the reset file). / リセット日時を記録 (リセットファイルを無効化)。
	if err := h.store.SetSetting(ctx, SettingLastPasswordReset, time.Now().UTC().Format(time.RFC3339)); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Try to delete the reset file (best effort). / リセットファイルの削除を試みる (ベストエフォート)。
	os.Remove(h.resetFilePath())

	http.Redirect(w, r, "/my/login", http.StatusSeeOther)
}
