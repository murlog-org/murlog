package sqlite

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/queue/sqlqueue"
	"github.com/murlog-org/murlog/store"
)

// seedBenchDB populates a store with large dataset for benchmarking.
// ベンチマーク用に大量データを投入する。
func seedBenchDB(b *testing.B, s store.Store, postCount, followerCount, favCount, reblogCount, jobCount int) (id.ID, []id.ID) {
	b.Helper()
	ctx := context.Background()

	// Create persona. / ペルソナを作成。
	persona := &murlog.Persona{
		ID:            id.New(),
		Username:      "bench",
		DisplayName:   "Benchmark User",
		PublicKeyPEM:  "-----BEGIN PUBLIC KEY-----\nMIIBIjANBg...\n-----END PUBLIC KEY-----",
		PrivateKeyPEM: "-----BEGIN PRIVATE KEY-----\nMIIEv...\n-----END PRIVATE KEY-----",
		Primary:       true,
		FieldsJSON:    "[]",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := s.CreatePersona(ctx, persona); err != nil {
		b.Fatalf("CreatePersona: %v", err)
	}

	// Create posts. / 投稿を作成。
	postIDs := make([]id.ID, postCount)
	now := time.Now()
	for i := 0; i < postCount; i++ {
		pid := id.New()
		postIDs[i] = pid
		post := &murlog.Post{
			ID:         pid,
			PersonaID:  persona.ID,
			Content:    fmt.Sprintf("<p>Benchmark post number %d with some content to simulate real data.</p>", i),
			Visibility: murlog.VisibilityPublic,
			Origin:     "local",
			CreatedAt:  now.Add(-time.Duration(postCount-i) * time.Minute),
			UpdatedAt:  now,
		}
		if err := s.CreatePostBulk(ctx, post); err != nil {
			b.Fatalf("CreatePost %d: %v", i, err)
		}
	}

	// Create followers. / フォロワーを作成。
	for i := 0; i < followerCount; i++ {
		f := &murlog.Follower{
			ID:        id.New(),
			PersonaID: persona.ID,
			ActorURI:  fmt.Sprintf("https://remote%d.example/users/follower%d", i%10, i),
			Approved:  true,
			CreatedAt: now,
		}
		s.CreateFollowerBulk(ctx, f)
	}

	// Create favourites (random distribution). / いいねを作成（ランダム分散）。
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < favCount; i++ {
		postIdx := rng.Intn(postCount)
		fav := &murlog.Favourite{
			ID:        id.New(),
			PostID:    postIDs[postIdx],
			ActorURI:  fmt.Sprintf("https://remote.example/users/faver%d", i),
			CreatedAt: now,
		}
		s.CreateFavourite(ctx, fav)
	}

	// Create reblogs (random distribution). / リブログを作成（ランダム分散）。
	for i := 0; i < reblogCount; i++ {
		postIdx := rng.Intn(postCount)
		reblog := &murlog.Reblog{
			ID:        id.New(),
			PostID:    postIDs[postIdx],
			ActorURI:  fmt.Sprintf("https://remote.example/users/reblogger%d", i),
			CreatedAt: now,
		}
		s.CreateReblog(ctx, reblog)
	}

	// Create queue jobs. / キュージョブを作成。
	q := sqlqueue.New(s.DB())
	for i := 0; i < jobCount; i++ {
		job := &murlog.QueueJob{
			ID:   id.New(),
			Type: murlog.JobDeliverNote,
			Payload: fmt.Sprintf(`{"persona_id":"%s","actor_uri":"https://remote%d.example/users/target%d"}`,
				persona.ID, i%10, i),
			Status:    murlog.JobPending,
			NextRunAt: now,
			CreatedAt: now,
		}
		q.Enqueue(ctx, job)
	}

	// Refresh counters once after bulk insert. / バルク挿入後にカウンターを一括更新。
	s.RefreshAllCounters(ctx)

	return persona.ID, postIDs
}

func openBenchStore(b *testing.B, dsn string) store.Store {
	b.Helper()
	s, err := store.Open("sqlite", dsn)
	if err != nil {
		b.Fatalf("store.Open: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		b.Fatalf("Migrate: %v", err)
	}
	b.Cleanup(func() { s.Close() })
	return s
}

func runBenchWithModes(b *testing.B, fn func(b *testing.B, s store.Store, personaID id.ID, postIDs []id.ID)) {
	b.Run("InMemory", func(b *testing.B) {
		s := openBenchStore(b, ":memory:")
		personaID, postIDs := seedBenchDB(b, s, 10000, 1000, 5000, 3000, 2000)
		b.ResetTimer()
		fn(b, s, personaID, postIDs)
	})
	b.Run("File", func(b *testing.B) {
		dsn := filepath.Join(b.TempDir(), "bench.db")
		s := openBenchStore(b, dsn)
		personaID, postIDs := seedBenchDB(b, s, 10000, 1000, 5000, 3000, 2000)
		b.ResetTimer()
		fn(b, s, personaID, postIDs)
	})
}

// --- Store benchmarks ---

func BenchmarkListPostsByPersona(b *testing.B) {
	runBenchWithModes(b, func(b *testing.B, s store.Store, personaID id.ID, _ []id.ID) {
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			s.ListPostsByPersona(ctx, personaID, id.ID{}, 20)
		}
	})
}

func BenchmarkListPostsByPersonaCursor(b *testing.B) {
	runBenchWithModes(b, func(b *testing.B, s store.Store, personaID id.ID, postIDs []id.ID) {
		ctx := context.Background()
		cursor := postIDs[len(postIDs)/2] // Mid-point cursor. / 中間カーソル。
		for i := 0; i < b.N; i++ {
			s.ListPostsByPersona(ctx, personaID, cursor, 20)
		}
	})
}

func BenchmarkPostInteractionCounts(b *testing.B) {
	runBenchWithModes(b, func(b *testing.B, s store.Store, _ id.ID, postIDs []id.ID) {
		ctx := context.Background()
		batch := postIDs[:20] // First 20 posts. / 最初の 20 件。
		for i := 0; i < b.N; i++ {
			s.PostInteractionCounts(ctx, batch)
		}
	})
}

func BenchmarkCountFollowers(b *testing.B) {
	runBenchWithModes(b, func(b *testing.B, s store.Store, personaID id.ID, _ []id.ID) {
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			s.CountFollowers(ctx, personaID)
		}
	})
}

func BenchmarkListPublicLocalPosts(b *testing.B) {
	runBenchWithModes(b, func(b *testing.B, s store.Store, personaID id.ID, _ []id.ID) {
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			s.ListPublicLocalPosts(ctx, personaID, id.ID{}, 20)
		}
	})
}

// --- Queue benchmarks ---

func BenchmarkQueueClaim(b *testing.B) {
	b.Run("InMemory", func(b *testing.B) {
		s := openBenchStore(b, ":memory:")
		seedBenchDB(b, s, 10, 0, 0, 0, 2000)
		q := sqlqueue.New(s.DB())
		b.ResetTimer()
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			job, _ := q.Claim(ctx)
			if job != nil {
				q.Complete(ctx, job.ID)
				// Re-enqueue for next iteration. / 次のイテレーション用に再投入。
				job.ID = id.New()
				job.Status = murlog.JobPending
				q.Enqueue(ctx, job)
			}
		}
	})
	b.Run("File", func(b *testing.B) {
		dsn := filepath.Join(b.TempDir(), "bench-queue.db")
		s := openBenchStore(b, dsn)
		seedBenchDB(b, s, 10, 0, 0, 0, 2000)
		q := sqlqueue.New(s.DB())
		b.ResetTimer()
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			job, _ := q.Claim(ctx)
			if job != nil {
				q.Complete(ctx, job.ID)
				job.ID = id.New()
				job.Status = murlog.JobPending
				q.Enqueue(ctx, job)
			}
		}
	})
}

func BenchmarkQueueEnqueueComplete(b *testing.B) {
	b.Run("InMemory", func(b *testing.B) {
		s := openBenchStore(b, ":memory:")
		s.Migrate(context.Background())
		q := sqlqueue.New(s.DB())
		b.ResetTimer()
		ctx := context.Background()
		now := time.Now()
		for i := 0; i < b.N; i++ {
			job := &murlog.QueueJob{
				ID:        id.New(),
				Type:      murlog.JobDeliverNote,
				Payload:   "{}",
				Status:    murlog.JobPending,
				NextRunAt: now,
				CreatedAt: now,
			}
			q.Enqueue(ctx, job)
			claimed, _ := q.Claim(ctx)
			if claimed != nil {
				q.Complete(ctx, claimed.ID)
			}
		}
	})
	b.Run("File", func(b *testing.B) {
		dsn := filepath.Join(b.TempDir(), "bench-cycle.db")
		s := openBenchStore(b, dsn)
		q := sqlqueue.New(s.DB())
		b.ResetTimer()
		ctx := context.Background()
		now := time.Now()
		for i := 0; i < b.N; i++ {
			job := &murlog.QueueJob{
				ID:        id.New(),
				Type:      murlog.JobDeliverNote,
				Payload:   "{}",
				Status:    murlog.JobPending,
				NextRunAt: now,
				CreatedAt: now,
			}
			q.Enqueue(ctx, job)
			claimed, _ := q.Claim(ctx)
			if claimed != nil {
				q.Complete(ctx, claimed.ID)
			}
		}
	})
}
