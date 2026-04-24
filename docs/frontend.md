# Web フロントエンド

## 全体像

SPA (Preact + Vite) で全ページを構築する。フロントエンドは Go バイナリに embed せず静的ファイルとして配信する。静的ファイルの配信元は環境によって異なる（CGI: Apache が直接配信、serve/Docker: Go が配信）。Go は API・SSR を担当する。

| 要素 | 技術 | 役割 |
|------|------|------|
| SPA フレームワーク | [Preact](https://preactjs.com/) | React 互換、~4KB (min+gz)。マイページの固定 UI |
| テンプレートエンジン | [Handlebars](https://handlebarsjs.com/) | 公開ページのテーマレンダリング。Ghost テーマと同形式 |
| ビルド | Vite | 高速ビルド、Tree-shaking でバンドル最小化 |

画面は2つの領域に分かれ、レンダリング技術が異なる:

| 画面 | レンダリング | テーマ適用 |
|---|---|---|
| 公開ページ (プロフィール、投稿) | Handlebars テンプレート | フルカスタム (HTML テンプレート + CSS) |
| /my/ (タイムライン、設定、投稿) | Preact コンポーネント | 配色・フォントのみ (CSS 変数) |

## SPA 構成

1つの `index.html` をエントリポイントとし、フロント側ルーターでページを分岐:

```
index.html (SPA)
  ├── /                      → 公開ホーム (Handlebars テーマでレンダリング)
  ├── /users/alice           → 公開プロフィール (Handlebars テーマでレンダリング)
  ├── /users/alice/posts/xxx → 公開投稿パーマリンク (Handlebars テーマでレンダリング)
  ├── /users/alice/following → 公開フォローリスト (Preact)
  ├── /users/alice/followers → 公開フォロワーリスト (Preact)
  ├── /users/@user@host      → リモート Actor プロフィール (Handlebars テーマでレンダリング)
  ├── /tags/:tag             → タグページ (Handlebars テーマでレンダリング)
  ├── /my/                   → マイページ (Preact 固定 UI)
  ├── /my/posts/new          → マイページ (Preact 固定 UI)
  ├── /my/login              → ログイン (Preact 固定 UI)
  ├── /my/notifications      → 通知一覧 (Preact 固定 UI)
  ├── /my/settings           → 設定 (Preact 固定 UI)
  ├── /my/blocks             → ブロック管理 (Preact 固定 UI)
  ├── /my/follow             → フォロー検索 (Preact 固定 UI)
  └── /my/queue              → キュー管理 (Preact 固定 UI)
```

### ファイル構成 / File structure

```
web/src/
├── pages/                    # ページコンポーネント / Page components
│   ├── public-home.tsx         公開ホーム — 全ペルソナを Handlebars テーマでレンダリング
│   ├── public-profile.tsx      公開プロフィール — ペルソナの投稿一覧 (テーマ)
│   ├── public-post.tsx         公開投稿パーマリンク — スレッドコンテキスト付き単一投稿 (テーマ)
│   ├── public-follow-list.tsx  公開フォロー/フォロワーリスト
│   ├── remote-profile.tsx      リモート Actor プロフィール (テーマ)
│   ├── tag.tsx                 タグページ — 特定ハッシュタグの投稿一覧 (テーマ + 無限スクロール)
│   ├── my.tsx                  マイページ — タイムライン + 投稿作成
│   ├── login.tsx               ログインフォーム (TOTP 対応)
│   ├── notifications.tsx       通知一覧
│   ├── settings.tsx            設定 (プロフィール編集・カスタムフィールド・TOTP)
│   ├── blocks.tsx              ブロック管理 (Actor ブロック + ドメインブロック、タブ切り替え)
│   ├── follow.tsx              フォロー検索・管理
│   └── queue.tsx               キュー管理ダッシュボード
├── components/               # 共通コンポーネント / Shared components
│   ├── dropdown.tsx            汎用ドロップダウンメニュー (外側クリックで閉じる)
│   ├── icons.tsx               SVG アイコン (reply/reblog/heart/pin/share/attach/more)
│   ├── lightbox.tsx            画像ライトボックス (全画面オーバーレイ)
│   ├── loading.tsx             ローディングスピナー
│   └── logo.tsx                ドットコンステレーションロゴ (サイズ適応)
├── lib/                      # ユーティリティ / Utilities
│   ├── api.ts                  JSON-RPC 2.0 クライアント (`call()`, `isUnauthorized()`)
│   ├── auth.ts                 認証状態フラグ (`isLoggedIn()` / `setLoggedIn()`)
│   ├── format.ts               相対時間フォーマット (`formatTime()`)
│   ├── i18n.ts                 SPA 多言語対応 (`load()` + `t()`, /locales/{lang}.json)
│   ├── image.ts                画像変換 (HEIC → JPEG, Canvas API)
│   ├── sanitize.ts             HTML サニタイズ (DOMPurify, リモート投稿・Actor サマリー用)
│   ├── theme.ts                テーマローダー (Handlebars テンプレートの取得・キャッシュ・レンダリング)
│   └── types.ts                共通型定義 (`RemoteActor` 等)
└── app.tsx                   # ルーター + 認証チェック
```

## レンダリング方式

プログレッシブエンハンスメントによるハイブリッドレンダリング:

| リクエスト元 | 判定方法 | レスポンス |
|---|---|---|
| ActivityPub サーバー | `Accept: application/activity+json` | JSON-LD |
| それ以外（ブラウザ・クローラー共通） | 上記以外 | SSR HTML (OGP + 本文 + SPA エントリポイント) |

- **User-Agent によるクローラー判定は行わない**。公開ページは常に SSR HTML を返す
- SSR HTML には OGP メタタグ・本文テキスト・SPA の `<script>` タグを含む
- ブラウザ: JS が読み込まれると SPA が起動し、Handlebars テーマで公開ページを再描画（クライアントサイドハイドレーション）
- クローラー / JS 無効環境: SSR HTML がそのまま表示される。デザインは最小限だがコンテンツは読める
- `/my/` 配下はログイン必須のため SSR 不要。従来通り SPA の `index.html` を返す

### SSR データ埋め込み

SSR HTML にはページデータを JSON として埋め込む。SPA は API を叩かずにこのデータから Handlebars レンダリングを開始できる。

```html
<!-- Go SSR が埋め込む -->
<script id="ssr-data" type="application/json">
{
  "persona": { "username": "alice", "displayName": "Alice", ... },
  "posts": [{ "id": "...", "content": "...", ... }]
}
</script>
```

- JSON の形式は API レスポンス (`personas.get`, `posts.list`) と同一にする
- SPA のデータ取得は「SSR データがあれば使う、なければ API fetch」のフォールバック構造
- 初回表示（Go → ブラウザ）: SSR データから即座に Handlebars レンダリング。API 不要
- SPA 内遷移（例: `/my/` → `/users/alice`）: SSR データがないため API fetch にフォールバック

### SPA ↔ SSR 切り替えフロー

```
1. Go が SSR HTML を返す（<body class="ssr"> + ssr-data JSON 埋め込み）
   - インライン <style> で body.ssr の背景色 + #app { opacity: 0 }
2. JS ロード → Preact Router 起動
3. /users/{username} にマッチ → PublicProfile コンポーネント
4. ssr-data から JSON を取得（なければ API fetch）
5. テーマファイルを fetch（キャッシュ済みならスキップ）:
   - /themes/{name}/theme.json → テーマメタデータ (name, version)
   - /themes/{name}/templates/*.hbs → Handlebars コンパイル
   - /themes/{name}/style.css → <link> で動的読み込み (onload 待ち)
6. Handlebars.compile(template)(data) → ref.innerHTML に差し替え
7. activatePublic() → body.ssr を body.public に切り替え + .ssr-content を DOM から削除
```

テーマファイルは初回 fetch 後にメモリキャッシュし、ページ遷移のたびに再取得しない。body クラスの切り替えはテーマ描画完了後に行い、FOUC を防ぐ。

### SSR テンプレート生成

SSR 用の Go テンプレートは **Vite ビルド時に生成** する。ハッシュ付きアセットパスの解決をフロントエンド側に閉じ込め、Go はテンプレートにデータを流し込むだけにする。

```
web/
  templates/
    home.tmpl.html        ← ソース (Go テンプレート構文 + アセット参照プレースホルダー)
    profile.tmpl.html
    post.tmpl.html
  dist/
    index.html            ← SPA エントリポイント
    assets/               ← Vite ハッシュ付きアセット
    templates/            ← ビルド時に生成
      home.tmpl           ← プレースホルダーが実際のアセットパスに置換済み
      profile.tmpl
      post.tmpl
```

- Vite プラグインまたはポストビルドスクリプトが `dist/.vite/manifest.json` を読み、テンプレート内のアセット参照を実パスに置換
- Go 側は `dist/templates/*.tmpl` を読み込んで `html/template.Execute()` するだけ
- `make web-build` 一発で SPA + SSR テンプレートが揃う

## CSS・テーマ

CSS は 3 つの空間 (`body.spa` / `body.public` / `body.ssr`) に分離し、body クラスでスコープする。トークン定義、コーディング規約、スタイルガイドは [CSS 設計](css-design.md) を参照。

### Handlebars テンプレート

公開ページのレンダリングには Handlebars テンプレートを使用。テーマファイルは静的ファイルとして配信する。

```
themes/default/
├── theme.json         # メタデータ (name, version)
├── style.css          # テーマ CSS (body.public スコープ)
└── templates/         # Handlebars テンプレート
    ├── home.hbs
    ├── profile.hbs
    ├── post.hbs
    ├── post-card.hbs  # 無限スクロール追加読み込み用
    └── tag.hbs
```

**Handlebars ヘルパー（SPA 側で登録）:**

- `isoDate` — ISO 8601 形式
- `formatDate` — `Intl.RelativeTimeFormat` によるロケール対応の相対時間表示
- `t` — i18n 翻訳キーの解決
- `autoLink` — URL 文字列を `<a>` タグでラップ（カスタムフィールド用）

### キャッシュバスティング

Vite ビルドで固定ファイル名 (`assets/index.js`, `assets/index.css`) を出力し、ビルド時タイムスタンプハッシュをクエリパラメータ (`?v=xxx`) で付与。FTP コピーでも古いファイルが残らない。`locales/*.json` と `themes/**/*` の fetch にも同じハッシュを付与。

### カスタムテーマ（v2 予定）

テーマ切り替え機能は v2 スコープ。現在はデフォルトテーマのみ。

## 公開ホーム

`/` でサイトのホームページを表示 (`pages/public-home.tsx`)。

- SSR データがあれば即レンダリング、なければ `personas.list` API でフォールバック
- 全ペルソナの投稿を Handlebars テーマ (`home.hbs`) でレンダリング
- SSR テンプレート: `home.tmpl.html`

## 公開投稿パーマリンク

`/users/:username/posts/:id` で単一投稿のパーマリンクページを表示 (`pages/public-post.tsx`)。

- `posts.get_thread` RPC でスレッドコンテキスト (ancestors / descendants) を取得
- メイン投稿は Handlebars テーマ (`post.hbs`) でレンダリング
- スレッドの前後投稿はカード形式で表示（サニタイズ済み HTML + 相対時間）
- SSR データがあれば API コール不要でテーマレンダリング開始
- SSR テンプレート: `post.tmpl.html`

## タグページ

`/tags/:tag` で特定ハッシュタグを含む投稿一覧を表示 (`pages/tag.tsx`)。

- `posts.list_by_tag` RPC でタグ付き投稿を取得
- Handlebars テーマ (`tag.hbs`) でレンダリング
- IntersectionObserver による無限スクロール（カーソルベース、20 件ずつ）
- リモート投稿は Actor 名・アカウント名を表示
- テーマの `post-card.hbs` パーシャルで追加ページをレンダリング

## ブロック管理

`/my/blocks` でアクターブロックとドメインブロックを管理 (`pages/blocks.tsx`)。

- タブ切り替え: アクターブロック / ドメインブロック
- アクターブロック: Actor URI を入力してブロック追加、一覧から解除
- ドメインブロック: ドメインを入力してブロック追加、一覧から解除
- `blocks.list` / `blocks.create` / `blocks.delete` RPC を使用
- `domain_blocks.list` / `domain_blocks.create` / `domain_blocks.delete` RPC を使用
- 未認証時はログインページにリダイレクト

## 添付画像ライトボックス

投稿の添付画像をクリックすると全画面オーバーレイで表示する (`components/lightbox.tsx`)。

- 元の `<img>` 要素をオーバーレイに移動して再フェッチを回避
- オーバーレイクリック・Escape キーで閉じる
- `openLightbox()` をエクスポートし、投稿カード内の画像クリックから呼び出し

## 通知

`/my/notifications` で通知一覧を表示 (`pages/notifications.tsx`)。

- 通知タイプ: follow / mention / reblog / favourite
- 各通知に Actor のアイコン・表示名・アカウント名を表示
- mention / reblog / favourite は投稿プレビュー（先頭テキスト）を表示
- 未読インジケータ + 一括既読マーク
- Actor アイコンクリックでプロフィールページに遷移

## リモート Actor プロフィール

`/users/@user@host` でリモート Actor のプロフィールを表示。

- **SSR**: `resolveRemoteActor` で WebFinger + fetch → プロフィールテンプレートで描画
- **SPA**: `RemoteProfile` コンポーネント。`actors.lookup` で Actor 情報を即取得してテーマ描画、`actors.outbox` で投稿を遅延ロードして追加描画
- `resolveRemoteActor` はキャッシュを使わず毎回フレッシュフェッチ（能動的アクセスのため）
- リモートフォローボタン: 未フォローの Actor に対しフォロー送信。`follows.create` RPC 経由で Follow Activity を配送
- オリジナルページへのリンク: Actor の `url` フィールドから元サーバーのプロフィールページにリンク
- ヘッダー画像: Actor の `image` フィールドからヘッダー画像を表示

## 公開フォロー/フォロワーリスト

`/users/:username/following` と `/users/:username/followers` でフォロー/フォロワー一覧を表示。

- `PublicFollowList` コンポーネント (`public-follow-list.tsx`)
- ローカル Actor: `follows.list` / `followers.list` RPC を使用（`show_follows` 設定を尊重）
- リモート Actor (`@user@host`): `actors.following` / `actors.followers` RPC でリモートサーバの Collection をフェッチ
- Go 側の `handleFollowersCollection` / `handleFollowingCollection` で `isActivityPubRequest` 判定し、ブラウザは SPA にフォールバック
- 各エントリにアイコン・表示名・bio を表示。アイコンクリックでプロフィールページに遷移
- ページネーション対応（カーソルベース）
- 認証済みユーザーはマイページのフォロー管理、未認証は公開プロフィールにリンク

## 無限スクロール

マイページタイムラインと公開プロフィールは IntersectionObserver による無限スクロール対応。API のカーソルベースページング（デフォルト 20 件）を利用。

## ピン留め投稿

ペルソナごとに最大 1 件の投稿をピン留めできる。

**タイムライン:**
- ローカル投稿のアクション欄に「ピン留め」/「ピン留め解除」ボタン (Pin/Unpin)
- ピン留めされた投稿には 📌 インジケータを表示
- ピン留めは公開のローカル投稿のみ対象
- 別の投稿をピン留めすると前のピンは自動解除

## カスタムフィールド

設定画面でプロフィールに表示するカスタムフィールド (最大 4 件) を編集できる。

**設定画面 (settings.tsx):**
- name/value のペアを最大 4 行まで追加・編集・削除
- 「フィールドを追加」ボタンで行を追加、各行に「削除」ボタン
- 保存時に空行は自動除外

## CW (Content Warning)

投稿の折りたたみ表示機能。Mastodon 互換の `summary` + `sensitive` を使用。

**投稿作成:**
- CW ボタン (トグル) → CW テキスト入力フィールドが表示される
- summary が設定されると `sensitive: true` が自動で付与される
- CW ボタン再押下で CW 入力を閉じ、summary をクリア

**投稿表示:**
- `summary` がある投稿: summary テキスト + 「もっと見る」ボタンを表示。content と添付は非表示
- 「もっと見る」クリック: content と添付を展開。ボタンが「閉じる」に変わる
- `summary` がない投稿: 通常表示 (折りたたみなし)
- 展開状態は `expandedPosts` (Set) でクライアント側管理

## キュー管理ダッシュボード

`/my/queue` ページで Sidekiq 風のジョブキュー管理。

**表示:**
- stats サマリー (pending/running/done/failed の件数)
- フィルタタブ（すべて / 待機 / 実行中 / 完了 / 失敗）
- ジョブ一覧テーブル (type, status, attempts, error, created_at, elapsed)
- 行クリックでジョブ詳細ダイアログ（エラー全文、ペイロード、宛先、次回実行）
- 失敗ジョブにリトライ / ディスミスボタン（一覧 + ダイアログ）
- モバイル 640px 以下で試行・エラー・アクション列を非表示、ダイアログフル幅

**CGI アクセラレータ:**
- ページ表示中は 5 秒間隔で `queue.tick` を自動実行
- CGI 環境でリクエストがない間のジョブ詰まりに対応

## TOTP 設定

設定画面 (`/my/settings`) 内の TOTP セットアップ/無効化コンポーネント。

**フロー:**
1. 「セットアップ」ボタン → `totp.setup` RPC → 秘密鍵 + otpauth URI 取得
2. QR コードを Canvas に描画 (`qrcode` npm)
3. 認証アプリでスキャン → 6 桁コード入力 → `totp.verify` で有効化
4. 無効化は「無効化」ボタン → `totp.disable`

**ログインフォーム:**
- `totp.status` (public) で TOTP 有効かチェック
- 有効時: パスワードの下に 6 桁コード入力フィールドを表示
- `inputMode="numeric"`, `autocomplete="one-time-code"`

**CMS プレビュー (v2):**

- v1: 「プレビュー」ボタンで別タブに下書きの公開ページを表示
- v2: エディタ内 iframe でリアルタイムプレビュー