# Follow

フォロー関連の Activity。Follow / Accept / Reject / Undo Follow を含む。

> **凡例:** [仕様] = W3C/RFC/Draft の規定。[fediverse] = Mastodon/GoToSocial 等の実装で事実上必要な挙動。

## Follow

フォローリクエスト。受信側は Accept or Reject で応答する。

```json
{
  "type": "Follow",
  "actor": "https://a.example/users/alice",
  "object": "https://b.example/users/bob"
}
```

**[仕様] Inbox 受信時 (7.5):**
- Accept または Reject Activity を生成し配送する
- Accept 受信: actor を Followers コレクションに追加
- Reject 受信: 追加しない

**[仕様] Outbox 送信時 (6.5):**
- Accept Activity 受信後に object を Following コレクションに追加

**murlog の実装:**
- 送信: `follows.create` RPC → ワーカーが Actor fetch → Inbox に POST
- 受信: フォロワーレコード作成 + Accept 配送ジョブ登録

## Accept

Follow に対する承認応答。object に元の Follow Activity (の URI or オブジェクト) を含む。

```json
{
  "type": "Accept",
  "actor": "https://b.example/users/bob",
  "object": {
    "type": "Follow",
    "id": "https://a.example/users/alice#follows/123",
    "actor": "https://a.example/users/alice",
    "object": "https://b.example/users/bob"
  }
}
```

**[仕様] Inbox 受信時 (7.6):**
- object の型に応じて副作用を決定
- Follow の Accept: actor を Following コレクションに追加

**murlog の実装:**
- 送信: Follow 受信時にワーカーが自動送信
- 受信: Accept の `actor` が Follow の `target_uri` と一致する Follow を検索し、`accepted = true` に更新

## Undo Follow

フォロー解除。

```json
{
  "type": "Undo",
  "actor": "https://a.example/users/alice",
  "object": {
    "type": "Follow",
    "actor": "https://a.example/users/alice",
    "object": "https://b.example/users/bob"
  }
}
```

**murlog の実装:**
- 受信: `actor` に一致するフォロワーを削除
- 送信: Undo Follow をワーカーが配送
