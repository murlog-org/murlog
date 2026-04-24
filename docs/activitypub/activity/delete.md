# Delete

オブジェクトまたは Actor の削除。

> **凡例:** [仕様] = W3C/RFC/Draft の規定。[fediverse] = Mastodon/GoToSocial 等の実装で事実上必要な挙動。

## Delete

```json
{
  "type": "Delete",
  "actor": "https://a.example/users/alice",
  "object": "https://a.example/users/alice/posts/456"
}
```

**[仕様] (6.4, 7.4):**
- Outbox: object を Tombstone に置換可能。削除後のリクエストには 410 Gone または 404 を返す
- Inbox: 受信サーバーは object を除去または Tombstone に置換

**murlog の実装:**
- object が投稿 URI → 該当投稿を削除
- object が Actor URI (= actor と同一) → そのアカウントのフォロワーレコードを全削除
