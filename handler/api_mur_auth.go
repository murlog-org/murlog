package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/totp"
	"golang.org/x/crypto/bcrypt"
)

// loginParams is the params for auth.login.
type loginParams struct {
	Password string `json:"password"`
	TOTPCode string `json:"totp_code,omitempty"` // 6-digit TOTP code (if 2FA enabled) / 6桁 TOTP コード (2FA 有効時)
}

// loginMaxAttempts is the number of failures before lockout.
// ロックアウトまでの失敗回数。
const loginMaxAttempts = 5

// loginLockDuration is the lockout duration after max attempts.
// 最大試行回数後のロック時間。
const loginLockDuration = 5 * time.Minute

// rpcAuthLogin handles auth.login.
// パスワードを検証しセッション Cookie を発行する。
func (h *Handler) rpcAuthLogin(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[loginParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.Password == "" {
		return nil, newRPCErr(codeInvalidParams, "password is required")
	}

	// Rate limit check. / レートリミットチェック。
	ip := ""
	if r := rpcRequest_(ctx); r != nil {
		ip = h.clientIP(r)
	}
	if ip != "" {
		_, lockedUntil, _ := h.store.GetLoginAttempt(ctx, ip)
		if !lockedUntil.IsZero() && time.Now().Before(lockedUntil) {
			return nil, newRPCErr(codeUnauthorized, "too many attempts, try again later")
		}
	}

	// Get password hash from DB settings. / DB settings からパスワードハッシュを取得。
	storedHash, err := h.store.GetSetting(ctx, SettingPasswordHash)
	if err != nil || storedHash == "" {
		return nil, newRPCErr(codeInternalError, "login not available")
	}

	// Verify password with bcrypt. / bcrypt でパスワードを検証。
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password)); err != nil {
		// Record failure and lock if threshold reached.
		// 失敗を記録し、閾値に達したらロック。
		if ip != "" {
			failCount, _, _ := h.store.GetLoginAttempt(ctx, ip)
			var lockUntil time.Time
			if failCount+1 >= loginMaxAttempts {
				lockUntil = time.Now().Add(loginLockDuration)
			}
			h.store.RecordLoginFailure(ctx, ip, lockUntil)
		}
		return nil, newRPCErr(codeUnauthorized, "invalid password")
	}

	// Check TOTP if enabled. / TOTP が有効なら検証。
	totpSecret, _ := h.store.GetSetting(ctx, SettingTOTPSecret)
	if totpSecret != "" {
		if req.TOTPCode == "" {
			return nil, newRPCErr(codeInvalidParams, "totp_code is required")
		}
		if !totp.Validate(totpSecret, req.TOTPCode) {
			// Count as failure for rate limiting. / レートリミット用に失敗カウント。
			if ip != "" {
				failCount, _, _ := h.store.GetLoginAttempt(ctx, ip)
				var lockUntil time.Time
				if failCount+1 >= loginMaxAttempts {
					lockUntil = time.Now().Add(loginLockDuration)
				}
				h.store.RecordLoginFailure(ctx, ip, lockUntil)
			}
			return nil, newRPCErr(codeUnauthorized, "invalid TOTP code")
		}
	}

	// Success — clear rate limit. / 成功 — レートリミットをクリア。
	if ip != "" {
		h.store.ClearLoginAttempts(ctx, ip)
	}
	// Generate session token. / セッショントークンを生成。
	token, err := generateToken()
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	now := time.Now()
	sess := &murlog.Session{
		ID:        id.New(),
		TokenHash: hashToken(token),
		ExpiresAt: now.Add(sessionDuration),
		CreatedAt: now,
	}
	if err := h.store.CreateSession(ctx, sess); err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	// Set HttpOnly cookie via ResponseWriter.
	// ResponseWriter 経由で HttpOnly Cookie をセット。
	if w := rpcWriter(ctx); w != nil {
		r := rpcRequest_(ctx)
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   h.isSecure(r),
			SameSite: http.SameSiteStrictMode,
			MaxAge:   int(sessionDuration.Seconds()),
		})
	}

	return statusOK, nil
}

// changePasswordParams is the params for auth.change_password.
type changePasswordParams struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// rpcAuthChangePassword handles auth.change_password.
// 旧パスワードを検証し、新パスワードに変更する。
func (h *Handler) rpcAuthChangePassword(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[changePasswordParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return nil, newRPCErr(codeInvalidParams, "current_password and new_password are required")
	}
	if err := murlog.ValidatePassword(req.NewPassword); err != nil {
		return nil, newRPCErr(codeInvalidParams, err.Error())
	}

	// Verify current password. / 現在のパスワードを検証。
	storedHash, err := h.store.GetSetting(ctx, SettingPasswordHash)
	if err != nil || storedHash == "" {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.CurrentPassword)); err != nil {
		return nil, newRPCErr(codeUnauthorized, "current password is incorrect")
	}

	// Hash and store new password. / 新パスワードをハッシュして保存。
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	if err := h.store.SetSetting(ctx, SettingPasswordHash, string(newHash)); err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	return statusOK, nil
}

// rpcAuthLogout handles auth.logout.
// 現在のセッションを無効化する。
func (h *Handler) rpcAuthLogout(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	r := rpcRequest_(ctx)
	if r == nil {
		return nil, newRPCErr(codeUnauthorized, "unauthorized")
	}

	// Invalidate session cookie. / セッション Cookie を無効化。
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		hash := hashToken(cookie.Value)
		h.store.DeleteSession(ctx, hash)
	}

	// Also check Bearer token for backward compatibility.
	// Bearer トークンも確認 (後方互換)。
	if token := extractBearerToken(r); token != "" {
		hash := hashToken(token)
		if tok, err := h.store.GetAPIToken(ctx, hash); err == nil {
			h.store.DeleteAPIToken(ctx, tok.ID)
		}
	}

	// Clear cookie. / Cookie をクリア。
	if w := rpcWriter(ctx); w != nil {
		r := rpcRequest_(ctx)
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   h.isSecure(r),
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
		})
	}

	return statusOK, nil
}
