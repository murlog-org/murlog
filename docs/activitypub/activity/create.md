# Create

オブジェクトの作成。通常 Create/Note (投稿) に使われる。
リプライ・スレッド・メンション、および関連する Undo (Like/Announce) もこのページで扱う。

> **凡例:** [仕様] = W3C/RFC/Draft の規定。[fediverse] = Mastodon/GoToSocial 等の実装で事実上必要な挙動。

## Create/Note

```json
{
  "type": "Create",
  "actor": "https://a.example/users/alice",
  "object": {
    "type": "Note",
    "id": "https://a.example/users/alice/posts/456",
    "attributedTo": "https://a.example/users/alice",
    "content": "<p>Hello, world!</p>",
    "published": "2026-04-13T12:00:00Z",
    "to": ["https://www.w3.org/ns/activitystreams#Public"],
    "cc": ["https://a.example/users/alice/followers"]
  }
}
```

**リプライ + メンション付きの例:**

```json
{
  "type": "Create",
  "actor": "https://a.example/users/alice",
  "object": {
    "type": "Note",
    "id": "https://a.example/users/alice/posts/789",
    "attributedTo": "https://a.example/users/alice",
    "inReplyTo": "https://b.example/users/bob/posts/100",
    "content": "<p><span class=\"h-card\"><a href=\"https://b.example/@bob\" class=\"u-url mention\">@<span>bob</span></a></span> いいね！</p>",
    "published": "2026-04-13T12:30:00Z",
    "to": ["https://www.w3.org/ns/activitystreams#Public"],
    "cc": [
      "https://a.example/users/alice/followers",
      "https://b.example/users/bob"
    ],
    "tag": [
      {
        "type": "Mention",
        "href": "https://b.example/users/bob",
        "name": "@bob@b.example"
      }
    ]
  }
}
```

### Note の主要フィールド

- `content` — HTML。[fediverse] XSS 対策として DOMPurify 等でサニタイズ必須
- `contentMap` — 言語タグ付きコンテンツ (`{ "en": "Hello", "ja": "こんにちは" }`)
- `summary` — CW (Content Warning) テキスト。設定するとクライアントは content を折りたたんで表示する
- `sensitive` — センシティブフラグ (boolean)。メディアをぼかし表示する。summary 設定時は通常 true
- `to` / `cc` — 公開範囲
- `published` — RFC 3339 形式
- `attributedTo` — 投稿者の Actor URI
- `inReplyTo` — 返信先の Note URI (または null)
- `tag` — Mention / Hashtag 等の構造化タグの配列

**[仕様] Outbox 送信時 (6.2):**
- object の `attributedTo` に actor をコピー
- Create と object 間のアドレス指定 (to/cc) の一致を推奨

**[仕様] Inbox 受信時 (7.2):**
- Activity と埋め込み object をローカルに保存

**murlog の実装:**
- 受信時: URI ベースの冪等性チェック（重複排除）→ HTML サニタイズ → DB 保存
- リプライ受信時: `inReplyTo` を DB に保存。返信先がローカル投稿ならスレッド関係を構築
- メンション受信時: `tag` 内の Mention の `href` が自ペルソナの Actor URI と一致すれば `mention` 通知を生成
- CW 受信時: `summary` と `sensitive` を DB に保存。フロントエンドで折りたたみ表示
- CW 送信時: Post に `summary` があれば Note の `summary` + `sensitive` フィールドとして配送

## リプライ

**[仕様] Activity Vocabulary — `inReplyTo`:**
- 任意のオブジェクトが持てるプロパティ（Note 固有ではない）
- 値は URI or 埋め込みオブジェクト。複数指定可能だが fediverse では単一 URI が慣習

**[fediverse] 各実装の挙動:**
- 返信先 Note の URI を `inReplyTo` に設定
- 返信先の Actor を `cc` に含める（フォロー関係がなくても相手の Inbox に届くようにする）
- 返信先がリモートの場合、サーバーは `inReplyTo` の URI を fetch してスレッドを構築する実装が多い

**murlog の実装:**
- 送信: `inReplyTo` に返信先の投稿 URI を設定。返信先がローカル投稿なら `https://{domain}/users/{username}/posts/{id}`、リモートなら元の URI をそのまま使用
- 送信: 返信先 Actor の URI を `cc` に追加して配送（フォロワー配送 + 返信先 Actor の Inbox への配送）
- 受信: `inReplyTo` を DB の `in_reply_to` カラムに保存。返信先がローカル投稿なら投稿 ID でも参照可能にする

## スレッド

投稿の詳細画面で、祖先（返信先チェーンを遡る）と子孫（返信ツリー）を表示する。

**[fediverse] スレッド取得の一般的なアプローチ:**
- Mastodon: `/api/v1/statuses/:id/context` で ancestors + descendants を返す。リモート投稿は手元にあるデータの範囲で構築
- GoToSocial: 同様の context API。未取得の投稿はスレッドに含まれない
- `conversation` / `context` プロパティは仕様上存在するが、fediverse ではスレッド構築に `inReplyTo` チェーンを辿る方式が主流

**murlog の実装:**
- 祖先取得: `in_reply_to` を再帰的に辿ってチェーンを構築。リモート投稿は DB にあるもののみ（未取得の中間ノードがあるとそこで途切れる）
- 子孫取得: `in_reply_to` が対象投稿 URI に一致する投稿を再帰的に収集
- リモートスレッドの fetch は行わない（手元のデータ範囲で表示する方針）

## メンション

**[仕様] Activity Vocabulary — `tag` + `Mention`:**
- `tag` は任意のオブジェクトに付けられる構造化メタデータの配列
- `Mention` は `Link` のサブタイプ (Activity Vocabulary 4.2)
  - `href` (必須) — メンション先 Actor の URI
  - `name` (推奨) — `@user@domain` 形式の表示名

**[fediverse] メンションの慣習:**
- `content` (HTML) 内にメンションリンクを含める（人間が読める形）
- `tag` 配列に `Mention` オブジェクトを含める（機械処理用）
- メンション先 Actor を `cc` に追加する（配送先として確実に届ける）
- Mastodon の HTML 形式: `<span class="h-card"><a href="{profile_url}" class="u-url mention">@<span>{username}</span></a></span>`
- メンション先の解決: 投稿作成時に `@user@domain` を WebFinger で解決し、Actor URI を取得

**murlog の実装:**
- 送信: 投稿本文の `@user@domain` を WebFinger で解決 → Actor URI を取得 → `tag` に Mention として追加 + `cc` に Actor URI を追加 + HTML 内にリンクを生成
- 受信: `tag` 内の Mention の `href` と自ペルソナの Actor URI を照合 → 一致すれば `mention` 通知を生成
- 受信: `content` 内のメンションリンクはサニタイズ後にそのまま表示（HTML として保持）

## Undo Like / Undo Announce

投稿へのリアクション（お気に入り・リブログ）の取り消し。

**murlog の実装:**
- Undo Like 受信: 対象投稿の favourite レコードを削除
- Undo Announce 受信: 対象投稿の reblog レコードを削除
- 送信: Undo Like / Undo Announce をワーカーが配送
