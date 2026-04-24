package sqlqueue

import (
	"context"
	"testing"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/store"
	_ "github.com/murlog-org/murlog/store/sqlite"
)

func newTestQueue(t *testing.T) *sqlQueue {
	t.Helper()
	s, err := store.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return New(s.DB()).(*sqlQueue)
}

func TestEnqueueAndClaim(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	baseTime := time.Now().Truncate(time.Second).UTC()

	// Override now so the job is claimable.
	now = func() time.Time { return baseTime.Add(time.Second) }
	t.Cleanup(func() { now = time.Now })

	job := &murlog.QueueJob{
		ID:        id.New(),
		Type:      murlog.JobAcceptFollow,
		Payload:   `{"persona_id":"abc"}`,
		Status:    murlog.JobPending,
		NextRunAt: baseTime,
		CreatedAt: baseTime,
	}
	if err := q.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Claim should return the job with status=running, attempts=1.
	claimed, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("Claim returned nil, want job")
	}
	if claimed.Type != murlog.JobAcceptFollow {
		t.Errorf("Type = %q, want %q", claimed.Type, murlog.JobAcceptFollow)
	}
	if claimed.Status != murlog.JobRunning {
		t.Errorf("Status = %d, want %d (JobRunning)", claimed.Status, murlog.JobRunning)
	}
	if claimed.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", claimed.Attempts)
	}

	// No more jobs to claim.
	next, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("Claim (empty): %v", err)
	}
	if next != nil {
		t.Errorf("Claim returned %+v, want nil", next)
	}
}

func TestComplete(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	baseTime := time.Now().Truncate(time.Second).UTC()

	now = func() time.Time { return baseTime.Add(time.Second) }
	t.Cleanup(func() { now = time.Now })

	job := &murlog.QueueJob{
		ID:        id.New(),
		Type:      murlog.JobDeliverPost,
		Payload:   `{}`,
		Status:    murlog.JobPending,
		NextRunAt: baseTime,
		CreatedAt: baseTime,
	}
	q.Enqueue(ctx, job)

	claimed, _ := q.Claim(ctx)
	if err := q.Complete(ctx, claimed.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// Completed jobs should not be claimable.
	next, _ := q.Claim(ctx)
	if next != nil {
		t.Error("completed job should not be claimable")
	}
}

func TestFailAndRetry(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	baseTime := time.Now().Truncate(time.Second).UTC()

	now = func() time.Time { return baseTime.Add(time.Second) }
	t.Cleanup(func() { now = time.Now })

	job := &murlog.QueueJob{
		ID:        id.New(),
		Type:      murlog.JobAcceptFollow,
		Payload:   `{}`,
		Status:    murlog.JobPending,
		NextRunAt: baseTime,
		CreatedAt: baseTime,
	}
	q.Enqueue(ctx, job)

	claimed, _ := q.Claim(ctx)

	// Fail with retry in the future.
	retryAt := baseTime.Add(10 * time.Minute)
	if err := q.Fail(ctx, claimed.ID, retryAt, "test error"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	// Not claimable yet (now < retryAt).
	next, _ := q.Claim(ctx)
	if next != nil {
		t.Error("failed job should not be claimable before retryAt")
	}

	// Advance time past retryAt.
	now = func() time.Time { return retryAt.Add(time.Second) }

	// Now it should be claimable again.
	retried, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("Claim (retry): %v", err)
	}
	if retried == nil {
		t.Fatal("failed job should be claimable after retryAt")
	}
	if retried.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", retried.Attempts)
	}
}

func TestClaimOrder(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	baseTime := time.Now().Truncate(time.Second).UTC()

	now = func() time.Time { return baseTime.Add(10 * time.Second) }
	t.Cleanup(func() { now = time.Now })

	// Enqueue two jobs: second one has earlier next_run_at.
	job1 := &murlog.QueueJob{
		ID:        id.New(),
		Type:      murlog.JobDeliverPost,
		Payload:   `{}`,
		Status:    murlog.JobPending,
		NextRunAt: baseTime.Add(5 * time.Second),
		CreatedAt: baseTime,
	}
	job2 := &murlog.QueueJob{
		ID:        id.New(),
		Type:      murlog.JobDeliverNote,
		Payload:   `{}`,
		Status:    murlog.JobPending,
		NextRunAt: baseTime,
		CreatedAt: baseTime,
	}
	q.Enqueue(ctx, job1)
	q.Enqueue(ctx, job2)

	// First claim should get the earlier job.
	first, _ := q.Claim(ctx)
	if first == nil || first.Type != murlog.JobDeliverNote {
		t.Errorf("first claim type = %v, want job_early", first)
	}

	second, _ := q.Claim(ctx)
	if second == nil || second.Type != murlog.JobDeliverPost {
		t.Errorf("second claim type = %v, want job_late", second)
	}
}

func TestHasPending(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	baseTime := time.Now().Truncate(time.Second).UTC()

	now = func() time.Time { return baseTime.Add(time.Second) }
	t.Cleanup(func() { now = time.Now })

	// Empty queue → no pending.
	if q.HasPending(ctx) {
		t.Error("HasPending should be false on empty queue")
	}

	// Add a job.
	q.Enqueue(ctx, &murlog.QueueJob{
		ID: id.New(), Type: murlog.JobDeliverNote, Payload: `{}`,
		Status: murlog.JobPending, NextRunAt: baseTime, CreatedAt: baseTime,
	})

	if !q.HasPending(ctx) {
		t.Error("HasPending should be true with pending job")
	}

	// Claim and complete it.
	claimed, _ := q.Claim(ctx)
	q.Complete(ctx, claimed.ID)

	if q.HasPending(ctx) {
		t.Error("HasPending should be false after completing the only job")
	}
}

func TestCleanup(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	baseTime := time.Now().Truncate(time.Second).UTC()

	now = func() time.Time { return baseTime.Add(time.Second) }
	t.Cleanup(func() { now = time.Now })

	// Create and complete a job.
	job := &murlog.QueueJob{
		ID: id.New(), Type: murlog.JobDeliverNote, Payload: `{}`,
		Status: murlog.JobPending, NextRunAt: baseTime, CreatedAt: baseTime,
	}
	q.Enqueue(ctx, job)
	claimed, _ := q.Claim(ctx)
	q.Complete(ctx, claimed.ID)

	// Cleanup with cutoff in the past → nothing deleted.
	n, err := q.Cleanup(ctx, baseTime.Add(-time.Hour))
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if n != 0 {
		t.Errorf("Cleanup: deleted %d, want 0", n)
	}

	// Cleanup with cutoff in the future → job deleted.
	n, err = q.Cleanup(ctx, baseTime.Add(time.Hour))
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if n != 1 {
		t.Errorf("Cleanup: deleted %d, want 1", n)
	}
}

func TestEnqueueBatch(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	baseTime := time.Now().Truncate(time.Second).UTC()

	now = func() time.Time { return baseTime.Add(time.Second) }
	t.Cleanup(func() { now = time.Now })

	// Empty batch — should succeed without error.
	// 空バッチ — エラーなしで成功すべき。
	if err := q.EnqueueBatch(ctx, nil); err != nil {
		t.Fatalf("EnqueueBatch(nil): %v", err)
	}

	// Batch of 5 jobs.
	// 5件のバッチ。
	var jobs []*murlog.QueueJob
	for i := 0; i < 5; i++ {
		jobs = append(jobs, &murlog.QueueJob{
			ID: id.New(), Type: murlog.JobDeliverNote, Payload: `{}`,
			Status: murlog.JobPending, NextRunAt: baseTime, CreatedAt: baseTime,
		})
	}
	if err := q.EnqueueBatch(ctx, jobs); err != nil {
		t.Fatalf("EnqueueBatch: %v", err)
	}

	// All 5 should be claimable.
	// 5件全部 claim できるべき。
	for i := 0; i < 5; i++ {
		claimed, err := q.Claim(ctx)
		if err != nil {
			t.Fatalf("Claim #%d: %v", i, err)
		}
		if claimed == nil {
			t.Fatalf("Claim #%d: got nil, want job", i)
		}
		if claimed.Type != murlog.JobDeliverNote {
			t.Errorf("Claim #%d type = %q, want deliver_note", i, claimed.Type)
		}
	}

	// Queue should be empty now.
	// キューは空になっているべき。
	extra, _ := q.Claim(ctx)
	if extra != nil {
		t.Errorf("unexpected extra job: %+v", extra)
	}
}

func TestParseTimeAndScanID(t *testing.T) {
	// parseTime: valid RFC 3339 / 正常な RFC 3339
	ts, err := parseTime("2025-04-19T10:30:00Z")
	if err != nil {
		t.Fatalf("parseTime(valid): %v", err)
	}
	if ts.IsZero() {
		t.Fatal("parseTime(valid): got zero time")
	}

	// parseTime: invalid → error with prefix / 不正 → プレフィックス付きエラー
	_, err = parseTime("not-a-date")
	if err == nil {
		t.Fatal("parseTime(invalid): expected error")
	}
	if got := err.Error(); len(got) < 10 || got[:7] != "sqlutil" {
		t.Fatalf("parseTime error prefix: got %q", got)
	}

	// scanID: nil → zero ID, nil error / nil → ゼロ ID, エラーなし
	zeroID, err := scanID(nil)
	if err != nil {
		t.Fatalf("scanID(nil): %v", err)
	}
	if zeroID != id.Nil {
		t.Fatalf("scanID(nil): want Nil, got %s", zeroID)
	}

	// scanID: valid bytes / 正常バイト
	validID := id.New()
	got, err := scanID(validID[:])
	if err != nil {
		t.Fatalf("scanID(valid): %v", err)
	}
	if got != validID {
		t.Fatalf("scanID(valid): want %s, got %s", validID, got)
	}

	// scanID: invalid bytes → error with prefix / 不正バイト → プレフィックス付きエラー
	_, err = scanID([]byte{0xDE, 0xAD})
	if err == nil {
		t.Fatal("scanID(invalid): expected error")
	}
	if got := err.Error(); len(got) < 10 || got[:7] != "sqlutil" {
		t.Fatalf("scanID error prefix: got %q", got)
	}
}

func TestDead(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	baseTime := time.Now().Truncate(time.Second).UTC()

	now = func() time.Time { return baseTime.Add(time.Second) }
	t.Cleanup(func() { now = time.Now })

	job := &murlog.QueueJob{
		ID: id.New(), Type: murlog.JobDeliverNote, Payload: `{}`,
		Status: murlog.JobPending, NextRunAt: baseTime, CreatedAt: baseTime,
	}
	q.Enqueue(ctx, job)
	claimed, _ := q.Claim(ctx)

	if err := q.Dead(ctx, claimed.ID, "max attempts reached"); err != nil {
		t.Fatalf("Dead: %v", err)
	}

	// Dead job は claim できないこと。
	// Dead job should not be claimable.
	next, _ := q.Claim(ctx)
	if next != nil {
		t.Error("dead job should not be claimable")
	}

	// Stats に dead=1 が含まれること。
	// Stats should include dead=1.
	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Dead != 1 {
		t.Errorf("stats.Dead = %d, want 1", stats.Dead)
	}
}

func TestCleanupDeletesDeadJobs(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	baseTime := time.Now().Truncate(time.Second).UTC()

	now = func() time.Time { return baseTime.Add(time.Second) }
	t.Cleanup(func() { now = time.Now })

	job := &murlog.QueueJob{
		ID: id.New(), Type: murlog.JobDeliverNote, Payload: `{}`,
		Status: murlog.JobPending, NextRunAt: baseTime, CreatedAt: baseTime,
	}
	q.Enqueue(ctx, job)
	claimed, _ := q.Claim(ctx)
	q.Dead(ctx, claimed.ID, "dead")

	// Cleanup with cutoff in the future should delete dead jobs.
	// 未来の cutoff で dead ジョブが削除されること。
	n, err := q.Cleanup(ctx, baseTime.Add(time.Hour))
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if n != 1 {
		t.Errorf("Cleanup: deleted %d, want 1", n)
	}
}

func TestCancelByDomainWithPort(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	baseTime := time.Now().Truncate(time.Second).UTC()

	now = func() time.Time { return baseTime.Add(time.Second) }
	t.Cleanup(func() { now = time.Now })

	// ポートなし URL のジョブ。
	// Job with URL without port.
	q.Enqueue(ctx, &murlog.QueueJob{
		ID: id.New(), Type: murlog.JobDeliverNote, Payload: `{"actor_uri":"https://example.com/users/bob"}`,
		Status: murlog.JobPending, NextRunAt: baseTime, CreatedAt: baseTime,
	})
	// ポート付き URL のジョブ。
	// Job with URL with port.
	q.Enqueue(ctx, &murlog.QueueJob{
		ID: id.New(), Type: murlog.JobDeliverNote, Payload: `{"actor_uri":"https://example.com:3000/users/bob"}`,
		Status: murlog.JobPending, NextRunAt: baseTime, CreatedAt: baseTime,
	})
	// 別ドメインのジョブ (削除されないこと)。
	// Job with different domain (should NOT be deleted).
	q.Enqueue(ctx, &murlog.QueueJob{
		ID: id.New(), Type: murlog.JobDeliverNote, Payload: `{"actor_uri":"https://other.com/users/alice"}`,
		Status: murlog.JobPending, NextRunAt: baseTime, CreatedAt: baseTime,
	})

	n, err := q.CancelByDomain(ctx, "example.com")
	if err != nil {
		t.Fatalf("CancelByDomain: %v", err)
	}
	if n != 2 {
		t.Errorf("CancelByDomain: deleted %d, want 2 (with and without port)", n)
	}

	// other.com のジョブが残っていること。
	// other.com job should remain.
	stats, _ := q.Stats(ctx)
	if stats.Pending != 1 {
		t.Errorf("remaining pending = %d, want 1", stats.Pending)
	}
}
