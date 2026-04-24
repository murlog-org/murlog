# murlog

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)

**自分のドメインに、自分のホームを持つ。**

murlog は一人用の ActivityPub サーバー。Go シングルバイナリ + SQLite で、レンタルサーバーの CGI から VPS まで、どこでも動く。

## Features

- **ActivityPub 連合** — Mastodon / Misskey / GoToSocial と相互フォロー・投稿配信
- **CGI 対応** — 共用レンタルサーバー (さくら、Xserver 等) でも動作
- **ゼロ依存** — Go シングルバイナリ + SQLite、外部サービス不要
- **SPA + SSR** — Preact SPA (マイページ) + Handlebars テーマ (公開ページ)
- **S3 互換ストレージ** — メディアを Cloudflare R2 等に保存可能
- **多言語対応** — 日本語 / 英語、Accept-Language で自動切替
- **PWA** — モバイルでアプリライクに使える

## Quick Start

```bash
# ビルド
make build

# 起動 (ポート 8080)
make serve
```

ブラウザで `http://localhost:8080` にアクセスし、セットアップウィザードに従う。

## Build from Source

### Prerequisites

- Go 1.22+
- Node.js 20+ (フロントエンドビルド用)

```bash
git clone https://github.com/murlog-org/murlog.git
cd murlog

# フロントエンドビルド
make web-install
make web-build

# Go バイナリビルド
make build

# 開発モード (Go + Vite dev server 並走)
make dev
```

## CGI デプロイ

レンタルサーバー向けのクロスコンパイル:

```bash
# Linux/FreeBSD 向けバイナリを一括ビルド
make cross

# dist/release/ にバイナリが生成される
ls dist/release/
# murlog-linux-amd64  murlog-freebsd-amd64  ...
```

詳細は [docs/cgi.md](docs/cgi.md) を参照。

## Documentation

| ドキュメント | 内容 |
|---|---|
| [docs/overview.md](docs/overview.md) | プロダクト概要・技術選定 |
| [docs/architecture.md](docs/architecture.md) | サーバーアーキテクチャ |
| [docs/domain.md](docs/domain.md) | ドメインモデル |
| [docs/murlog-api.md](docs/murlog-api.md) | API 仕様 (JSON-RPC 2.0) |
| [docs/activitypub/](docs/activitypub/) | ActivityPub 実装・互換性 |
| [docs/frontend.md](docs/frontend.md) | フロントエンド (SPA + テーマ) |
| [docs/cgi.md](docs/cgi.md) | CGI デプロイ |
| [docs/security.md](docs/security.md) | セキュリティ設計 |

## License

[AGPL-3.0](LICENSE) — Copyright (C) 2026 alarky
