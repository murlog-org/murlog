# murlog プロダクト概要

## コンセプト

**自分のドメインに、自分のホームを持つ**

murlog は一人用の ActivityPub サーバー。自分のドメインで、誰にも所有されない場所を作る。SNS もブログもプロフィールも、全部自分のもの。fediverse に繋がるから孤立もしない。

### 設計哲学

**依存性最小。** ライブラリにも、プラットフォームにも、他プロダクトの設計判断にも依存しない。

**どこでも動く。** Mastodon や Misskey 等の分散 SNS は多機能で大規模コミュニティ向けに設計されている。murlog は「一人用」に割り切ることで、レンタルサーバーの CGI から VPS まで、どんな環境でも動くシンプルな構成を実現する。

## プロトコル

- **ActivityPub** で Mastodon / Misskey / GoToSocial 等と連合
- **Bluesky** とは将来的に対応を検討

## 技術選定

| 要素 | 選定 | 理由 |
|------|------|------|
| 言語 | Go | シングルバイナリ、クロスコンパイル容易、CGI対応 (`net/http/cgi`) |
| DB | SQLite via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) | Pure Go実装のためcgo不要。クロスコンパイルが一発で通る。将来的にMySQL/PostgreSQL対応もビルドタグで切り替え可能 (Pure Goドライバのため同様にcgo不要) |
| ActivityPub | 自前実装 | 各サーバーの方言に柔軟に対応するため、ライブラリに縛られず実装をコントロールする |
| ビルド | クロスコンパイル | `GOOS=linux/freebsd GOARCH=amd64` でレンタルサーバー向けビルド |
| バイナリ最適化 | `-ldflags="-s -w"` + UPX圧縮 | サイズ削減 (目安: 10-15MB → 5-8MB程度) |
| SPA フレームワーク | [Preact](https://preactjs.com/) | React 互換で ~4KB (min+gz)。マイページ (/my/) の固定 UI に使用 |
| テンプレートエンジン | [Handlebars](https://handlebarsjs.com/) | 公開ページのテーマレンダリング。Ghost テーマと同じ形式で互換性が高い |
| SPA ビルド | Vite | Preact 公式推奨。高速ビルド、Tree-shaking でバンドル最小化 |

## ネイティブクライアント（予定）

- C# MAUI (iOS/Android、将来的にデスクトップ) を検討中
- murlog 独自 API (`/api/mur/v1/`) を使用
- CMS 機能を含む全操作に対応

```
[Go サーバー]
  ├─ 公開ページ SSR (OGP + 本文 + SPA script)
  ├─ murlog 独自 API /api/mur/v1/    … SPA・独自クライアントが共用
  ├─ Mastodon 互換 API /api/v1/      … 既存クライアントアプリ対応
  ├─ OAuth 2.0 /oauth/
  ├─ ActivityPub
  └─ ジョブキュー

[SPA (静的ファイル、Go バイナリに含まない)]
  ├─ 公開ページ (Handlebars テーマ)
  └─ マイページ /my/ (Preact 固定 UI)

[C# ネイティブクライアント]
  ├─ murlog 独自 API クライアント
  └─ MAUI (iOS/Android/Desktop)

[Mastodon クライアント (Ivory, Ice Cubes 等)]
  └─ Mastodon 互換 API
```

## 自己更新（予定）

サーバー管理経験のないユーザーでもバージョン追従できるよう、本体自身に更新機構を持たせる予定。

- 起動時および定期的に GitHub Releases API をポーリング
- 新バージョンがあればマイページにバナー表示
- 「更新」ボタンを押すと `murlog update` 相当の処理が走り、バイナリを差し替え
- DB マイグレーションは起動時に自動実行（手動オペレーションを要求しない）
- CGI モードでも同じフローで自己更新可能（ファイル差し替え方式）

## 機能スコープ

### v1 (MVP)

- テキスト投稿・編集・削除
- 画像添付 (EXIF 除去、HEIC→JPEG 変換)
- S3 互換ストレージ (Cloudflare R2 等)
- ペルソナ（複数Actor）
- ActivityPub連合 (HTTP Signatures, WebFinger)
- フォロー/フォロワー管理
- ホームタイムライン
- 公開プロフィール・投稿ページ
- マイページ /my/ (Preact SPA)
- murlog 独自 API (`/api/mur/v1/`)
- CGI / スタンドアロン両対応
- Docker対応
- ジョブキュー (リクエスト駆動 + アダプティブ並列処理)
- 多言語対応 (日本語 / English)
- ハッシュタグ (タグ別一覧、公開タグページ、ActivityPub Hashtag タグ)

### v2 以降

- Mastodon 互換 API (`/api/v1/`) + OAuth 2.0
- ネイティブクライアント (C# MAUI — iOS/Android/Desktop)
- 長文記事 (CMS機能)
- Bluesky 対応（検討中）
- サーバーレス環境対応 (Cloudflare Workers + D1 等)
- テーマ管理
- WebSocket ストリーミング (通知・タイムラインのリアルタイム配信)
