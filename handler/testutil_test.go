package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/activitypub"
	"github.com/murlog-org/murlog/config"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/queue"
	"github.com/murlog-org/murlog/queue/sqlqueue"
	"github.com/murlog-org/murlog/store"
	_ "github.com/murlog-org/murlog/store/sqlite"
	"github.com/murlog-org/murlog/worker"

	"golang.org/x/crypto/bcrypt"
)

// nopMedia is a no-op media store for testing.
// テスト用の no-op メディアストア。
type nopMedia struct{}

func (nopMedia) Save(string, io.Reader) error       { return nil }
func (nopMedia) Open(string) (io.ReadCloser, error)  { return nil, nil }
func (nopMedia) Delete(string) error                 { return nil }
func (nopMedia) URL(string) string                   { return "" }

// testEnv holds a test environment with a running murlog server.
// テスト環境を保持する構造体。
type testEnv struct {
	handler *Handler
	server  *httptest.Server
	store   store.Store
	queue   queue.Queue
	persona *murlog.Persona
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	s, err := store.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	s.SetSetting(ctx, SettingSetupComplete, "true")
	s.SetSetting(ctx, SettingDomain, "murlog.test")

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
		ShowFollows:   true,
		Discoverable:  true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.CreatePersona(ctx, persona); err != nil {
		t.Fatalf("CreatePersona: %v", err)
	}

	tmpToml := t.TempDir() + "/murlog.ini"
	if err := os.WriteFile(tmpToml, []byte("db_driver = sqlite\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Disable SSRF protection for tests (httptest uses 127.0.0.1).
	// テスト用に SSRF 保護を無効化 (httptest は 127.0.0.1 を使用)。
	origClient := activitypub.HTTPClient
	activitypub.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(func() { activitypub.HTTPClient = origClient })

	cfg := &config.Config{Path: tmpToml, WebDir: ""}
	q := sqlqueue.New(s.DB())
	m := nopMedia{}
	w := worker.New(q, s, m, 0, 0, 0)
	h := New(cfg, s, q, w, m)
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close() })

	return &testEnv{handler: h, server: srv, store: s, queue: q, persona: persona}
}

// setupRawTestEnv creates a test environment in pre-setup state (no toml file, no setup_complete).
// セットアップ前状態のテスト環境を作成する (toml なし、setup_complete なし)。
func setupRawTestEnv(t *testing.T) *testEnv {
	t.Helper()

	s, err := store.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// toml path points to a non-existent file in a temp dir.
	// toml パスは一時ディレクトリ内の存在しないファイルを指す。
	tmpDir := t.TempDir()
	tmpToml := tmpDir + "/murlog.ini"
	// Do NOT create the file — this simulates pre-setup state.
	// ファイルは作成しない — セットアップ前状態をシミュレート。

	cfg := &config.Config{Path: tmpToml, DBDriver: "sqlite", DataDir: tmpDir, WebDir: ""}
	q := sqlqueue.New(s.DB())
	m := nopMedia{}
	w := worker.New(q, s, m, 0, 0, 0)
	h := New(cfg, s, q, w, m)
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close() })

	return &testEnv{handler: h, server: srv, store: s, queue: q}
}

// ─────────────────────────────────────────────
// JSON-RPC test helpers
// JSON-RPC テストヘルパー
// ─────────────────────────────────────────────

// testRPCResponse is the raw JSON-RPC response structure for test assertions.
// テストアサーション用の生 JSON-RPC レスポンス構造体。
type testRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcErr         `json:"error"`
}

// rpcCallRaw sends a JSON-RPC request and returns the raw response (including errors).
// JSON-RPC リクエストを送信し、生レスポンス (エラー含む) を返す。
func rpcCallRaw(t *testing.T, env *testEnv, method string, params any, cookie string) testRPCResponse {
	t.Helper()

	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}
	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", env.server.URL+"/api/mur/v1/rpc", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rpcCallRaw %s: %v", method, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var rpcResp testRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("rpcCallRaw %s: decode response: %v\nbody: %s", method, err, respBody)
	}
	return rpcResp
}

// rpcCallRawWithHTTP sends a JSON-RPC request and returns both the raw response and HTTP response.
// JSON-RPC リクエストを送信し、生レスポンスと HTTP レスポンスの両方を返す。
func rpcCallRawWithHTTP(t *testing.T, env *testEnv, method string, params any, cookie string) (testRPCResponse, *http.Response) {
	t.Helper()

	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}
	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", env.server.URL+"/api/mur/v1/rpc", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rpcCallRawWithHTTP %s: %v", method, err)
	}

	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var rpcResp testRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("rpcCallRawWithHTTP %s: decode response: %v\nbody: %s", method, err, respBody)
	}
	return rpcResp, resp
}

// rpcCall sends a JSON-RPC request and decodes the result. Fails on RPC errors.
// JSON-RPC リクエストを送信し、結果をデコードする。RPC エラー時は失敗。
func rpcCall(t *testing.T, env *testEnv, method string, params any, result any) {
	t.Helper()
	rpcCallWithCookie(t, env, method, params, result, "")
}

// rpcCallWithCookie sends a JSON-RPC request with an optional session cookie.
// セッション Cookie 付きで JSON-RPC リクエストを送信する。
func rpcCallWithCookie(t *testing.T, env *testEnv, method string, params any, result any, cookie string) {
	t.Helper()

	rpcResp := rpcCallRaw(t, env, method, params, cookie)
	if rpcResp.Error != nil {
		t.Fatalf("rpcCall %s: error %d: %s", method, rpcResp.Error.Code, rpcResp.Error.Message)
	}
	if result != nil {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			t.Fatalf("rpcCall %s: decode result: %v", method, err)
		}
	}
}

// loginTestEnv creates a session and returns the cookie value.
// セッションを作成し、Cookie 値を返す。
func loginTestEnv(t *testing.T, env *testEnv, password string) string {
	t.Helper()
	ctx := context.Background()
	// Set bcrypt password hash in DB.
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	env.store.SetSetting(ctx, SettingPasswordHash, string(hash))

	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"method":  "auth.login",
		"params":  map[string]string{"password": password},
		"id":      1,
	}
	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(env.server.URL+"/api/mur/v1/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()

	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			return c.Value
		}
	}
	t.Fatal("login: no session cookie returned")
	return ""
}

// loginRPC sends an auth.login RPC and returns the cookie from the response.
// auth.login RPC を送信し、レスポンスから Cookie を返す。
func loginRPC(t *testing.T, env *testEnv, params map[string]string) (testRPCResponse, string) {
	t.Helper()

	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"method":  "auth.login",
		"params":  params,
		"id":      1,
	}
	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(env.server.URL+"/api/mur/v1/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("loginRPC: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp testRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("loginRPC: decode response: %v\nbody: %s", err, respBody)
	}

	var cookie string
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			cookie = c.Value
		}
	}
	return rpcResp, cookie
}
