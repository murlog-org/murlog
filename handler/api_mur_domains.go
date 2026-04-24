package handler

import (
	"context"
	"encoding/json"
	"time"
)

type domainFailureJSON struct {
	Domain       string `json:"domain"`
	FailureCount int    `json:"failure_count"`
	LastError    string `json:"last_error"`
	Dead         bool   `json:"dead"`
	FirstAt      string `json:"first_failure_at"`
	LastAt       string `json:"last_failure_at"`
}

// rpcDomainsListFailures returns all domain failure records with dead status.
// 全ドメイン失敗レコードを dead 判定付きで返す。
func (h *Handler) rpcDomainsListFailures(ctx context.Context, _ json.RawMessage) (any, *rpcErr) {
	failures, err := h.store.ListDomainFailures(ctx)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	out := make([]domainFailureJSON, len(failures))
	for i, f := range failures {
		dead := f.FailureCount >= 10 && time.Since(f.LastFailureAt) < time.Hour
		out[i] = domainFailureJSON{
			Domain:       f.Domain,
			FailureCount: f.FailureCount,
			LastError:    f.LastError,
			Dead:         dead,
			FirstAt:      f.FirstFailureAt.Format(time.RFC3339),
			LastAt:       f.LastFailureAt.Format(time.RFC3339),
		}
	}
	return out, nil
}

type domainResetParams struct {
	Domain string `json:"domain"`
}

// rpcDomainsResetFailure resets the failure count for a domain.
// 指定ドメインの失敗カウントをリセットする。
func (h *Handler) rpcDomainsResetFailure(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[domainResetParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.Domain == "" {
		return nil, newRPCErr(codeInvalidParams, "domain is required")
	}

	if err := h.store.ResetDomainFailure(ctx, req.Domain); err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	return statusOK, nil
}
