package handler

import (
	"context"
	"encoding/json"

	"github.com/murlog-org/murlog/totp"
)

// rpcTOTPSetup handles totp.setup.
// TOTP 秘密鍵を生成し、otpauth URI を返す。まだ有効化はしない (verify で確定)。
func (h *Handler) rpcTOTPSetup(ctx context.Context, _ json.RawMessage) (any, *rpcErr) {
	secret, err := totp.GenerateSecret()
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	// Store provisionally — overwritten on each setup call until verified.
	// 仮保存 — verify で確定するまで setup 呼び出しごとに上書き。
	if err := h.store.SetSetting(ctx, SettingTOTPSecret+"_pending", secret); err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	// Build otpauth URI. / otpauth URI を組み立てる。
	domain, _ := h.store.GetSetting(ctx, SettingDomain)
	if domain == "" {
		domain = "murlog"
	}
	personas, _ := h.store.ListPersonas(ctx)
	account := "admin"
	if len(personas) > 0 {
		account = personas[0].Username + "@" + domain
	}
	uri := totp.URI(secret, domain, account)

	return map[string]string{
		"secret": secret,
		"uri":    uri,
	}, nil
}

// rpcTOTPVerify handles totp.verify.
// pending 秘密鍵に対して TOTP コードを検証し、成功したら有効化する。
func (h *Handler) rpcTOTPVerify(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	type verifyParams struct {
		Code string `json:"code"`
	}
	req, rErr := parseParams[verifyParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.Code == "" {
		return nil, newRPCErr(codeInvalidParams, "code is required")
	}

	pendingSecret, err := h.store.GetSetting(ctx, SettingTOTPSecret+"_pending")
	if err != nil || pendingSecret == "" {
		return nil, newRPCErr(codeInvalidParams, "no pending TOTP setup")
	}

	if !totp.Validate(pendingSecret, req.Code) {
		return nil, newRPCErr(codeInvalidParams, "invalid code")
	}

	// Activate: move pending → active. / 有効化: pending → active に移動。
	if err := h.store.SetSetting(ctx, SettingTOTPSecret, pendingSecret); err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	h.store.SetSetting(ctx, SettingTOTPSecret+"_pending", "")

	return statusOK, nil
}

// rpcTOTPDisable handles totp.disable.
// TOTP を無効化する。
func (h *Handler) rpcTOTPDisable(ctx context.Context, _ json.RawMessage) (any, *rpcErr) {
	h.store.SetSetting(ctx, SettingTOTPSecret, "")
	h.store.SetSetting(ctx, SettingTOTPSecret+"_pending", "")

	return statusOK, nil
}

// rpcTOTPStatus handles totp.status.
// TOTP が有効かどうかを返す。
func (h *Handler) rpcTOTPStatus(ctx context.Context, _ json.RawMessage) (any, *rpcErr) {
	secret, _ := h.store.GetSetting(ctx, SettingTOTPSecret)
	return map[string]bool{"enabled": secret != ""}, nil
}
