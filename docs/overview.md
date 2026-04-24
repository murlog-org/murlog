# murlog プロダクト概要

## 概要

murlog は一人用の ActivityPub マイクロブログサーバー。自分のドメインで自分だけのインスタンスを持ち、fediverse の他のサーバーと繋がる。

## できること

- **自分のドメインで投稿する** — `@you@yourdomain.com` が自分のアドレスになる
- **Mastodon や Misskey のユーザーとやりとりする** — フォロー、いいね、リブログ、返信
- **画像を添付して投稿する** — EXIF 自動除去、HEIC→JPEG 変換対応
- **投稿に公開範囲を設定する** — 公開・未収載・フォロワー限定・CW (Content Warning)
- **スマホから使う** — レスポンシブ対応の Web UI
- **二要素認証でアカウントを守る** — TOTP 対応
- **Mastodon クライアントアプリから使う** — Ivory、Ice Cubes 等の既存アプリに対応 (v1 予定)
- **複数のペルソナを使い分ける** — 1つのサーバーで複数のアカウントを運用 (v2 予定)
- **テーマで見た目を変える** — 公開ページを Handlebars テンプレートでカスタマイズ (v2 予定)
- **ブログとしても使う** — 長文記事の投稿・管理 (v2 予定)

## 特徴

- **一人用特化** — マルチユーザー管理やモデレーション機能を持たない。一人で使う前提でシンプルに設計
- **Go シングルバイナリ + SQLite** — 外部サービス不要。Redis・PostgreSQL・nginx なしで動作する
- **CGI 対応** — レンタルサーバー (さくら・Xserver 等) にバイナリを置くだけでデプロイ可能。月100円台から運用できる
- **ActivityPub 連合** — Mastodon・Misskey・GoToSocial 等と相互にフォロー・投稿・リアクションできる

技術選定・アーキテクチャの詳細は [architecture.md](architecture.md) を参照。

## 機能

### コア
- テキスト投稿・編集・削除
- 画像添付 (EXIF 除去、HEIC→JPEG 変換)
- S3 互換ストレージ (Cloudflare R2 等)
- ペルソナ（複数 Actor）
- フォロー/フォロワー管理（承認制フォロー対応）
- いいね・リブログ送受信
- メンション送受信
- CW (Content Warning) / フォロワー限定投稿
- ピン留め投稿
- カスタムフィールド
- ハッシュタグ (タグ別一覧、公開タグページ)
- 通知一覧
- ブロック / ドメインブロック
- 多言語対応（日本語 / English）

### ActivityPub 互換性
- WebFinger / Actor / NodeInfo
- HTTP Signature 署名・検証（RSA + Ed25519 検証対応）
- Digest ヘッダー送受信検証
- Inbox: Follow / Undo Follow / Accept / Reject / Create / Delete / Like / Announce / Undo Like / Undo Announce / Update / Block
- Collections 実データ（Outbox / Followers / Following / Featured）
- Actor discoverable フラグ

### フロントエンド
- マイページ SPA（タイムライン、フォロー管理、通知、設定、キュー管理、DB バックアップ）
- 公開プロフィール + 投稿ページ（SSR + OGP + Handlebars テーマ）
- リモートユーザープロフィール表示
- 内部投稿パーマリンク（スレッドツリー + リアクション）
- 無限スクロール
- TOTP 二要素認証

### セキュリティ
- CSRF 保護
- ログインレートリミット（DB ベース、CGI 対応）
- HTML サニタイズ 3 層（保存時 + SSR + SPA）
- コンテンツ長制限（ローカル 3000 文字 / リモート受信 100K）
- リモートデータ配列上限（contentMap / tag / attachment / actor fields 等）
- SSRF 対策（DNS 解決後 IP チェック）
- 添付 URL スキーム検証
- httpoxy 対策 (CVE-2016-5385)
- Content Negotiation キャッシュ分離（Vary: Accept）

### 運用
- CGI / serve 両対応
- ジョブキュー（リクエスト駆動 + アダプティブ並列処理）
- 死亡サーバー判定（ドメイン別サーキットブレーカー）
- 完了ジョブクリーンアップ + VACUUM
- 検索クローラ・AI クローラー設定（robots.txt + meta robots）
- DB バックアップ（マイページからダウンロード）
- 起動時自動マイグレーション（マイグレーション前バックアップ付き）
- ビルド時バージョン注入（version.txt + ldflags）

## デプロイ構成

| 環境 | モード | 特徴 |
|------|--------|------|
| レンタルサーバー (さくら / Xserver) | CGI | shell wrapper + バイナリ、Apache RewriteRule |
| VPS / コンテナ | serve | 常駐プロセス、systemd / k8s |

## ロードマップ

### v1 (一般公開)
- Mastodon 互換 API (`/api/v1/`) + OAuth 2.0
- セットアップウィザードの改善
- ドキュメント整備

### v2 以降
- ネイティブクライアント (iOS/Android/Desktop)
- ペルソナ切り替え（複数 Actor の使い分け UI）
- CMS（長文記事）
- Web Push 通知
- テーマ管理 UI
- 自己更新機構
- Bluesky 対応（検討中）
