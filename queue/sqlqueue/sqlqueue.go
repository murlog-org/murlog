// Package sqlqueue implements queue.Queue using database/sql (SQLite-compatible).
// database/sql (SQLite 互換) による queue.Queue の実装。
package sqlqueue

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/internal/sqlutil"
	"github.com/murlog-org/murlog/queue"
)


// now returns the current time. Replaceable in tests.
// 現在時刻を返す。テスト時に差し替え可能。
var now = time.Now

// sqlQueue implements queue.Queue backed by a SQL database.
type sqlQueue struct {
	db *sql.DB
}

// New creates a new Queue backed by the given *sql.DB.
// The queue_jobs table must already exist (created by store migrations).
// 指定 *sql.DB をバックエンドとする Queue を生成する。
// queue_jobs テーブルは Store のマイグレーションで作成済みであること。
func New(db *sql.DB) queue.Queue {
	return &sqlQueue{db: db}
}

func (q *sqlQueue) HasPending(ctx context.Context) bool {
	var exists int
	err := q.db.QueryRowContext(ctx, `
		SELECT 1 FROM queue_jobs
		WHERE status IN (?, ?) AND next_run_at <= ?
		LIMIT 1`,
		int(murlog.JobPending), int(murlog.JobFailed),
		formatTime(now())).Scan(&exists)
	return err == nil
}

func (q *sqlQueue) Enqueue(ctx context.Context, job *murlog.QueueJob) error {
	_, err := q.db.ExecContext(ctx, `
		INSERT INTO queue_jobs (id, type, payload, status, attempts, next_run_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		job.ID.Bytes(), int(job.Type), job.Payload, int(job.Status), job.Attempts,
		formatTime(job.NextRunAt), formatTime(job.CreatedAt))
	if err != nil {
		return fmt.Errorf("enqueue: %w", err)
	}
	return nil
}

func (q *sqlQueue) EnqueueBatch(ctx context.Context, jobs []*murlog.QueueJob) error {
	if len(jobs) == 0 {
		return nil
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("enqueue batch begin: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO queue_jobs (id, type, payload, status, attempts, next_run_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("enqueue batch prepare: %w", err)
	}
	defer stmt.Close()

	for _, job := range jobs {
		if _, err := stmt.ExecContext(ctx,
			job.ID.Bytes(), int(job.Type), job.Payload, int(job.Status), job.Attempts,
			formatTime(job.NextRunAt), formatTime(job.CreatedAt)); err != nil {
			return fmt.Errorf("enqueue batch insert: %w", err)
		}
	}
	return tx.Commit()
}

func (q *sqlQueue) Claim(ctx context.Context) (*murlog.QueueJob, error) {
	// Atomically pick the next ready job and set status to running.
	// UPDATE ... RETURNING で1ステートメントの排他制御。
	// M11: Claim 時に next_run_at を現在時刻に更新し、RecoverStale の stale 判定を正確にする。
	// M11: Update next_run_at to now on Claim so RecoverStale can accurately detect stale jobs.
	claimTime := formatTime(now())
	row := q.db.QueryRowContext(ctx, `
		UPDATE queue_jobs
		SET status = ?, attempts = attempts + 1, next_run_at = ?
		WHERE id = (
			SELECT id FROM queue_jobs
			WHERE status IN (?, ?) AND next_run_at <= ?
			ORDER BY next_run_at
			LIMIT 1
		)
		RETURNING id, type, payload, status, attempts, last_error, next_run_at, created_at`,
		int(murlog.JobRunning), claimTime,
		int(murlog.JobPending), int(murlog.JobFailed),
		claimTime)

	var j murlog.QueueJob
	var rawID []byte
	var status int
	var nextRunAt, createdAt string
	var err error
	if err = row.Scan(&rawID, &j.Type, &j.Payload, &status, &j.Attempts, &j.LastError, &nextRunAt, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	j.ID, err = scanID(rawID)
	if err != nil {
		return nil, err
	}
	j.Status = murlog.JobStatus(status)
	j.NextRunAt, err = parseTime(nextRunAt)
	if err != nil {
		return nil, err
	}
	j.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (q *sqlQueue) Complete(ctx context.Context, jobID id.ID) error {
	// Clear payload and last_error on completion to save space.
	// 完了時に payload と last_error をクリアしてストレージを節約。
	_, err := q.db.ExecContext(ctx, `
		UPDATE queue_jobs SET status = ?, completed_at = ?, payload = '', last_error = '' WHERE id = ?`,
		int(murlog.JobDone), formatTime(now()), jobID.Bytes())
	if err != nil {
		return fmt.Errorf("complete: %w", err)
	}
	return nil
}

func (q *sqlQueue) Dead(ctx context.Context, jobID id.ID, lastError string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE queue_jobs SET status = ?, last_error = ?, completed_at = ? WHERE id = ?`,
		int(murlog.JobDead), lastError, formatTime(now()), jobID.Bytes())
	if err != nil {
		return fmt.Errorf("dead: %w", err)
	}
	return nil
}

func (q *sqlQueue) Fail(ctx context.Context, jobID id.ID, nextRun time.Time, lastError string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE queue_jobs SET status = ?, next_run_at = ?, last_error = ? WHERE id = ?`,
		int(murlog.JobFailed), formatTime(nextRun), lastError, jobID.Bytes())
	if err != nil {
		return fmt.Errorf("fail: %w", err)
	}
	return nil
}

func (q *sqlQueue) Retry(ctx context.Context, jobID id.ID) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE queue_jobs SET status = ?, next_run_at = ?, last_error = '' WHERE id = ?`,
		int(murlog.JobPending), formatTime(now()), jobID.Bytes())
	if err != nil {
		return fmt.Errorf("retry: %w", err)
	}
	return nil
}

func (q *sqlQueue) Stats(ctx context.Context) (*queue.QueueStats, error) {
	var s queue.QueueStats
	rows, err := q.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM queue_jobs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var status, count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		switch murlog.JobStatus(status) {
		case murlog.JobPending:
			s.Pending = count
		case murlog.JobRunning:
			s.Running = count
		case murlog.JobDone:
			s.Done = count
		case murlog.JobFailed:
			s.Failed = count
		case murlog.JobDead:
			s.Dead = count
		}
	}
	return &s, rows.Err()
}

func (q *sqlQueue) List(ctx context.Context, status string, cursor string, limit int) ([]*murlog.QueueJob, error) {
	// ステータスフィルタとカーソルページングに対応。
	// Support status filter and cursor-based pagination.
	query := `SELECT id, type, payload, status, attempts, last_error, next_run_at, created_at, completed_at FROM queue_jobs`
	var args []interface{}
	var conditions []string

	if status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, jobStatusInt(status))
	}
	if cursor != "" {
		conditions = append(conditions, "id < ?")
		cid, err := parseID(cursor)
		if err == nil {
			args = append(args, cid)
		}
	}
	if len(conditions) > 0 {
		query += " WHERE " + conditions[0]
		for _, c := range conditions[1:] {
			query += " AND " + c
		}
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*murlog.QueueJob
	for rows.Next() {
		var j murlog.QueueJob
		var rawID []byte
		var status int
		var nextRunAt, createdAt, completedAt string
		var err error
		if err = rows.Scan(&rawID, &j.Type, &j.Payload, &status, &j.Attempts, &j.LastError, &nextRunAt, &createdAt, &completedAt); err != nil {
			return nil, err
		}
		j.ID, err = scanID(rawID)
		if err != nil {
			return nil, err
		}
		j.Status = murlog.JobStatus(status)
		j.NextRunAt, err = parseTime(nextRunAt)
		if err != nil {
			return nil, err
		}
		j.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		if completedAt != "" {
			j.CompletedAt, _ = parseTime(completedAt)
		}
		jobs = append(jobs, &j)
	}
	return jobs, rows.Err()
}

// RecoverStale resets running jobs that have been stuck longer than the given duration.
// CGI timeout or crash can leave jobs in running state permanently.
// Jobs that have already reached MaxAttempts are marked dead instead of failed.
// 指定時間以上 running のままのジョブを回復する。
// MaxAttempts に達しているジョブは failed ではなく dead にする。
func (q *sqlQueue) RecoverStale(ctx context.Context, staleAfter time.Duration) (int64, error) {
	cutoff := now().Add(-staleAfter)
	nowStr := formatTime(now())

	// Mark stale jobs that have reached max attempts as dead.
	// MaxAttempts に達した stale ジョブを dead にする。
	q.db.ExecContext(ctx, `
		UPDATE queue_jobs SET status = ?, last_error = 'recovered: stale + max attempts', completed_at = ?
		WHERE status = ? AND next_run_at < ? AND attempts >= ?`,
		int(murlog.JobDead), nowStr,
		int(murlog.JobRunning), formatTime(cutoff), murlog.MaxJobAttempts)

	// Recover remaining stale jobs as failed for retry.
	// 残りの stale ジョブを failed にリセットしてリトライ。
	res, err := q.db.ExecContext(ctx, `
		UPDATE queue_jobs SET status = ?, last_error = 'recovered: stale running job', next_run_at = ?
		WHERE status = ? AND next_run_at < ?`,
		int(murlog.JobFailed), nowStr,
		int(murlog.JobRunning), formatTime(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Cleanup deletes completed jobs older than the given time.
// 指定時刻より古い完了済みジョブを削除する。
func (q *sqlQueue) Cleanup(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := q.db.ExecContext(ctx, `
		DELETE FROM queue_jobs WHERE status IN (?, ?) AND created_at < ?`,
		int(murlog.JobDone), int(murlog.JobDead), formatTime(olderThan))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CancelByDomain deletes pending/failed jobs whose payload contains the given domain.
// payload に指定ドメインを含む pending/failed ジョブを削除する。
func (q *sqlQueue) CancelByDomain(ctx context.Context, domain string) (int64, error) {
	// L3: ポート付き URL にも対応。
	// L3: Also match URLs with port numbers.
	pattern := "%://" + domain + "/%"
	patternPort := "%://" + domain + ":%"
	res, err := q.db.ExecContext(ctx, `
		DELETE FROM queue_jobs WHERE status IN (?, ?) AND (payload LIKE ? OR payload LIKE ?)`,
		int(murlog.JobPending), int(murlog.JobFailed), pattern, patternPort)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- helpers ---

// formatTime delegates to sqlutil.FormatTime.
// sqlutil.FormatTime に委譲する。
var formatTime = sqlutil.FormatTime

// parseTime delegates to sqlutil.ParseTime.
// sqlutil.ParseTime に委譲する。
var parseTime = sqlutil.ParseTime

// scanID delegates to sqlutil.ScanID.
// sqlutil.ScanID に委譲する。
var scanID = sqlutil.ScanID

func jobStatusInt(s string) int {
	switch s {
	case "pending":
		return 0
	case "running":
		return 1
	case "done":
		return 2
	case "failed":
		return 3
	case "dead":
		return 4
	default:
		return -1
	}
}

func parseID(s string) ([]byte, error) {
	v, err := id.Parse(s)
	if err != nil {
		return nil, err
	}
	return v.Bytes(), nil
}

