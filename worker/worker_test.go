package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/activitypub"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/queue"
	"github.com/murlog-org/murlog/queue/sqlqueue"
	"github.com/murlog-org/murlog/store"
	_ "github.com/murlog-org/murlog/store/sqlite"
)

type testEnv struct {
	store  store.Store
	queue  queue.Queue
	worker *Worker
}

func setup(t *testing.T) *testEnv {
	t.Helper()
	s, err := store.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// Disable SSRF protection for tests (httptest uses 127.0.0.1).
	// テスト用に SSRF 保護を無効化 (httptest は 127.0.0.1 を使用)。
	origClient := activitypub.HTTPClient
	activitypub.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(func() { activitypub.HTTPClient = origClient })

	q := sqlqueue.New(s.DB())
	w := New(q, s, nopMedia{}, 0, 0, 0)
	return &testEnv{store: s, queue: q, worker: w}
}

// nopMedia is a no-op media store for testing.
// テスト用の no-op メディアストア。
type nopMedia struct{}

func (nopMedia) Save(string, io.Reader) error       { return nil }
func (nopMedia) Open(string) (io.ReadCloser, error)  { return nil, nil }
func (nopMedia) Delete(string) error                 { return nil }
func (nopMedia) URL(name string) string              { return "/media/" + name }

func TestAcceptFollow(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	// Set up domain.
	env.store.SetSetting(ctx, "domain", "murlog.test")

	// Create persona with keypair.
	pubPEM, privPEM, err := activitypub.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	now := time.Now()
	persona := &murlog.Persona{
		ID:            id.New(),
		Username:      "alice",
		DisplayName:   "Alice",
		PublicKeyPEM:  pubPEM,
		PrivateKeyPEM: privPEM,
		Primary:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	env.store.CreatePersona(ctx, persona)

	// Mock remote server: serves Actor JSON and accepts POST to inbox.
	var deliverCount atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/users/bob":
			actor := activitypub.Actor{
				Context:          "https://www.w3.org/ns/activitystreams",
				ID:               "REPLACE_ACTOR_URI",
				Type:             "Person",
				PreferredUsername: "bob",
				Inbox:            "REPLACE_INBOX_URI",
				Outbox:           "REPLACE_OUTBOX_URI",
			}
			// Replace placeholder URLs with actual server URL.
			w.Header().Set("Content-Type", "application/activity+json")
			json.NewEncoder(w).Encode(actor)
		case r.Method == "POST" && r.URL.Path == "/users/bob/inbox":
			deliverCount.Add(1)
			w.WriteHeader(http.StatusAccepted)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(func() { remote.Close() })

	// Pre-cache the remote actor so FetchActor isn't called (it would hit the wrong URLs).
	env.store.UpsertRemoteActor(ctx, &murlog.RemoteActor{
		URI:      remote.URL + "/users/bob",
		Username: "bob",
		Inbox:    remote.URL + "/users/bob/inbox",
		FetchedAt: time.Now(),
	})

	// Enqueue accept_follow job.
	job := &murlog.QueueJob{
		ID:   id.New(),
		Type: murlog.JobAcceptFollow,
		Payload: murlog.MustJSON(map[string]string{
			"persona_id":  persona.ID.String(),
			"activity_id": remote.URL + "/users/bob#follows/123",
			"actor_uri":   remote.URL + "/users/bob",
		}),
		Status:    murlog.JobPending,
		NextRunAt: now,
		CreatedAt: now,
	}
	env.queue.Enqueue(ctx, job)

	// Process.
	claimed, _ := env.queue.Claim(ctx)
	if claimed == nil {
		t.Fatal("no job to claim")
	}
	env.worker.process(ctx, claimed)

	// Verify delivery happened.
	if got := deliverCount.Load(); got != 1 {
		t.Errorf("deliver count = %d, want 1", got)
	}
}

func TestDeliverPostFanout(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	env.store.SetSetting(ctx, "domain", "murlog.test")

	pubPEM, privPEM, err := activitypub.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	now := time.Now()
	persona := &murlog.Persona{
		ID:            id.New(),
		Username:      "alice",
		DisplayName:   "Alice",
		PublicKeyPEM:  pubPEM,
		PrivateKeyPEM: privPEM,
		Primary:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	env.store.CreatePersona(ctx, persona)

	post := &murlog.Post{
		ID:         id.New(),
		PersonaID:  persona.ID,
		Content:    "<p>Hello world</p>",
		Visibility: murlog.VisibilityPublic,
		Origin:     "local",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	env.store.CreatePost(ctx, post)

	// Two mock remote followers.
	var deliverCount atomic.Int32
	remote1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			deliverCount.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(func() { remote1.Close() })

	remote2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			deliverCount.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(func() { remote2.Close() })

	// Create followers and cache their actors.
	for _, srv := range []*httptest.Server{remote1, remote2} {
		uri := srv.URL + "/users/follower"
		env.store.CreateFollower(ctx, &murlog.Follower{
			ID:        id.New(),
			PersonaID: persona.ID,
			ActorURI:  uri,
			Approved:  true,
			CreatedAt: now,
		})
		env.store.UpsertRemoteActor(ctx, &murlog.RemoteActor{
			URI:       uri,
			Username:  "follower",
			Inbox:     srv.URL + "/inbox",
			FetchedAt: time.Now(),
		})
	}

	// Enqueue deliver_post job.
	job := &murlog.QueueJob{
		ID:        id.New(),
		Type:      murlog.JobDeliverPost,
		Payload:   murlog.MustJSON(map[string]string{"post_id": post.ID.String()}),
		Status:    murlog.JobPending,
		NextRunAt: now,
		CreatedAt: now,
	}
	env.queue.Enqueue(ctx, job)

	// Step 1: deliver_post should fan out into deliver_note jobs.
	claimed, _ := env.queue.Claim(ctx)
	if claimed == nil {
		t.Fatal("no job to claim")
	}
	if claimed.Type != murlog.JobDeliverPost {
		t.Fatalf("expected deliver_post, got %s", claimed.Type)
	}
	env.worker.process(ctx, claimed)

	// No deliveries yet — only fan-out happened.
	if got := deliverCount.Load(); got != 0 {
		t.Errorf("deliver count after fan-out = %d, want 0", got)
	}

	// Step 2: process the two deliver_note jobs.
	for i := 0; i < 2; i++ {
		noteJob, _ := env.queue.Claim(ctx)
		if noteJob == nil {
			t.Fatalf("deliver_note job #%d not found", i+1)
		}
		if noteJob.Type != murlog.JobDeliverNote {
			t.Fatalf("expected deliver_note, got %s", noteJob.Type)
		}
		env.worker.process(ctx, noteJob)
	}

	if got := deliverCount.Load(); got != 2 {
		t.Errorf("deliver count = %d, want 2", got)
	}

	// Queue should be empty now.
	extra, _ := env.queue.Claim(ctx)
	if extra != nil {
		t.Errorf("unexpected extra job: %+v", extra)
	}
}

func TestUpdatePostFanout(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	env.store.SetSetting(ctx, "domain", "murlog.test")

	pubPEM, privPEM, err := activitypub.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	now := time.Now()
	persona := &murlog.Persona{
		ID:            id.New(),
		Username:      "alice",
		DisplayName:   "Alice",
		PublicKeyPEM:  pubPEM,
		PrivateKeyPEM: privPEM,
		Primary:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	env.store.CreatePersona(ctx, persona)

	post := &murlog.Post{
		ID:         id.New(),
		PersonaID:  persona.ID,
		Content:    "<p>Hello world</p>",
		Visibility: murlog.VisibilityPublic,
		Origin:     "local",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	env.store.CreatePost(ctx, post)

	// Two mock remote followers.
	var deliverCount atomic.Int32
	var lastActivityType atomic.Value
	remote1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			var act map[string]interface{}
			json.Unmarshal(body, &act)
			if typ, ok := act["type"].(string); ok {
				lastActivityType.Store(typ)
			}
			deliverCount.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(func() { remote1.Close() })

	remote2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			var act map[string]interface{}
			json.Unmarshal(body, &act)
			if typ, ok := act["type"].(string); ok {
				lastActivityType.Store(typ)
			}
			deliverCount.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(func() { remote2.Close() })

	// Create followers and cache their actors.
	for _, srv := range []*httptest.Server{remote1, remote2} {
		uri := srv.URL + "/users/follower"
		env.store.CreateFollower(ctx, &murlog.Follower{
			ID:        id.New(),
			PersonaID: persona.ID,
			ActorURI:  uri,
			Approved:  true,
			CreatedAt: now,
		})
		env.store.UpsertRemoteActor(ctx, &murlog.RemoteActor{
			URI:       uri,
			Username:  "follower",
			Inbox:     srv.URL + "/inbox",
			FetchedAt: time.Now(),
		})
	}

	// Enqueue update_post job.
	job := &murlog.QueueJob{
		ID:        id.New(),
		Type:      murlog.JobUpdatePost,
		Payload:   murlog.MustJSON(map[string]string{"post_id": post.ID.String()}),
		Status:    murlog.JobPending,
		NextRunAt: now,
		CreatedAt: now,
	}
	env.queue.Enqueue(ctx, job)

	// Step 1: update_post should fan out into deliver_update_note jobs.
	claimed, _ := env.queue.Claim(ctx)
	if claimed == nil {
		t.Fatal("no job to claim")
	}
	if claimed.Type != murlog.JobUpdatePost {
		t.Fatalf("expected update_post, got %s", claimed.Type)
	}
	env.worker.process(ctx, claimed)

	// No deliveries yet — only fan-out happened.
	if got := deliverCount.Load(); got != 0 {
		t.Errorf("deliver count after fan-out = %d, want 0", got)
	}

	// Step 2: process the two deliver_update_note jobs.
	for i := 0; i < 2; i++ {
		noteJob, _ := env.queue.Claim(ctx)
		if noteJob == nil {
			t.Fatalf("deliver_update_note job #%d not found", i+1)
		}
		if noteJob.Type != murlog.JobDeliverUpdateNote {
			t.Fatalf("expected deliver_update_note, got %s", noteJob.Type)
		}
		env.worker.process(ctx, noteJob)
	}

	if got := deliverCount.Load(); got != 2 {
		t.Errorf("deliver count = %d, want 2", got)
	}

	// Verify that the delivered activity type is "Update".
	// 配送された Activity の type が "Update" であることを確認。
	if typ, ok := lastActivityType.Load().(string); !ok || typ != "Update" {
		t.Errorf("activity type = %v, want Update", lastActivityType.Load())
	}

	// Queue should be empty now.
	extra, _ := env.queue.Claim(ctx)
	if extra != nil {
		t.Errorf("unexpected extra job: %+v", extra)
	}
}

func TestRetryOnFailure(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	env.store.SetSetting(ctx, "domain", "murlog.test")

	pubPEM, privPEM, _ := activitypub.GenerateKeyPair()
	now := time.Now()
	persona := &murlog.Persona{
		ID:            id.New(),
		Username:      "alice",
		PublicKeyPEM:  pubPEM,
		PrivateKeyPEM: privPEM,
		Primary:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	env.store.CreatePersona(ctx, persona)

	// Remote server that always fails.
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(func() { remote.Close() })

	env.store.UpsertRemoteActor(ctx, &murlog.RemoteActor{
		URI:       remote.URL + "/users/bob",
		Username:  "bob",
		Inbox:     remote.URL + "/inbox",
		FetchedAt: time.Now(),
	})

	job := &murlog.QueueJob{
		ID:   id.New(),
		Type: murlog.JobAcceptFollow,
		Payload: murlog.MustJSON(map[string]string{
			"persona_id":  persona.ID.String(),
			"activity_id": "https://remote.example/follows/1",
			"actor_uri":   remote.URL + "/users/bob",
		}),
		Status:    murlog.JobPending,
		NextRunAt: now,
		CreatedAt: now,
	}
	env.queue.Enqueue(ctx, job)

	// First attempt: should fail and reschedule.
	claimed, _ := env.queue.Claim(ctx)
	env.worker.process(ctx, claimed)

	// Job should be in failed state (not claimable right now due to future next_run_at).
	// Advance sqlqueue's now to pick up the retried job.
	next, _ := env.queue.Claim(ctx)
	if next != nil {
		t.Error("failed job should not be immediately claimable")
	}
}

func TestMaxAttemptsMarksDead(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	env.store.SetSetting(ctx, "domain", "murlog.test")

	pubPEM, privPEM, _ := activitypub.GenerateKeyPair()
	now := time.Now()
	persona := &murlog.Persona{
		ID:            id.New(),
		Username:      "alice",
		PublicKeyPEM:  pubPEM,
		PrivateKeyPEM: privPEM,
		Primary:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	env.store.CreatePersona(ctx, persona)

	// Remote server that always fails.
	// 常に失敗するリモートサーバー。
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(func() { remote.Close() })

	env.store.UpsertRemoteActor(ctx, &murlog.RemoteActor{
		URI:       remote.URL + "/users/bob",
		Username:  "bob",
		Inbox:     remote.URL + "/inbox",
		FetchedAt: time.Now(),
	})

	job := &murlog.QueueJob{
		ID:   id.New(),
		Type: murlog.JobAcceptFollow,
		Payload: murlog.MustJSON(map[string]string{
			"persona_id":  persona.ID.String(),
			"activity_id": "https://remote.example/follows/1",
			"actor_uri":   remote.URL + "/users/bob",
		}),
		Status:    murlog.JobPending,
		Attempts:  MaxAttempts, // Already at max attempts.
		NextRunAt: now,
		CreatedAt: now,
	}
	env.queue.Enqueue(ctx, job)

	claimed, _ := env.queue.Claim(ctx)
	if claimed == nil {
		t.Fatal("expected job to be claimed")
	}
	env.worker.process(ctx, claimed)

	// Job should be dead (not done). / ジョブは dead であること (done ではない)。
	stats, _ := env.queue.Stats(ctx)
	if stats.Dead != 1 {
		t.Errorf("expected 1 dead job, got %d (done=%d, failed=%d)", stats.Dead, stats.Done, stats.Failed)
	}
	if stats.Done != 0 {
		t.Errorf("expected 0 done jobs, got %d", stats.Done)
	}
}

func TestRunOnce(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	// Empty queue — should return false.
	if env.worker.RunOnce(ctx) {
		t.Error("RunOnce on empty queue should return false")
	}

	// Enqueue a job that will succeed (unknown type gets completed).
	env.store.SetSetting(ctx, "domain", "murlog.test")
	job := &murlog.QueueJob{
		ID:        id.New(),
		Type:      murlog.JobType(999),
		Payload:   `{}`,
		Status:    murlog.JobPending,
		NextRunAt: time.Now(),
		CreatedAt: time.Now(),
	}
	env.queue.Enqueue(ctx, job)

	// Should process one job and return true.
	if !env.worker.RunOnce(ctx) {
		t.Error("RunOnce should return true when a job was processed")
	}

	// Queue is now empty again.
	if env.worker.RunOnce(ctx) {
		t.Error("RunOnce should return false after queue is drained")
	}
}

func TestRunBatchLimit(t *testing.T) {
	env := setup(t)
	ctx := context.Background()
	now := time.Now()

	// Enqueue 5 jobs.
	for i := 0; i < 5; i++ {
		env.queue.Enqueue(ctx, &murlog.QueueJob{
			ID:        id.New(),
			Type:      murlog.JobType(999),
			Payload:   `{}`,
			Status:    murlog.JobPending,
			NextRunAt: now,
			CreatedAt: now,
		})
	}

	// Limit to 3 — should process exactly 3.
	n := env.worker.RunBatch(ctx, 3, 30*time.Second)
	if n != 3 {
		t.Errorf("RunBatch processed %d, want 3", n)
	}

	// Remaining 2.
	n = env.worker.RunBatch(ctx, 10, 30*time.Second)
	if n != 2 {
		t.Errorf("RunBatch processed %d, want 2", n)
	}
}

func TestRunBatchTimeout(t *testing.T) {
	env := setup(t)
	ctx := context.Background()
	now := time.Now()

	// Enqueue many jobs.
	for i := 0; i < 100; i++ {
		env.queue.Enqueue(ctx, &murlog.QueueJob{
			ID:        id.New(),
			Type:      murlog.JobType(999),
			Payload:   `{}`,
			Status:    murlog.JobPending,
			NextRunAt: now,
			CreatedAt: now,
		})
	}

	// Very short timeout — should stop before processing all.
	n := env.worker.RunBatch(ctx, 100, 1*time.Millisecond)
	if n >= 100 {
		t.Errorf("RunBatch should have been cut short by timeout, processed %d", n)
	}
}

func TestRunBatchConcurrent(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	env.store.SetSetting(ctx, "domain", "murlog.test")

	pubPEM, privPEM, _ := activitypub.GenerateKeyPair()
	now := time.Now()
	persona := &murlog.Persona{
		ID: id.New(), Username: "alice",
		PublicKeyPEM: pubPEM, PrivateKeyPEM: privPEM,
		Primary: true, CreatedAt: now, UpdatedAt: now,
	}
	env.store.CreatePersona(ctx, persona)

	// Mock remote server with 100ms latency per request.
	// 1リクエストあたり 100ms の遅延を持つモックリモートサーバー。
	var deliverCount atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			time.Sleep(100 * time.Millisecond)
			deliverCount.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(func() { remote.Close() })

	// Enqueue 8 accept_follow jobs.
	// 8件の accept_follow ジョブをエンキュー。
	for i := 0; i < 8; i++ {
		actorURI := fmt.Sprintf("%s/users/u%d", remote.URL, i)
		env.store.UpsertRemoteActor(ctx, &murlog.RemoteActor{
			URI:       actorURI,
			Username:  fmt.Sprintf("u%d", i),
			Inbox:     remote.URL + "/inbox",
			FetchedAt: now,
		})
		env.queue.Enqueue(ctx, &murlog.QueueJob{
			ID:   id.New(),
			Type: murlog.JobAcceptFollow,
			Payload: murlog.MustJSON(map[string]string{
				"persona_id":  persona.ID.String(),
				"activity_id": fmt.Sprintf("%s/follows/%d", actorURI, i),
				"actor_uri":   actorURI,
			}),
			Status:    murlog.JobPending,
			NextRunAt: now,
			CreatedAt: now,
		})
	}

	// Sequential would take 8 * 100ms = 800ms minimum.
	// With concurrency=4, expect ~200ms (2 batches of 4).
	// 逐次なら 8 * 100ms = 最低 800ms。並列度4なら約 200ms（4件×2バッチ）。
	start := time.Now()
	n := env.worker.RunBatch(ctx, 8, 10*time.Second)
	elapsed := time.Since(start)

	if n != 8 {
		t.Errorf("RunBatch processed %d, want 8", n)
	}
	if got := deliverCount.Load(); got != 8 {
		t.Errorf("deliver count = %d, want 8", got)
	}

	// Should be significantly faster than sequential (800ms).
	// Allow generous margin but reject clearly sequential execution.
	// 逐次 (800ms) より明らかに速いことを検証。余裕を持った閾値で判定。
	if elapsed > 600*time.Millisecond {
		t.Errorf("RunBatch took %v, expected < 600ms (sequential would be ~800ms)", elapsed)
	}
	t.Logf("RunBatch processed 8 jobs in %v (concurrency working)", elapsed)
}

func TestDecideConcurrency(t *testing.T) {
	env := setup(t)
	ctx := context.Background()
	now := time.Now()

	// Empty queue — should return min concurrency.
	// 空キュー — min concurrency を返すべき。
	c := env.worker.decideConcurrency(ctx)
	if c != DefaultMinConcurrency {
		t.Errorf("empty queue concurrency = %d, want %d", c, DefaultMinConcurrency)
	}

	// 100 pending — should scale up to mid.
	// 100件 pending — mid にスケールアップすべき。
	for i := 0; i < 100; i++ {
		env.queue.Enqueue(ctx, &murlog.QueueJob{
			ID: id.New(), Type: murlog.JobType(999), Payload: `{}`,
			Status: murlog.JobPending, NextRunAt: now, CreatedAt: now,
		})
	}
	mid := (DefaultMinConcurrency + DefaultMaxConcurrency) / 2
	c = env.worker.decideConcurrency(ctx)
	if c != mid {
		t.Errorf("100 pending concurrency = %d, want %d", c, mid)
	}

	// 500+ pending — should scale to max concurrency.
	// 500件以上 pending — max concurrency にスケールすべき。
	for i := 0; i < 400; i++ {
		env.queue.Enqueue(ctx, &murlog.QueueJob{
			ID: id.New(), Type: murlog.JobType(999), Payload: `{}`,
			Status: murlog.JobPending, NextRunAt: now, CreatedAt: now,
		})
	}
	c = env.worker.decideConcurrency(ctx)
	if c != DefaultMaxConcurrency {
		t.Errorf("500 pending concurrency = %d, want %d", c, DefaultMaxConcurrency)
	}
}

func TestDeliverDeleteFanout(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	env.store.SetSetting(ctx, "domain", "murlog.test")

	pubPEM, privPEM, _ := activitypub.GenerateKeyPair()
	now := time.Now()
	persona := &murlog.Persona{
		ID: id.New(), Username: "alice",
		PublicKeyPEM: pubPEM, PrivateKeyPEM: privPEM,
		Primary: true, CreatedAt: now, UpdatedAt: now,
	}
	env.store.CreatePersona(ctx, persona)

	// Create 2 followers with cached actors.
	var deliverCount atomic.Int32
	for i := 0; i < 2; i++ {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				deliverCount.Add(1)
				w.WriteHeader(http.StatusAccepted)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(func() { srv.Close() })

		uri := srv.URL + "/users/follower"
		env.store.CreateFollower(ctx, &murlog.Follower{
			ID: id.New(), PersonaID: persona.ID, ActorURI: uri, Approved: true, CreatedAt: now,
		})
		env.store.UpsertRemoteActor(ctx, &murlog.RemoteActor{
			URI: uri, Username: "follower", Inbox: srv.URL + "/inbox", FetchedAt: now,
		})
	}

	// Enqueue deliver_delete fan-out job.
	env.queue.Enqueue(ctx, &murlog.QueueJob{
		ID: id.New(), Type: murlog.JobDeliverDelete,
		Payload: murlog.MustJSON(map[string]string{
			"persona_id": persona.ID.String(),
			"post_id":    id.New().String(),
		}),
		Status: murlog.JobPending, NextRunAt: now, CreatedAt: now,
	})

	// Step 1: fan-out.
	claimed, _ := env.queue.Claim(ctx)
	if claimed == nil || claimed.Type != murlog.JobDeliverDelete {
		t.Fatalf("expected deliver_delete, got %v", claimed)
	}
	env.worker.process(ctx, claimed)

	if got := deliverCount.Load(); got != 0 {
		t.Errorf("deliver count after fan-out = %d, want 0", got)
	}

	// Step 2: process deliver_delete_note jobs.
	for i := 0; i < 2; i++ {
		job, _ := env.queue.Claim(ctx)
		if job == nil {
			t.Fatalf("deliver_delete_note #%d not found", i+1)
		}
		if job.Type != murlog.JobDeliverDeleteNote {
			t.Fatalf("expected deliver_delete_note, got %s", job.Type)
		}
		env.worker.process(ctx, job)
	}

	if got := deliverCount.Load(); got != 2 {
		t.Errorf("deliver count = %d, want 2", got)
	}
}

func TestSendUndoFollow(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	env.store.SetSetting(ctx, "domain", "murlog.test")

	pubPEM, privPEM, _ := activitypub.GenerateKeyPair()
	now := time.Now()
	persona := &murlog.Persona{
		ID: id.New(), Username: "alice",
		PublicKeyPEM: pubPEM, PrivateKeyPEM: privPEM,
		Primary: true, CreatedAt: now, UpdatedAt: now,
	}
	env.store.CreatePersona(ctx, persona)

	// Mock remote server.
	var deliverCount atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			deliverCount.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(func() { remote.Close() })

	env.store.UpsertRemoteActor(ctx, &murlog.RemoteActor{
		URI: remote.URL + "/users/bob", Username: "bob",
		Inbox: remote.URL + "/inbox", FetchedAt: now,
	})

	// Enqueue send_undo_follow.
	followID := id.New()
	env.queue.Enqueue(ctx, &murlog.QueueJob{
		ID: id.New(), Type: murlog.JobSendUndoFollow,
		Payload: murlog.MustJSON(map[string]string{
			"persona_id": persona.ID.String(),
			"follow_id":  followID.String(),
			"target_uri": remote.URL + "/users/bob",
		}),
		Status: murlog.JobPending, NextRunAt: now, CreatedAt: now,
	})

	claimed, _ := env.queue.Claim(ctx)
	env.worker.process(ctx, claimed)

	if got := deliverCount.Load(); got != 1 {
		t.Errorf("deliver count = %d, want 1", got)
	}
}

func TestDeliverAnnounce(t *testing.T) {
	env := setup(t)
	ctx := context.Background()

	env.store.SetSetting(ctx, "domain", "murlog.test")

	pubPEM, privPEM, err := activitypub.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	now := time.Now()
	persona := &murlog.Persona{
		ID:            id.New(),
		Username:      "alice",
		DisplayName:   "Alice",
		PublicKeyPEM:  pubPEM,
		PrivateKeyPEM: privPEM,
		Primary:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	env.store.CreatePersona(ctx, persona)

	// Mock remote server: capture delivered activity.
	// モックリモートサーバー: 配送されたアクティビティをキャプチャ。
	var deliverCount atomic.Int32
	var lastBody []byte
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			deliverCount.Add(1)
			lastBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(func() { remote.Close() })

	env.store.UpsertRemoteActor(ctx, &murlog.RemoteActor{
		URI: remote.URL + "/users/bob", Username: "bob",
		Inbox: remote.URL + "/inbox", FetchedAt: now,
	})

	postURI := "https://remote.example/notes/42"

	// Announce activity / Announce アクティビティの配送
	t.Run("Announce", func(t *testing.T) {
		deliverCount.Store(0)
		env.queue.Enqueue(ctx, &murlog.QueueJob{
			ID: id.New(), Type: murlog.JobDeliverAnnounce,
			Payload: murlog.MustJSON(map[string]string{
				"persona_id": persona.ID.String(),
				"post_uri":   postURI,
				"actor_uri":  remote.URL + "/users/bob",
				"activity":   "Announce",
			}),
			Status: murlog.JobPending, NextRunAt: now, CreatedAt: now,
		})

		claimed, _ := env.queue.Claim(ctx)
		if claimed == nil {
			t.Fatal("no job to claim")
		}
		env.worker.process(ctx, claimed)

		if got := deliverCount.Load(); got != 1 {
			t.Fatalf("deliver count = %d, want 1", got)
		}

		var activity map[string]interface{}
		if err := json.Unmarshal(lastBody, &activity); err != nil {
			t.Fatalf("unmarshal activity: %v", err)
		}
		if activity["type"] != "Announce" {
			t.Errorf("type = %v, want Announce", activity["type"])
		}
		if activity["object"] != postURI {
			t.Errorf("object = %v, want %s", activity["object"], postURI)
		}
	})

	// Undo Announce activity / Undo Announce アクティビティの配送
	t.Run("Undo", func(t *testing.T) {
		deliverCount.Store(0)
		env.queue.Enqueue(ctx, &murlog.QueueJob{
			ID: id.New(), Type: murlog.JobDeliverAnnounce,
			Payload: murlog.MustJSON(map[string]string{
				"persona_id": persona.ID.String(),
				"post_uri":   postURI,
				"actor_uri":  remote.URL + "/users/bob",
				"activity":   "Undo",
			}),
			Status: murlog.JobPending, NextRunAt: now, CreatedAt: now,
		})

		claimed, _ := env.queue.Claim(ctx)
		if claimed == nil {
			t.Fatal("no job to claim")
		}
		env.worker.process(ctx, claimed)

		if got := deliverCount.Load(); got != 1 {
			t.Fatalf("deliver count = %d, want 1", got)
		}

		var activity map[string]interface{}
		if err := json.Unmarshal(lastBody, &activity); err != nil {
			t.Fatalf("unmarshal activity: %v", err)
		}
		if activity["type"] != "Undo" {
			t.Errorf("type = %v, want Undo", activity["type"])
		}
		obj, ok := activity["object"].(map[string]interface{})
		if !ok {
			t.Fatalf("object is not a map: %T", activity["object"])
		}
		if obj["type"] != "Announce" {
			t.Errorf("object.type = %v, want Announce", obj["type"])
		}
		if obj["object"] != postURI {
			t.Errorf("object.object = %v, want %s", obj["object"], postURI)
		}
	})
}
