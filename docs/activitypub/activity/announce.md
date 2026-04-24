# Announce (リブログ)

投稿のリブログ。

## 受信

### ローカル投稿のリブログ

リモート Actor が自分のローカル投稿をリブログした場合。

- `resolveLocalPost` で対象投稿を特定
- `reblogs` テーブルにレコード作成（UNIQUE 制約で冪等）
- `notifications` テーブルに `type="reblog"` 通知を作成

### リモート投稿のリブログ

フォロー中ユーザが第三者の投稿をリブログした場合。

1. `GetPostByURI` で冪等性チェック（既存なら 202 返却）
2. `FetchNoteSigned` で元 Note を署名付き HTTP GET で取得
3. `storeRemoteNote` で `origin="remote"` の投稿として保存
   - `reblogged_by_uri` にリブログ元 Actor URI をセット
   - `actor_uri` には元投稿者（attributedTo）の URI をセット
4. `ensureRemoteActorCached` で投稿者・リブログ元の Actor 情報をフェッチ+キャッシュ

### 表示

- API レスポンスの `reblogged_by_*` フィールドでリブログ元情報を返す
- `reblogged_by_uri` がある投稿にのみリブログラベルを表示
- `reblogged_by_name` / `reblogged_by_acct` は `remote_actors` テーブルから解決

## 送信

ローカルユーザが投稿をリブログした場合。

- `reblogs.create` RPC → `send_announce` ジョブ → ワーカーが Announce Activity を配送
- 配送先: 全フォロワー + 元投稿者

## Undo Announce

- 受信: 対象投稿の reblog レコードを削除
- 送信: `reblogs.delete` RPC → `send_undo_announce` ジョブ → ワーカーが Undo(Announce) を配送
