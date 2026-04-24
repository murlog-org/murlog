package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/murlog-org/murlog/id"
)

// jobJSON is the API representation of a QueueJob.
// QueueJob の API レスポンス表現。
type jobJSON struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Payload     string `json:"payload"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	LastError   string `json:"last_error,omitempty"`
	NextRunAt   string `json:"next_run_at"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at,omitempty"`
}

func jobStatusString(s int) string {
	switch s {
	case 0:
		return "pending"
	case 1:
		return "running"
	case 2:
		return "done"
	case 3:
		return "failed"
	case 4:
		return "dead"
	default:
		return "unknown"
	}
}

// rpcQueueStats handles queue.stats.
// キューの集計カウントを返す。
func (h *Handler) rpcQueueStats(ctx context.Context, _ json.RawMessage) (any, *rpcErr) {
	stats, err := h.queue.Stats(ctx)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}
	return stats, nil
}

// rpcQueueList handles queue.list.
// 最近のジョブ一覧を返す。
func (h *Handler) rpcQueueList(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	type listParams struct {
		Status string `json:"status,omitempty"` // "pending", "running", "done", "failed", "" (all)
		Cursor string `json:"cursor,omitempty"` // 前ページの最後の ID / last ID from previous page
		Limit  int    `json:"limit,omitempty"`
	}
	req, rErr := parseParams[listParams](params)
	if rErr != nil {
		return nil, rErr
	}
	limit := clampLimit(req.Limit, 50, 200)

	jobs, err := h.queue.List(ctx, req.Status, req.Cursor, limit)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "internal error")
	}

	out := make([]jobJSON, len(jobs))
	for i, j := range jobs {
		jj := jobJSON{
			ID:        j.ID.String(),
			Type:      j.Type.String(),
			Payload:   j.Payload,
			Status:    jobStatusString(int(j.Status)),
			Attempts:  j.Attempts,
			LastError: j.LastError,
			NextRunAt: j.NextRunAt.Format(time.RFC3339),
			CreatedAt: j.CreatedAt.Format(time.RFC3339),
		}
		if !j.CompletedAt.IsZero() {
			jj.CompletedAt = j.CompletedAt.Format(time.RFC3339)
		}
		out[i] = jj
	}
	return out, nil
}

// rpcQueueRetry handles queue.retry.
// 失敗ジョブを pending に戻す。
func (h *Handler) rpcQueueRetry(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[getParams](params)
	if rErr != nil {
		return nil, rErr
	}
	jobID, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}
	if err := h.queue.Retry(ctx, jobID); err != nil {
		return nil, newRPCErr(codeInternalError, "retry failed")
	}
	return statusOK, nil
}

// rpcQueueDismiss handles queue.dismiss.
// 失敗ジョブを完了扱いにする（諦め）。
func (h *Handler) rpcQueueDismiss(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[getParams](params)
	if rErr != nil {
		return nil, rErr
	}
	jobID, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}
	if err := h.queue.Complete(ctx, jobID); err != nil {
		return nil, newRPCErr(codeInternalError, "dismiss failed")
	}
	return statusOK, nil
}

// rpcQueueTick handles queue.tick.
// No-op: actual processing is handled by the worker loop (serve mode)
// or spawnWorker (CGI mode). This endpoint exists so the SPA queue page
// triggers a CGI request, which in turn spawns the worker.
// No-op: 実際の処理は worker ループ (serve) か spawnWorker (CGI) が担当。
// SPA キュー画面から CGI リクエストを発生させ、worker spawn のトリガーにする。
func (h *Handler) rpcQueueTick(ctx context.Context, _ json.RawMessage) (any, *rpcErr) {
	return statusOK, nil
}

// rpcQueueVacuum handles queue.vacuum.
// 古い完了ジョブを削除し、VACUUM で DB ファイルを縮小する。
// Delete old completed jobs and run VACUUM to shrink the DB file.
func (h *Handler) rpcQueueVacuum(ctx context.Context, _ json.RawMessage) (any, *rpcErr) {
	days := h.cfg.WorkerJobRetentionDays
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	deleted, err := h.queue.Cleanup(ctx, cutoff)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "cleanup failed")
	}
	if _, err := h.store.DB().ExecContext(ctx, "VACUUM"); err != nil {
		return nil, newRPCErr(codeInternalError, "vacuum failed")
	}
	return map[string]int64{"deleted": deleted}, nil
}
