package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/activitypub"
	"github.com/murlog-org/murlog/id"
)

// remoteActor is a simulated remote ActivityPub server for testing.
type remoteActor struct {
	server   *httptest.Server
	username string
	pubPEM   string
	privPEM  string
}

// setupRemoteActor creates a test server that serves an Actor JSON-LD.
func setupRemoteActor(t *testing.T, username string) *remoteActor {
	t.Helper()

	pubPEM, privPEM, err := activitypub.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	ra := &remoteActor{username: username, pubPEM: pubPEM, privPEM: privPEM}

	ra.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		actor := activitypub.Actor{
			Context:          "https://www.w3.org/ns/activitystreams",
			ID:               base + "/users/" + username,
			Type:             "Person",
			PreferredUsername: username,
			Inbox:            base + "/users/" + username + "/inbox",
			PublicKey: activitypub.PublicKey{
				ID:           base + "/users/" + username + "#main-key",
				Owner:        base + "/users/" + username,
				PublicKeyPEM: pubPEM,
			},
		}
		w.Header().Set("Content-Type", "application/activity+json")
		json.NewEncoder(w).Encode(actor)
	}))
	t.Cleanup(func() { ra.server.Close() })

	return ra
}

// actorURI returns the actor URI for this remote actor.
func (ra *remoteActor) actorURI() string {
	return ra.server.URL + "/users/" + ra.username
}

// keyID returns the key ID for signing.
func (ra *remoteActor) keyID() string {
	return ra.actorURI() + "#main-key"
}

// signedPost sends a signed POST to the given URL with the given body.
func (ra *remoteActor) signedPost(t *testing.T, url string, body []byte) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/activity+json")
	if err := activitypub.SignRequest(req, ra.keyID(), ra.privPEM, body); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// ─────────────────────────────────────────────
// Actor endpoint
// ─────────────────────────────────────────────

func TestActorEndpoint(t *testing.T) {
	env := setupTestEnv(t)

	req, _ := http.NewRequest("GET", env.server.URL+"/users/alice", nil)
	req.Header.Set("Accept", "application/activity+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /users/alice: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var actor activitypub.Actor
	json.NewDecoder(resp.Body).Decode(&actor)

	if actor.PreferredUsername != "alice" {
		t.Errorf("preferredUsername = %q, want %q", actor.PreferredUsername, "alice")
	}
	if actor.Type != "Person" {
		t.Errorf("type = %q, want %q", actor.Type, "Person")
	}
	if actor.PublicKey.PublicKeyPEM == "" {
		t.Error("publicKeyPem is empty")
	}
	if !actor.Discoverable {
		t.Error("discoverable should be true by default")
	}
}

func TestActorDiscoverableDisabled(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	env.persona.Discoverable = false
	env.persona.UpdatedAt = time.Now()
	env.store.UpdatePersona(ctx, env.persona)

	req, _ := http.NewRequest("GET", env.server.URL+"/users/alice", nil)
	req.Header.Set("Accept", "application/activity+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /users/alice: %v", err)
	}
	defer resp.Body.Close()

	var actor activitypub.Actor
	json.NewDecoder(resp.Body).Decode(&actor)

	if actor.Discoverable {
		t.Error("discoverable should be false when disabled")
	}
}

func TestActorNotFound(t *testing.T) {
	env := setupTestEnv(t)

	req, _ := http.NewRequest("GET", env.server.URL+"/users/nobody", nil)
	req.Header.Set("Accept", "application/activity+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /users/nobody: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────
// WebFinger
// ─────────────────────────────────────────────

func TestWebFinger(t *testing.T) {
	env := setupTestEnv(t)

	resp, err := http.Get(env.server.URL + "/.well-known/webfinger?resource=acct:alice@murlog.test")
	if err != nil {
		t.Fatalf("GET webfinger: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	if body["subject"] != "acct:alice@murlog.test" {
		t.Errorf("subject = %v, want acct:alice@murlog.test", body["subject"])
	}
}

// ─────────────────────────────────────────────
// Inbox: Follow → Accept (signed)
// ─────────────────────────────────────────────

func TestInboxFollow(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")

	followActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      bob.actorURI() + "/activities/follow-1",
		Type:    "Follow",
		Actor:   bob.actorURI(),
		Object:  env.server.URL + "/users/alice",
	}

	body, _ := json.Marshal(followActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	// Verify follower was created.
	followers, err := env.store.ListFollowers(context.Background(), env.persona.ID)
	if err != nil {
		t.Fatalf("ListFollowers: %v", err)
	}
	if len(followers) != 1 {
		t.Fatalf("followers = %d, want 1", len(followers))
	}
	if followers[0].ActorURI != bob.actorURI() {
		t.Errorf("follower actor = %q, want %q", followers[0].ActorURI, bob.actorURI())
	}

	// Verify accept_follow job was queued.
	job, err := env.queue.Claim(context.Background())
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if job == nil {
		t.Fatal("accept_follow job not found in queue")
	}
	if job.Type != murlog.JobAcceptFollow {
		t.Errorf("job type = %q, want %q", job.Type, murlog.JobAcceptFollow)
	}

	// Verify remote actor was cached for display. / 表示用にリモート Actor がキャッシュされたことを確認。
	ra, err := env.store.GetRemoteActor(context.Background(), bob.actorURI())
	if err != nil {
		t.Fatalf("GetRemoteActor: %v", err)
	}
	if ra.Username != "bob" {
		t.Errorf("cached actor username = %q, want bob", ra.Username)
	}
}

// ─────────────────────────────────────────────
// Inbox: Follow (locked) → pending follower, no Accept
// ─────────────────────────────────────────────

func TestInboxFollowLocked(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")

	// Lock the persona. / ペルソナをロック。
	env.persona.Locked = true
	env.persona.UpdatedAt = time.Now()
	env.store.UpdatePersona(context.Background(), env.persona)

	followActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      bob.actorURI() + "/activities/follow-locked-1",
		Type:    "Follow",
		Actor:   bob.actorURI(),
		Object:  env.server.URL + "/users/alice",
	}

	body, _ := json.Marshal(followActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	// Approved followers list should be empty. / 承認済みフォロワーは空。
	followers, _ := env.store.ListFollowers(context.Background(), env.persona.ID)
	if len(followers) != 0 {
		t.Fatalf("approved followers = %d, want 0", len(followers))
	}

	// Pending followers list should have 1. / 保留中フォロワーが 1 件。
	pending, _ := env.store.ListPendingFollowers(context.Background(), env.persona.ID)
	if len(pending) != 1 {
		t.Fatalf("pending followers = %d, want 1", len(pending))
	}
	if pending[0].ActorURI != bob.actorURI() {
		t.Errorf("pending actor = %q, want %q", pending[0].ActorURI, bob.actorURI())
	}

	// No accept_follow job should be queued. / accept_follow ジョブはキューされない。
	job, _ := env.queue.Claim(context.Background())
	if job != nil {
		t.Errorf("expected no job, got type=%q", job.Type)
	}
}

// ─────────────────────────────────────────────
// Inbox: Undo Follow → remove follower (signed)
// ─────────────────────────────────────────────

func TestInboxUndoFollow(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Create a follower first.
	follower := &murlog.Follower{
		ID:        id.New(),
		PersonaID: env.persona.ID,
		ActorURI:  bob.actorURI(),
		CreatedAt: time.Now(),
		Approved:  true,
	}
	if err := env.store.CreateFollower(ctx, follower); err != nil {
		t.Fatalf("CreateFollower: %v", err)
	}

	undoActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       bob.actorURI() + "/activities/undo-1",
		"type":     "Undo",
		"actor":    bob.actorURI(),
		"object": map[string]interface{}{
			"type":   "Follow",
			"actor":  bob.actorURI(),
			"object": env.server.URL + "/users/alice",
		},
	}

	body, _ := json.Marshal(undoActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	// Verify follower was removed.
	followers, err := env.store.ListFollowers(ctx, env.persona.ID)
	if err != nil {
		t.Fatalf("ListFollowers: %v", err)
	}
	if len(followers) != 0 {
		t.Fatalf("followers = %d, want 0", len(followers))
	}
}

// ─────────────────────────────────────────────
// Inbox: Create (Note) → save to DB
// ─────────────────────────────────────────────

func TestInboxCreateNote(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Bob must be a follower of alice for the note to be accepted.
	// alice のフォロワーとして bob を登録。
	if err := env.store.CreateFollower(ctx, &murlog.Follower{
		ID:        id.New(),
		PersonaID: env.persona.ID,
		ActorURI:  bob.actorURI(),
		CreatedAt: time.Now(),
		Approved:  true,
	}); err != nil {
		t.Fatalf("CreateFollower: %v", err)
	}

	noteURI := bob.server.URL + "/users/bob/posts/1"
	createActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       bob.server.URL + "/activities/create-1",
		"type":     "Create",
		"actor":    bob.actorURI(),
		"object": map[string]interface{}{
			"id":           noteURI,
			"type":         "Note",
			"attributedTo": bob.actorURI(),
			"content":      "<p>Hello from Bob!</p>",
			"published":    "2026-04-12T00:00:00Z",
			"to":           []string{"https://www.w3.org/ns/activitystreams#Public"},
		},
	}

	body, _ := json.Marshal(createActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	// Verify post was saved in DB. / 投稿が DB に保存されたことを確認。
	post, err := env.store.GetPostByURI(ctx, noteURI)
	if err != nil {
		t.Fatalf("GetPostByURI: %v", err)
	}
	if post.Content != "<p>Hello from Bob!</p>" {
		t.Errorf("content = %q, want %q", post.Content, "<p>Hello from Bob!</p>")
	}
	if post.Origin != "remote" {
		t.Errorf("origin = %q, want %q", post.Origin, "remote")
	}
	if post.ActorURI != bob.actorURI() {
		t.Errorf("actor_uri = %q, want %q", post.ActorURI, bob.actorURI())
	}
	if post.URI != noteURI {
		t.Errorf("uri = %q, want %q", post.URI, noteURI)
	}
	if post.PersonaID != env.persona.ID {
		t.Errorf("persona_id = %v, want %v", post.PersonaID, env.persona.ID)
	}
}

func TestInboxCreateNoteDuplicate(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Bob is a follower.
	env.store.CreateFollower(ctx, &murlog.Follower{
		ID: id.New(), PersonaID: env.persona.ID,
		ActorURI:  bob.actorURI(), CreatedAt: time.Now(),
		Approved:  true,
	})

	noteURI := bob.server.URL + "/users/bob/posts/dup"
	createActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       bob.server.URL + "/activities/create-dup",
		"type":     "Create",
		"actor":    bob.actorURI(),
		"object": map[string]interface{}{
			"id":           noteURI,
			"type":         "Note",
			"attributedTo": bob.actorURI(),
			"content":      "<p>Duplicate test</p>",
			"published":    "2026-04-12T00:00:00Z",
			"to":           []string{"https://www.w3.org/ns/activitystreams#Public"},
		},
	}

	body, _ := json.Marshal(createActivity)

	// Send twice — second should be silently accepted (idempotent).
	// 2回送信 — 2回目は黙って受け入れる (冪等)。
	resp1 := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	resp1.Body.Close()
	if resp1.StatusCode != 202 {
		t.Fatalf("first send: status = %d, want 202", resp1.StatusCode)
	}

	resp2 := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	resp2.Body.Close()
	if resp2.StatusCode != 202 {
		t.Fatalf("second send: status = %d, want 202", resp2.StatusCode)
	}
}

// ─────────────────────────────────────────────
// Inbox: unsigned request → 401
// ─────────────────────────────────────────────

func TestInboxRejectsUnsignedRequest(t *testing.T) {
	env := setupTestEnv(t)

	followActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      "https://remote.test/activities/follow-unsigned",
		Type:    "Follow",
		Actor:   "https://remote.test/users/eve",
		Object:  env.server.URL + "/users/alice",
	}

	body, _ := json.Marshal(followActivity)
	resp, err := http.Post(env.server.URL+"/users/alice/inbox", "application/activity+json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST inbox: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}

	// Verify no follower was created.
	followers, err := env.store.ListFollowers(context.Background(), env.persona.ID)
	if err != nil {
		t.Fatalf("ListFollowers: %v", err)
	}
	if len(followers) != 0 {
		t.Fatalf("followers = %d, want 0", len(followers))
	}
}

// ─────────────────────────────────────────────
// Inbox: invalid signature → 401
// ─────────────────────────────────────────────

func TestInboxRejectsInvalidSignature(t *testing.T) {
	env := setupTestEnv(t)

	// Sign with one key, but serve a different public key.
	_, signPriv, _ := activitypub.GenerateKeyPair()
	wrongPub, _, _ := activitypub.GenerateKeyPair()

	remoteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor := activitypub.Actor{
			Context:          "https://www.w3.org/ns/activitystreams",
			ID:               "http://" + r.Host + "/users/mallory",
			Type:             "Person",
			PreferredUsername: "mallory",
			Inbox:            "http://" + r.Host + "/users/mallory/inbox",
			PublicKey: activitypub.PublicKey{
				ID:           "http://" + r.Host + "/users/mallory#main-key",
				Owner:        "http://" + r.Host + "/users/mallory",
				PublicKeyPEM: wrongPub,
			},
		}
		w.Header().Set("Content-Type", "application/activity+json")
		json.NewEncoder(w).Encode(actor)
	}))
	t.Cleanup(func() { remoteSrv.Close() })

	followActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      remoteSrv.URL + "/activities/follow-bad",
		Type:    "Follow",
		Actor:   remoteSrv.URL + "/users/mallory",
		Object:  env.server.URL + "/users/alice",
	}

	body, _ := json.Marshal(followActivity)
	req, _ := http.NewRequest("POST", env.server.URL+"/users/alice/inbox", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/activity+json")
	activitypub.SignRequest(req, remoteSrv.URL+"/users/mallory#main-key", signPriv, body)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST inbox: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────
// Inbox: tampered Digest → 401
// ─────────────────────────────────────────────

func TestInboxRejectsTamperedBody(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")

	// Sign a valid Follow activity, then send a different body.
	// The signature covers (request-target), host, date, digest.
	// Tampering the body makes the Digest header mismatch.
	// 正しい Follow Activity で署名し、異なるボディを送信する。
	// 署名は (request-target), host, date, digest をカバーしている。
	// ボディを改竄すると Digest ヘッダーとの不一致が発生する。
	followActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      bob.actorURI() + "/activities/follow-tampered",
		Type:    "Follow",
		Actor:   bob.actorURI(),
		Object:  env.server.URL + "/users/alice",
	}
	originalBody, _ := json.Marshal(followActivity)

	req, _ := http.NewRequest("POST", env.server.URL+"/users/alice/inbox", nil)
	req.Header.Set("Content-Type", "application/activity+json")
	if err := activitypub.SignRequest(req, bob.keyID(), bob.privPEM, originalBody); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	// Replace body with tampered content (different actor).
	// ボディを改竄（別の actor に差し替え）。
	tamperedActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      bob.actorURI() + "/activities/follow-tampered",
		Type:    "Follow",
		Actor:   "https://evil.test/users/mallory",
		Object:  env.server.URL + "/users/alice",
	}
	tamperedBody, _ := json.Marshal(tamperedActivity)
	req.Body = io.NopCloser(bytes.NewReader(tamperedBody))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST inbox: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401 for tampered body", resp.StatusCode)
	}

	// Verify no follower was created. / フォロワーが作成されていないことを確認。
	followers, err := env.store.ListFollowers(context.Background(), env.persona.ID)
	if err != nil {
		t.Fatalf("ListFollowers: %v", err)
	}
	if len(followers) != 0 {
		t.Fatalf("followers = %d, want 0", len(followers))
	}
}

func TestInboxRejectsMissingDigest(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")

	followActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      bob.actorURI() + "/activities/follow-nodigest",
		Type:    "Follow",
		Actor:   bob.actorURI(),
		Object:  env.server.URL + "/users/alice",
	}

	body, _ := json.Marshal(followActivity)

	// Sign WITHOUT digest in headers, and don't set Digest header.
	// This simulates a sender that doesn't include digest coverage.
	// Without explicit Digest verification, this would pass signature check.
	// digest を署名対象に含めず、Digest ヘッダーも設定しない。
	// 明示的な Digest 検証がないと署名検証を通過してしまう。
	req, _ := http.NewRequest("POST", env.server.URL+"/users/alice/inbox", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/activity+json")
	// Manual signing without digest coverage.
	if err := activitypub.SignRequest(req, bob.keyID(), bob.privPEM, nil); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}
	req.Header.Del("Digest")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST inbox: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401 for missing Digest", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────
// Outbox / Followers / Following collections
// ─────────────────────────────────────────────

func TestOutboxCollection(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Create a public local post. / 公開ローカル投稿を作成。
	now := time.Now()
	post := &murlog.Post{
		ID:         id.New(),
		PersonaID:  env.persona.ID,
		Content:    "<p>Hello from outbox</p>",
		Visibility: murlog.VisibilityPublic,
		Origin:     "local",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := env.store.CreatePost(ctx, post); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	req, _ := http.NewRequest("GET", env.server.URL+"/users/alice/outbox", nil)
	req.Header.Set("Accept", "application/activity+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET outbox: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["type"] != "OrderedCollection" {
		t.Errorf("type = %v, want OrderedCollection", body["type"])
	}

	totalItems := int(body["totalItems"].(float64))
	if totalItems != 1 {
		t.Errorf("totalItems = %d, want 1", totalItems)
	}

	items, ok := body["orderedItems"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("orderedItems length = %d, want 1", len(items))
	}

	activity, _ := items[0].(map[string]interface{})
	if activity["type"] != "Create" {
		t.Errorf("activity type = %v, want Create", activity["type"])
	}

	note, _ := activity["object"].(map[string]interface{})
	if note["type"] != "Note" {
		t.Errorf("note type = %v, want Note", note["type"])
	}
	if note["content"] != "<p>Hello from outbox</p>" {
		t.Errorf("note content = %v, want <p>Hello from outbox</p>", note["content"])
	}
}

func TestOutboxAnnounce(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Create an original local post. / 元のローカル投稿を作成。
	now := time.Now()
	original := &murlog.Post{
		ID:         id.New(),
		PersonaID:  env.persona.ID,
		Content:    "<p>Original post</p>",
		Visibility: murlog.VisibilityPublic,
		Origin:     "local",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := env.store.CreatePost(ctx, original); err != nil {
		t.Fatalf("CreatePost original: %v", err)
	}

	// Create a reblog wrapper post. / リブログ wrapper post を作成。
	wrapper := &murlog.Post{
		ID:             id.New(),
		PersonaID:      env.persona.ID,
		Visibility:     murlog.VisibilityPublic,
		Origin:         "local",
		ReblogOfPostID: original.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := env.store.CreatePost(ctx, wrapper); err != nil {
		t.Fatalf("CreatePost wrapper: %v", err)
	}

	req, _ := http.NewRequest("GET", env.server.URL+"/users/alice/outbox", nil)
	req.Header.Set("Accept", "application/activity+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET outbox: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	items, ok := body["orderedItems"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("orderedItems length = %d, want 2", len(items))
	}

	// Verify both Announce and Create activities exist (order may vary).
	// Announce と Create の両方が存在することを確認 (順序は不定)。
	types := map[string]bool{}
	for _, item := range items {
		act, _ := item.(map[string]interface{})
		if tp, ok := act["type"].(string); ok {
			types[tp] = true
		}
	}
	if !types["Announce"] {
		t.Error("Announce activity not found in outbox")
	}
	if !types["Create"] {
		t.Error("Create activity not found in outbox")
	}
}

func TestFollowersCollection(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Create a follower record. / フォロワーレコードを作成。
	follower := &murlog.Follower{
		ID:        id.New(),
		PersonaID: env.persona.ID,
		ActorURI:  "https://remote.test/users/bob",
		CreatedAt: time.Now(),
		Approved:  true,
	}
	if err := env.store.CreateFollower(ctx, follower); err != nil {
		t.Fatalf("CreateFollower: %v", err)
	}

	req, _ := http.NewRequest("GET", env.server.URL+"/users/alice/followers", nil)
	req.Header.Set("Accept", "application/activity+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET followers: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["type"] != "OrderedCollection" {
		t.Errorf("type = %v, want OrderedCollection", body["type"])
	}

	totalItems := int(body["totalItems"].(float64))
	if totalItems != 1 {
		t.Errorf("totalItems = %d, want 1", totalItems)
	}

	items, ok := body["orderedItems"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("orderedItems length = %d, want 1", len(items))
	}
	if items[0] != "https://remote.test/users/bob" {
		t.Errorf("orderedItems[0] = %v, want https://remote.test/users/bob", items[0])
	}
}

func TestFollowersCollectionHidden(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Disable show_follows. / show_follows を無効化。
	env.persona.ShowFollows = false
	env.persona.UpdatedAt = time.Now()
	env.store.UpdatePersona(ctx, env.persona)

	// Create a follower record. / フォロワーレコードを作成。
	env.store.CreateFollower(ctx, &murlog.Follower{
		ID: id.New(), PersonaID: env.persona.ID,
		ActorURI: "https://remote.test/users/bob", CreatedAt: time.Now(), Approved: true,
	})

	req, _ := http.NewRequest("GET", env.server.URL+"/users/alice/followers", nil)
	req.Header.Set("Accept", "application/activity+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET followers: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	// totalItems should still report real count.
	// totalItems は実際のカウントを返すべき。
	totalItems := int(body["totalItems"].(float64))
	if totalItems != 1 {
		t.Errorf("totalItems = %d, want 1", totalItems)
	}

	// orderedItems should be empty (hidden).
	// orderedItems は空であるべき（非公開）。
	items, _ := body["orderedItems"].([]interface{})
	if len(items) != 0 {
		t.Errorf("orderedItems length = %d, want 0 (hidden)", len(items))
	}
}

func TestFollowingCollection(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Create a follow record (accepted). / フォローレコードを作成 (承認済み)。
	follow := &murlog.Follow{
		ID:        id.New(),
		PersonaID: env.persona.ID,
		TargetURI: "https://remote.test/users/carol",
		Accepted:  true,
		CreatedAt: time.Now(),
	}
	if err := env.store.CreateFollow(ctx, follow); err != nil {
		t.Fatalf("CreateFollow: %v", err)
	}

	req, _ := http.NewRequest("GET", env.server.URL+"/users/alice/following", nil)
	req.Header.Set("Accept", "application/activity+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET following: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["type"] != "OrderedCollection" {
		t.Errorf("type = %v, want OrderedCollection", body["type"])
	}

	totalItems := int(body["totalItems"].(float64))
	if totalItems != 1 {
		t.Errorf("totalItems = %d, want 1", totalItems)
	}

	items, ok := body["orderedItems"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("orderedItems length = %d, want 1", len(items))
	}
	if items[0] != "https://remote.test/users/carol" {
		t.Errorf("orderedItems[0] = %v, want https://remote.test/users/carol", items[0])
	}
}

func TestFollowingCollectionHidden(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Disable show_follows. / show_follows を無効化。
	env.persona.ShowFollows = false
	env.persona.UpdatedAt = time.Now()
	env.store.UpdatePersona(ctx, env.persona)

	// Create a follow record. / フォローレコードを作成。
	env.store.CreateFollow(ctx, &murlog.Follow{
		ID: id.New(), PersonaID: env.persona.ID,
		TargetURI: "https://remote.test/users/carol", Accepted: true, CreatedAt: time.Now(),
	})

	req, _ := http.NewRequest("GET", env.server.URL+"/users/alice/following", nil)
	req.Header.Set("Accept", "application/activity+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET following: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	totalItems := int(body["totalItems"].(float64))
	if totalItems != 1 {
		t.Errorf("totalItems = %d, want 1", totalItems)
	}

	items, _ := body["orderedItems"].([]interface{})
	if len(items) != 0 {
		t.Errorf("orderedItems length = %d, want 0 (hidden)", len(items))
	}
}

// ────────────────────────────────────────────────
// Inbox: Like → create favourite + notification
// ────────────────────────────────────────────────

func TestInboxLike(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Create a local post to be liked. / いいねされるローカル投稿を作成。
	now := time.Now()
	post := &murlog.Post{
		ID:         id.New(),
		PersonaID:  env.persona.ID,
		Content:    "<p>Like me</p>",
		Visibility: murlog.VisibilityPublic,
		Origin:     "local",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := env.store.CreatePost(ctx, post); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	postURI := env.server.URL + "/users/alice/posts/" + post.ID.String()
	likeActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      bob.actorURI() + "/activities/like-1",
		Type:    "Like",
		Actor:   bob.actorURI(),
		Object:  postURI,
	}

	body, _ := json.Marshal(likeActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	// Verify favourite was created. / お気に入りが作成されたことを確認。
	favs, err := env.store.ListFavouritesByPost(ctx, post.ID)
	if err != nil {
		t.Fatalf("ListFavouritesByPost: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("favourites = %d, want 1", len(favs))
	}
	if favs[0].ActorURI != bob.actorURI() {
		t.Errorf("favourite actor = %q, want %q", favs[0].ActorURI, bob.actorURI())
	}

	// Verify notification was created. / 通知が作成されたことを確認。
	notifs, err := env.store.ListNotifications(ctx, env.persona.ID, id.Nil, 10)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(notifs) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifs))
	}
	if notifs[0].Type != "favourite" {
		t.Errorf("notification type = %q, want favourite", notifs[0].Type)
	}

	// Verify remote actor was cached for display. / 表示用にリモート Actor がキャッシュされたことを確認。
	ra, err := env.store.GetRemoteActor(ctx, bob.actorURI())
	if err != nil {
		t.Fatalf("GetRemoteActor: %v", err)
	}
	if ra.Username != "bob" {
		t.Errorf("cached actor username = %q, want bob", ra.Username)
	}
}

// ─────────────────────────────────────────────
// Inbox: Announce → create reblog + notification
// ─────────────────────────────────────────────

func TestInboxAnnounce(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Create a local post to be announced. / リブログされるローカル投稿を作成。
	now := time.Now()
	post := &murlog.Post{
		ID:         id.New(),
		PersonaID:  env.persona.ID,
		Content:    "<p>Boost me</p>",
		Visibility: murlog.VisibilityPublic,
		Origin:     "local",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := env.store.CreatePost(ctx, post); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	postURI := env.server.URL + "/users/alice/posts/" + post.ID.String()
	announceActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      bob.actorURI() + "/activities/announce-1",
		Type:    "Announce",
		Actor:   bob.actorURI(),
		Object:  postURI,
	}

	body, _ := json.Marshal(announceActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	// Verify reblog was created. / リブログが作成されたことを確認。
	reblogs, err := env.store.ListReblogsByPost(ctx, post.ID)
	if err != nil {
		t.Fatalf("ListReblogsByPost: %v", err)
	}
	if len(reblogs) != 1 {
		t.Fatalf("reblogs = %d, want 1", len(reblogs))
	}
	if reblogs[0].ActorURI != bob.actorURI() {
		t.Errorf("reblog actor = %q, want %q", reblogs[0].ActorURI, bob.actorURI())
	}

	// Verify notification was created. / 通知が作成されたことを確認。
	notifs, err := env.store.ListNotifications(ctx, env.persona.ID, id.Nil, 10)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(notifs) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifs))
	}
	if notifs[0].Type != "reblog" {
		t.Errorf("notification type = %q, want reblog", notifs[0].Type)
	}

	// Verify remote actor was cached for display. / 表示用にリモート Actor がキャッシュされたことを確認。
	ra, err := env.store.GetRemoteActor(ctx, bob.actorURI())
	if err != nil {
		t.Fatalf("GetRemoteActor: %v", err)
	}
	if ra.Username != "bob" {
		t.Errorf("cached actor username = %q, want bob", ra.Username)
	}
}

// ────────────────────────────────────────────────
// Inbox: Announce (remote Note) → fetch Note and create reblogged post
// ────────────────────────────────────────────────

func TestInboxAnnounceRemoteNote(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Start a server that serves a remote Note. / リモート Note を返すサーバーを起動。
	noteURI := ""
	noteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/activity+json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"@context":     "https://www.w3.org/ns/activitystreams",
			"id":           noteURI,
			"type":         "Note",
			"attributedTo": "https://other.example/users/carol",
			"content":      "<p>Hello from Carol</p>",
			"published":    time.Now().Format(time.RFC3339),
			"to":           []string{"https://www.w3.org/ns/activitystreams#Public"},
		})
	}))
	defer noteServer.Close()
	noteURI = noteServer.URL + "/notes/123"

	// Send Announce activity. / Announce Activity を送信。
	announceActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      bob.actorURI() + "/activities/announce-remote-1",
		Type:    "Announce",
		Actor:   bob.actorURI(),
		Object:  noteURI,
	}

	body, _ := json.Marshal(announceActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	// Verify post was created with reblogged_by_uri.
	// reblogged_by_uri 付きで投稿が作成されたことを確認。
	post, err := env.store.GetPostByURI(ctx, noteURI)
	if err != nil {
		t.Fatalf("GetPostByURI: %v", err)
	}
	if post.Origin != "remote" {
		t.Errorf("origin = %q, want remote", post.Origin)
	}
	if post.Content != "<p>Hello from Carol</p>" {
		t.Errorf("content = %q, want '<p>Hello from Carol</p>'", post.Content)
	}
	if post.ActorURI != "https://other.example/users/carol" {
		t.Errorf("actor_uri = %q, want carol", post.ActorURI)
	}
	if post.RebloggedByURI != bob.actorURI() {
		t.Errorf("reblogged_by_uri = %q, want %q", post.RebloggedByURI, bob.actorURI())
	}

	// Idempotent: send same Announce again → no duplicate.
	// 冪等: 同じ Announce を再送 → 重複なし。
	resp2 := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp2.Body.Close()
	if resp2.StatusCode != 202 {
		t.Fatalf("idempotent status = %d, want 202", resp2.StatusCode)
	}
}

// ────────────────────────────────────────────────
// Inbox: Undo Like → remove favourite
// ────────────────────────────────────────────────

func TestInboxUndoLike(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Create a local post and a favourite. / ローカル投稿とお気に入りを作成。
	now := time.Now()
	post := &murlog.Post{
		ID:         id.New(),
		PersonaID:  env.persona.ID,
		Content:    "<p>Unlike me</p>",
		Visibility: murlog.VisibilityPublic,
		Origin:     "local",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	env.store.CreatePost(ctx, post)
	env.store.CreateFavourite(ctx, &murlog.Favourite{
		ID:        id.New(),
		PostID:    post.ID,
		ActorURI:  bob.actorURI(),
		CreatedAt: now,
	})

	// Create a favourite notification. / お気に入り通知を作成。
	env.store.CreateNotification(ctx, &murlog.Notification{
		ID:        id.New(),
		PersonaID: env.persona.ID,
		Type:      "favourite",
		ActorURI:  bob.actorURI(),
		PostID:    post.ID,
		CreatedAt: now,
	})

	postURI := env.server.URL + "/users/alice/posts/" + post.ID.String()
	undoActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       bob.actorURI() + "/activities/undo-like-1",
		"type":     "Undo",
		"actor":    bob.actorURI(),
		"object": map[string]interface{}{
			"type":   "Like",
			"id":     bob.actorURI() + "/activities/like-1",
			"actor":  bob.actorURI(),
			"object": postURI,
		},
	}

	body, _ := json.Marshal(undoActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	// Verify favourite was removed. / お気に入りが削除されたことを確認。
	favs, _ := env.store.ListFavouritesByPost(ctx, post.ID)
	if len(favs) != 0 {
		t.Fatalf("favourites = %d, want 0", len(favs))
	}

	// Verify notification was also removed. / 通知も削除されたことを確認。
	notifs, _ := env.store.ListNotifications(ctx, env.persona.ID, id.ID{}, 10)
	for _, n := range notifs {
		if n.Type == "favourite" && n.ActorURI == bob.actorURI() {
			t.Error("favourite notification should be removed after Undo Like")
		}
	}
}

// ─────────────────────────────────────────────
// Inbox: Undo Announce → remove reblog
// ─────────────────────────────────────────────

func TestInboxUndoAnnounce(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Create a local post and a reblog. / ローカル投稿とリブログを作成。
	now := time.Now()
	post := &murlog.Post{
		ID:         id.New(),
		PersonaID:  env.persona.ID,
		Content:    "<p>Unreblog me</p>",
		Visibility: murlog.VisibilityPublic,
		Origin:     "local",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	env.store.CreatePost(ctx, post)
	env.store.CreateReblog(ctx, &murlog.Reblog{
		ID:        id.New(),
		PostID:    post.ID,
		ActorURI:  bob.actorURI(),
		CreatedAt: now,
	})

	postURI := env.server.URL + "/users/alice/posts/" + post.ID.String()
	undoActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       bob.actorURI() + "/activities/undo-announce-1",
		"type":     "Undo",
		"actor":    bob.actorURI(),
		"object": map[string]interface{}{
			"type":   "Announce",
			"id":     bob.actorURI() + "/activities/announce-1",
			"actor":  bob.actorURI(),
			"object": postURI,
		},
	}

	body, _ := json.Marshal(undoActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	// Verify reblog was removed. / リブログが削除されたことを確認。
	reblogs, _ := env.store.ListReblogsByPost(ctx, post.ID)
	if len(reblogs) != 0 {
		t.Fatalf("reblogs = %d, want 0", len(reblogs))
	}
}

// ---------------------------------------------
// Inbox: Block -> remove bidirectional follow relationships
// ---------------------------------------------

func TestInboxBlock(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Create follower (bob -> alice) and follow (alice -> bob).
	now := time.Now()
	env.store.CreateFollower(ctx, &murlog.Follower{
		ID: id.New(), PersonaID: env.persona.ID,
		ActorURI: bob.actorURI(),
		Approved: true,
		CreatedAt: now,
	})
	env.store.CreateFollow(ctx, &murlog.Follow{
		ID: id.New(), PersonaID: env.persona.ID,
		TargetURI: bob.actorURI(), Accepted: true, CreatedAt: now,
	})

	blockActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      bob.actorURI() + "/activities/block-1",
		Type:    "Block",
		Actor:   bob.actorURI(),
		Object:  env.server.URL + "/users/alice",
	}

	body, _ := json.Marshal(blockActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	followers, _ := env.store.ListFollowers(ctx, env.persona.ID)
	if len(followers) != 0 {
		t.Fatalf("followers = %d, want 0", len(followers))
	}

	follows, _ := env.store.ListFollows(ctx, env.persona.ID)
	if len(follows) != 0 {
		t.Fatalf("follows = %d, want 0", len(follows))
	}
}

// ---------------------------------------------
// Inbox: blocked actor's Follow is rejected
// ---------------------------------------------

func TestInboxBlockedActorRejected(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	env.store.CreateBlock(ctx, &murlog.Block{
		ID: id.New(), ActorURI: bob.actorURI(), CreatedAt: time.Now(),
	})

	followActivity := activitypub.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      bob.actorURI() + "/activities/follow-1",
		Type:    "Follow",
		Actor:   bob.actorURI(),
		Object:  env.server.URL + "/users/alice",
	}

	body, _ := json.Marshal(followActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	followers, _ := env.store.ListFollowers(ctx, env.persona.ID)
	if len(followers) != 0 {
		t.Fatalf("followers = %d, want 0 (blocked actor should be rejected)", len(followers))
	}
}

// ---------------------------------------------
// Inbox: domain-blocked actor's Create is rejected
// ---------------------------------------------

// ─────────────────────────────────────────────
// Inbox: Create (Note with Mention tag) → mention notification
// ─────────────────────────────────────────────

func TestInboxCreateNoteWithMentionTag(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Bob is a follower of alice.
	env.store.CreateFollower(ctx, &murlog.Follower{
		ID: id.New(), PersonaID: env.persona.ID,
		ActorURI:  bob.actorURI(), CreatedAt: time.Now(),
		Approved:  true,
	})

	// Bob sends a Note that mentions alice via tag (not a reply).
	// bob が alice を tag でメンションする Note を送信 (リプライではない)。
	noteURI := bob.server.URL + "/users/bob/posts/mention-1"
	// Use the canonical alice URI based on DB settings (https://murlog.test).
	// DB 設定に基づく正規の alice URI を使用 (https://murlog.test)。
	aliceURI := "https://murlog.test/users/alice"
	createActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       bob.server.URL + "/activities/create-mention-1",
		"type":     "Create",
		"actor":    bob.actorURI(),
		"object": map[string]interface{}{
			"id":           noteURI,
			"type":         "Note",
			"attributedTo": bob.actorURI(),
			"content":      `<p><span class="h-card"><a href="` + aliceURI + `" class="u-url mention">@<span>alice</span></a></span> hello!</p>`,
			"published":    "2026-04-17T00:00:00Z",
			"to":           []string{"https://www.w3.org/ns/activitystreams#Public"},
			"cc":           []string{aliceURI},
			"tag": []map[string]interface{}{
				{
					"type": "Mention",
					"href": aliceURI,
					"name": "@alice@murlog.test",
				},
			},
		},
	}

	body, _ := json.Marshal(createActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	// Verify post was saved. / 投稿が保存されたことを確認。
	post, err := env.store.GetPostByURI(ctx, noteURI)
	if err != nil {
		t.Fatalf("GetPostByURI: %v", err)
	}
	if post.Origin != "remote" {
		t.Errorf("origin = %q, want remote", post.Origin)
	}

	// Verify mention notification was created. / メンション通知が作成されたことを確認。
	notifs, err := env.store.ListNotifications(ctx, env.persona.ID, id.Nil, 10)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(notifs) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifs))
	}
	if notifs[0].Type != "mention" {
		t.Errorf("notification type = %q, want mention", notifs[0].Type)
	}
	if notifs[0].ActorURI != bob.actorURI() {
		t.Errorf("notification actor = %q, want %q", notifs[0].ActorURI, bob.actorURI())
	}
	if notifs[0].PostID != post.ID {
		t.Errorf("notification post_id = %v, want %v", notifs[0].PostID, post.ID)
	}
}

func TestInboxCreateNoteReplyAndMentionNoDuplicate(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Bob is a follower of alice.
	env.store.CreateFollower(ctx, &murlog.Follower{
		ID: id.New(), PersonaID: env.persona.ID,
		ActorURI:  bob.actorURI(), CreatedAt: time.Now(),
		Approved:  true,
	})

	// Create a local post that Bob will reply to while also mentioning alice.
	// bob がリプライしつつ alice をメンションするローカル投稿を作成。
	now := time.Now()
	localPost := &murlog.Post{
		ID:         id.New(),
		PersonaID:  env.persona.ID,
		Content:    "<p>Original post</p>",
		Visibility: murlog.VisibilityPublic,
		Origin:     "local",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	env.store.CreatePost(ctx, localPost)

	// Use canonical base URL for local post URI and alice URI.
	// ローカル投稿 URI と alice URI は正規のベース URL を使用。
	localPostURI := "https://murlog.test/users/alice/posts/" + localPost.ID.String()
	aliceURI := "https://murlog.test/users/alice"
	noteURI := bob.server.URL + "/users/bob/posts/reply-mention-1"
	createActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       bob.server.URL + "/activities/create-reply-mention-1",
		"type":     "Create",
		"actor":    bob.actorURI(),
		"object": map[string]interface{}{
			"id":           noteURI,
			"type":         "Note",
			"attributedTo": bob.actorURI(),
			"inReplyTo":    localPostURI,
			"content":      `<p><span class="h-card"><a href="` + aliceURI + `">@<span>alice</span></a></span> replying!</p>`,
			"published":    "2026-04-17T00:00:00Z",
			"to":           []string{"https://www.w3.org/ns/activitystreams#Public"},
			"cc":           []string{aliceURI},
			"tag": []map[string]interface{}{
				{
					"type": "Mention",
					"href": aliceURI,
					"name": "@alice@murlog.test",
				},
			},
		},
	}

	body, _ := json.Marshal(createActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	// Should have exactly 1 notification (not 2 — reply + tag should dedup).
	// 通知は1件のみ (リプライ + tag で重複しない)。
	notifs, err := env.store.ListNotifications(ctx, env.persona.ID, id.Nil, 10)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(notifs) != 1 {
		t.Fatalf("notifications = %d, want 1 (reply + mention tag should not create duplicate)", len(notifs))
	}
	if notifs[0].Type != "mention" {
		t.Errorf("notification type = %q, want mention", notifs[0].Type)
	}
}

// ---------------------------------------------
// Inbox: domain-blocked actor's Create is rejected
// ---------------------------------------------

func TestInboxDomainBlockedRejected(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Extract domain (without port) from bob's server URL.
	// domainFromURI uses url.Parse().Hostname() which strips port.
	// domainFromURI は url.Parse().Hostname() を使いポートを除去する。
	bobURL, _ := url.Parse(bob.server.URL)
	bobHost := bobURL.Hostname()

	env.store.CreateDomainBlock(ctx, &murlog.DomainBlock{
		ID: id.New(), Domain: bobHost, CreatedAt: time.Now(),
	})

	createActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       bob.actorURI() + "/activities/create-1",
		"type":     "Create",
		"actor":    bob.actorURI(),
		"object": map[string]interface{}{
			"type":         "Note",
			"id":           bob.actorURI() + "/posts/1",
			"attributedTo": bob.actorURI(),
			"content":      "<p>Should be blocked</p>",
			"published":    time.Now().UTC().Format(time.RFC3339),
		},
	}

	body, _ := json.Marshal(createActivity)
	resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
	}

	posts, _ := env.store.ListPostsByPersona(ctx, env.persona.ID, id.Nil, 10)
	if len(posts) != 0 {
		t.Fatalf("posts = %d, want 0 (domain-blocked actor should be rejected)", len(posts))
	}
}

func TestInboxCreateNoteTruncatesLongContent(t *testing.T) {
	env := setupTestEnv(t)
	bob := setupRemoteActor(t, "bob")
	ctx := context.Background()

	// Bob is a follower. / bob をフォロワー登録。
	env.store.CreateFollower(ctx, &murlog.Follower{
		ID: id.New(), PersonaID: env.persona.ID,
		ActorURI:  bob.actorURI(), CreatedAt: time.Now(),
		Approved:  true,
	})

	sendNote := func(t *testing.T, noteID, content string) *murlog.Post {
		t.Helper()
		createActivity := map[string]interface{}{
			"@context": "https://www.w3.org/ns/activitystreams",
			"id":       bob.server.URL + "/activities/" + noteID,
			"type":     "Create",
			"actor":    bob.actorURI(),
			"object": map[string]interface{}{
				"id":           bob.server.URL + "/users/bob/posts/" + noteID,
				"type":         "Note",
				"attributedTo": bob.actorURI(),
				"content":      content,
				"published":    time.Now().UTC().Format(time.RFC3339),
			},
		}
		body, _ := json.Marshal(createActivity)
		resp := bob.signedPost(t, env.server.URL+"/users/alice/inbox", body)
		defer resp.Body.Close()
		if resp.StatusCode != 202 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 202; body = %s", resp.StatusCode, respBody)
		}
		post, err := env.store.GetPostByURI(ctx, bob.server.URL+"/users/bob/posts/"+noteID)
		if err != nil {
			t.Fatalf("GetPostByURI: %v", err)
		}
		return post
	}

	// Exactly 3000 runes → not truncated / ちょうど 3000 rune → 切り詰めなし
	t.Run("exactly_3000", func(t *testing.T) {
		content := "<p>" + strings.Repeat("あ", 2996) + "</p>" // <p></p> = 7 chars + 2996 = 3003... let's use plain chars
		// Use runes that survive HTML sanitization.
		// HTML サニタイズを通過するルーンを使用。
		content = strings.Repeat("a", 3000)
		post := sendNote(t, "exact3000", content)
		if len([]rune(post.Content)) != 3000 {
			t.Fatalf("content length = %d, want 3000", len([]rune(post.Content)))
		}
	})

	// 3001 runes → truncated to 3000 / 3001 rune → 3000 に切り詰め
	t.Run("over_3000_truncated", func(t *testing.T) {
		content := strings.Repeat("b", 3001)
		post := sendNote(t, "over3000", content)
		if runeLen := len([]rune(post.Content)); runeLen != 3000 {
			t.Fatalf("content length = %d, want 3000", runeLen)
		}
	})
}
