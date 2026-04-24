# Actor

ActivityPub の主体。murlog では 1 インスタンス = 1 ユーザー（Persona）= 1 Actor。

> **凡例:** [仕様] = W3C/RFC/Draft の規定。[fediverse] = Mastodon/GoToSocial 等の実装で事実上必要な挙動。

## Actor オブジェクト

```json
{
  "@context": [
    "https://www.w3.org/ns/activitystreams",
    "https://w3id.org/security/v1"
  ],
  "id": "https://example.com/users/alice",
  "type": "Person",
  "preferredUsername": "alice",
  "name": "Alice",
  "summary": "自己紹介文",
  "inbox": "https://example.com/users/alice/inbox",
  "outbox": "https://example.com/users/alice/outbox",
  "followers": "https://example.com/users/alice/followers",
  "following": "https://example.com/users/alice/following",
  "publicKey": {
    "id": "https://example.com/users/alice#main-key",
    "owner": "https://example.com/users/alice",
    "publicKeyPem": "-----BEGIN PUBLIC KEY-----\n..."
  }
}
```

**[仕様] 必須フィールド (ActivityPub 4.1):**
- `id` — グローバル一意識別子
- `type` — Actor の種類
- `inbox` — Activity を受信する OrderedCollection
- `outbox` — Activity を公開する OrderedCollection

**[仕様] 推奨フィールド:**
- `following` — フォロー中の Actor 集合へのリンク
- `followers` — フォロワー集合へのリンク

**[仕様] オプショナル:**
- `preferredUsername` — 短いユーザー名（一意性保証なし）
- `liked` — いいねしたオブジェクト集合
- `streams` — 補助的なコレクション一覧
- `endpoints` — サーバー全体のエンドポイント群（`sharedInbox` 等）

**[fediverse] 事実上必須:**
- `preferredUsername` — `@alice@example.com` の `alice` 部分。WebFinger 解決に必要
- `followers` / `following` — 中身が空でもエンドポイント自体は必要
- `publicKey` — HTTP Signature 検証用。W3C Security Vocabulary (`https://w3id.org/security/v1`) で定義。ActivityPub 仕様自体には含まれないが、fediverse では必須
- `@context` に `https://w3id.org/security/v1` を含める（publicKey の語彙定義）

**[fediverse] 推奨:**
- `featured` — ピン留め投稿の OrderedCollection URI。GoToSocial 等が fetch を試みる
- `icon` — アバター画像。`{ "type": "Image", "url": "..." }`
- `image` — ヘッダー画像。`{ "type": "Image", "url": "..." }`
- `url` — 人間向けプロフィール URL（Actor URI とは別）
- `attachment` — カスタムフィールド。Mastodon 互換の PropertyValue 配列

**murlog の実装:**
- `attachment` にカスタムフィールドを PropertyValue 形式で含める (最大 4 件)
  ```json
  "attachment": [
    { "type": "PropertyValue", "name": "Website", "value": "<a href=\"https://example.com\">example.com</a>" }
  ]
  ```
- `featured` は `/users/{username}/collections/featured` にピン留め投稿の Note を含む OrderedCollection を返す (最大 1 件)

## エンドポイント

| パス | メソッド | Content-Type | 説明 |
|------|---------|-------------|------|
| `/users/{username}` | GET | `application/activity+json` | Actor JSON を返す |
| `/users/{username}/inbox` | POST | `application/activity+json` | Activity を受信 |
| `/users/{username}/outbox` | GET | `application/activity+json` | OrderedCollection |
| `/users/{username}/followers` | GET | `application/activity+json` | OrderedCollection or Collection |
| `/users/{username}/following` | GET | `application/activity+json` | OrderedCollection or Collection |
| `/users/{username}/collections/featured` | GET | `application/activity+json` | ピン留め投稿の OrderedCollection |

[仕様] Actor の GET は `Accept: application/activity+json` または `application/ld+json; profile="https://www.w3.org/ns/activitystreams"` で JSON-LD を返す。この 2 つは同等として扱う (Activity Streams 2.0 Core)。

## Collections

[仕様] OrderedCollection / Collection はページネーション対応のリスト。Outbox、Followers、Following で使用。

```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "https://example.com/users/alice/outbox",
  "type": "OrderedCollection",
  "totalItems": 42,
  "first": "https://example.com/users/alice/outbox?page=1"
}
```

**[仕様] ページネーション (Activity Streams 2.0 Core):**
- `first` / `last` — 最初/最後のページへの参照
- `next` / `prev` — 前後のページへの参照 (CollectionPage / OrderedCollectionPage)
- `startIndex` — OrderedCollectionPage での相対位置
- ページは単一または二重連結リストで配置

**[仕様] Followers / Following:**
- 型は OrderedCollection **または** Collection (ActivityPub 5.3, 5.4)
- リクエスターの認証に応じてフィルタリング可能

## 公開範囲

`to` / `cc` フィールドで制御する。

| 種類 | to | cc |
|------|----|----|
| 公開 | `["as:Public"]` | `["{actor}/followers"]` |
| 未収載 | `["{actor}/followers"]` | `["as:Public"]` |
| フォロワー限定 | `["{actor}/followers"]` | - |
| ダイレクト | `["相手のActor URI"]` | - |

**[仕様] Public Addressing (ActivityPub 5.6):**
- 識別子: `https://www.w3.org/ns/activitystreams#Public`
- JSON-LD コンパクション後は `Public` や `as:Public` も有効
- 認証なしで全ユーザーがアクセス可能
- Public 自体は配送対象ではない（実際の配送先は to/cc の他の値で決まる）
