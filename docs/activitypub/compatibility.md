# fediverse 実装との互換性メモ

> ソース: Mastodon 公式ドキュメントリポ (mastodon/documentation, content/en/spec/) + ソースコード (v4.5.8, app/lib/)、GoToSocial ソースコード内ドキュメント (v0.21.0, docs/federation/)、murlog 連合テスト実測結果
>
> 対象バージョン: **Mastodon v4.5.8** (2025-03-24)、**GoToSocial v0.21.0**、**Misskey v2026.3.2**

## Mastodon

**HTTP Signature** (出典: app/lib/http_signature_draft.rb, app/lib/signed_request.rb):
- 送信アルゴリズム: `rsa-sha256` (RSASSA-PKCS1-v1_5 with SHA-256)
- 検証アルゴリズム: `rsa-sha256` と `hs2019` をサポート
- `keyId` の形式: `{actor_uri}#main-key`（フラグメント識別子）
- 送信時の headers: `(request-target) host date` (GET)、`(request-target) host date digest` (POST)
- 検証時の要件 (verify_signature_strength!):
  - `date` or `(created)` が署名に含まれること（必須）
  - GET: `host` が署名に含まれること（必須）
  - POST: `digest` が署名に含まれること（必須）
  - `(request-target)` or `digest` が署名に含まれること（必須）
- 時間窓: 12 時間 (EXPIRATION_WINDOW_LIMIT) + 1 時間のクロックスキュー許容 (CLOCK_SKEW_MARGIN)
- クエリパラメータ: あり/なし両方で検証を試行（互換性対応）
- **HTTP Message Signatures (RFC 9421)** (出典: content/en/spec/security.md):
  - v4.4.0: 検証サポート追加（`http_message_signatures` フラグ必要）
  - v4.5.0: デフォルト有効
  - `Signature` と `Signature-Input` の 2 ヘッダーを使用
  - 要件: 単一署名のみ、`created` 必須、`@method` + `@target-uri` 必須、POST は `Content-Digest` (RFC 9530) 必須
  - アルゴリズム: RSASSA-PKCS1-v1_5 Using SHA-256 のみ
- Authorized Fetch (Secure Mode): 有効時は GET リクエストにも署名を要求
- **keyId の処理** (出典: content/en/spec/security.md 脚注):
  - フラグメント形式 (`actor#key`): owner は同一ドキュメント (actor) を指す
  - パス形式 (`/key`): owner が actor を指し、actor がその key を指すこと（双方向確認）

**Linked Data Signatures** (出典: docs.joinmastodon.org/spec/security/):
- `RsaSignature2017` を使用（現在は `RsaSignature2018` が標準）
- 仕様自体が Verifiable Credential Data Integrity 1.0 に置き換わっている
- 新規実装は非推奨: "it is not advised to implement support for LD Signatures"

**Actor** (出典: docs.joinmastodon.org/spec/activitypub/):
- 必須: `preferredUsername`, `publicKey`
- 推奨: `name`, `summary`, `url`, `icon`, `image`, `manuallyApprovesFollowers`, `featured`, `attachment` (PropertyValue)
- オプション: `discoverable`, `indexable`, `suspended`, `memorial`, `featuredTags`

**Inbox で処理する Activity** (出典: docs.joinmastodon.org/spec/activitypub/):
- ステータス: `Create`, `Delete`, `Like`, `Announce`, `Update`, `Undo`, `Flag`, `QuoteRequest`
- プロフィール: `Follow`, `Accept`, `Reject`, `Add`, `Remove`, `Update`, `Delete`, `Block`, `Move`

**公開範囲** (出典: docs.joinmastodon.org/spec/activitypub/):
- `public`: `as:Public` が `to` に含まれる
- `unlisted`: `as:Public` が `cc` に含まれる
- `private`: follower collection が `to`/`cc` に含まれ、`as:Public` なし
- `limited`: `to`/`cc` に複数アクター、少なくとも 1 つが Mention されていない
- `direct`: `to`/`cc` 内のすべてのアクターが Mention タグに含まれる

**リプライ・メンション** (出典: docs.joinmastodon.org/spec/activitypub/):
- リプライ: `inReplyTo` に返信先 Note の URI。受信時に `inReplyTo` を fetch してスレッドを構築
- メンション HTML: `<span class="h-card"><a href="{profile_url}" class="u-url mention">@<span>{username}</span></a></span>`
- メンション `tag`: `{ "type": "Mention", "href": "{actor_uri}", "name": "@user@domain" }`
- メンション先は `cc` に含める。`to`/`cc` 内の全アクターが Mention タグに含まれる場合は `direct` として扱う
- スレッド context: `/api/v1/statuses/:id/context` → `{ "ancestors": [...], "descendants": [...] }`

**WebFinger** (出典: content/en/spec/webfinger.md):
- `acct:` URI で統一形式に変換
- links に `rel=self` (Actor URI)、`rel=http://webfinger.net/rel/profile-page` (Web URL)、`rel=http://ostatus.org/schema/1.0/subscribe` (リモートフォロー用 template) を含む
- WebFinger は Mastodon との完全な相互運用に必須。WebFinger 未対応の ActivityPub 実装は検索やフォローに失敗する
- **正規化フロー**: Actor の `preferredUsername` + ホスト名から acct URI を再構築 → `subject` と比較 → 不一致の場合は正規 URI で追加 WebFinger → 同一 Actor を指すことを確認
- 最小限の WebFinger レスポンス: `subject` + `links[rel=self, type=application/activity+json]` のみで動作

## GoToSocial

**HTTP Signature** (出典: docs/federation/http_signatures.md):
- 全ての GET/POST リクエストに署名を要求 (Mastodon の Authorized Fetch / Secure Mode 相当がデフォルト)
- 署名検証アルゴリズム (優先順): `RSA_SHA256` → `RSA_SHA512` → `ED25519`
- 送信時の `algorithm` フィールド: `hs2019` を設定（実際の署名は RSA_SHA256）
- 送信時の headers: GET は `(request-target) host date`、POST は `(request-target) host date digest`
- `keyId` の形式: `{actor_uri}/main-key`（パスセグメント。Mastodon のフラグメント `#main-key` とは異なる）
- `keyId` の GET で Actor 全体ではなく publicKey のみの stub を返す
- クエリパラメータの署名: v0.14 以降はあり/なし両方で試行（互換性対応）

**Actor** (出典: docs/federation/actors.md):
- `Person` (通常)、`Service` (bot)、`Application` (インスタンスアカウント) を使い分け
- `manuallyApprovesFollowers`: デフォルト `true`（locked アカウント）
- `featured`: ピン留めコレクション URI を提供。中身は Note URI のリスト（Mastodon と異なり完全な Note オブジェクトではない）
- `attachment`: Mastodon 互換の PropertyValue (最大 6 フィールド、Mastodon は 4)
- sharedInbox: 未実装（重複排除はサーバー側で処理）

**Inbox** (出典: docs/federation/actors.md):
- Content-Type: `application/activity+json`, `application/activity+json; charset=utf-8`, `application/ld+json; profile="https://www.w3.org/ns/activitystreams"` のみ受付。それ以外は 406
- 署名付き POST のみ受付。正常時は 202、不正時は 400 を返す
- 202 を返しても処理を継続するとは限らない

**アクセス制御** (出典: docs/federation/access_control.md):
- 未署名リクエストは 401 Unauthorized
- `keyId` のホストがブロックドメインなら 403 Forbidden
- ブロック関係がある場合も 403 Forbidden

**Outbox** (出典: docs/federation/actors.md):
- OrderedCollection でページネーション対応 (`first` → OrderedCollectionPage)
- `orderedItems` は Create Activity のみ。object は Note の AP URI（本文は含まない）
- 1 ページ最大 30 件

**Followers/Following** (出典: docs/federation/actors.md):
- OrderedCollection でページネーション対応（1 ページ最大 40 件）
- ユーザー設定でコレクション非公開可能（`totalItems` のみの stub を返す）

**Interaction Policy** (出典: docs/federation/interaction_controls.md):
- `interactionPolicy` プロパティで投稿ごとの操作制御（`canLike`, `canReply`, `canAnnounce`）
- GTS 独自拡張 (`https://gotosocial.org/ns`)

**投稿** (出典: docs/federation/posts.md):
- 添付ファイル: `Image`, `Video`, `Audio`, `Document` タイプ。blurhash, focalPoint 対応
- `content` + `contentMap` で言語推定（BCP47 タグ）
- Hashtag: ActivityStreams `Hashtag` type 拡張。`tag` プロパティに含める
- カスタム絵文字: `http://joinmastodon.org/ns#Emoji` タイプ
- Delete: `Object` フィールドから URI を抽出、所有者チェック後に削除

**リプライ・メンション** (出典: docs/federation/posts.md, docs/federation/actors.md):
- リプライ: `inReplyTo` に返信先 Note URI。受信時に `inReplyTo` を fetch してスレッドを構築
- メンション: Mastodon 互換の `tag` Mention + `cc` 配送。HTML も Mastodon 互換形式
- `interactionPolicy.canReply` でリプライ可能な Actor を制限可能（GTS 独自拡張）

## Misskey

> 出典: vendor/misskey (v2026.3.2) ソースコード。主に packages/backend/src/core/activitypub/ 配下

**HTTP Signature** (出典: ApRequestService.ts):
- 送信アルゴリズム: `rsa-sha256`
- 送信時の headers: POST は `(request-target) date host digest`、GET は `(request-target) date host`
- Digest: `SHA-256=base64...`
- Content-Type (送信): `application/activity+json`
- Content-Type (受信): `application/activity+json` または `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`
- Accept (GET 送信時): `application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams"`

**署名検証** (出典: InboxProcessorService.ts, ActivityPubServerService.ts):
- ライブラリ: `@peertube/http-signature`
- 受信時の必須ヘッダー: `host`, `digest`, `(request-target)`
- Digest は SHA-256 のみ許可
- HTTP Signature 失敗時に **LD-Signature (RsaSignature2017) にフォールバック**
- Authorized Fetch: 未実装（HTTP Signature ベースの認証に依存）

**Actor** (出典: ApRendererService.ts, type.ts):
- type: `Person` (通常), `Service` (bot), `Organization`, `Group`, `Application`
- `keyId` の形式: `{base_url}/users/{id}#main-key`（フラグメント形式、Mastodon と同じ）
- `publicKey.id`: `{base_url}/users/{id}/publickey`（別エンドポイント）
- `endpoints.sharedInbox`: 実装あり
- `featured`: ピン留めコレクション対応
- `manuallyApprovesFollowers`, `discoverable` 対応
- `movedTo`, `alsoKnownAs`: アカウント移行対応
- `attachment`: Mastodon 互換の PropertyValue
- `isCat`: 猫フラグ（Misskey 独自）

**Inbox で処理する Activity** (出典: ApInboxService.ts):
- `Create` (Note), `Delete` (Actor/Note), `Update` (Person/Question)
- `Follow`, `Accept`, `Reject`
- `Add` / `Remove` (featured ピン留め)
- `Announce` (リブログ/リノート)
- `Like` (リアクション — Misskey 独自拡張あり)
- `Undo` (Follow/Block/Like/Announce/Accept)
- `Block`, `Flag` (通報), `Move` (アカウント移行)

**WebFinger** (出典: WellKnownServerService.ts):
- JRD / XRD 両対応（Accept ヘッダーで選択）
- links: `rel=self` (Actor URI), `rel=http://webfinger.net/rel/profile-page` (HTML), `rel=http://ostatus.org/schema/1.0/subscribe` (リモートフォロー)
- リソース: `acct:` URI と URL 形式の両方に対応

**Misskey 独自拡張** (namespace: `https://misskey-hub.net/ns#`):
- `_misskey_content`: MFM (Misskey Flavored Markdown) 形式のテキスト。`content` (HTML) と並行して提供
- `_misskey_quote` / `quoteUrl`: 引用投稿 URL。両方のフィールドを並行提供（互換性のため）
- `_misskey_reaction`: Like Activity 内のリアクション絵文字。Unicode 絵文字 or カスタム絵文字識別子
- `EmojiReaction` / `EmojiReact`: Like の代替 Activity タイプとして受信対応
- `_misskey_summary`: Actor の別表現説明文
- `_misskey_votes`: Poll の投票数（`replies.totalItems` と並行）
- `source.content` + `source.mediaType: "text/x.misskeymarkdown"`: MFM ソーステキスト

**リプライ・メンション** (出典: ApRendererService.ts, ApNoteService.ts):
- リプライ: `inReplyTo` に返信先 Note URI。受信時に `inReplyTo` を fetch してスレッドを構築
- メンション: Mastodon 互換の `tag` Mention + `cc` 配送
- メンション HTML: Mastodon 互換 + `_misskey_content` (MFM) で `@user@domain` をそのまま含む
- 引用リプライ: `_misskey_quote` / `quoteUrl` で引用先 URI を指定（murlog では対応不要、無視してよい）

**murlog との互換性で注意すべき点:**
- Like Activity に `_misskey_reaction` がある場合、リアクション絵文字として扱う（murlog 側で対応しない場合は通常の Like として処理すればよい）
- `_misskey_quote` / `quoteUrl` は引用投稿。対応しない場合は無視してよい
- LD-Signature フォールバックがあるため、HTTP Signature が正しく動作すれば互換性問題は少ない
- sharedInbox 対応済みなので、複数ユーザー宛の配送効率が良い

## 3 実装の比較

| 項目 | Mastodon | GoToSocial | Misskey |
|------|----------|------------|---------|
| 送信 algorithm | `rsa-sha256` | `hs2019` (実体は RSA-SHA256) | `rsa-sha256` |
| 検証 algorithm | rsa-sha256, hs2019, RFC 9421 | RSA-SHA256, RSA-SHA512, Ed25519 | rsa-sha256 (+ LD-Sig フォールバック) |
| 送信 headers (POST) | `(request-target) host date digest` | `(request-target) host date digest` | `(request-target) date host digest` |
| keyId 形式 | `actor#main-key` | `actor/main-key` | `actor#main-key` |
| Authorized Fetch | オプション (Secure Mode) | デフォルト有効 | なし |
| sharedInbox | あり | なし | あり |
| LD-Signature | 送信あり (非推奨) | なし | 検証フォールバック |
| featured コレクション | fetch する | fetch する (ないとエラーログ) | 対応 |
| 独自拡張 | PropertyValue, blurhash, focalPoint | interactionPolicy, hidesToPublic | _misskey_reaction, _misskey_quote, isCat |
| リプライの `inReplyTo` | Note URI | Note URI | Note URI |
| メンション HTML | `<span class="h-card"><a class="u-url mention">` | Mastodon 互換 | Mastodon 互換 + MFM |
| メンション `tag` | `Mention` type, `href` + `name` | 同左 | 同左 |
| メンション先の配送 | `cc` に Actor URI を追加 | 同左 | 同左 |
| スレッド context API | `/api/v1/statuses/:id/context` | 同左 | 同左 (ancestors + descendants) |
| リモートスレッド fetch | `inReplyTo` を再帰 fetch | 同左 | 同左 |

## 互換性上の注意点

**murlog 連合テストで判明した事項:**
- GTS はデフォルトで `locked=true`。CLI でアカウント作成すると手動承認モードになる（テスト時は DB で直接 `locked=0` に更新が必要）
- GTS は Actor の `featured` コレクションがない場合エラーログを出す（`"error fetching account featured collection: empty url host"`）。致命的ではないが、空の OrderedCollection を返すのが望ましい
- GTS は `/api/v1/instance` を叩いてインスタンス情報を取得しようとする（404 → nodeinfo にフォールバック）
- GTS の `keyId` はパス形式 (`/main-key`) なので、`keyId` から Actor URI を取得する際はフラグメント (`#`) だけでなくパスの除去も必要

**murlog が全 3 実装と互換するために必要なこと:**
- HTTP Signature: `rsa-sha256` で `(request-target) host date digest` を署名すれば全実装で通る
- `keyId` のパース: フラグメント (`#`) とパス (`/`) の両方に対応すること（GTS 対策）
- `featured` コレクション: 空でもエンドポイントを用意すること（GTS 対策）
- WebFinger: `subject` + `links[rel=self]` を返せば最低限動作する
- Content-Type: `application/activity+json` で送信、受信は `application/ld+json; profile=...` も許可
- リプライ: `inReplyTo` に返信先 Note URI を設定 + 返信先 Actor を `cc` に追加。全 3 実装で共通の挙動
- メンション: `tag` に `Mention` オブジェクト (`href` + `name`) + `cc` に Actor URI。HTML 内のメンションリンクは Mastodon 形式 (`<span class="h-card">`) が事実上の標準
- スレッド: `inReplyTo` チェーンを辿る方式が全実装共通。リモートスレッドの fetch は murlog では行わない（手元のデータ範囲で表示）
