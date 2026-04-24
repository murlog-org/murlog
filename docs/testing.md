# テスト

## テスト戦略

テストを 4 レイヤに分離し、速度とカバレッジを両立する。

| レイヤ | 何をテスト | バックエンド | ブラウザ | 速度 |
|---|---|---|---|---|
| **Go httptest** | JSON-RPC API・セットアップフロー | `httptest` + SQLite `:memory:` | 不要 | 爆速 (~1s) |
| **Vitest** | フロントエンドユーティリティ・ロジック | 不要 (jsdom) | 不要 | 爆速 (~0.7s) |
| **Playwright e2e** | UI 操作・API 結合 | 実サーバー | 要 | 遅い (~30s) |
| **連合テスト** | ActivityPub 送受信 (対向サーバー) | Docker Compose | 不要 | 最遅 (~数分) |

## テストファイル一覧

### handler/ — Go httptest

| ファイル | テスト関数 | 件数 | 内容 |
|---|---|---|---|
| `testutil_test.go` | — | — | 共通ヘルパー (`setupTestEnv`, `rpcCall`, `loginTestEnv` 等) |
| `rpc_integration_test.go` | `TestRPCIntegration` | 83 | JSON-RPC API フルフロー (認証 → Personas → Posts → CW → Pin → Visibility → Favourites/Reblogs+カウント → Blocks → Domain Blocks → Media → Queue → 承認制フォロー → TOTP → Logout) |
| `rpc_integration_test.go` | `TestSetupFlow` | 7 | セットアップフロー (ini 未生成 → Step 1 → Step 2 → 完了リダイレクト) |
| `activitypub_test.go` | `TestActorEndpoint` 他 | 29 | ActivityPub: Actor (discoverable)/WebFinger/Inbox (Follow/Follow Locked/Undo/Create Note/Duplicate/Mention Tag/Reply+Mention/署名拒否(unsigned/invalid/tampered/missing-digest)/Block/Domain Block/Truncate)/Like/Undo Like/Announce/Undo Announce/Announce Remote/Collections (Outbox/Followers/Following + Hidden) |
| `visibility_test.go` | `TestDetectVisibility` | 1 | detectVisibility: テーブル駆動で public/unlisted/followers/direct 判定 |
| `rpc_test.go` | `TestTimelineHome` 他 | 8 | Timeline/Follows/Followers/Notifications CRUD/Batch limit/Content too long/Enrich equivalence/ValidateCursorHost |

### パッケージ単体テスト

| ファイル | 件数 | 内容 |
|---|---|---|
| `activitypub/signature_test.go` | 12 | HTTP Signature 署名・検証・改竄検出・鍵ペア生成・Ed25519・期限切れ・ヘッダーパース・プライベート IP・Digest 不一致・署名必須ヘッダー欠如・非対応 Digest アルゴリズム |
| `config/config_test.go` | 8 | 設定デフォルト値・ファイル読込・不在ファイル・環境変数・保存/再読込・パーミッション・Exists・MainDBPath |
| `hashtag/hashtag_test.go` | 2 | ハッシュタグパース (テーブル駆動)・HTML 変換 |
| `i18n/i18n_test.go` | 2 | 翻訳読込・言語検出 |
| `id/id_test.go` | 7 | UUIDv7 生成・時刻抽出・文字列変換・パース・バイト列・順序性 |
| `totp/totp_test.go` | 3 | 秘密鍵生成・TOTP 検証・URI 生成 |
| `store/sqlite/login_attempt_test.go` | 1 | Record/Get/Clear + ロック検証 |
| `store/sqlite/sqlite_test.go` | 23 | Persona/Post/Follow/Follower/ListFollowersPaged/RemoteActor/Session/Setting/Attachment/Notification/OAuth/Favourite+Reblog/PublicPosts/URI 検索/RebloggedByURI/HasFavourited+HasReblogged/ListAttachmentsByPosts/GetPostsByURIs/ドメイン検証/DomainFromURI/ScanHelper/ConcurrentOpen/カウンターキャッシュ |
| `queue/sqlqueue/sqlqueue_test.go` | 11 | Enqueue+Claim/Complete/Fail+Retry/ClaimOrder/HasPending/Cleanup/EnqueueBatch/ParseTime/Dead/CleanupDeletesDead/CancelByDomainWithPort |
| `worker/worker_test.go` | 12 | AcceptFollow/DeliverPost/DeliverDelete/SendUndoFollow/DeliverAnnounce/リトライ/MaxAttempts/RunOnce/バッチ limit+timeout/並行性/並列度判定 |
| `media/fs/fs_test.go` | 6 | ファイル保存・削除・URL 生成・パストラバーサル防止・サブディレクトリ作成・エラー時クリーンアップ |
| `media/s3/sigv4_test.go` | 8 | AWS Signature V4 署名・Body 付き署名・導出鍵・SHA256・クエリ文字列・一貫性・URI エンコーディング・クエリエンコーディング |
| `media/s3/s3_integration_test.go` | 1 | S3 互換ストレージ結合テスト |
| `media/imageproc/strip_test.go` | 6 | EXIF 除去 (Normal/Rotated)・EXIF なし・PNG 素通り・APP0/APP2 保存・不正セグメント長 |
| `mention/mention_test.go` | 4 | @user@domain パース (テーブル駆動)・HTML 変換・IsSafeURL・JavaScript URL 拒否 |

### Vitest — フロントエンド単体テスト

| ファイル | 件数 | 内容 |
|---|---|---|
| `web/src/lib/api.test.ts` | 5 | isUnauthorized 判定・エラーコード定数 |
| `web/src/lib/format.test.ts` | 10 | formatTime: 相対時刻 (秒/分/時/日)・古い日付・空文字列/不正入力・境界値 |
| `web/src/lib/sanitize.test.ts` | 15 | HTML サニタイズ: 安全タグ許可・リンク属性・script/img/iframe/style 除去・イベントハンドラ・ネスト |
| `web/src/lib/theme.test.ts` | 11 | autoLinkText: URL → `<a>` 変換・プレーンテキスト・プロトコルなし・HTML エスケープ・Unicode ドメイン |
| `web/src/pages/my.test.ts` | 9 | acctFromURI: Mastodon/GTS/ポート付き/ネストパス/末尾スラッシュ/不正 URL |
| `web/src/pages/queue.test.ts` | 10 | extractHost: actor_uri/target_actor_uri/target_uri 抽出・優先順位・空/不正入力・ポート/サブドメイン |

### e2e — Playwright

| ファイル | 件数 | 内容 |
|---|---|---|
| `e2e/tests/auth.spec.ts` | 4 | ログイン失敗・パスワード変更・TOTP フルサイクル (setup → verify → login → disable) |
| `e2e/tests/follow.spec.ts` | 4 | 検索 UI (エラー表示)・タブ切替・空状態・RPC 契約検証 |
| `e2e/tests/media.spec.ts` | 4 | REST アップロード・投稿添付・メディア削除 |
| `e2e/tests/notifications.spec.ts` | 5 | 通知一覧・全既読・空状態 UI・ポーリング |
| `e2e/tests/post-features.spec.ts` | 5 | 返信 (in_reply_to)・ピン留め/解除・CW 折りたたみ・公開範囲 |
| `e2e/tests/spa-my.spec.ts` | 11 | ログイン → 投稿 CRUD → ブロック管理 → いいね・リブログ → リモート/ローカルプロフィール → ログアウト |
| `e2e/tests/infinite-scroll.spec.ts` | 3 | 無限スクロール: 25件投稿 → 初回20件 → スクロールで全件表示 |

## 実行方法

```bash
# Go テスト (全パッケージ, 145 件)
cd murlog && go test ./...

# handler テストのみ
go test ./handler/ -v

# 特定テストだけ実行
go test ./handler/ -run TestRPCIntegration -v
go test ./handler/ -run TestSetupFlow -v

# Vitest フロントエンド単体テスト (60 件)
cd web && npx vitest run

# e2e テスト (Playwright, 36 件)
make e2e

# Apache CGI モードで e2e
make docker-cgi-test
```

## テスト環境のヘルパー

`handler/testutil_test.go` に共通ヘルパーを集約している。

### setupTestEnv

セットアップ完了状態のテスト環境を構築する。

- SQLite `:memory:` + マイグレーション済み
- `setup_complete = true`, `domain = murlog.test` 設定済み
- alice ペルソナ (primary) + RSA 鍵ペア作成済み
- `nopMedia` (no-op メディアストア)
- `httptest.NewServer` で HTTP サーバー起動
- SSRF 保護を無効化 (httptest は 127.0.0.1)

```go
env := setupTestEnv(t)
// env.server.URL → "http://127.0.0.1:xxxxx"
// env.store      → SQLite :memory:
// env.persona    → alice
```

### setupRawTestEnv

セットアップ前状態のテスト環境を構築する。`TestSetupFlow` 用。

- ini ファイル未作成 → `setupPhase() == SetupStep1`
- `setup_complete` 未設定
- ペルソナ未作成

### RPC ヘルパー

| ヘルパー | 用途 |
|---|---|
| `rpcCall(t, env, method, params, &result)` | JSON-RPC 呼び出し。エラー時は `t.Fatalf` |
| `rpcCallWithCookie(t, env, method, params, &result, cookie)` | Cookie 付き呼び出し |
| `rpcCallRaw(t, env, method, params, cookie)` | 生レスポンス返却 (エラーコード検証用) |
| `loginTestEnv(t, env, password)` | パスワード設定 + ログイン → Cookie 返却 |
| `loginRPC(t, env, params)` | auth.login RPC + Cookie 取得 (TOTP 対応) |

### テストの直列依存

`TestRPCIntegration` は `t.Run()` サブテストで直列実行し、`personaID` / `postID` 等の共有状態をクロージャの変数で管理する。

```go
func TestRPCIntegration(t *testing.T) {
    env := setupTestEnv(t)
    cookie := loginTestEnv(t, env, "test1234")

    var personaID string

    t.Run("Personas", func(t *testing.T) {
        t.Run("personas.list", func(t *testing.T) {
            var personas []personaJSON
            rpcCallWithCookie(t, env, "personas.list", nil, &personas, cookie)
            personaID = personas[0].ID  // 後続テストで使用
        })
        // ...
    })
}
```
