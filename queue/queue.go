// Package queue defines the background job queue interface for murlog.
// murlog のバックグラウンドジョブキューインターフェースを定義するパッケージ。
package queue

import (
	"context"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

// QueueStats holds aggregate counts for each job status.
// 各ジョブステータスの集計カウント。
type QueueStats struct {
	Pending int `json:"pending"`
	Running int `json:"running"`
	Done    int `json:"done"`
	Failed  int `json:"failed"`
	Dead    int `json:"dead"`
}

// Queue defines the interface for background job queue operations.
// バックグラウンドジョブキュー操作のインターフェース。
type Queue interface {
	// HasPending returns true if there are jobs ready to run.
	// 実行可能なジョブがあれば true を返す。
	HasPending(ctx context.Context) bool

	// Enqueue adds a new job to the queue.
	// 新しいジョブをキューに追加する。
	Enqueue(ctx context.Context, job *murlog.QueueJob) error

	// EnqueueBatch adds multiple jobs in a single transaction.
	// 1トランザクションで複数ジョブを一括追加する。
	EnqueueBatch(ctx context.Context, jobs []*murlog.QueueJob) error

	// Claim atomically picks and locks the next ready job (status → running).
	// Returns nil, nil when no job is available.
	// 次の実行可能ジョブを atomic に取得しロックする（status → running）。
	// 利用可能なジョブがなければ nil, nil を返す。
	Claim(ctx context.Context) (*murlog.QueueJob, error)

	// Complete marks a job as done.
	// ジョブを完了済みにする。
	Complete(ctx context.Context, jobID id.ID) error

	// Dead marks a job as permanently failed (max retries exhausted).
	// ジョブをリトライ上限到達 (永久失敗) にする。
	Dead(ctx context.Context, jobID id.ID, lastError string) error

	// Fail marks a job as failed with an error message and schedules a retry at nextRun.
	// ジョブを失敗にし、エラーメッセージを記録、nextRun でリトライをスケジュールする。
	Fail(ctx context.Context, jobID id.ID, nextRun time.Time, lastError string) error

	// Retry resets a failed job to pending for immediate reprocessing.
	// 失敗ジョブを pending に戻して即座に再処理可能にする。
	Retry(ctx context.Context, jobID id.ID) error

	// Stats returns aggregate counts for each job status.
	// 各ジョブステータスの集計カウントを返す。
	Stats(ctx context.Context) (*QueueStats, error)

	// List returns recent jobs ordered by created_at DESC.
	// status が空文字なら全件、指定すればフィルタ。cursor は前ページの最後の ID。
	// 最近のジョブを created_at DESC で返す。
	List(ctx context.Context, status string, cursor string, limit int) ([]*murlog.QueueJob, error)

	// Cleanup deletes completed jobs older than the given time.
	// 指定時刻より古い完了済みジョブを削除する。
	Cleanup(ctx context.Context, olderThan time.Time) (int64, error)

	// RecoverStale resets running jobs stuck longer than staleAfter to failed.
	// staleAfter 以上 running のままのジョブを failed にリセットする。
	RecoverStale(ctx context.Context, staleAfter time.Duration) (int64, error)

	// CancelByDomain deletes pending/failed jobs targeting the given domain.
	// 指定ドメイン宛ての pending/failed ジョブを削除する。
	CancelByDomain(ctx context.Context, domain string) (int64, error)
}
