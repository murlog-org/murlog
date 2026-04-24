// Package store defines the data access interface and driver registry for murlog.
// murlog のデータアクセスインターフェースとドライバレジストリを定義するパッケージ。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
)

// Store defines the data access interface for murlog.
// murlog のデータアクセスインターフェース。
type Store interface {
	// Persona / ペルソナ
	GetPersona(ctx context.Context, pid id.ID) (*murlog.Persona, error)
	GetPersonaByUsername(ctx context.Context, username string) (*murlog.Persona, error)
	ListPersonas(ctx context.Context) ([]*murlog.Persona, error)
	CreatePersona(ctx context.Context, p *murlog.Persona) error
	UpdatePersona(ctx context.Context, p *murlog.Persona) error

	// Post / 投稿
	GetPost(ctx context.Context, pid id.ID) (*murlog.Post, error)
	ListPostsByPersona(ctx context.Context, personaID id.ID, cursor id.ID, limit int) ([]*murlog.Post, error)
	ListPublicLocalPosts(ctx context.Context, personaID id.ID, cursor id.ID, limit int) ([]*murlog.Post, error)
	CreatePost(ctx context.Context, p *murlog.Post) error
	UpdatePost(ctx context.Context, p *murlog.Post) error
	DeletePost(ctx context.Context, pid id.ID) error
	DeleteReblogPost(ctx context.Context, personaID id.ID, reblogOfPostID id.ID) error
	GetPostByURI(ctx context.Context, uri string) (*murlog.Post, error)
	GetPostsByURIs(ctx context.Context, uris []string) (map[string]*murlog.Post, error)
	ListReplies(ctx context.Context, inReplyToURI string, cursor id.ID, limit int) ([]*murlog.Post, error)
	ListPostsByHashtag(ctx context.Context, tag string, cursor id.ID, limit int, localOnly bool) ([]*murlog.Post, error)

	// Follow (local -> remote) / フォロー (ローカル → リモート)
	GetFollow(ctx context.Context, fid id.ID) (*murlog.Follow, error)
	GetFollowByTarget(ctx context.Context, personaID id.ID, targetURI string) (*murlog.Follow, error)
	ListFollows(ctx context.Context, personaID id.ID) ([]*murlog.Follow, error)
	CreateFollow(ctx context.Context, f *murlog.Follow) error
	UpdateFollow(ctx context.Context, f *murlog.Follow) error
	DeleteFollow(ctx context.Context, fid id.ID) error
	DeleteFollowsByTargetDomain(ctx context.Context, domain string) error

	// Follower (remote -> local) / フォロワー (リモート → ローカル)
	GetFollower(ctx context.Context, fid id.ID) (*murlog.Follower, error)
	ListFollowers(ctx context.Context, personaID id.ID) ([]*murlog.Follower, error)
	ListFollowersPaged(ctx context.Context, personaID id.ID, cursor id.ID, limit int) ([]*murlog.Follower, error)
	ListPendingFollowers(ctx context.Context, personaID id.ID) ([]*murlog.Follower, error)
	CreateFollower(ctx context.Context, f *murlog.Follower) error
	ApproveFollower(ctx context.Context, fid id.ID) error
	DeleteFollower(ctx context.Context, fid id.ID) error
	DeleteFollowerByActorURI(ctx context.Context, personaID id.ID, actorURI string) error
	DeleteFollowersByActorDomain(ctx context.Context, domain string) error

	// RemoteActor / リモート Actor キャッシュ
	GetRemoteActor(ctx context.Context, uri string) (*murlog.RemoteActor, error)
	GetRemoteActorByAcct(ctx context.Context, acct string) (*murlog.RemoteActor, error)
	UpsertRemoteActor(ctx context.Context, a *murlog.RemoteActor) error

	// Pin / ピン留め
	PinPost(ctx context.Context, personaID id.ID, postID id.ID) error
	UnpinPost(ctx context.Context, personaID id.ID) error
	GetPinnedPost(ctx context.Context, personaID id.ID) (*murlog.Post, error)

	// Session / セッション
	GetSession(ctx context.Context, tokenHash string) (*murlog.Session, error)
	CreateSession(ctx context.Context, s *murlog.Session) error
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteExpiredSessions(ctx context.Context) error

	// OAuthApp / OAuth アプリ
	GetOAuthApp(ctx context.Context, clientID string) (*murlog.OAuthApp, error)
	CreateOAuthApp(ctx context.Context, app *murlog.OAuthApp) error

	// OAuthCode / OAuth 認可コード
	GetOAuthCode(ctx context.Context, code string) (*murlog.OAuthCode, error)
	CreateOAuthCode(ctx context.Context, c *murlog.OAuthCode) error
	DeleteOAuthCode(ctx context.Context, code string) error

	// APIToken / API トークン
	GetAPIToken(ctx context.Context, tokenHash string) (*murlog.APIToken, error)
	CreateAPIToken(ctx context.Context, t *murlog.APIToken) error
	DeleteAPIToken(ctx context.Context, tid id.ID) error
	DeleteAPITokensByApp(ctx context.Context, appID id.ID) error
	DeleteExpiredAPITokens(ctx context.Context) error

	// Attachment / 添付ファイル
	GetAttachment(ctx context.Context, aid id.ID) (*murlog.Attachment, error)
	ListAttachmentsByPost(ctx context.Context, postID id.ID) ([]*murlog.Attachment, error)
	ListAttachmentsByPosts(ctx context.Context, postIDs []id.ID) (map[id.ID][]*murlog.Attachment, error)
	CreateAttachment(ctx context.Context, a *murlog.Attachment) error
	AttachToPost(ctx context.Context, attachmentIDs []id.ID, postID id.ID) error
	DeleteAttachment(ctx context.Context, aid id.ID) error
	ListOrphanAttachments(ctx context.Context, olderThan time.Time) ([]*murlog.Attachment, error)

	// Reblog / リブログ
	ListReblogsByPost(ctx context.Context, postID id.ID) ([]*murlog.Reblog, error)
	HasReblogged(ctx context.Context, postID id.ID, actorURI string) (bool, error)
	CreateReblog(ctx context.Context, r *murlog.Reblog) error
	DeleteReblog(ctx context.Context, postID id.ID, actorURI string) error

	// Favourite / お気に入り
	ListFavouritesByPost(ctx context.Context, postID id.ID) ([]*murlog.Favourite, error)
	HasFavourited(ctx context.Context, postID id.ID, actorURI string) (bool, error)
	CreateFavourite(ctx context.Context, f *murlog.Favourite) error
	DeleteFavourite(ctx context.Context, postID id.ID, actorURI string) error

	// Notification / 通知
	ListNotifications(ctx context.Context, personaID id.ID, cursor id.ID, limit int) ([]*murlog.Notification, error)
	CountUnreadNotifications(ctx context.Context, personaID id.ID) (int, error)
	CreateNotification(ctx context.Context, n *murlog.Notification) error
	MarkNotificationRead(ctx context.Context, nid id.ID) error
	MarkAllNotificationsRead(ctx context.Context, personaID id.ID) error
	DeleteNotification(ctx context.Context, nid id.ID) error
	DeleteNotificationByActor(ctx context.Context, personaID id.ID, actorURI string, notifType string, postID id.ID) error
	CleanupNotifications(ctx context.Context, olderThan time.Time) error

	// Block / ブロック
	ListBlocks(ctx context.Context) ([]*murlog.Block, error)
	GetBlockByActorURI(ctx context.Context, actorURI string) (*murlog.Block, error)
	CreateBlock(ctx context.Context, b *murlog.Block) error
	DeleteBlock(ctx context.Context, actorURI string) error
	IsBlocked(ctx context.Context, actorURI string) (bool, error)

	// DomainBlock / ドメインブロック
	ListDomainBlocks(ctx context.Context) ([]*murlog.DomainBlock, error)
	CreateDomainBlock(ctx context.Context, db *murlog.DomainBlock) error
	DeleteDomainBlock(ctx context.Context, domain string) error
	IsDomainBlocked(ctx context.Context, domain string) (bool, error)

	// DomainFailure / ドメイン配送失敗
	ListDomainFailures(ctx context.Context) ([]*murlog.DomainFailure, error)
	IncrementDomainFailure(ctx context.Context, domain, lastError string) error
	ResetDomainFailure(ctx context.Context, domain string) error
	IsDomainDead(ctx context.Context, domain string) (bool, error)

	// LoginAttempts / ログイン試行
	GetLoginAttempt(ctx context.Context, ip string) (failCount int, lockedUntil time.Time, err error)
	RecordLoginFailure(ctx context.Context, ip string, lockedUntil time.Time) error
	ClearLoginAttempts(ctx context.Context, ip string) error

	// Setting / 設定
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error

	// Stats / 統計
	CountLocalPosts(ctx context.Context) (int, error)
	CountLocalPostsByPersona(ctx context.Context, personaID id.ID) (int, error)
	CountPersonas(ctx context.Context) (int, error)
	CountFollowers(ctx context.Context, personaID id.ID) (int, error)
	CountFollows(ctx context.Context, personaID id.ID) (int, error)
	// PostInteractionCounts returns favourites/reblogs counts for the given post IDs.
	// 指定した投稿 ID のいいね/リブログ数を返す。
	PostInteractionCounts(ctx context.Context, postIDs []id.ID) (map[id.ID]murlog.InteractionCounts, error)

	// Bulk insert (no counter refresh — call RefreshAllCounters after).
	// バルク挿入 (カウンター更新なし — 完了後に RefreshAllCounters を呼ぶ)。
	CreatePostBulk(ctx context.Context, p *murlog.Post) error
	CreateFollowerBulk(ctx context.Context, f *murlog.Follower) error

	// RefreshAllCounters recalculates all cached persona counters from actual data.
	// 全ペルソナのキャッシュカウンターを実データから再計算する。
	RefreshAllCounters(ctx context.Context)

	// Backup / バックアップ
	BackupTo(ctx context.Context, destPath string) error

	// Lifecycle / ライフサイクル
	DB() *sql.DB
	Migrate(ctx context.Context) error
	Close() error
}

// Driver is a factory function that creates a Store from a DSN.
// DSN から Store を生成するファクトリ関数。
type Driver func(dsn string) (Store, error)

var drivers = map[string]Driver{}

// Register registers a store driver by name.
// ドライバを名前で登録する。
func Register(name string, d Driver) {
	drivers[name] = d
}

// Open opens a store using the named driver and DSN.
// 指定ドライバと DSN で Store を開く。
func Open(name, dsn string) (Store, error) {
	d, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("store: unknown driver %q", name)
	}
	return d(dsn)
}
