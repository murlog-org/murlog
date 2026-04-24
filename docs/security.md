# セキュリティ

murlog のセキュリティ設計と実装方針をまとめる。認証フローの詳細は [auth.md](./auth.md)、ActivityPub 署名の仕様は [HTTP Signature](./activitypub/http-signature.md) を参照。

## 脅威モデル

murlog は一人用の ActivityPub サーバーであり、ログインする人間は 1 人。ただし以下の外部接点を持つ：

| 接点 | 脅威 |
|------|------|
| ActivityPub Inbox | 悪意あるリモートサーバーからの Activity 受信 |
| WebFinger / Actor 取得 | 外部 URL へのフェッチ (SSRF) |
| ログイン | ブルートフォース |
| 投稿・プロフィール表示 | XSS（ローカル / リモート投稿の HTML） |
| ファイルアップロード | 不正ファイル、パストラバーサル |

## 認証・セッション

| 項目 | 実装 |
|------|------|
| パスワード保管 | bcrypt ハッシュ (`settings.password_hash`) |
| セッショントークン生成 | `crypto/rand` 32 バイト → hex (256-bit エントロピー) |
| トークン保管 | SHA-256 ハッシュを DB に保存（平文は Cookie のみ） |
| Cookie フラグ | `HttpOnly`, `SameSite=Strict`, `Secure`（`protocol` 設定が `https` 時。リバースプロキシ対応） |
| セッション有効期間 | 14 日 |
| Bearer トークン | API トークン (`api_tokens` テーブル)、同じく SHA-256 ハッシュで保管 |

### 実装済みの保護

- **レートリミット**: `auth.login` に IP 別レートリミット (5 回失敗 → 5 分ロック、`login_attempts` テーブル、`trusted_proxy` 対応)
- **CSRF 保護**: セットアップ・リセットフォームに Double Submit Cookie パターン
- **TOTP 2FA**: 設定画面から TOTP セットアップ可能 (RFC 6238, HMAC-SHA1, 前後 1 ウィンドウ)
- **HTML サニタイズ**: bluemonday でリモート投稿を保存時 + SSR 出力時にサニタイズ (多層防御)。SPA は DOMPurify
- **セットアップトークン**: `MURLOG_SETUP_TOKEN` 環境変数 or `murlog.setup-token` ファイルでオプトイン認証
- **パスワード制約**: 8-128 文字、4 種の文字種（小文字・大文字・数字・記号）のうち 3 種以上
- **API トークン有効期限**: `expires_at` カラムで期限チェック (NULL = 無期限)

## HTTP レスポンスヘッダ

`ServeHTTP` の先頭で以下のセキュリティヘッダを設定する（`handler/handler.go`）。

| ヘッダ | 値 | 目的 |
|--------|------|------|
| `X-Content-Type-Options` | `nosniff` | MIME スニッフィング防止 |
| `X-Frame-Options` | `DENY` | クリックジャッキング防止 |
| `Referrer-Policy` | `same-origin` | リファラ漏洩防止 |
| `Content-Security-Policy` | `default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' https:; media-src 'self' https:` | XSS 緩和・リソース読込制限 (Handlebars テンプレートに `unsafe-eval` 必要) |
| `Strict-Transport-Security` | `max-age=31536000`（HTTPS 時のみ） | HTTPS 強制 (HSTS) |

## SQL インジェクション対策

全 DB 操作がパラメタライズドクエリ（`?` プレースホルダ）を使用。

- **動的 IN 句**: `placeholders[i] = "?"` でプレースホルダを生成し、値は引数で渡す（`store/sqlite/attachment.go`）
- **VACUUM INTO**: パラメタライズド非対応のため、パス文字列にシングルクォートが含まれないことを検証（`store/sqlite/sqlite.go`）。呼び出し元は内部生成パスのみ
- **動的テーブル名・カラム名**: なし。全てコンパイル時定数
- **LIKE クエリ**: 使用箇所なし

## SSRF 対策

リモート Actor 取得・WebFinger・Activity 配送で外部 URL にアクセスする。`activitypub/resolve.go` で SSRF を防止。

### ssrfSafeDialer

全連合 HTTP リクエストは `activitypub.HTTPClient` を経由し、カスタム `DialContext` でプライベート IP をブロックする。

**ブロック対象:**

| チェック | ブロックされる範囲 |
|----------|-------------------|
| `ip.IsLoopback()` | `127.0.0.0/8`, `::1` |
| `ip.IsPrivate()` | `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16` |
| `ip.IsLinkLocalUnicast()` | `169.254.0.0/16`（AWS メタデータサービス等） |
| `ip.IsLinkLocalMulticast()` | `224.0.0.0/4` |
| `ip.IsUnspecified()` | `0.0.0.0`, `::` |

**DNS 解決後チェック**: `net.DefaultResolver.LookupIPAddr` で DNS 解決し、返された**全 IP** を検証してからダイヤル。DNS リバインディング攻撃を緩和。

### レスポンスサイズ制限

| エンドポイント | 制限 |
|---------------|------|
| Actor 取得 | 1 MB |
| WebFinger | 256 KB |
| Inbox ボディ | 1 MB |

## HTTP Signature 検証

ActivityPub Inbox で受信する全リクエストに対し、3 段階の検証を行う（`handler/activitypub.go`）。

```
1. Digest 検証  — ボディの SHA-256 が Digest ヘッダと一致するか
2. 署名検証     — 送信者の公開鍵で HTTP Signature を検証
3. Actor 一致   — 署名者の keyId から導出した Actor URI が activity.actor と一致するか
```

### 署名検証の詳細（`activitypub/signature.go`）

| 項目 | 仕様 |
|------|------|
| 対応アルゴリズム | `rsa-sha256`, `ed25519`, `hs2019` |
| algorithm 検証 | 指定時は許可リストでチェック、未指定は鍵種別から推論 |
| 必須署名ヘッダ | `date`（Mastodon 互換） |
| 時刻検証 | Date ヘッダが現在時刻から ±13 時間以内（`EXPIRATION_WINDOW_LIMIT` 12h + `CLOCK_SKEW_MARGIN` 1h） |
| 公開鍵取得 | 署名付き GET で Actor をフェッチ（GoToSocial の Authorized Fetch 対応） |
| keyId パース | `#main-key`, `/main-key`, `/publickey` 形式に対応 |

## XSS 対策

### 現状

| レンダリング経路 | サニタイズ |
|-----------------|-----------|
| SPA (Preact) | DOMPurify でタグ・属性ホワイトリスト方式（`web/src/lib/sanitize.ts`） |
| SSR (Go テンプレート) | bluemonday でタグ・属性ホワイトリスト方式（`handler/ssr.go` `sanitizeHTML`） |

SSR 出力は `sanitizeHTML()` → `template.HTML()` の順で処理する。bluemonday のポリシーは SPA 側の DOMPurify 許可リストと同等のホワイトリストを定義し、多層防御を実現している。

対象:
- 投稿 `content`（ローカル作成 + リモート受信）
- ペルソナ `summary`
- リモート Actor の `summary`

## ファイルアップロード

メディアアップロード（`handler/api_mur_media.go`）のセキュリティ制御。

| 項目 | 実装 |
|------|------|
| 認証 | セッション Cookie / Bearer トークン必須 |
| サイズ制限 | 10 MB (`maxMediaSize`) |
| MIME ホワイトリスト | `image/jpeg`, `image/png`, `image/gif`, `image/webp` |
| プレフィックス検証 | `attachments`, `avatars`, `headers` のみ許可 |
| EXIF 除去 | JPEG APP1 セグメントをバイナリ除去（GPS 等の個人情報）。Orientation・ICC プロファイルは保持 |
| 画質劣化 | なし（再エンコードしない） |

## パストラバーサル対策

静的ファイル配信（`handler/handler.go` `serveSPA`）とメディア配信（`handler/api_mur_media.go` `serveMedia`）で対策。

```go
filePath := filepath.Join(baseDir, filepath.Clean(r.URL.Path))
if !strings.HasPrefix(filePath, filepath.Clean(baseDir)+string(filepath.Separator)) {
    http.NotFound(w, r)
    return
}
```

`filepath.Clean` で `..` を正規化し、結果が `baseDir` 配下であることを prefix チェックで確認。

## .htaccess による機密ファイル保護

CGI モード起動時に `.htaccess` を自動生成。以下のファイルへのブラウザアクセスをブロック:

- `*.ini`（設定ファイル）
- `*.db`（データベース）
- `*.reset`（パスワードリセットトークン）
