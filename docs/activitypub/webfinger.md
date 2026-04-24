# WebFinger・NodeInfo

> **凡例:** [仕様] = W3C/RFC/Draft の規定。[fediverse] = Mastodon/GoToSocial 等の実装で事実上必要な挙動。

## WebFinger

`user@domain` 形式のアドレスから Actor URI を解決するプロトコル (RFC 7033)。

### リクエスト

```
GET /.well-known/webfinger?resource=acct:alice@example.com
```

**[仕様] パラメータ:**
- `resource` (必須) — クエリ対象の URI
- `rel` (オプション) — リンク関係タイプのフィルタ（複数指定可）

### レスポンス

**[仕様] Content-Type:** `application/jrd+json`

```json
{
  "subject": "acct:alice@example.com",
  "aliases": ["https://example.com/@alice"],
  "links": [
    {
      "rel": "self",
      "type": "application/activity+json",
      "href": "https://example.com/users/alice"
    }
  ]
}
```

**[仕様] JRD のフィールド:**
- `subject` — リソースを識別する URI。リクエストの `resource` とは異なる場合がある（正規化時）。SHOULD で出現
- `aliases` — subject の別名 URI の配列（オプション）
- `properties` — リソースの追加プロパティ（オプション）
- `links` — リンク関係の配列。各要素は:
  - `rel` (必須) — リンク関係タイプ (URI or IANA 登録タイプ)
  - `href` (オプション) — ターゲット URI
  - `type` (オプション) — メディアタイプ
  - `titles` (オプション) — 言語別タイトル
  - `properties` (オプション) — リンク固有の追加情報

### フォロー時の Actor 解決フロー

```
@alice@remote.example
  → WebFinger: GET https://remote.example/.well-known/webfinger?resource=acct:alice@remote.example
  → links[rel=self, type=application/activity+json].href = https://remote.example/users/alice
  → Actor fetch: GET https://remote.example/users/alice (Accept: application/activity+json)
  → actor.inbox = https://remote.example/users/alice/inbox
```

## NodeInfo

インスタンスのメタ情報を公開する仕様。fediverse のインスタンス一覧サービスやリモートサーバーが参照する。

### Discovery

```
GET /.well-known/nodeinfo
```

[仕様] JRD 形式で、サポートするスキーマへの Link を返す。クライアントは HTTPS を優先し、接続エラー時に HTTP へフォールバック。

```json
{
  "links": [
    {
      "rel": "http://nodeinfo.diaspora.software/ns/schema/2.0",
      "href": "https://example.com/nodeinfo/2.0"
    }
  ]
}
```

### NodeInfo 2.0

```
GET /nodeinfo/2.0
```

**[仕様] 全フィールド (すべて必須):**

| フィールド | 型 | 説明 |
|-----------|-----|------|
| `version` | string | スキーマバージョン。`"2.0"` |
| `software` | object | `name` (a-z0-9- のみ), `version` |
| `protocols` | array | サポートプロトコル。`["activitypub"]` 等。最低 1 項目 |
| `services` | object | `inbound` (受信元サービス), `outbound` (送信先サービス)。共に配列 |
| `openRegistrations` | boolean | 自己登録を許可するか |
| `usage` | object | `users` (`total`, `activeHalfyear`, `activeMonth`), `localPosts`, `localComments` |
| `metadata` | object | ソフトウェア固有の自由形式キーバリューペア |

```json
{
  "version": "2.0",
  "software": { "name": "murlog", "version": "0.1" },
  "protocols": ["activitypub"],
  "services": { "inbound": [], "outbound": [] },
  "openRegistrations": false,
  "usage": {
    "users": { "total": 1 },
    "localPosts": 42
  },
  "metadata": {}
}
```
