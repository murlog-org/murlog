package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

func TestTimelineHome(t *testing.T) {
	env := setupTestEnv(t)
	cookie := loginTestEnv(t, env, "testpass")
	ctx := context.Background()

	// Create posts (local + remote).
	now := time.Now()
	for i, origin := range []string{"local", "remote"} {
		env.store.CreatePost(ctx, &murlog.Post{
			ID:         id.New(),
			PersonaID:  env.persona.ID,
			Content:    "<p>post " + origin + "</p>",
			Visibility: murlog.VisibilityPublic,
			Origin:     origin,
			CreatedAt:  now.Add(time.Duration(i) * time.Second),
			UpdatedAt:  now.Add(time.Duration(i) * time.Second),
		})
	}

	var posts []postJSON
	rpcCallWithCookie(t, env, "timeline.home", map[string]any{}, &posts, cookie)

	if len(posts) != 2 {
		t.Fatalf("timeline.home: got %d posts, want 2", len(posts))
	}
}

func TestFollowsCRUD(t *testing.T) {
	env := setupTestEnv(t)
	cookie := loginTestEnv(t, env, "testpass")

	// Create follow.
	var created followJSON
	rpcCallWithCookie(t, env, "follows.create", map[string]string{
		"target_uri": "https://remote.example/users/bob",
	}, &created, cookie)
	if created.TargetURI != "https://remote.example/users/bob" {
		t.Errorf("target_uri = %q, want remote bob", created.TargetURI)
	}

	// List follows.
	var follows []followJSON
	rpcCallWithCookie(t, env, "follows.list", map[string]any{}, &follows, cookie)
	if len(follows) != 1 {
		t.Fatalf("follows.list: got %d, want 1", len(follows))
	}

	// Delete follow.
	rpcCallWithCookie(t, env, "follows.delete", map[string]string{
		"id": created.ID,
	}, nil, cookie)

	// Verify deleted.
	rpcCallWithCookie(t, env, "follows.list", map[string]any{}, &follows, cookie)
	if len(follows) != 0 {
		t.Fatalf("follows.list after delete: got %d, want 0", len(follows))
	}
}

func TestFollowersCRUD(t *testing.T) {
	env := setupTestEnv(t)
	cookie := loginTestEnv(t, env, "testpass")
	ctx := context.Background()

	// Simulate a remote follower.
	follower := &murlog.Follower{
		ID:        id.New(),
		PersonaID: env.persona.ID,
		ActorURI:  "https://remote.example/users/carol",
		CreatedAt: time.Now(),
		Approved:  true,
	}
	env.store.CreateFollower(ctx, follower)

	// List followers.
	var followers []followerJSON
	rpcCallWithCookie(t, env, "followers.list", map[string]any{}, &followers, cookie)
	if len(followers) != 1 {
		t.Fatalf("followers.list: got %d, want 1", len(followers))
	}

	// Delete follower.
	rpcCallWithCookie(t, env, "followers.delete", map[string]string{
		"id": follower.ID.String(),
	}, nil, cookie)

	rpcCallWithCookie(t, env, "followers.list", map[string]any{}, &followers, cookie)
	if len(followers) != 0 {
		t.Fatalf("followers.list after delete: got %d, want 0", len(followers))
	}
}

func TestNotificationsCRUD(t *testing.T) {
	env := setupTestEnv(t)
	cookie := loginTestEnv(t, env, "testpass")
	ctx := context.Background()

	// Create notifications with distinct timestamps (RFC3339 is second-precision).
	now := time.Now().Truncate(time.Second)
	n1 := &murlog.Notification{
		ID:        id.New(),
		PersonaID: env.persona.ID,
		Type:      "follow",
		ActorURI:  "https://remote.example/users/bob",
		CreatedAt: now,
	}
	time.Sleep(2 * time.Millisecond) // ensure n2 gets a later UUIDv7
	n2 := &murlog.Notification{
		ID:        id.New(),
		PersonaID: env.persona.ID,
		Type:      "mention",
		ActorURI:  "https://remote.example/users/carol",
		PostID:    id.New(),
		CreatedAt: now.Add(time.Second),
	}
	env.store.CreateNotification(ctx, n1)
	env.store.CreateNotification(ctx, n2)

	// List.
	var notifications []notificationJSON
	rpcCallWithCookie(t, env, "notifications.list", map[string]any{}, &notifications, cookie)
	if len(notifications) != 2 {
		t.Fatalf("notifications.list: got %d, want 2", len(notifications))
	}

	// Read one.
	rpcCallWithCookie(t, env, "notifications.read", map[string]string{
		"id": n1.ID.String(),
	}, nil, cookie)

	// Read all.
	rpcCallWithCookie(t, env, "notifications.read_all", map[string]any{}, nil, cookie)

	// Count unread — both were marked read above, so expect 0.
	// 未読カウント — 上で全部既読にしたので 0 のはず。
	var countResult map[string]int
	rpcCallWithCookie(t, env, "notifications.count_unread", map[string]any{}, &countResult, cookie)
	if countResult["count"] != 0 {
		t.Errorf("count_unread after read_all = %d, want 0", countResult["count"])
	}

	// Create a new unread notification and verify count.
	// 新しい未読通知を作って count を確認。
	n3 := &murlog.Notification{
		ID:        id.New(),
		PersonaID: env.persona.ID,
		Type:      "favourite",
		ActorURI:  "https://remote.example/users/dave",
		PostID:    id.New(),
		CreatedAt: now.Add(2 * time.Second),
	}
	env.store.CreateNotification(ctx, n3)
	rpcCallWithCookie(t, env, "notifications.count_unread", map[string]any{}, &countResult, cookie)
	if countResult["count"] != 1 {
		t.Errorf("count_unread = %d, want 1", countResult["count"])
	}

	// Poll since n1 — should get n2 and n3.
	var polled []notificationJSON
	rpcCallWithCookie(t, env, "notifications.poll", map[string]any{
		"since": n1.ID.String(),
	}, &polled, cookie)
	if len(polled) != 2 {
		t.Fatalf("notifications.poll: got %d, want 2", len(polled))
	}

	// Delete n3 via RPC.
	// RPC で n3 を削除。
	rpcCallWithCookie(t, env, "notifications.delete", map[string]string{
		"id": n3.ID.String(),
	}, nil, cookie)
	rpcCallWithCookie(t, env, "notifications.list", map[string]any{}, &notifications, cookie)
	if len(notifications) != 2 {
		t.Errorf("notifications after delete: got %d, want 2", len(notifications))
	}

	// DeleteNotificationByActor — remove n1 (follow from bob).
	// DeleteNotificationByActor — n1 (bob からのフォロー) を削除。
	env.store.DeleteNotificationByActor(ctx, env.persona.ID, "https://remote.example/users/bob", "follow", id.ID{})
	rpcCallWithCookie(t, env, "notifications.list", map[string]any{}, &notifications, cookie)
	if len(notifications) != 1 {
		t.Errorf("notifications after DeleteNotificationByActor: got %d, want 1", len(notifications))
	}
}

func TestBatchRPCLimit(t *testing.T) {
	env := setupTestEnv(t)

	sendBatch := func(t *testing.T, count int) (int, []byte) {
		t.Helper()
		reqs := make([]map[string]any, count)
		for i := range reqs {
			reqs[i] = map[string]any{
				"jsonrpc": "2.0",
				"method":  "system.version",
				"id":      i + 1,
			}
		}
		body, _ := json.Marshal(reqs)
		resp, err := http.Post(env.server.URL+"/api/mur/v1/rpc", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("batch request: %v", err)
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, respBody
	}

	// 100 requests → accepted / 100 件 → 受理
	t.Run("100_accepted", func(t *testing.T) {
		status, _ := sendBatch(t, 100)
		if status != http.StatusOK {
			t.Fatalf("batch 100: want 200, got %d", status)
		}
	})

	// 101 requests → batch too large / 101 件 → バッチサイズ超過
	t.Run("101_rejected", func(t *testing.T) {
		status, body := sendBatch(t, 101)
		if status != http.StatusOK {
			t.Fatalf("batch 101: want 200 (JSON-RPC error), got %d", status)
		}
		var rpcResp testRPCResponse
		if err := json.Unmarshal(body, &rpcResp); err != nil {
			t.Fatalf("batch 101: decode: %v", err)
		}
		if rpcResp.Error == nil || rpcResp.Error.Message != "batch too large" {
			t.Fatalf("batch 101: want 'batch too large', got %+v", rpcResp.Error)
		}
	})

	// Empty batch → empty batch error / 空バッチ → エラー
	t.Run("empty_rejected", func(t *testing.T) {
		body, _ := json.Marshal([]map[string]any{})
		resp, err := http.Post(env.server.URL+"/api/mur/v1/rpc", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("empty batch: %v", err)
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		var rpcResp testRPCResponse
		if err := json.Unmarshal(respBody, &rpcResp); err != nil {
			t.Fatalf("empty batch: decode: %v", err)
		}
		if rpcResp.Error == nil || rpcResp.Error.Message != "empty batch" {
			t.Fatalf("empty batch: want 'empty batch', got %+v", rpcResp.Error)
		}
	})

}

func TestPostsUpdateContentTooLong(t *testing.T) {
	env := setupTestEnv(t)
	cookie := loginTestEnv(t, env, "testpass")

	// Create a post first. / まず投稿を作成。
	var post postJSON
	rpcCallWithCookie(t, env, "posts.create", map[string]any{
		"content": "<p>short</p>",
	}, &post, cookie)

	// Update with 3001 runes → rejected / 3001 rune で更新 → 拒否
	longContent := strings.Repeat("x", 3001)
	resp := rpcCallRaw(t, env, "posts.update", map[string]any{
		"id": post.ID, "content": longContent,
	}, cookie)
	if resp.Error == nil {
		t.Fatal("posts.update with 3001 chars: expected error")
	}
	if resp.Error.Message != "content too long" {
		t.Fatalf("posts.update error = %q, want 'content too long'", resp.Error.Message)
	}

	// Update with exactly 3000 runes → accepted / ちょうど 3000 rune → 受理
	exactContent := strings.Repeat("y", 3000)
	var updated postJSON
	rpcCallWithCookie(t, env, "posts.update", map[string]any{
		"id": post.ID, "content": exactContent,
	}, &updated, cookie)
	if len([]rune(updated.Content)) != 3000 {
		t.Fatalf("posts.update content length = %d, want 3000", len([]rune(updated.Content)))
	}
}

func TestEnrichPostJSONListBatchEquivalence(t *testing.T) {
	env := setupTestEnv(t)
	cookie := loginTestEnv(t, env, "testpass")
	ctx := context.Background()

	// Create a remote post that will be a reply parent.
	// リプライ親になるリモート投稿を作成。
	parentURI := "https://remote.example/notes/parent-1"
	now := time.Now().Truncate(time.Second).UTC()
	parentPost := &murlog.Post{
		ID: id.New(), PersonaID: env.persona.ID, Content: "<p>parent</p>",
		Origin: "remote", URI: parentURI, ActorURI: "https://remote.example/users/bob",
		Visibility: murlog.VisibilityPublic,
		CreatedAt:  now, UpdatedAt: now,
	}
	env.store.CreatePost(ctx, parentPost)

	// Create a local post with an attachment. / 添付付きローカル投稿を作成。
	var postWithAtt postJSON
	rpcCallWithCookie(t, env, "posts.create", map[string]any{
		"content": "<p>post with attachment</p>",
	}, &postWithAtt, cookie)

	// Create a local reply to the remote post. / リモート投稿へのリプライを作成。
	replyPost := &murlog.Post{
		ID: id.New(), PersonaID: env.persona.ID, Content: "<p>reply</p>",
		Origin: "local", InReplyToURI: parentURI,
		Visibility: murlog.VisibilityPublic,
		CreatedAt:  now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second),
	}
	env.store.CreatePost(ctx, replyPost)

	// Create attachment on a post directly in DB. / DB 上で直接添付を作成。
	postID, _ := id.Parse(postWithAtt.ID)
	att := &murlog.Attachment{
		ID: id.New(), PostID: postID, FilePath: "test.jpg", MimeType: "image/jpeg",
		CreatedAt: now,
	}
	env.store.CreateAttachment(ctx, att)

	// Fetch via posts.list (uses enrichPostJSONList batch path).
	// posts.list 経由で取得 (enrichPostJSONList バッチパスを使用)。
	var posts []postJSON
	rpcCallWithCookie(t, env, "posts.list", map[string]any{}, &posts, cookie)

	// Find the post with attachment. / 添付付き投稿を検索。
	var foundAtt, foundReply bool
	for _, p := range posts {
		if p.ID == postWithAtt.ID {
			if len(p.Attachments) != 1 {
				t.Errorf("batch: post %s attachments = %d, want 1", p.ID, len(p.Attachments))
			}
			foundAtt = true
		}
		if p.ID == replyPost.ID.String() {
			if p.InReplyToPost == nil {
				t.Error("batch: reply post should have InReplyToPost embedded")
			} else if p.InReplyToPost.Content != "<p>parent</p>" {
				t.Errorf("batch: InReplyToPost.Content = %q, want '<p>parent</p>'", p.InReplyToPost.Content)
			}
			foundReply = true
		}
	}
	if !foundAtt {
		t.Error("batch: post with attachment not found in posts.list")
	}
	if !foundReply {
		t.Error("batch: reply post not found in posts.list")
	}
}

func TestValidateCursorHost(t *testing.T) {
	tests := []struct {
		name     string
		cursor   string
		domain   string
		wantErr  bool
	}{
		{"valid https", "https://example.com/users/alice/outbox?page=1", "example.com", false},
		{"valid http", "http://example.com/outbox?page=2", "example.com", false},
		{"host mismatch", "https://evil.com/outbox?page=1", "example.com", true},
		{"javascript scheme", "javascript:alert(1)", "example.com", true},
		{"empty scheme", "//example.com/outbox", "example.com", true},
		{"case insensitive", "https://Example.COM/outbox", "example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCursorHost(tt.cursor, tt.domain)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCursorHost(%q, %q) error = %v, wantErr %v", tt.cursor, tt.domain, err, tt.wantErr)
			}
		})
	}
}
