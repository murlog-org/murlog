// Package murlog defines the domain types for the murlog ActivityPub server.
// murlog ActivityPub サーバーのドメイン型を定義するパッケージ。
package murlog

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/murlog-org/murlog/id"
)

// MaxJobAttempts is the maximum number of attempts before a job is marked dead.
// ジョブが dead になるまでの最大試行回数。
const MaxJobAttempts = 5

// Version is the murlog version, injected at build time via -ldflags.
// ビルド時に -ldflags で注入される murlog バージョン。
var Version = "dev"

// Commit is the git short hash, injected at build time via -ldflags.
// ビルド時に -ldflags で注入される git short hash。
var Commit = "unknown"

// VersionString returns "version (commit)" for display.
// 表示用の "version (commit)" 文字列を返す。
func VersionString() string {
	if Commit == "unknown" || Commit == "" {
		return Version
	}
	return Version + " (" + Commit + ")"
}

// Password constraints.
// パスワードの制約。
const (
	PasswordMinLen = 8
	PasswordMaxLen = 128
)

// ValidatePassword checks that a password meets minimum strength requirements.
// Requires 8-128 characters and at least 3 of 4 character types (lower, upper, digit, symbol).
// パスワードが最低限の強度要件を満たしているか検証する。
// 8〜128 文字かつ 4 種の文字種（小文字・大文字・数字・記号）のうち 3 種以上を要求。
func ValidatePassword(s string) error {
	if len(s) < PasswordMinLen {
		return fmt.Errorf("password must be at least %d characters", PasswordMinLen)
	}
	if len(s) > PasswordMaxLen {
		return fmt.Errorf("password must be %d characters or fewer", PasswordMaxLen)
	}
	var hasLower, hasUpper, hasDigit, hasSymbol bool
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= '0' && c <= '9':
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	kinds := 0
	if hasLower { kinds++ }
	if hasUpper { kinds++ }
	if hasDigit { kinds++ }
	if hasSymbol { kinds++ }
	if kinds < 3 {
		return fmt.Errorf("password must contain at least 3 of: lowercase, uppercase, digit, symbol")
	}
	return nil
}

// Username constraints.
// ユーザー名の制約。
const (
	UsernameMinLen = 1
	UsernameMaxLen = 30
)

var usernameRe = regexp.MustCompile(`^[a-z0-9_]+$`)

// ValidateUsername checks that a username is valid for use in ActivityPub URIs.
// Returns nil if valid, or an error describing the problem.
// ActivityPub URI で使用可能なユーザー名かを検証する。
func ValidateUsername(s string) error {
	if len(s) < UsernameMinLen {
		return fmt.Errorf("username is required")
	}
	if len(s) > UsernameMaxLen {
		return fmt.Errorf("username must be %d characters or fewer", UsernameMaxLen)
	}
	if !usernameRe.MatchString(s) {
		return fmt.Errorf("username must contain only lowercase letters, numbers, and underscores")
	}
	return nil
}

// Persona is a local ActivityPub Actor.
// ローカルの ActivityPub Actor。
type Persona struct {
	ID            id.ID
	Username      string // unique, used in URIs / 一意、URIに使用
	DisplayName   string
	Summary       string // bio (HTML) / 自己紹介 (HTML)
	AvatarPath    string // relative path in media store / メディアストア内の相対パス
	HeaderPath    string // relative path in media store / メディアストア内の相対パス
	PublicKeyPEM  string
	PrivateKeyPEM string
	Primary       bool   // true for the first persona / プライマリペルソナなら true
	Locked        bool   // true = manually approves followers / 手動フォロー承認
	ShowFollows   bool   // true = public follow/follower lists / フォロー・フォロワー一覧を公開
	Discoverable  bool   // true = appear in search results / 検索結果に表示される
	PinnedPostID   id.ID  // zero = no pinned post / ゼロ値 = ピン留めなし
	FieldsJSON     string // JSON array of custom fields / カスタムフィールドの JSON 配列
	PostCount      int    // cached count of local posts / ローカル投稿数キャッシュ
	FollowersCount int    // cached count of approved followers / 承認済みフォロワー数キャッシュ
	FollowingCount int    // cached count of follows / フォロー数キャッシュ
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CustomField is a key-value pair displayed on the profile (PropertyValue).
// プロフィールに表示される key-value ペア (PropertyValue)。
type CustomField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Fields returns the parsed custom fields from FieldsJSON.
// FieldsJSON からパース済みカスタムフィールドを返す。
func (p *Persona) Fields() []CustomField {
	if p.FieldsJSON == "" || p.FieldsJSON == "[]" {
		return nil
	}
	var f []CustomField
	if err := json.Unmarshal([]byte(p.FieldsJSON), &f); err != nil {
		log.Printf("murlog: Persona.Fields: json.Unmarshal: %v", err)
	}
	return f
}

// SetFields serializes custom fields to FieldsJSON.
// カスタムフィールドを FieldsJSON にシリアライズする。
func (p *Persona) SetFields(fields []CustomField) {
	if len(fields) == 0 {
		p.FieldsJSON = "[]"
		return
	}
	data, err := json.Marshal(fields)
	if err != nil {
		log.Printf("murlog: Persona.SetFields: json.Marshal: %v", err)
		return
	}
	p.FieldsJSON = string(data)
}

// Post is a note/article (local or remote).
// 投稿（ローカルまたはリモート受信）。
// ContentType represents the format of post content.
// 投稿コンテンツの形式。
const (
	ContentTypeText = "text" // plain text (local posts) / プレーンテキスト (ローカル投稿)
	ContentTypeHTML = "html" // HTML (remote posts) / HTML (リモート投稿)
)

type Post struct {
	ID          id.ID
	PersonaID   id.ID             // local: author, remote: receiving persona / ローカル:投稿者, リモート:受信先ペルソナ
	Content     string            // source content (text or HTML depending on ContentType) / ソースコンテンツ
	ContentType string            // "text" or "html" / コンテンツ形式
	ContentMap  map[string]string // lang -> content for multilingual / 言語別コンテンツ
	Visibility Visibility
	Origin     string // "local", "remote", "system"
	URI          string // ActivityPub URI (remote only) / AP URI (リモートのみ)
	ActorURI     string // remote actor URI (remote only) / リモート投稿者の Actor URI
	InReplyToURI string // AP URI of parent post (reply target) / リプライ先投稿の AP URI
	MentionsJSON string // JSON array of resolved mentions / 解決済みメンションの JSON 配列
	HashtagsJSON   string // JSON array of hashtag strings / ハッシュタグの JSON 配列
	RebloggedByURI  string // Actor URI of who reblogged this post / この投稿をリブログした Actor の URI
	ReblogOfPostID  id.ID  // original post ID for local reblog wrapper / ローカルリブログ wrapper の元投稿 ID
	Summary         string // CW text (Content Warning) / CW テキスト
	Sensitive    bool   // sensitive media flag / センシティブメディアフラグ
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Mention is a resolved mention target in a post.
// 投稿内の解決済みメンション。
type Mention struct {
	Acct string `json:"acct"` // "user@domain"
	Href string `json:"href"` // Actor URI
}

// Mentions returns the parsed mentions from MentionsJSON.
// MentionsJSON からパース済みメンションを返す。
func (p *Post) Mentions() []Mention {
	if p.MentionsJSON == "" {
		return nil
	}
	var m []Mention
	if err := json.Unmarshal([]byte(p.MentionsJSON), &m); err != nil {
		log.Printf("murlog: Post.Mentions: json.Unmarshal: %v", err)
	}
	return m
}

// SetMentions serializes mentions to MentionsJSON.
// メンションを MentionsJSON にシリアライズする。
func (p *Post) SetMentions(mentions []Mention) {
	if len(mentions) == 0 {
		p.MentionsJSON = ""
		return
	}
	data, err := json.Marshal(mentions)
	if err != nil {
		log.Printf("murlog: Post.SetMentions: json.Marshal: %v", err)
		return
	}
	p.MentionsJSON = string(data)
}

// Hashtags returns the parsed hashtags from HashtagsJSON.
// HashtagsJSON からパース済みハッシュタグを返す。
func (p *Post) Hashtags() []string {
	if p.HashtagsJSON == "" || p.HashtagsJSON == "[]" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(p.HashtagsJSON), &tags); err != nil {
		log.Printf("murlog: Post.Hashtags: json.Unmarshal: %v", err)
	}
	return tags
}

// SetHashtags serializes hashtags to HashtagsJSON.
// ハッシュタグを HashtagsJSON にシリアライズする。
func (p *Post) SetHashtags(tags []string) {
	if len(tags) == 0 {
		p.HashtagsJSON = "[]"
		return
	}
	data, err := json.Marshal(tags)
	if err != nil {
		log.Printf("murlog: Post.SetHashtags: json.Marshal: %v", err)
		return
	}
	p.HashtagsJSON = string(data)
}

// Visibility controls who can see a post.
// 投稿の公開範囲。
type Visibility int

const (
	VisibilityPublic   Visibility = iota // public timeline + federation / 公開タイムライン + 連合
	VisibilityUnlisted                   // federation but not public timeline / 連合のみ、公開タイムラインには載せない
	VisibilityFollowers                  // followers only / フォロワー限定
	VisibilityDirect                     // direct message / ダイレクトメッセージ
)

// Follow represents a local persona following a remote actor.
// ローカルペルソナがリモート Actor をフォローしている関係。
type Follow struct {
	ID        id.ID
	PersonaID id.ID  // local persona / ローカルペルソナ
	TargetURI string // remote actor URI / リモート Actor の URI
	Accepted  bool   // true after receiving Accept / Accept 受信済みなら true
	CreatedAt time.Time
}

// Follower represents a remote actor following a local persona.
// リモート Actor がローカルペルソナをフォローしている関係。
type Follower struct {
	ID        id.ID
	PersonaID id.ID  // local persona / ローカルペルソナ
	ActorURI  string // remote actor URI / リモート Actor の URI
	Approved  bool   // true after approval / 承認済みなら true
	CreatedAt time.Time
}

// RemoteActor is a cached representation of a remote ActivityPub actor.
// リモート ActivityPub Actor のキャッシュ。
type RemoteActor struct {
	URI          string // primary key (actor URI) / 主キー (Actor URI)
	Username     string
	DisplayName  string
	Summary      string
	Inbox        string
	AvatarURL    string
	HeaderURL    string
	FeaturedURL  string    // Featured (pinned) collection URL / ピン留めコレクション URL
	FieldsJSON   string    // JSON array of custom fields / カスタムフィールドの JSON 配列
	Acct         string    // "user@domain" for mention resolution / メンション解決用の acct
	FetchedAt    time.Time // cache freshness / キャッシュ鮮度
}

// Fields returns the parsed custom fields from FieldsJSON.
// FieldsJSON からパース済みカスタムフィールドを返す。
func (a *RemoteActor) Fields() []CustomField {
	if a.FieldsJSON == "" || a.FieldsJSON == "[]" {
		return nil
	}
	var f []CustomField
	if err := json.Unmarshal([]byte(a.FieldsJSON), &f); err != nil {
		log.Printf("murlog: RemoteActor.Fields: json.Unmarshal: %v", err)
	}
	return f
}

// Session is a login session for the admin UI.
// 管理画面のログインセッション。
type Session struct {
	ID        id.ID
	TokenHash string // SHA-256 hash of the session token / セッショントークンの SHA-256 ハッシュ
	ExpiresAt time.Time
	CreatedAt time.Time
}

// APIToken is a Bearer token for CLI/API access or OAuth 2.0.
// CLI/API アクセスまたは OAuth 2.0 用の Bearer トークン。
type APIToken struct {
	ID        id.ID
	Name      string    // human-readable label / 識別用ラベル
	TokenHash string    // SHA-256 hash / SHA-256 ハッシュ
	AppID     id.ID     // OAuth app ID (zero for direct issue) / OAuth アプリ ID (直接発行ならゼロ値)
	Scopes    string    // space-separated e.g. "read write" / スペース区切り
	ExpiresAt time.Time // zero value = never expires / ゼロ値 = 無期限
	CreatedAt time.Time
}

// OAuthApp is a registered OAuth 2.0 client application.
// 登録済み OAuth 2.0 クライアントアプリケーション。
type OAuthApp struct {
	ID           id.ID
	ClientID     string // randomly generated / ランダム生成
	ClientSecret string // randomly generated / ランダム生成
	Name         string // app name / アプリ名
	RedirectURI  string
	Scopes       string // space-separated e.g. "read write" / スペース区切り
	CreatedAt    time.Time
}

// OAuthCode is a temporary authorization code for the OAuth 2.0 flow.
// OAuth 2.0 フローの一時的な認可コード。
type OAuthCode struct {
	ID            id.ID
	AppID         id.ID  // oauth_apps.id
	Code          string // randomly generated / ランダム生成
	RedirectURI   string
	Scopes        string
	CodeChallenge string // PKCE S256 challenge
	ExpiresAt     time.Time
	CreatedAt     time.Time
}

// Attachment represents a media file attached to a post.
// 投稿に添付されたメディアファイル。
type Attachment struct {
	ID        id.ID
	PostID    id.ID  // zero until attached to a post / 投稿に紐づくまでゼロ値
	FilePath  string // relative path in media store, or remote URL / メディアストア内の相対パス、またはリモート URL
	MimeType  string
	Alt       string // alt text / 代替テキスト
	Width     int
	Height    int
	Size      int64
	CreatedAt time.Time
}

// Reblog represents a remote actor reblogging (Announce) a local post.
// リモート Actor がローカル投稿をリブログ (Announce) した記録。
type Reblog struct {
	ID        id.ID
	PostID    id.ID  // reblogged post / リブログされた投稿
	ActorURI  string // actor who reblogged / リブログした Actor
	CreatedAt time.Time
}

// Favourite represents a remote actor favouriting a local post.
// リモート Actor がローカル投稿をお気に入りした記録。
type Favourite struct {
	ID        id.ID
	PostID    id.ID  // favourited post / お気に入りされた投稿
	ActorURI  string // actor who favourited / お気に入りした Actor
	CreatedAt time.Time
}

// Notification is a notification for a local persona.
// ローカルペルソナへの通知。
type Notification struct {
	ID        id.ID
	PersonaID id.ID  // notification recipient / 通知の受信者
	Type      string // "follow", "mention", "reblog", "favourite"
	ActorURI  string // actor who triggered the notification / 通知を発生させた Actor
	PostID    id.ID  // related post (zero for follow) / 関連投稿 (follow の場合はゼロ値)
	Read      bool
	CreatedAt time.Time
}

// Block represents a blocked remote actor (instance-wide).
// ブロック済みリモート Actor (インスタンス全体)。
type Block struct {
	ID        id.ID
	ActorURI  string // blocked actor URI / ブロック対象の Actor URI
	CreatedAt time.Time
}

// DomainBlock represents a blocked remote domain (instance-wide).
// ブロック済みリモートドメイン (インスタンス全体)。
type DomainBlock struct {
	ID        id.ID
	Domain    string // blocked domain / ブロック対象のドメイン
	CreatedAt time.Time
}

// DomainFailure tracks delivery failures per domain for circuit-breaker logic.
// サーキットブレーカー用のドメイン別配送失敗カウンター。
type DomainFailure struct {
	Domain         string
	FailureCount   int
	LastError      string
	FirstFailureAt time.Time
	LastFailureAt  time.Time
}

// JobType identifies the kind of background job.
// バックグラウンドジョブの種別。
type JobType int

const (
	JobAcceptFollow     JobType = iota + 1 // accept incoming follow / フォロー承認
	JobRejectFollow                         // reject incoming follow / フォロー拒否
	JobDeliverPost                          // fan-out new post to followers / 投稿をフォロワーに配信
	JobDeliverNote                          // deliver note to single actor / 1 Actor に Note 配送
	JobUpdatePost                           // fan-out post update / 投稿更新をファンアウト
	JobDeliverUpdateNote                    // deliver update note to single actor / 1 Actor に更新 Note 配送
	JobSendFollow                           // send follow request / フォローリクエスト送信
	JobUpdateActor                          // fan-out actor update / Actor 更新をファンアウト
	JobDeliverUpdate                        // deliver actor update to single actor / 1 Actor に更新配送
	JobDeliverDelete                        // fan-out post deletion / 投稿削除をファンアウト
	JobDeliverDeleteNote                    // deliver delete to single actor / 1 Actor に削除配送
	JobSendUndoFollow                       // send undo follow / フォロー解除送信
	JobSendLike                             // send like / いいね送信
	JobSendUndoLike                         // send undo like / いいね取消送信
	JobSendAnnounce                         // fan-out announce / リブログをファンアウト
	JobSendUndoAnnounce                     // fan-out undo announce / リブログ取消をファンアウト
	JobDeliverAnnounce                      // deliver announce to single actor / 1 Actor にリブログ配送
	JobSendBlock                            // send block / ブロック送信
	JobSendUndoBlock                        // send undo block / ブロック解除送信
	JobFetchRemoteActor                     // fetch and cache remote actor / リモート Actor フェッチ
)

// String returns the enum name of a JobType (e.g. "JobSendLike").
// JobType の enum 名を返す（例: "JobSendLike"）。
func (t JobType) String() string {
	names := [...]string{
		JobAcceptFollow:      "JobAcceptFollow",
		JobRejectFollow:      "JobRejectFollow",
		JobDeliverPost:       "JobDeliverPost",
		JobDeliverNote:       "JobDeliverNote",
		JobUpdatePost:        "JobUpdatePost",
		JobDeliverUpdateNote: "JobDeliverUpdateNote",
		JobSendFollow:        "JobSendFollow",
		JobUpdateActor:       "JobUpdateActor",
		JobDeliverUpdate:     "JobDeliverUpdate",
		JobDeliverDelete:     "JobDeliverDelete",
		JobDeliverDeleteNote: "JobDeliverDeleteNote",
		JobSendUndoFollow:    "JobSendUndoFollow",
		JobSendLike:          "JobSendLike",
		JobSendUndoLike:      "JobSendUndoLike",
		JobSendAnnounce:      "JobSendAnnounce",
		JobSendUndoAnnounce:  "JobSendUndoAnnounce",
		JobDeliverAnnounce:   "JobDeliverAnnounce",
		JobSendBlock:         "JobSendBlock",
		JobSendUndoBlock:     "JobSendUndoBlock",
		JobFetchRemoteActor:  "JobFetchRemoteActor",
	}
	if int(t) > 0 && int(t) < len(names) {
		return names[t]
	}
	return fmt.Sprintf("JobType(%d)", t)
}

// QueueJob is a background job in the queue.
// ジョブキューにおけるバックグラウンドジョブ。
type QueueJob struct {
	ID          id.ID
	Type        JobType // job kind / ジョブ種別
	Payload     string // JSON
	Status      JobStatus
	Attempts    int
	LastError   string // last error message / 最終エラーメッセージ
	NextRunAt   time.Time
	CreatedAt   time.Time
	CompletedAt time.Time // zero if not completed / 未完了ならゼロ値
}

// JobStatus represents the state of a queue job.
// ジョブの実行状態。
type JobStatus int

const (
	JobPending JobStatus = iota // waiting to run / 実行待ち
	JobRunning                  // currently running / 実行中
	JobDone                     // completed successfully / 完了
	JobFailed                   // failed, may retry / 失敗、リトライ対象
	JobDead                     // max retries exhausted / リトライ上限到達
)

// Setting is a key-value pair for application settings stored in DB.
// DB に保存されるアプリケーション設定の KV ペア。
type Setting struct {
	Key   string
	Value string
}

// InteractionCounts holds favourites/reblogs counts for a post.
// 投稿のいいね/リブログ数。
type InteractionCounts struct {
	Favourites int
	Reblogs    int
}

// MustJSON marshals v to a JSON string. Errors are silently ignored.
// v を JSON 文字列にマーシャルする。エラーは無視する。
// NewJob creates a pending QueueJob with the given type and payload.
// 指定された型とペイロードで保留中の QueueJob を生成する。
func NewJob(jobType JobType, payload any) *QueueJob {
	now := time.Now()
	return &QueueJob{
		ID:        id.New(),
		Type:      jobType,
		Payload:   MustJSON(payload),
		Status:    JobPending,
		NextRunAt: now,
		CreatedAt: now,
	}
}

func MustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
