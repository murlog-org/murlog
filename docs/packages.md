# murlog アーキテクチャ

本ドキュメントでは Go プロジェクトの構造、パッケージ設計、ストレージ抽象化の方針を示す。機能要件・技術選定については [プロダクト概要](./overview.md) を参照。

## ディレクトリ構成

```
murlog/
├── murlog.go                  # ドメイン型 (Persona, Post, Follow 等)
│
├── cmd/
│   └── murlog/
│       └── main.go            # サブコマンド振り分け (cgi/serve/worker/reset/update)
│
├── app/
│   └── app.go                 # App struct: 依存の組み立て・Handler 構築
│
├── handler/
│   ├── handler.go             # ルーティング (http.Handler)
│   ├── rpc.go                 # JSON-RPC 2.0 ディスパッチャ
│   ├── activitypub.go         # inbox / outbox / actor / Content Negotiation
│   ├── webfinger.go           # .well-known/webfinger
│   ├── nodeinfo.go            # NodeInfo エンドポイント
│   ├── api.go                 # JSON ヘルパー・API 認証ミドルウェア
│   ├── api_mur_auth.go        # murlog 独自 API — 認証
│   ├── api_mur_personas.go    # murlog 独自 API — ペルソナ
│   ├── api_mur_posts.go       # murlog 独自 API — 投稿
│   ├── api_mur_actors.go      # murlog 独自 API — リモート Actor
│   ├── api_mur_blocks.go      # murlog 独自 API — ブロック
│   ├── api_mur_follows.go     # murlog 独自 API — フォロー
│   ├── api_mur_interactions.go # murlog 独自 API — いいね・リブログ
│   ├── api_mur_media.go       # murlog 独自 API — メディア
│   ├── api_mur_notifications.go # murlog 独自 API — 通知
│   ├── api_mur_queue.go       # murlog 独自 API — キュー管理
│   ├── api_mur_site.go        # murlog 独自 API — サイト情報
│   ├── api_mur_timeline.go    # murlog 独自 API — タイムライン
│   ├── api_mur_totp.go        # murlog 独自 API — TOTP 2FA
│   ├── auth.go                # トークン生成・ハッシュ・Bearer 抽出
│   ├── setup.go               # 初回セットアップ (/admin/setup, SSR)
│   ├── reset.go               # パスワードリセット (/admin/reset, SSR)
│   ├── ssr.go                 # クローラー/OGP 向け軽量 SSR
│   ├── tags.go                # ハッシュタグページ
│   ├── robots.go              # robots.txt
│   ├── backup.go              # DB バックアップダウンロード
│   └── worker_tick.go         # CGI 用ワーカー tick エンドポイント
│
├── activitypub/
│   ├── vocab.go               # Activity/Actor/Object の型定義
│   ├── signature.go           # HTTP Signatures (署名・検証)
│   ├── deliver.go             # リモート配送
│   ├── resolve.go             # リモート Actor/Note/Collection 取得 (FetchActorSigned, FetchNoteSigned, FetchCollectionSigned)
│   └── webfinger.go           # WebFinger リクエスト (リモート Actor 発見)
│
├── store/
│   ├── store.go               # Store interface + レジストリ (Register/Open)
│   ├── sqlite/
│   │   ├── sqlite.go          # SQLite 実装 (modernc.org/sqlite)
│   │   ├── persona.go         # Persona CRUD
│   │   ├── post.go            # Post CRUD
│   │   ├── follow.go          # Follow 管理
│   │   ├── session.go         # セッション管理
│   │   ├── setting.go         # サイト設定
│   │   ├── attachment.go      # 添付ファイル
│   │   ├── block.go           # ブロック (ユーザー・ドメイン)
│   │   ├── domain_failure.go  # ドメイン配送失敗記録
│   │   ├── favourite.go       # いいね
│   │   ├── reblog.go          # リブログ
│   │   ├── notification.go    # 通知
│   │   ├── remote_actor.go    # リモート Actor キャッシュ
│   │   ├── login_attempt.go   # ログイン試行 (レート制限)
│   │   ├── oauth.go           # OAuth トークン
│   │   └── migrations/        # SQLite 用マイグレーション SQL
│   ├── mysql/
│   │   ├── mysql.go           # MySQL 実装 (go-sql-driver/mysql)
│   │   └── migrations/
│   ├── postgres/
│   │   ├── postgres.go        # PostgreSQL 実装 (pgx)
│   │   └── migrations/
│   └── all/
│       └── all.go             # 全ドライバの blank import
│
├── media/
│   ├── media.go               # Interface 定義
│   ├── fs/
│   │   └── fs.go              # ファイルシステム実装
│   ├── s3/
│   │   ├── s3.go              # S3 互換ストレージ実装
│   │   └── sigv4.go           # AWS Signature V4 自前実装
│   └── imageproc/
│       └── strip.go           # EXIF 除去 (GPS 削除、Orientation 保持)
│
├── mention/
│   └── mention.go             # @user@domain 解析・HTML 変換
│
├── hashtag/
│   └── hashtag.go             # #tag 解析・HTML 変換
│
├── totp/
│   └── totp.go                # TOTP 生成・検証 (RFC 6238, HMAC-SHA1)
│
├── internal/
│   ├── mediautil/
│   │   └── mediautil.go       # メディアユーティリティ (MIME 判定, Actor 画像抽出)
│   └── sqlutil/
│       └── sqlutil.go         # SQL ユーティリティ (時刻変換, ID 変換)
│
├── queue/
│   ├── queue.go               # ジョブキュー Interface
│   └── sqlqueue/
│       └── sqlqueue.go        # SQL ベースのキュー実装
│
├── worker/
│   └── worker.go              # ジョブワーカー (配送・フォロー承認等の非同期処理)
│
├── id/
│   └── id.go                  # UUIDv7 生成・パース
│
├── config/
│   └── config.go              # murlog.toml の読み込み・生成
│
├── i18n/
│   ├── i18n.go                # 翻訳関数 t("key")
│   └── locales/
│       ├── en.json
│       └── ja.json
│
├── scripts/
│   └── seeddb.go              # パフォーマンステスト用シードデータ生成
│
├── web/                       # SPA フロントエンド (Preact + Vite)
│   └── templates/             # クローラー向け軽量 SSR テンプレート (embed)
│
├── e2e/                       # Playwright e2e テスト
├── docker/                    # CGI デプロイ用 Docker 構成
├── docs/
├── go.mod
└── go.sum
```

### フラット構成から始める

v1 ではパッケージをフラットに保つ。Go のパッケージは公開 API 境界であり、細かく分けすぎると本来内部的な型や関数まで export が必要になる。規模が大きくなってから分割する。

想定される分割パス:

| 現状 | 肥大化したら |
|------|-------------|
| `handler/api_mur_*.go` | `handler/api/mur/` にサブパッケージ化 |
| `store/store.go` の単一 Interface | `PostStore`, `PersonaStore` 等に分割 |
| `activitypub/*.go` | `activitypub/inbox/`, `activitypub/outbox/` に分離 |

### ドメイン型はルートパッケージに置く

`Persona`, `Post`, `Follow` 等のドメイン型はプロジェクトルートの `murlog.go` に定義する。

```go
// murlog.go
package murlog

type Persona struct {
    ID          string
    Username    string
    DisplayName string
    Summary     string
    PublicKey   string
    PrivateKey  string
    CreatedAt   time.Time
}

type Post struct {
    ID        string
    PersonaID string
    Content   string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

全パッケージがルートに依存する一方通行になり、循環参照が起きない。`murlog.Post`, `murlog.Persona` のように読みやすい名前になる。

## ストレージ抽象化

### Store interface

```go
// store/store.go
package store

type Store interface {
    // Persona
    GetPersona(ctx context.Context, id string) (*murlog.Persona, error)
    GetPersonaByUsername(ctx context.Context, username string) (*murlog.Persona, error)
    ListPersonas(ctx context.Context) ([]*murlog.Persona, error)
    CreatePersona(ctx context.Context, p *murlog.Persona) error

    // Post
    GetPost(ctx context.Context, id string) (*murlog.Post, error)
    ListPostsByPersona(ctx context.Context, personaID string, cursor string, limit int) ([]*murlog.Post, error)
    CreatePost(ctx context.Context, p *murlog.Post) error
    UpdatePost(ctx context.Context, p *murlog.Post) error
    DeletePost(ctx context.Context, id string) error

    // Follow
    // Session
    // Queue
    // ...

    // ライフサイクル
    Migrate(ctx context.Context) error
    Close() error
}
```

### レジストリパターン

`database/sql` の `sql.Register` / `sql.Open` と同じパターンでドライバを登録する。

```go
// store/store.go
type Driver func(dsn string) (Store, error)

var drivers = map[string]Driver{}

func Register(name string, d Driver) {
    drivers[name] = d
}

func Open(name, dsn string) (Store, error) {
    d, ok := drivers[name]
    if !ok {
        return nil, fmt.Errorf("store: unknown driver %q", name)
    }
    return d(dsn)
}
```

各実装は `init()` で自身を登録する。

```go
// store/sqlite/sqlite.go
package sqlite

import "github.com/yourname/murlog/store"

func init() {
    store.Register("sqlite", func(dsn string) (store.Store, error) {
        return New(dsn)
    })
}
```

### ビルドタグによるドライバ選択

各ドライバはビルドタグで制御する。バイナリに含まれるドライバを限定できる。

```go
// store/sqlite/sqlite.go
//go:build sqlite || all_stores || (!mysql && !postgres)
```

```go
// store/mysql/mysql.go
//go:build mysql || all_stores
```

```go
// store/postgres/postgres.go
//go:build postgres || all_stores
```

| ビルド方法 | コマンド | バイナリに含まれるドライバ |
|---|---|---|
| SQLite のみ (デフォルト) | `go build` | SQLite |
| MySQL のみ | `go build -tags mysql` | MySQL |
| PostgreSQL のみ | `go build -tags postgres` | PostgreSQL |
| 全部入り | `go build -tags all_stores` | SQLite + MySQL + PostgreSQL |

### 全部入りビルド

`store/all/all.go` は全ドライバを blank import するだけのファイル。

```go
// store/all/all.go
//go:build all_stores

package all

import (
    _ "github.com/yourname/murlog/store/sqlite"
    _ "github.com/yourname/murlog/store/mysql"
    _ "github.com/yourname/murlog/store/postgres"
)
```

全部入りビルドでは `murlog.toml` の `db_driver` で実行時に切り替える。

```toml
# murlog.toml
db_driver = "sqlite"
db_source = "./murlog.db"
```

### SQL 方言の吸収

SQL 方言の差（プレースホルダ、型名、DDL 構文等）は各実装パッケージ内で吸収する。共通の SQL ビルダーやヘルパーは設けない。マイグレーション SQL も実装ごとに持つ。

```
store/sqlite/migrations/001_init.sql      -- SQLite 用 DDL
store/mysql/migrations/001_init.sql       -- MySQL 用 DDL
store/postgres/migrations/001_init.sql    -- PostgreSQL 用 DDL
```

## パッケージ依存の方向

```
murlog.go (ドメイン型)
    ↑
    ├── store/       Interface 定義 + レジストリ
    │     ↑
    │     ├── store/sqlite/
    │     ├── store/mysql/
    │     └── store/postgres/
    │
    ├── activitypub/ プロトコルロジック
    ├── mention/     メンション解析 (ParseMentions, ReplaceWithHTML)
    ├── hashtag/     ハッシュタグ解析・HTML 変換
    ├── totp/        TOTP 生成・検証 (RFC 6238)
    ├── queue/       ジョブキュー (store に依存)
    ├── worker/      ジョブワーカー (queue, activitypub に依存)
    ├── internal/
    │     ├── mediautil/  メディアユーティリティ (handler, worker で共有)
    │     └── sqlutil/    SQL ユーティリティ (store/sqlite, queue/sqlqueue で共有)
    ├── id/          UUIDv7 生成
    ├── config/      設定読み込み
    ├── i18n/        翻訳
    ├── media/       メディアストレージ (fs / s3 / imageproc)
    │
    └── handler/     HTTP ハンドラ (store, activitypub, queue, i18n に依存)
          ↑
          app/       依存の組み立て
          ↑
          cmd/murlog/ エントリポイント
```

すべてのパッケージはルートパッケージ (`murlog`) に依存し、ルートパッケージは他のどのパッケージにも依存しない。
