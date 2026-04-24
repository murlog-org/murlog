# Block

Actor のブロック。受信側はフォロー関係を解消し、以降のインタラクションを拒否する。

> **凡例:** [仕様] = W3C/RFC/Draft の規定。[fediverse] = Mastodon/GoToSocial 等の実装で事実上必要な挙動。

## Block

```json
{
  "type": "Block",
  "actor": "https://a.example/users/alice",
  "object": "https://b.example/users/bob"
}
```

**[仕様] (6.9, 7.9):**
- Outbox: object のサーバーに配送する。object の Actor がブロックした Actor の Activity にアクセスできないよう副作用を実行する
- Inbox: 実質的なフォロー関係の解消通知として機能

**[fediverse] Mastodon/GoToSocial の挙動:**
- 受信時: 双方向のフォロー関係を解消（フォロー + フォロワー両方を削除）
- ブロックした事実のレコードは受信側では保存しない（再フォローは送信側が拒否する）
- Undo Block を受信しても特に処理なし

**murlog の実装:**
- 送信: `blocks.create` RPC → ブロックレコード作成 + 双方向フォロー削除 + Block Activity 配送
- 受信: 双方向のフォロー関係を削除。ブロックレコードは保存しない
- ブロック済み Actor / ドメインからの Activity は Inbox 入口で拒否（202 返却、処理しない）
- ドメインブロック: Activity 配送なし（サイレント拒否）。アクターブロックとは別テーブルで管理

## Undo Block

ブロック解除。

**murlog の実装:**
- 受信: 特に処理なし（受信 Block でレコードを保存していないため）
- 送信: ブロックレコード削除 + Undo Block をワーカーが配送
