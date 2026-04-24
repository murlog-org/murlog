package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/config"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/queue/sqlqueue"
	"github.com/murlog-org/murlog/store"
	_ "github.com/murlog-org/murlog/store/sqlite"
	"github.com/murlog-org/murlog/worker"

	"golang.org/x/crypto/bcrypt"
)

type benchEnv struct {
	server    *httptest.Server
	store     store.Store
	cookie    string
	personaID id.ID
	postIDs   []id.ID
}

func setupBenchEnv(b *testing.B, dsn string) *benchEnv {
	b.Helper()
	s, err := store.Open("sqlite", dsn)
	if err != nil {
		b.Fatalf("store.Open: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		b.Fatalf("Migrate: %v", err)
	}

	ctx := context.Background()
	now := time.Now()

	persona := &murlog.Persona{
		ID:            id.New(),
		Username:      "bench",
		DisplayName:   "Benchmark User",
		PublicKeyPEM:  "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----",
		PrivateKeyPEM: "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----",
		Primary:       true,
		FieldsJSON:    "[]",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	s.CreatePersona(ctx, persona)

	// Set password (low cost for speed). / パスワード設定 (速度のため低コスト)。
	hash, _ := bcrypt.GenerateFromPassword([]byte("benchpass"), 4)
	s.SetSetting(ctx, SettingPasswordHash, string(hash))
	s.SetSetting(ctx, SettingSetupComplete, "true")
	s.SetSetting(ctx, SettingDomain, "localhost")
	s.SetSetting(ctx, SettingProtocol, "http")

	// Seed 5000 posts, 500 followers, 2000 favourites.
	// 5000 投稿、500 フォロワー、2000 いいね。
	postIDs := make([]id.ID, 5000)
	for i := 0; i < 5000; i++ {
		pid := id.New()
		postIDs[i] = pid
		s.CreatePostBulk(ctx, &murlog.Post{
			ID: pid, PersonaID: persona.ID,
			Content:    fmt.Sprintf("<p>Benchmark post %d</p>", i),
			Visibility: murlog.VisibilityPublic, Origin: "local",
			CreatedAt: now.Add(-time.Duration(5000-i) * time.Minute), UpdatedAt: now,
		})
	}

	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 500; i++ {
		s.CreateFollowerBulk(ctx, &murlog.Follower{
			ID: id.New(), PersonaID: persona.ID,
			ActorURI: fmt.Sprintf("https://remote%d.example/users/f%d", i%10, i),
			Approved: true, CreatedAt: now,
		})
	}
	for i := 0; i < 2000; i++ {
		s.CreateFavourite(ctx, &murlog.Favourite{
			ID: id.New(), PostID: postIDs[rng.Intn(5000)],
			ActorURI: fmt.Sprintf("https://r.example/u/fav%d", i), CreatedAt: now,
		})
	}

	// Queue jobs. / キュージョブ。
	q := sqlqueue.New(s.DB())
	for i := 0; i < 500; i++ {
		q.Enqueue(ctx, &murlog.QueueJob{
			ID: id.New(), Type: murlog.JobDeliverNote,
			Payload:   fmt.Sprintf(`{"persona_id":"%s","actor_uri":"https://r%d.example/u/t%d"}`, persona.ID, i%10, i),
			Status:    murlog.JobPending,
			NextRunAt: now, CreatedAt: now,
		})
	}

	// Refresh counters after bulk insert. / バルク挿入後にカウンターを一括更新。
	s.RefreshAllCounters(ctx)

	// Start server. / サーバー起動。
	tmpToml := filepath.Join(b.TempDir(), "murlog.ini")
	os.WriteFile(tmpToml, []byte("db_driver = sqlite\n"), 0644)
	cfg := &config.Config{Path: tmpToml, WebDir: ""}
	m := nopMedia{}
	w := worker.New(q, s, m, 0, 0, 0)
	h := New(cfg, s, q, w, m)
	srv := httptest.NewServer(h)
	b.Cleanup(func() { srv.Close(); s.Close() })

	// Login via HTTP. / HTTP 経由でログイン。
	cookie := benchLogin(b, srv.URL)

	return &benchEnv{
		server: srv, store: s, cookie: cookie,
		personaID: persona.ID, postIDs: postIDs,
	}
}

func benchLogin(b *testing.B, baseURL string) string {
	b.Helper()
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1,
		"method": "auth.login",
		"params": map[string]string{"password": "benchpass"},
	})
	resp, err := http.Post(baseURL+"/api/mur/v1/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		b.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	var respBody json.RawMessage
	json.NewDecoder(resp.Body).Decode(&respBody)
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			return c.Value
		}
	}
	b.Fatalf("login: no session cookie, status=%d, body=%s", resp.StatusCode, respBody)
	return ""
}

func benchRPC(b *testing.B, baseURL, cookie, method string, params any) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	req, _ := http.NewRequest("POST", baseURL+"/api/mur/v1/rpc", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.Fatalf("rpc %s: %v", method, err)
	}
	resp.Body.Close()
}

// --- Read (QPS) ---

func BenchmarkRPC_TimelineHome(b *testing.B) {
	env := setupBenchEnv(b, filepath.Join(b.TempDir(), "bench.db"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchRPC(b, env.server.URL, env.cookie, "timeline.home", map[string]int{"limit": 20})
	}
}

func BenchmarkRPC_PostsList_Public(b *testing.B) {
	env := setupBenchEnv(b, filepath.Join(b.TempDir(), "bench.db"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchRPC(b, env.server.URL, "", "posts.list", map[string]any{"persona_id": env.personaID.String(), "limit": 20})
	}
}

func BenchmarkRPC_PersonasGet(b *testing.B) {
	env := setupBenchEnv(b, filepath.Join(b.TempDir(), "bench.db"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchRPC(b, env.server.URL, env.cookie, "personas.get", map[string]string{"id": env.personaID.String()})
	}
}

// --- Write (QPS) ---

func BenchmarkRPC_PostsCreate(b *testing.B) {
	env := setupBenchEnv(b, filepath.Join(b.TempDir(), "bench.db"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchRPC(b, env.server.URL, env.cookie, "posts.create", map[string]string{"content": fmt.Sprintf("<p>Bench write %d</p>", i)})
	}
}

// --- Queue tick (QPS) ---

func BenchmarkRPC_QueueTick(b *testing.B) {
	env := setupBenchEnv(b, filepath.Join(b.TempDir(), "bench.db"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchRPC(b, env.server.URL, env.cookie, "queue.tick", nil)
	}
}
