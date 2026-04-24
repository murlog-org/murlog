//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

// newTestStore creates an in-memory SQLite store with migrations applied.
// マイグレーション済みのインメモリ SQLite Store を生成する。
func newTestStore(t *testing.T) *sqliteStore {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s.(*sqliteStore)
}

func TestPersonaCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	p := &murlog.Persona{
		ID:            id.New(),
		Username:      "alice",
		DisplayName:   "Alice",
		Summary:       "Hello, world!",
		PublicKeyPEM:  "pub",
		PrivateKeyPEM: "priv",
		Primary:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Create / 作成
	if err := s.CreatePersona(ctx, p); err != nil {
		t.Fatalf("CreatePersona: %v", err)
	}

	// Get by ID / ID で取得
	got, err := s.GetPersona(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPersona: %v", err)
	}
	if got.Username != "alice" || !got.Primary {
		t.Fatalf("GetPersona: unexpected %+v", got)
	}

	// Get by username / ユーザー名で取得
	got, err = s.GetPersonaByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("GetPersonaByUsername: %v", err)
	}
	if got.ID != p.ID {
		t.Fatalf("GetPersonaByUsername: ID mismatch")
	}

	// List / 一覧
	list, err := s.ListPersonas(ctx)
	if err != nil {
		t.Fatalf("ListPersonas: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListPersonas: want 1, got %d", len(list))
	}

	// Update / 更新
	p.DisplayName = "Alice Updated"
	p.UpdatedAt = now.Add(time.Hour)
	if err := s.UpdatePersona(ctx, p); err != nil {
		t.Fatalf("UpdatePersona: %v", err)
	}
	got, _ = s.GetPersona(ctx, p.ID)
	if got.DisplayName != "Alice Updated" {
		t.Fatalf("UpdatePersona: display_name not updated")
	}
}

func TestPostCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "bob", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	p := &murlog.Post{
		ID:         id.New(),
		PersonaID:  persona.ID,
		Content:    "<p>Hello</p>",
		ContentMap: map[string]string{"en": "Hello", "ja": "こんにちは"},
		Visibility: murlog.VisibilityPublic,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Create / 作成
	if err := s.CreatePost(ctx, p); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	// Get / 取得
	got, err := s.GetPost(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if got.Content != "<p>Hello</p>" {
		t.Fatalf("GetPost: content mismatch")
	}
	if got.ContentMap["ja"] != "こんにちは" {
		t.Fatalf("GetPost: content_map mismatch")
	}

	// List / 一覧
	list, err := s.ListPostsByPersona(ctx, persona.ID, id.Nil, 10)
	if err != nil {
		t.Fatalf("ListPostsByPersona: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListPostsByPersona: want 1, got %d", len(list))
	}

	// Update / 更新
	p.Content = "<p>Updated</p>"
	p.UpdatedAt = now.Add(time.Hour)
	if err := s.UpdatePost(ctx, p); err != nil {
		t.Fatalf("UpdatePost: %v", err)
	}

	// Delete / 削除
	if err := s.DeletePost(ctx, p.ID); err != nil {
		t.Fatalf("DeletePost: %v", err)
	}
	_, err = s.GetPost(ctx, p.ID)
	if err != sql.ErrNoRows {
		t.Fatalf("DeletePost: expected ErrNoRows, got %v", err)
	}
}

func TestFollowCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "carol", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	f := &murlog.Follow{
		ID: id.New(), PersonaID: persona.ID,
		TargetURI: "https://remote.example/users/dave",
		CreatedAt: now,
	}

	// Create / 作成
	if err := s.CreateFollow(ctx, f); err != nil {
		t.Fatalf("CreateFollow: %v", err)
	}

	// Get / 取得
	got, err := s.GetFollow(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFollow: %v", err)
	}
	if got.Accepted {
		t.Fatal("GetFollow: should not be accepted yet")
	}

	// Update (accept) / 更新 (承認)
	f.Accepted = true
	if err := s.UpdateFollow(ctx, f); err != nil {
		t.Fatalf("UpdateFollow: %v", err)
	}
	got, _ = s.GetFollow(ctx, f.ID)
	if !got.Accepted {
		t.Fatal("UpdateFollow: should be accepted")
	}

	// GetFollowByTarget / ターゲット URI で取得
	got2, err := s.GetFollowByTarget(ctx, persona.ID, "https://remote.example/users/dave")
	if err != nil {
		t.Fatalf("GetFollowByTarget: %v", err)
	}
	if got2.ID != f.ID {
		t.Fatal("GetFollowByTarget: ID mismatch")
	}

	// GetFollowByTarget (not found) / 存在しない
	_, err = s.GetFollowByTarget(ctx, persona.ID, "https://nonexistent.example/users/nobody")
	if err != sql.ErrNoRows {
		t.Fatalf("GetFollowByTarget (not found): expected ErrNoRows, got %v", err)
	}

	// List / 一覧
	list, err := s.ListFollows(ctx, persona.ID)
	if err != nil {
		t.Fatalf("ListFollows: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListFollows: want 1, got %d", len(list))
	}

	// Delete / 削除
	if err := s.DeleteFollow(ctx, f.ID); err != nil {
		t.Fatalf("DeleteFollow: %v", err)
	}
}

func TestFollowerCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "eve", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	f := &murlog.Follower{
		ID: id.New(), PersonaID: persona.ID,
		ActorURI:  "https://remote.example/users/frank",
		CreatedAt: now,
		Approved:  true,
	}

	if err := s.CreateFollower(ctx, f); err != nil {
		t.Fatalf("CreateFollower: %v", err)
	}

	list, err := s.ListFollowers(ctx, persona.ID)
	if err != nil {
		t.Fatalf("ListFollowers: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListFollowers: want 1, got %d", len(list))
	}

	if err := s.DeleteFollower(ctx, f.ID); err != nil {
		t.Fatalf("DeleteFollower: %v", err)
	}

	// DeleteFollowerByActorURI / Actor URI で削除
	f2 := &murlog.Follower{
		ID: id.New(), PersonaID: persona.ID,
		ActorURI: "https://remote.example/users/grace",
		Approved: true,
		CreatedAt: now,
	}
	s.CreateFollower(ctx, f2)

	if err := s.DeleteFollowerByActorURI(ctx, persona.ID, "https://remote.example/users/grace"); err != nil {
		t.Fatalf("DeleteFollowerByActorURI: %v", err)
	}
	list, _ = s.ListFollowers(ctx, persona.ID)
	if len(list) != 0 {
		t.Fatalf("DeleteFollowerByActorURI: want 0 followers, got %d", len(list))
	}

	// DeleteFollowerByActorURI (nonexistent — should not error)
	// 存在しない Actor URI — エラーにならないこと
	if err := s.DeleteFollowerByActorURI(ctx, persona.ID, "https://nonexistent.example/users/nobody"); err != nil {
		t.Fatalf("DeleteFollowerByActorURI (nonexistent): %v", err)
	}
}

func TestListFollowersPaged(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "paged_user", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	// Create 5 followers / フォロワー5件を作成
	for i := 0; i < 5; i++ {
		f := &murlog.Follower{
			ID: id.New(), PersonaID: persona.ID,
			ActorURI:  fmt.Sprintf("https://remote.example/users/follower%d", i),
			CreatedAt: now,
		Approved:  true,
		}
		s.CreateFollower(ctx, f)
	}

	// Page 1: first 3 / 1ページ目: 先頭3件
	page1, err := s.ListFollowersPaged(ctx, persona.ID, id.Nil, 3)
	if err != nil {
		t.Fatalf("ListFollowersPaged page1: %v", err)
	}
	if len(page1) != 3 {
		t.Fatalf("page1: want 3, got %d", len(page1))
	}

	// Page 2: next 3 (should get 2) / 2ページ目: 残り2件
	page2, err := s.ListFollowersPaged(ctx, persona.ID, page1[2].ID, 3)
	if err != nil {
		t.Fatalf("ListFollowersPaged page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2: want 2, got %d", len(page2))
	}

	// Page 3: empty / 3ページ目: 空
	page3, err := s.ListFollowersPaged(ctx, persona.ID, page2[1].ID, 3)
	if err != nil {
		t.Fatalf("ListFollowersPaged page3: %v", err)
	}
	if len(page3) != 0 {
		t.Fatalf("page3: want 0, got %d", len(page3))
	}

	// Verify no duplicates / 重複がないことを確認
	seen := map[string]bool{}
	for _, f := range append(page1, page2...) {
		if seen[f.ActorURI] {
			t.Fatalf("duplicate follower: %s", f.ActorURI)
		}
		seen[f.ActorURI] = true
	}
}

func TestRemoteActorUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	a := &murlog.RemoteActor{
		URI:         "https://remote.example/users/grace",
		Username:    "grace",
		DisplayName: "Grace",
		Inbox:       "https://remote.example/users/grace/inbox",
		FetchedAt:   now,
	}

	// Insert / 挿入
	if err := s.UpsertRemoteActor(ctx, a); err != nil {
		t.Fatalf("UpsertRemoteActor (insert): %v", err)
	}

	got, err := s.GetRemoteActor(ctx, a.URI)
	if err != nil {
		t.Fatalf("GetRemoteActor: %v", err)
	}
	if got.DisplayName != "Grace" {
		t.Fatalf("GetRemoteActor: display_name mismatch")
	}

	// Update / 更新
	a.DisplayName = "Grace Updated"
	a.FetchedAt = now.Add(time.Hour)
	if err := s.UpsertRemoteActor(ctx, a); err != nil {
		t.Fatalf("UpsertRemoteActor (update): %v", err)
	}
	got, _ = s.GetRemoteActor(ctx, a.URI)
	if got.DisplayName != "Grace Updated" {
		t.Fatalf("UpsertRemoteActor: display_name not updated")
	}
}

func TestSessionCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	sess := &murlog.Session{
		ID:        id.New(),
		TokenHash: "sha256:abc123",
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
	}

	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := s.GetSession(ctx, sess.TokenHash)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.ID != sess.ID {
		t.Fatal("GetSession: ID mismatch")
	}

	if err := s.DeleteSession(ctx, sess.TokenHash); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	_, err = s.GetSession(ctx, sess.TokenHash)
	if err != sql.ErrNoRows {
		t.Fatalf("DeleteSession: expected ErrNoRows, got %v", err)
	}
}

func TestSettingKV(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Get non-existent key returns empty string.
	// 存在しないキーは空文字列を返す。
	val, err := s.GetSetting(ctx, "theme")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if val != "" {
		t.Fatalf("GetSetting: want empty, got %q", val)
	}

	// Set / 設定
	if err := s.SetSetting(ctx, "theme", "dark"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	val, _ = s.GetSetting(ctx, "theme")
	if val != "dark" {
		t.Fatalf("GetSetting: want dark, got %q", val)
	}

	// Upsert / 上書き
	if err := s.SetSetting(ctx, "theme", "light"); err != nil {
		t.Fatalf("SetSetting (upsert): %v", err)
	}
	val, _ = s.GetSetting(ctx, "theme")
	if val != "light" {
		t.Fatalf("GetSetting: want light, got %q", val)
	}
}

func TestAttachmentCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "att_user", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	post := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "<p>test</p>",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePost(ctx, post)

	// Create orphan attachment / 孤立添付ファイルを作成
	att := &murlog.Attachment{
		ID: id.New(), FilePath: "attachments/test.jpg", MimeType: "image/jpeg",
		Alt: "a photo", Width: 800, Height: 600, Size: 12345, CreatedAt: now,
	}
	if err := s.CreateAttachment(ctx, att); err != nil {
		t.Fatalf("CreateAttachment: %v", err)
	}

	// Get / 取得
	got, err := s.GetAttachment(ctx, att.ID)
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}
	if got.Alt != "a photo" || got.Width != 800 {
		t.Fatalf("GetAttachment: unexpected %+v", got)
	}

	// List orphans / 孤立添付一覧
	orphans, err := s.ListOrphanAttachments(ctx, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListOrphanAttachments: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("ListOrphanAttachments: want 1, got %d", len(orphans))
	}

	// Attach to post / 投稿に紐づけ
	if err := s.AttachToPost(ctx, []id.ID{att.ID}, post.ID); err != nil {
		t.Fatalf("AttachToPost: %v", err)
	}

	// List by post / 投稿別一覧
	atts, err := s.ListAttachmentsByPost(ctx, post.ID)
	if err != nil {
		t.Fatalf("ListAttachmentsByPost: %v", err)
	}
	if len(atts) != 1 {
		t.Fatalf("ListAttachmentsByPost: want 1, got %d", len(atts))
	}

	// No longer orphan / 孤立ではなくなった
	orphans, _ = s.ListOrphanAttachments(ctx, now.Add(time.Hour))
	if len(orphans) != 0 {
		t.Fatalf("ListOrphanAttachments: want 0, got %d", len(orphans))
	}

	// Delete / 削除
	if err := s.DeleteAttachment(ctx, att.ID); err != nil {
		t.Fatalf("DeleteAttachment: %v", err)
	}
	_, err = s.GetAttachment(ctx, att.ID)
	if err != sql.ErrNoRows {
		t.Fatalf("DeleteAttachment: expected ErrNoRows, got %v", err)
	}
}

func TestNotificationCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "notif_user", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	n := &murlog.Notification{
		ID: id.New(), PersonaID: persona.ID, Type: "follow",
		ActorURI: "https://remote.example/users/bob", CreatedAt: now,
	}
	if err := s.CreateNotification(ctx, n); err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}

	// List / 一覧
	list, err := s.ListNotifications(ctx, persona.ID, id.Nil, 10)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(list) != 1 || list[0].Type != "follow" {
		t.Fatalf("ListNotifications: unexpected %+v", list)
	}
	if list[0].Read {
		t.Fatal("notification should be unread")
	}

	// Mark read / 既読にする
	if err := s.MarkNotificationRead(ctx, n.ID); err != nil {
		t.Fatalf("MarkNotificationRead: %v", err)
	}
	list, _ = s.ListNotifications(ctx, persona.ID, id.Nil, 10)
	if !list[0].Read {
		t.Fatal("notification should be read")
	}

	// Mark all read / 全既読
	postID := id.New()
	n2 := &murlog.Notification{
		ID: id.New(), PersonaID: persona.ID, Type: "favourite",
		ActorURI: "https://remote.example/users/carol", PostID: postID, CreatedAt: now.Add(time.Second),
	}
	s.CreateNotification(ctx, n2)
	s.MarkAllNotificationsRead(ctx, persona.ID)
	list, _ = s.ListNotifications(ctx, persona.ID, id.Nil, 10)
	for _, nn := range list {
		if !nn.Read {
			t.Fatalf("notification %s should be read", nn.ID)
		}
	}

	// Count unread — all read so expect 0. / 未読カウント — 全部既読なので 0。
	count, err := s.CountUnreadNotifications(ctx, persona.ID)
	if err != nil {
		t.Fatalf("CountUnreadNotifications: %v", err)
	}
	if count != 0 {
		t.Fatalf("CountUnreadNotifications: got %d, want 0", count)
	}

	// Delete notification / 通知削除
	if err := s.DeleteNotification(ctx, n.ID); err != nil {
		t.Fatalf("DeleteNotification: %v", err)
	}
	list, _ = s.ListNotifications(ctx, persona.ID, id.Nil, 10)
	if len(list) != 1 {
		t.Fatalf("after DeleteNotification: got %d, want 1", len(list))
	}

	// DeleteNotificationByActor / Actor 指定削除
	if err := s.DeleteNotificationByActor(ctx, persona.ID, "https://remote.example/users/carol", "favourite", postID); err != nil {
		t.Fatalf("DeleteNotificationByActor: %v", err)
	}
	list, _ = s.ListNotifications(ctx, persona.ID, id.Nil, 10)
	if len(list) != 0 {
		t.Fatalf("after DeleteNotificationByActor: got %d, want 0", len(list))
	}

	// CleanupNotifications — old read notifications should be deleted.
	// 古い既読通知はクリーンアップで削除される。
	oldNotif := &murlog.Notification{
		ID: id.New(), PersonaID: persona.ID, Type: "follow",
		ActorURI: "https://remote.example/users/old", CreatedAt: now.Add(-100 * 24 * time.Hour),
	}
	s.CreateNotification(ctx, oldNotif)
	s.MarkNotificationRead(ctx, oldNotif.ID)
	newNotif := &murlog.Notification{
		ID: id.New(), PersonaID: persona.ID, Type: "follow",
		ActorURI: "https://remote.example/users/new", CreatedAt: now,
	}
	s.CreateNotification(ctx, newNotif)

	if err := s.CleanupNotifications(ctx, now.Add(-90*24*time.Hour)); err != nil {
		t.Fatalf("CleanupNotifications: %v", err)
	}
	list, _ = s.ListNotifications(ctx, persona.ID, id.Nil, 10)
	if len(list) != 1 {
		t.Fatalf("after CleanupNotifications: got %d, want 1", len(list))
	}
	if list[0].ActorURI != "https://remote.example/users/new" {
		t.Errorf("remaining notification actor = %s, want new", list[0].ActorURI)
	}
}

func TestOAuthCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	app := &murlog.OAuthApp{
		ID: id.New(), ClientID: "client123", ClientSecret: "secret456",
		Name: "TestApp", RedirectURI: "https://example.com/callback",
		Scopes: "read write", CreatedAt: now,
	}
	if err := s.CreateOAuthApp(ctx, app); err != nil {
		t.Fatalf("CreateOAuthApp: %v", err)
	}

	got, err := s.GetOAuthApp(ctx, "client123")
	if err != nil {
		t.Fatalf("GetOAuthApp: %v", err)
	}
	if got.Name != "TestApp" {
		t.Fatalf("GetOAuthApp: name mismatch")
	}

	// OAuth code / 認可コード
	code := &murlog.OAuthCode{
		ID: id.New(), AppID: app.ID, Code: "authcode789",
		RedirectURI: "https://example.com/callback", Scopes: "read",
		CodeChallenge: "S256challenge", ExpiresAt: now.Add(10 * time.Minute), CreatedAt: now,
	}
	if err := s.CreateOAuthCode(ctx, code); err != nil {
		t.Fatalf("CreateOAuthCode: %v", err)
	}

	gotCode, err := s.GetOAuthCode(ctx, "authcode789")
	if err != nil {
		t.Fatalf("GetOAuthCode: %v", err)
	}
	if gotCode.CodeChallenge != "S256challenge" {
		t.Fatalf("GetOAuthCode: code_challenge mismatch")
	}

	if err := s.DeleteOAuthCode(ctx, "authcode789"); err != nil {
		t.Fatalf("DeleteOAuthCode: %v", err)
	}
	_, err = s.GetOAuthCode(ctx, "authcode789")
	if err != sql.ErrNoRows {
		t.Fatalf("DeleteOAuthCode: expected ErrNoRows, got %v", err)
	}

	// API token via app / アプリ経由の API トークン
	token := &murlog.APIToken{
		ID: id.New(), Name: "test-token", TokenHash: "sha256:tokenhash",
		AppID: app.ID, Scopes: "read", CreatedAt: now,
	}
	if err := s.CreateAPIToken(ctx, token); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}

	// Delete tokens by app / アプリ別トークン削除
	if err := s.DeleteAPITokensByApp(ctx, app.ID); err != nil {
		t.Fatalf("DeleteAPITokensByApp: %v", err)
	}
	_, err = s.GetAPIToken(ctx, "sha256:tokenhash")
	if err != sql.ErrNoRows {
		t.Fatalf("DeleteAPITokensByApp: expected ErrNoRows, got %v", err)
	}
}

func TestFavouriteAndReblog(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "fav_user", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	post := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "<p>likeable</p>",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePost(ctx, post)

	// Favourite / お気に入り
	fav := &murlog.Favourite{
		ID: id.New(), PostID: post.ID,
		ActorURI: "https://remote.example/users/liker", CreatedAt: now,
	}
	if err := s.CreateFavourite(ctx, fav); err != nil {
		t.Fatalf("CreateFavourite: %v", err)
	}

	favs, err := s.ListFavouritesByPost(ctx, post.ID)
	if err != nil {
		t.Fatalf("ListFavouritesByPost: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("ListFavouritesByPost: want 1, got %d", len(favs))
	}

	if err := s.DeleteFavourite(ctx, post.ID, fav.ActorURI); err != nil {
		t.Fatalf("DeleteFavourite: %v", err)
	}
	favs, _ = s.ListFavouritesByPost(ctx, post.ID)
	if len(favs) != 0 {
		t.Fatalf("DeleteFavourite: want 0, got %d", len(favs))
	}

	// Reblog / リブログ
	reblog := &murlog.Reblog{
		ID: id.New(), PostID: post.ID,
		ActorURI: "https://remote.example/users/reblogger", CreatedAt: now,
	}
	if err := s.CreateReblog(ctx, reblog); err != nil {
		t.Fatalf("CreateReblog: %v", err)
	}

	reblogs, err := s.ListReblogsByPost(ctx, post.ID)
	if err != nil {
		t.Fatalf("ListReblogsByPost: %v", err)
	}
	if len(reblogs) != 1 {
		t.Fatalf("ListReblogsByPost: want 1, got %d", len(reblogs))
	}

	if err := s.DeleteReblog(ctx, post.ID, reblog.ActorURI); err != nil {
		t.Fatalf("DeleteReblog: %v", err)
	}
	reblogs, _ = s.ListReblogsByPost(ctx, post.ID)
	if len(reblogs) != 0 {
		t.Fatalf("DeleteReblog: want 0, got %d", len(reblogs))
	}
}

func TestListPublicLocalPosts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "pub_user", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	// Create local public post / ローカル公開投稿を作成
	p1 := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "<p>public</p>",
		Visibility: murlog.VisibilityPublic, Origin: "local",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePost(ctx, p1)

	// Create remote post (should not appear) / リモート投稿 (表示されないはず)
	p2 := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "<p>remote</p>",
		Visibility: murlog.VisibilityPublic, Origin: "remote",
		URI: "https://remote.example/notes/1", ActorURI: "https://remote.example/users/x",
		CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
	}
	s.CreatePost(ctx, p2)

	list, err := s.ListPublicLocalPosts(ctx, persona.ID, id.Nil, 10)
	if err != nil {
		t.Fatalf("ListPublicLocalPosts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListPublicLocalPosts: want 1, got %d", len(list))
	}
	if list[0].ID != p1.ID {
		t.Fatal("ListPublicLocalPosts: wrong post returned")
	}
}

func TestGetPostByURI(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "uri_user", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	post := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "<p>remote note</p>",
		Origin: "remote", URI: "https://remote.example/notes/42",
		ActorURI: "https://remote.example/users/bob",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePost(ctx, post)

	got, err := s.GetPostByURI(ctx, "https://remote.example/notes/42")
	if err != nil {
		t.Fatalf("GetPostByURI: %v", err)
	}
	if got.ID != post.ID {
		t.Fatal("GetPostByURI: ID mismatch")
	}

	_, err = s.GetPostByURI(ctx, "https://nonexistent.example/notes/999")
	if err != sql.ErrNoRows {
		t.Fatalf("GetPostByURI (not found): expected ErrNoRows, got %v", err)
	}
}

func TestPostRebloggedByURI(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "reblog_user", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	post := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "<p>reblogged note</p>",
		Origin: "remote", URI: "https://other.example/notes/99",
		ActorURI:       "https://other.example/users/carol",
		RebloggedByURI: "https://remote.example/users/bob",
		CreatedAt:      now, UpdatedAt: now,
	}
	if err := s.CreatePost(ctx, post); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	got, err := s.GetPost(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if got.RebloggedByURI != "https://remote.example/users/bob" {
		t.Errorf("RebloggedByURI = %q, want bob", got.RebloggedByURI)
	}
	if got.ActorURI != "https://other.example/users/carol" {
		t.Errorf("ActorURI = %q, want carol", got.ActorURI)
	}
}

func TestListPostsByHashtagLocalOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "hashtag_user", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	// Create a local post with hashtag. / ハッシュタグ付きローカル投稿。
	local := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "local #test",
		Origin: "local", HashtagsJSON: `["test"]`,
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePost(ctx, local)

	// Create a remote post with same hashtag. / 同じハッシュタグのリモート投稿。
	remote := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "remote #test",
		Origin: "remote", URI: "https://remote.example/notes/1", ActorURI: "https://remote.example/users/bob",
		HashtagsJSON: `["test"]`,
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePost(ctx, remote)

	// localOnly=false returns both. / localOnly=false は両方返す。
	all, _ := s.ListPostsByHashtag(ctx, "test", id.ID{}, 50, false)
	if len(all) != 2 {
		t.Fatalf("localOnly=false: got %d, want 2", len(all))
	}

	// localOnly=true returns only local. / localOnly=true はローカルのみ。
	localOnly, _ := s.ListPostsByHashtag(ctx, "test", id.ID{}, 50, true)
	if len(localOnly) != 1 {
		t.Fatalf("localOnly=true: got %d, want 1", len(localOnly))
	}
	if localOnly[0].Origin != "local" {
		t.Errorf("expected local post, got origin=%q", localOnly[0].Origin)
	}
}

func TestDeleteReblogPost(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "reblog_wrapper", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	// Create an original post. / 元投稿を作成。
	original := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "<p>original</p>",
		Origin: "local", CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePost(ctx, original)

	// Create a reblog wrapper post. / リブログ wrapper post を作成。
	wrapper := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID,
		Origin: "local", Visibility: murlog.VisibilityPublic,
		ReblogOfPostID: original.ID,
		CreatedAt:      now, UpdatedAt: now,
	}
	if err := s.CreatePost(ctx, wrapper); err != nil {
		t.Fatalf("CreatePost wrapper: %v", err)
	}

	// Verify wrapper exists. / wrapper が存在することを確認。
	got, err := s.GetPost(ctx, wrapper.ID)
	if err != nil {
		t.Fatalf("GetPost wrapper: %v", err)
	}
	if got.ReblogOfPostID != original.ID {
		t.Errorf("ReblogOfPostID = %v, want %v", got.ReblogOfPostID, original.ID)
	}

	// Verify wrapper appears in public local posts. / 公開ローカル投稿に含まれる。
	publicPosts, _ := s.ListPublicLocalPosts(ctx, persona.ID, id.ID{}, 50)
	found := false
	for _, p := range publicPosts {
		if p.ID == wrapper.ID {
			found = true
		}
	}
	if !found {
		t.Error("wrapper not found in ListPublicLocalPosts")
	}

	// Delete and verify. / 削除して確認。
	if err := s.DeleteReblogPost(ctx, persona.ID, original.ID); err != nil {
		t.Fatalf("DeleteReblogPost: %v", err)
	}
	if _, err := s.GetPost(ctx, wrapper.ID); err == nil {
		t.Error("wrapper still exists after DeleteReblogPost")
	}
}

func TestHasFavouritedAndHasReblogged(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "has_check", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	post := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "<p>test</p>",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePost(ctx, post)

	actor := "https://remote.example/users/checker"

	// Before create → false / 作成前 → false
	if got, _ := s.HasFavourited(ctx, post.ID, actor); got {
		t.Fatal("HasFavourited: want false before create")
	}
	if got, _ := s.HasReblogged(ctx, post.ID, actor); got {
		t.Fatal("HasReblogged: want false before create")
	}

	// Create favourite and reblog / お気に入りとリブログを作成
	s.CreateFavourite(ctx, &murlog.Favourite{
		ID: id.New(), PostID: post.ID, ActorURI: actor, CreatedAt: now,
	})
	s.CreateReblog(ctx, &murlog.Reblog{
		ID: id.New(), PostID: post.ID, ActorURI: actor, CreatedAt: now,
	})

	// After create → true / 作成後 → true
	if got, _ := s.HasFavourited(ctx, post.ID, actor); !got {
		t.Fatal("HasFavourited: want true after create")
	}
	if got, _ := s.HasReblogged(ctx, post.ID, actor); !got {
		t.Fatal("HasReblogged: want true after create")
	}

	// After delete → false / 削除後 → false
	s.DeleteFavourite(ctx, post.ID, actor)
	s.DeleteReblog(ctx, post.ID, actor)
	if got, _ := s.HasFavourited(ctx, post.ID, actor); got {
		t.Fatal("HasFavourited: want false after delete")
	}
	if got, _ := s.HasReblogged(ctx, post.ID, actor); got {
		t.Fatal("HasReblogged: want false after delete")
	}
}

func TestListAttachmentsByPosts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "att_batch", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	// Empty input → nil / 空入力 → nil
	result, err := s.ListAttachmentsByPosts(ctx, nil)
	if err != nil {
		t.Fatalf("ListAttachmentsByPosts(nil): %v", err)
	}
	if result != nil {
		t.Fatalf("ListAttachmentsByPosts(nil): want nil, got %v", result)
	}

	// Create 2 posts with attachments / 添付付き投稿を2件作成
	p1 := &murlog.Post{ID: id.New(), PersonaID: persona.ID, Content: "<p>1</p>", CreatedAt: now, UpdatedAt: now}
	p2 := &murlog.Post{ID: id.New(), PersonaID: persona.ID, Content: "<p>2</p>", CreatedAt: now, UpdatedAt: now}
	p3 := &murlog.Post{ID: id.New(), PersonaID: persona.ID, Content: "<p>3</p>", CreatedAt: now, UpdatedAt: now} // no attachment
	s.CreatePost(ctx, p1)
	s.CreatePost(ctx, p2)
	s.CreatePost(ctx, p3)

	a1 := &murlog.Attachment{ID: id.New(), PostID: p1.ID, FilePath: "a1.jpg", MimeType: "image/jpeg", CreatedAt: now}
	a2 := &murlog.Attachment{ID: id.New(), PostID: p2.ID, FilePath: "a2.png", MimeType: "image/png", CreatedAt: now}
	a3 := &murlog.Attachment{ID: id.New(), PostID: p2.ID, FilePath: "a3.png", MimeType: "image/png", CreatedAt: now}
	s.CreateAttachment(ctx, a1)
	s.CreateAttachment(ctx, a2)
	s.CreateAttachment(ctx, a3)

	result, err = s.ListAttachmentsByPosts(ctx, []id.ID{p1.ID, p2.ID, p3.ID})
	if err != nil {
		t.Fatalf("ListAttachmentsByPosts: %v", err)
	}
	if len(result[p1.ID]) != 1 {
		t.Fatalf("p1 attachments: want 1, got %d", len(result[p1.ID]))
	}
	if len(result[p2.ID]) != 2 {
		t.Fatalf("p2 attachments: want 2, got %d", len(result[p2.ID]))
	}
	if _, ok := result[p3.ID]; ok {
		t.Fatal("p3 should have no attachments in map")
	}
}

func TestGetPostsByURIs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "uri_batch", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	// Empty input → nil / 空入力 → nil
	result, err := s.GetPostsByURIs(ctx, nil)
	if err != nil {
		t.Fatalf("GetPostsByURIs(nil): %v", err)
	}
	if result != nil {
		t.Fatalf("GetPostsByURIs(nil): want nil, got %v", result)
	}

	uri1 := "https://remote.example/notes/100"
	uri2 := "https://remote.example/notes/200"
	p1 := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "<p>r1</p>",
		Origin: "remote", URI: uri1, ActorURI: "https://remote.example/users/a",
		CreatedAt: now, UpdatedAt: now,
	}
	p2 := &murlog.Post{
		ID: id.New(), PersonaID: persona.ID, Content: "<p>r2</p>",
		Origin: "remote", URI: uri2, ActorURI: "https://remote.example/users/b",
		CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePost(ctx, p1)
	s.CreatePost(ctx, p2)

	result, err = s.GetPostsByURIs(ctx, []string{uri1, uri2, "https://nonexistent.example/notes/999"})
	if err != nil {
		t.Fatalf("GetPostsByURIs: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("GetPostsByURIs: want 2, got %d", len(result))
	}
	if result[uri1].ID != p1.ID {
		t.Fatal("GetPostsByURIs: uri1 ID mismatch")
	}
	if result[uri2].ID != p2.ID {
		t.Fatal("GetPostsByURIs: uri2 ID mismatch")
	}
	if _, ok := result["https://nonexistent.example/notes/999"]; ok {
		t.Fatal("GetPostsByURIs: nonexistent URI should not be in map")
	}
}

func TestIsValidDomain(t *testing.T) {
	tests := []struct {
		domain string
		want   bool
	}{
		{"example.com", true},
		{"test-host.example.org", true},
		{"123.456", true},
		{"UPPER.case", true},
		{"", false},
		{"ex%ample", false},
		{"ex_ample", false},
		{"foo bar", false},
		{"evil@domain", false},
		{"path/inject", false},
		{"uni\u00e9code.com", false},
	}
	for _, tt := range tests {
		if got := isValidDomain(tt.domain); got != tt.want {
			t.Errorf("isValidDomain(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestDomainFromURI(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"https://example.com/users/bob", "example.com"},
		{"https://example.com:8080/path", "example.com"},
		{"http://sub.domain.org/x", "sub.domain.org"},
		{"", ""},
		{"://broken", ""},
	}
	for _, tt := range tests {
		if got := domainFromURI(tt.uri); got != tt.want {
			t.Errorf("domainFromURI(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestScanHelperErrorAccumulation(t *testing.T) {
	// Valid bytes → correct ID / 正常バイト → 正しい ID
	validID := id.New()
	var sh scanHelper
	got := sh.scanID(validID[:])
	if sh.err != nil {
		t.Fatalf("scanID(valid): unexpected error: %v", sh.err)
	}
	if got != validID {
		t.Fatalf("scanID(valid): want %s, got %s", validID, got)
	}

	// Valid time / 正常な時刻
	sh = scanHelper{}
	ts := sh.parseTime("2025-04-19T10:30:00Z")
	if sh.err != nil {
		t.Fatalf("parseTime(valid): unexpected error: %v", sh.err)
	}
	if ts.IsZero() {
		t.Fatal("parseTime(valid): got zero time")
	}

	// NULL column → zero ID, no error / NULL カラム → ゼロ ID, エラーなし
	sh = scanHelper{}
	got = sh.scanID(nil)
	if sh.err != nil {
		t.Fatalf("scanID(nil): unexpected error: %v", sh.err)
	}
	if got != id.Nil {
		t.Fatalf("scanID(nil): want Nil, got %s", got)
	}

	// Invalid bytes → error accumulated / 不正バイト → エラー蓄積
	sh = scanHelper{}
	sh.scanID([]byte{0xFF, 0xFE}) // too short
	if sh.err == nil {
		t.Fatal("scanID(invalid): expected error")
	}

	// After error, parseTime returns zero without overwriting / エラー後は parseTime がゼロを返す
	ts = sh.parseTime("2025-04-19T10:30:00Z")
	if !ts.IsZero() {
		t.Fatal("parseTime after error: should return zero time")
	}

	// parseTime with invalid string / 不正文字列の parseTime
	sh = scanHelper{}
	sh.parseTime("not-a-date")
	if sh.err == nil {
		t.Fatal("parseTime(invalid): expected error")
	}
}

// TestConcurrentOpen simulates CGI concurrent process startup —
// multiple goroutines open the same DB file and run Migrate simultaneously.
// CGI 同時起動をシミュレート — 複数ゴルーチンが同一 DB を同時に Open+Migrate する。
func TestConcurrentOpen(t *testing.T) {
	dbPath := t.TempDir() + "/concurrent.db"
	const n = 10

	// Initialize DB first (simulates first deployment).
	// まず DB を初期化する (初回デプロイをシミュレート)。
	s0, err := New(dbPath)
	if err != nil {
		t.Fatalf("initial New: %v", err)
	}
	if err := s0.Migrate(context.Background()); err != nil {
		t.Fatalf("initial Migrate: %v", err)
	}
	s0.Close()

	// Simulate concurrent CGI processes opening the same DB.
	// 同一 DB を同時に開く CGI プロセスをシミュレート。
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			s, err := New(dbPath)
			if err != nil {
				errs <- fmt.Errorf("New: %w", err)
				return
			}
			defer s.Close()
			if err := s.Migrate(context.Background()); err != nil {
				errs <- fmt.Errorf("Migrate: %w", err)
				return
			}
			errs <- nil
		}()
	}

	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// TestMigratePreBackup verifies that Migrate creates a pre-migration
// backup when pending migrations exist on a file-based DB.
// ファイル DB で pending マイグレーションがある場合、Migrate が
// マイグレーション前バックアップを作成することを検証する。
func TestMigratePreBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "murlog.db")
	backupPath := filepath.Join(dir, "murlog-premigrate.db")

	// Apply all migrations (first run — pending exists).
	// 全マイグレーションを適用 (初回 — pending あり)。
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("initial Migrate: %v", err)
	}

	// Backup should be created. / バックアップが作成されているはず。
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup not created on initial migrate: %v", err)
	}

	// Verify backup is a valid SQLite DB. / バックアップが有効な SQLite DB であることを検証。
	sb, err := New(backupPath)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	sb.Close()

	// Remove backup and re-migrate (no pending — should NOT create backup).
	// バックアップを削除して再 migrate (pending なし — バックアップ作成されないはず)。
	os.Remove(backupPath)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("no-op Migrate: %v", err)
	}
	s.Close()

	if _, err := os.Stat(backupPath); err == nil {
		t.Error("backup should not be created when no pending migrations")
	}
}

// TestMigrateNoBackupInMemory verifies that in-memory DBs skip backup.
// インメモリ DB ではバックアップをスキップすることを検証する。
func TestMigrateNoBackupInMemory(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	// Should not panic or error — just skips backup.
	// パニックやエラーにならない — バックアップをスキップするだけ。
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
}

func TestPersonaCounterCache(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	persona := &murlog.Persona{
		ID: id.New(), Username: "counter_test", PublicKeyPEM: "pub", PrivateKeyPEM: "priv",
		Primary: true, FieldsJSON: "[]", CreatedAt: now, UpdatedAt: now,
	}
	s.CreatePersona(ctx, persona)

	// Helper to reload persona and check counters.
	// ペルソナを再取得してカウンターを検証するヘルパー。
	check := func(wantPosts, wantFollowers, wantFollowing int) {
		t.Helper()
		p, err := s.GetPersona(ctx, persona.ID)
		if err != nil {
			t.Fatalf("GetPersona: %v", err)
		}
		if p.PostCount != wantPosts {
			t.Errorf("PostCount = %d, want %d", p.PostCount, wantPosts)
		}
		if p.FollowersCount != wantFollowers {
			t.Errorf("FollowersCount = %d, want %d", p.FollowersCount, wantFollowers)
		}
		if p.FollowingCount != wantFollowing {
			t.Errorf("FollowingCount = %d, want %d", p.FollowingCount, wantFollowing)
		}
	}

	// Initially all zero. / 初期値はすべて 0。
	check(0, 0, 0)

	// Create local posts → post_count increases.
	// ローカル投稿作成 → post_count 増加。
	p1 := &murlog.Post{ID: id.New(), PersonaID: persona.ID, Content: "<p>1</p>", Origin: "local", CreatedAt: now, UpdatedAt: now}
	p2 := &murlog.Post{ID: id.New(), PersonaID: persona.ID, Content: "<p>2</p>", Origin: "local", CreatedAt: now, UpdatedAt: now}
	s.CreatePost(ctx, p1)
	s.CreatePost(ctx, p2)
	check(2, 0, 0)

	// Remote post should NOT increment post_count.
	// リモート投稿は post_count に影響しない。
	pr := &murlog.Post{ID: id.New(), PersonaID: persona.ID, Content: "<p>r</p>", Origin: "remote", CreatedAt: now, UpdatedAt: now}
	s.CreatePost(ctx, pr)
	check(2, 0, 0)

	// Delete local post → post_count decreases.
	// ローカル投稿削除 → post_count 減少。
	s.DeletePost(ctx, p1.ID)
	check(1, 0, 0)

	// Create approved follower → followers_count increases.
	// 承認済みフォロワー作成 → followers_count 増加。
	f1 := &murlog.Follower{ID: id.New(), PersonaID: persona.ID, ActorURI: "https://r.example/u/f1", Approved: true, CreatedAt: now}
	s.CreateFollower(ctx, f1)
	check(1, 1, 0)

	// Create pending follower → followers_count unchanged.
	// 未承認フォロワー作成 → followers_count 変化なし。
	f2 := &murlog.Follower{ID: id.New(), PersonaID: persona.ID, ActorURI: "https://r.example/u/f2", Approved: false, CreatedAt: now}
	s.CreateFollower(ctx, f2)
	check(1, 1, 0)

	// Approve pending → followers_count increases.
	// 承認 → followers_count 増加。
	s.ApproveFollower(ctx, f2.ID)
	check(1, 2, 0)

	// Delete follower → followers_count decreases.
	// フォロワー削除 → followers_count 減少。
	s.DeleteFollower(ctx, f1.ID)
	check(1, 1, 0)

	// Create follow → following_count increases.
	// フォロー作成 → following_count 増加。
	fl := &murlog.Follow{ID: id.New(), PersonaID: persona.ID, TargetURI: "https://r.example/u/t1", CreatedAt: now}
	s.CreateFollow(ctx, fl)
	check(1, 1, 1)

	// Delete follow → following_count decreases.
	// フォロー削除 → following_count 減少。
	s.DeleteFollow(ctx, fl.ID)
	check(1, 1, 0)
}

