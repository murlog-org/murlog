---
layout: home
hero:
  name: murlog
  text: 技術ドキュメント
  tagline: 1人1インスタンスの ActivityPub マイクロブログ
  actions:
    - theme: brand
      text: プロダクト概要
      link: /overview
    - theme: alt
      text: murlog API
      link: /murlog-api
features:
  - title: CGI 対応
    details: Go シングルバイナリ + SQLite。レンタルサーバーの CGI でも動く
  - title: ActivityPub
    details: Mastodon, GoToSocial, Misskey と連合可能
  - title: S3 互換ストレージ
    details: Cloudflare R2 等の S3 互換ストレージに対応。外部依存ゼロの自前 SigV4 実装
---
