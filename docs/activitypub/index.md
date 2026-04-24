# ActivityPub 仕様メモ

murlog が実装する ActivityPub 関連プロトコルの仕様まとめ。
W3C 勧告の正式な定義と、実際の fediverse で求められる挙動を区別して記載する。

> **凡例:** [仕様] = W3C/RFC/Draft の規定。[fediverse] = Mastodon/GoToSocial 等の実装で事実上必要な挙動。

## 参照仕様

| 仕様 | URL |
|------|-----|
| ActivityPub (W3C Recommendation) | https://www.w3.org/TR/activitypub/ |
| Activity Streams 2.0 Core | https://www.w3.org/TR/activitystreams-core/ |
| Activity Vocabulary | https://www.w3.org/TR/activitystreams-vocabulary/ |
| HTTP Signatures (draft-cavage-http-signatures) | https://datatracker.ietf.org/doc/html/draft-cavage-http-signatures |
| WebFinger (RFC 7033) | https://datatracker.ietf.org/doc/html/rfc7033 |
| NodeInfo | https://nodeinfo.diaspora.software/protocol |

## ページ構成

- [Actor](./actor.md) — Actor オブジェクト、エンドポイント、Collections、公開範囲
- [Activity](./activity/) — 共通構造、Inbox、サーバー間配送
  - [Follow](./activity/follow.md) — Follow / Accept / Undo Follow
  - [Create](./activity/create.md) — Create/Note、リプライ・スレッド・メンション、Undo Like/Announce
  - [Delete](./activity/delete.md) — Delete（投稿・Actor の削除）
  - [Block](./activity/block.md) — Block / Undo Block
- [HTTP Signature](./http-signature.md) — 署名生成・検証、対応アルゴリズム、鍵ペア
- [WebFinger・NodeInfo](./webfinger.md) — WebFinger (RFC 7033)、NodeInfo 2.0
- [fediverse 互換性](./compatibility.md) — Mastodon / GoToSocial / Misskey の実装差異と互換性メモ
