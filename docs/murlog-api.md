# murlog API

murlog 独自 API (`/api/mur/v1/`) の仕様。JSON-RPC 2.0 + REST エンドポイントで構成。

Mastodon 互換 API (`/api/v1/`) は別途記載予定。

## 共通仕様

- **ID 形式**: UUIDv7 文字列 (`"0192a3b4-5c6d-7e8f-9a0b-1c2d3e4f5a6b"`)
- **日時形式**: RFC 3339 (`"2025-04-10T12:00:00Z"`)
- **ページネーション**: カーソルベース (`?cursor=<id>&limit=20`)

## 認証

| クライアント | 方式 |
|---|---|
| SPA (ブラウザ) | HttpOnly Cookie (`murlog_session`) |
| ネイティブ / 外部クライアント | 将来対応時に設計 |

`auth.login` で認証後、サーバーが HttpOnly Cookie を Set する。以降のリクエスト (HTTP POST / WebSocket 接続) で自動送信される。

CGI 環境で `Authorization` ヘッダーが届かない問題を回避でき、XSS 耐性も確保。

## REST エンドポイント

### `POST /api/mur/v1/media` (multipart)

メディアアップロードは multipart/form-data で行う（JSON-RPC 外）。

| フィールド | 必須 | 説明 |
|---|---|---|
| `file` | はい | 画像ファイル (JPEG/PNG/GIF/WebP、最大 10MB) |
| `alt` | いいえ | 代替テキスト |
| `prefix` | いいえ | 保存先 prefix (`attachments` / `avatars` / `headers`、デフォルト `attachments`) |

```json
// response (201 Created)
{ "id": "019d...", "url": "https://...", "path": "attachments/019d....jpg", "mime_type": "image/jpeg", "alt": "", "width": 1920, "height": 1080 }
```

EXIF メタデータ (GPS 等) はアップロード時に自動除去される。Orientation タグは保持。

### `GET /api/mur/v1/backup`

DB バックアップのダウンロード。`VACUUM INTO` による整合性のあるスナップショット。
認証必須。レスポンスは `Content-Disposition: attachment` で SQLite ファイルを返す。

## JSON-RPC 2.0

### 設計方針

- **JSON-RPC 2.0 準拠** — リクエスト・レスポンス・エラー・バッチ全て仕様通り
- **トランスポート非依存** — WebSocket と HTTP POST で同じメッセージフォーマット
- **統一プロトコル** — RPC call もサーバープッシュも同じ JSON-RPC メッセージ
- **SPA・ネイティブクライアント共通** — クライアント実装が1つで済む

### トランスポート

#### WebSocket (推奨)

```
ws(s)://{domain}/api/mur/v1/ws
```

- 双方向通信。RPC call もサーバープッシュも1コネクションに乗る
- 認証: 接続時に Cookie (HttpOnly) で自動認証
- serve / VPS 環境で使用

#### HTTP POST (フォールバック)

```
POST /api/mur/v1/rpc
Content-Type: application/json
```

- リクエスト-レスポンスのみ。サーバープッシュは受けられない
- 認証: Cookie (HttpOnly)
- CGI 環境、または WebSocket が使えない環境で使用
- バッチリクエスト対応

### メッセージフォーマット

#### リクエスト (クライアント → サーバー)

```json
{
  "jsonrpc": "2.0",
  "method": "posts.create",
  "params": { "content": "Hello, world!" },
  "id": 1
}
```

#### レスポンス (成功)

```json
{
  "jsonrpc": "2.0",
  "result": { "id": "0192a3b4-...", "content": "Hello, world!", ... },
  "id": 1
}
```

#### レスポンス (エラー)

```json
{
  "jsonrpc": "2.0",
  "error": {
    "code": -32001,
    "message": "not found",
    "data": { "resource": "post", "id": "0192a3b4-..." }
  },
  "id": 1
}
```

#### サーバープッシュ (サーバー → クライアント、WebSocket のみ)

JSON-RPC notification (`id` なし = レスポンス不要):

```json
{
  "jsonrpc": "2.0",
  "method": "notifications.new",
  "params": { "type": "mention", "account": { ... }, "status": { ... } }
}
```

#### バッチリクエスト

配列で複数メソッドを一括送信:

```json
[
  {"jsonrpc": "2.0", "method": "posts.list", "params": {"limit": 10}, "id": 1},
  {"jsonrpc": "2.0", "method": "notifications.poll", "params": {"since": "0192a3b4-..."}, "id": 2}
]
```

レスポンスも配列で返る (順序は保証しない):

```json
[
  {"jsonrpc": "2.0", "result": [...], "id": 1},
  {"jsonrpc": "2.0", "result": [...], "id": 2}
]
```

### エラーコード

#### JSON-RPC 2.0 標準エラー

| コード | 名前 | 用途 |
|--------|------|------|
| -32700 | Parse error | JSON が不正 |
| -32600 | Invalid request | 必須フィールド欠落等 |
| -32601 | Method not found | 存在しないメソッド |
| -32602 | Invalid params | パラメータのバリデーションエラー |
| -32603 | Internal error | サーバー内部エラー (DB障害等) |

#### アプリ固有エラー (-32000 〜 -32099)

| コード | 名前 | 用途 |
|--------|------|------|
| -32000 | Unauthorized | 認証が必要、またはトークン無効 |
| -32001 | Not found | リソースが存在しない |
| -32002 | Conflict | username 重複等 |
| -32003 | Forbidden | 権限不足 (現在未使用、将来拡張用) |

エラーの `data` フィールドに詳細情報を含められる:

```json
{
  "code": -32002,
  "message": "conflict",
  "data": { "field": "username", "reason": "already taken" }
}
```

### メソッド一覧

認証不要のメソッドは明記。それ以外は全て認証必須。

#### 認証 / auth

| メソッド | 認証 | 用途 |
|---|---|---|
| `auth.login` | 不要 | パスワードログイン → Cookie 発行 |
| `auth.logout` | 必要 | セッション破棄 |
| `auth.change_password` | 必要 | パスワード変更（旧パスワード検証） |

##### `auth.login`

```json
// request
{ "params": { "password": "...", "totp_code": "123456" } }
// totp_code: TOTP 有効時は必須。無効時は省略可

// response
{ "result": { "status": "ok" } }
// + Set-Cookie: murlog_session=...; HttpOnly; Secure; SameSite=Strict
```

レートリミット: 同一 IP から 5 回失敗すると 5 分間ロック。成功でリセット。

##### `auth.logout`

```json
// request
{ "params": {} }

// response
{ "result": { "status": "ok" } }
```

##### `auth.change_password`

```json
// request
{ "params": { "current_password": "old", "new_password": "new" } }

// response
{ "result": { "status": "ok" } }
```

#### ペルソナ / personas

| メソッド | 公開 | 用途 |
|---|---|---|
| `personas.list` | ✓ | 一覧取得 |
| `personas.get` | ✓ | 個別取得 |
| `personas.create` | ✗ | 作成 (RSA 鍵ペア自動生成) |
| `personas.update` | ✗ | 更新 |

##### `personas.list`

```json
// request
{ "params": {} }

// response
{ "result": [{ "id": "...", "username": "alice", "display_name": "Alice", ... }] }
```

##### `personas.get`

```json
// request
{ "params": { "id": "0192a3b4-..." } }

// response
{ "result": { "id": "...", "username": "alice", ... } }
```

##### `personas.create`

```json
// request
{ "params": { "username": "alice", "display_name": "Alice", "summary": "..." } }

// response (201 相当)
{ "result": { "id": "...", "username": "alice", ... } }
```

##### `personas.update`

```json
// request
{ "params": {
    "id": "...",
    "display_name": "Alice Updated",
    "summary": "...",
    "fields": [
      { "name": "Website", "value": "https://example.com" },
      { "name": "Location", "value": "Tokyo" }
    ],
    "avatar_path": "avatars/xxx.jpg",
    "header_path": "headers/xxx.jpg"
} }
// fields: カスタムフィールド (最大4件)。ActivityPub Actor の attachment (PropertyValue) として配信される

// response
{ "result": { "id": "...", "display_name": "Alice Updated", "fields": [...], "avatar_url": "https://...", "header_url": "https://...", ... } }
```

`avatar_path` / `header_path` は省略可。空文字で画像削除。レスポンスは解決済み URL (`avatar_url` / `header_url`) を返す。
`fields` は省略可。設定すると既存のフィールドを完全に置換する。空配列でクリア。

#### 投稿 / posts

| メソッド | 公開 | 用途 |
|---|---|---|
| `posts.list` | ✓ | 一覧 (ページネーション、未認証時は公開投稿のみ) |
| `posts.list_by_tag` | ✓ | ハッシュタグで投稿検索 |
| `posts.get` | ✓ | 個別取得 (未認証時は公開投稿のみ) |
| `posts.get_thread` | ✓ | スレッド取得 (祖先 + 子孫) |
| `posts.create` | ✗ | 作成 |
| `posts.update` | ✗ | 更新 |
| `posts.delete` | ✗ | 削除 |
| `posts.pin` | ✗ | ピン留め (ペルソナごとに最大1件、置き換え) |
| `posts.unpin` | ✗ | ピン留め解除 |

##### `posts.list`

```json
// request
{ "params": { "persona_id": "...", "cursor": "...", "limit": 20 } }
// persona_id 省略時はプライマリペルソナ、limit のデフォルトは 20、最大 100

// response
{ "result": [{ "id": "...", "content": "...", "visibility": "public", ... }] }
```

##### `posts.list_by_tag`

```json
// request
{ "params": { "tag": "murlog", "cursor": "...", "limit": 20 } }
// tag: ハッシュタグ名 (必須、# なし)
// limit: 1-40, デフォルト 20

// response
{ "result": [{ "id": "...", "content": "...", "visibility": "public", ... }] }
```

##### `posts.get`

```json
// request
{ "params": { "id": "..." } }

// response
{ "result": { "id": "...", "content": "...", ... } }
```

##### `posts.create`

```json
// request
{ "params": {
    "content": "<p>Hello!</p>",
    "content_map": { "ja": "...", "en": "..." },
    "summary": "CW テキスト",
    "sensitive": true,
    "visibility": "public",
    "persona_id": "...",
    "in_reply_to": "...",
    "attachments": ["..."]
} }
// persona_id 省略時はプライマリペルソナ、visibility のデフォルトは "public"
// summary: CW (Content Warning) テキスト。設定すると投稿が折りたたまれる
// sensitive: センシティブメディアフラグ。summary 設定時は自動で true
// in_reply_to: リプライ先投稿 ID
// attachments: 添付ファイル ID の配列 (最大 4)

// response
{ "result": { "id": "...", "content": "...", "summary": "...", "sensitive": true, ... } }
```

##### `posts.update`

```json
// request
{ "params": { "id": "...", "content": "...", "summary": "", "sensitive": false, "visibility": "unlisted" } }
// summary, sensitive はオプション。明示的に空文字/false を渡すと CW を解除

// response
{ "result": { "id": "...", ... } }
```

##### `posts.delete`

```json
// request
{ "params": { "id": "..." } }

// response
{ "result": { "status": "ok" } }
```

##### `posts.pin`

```json
// request
{ "params": { "id": "..." } }
// ローカル公開投稿のみピン留め可能。既存のピンは自動的に置き換え

// response
{ "result": { "id": "...", "pinned": true, ... } }
```

##### `posts.get_thread`

```json
// request
{ "params": { "id": "..." } }

// response
{ "result": {
    "ancestors": [{ "id": "...", "content": "...", ... }],
    "post": { "id": "...", "content": "...", ... },
    "descendants": [{ "id": "...", "content": "...", ... }]
} }
```

祖先は `in_reply_to_uri` チェーンを辿って取得 (最大 20 件)。子孫は直接の返信をフラットリストで時系列順に返す。

##### `posts.unpin`

```json
// request
{ "params": {} }

// response
{ "result": { "status": "ok" } }
```

#### タイムライン / timeline

| メソッド | 用途 |
|---|---|
| `timeline.home` | ホームタイムライン |

##### `timeline.home`

```json
// request
{ "params": { "cursor": "...", "limit": 20 } }

// response
{ "result": [{ "id": "...", "content": "...", "account": { ... }, ... }] }
```

#### アクター / actors

| メソッド | 公開 | 用途 |
|---|---|---|
| `actors.lookup` | ✗ | リモート Actor を WebFinger で解決 |
| `actors.outbox` | ✓ | リモート Actor の outbox (投稿一覧) を取得 |
| `actors.following` | ✓ | リモート Actor の following コレクションを取得 |
| `actors.followers` | ✓ | リモート Actor の followers コレクションを取得 |

##### `actors.lookup`

```json
// request
{ "params": { "acct": "bob@example.com" } }
// acct: user@domain 形式。先頭の @ は省略可

// response
{ "result": { "uri": "https://example.com/users/bob", "username": "bob", "display_name": "Bob", "summary": "...", "avatar_url": "https://..." } }
```

##### `actors.outbox`

```json
// request
{ "params": { "acct": "bob@example.com", "limit": 20 } }
// limit: 1-40, デフォルト 20

// response
{ "result": [{ "uri": "https://example.com/notes/1", "content": "<p>Hello</p>", "published": "2026-04-21T..." }] }
```

##### `actors.following` / `actors.followers`

```json
// request
{ "params": { "acct": "bob@example.com" } }

// response
{ "result": [{ "uri": "https://example.com/users/carol" }] }
```

#### フォロー・フォロワー / follows

| メソッド | 公開 | 用途 |
|---|---|---|
| `follows.list` | ✓ | フォロー一覧 (未認証時は show_follows 設定を尊重) |
| `follows.check` | ✗ | 指定 Actor をフォロー済みか確認 |
| `follows.create` | ✗ | フォローする |
| `follows.delete` | ✗ | フォロー解除 |
| `followers.list` | ✓ | フォロワー一覧 (未認証時は show_follows 設定を尊重) |
| `followers.pending` | ✗ | 承認待ちフォロワー一覧 |
| `followers.approve` | ✗ | フォローリクエストを承認 |
| `followers.reject` | ✗ | フォローリクエストを拒否 |
| `followers.delete` | ✗ | フォロワー削除 |

##### `follows.list`

```json
// request
{ "params": { "persona_id": "...", "cursor": "...", "limit": 20 } }

// response
{ "result": [{ "id": "...", "acct": "bob@example.com", ... }] }
```

##### `follows.check`

```json
// request
{ "params": { "target_uri": "https://example.com/users/bob" } }
// target_uri: 確認対象の Actor URI (必須)
// プライマリペルソナで確認。フォロー済みなら Follow オブジェクト、未フォローなら null を返す

// response (フォロー済み / following)
{ "result": { "id": "...", "target_uri": "https://...", "acct": "@bob@example.com", "accepted": true, ... } }

// response (未フォロー / not following)
{ "result": null }
```

##### `follows.create`

```json
// request
{ "params": { "persona_id": "...", "target_uri": "https://example.com/users/bob" } }
// target_uri: フォロー対象の Actor URI

// response
{ "result": { "id": "...", "target_uri": "https://...", "acct": "@bob@example.com", ... } }
```

##### `follows.delete`

```json
// request
{ "params": { "id": "..." } }

// response
{ "result": { "status": "ok" } }
```

##### `followers.list`

```json
// request
{ "params": { "persona_id": "...", "cursor": "...", "limit": 20 } }

// response
{ "result": [{ "id": "...", "acct": "carol@example.com", ... }] }
```

##### `followers.pending`

```json
// request
{ "params": { "persona_id": "..." } }

// response — same shape as followers.list, but unapproved only
// followers.list と同じ形式、未承認のみ
{ "result": [{ "id": "...", "actor_uri": "...", ... }] }
```

##### `followers.approve`

```json
// request
{ "params": { "id": "...", "activity_id": "..." } }
// activity_id: original Follow activity ID for Accept delivery (optional)
// activity_id: Accept 配送用の元の Follow アクティビティ ID (省略可)

// response
{ "result": { "status": "ok" } }
```

##### `followers.reject`

```json
// request
{ "params": { "id": "...", "activity_id": "..." } }

// response
{ "result": { "status": "ok" } }
```

##### `followers.delete`

```json
// request
{ "params": { "id": "..." } }

// response
{ "result": { "status": "ok" } }
```

#### 通知 / notifications

| メソッド | 用途 |
|---|---|
| `notifications.list` | 一覧 (ページネーション) |
| `notifications.read` | 個別既読 |
| `notifications.read_all` | 全既読 |
| `notifications.poll` | 未読通知の取得 (CGI ポーリング用) |

##### `notifications.list`

```json
// request
{ "params": { "cursor": "...", "limit": 20 } }

// response
{ "result": [{ "id": "...", "type": "mention", "account": { ... }, ... }] }
```

##### `notifications.read`

```json
// request
{ "params": { "id": "..." } }

// response
{ "result": { "status": "ok" } }
```

##### `notifications.read_all`

```json
// request
{ "params": {} }

// response
{ "result": { "status": "ok" } }
```

##### `notifications.poll`

```json
// request
{ "params": { "since": "0192a3b4-..." } }
// since: この ID 以降の通知を取得。CGI 環境でのポーリング用

// response
{ "result": [{ "id": "...", "type": "follow", ... }] }
```

#### メディア / media

| メソッド | 用途 |
|---|---|
| `media.delete` | 添付ファイル削除 |

##### `media.delete`

```json
// request
{ "params": { "id": "019d..." } }

// response
{ "result": { "status": "ok" } }
```

#### いいね / favourites

| メソッド | 用途 |
|---|---|
| `favourites.create` | 投稿にいいね (Like Activity 配送) |
| `favourites.delete` | いいね解除 (Undo Like 配送) |

##### `favourites.create`

```json
// request
{ "params": { "post_id": "...", "persona_id": "..." } }
// persona_id 省略時はプライマリペルソナ

// response
{ "result": { "id": "...", "content": "...", "favourited": true, "favourites_count": 1, ... } }
```

##### `favourites.delete`

```json
// request
{ "params": { "post_id": "...", "persona_id": "..." } }

// response
{ "result": { "id": "...", "content": "...", "favourited": false, ... } }
```

#### リブログ / reblogs

| メソッド | 用途 |
|---|---|
| `reblogs.create` | 投稿をリブログ (Announce Activity 配送) |
| `reblogs.delete` | リブログ解除 (Undo Announce 配送) |

##### `reblogs.create`

```json
// request
{ "params": { "post_id": "...", "persona_id": "..." } }

// response
{ "result": { "id": "...", "content": "...", "reblogged": true, "reblogs_count": 1, ... } }
```

##### `reblogs.delete`

```json
// request
{ "params": { "post_id": "...", "persona_id": "..." } }

// response
{ "result": { "id": "...", "content": "...", "reblogged": false, ... } }
```

#### ブロック / blocks

| メソッド | 用途 |
|---|---|
| `blocks.list` | ブロック一覧 |
| `blocks.create` | リモート Actor をブロック (フォロー双方向削除 + Block Activity 配送) |
| `blocks.delete` | ブロック解除 |

##### `blocks.list`

```json
// request
{ "params": {} }

// response
{ "result": [{ "id": "...", "actor_uri": "https://...", "created_at": "..." }] }
```

##### `blocks.create`

```json
// request
{ "params": { "actor_uri": "https://example.com/users/bob" } }

// response
{ "result": { "id": "...", "actor_uri": "https://...", "created_at": "..." } }
```

##### `blocks.delete`

```json
// request
{ "params": { "id": "..." } }

// response
{ "result": { "status": "ok" } }
```

#### ドメインブロック / domain_blocks

| メソッド | 用途 |
|---|---|
| `domain_blocks.list` | ドメインブロック一覧 |
| `domain_blocks.create` | ドメイン単位でブロック (該当ドメインのフォロー・フォロワー削除) |
| `domain_blocks.delete` | ドメインブロック解除 |

##### `domain_blocks.list`

```json
// request
{ "params": {} }

// response
{ "result": [{ "id": "...", "domain": "spam.example.com", "created_at": "..." }] }
```

##### `domain_blocks.create`

```json
// request
{ "params": { "domain": "spam.example.com" } }

// response
{ "result": { "id": "...", "domain": "spam.example.com", "created_at": "..." } }
```

##### `domain_blocks.delete`

```json
// request
{ "params": { "id": "..." } }

// response
{ "result": { "status": "ok" } }
```

#### TOTP / 二要素認証

| メソッド | 認証 | 用途 |
|---|---|---|
| `totp.status` | 不要 | TOTP 有効/無効の確認 |
| `totp.setup` | 必要 | 秘密鍵生成 + otpauth URI 返却 |
| `totp.verify` | 必要 | 6桁コードで TOTP を有効化 |
| `totp.disable` | 必要 | TOTP を無効化 |

##### `totp.setup`

```json
// response
{ "result": { "secret": "JBSWY3DPEHPK3PXP...", "uri": "otpauth://totp/..." } }
```

`uri` を QR コードとして表示し、認証アプリでスキャン。`totp.verify` でコードを送信して確定するまでは有効化されない。

##### `totp.verify`

```json
// request
{ "params": { "code": "123456" } }

// response
{ "result": { "status": "ok" } }
```

#### キュー管理 / queue

| メソッド | 用途 |
|---|---|
| `queue.stats` | pending/running/done/failed の件数 |
| `queue.list` | 最近のジョブ一覧 (limit パラメータ) |
| `queue.retry` | 失敗ジョブを pending に戻す |
| `queue.dismiss` | 失敗ジョブを完了扱いにする（諦め） |
| `queue.tick` | No-op (CGI リクエスト発生 → worker spawn トリガー) |
| `queue.vacuum` | 古い完了ジョブ削除 + VACUUM |

##### `queue.tick`

No-op。SPA キュー画面から CGI リクエストを発生させ、`spawnWorker()` のトリガーとして機能する。

```json
// response
{ "result": { "status": "ok" } }
```

##### `queue.vacuum`

```json
// response
{ "result": { "deleted": 42 } }
```

#### サイト設定

| メソッド | 説明 |
|---|---|
| `site.get_settings` | サイト全体設定を取得 |
| `site.update_settings` | サイト全体設定を更新 |

##### `site.get_settings`

```json
// response
{ "result": { "robots_noindex": false, "robots_noai": true } }
```

##### `site.update_settings`

```json
// request params
{ "robots_noindex": true, "robots_noai": true }

// response
{ "result": { "status": "ok" } }
```

### サーバープッシュ (WebSocket)

WebSocket 接続時、通知は自動的にプッシュされる (購読操作不要)。

```json
// 新しい通知
{ "jsonrpc": "2.0", "method": "notifications.new", "params": { "type": "mention", ... } }
```

将来、タイムラインのリアルタイム更新等が必要になった場合は `subscribe` / `unsubscribe` メソッドを追加してチャンネル多重化に拡張する。現時点では通知のみ。

### CGI 環境での運用

CGI では WebSocket が使えないため:

- RPC call は `POST /api/mur/v1/rpc` で実行
- 通知は `notifications.poll` メソッドで定期取得
- SPA 側で WebSocket 接続を試み、失敗したら HTTP POST + ポーリングにフォールバック
