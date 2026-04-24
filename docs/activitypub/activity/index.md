# Activity

Actor が行うアクション。JSON-LD で表現し、Inbox/Outbox で送受信する。

> **凡例:** [仕様] = W3C/RFC/Draft の規定。[fediverse] = Mastodon/GoToSocial 等の実装で事実上必要な挙動。

## 共通構造

```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "https://example.com/users/alice#activity/123",
  "type": "Follow",
  "actor": "https://example.com/users/alice",
  "object": "https://remote.example/users/bob"
}
```

[仕様] Activity Streams 2.0 Core では**全プロパティがオプション**と定義されている（`id` や `type` 含む）。ただし実用上は以下が必要:
- `id` — Activity の一意 URI。Undo で参照するため必要
- `type` — Activity の種類を識別
- `actor` — Activity を行う Actor の URI
- `object` — Activity の対象。URI or 埋め込みオブジェクト

[仕様] `@context` は `https://www.w3.org/ns/activitystreams` への参照を含むべき (SHOULD)。他の context 定義で拡張してもよいが、規範的 context をオーバーライドしてはならない。

### Undo（共通仕様）

**[仕様] (6.10, 7.12):**
- 同一 actor で実行されなければならない
- 副作用を「取り消し可能な限度で」実行
- Create の取り消しには Delete の使用を推奨

各 Type 固有の Undo 処理はそれぞれのページを参照。

## Activity Type 一覧

- [Follow](./follow.md) — Follow + Accept + Undo Follow
- [Create](./create.md) — Create/Note + リプライ・スレッド・メンション + Undo Like/Announce
- [Announce](./announce.md) — Announce (リブログ) + リモート Note フェッチ + Undo Announce
- [Delete](./delete.md) — Delete（投稿・Actor の削除）
- [Block](./block.md) — Block + Undo Block

## Inbox

**[仕様] (5.2):**
- 全 Activity を受信する OrderedCollection
- GET: リクエスターの権限に応じてコンテンツをフィルタリング
- POST: フェデレーションサーバーからのみ受け入れ（非フェデレーションは 405 応答）
- **重複排除**: Activity の `id` を比較して既見の Activity を除外する

**[仕様] Content-Type:**
- `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`
- `application/activity+json` は上記と同等として扱う

**murlog の Inbox 処理フロー:**
1. HTTP Signature 検証（Digest ヘッダー + 署名検証）
2. **ブロックチェック**: `activity.actor` の Actor URI / ドメインがブロック済みなら 202 返却で終了
3. Activity Type に応じた処理（Follow / Like / Announce / Block / Create / Delete / Update / Undo / Accept / Reject）

## サーバー間配送

**[仕様] (7.1):**
1. `to`, `bto`, `cc`, `bcc`, `audience` フィールドを確認
2. Collection の場合はユーザー認可情報で逆参照し、各 item の inbox を検出
3. コレクション間接参照の層数制限を実装すべき
4. 最終受信者リストから重複を排除
5. Activity の actor と同一の Actor を除外
6. `bto` と `bcc` は配送前に除去

**Shared Inbox (7.1.3):**
- Public 対象の場合、既知のすべての sharedInbox に配送可能
