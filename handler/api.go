// Package handler — JSON API helpers.
// JSON API 用のヘルパー関数群。
package handler

// apiError is the standard error response body (used by non-RPC endpoints).
// 標準エラーレスポンスボディ (非 RPC エンドポイント用)。
type apiError struct {
	Error string `json:"error"`
}
