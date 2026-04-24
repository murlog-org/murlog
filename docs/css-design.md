# CSS 設計

## 3つのCSS空間

body クラスで切り替え。同一 Vite バンドル内で共存する。

| body クラス | 対象 | CSS の性質 |
|---|---|---|
| `body.spa` | `/my/*` (ログイン後) | murlog 本体管理。`--mur-*` トークン + コンポーネント |
| `body.public` | `/`, `/users/*` (公開ページ) | テーマ自由空間。default テーマは `--mur-*` を借用 |
| `body.ssr` | Go SSR 初期 HTML | 最小限。セマンティックタグ向け。テーマ/SPA 非依存 |

### 切り替えタイミング

SSR → SPA の引き継ぎフローの全体像は [Web フロントエンド](frontend.md) の「SPA ↔ SSR 切り替えフロー」を参照。以下は CSS 観点の詳細。

### SSR → SPA 引き継ぎフロー（CSS 観点）

**SSR 直接アクセス:**

```
Go HTML 配信
  <body class="ssr">
  <style> body.ssr 背景色 (ダークモード対応) + .ssr-content { opacity: 0 }
  <link> Vite CSS (body.spa スコープ → ssr では効かない)
  <link id="theme-css-default"> テーマ CSS (body.public スコープ → ssr では効かない)
  <div class="ssr-content"> SSR HTML
    ↓
  ブラウザ描画: 背景色のみ表示。SSR コンテンツは opacity:0 で非表示
    ↓
  JS ロード + Preact マウント
    ↓
  公開ページコンポーネントが loadTheme() → テーマ描画 → clearSSRContent()
    ↓
  body.ssr → body.public 切り替え (テーマ描画完了後)
    ↓
  テーマ CSS が効いた状態で表示
```

**SPA → 公開ページ遷移:**

```
  body.spa で SPA 画面を表示中
    ↓
  公開ページに遷移。body クラスはまだ切り替えない
    ↓
  公開ページコンポーネントが loadTheme() → テーマ CSS onload 待ち → テーマ描画
    ↓
  body.spa → body.public 切り替え (テーマ描画完了後)
    ↓
  テーマ CSS が効いた状態で表示
```

**重要: body クラスの切り替えはテーマ描画完了後に行う。**
app.tsx で即座に切り替えるとテーマ CSS ロード前にスタイルなしの瞬間が生まれる (FOUC)。

### SSR CSS の方針

- Go テンプレートの `<style>` にインラインで埋め込む（外部ファイル読み込みを待たない）
- `.ssr-content { opacity: 0 }` で SSR コンテンツを非表示（SSR と public のマークアップ差を隠す）
- `body.ssr` の背景色をライト/ダーク対応（ダークモードフラッシュ防止）
- SSR コンテンツが見えない代わりに、JS ロードまで空白になるトレードオフ

## テーマとの関係

- **テーマの CSS 空間は制約なし** — `--mur-*` を使うも使わないも自由
- **default テーマ** は `--mur-*` を参照し、SPA と体験を統一する
- **ヘッダーはテーマ管轄** — default は `--mur-*` 参照。カスタムテーマはヘッダーごと書き換え可能
- カスタムテーマが「ヘッダーだけ default のまま」の場合、`--mur-*` を定義すればヘッダーが馴染む

## SPA トークン (`--mur-*`)

`body.spa` スコープで定義。SPA コンポーネントのみが参照する。

### カラートークン

| トークン | Light | Dark | 用途 |
|---|---|---|---|
| `--mur-ink` | `#1d1d1f` | `#e5e5e7` | メインテキスト |
| `--mur-paper` | `#ffffff` | `#161618` | カード・入力欄の背景 |
| `--mur-surface` | `#f5f5f7` | `#1c1c1e` | body 背景、hover、サブ領域 (旧 `--secondary`) |
| `--mur-accent` | `#3a7ca5` | `#5aaddb` | ブランドカラー、リンク |
| `--mur-accent-soft` | `#eef4f8` | `#1a2a36` | アクセント薄め背景 |
| `--mur-muted` | `#86868b` | `#7c7c80` | 補助テキスト、時刻、ハンドル |
| `--mur-border` | `#d2d2d7` | `#303034` | ボーダー共通 |
| `--mur-danger` | `#dc2626` | `#ef4444` | エラー・危険アクション |
| `--mur-on-accent` | `#ffffff` | `#ffffff` | アクセント色の上の前景色 |
| `--mur-shadow` | `rgba(0,0,0,0.08)` | `rgba(0,0,0,0.3)` | ドロップシャドウ |
| `--mur-overlay` | `rgba(0,0,0,0.6)` | `rgba(0,0,0,0.6)` | オーバーレイ背景 |

### その他トークン

| トークン | 値 | 用途 |
|---|---|---|
| `--mur-radius-sm` | `3px` | 小パーツ角丸（badge 等） |
| `--mur-radius` | `6px` | 標準角丸 |

### Badge / 状態カラートークン

| トークン | Light | Dark | 用途 |
|---|---|---|---|
| `--mur-badge-pending-bg` | `#edf2f7` | `#2d3748` | pending badge 背景 |
| `--mur-badge-pending` | `#4a5568` | `#a0aec0` | pending badge テキスト |
| `--mur-badge-running-bg` | `#ebf8ff` | `#1a365d` | running badge 背景 |
| `--mur-badge-running` | `#2b6cb0` | `#63b3ed` | running badge テキスト |
| `--mur-badge-done-bg` | `#f0fff4` | `#1c4532` | done badge 背景 |
| `--mur-badge-done` | `#276749` | `#68d391` | done badge テキスト |
| `--mur-badge-failed-bg` | `#fff5f5` | `#3b1111` | failed badge 背景 |
| `--mur-badge-failed` | `#c53030` | `#fc8181` | failed badge テキスト |
| `--mur-row-failed-bg` | `rgba(229,62,62,0.05)` | `rgba(252,129,129,0.08)` | failed 行背景 |

### Spacing（トークン化しない — 規約のみ）

使って良い値: `2 / 4 / 6 / 8 / 12 / 16 / 20 / 24` (px)

- 4px ベースグリッド（Tailwind 準拠）
- 10px は使わない。8 か 12 に寄せる
- 60px 等のレイアウト固有の計算値はこのスケール外で OK
- トークン化しない理由: ダークモードで値が変わらない、px 直書きの方が可読性が高い

## グローバル基盤の扱い

全て `body.spa` にスコープする。公開ページはテーマが全責任を持つ。

| プロパティ | 扱い | 理由 |
|---|---|---|
| `* { margin:0; padding:0; box-sizing }` | `body.spa` スコープ | Ghost テーマ移植時にリセット衝突を避ける |
| `a { color; text-decoration }` | `body.spa a` | 公開ページのリンク色はテーマ任せ |
| `font-family` | `body.spa` | default テーマ側で再定義。カスタムテーマとの衝突回避 |
| `line-height` | `body.spa` | 同上 |
| `-webkit-font-smoothing` | `body.spa` | 同上 |
| `background` | `body.spa` | SPA は surface 色固定 |
| `color` | `body.spa` | SPA は ink 色固定 |

## default テーマ CSS の生成

`spa/style.css` をマスターとして `public/style.css` を生成する。
両ファイルはコメント行と `body.spa` / `body.public` のスコープ名だけが異なる。

```bash
make css   # spa/style.css → public/style.css を生成
```

SPA CSS を編集したら `make css` で public 側を再生成する。

---

## コーディングスタイルガイド

### レイアウト

- **2層構造** — `.screen` (ページコンテナ) + `.card` (コンテンツブロック) のみ
- サイドバーなし。全画面 1 カラム
- `.screen`: `max-width: 720px; margin: auto` で中央寄せ
- `.card`: `background + border + border-radius + padding` の汎用カード
- 投稿もフォームも設定も全て `.card` で囲む

### モバイル対応

- **640px 以下**: `.screen` の padding を 0、`.card` / `.tabs` をフルワイド化
  - `border-radius: 0`、左右ボーダーなし、`border-bottom` のみで区切り
  - `margin-bottom: 0` でカード間の隙間を詰める
  - テーブルは `.queue-desktop` クラスで列を出し分け（試行・エラー・アクション非表示）
  - `.dialog` はフル幅・角丸なし
- **480px 以下**: プロフィールのアバター縮小、フォローボタン非表示等

### セレクタ規約

- 全セレクタに `body.spa` (または `body.public`) スコープを付ける
- クラス名はフラット。BEM の `__` / `--` は使わず `-` で接続
  - 例: `.post-header`, `.post-author-name`, `.post-stat-action`
- コンポーネント prefix でグルーピング
  - `.post-*` (投稿), `.compose-*` (投稿フォーム), `.notif-*` (通知), `.settings-*` (設定), `.queue-*` (キュー), `.profile-*` (プロフィール), `.user-card-*` (ユーザーカード), `.dialog-*` (ダイアログ), `.job-detail-*` (ジョブ詳細)
- 汎用クラス（prefix なし、spa/public 共通）
  - `.screen` (ページコンテナ), `.card` (コンテンツブロック), `.tabs` / `.tab` (タブ), `.badge` (ステータスバッジ), `.btn` (ボタン), `.header` / `.header-brand` (ヘッダー), `.input` (フォーム入力), `.handle` (ハンドル表示), `.placeholder` (未設定領域の破線枠)

### 色・トークン

- ハードコード色を使わず `var(--mur-*)` を参照する
- ダークモードは `@media (prefers-color-scheme: dark)` でトークン値を差し替えるだけ。コンポーネント側で分岐しない
- badge 系は `--mur-badge-*` トークンで対応済み

### Spacing

- 使って良い値: `2 / 4 / 6 / 8 / 12 / 16 / 20 / 24` (px)
- 4px ベースグリッド（Tailwind 準拠）。10px は使わない
- レイアウト計算値（60px 等）はスケール外で OK
- トークン化しない。px 直書き

### テキスト overflow

- ユーザー入力を表示する要素には `overflow-wrap: break-word` を付ける
  - 対象: `.post-content`, `.bio`, `.lookup-bio`, `.job-detail-error`
- 単行で切り詰める要素には `overflow: hidden; text-overflow: ellipsis; white-space: nowrap`
  - 対象: `.handle`, `.profile-field dd`, `.error-cell`

### 状態クラス

- ボタンの状態: `.btn-primary`, `.btn-outline`, `.btn-active` — border + background で表現
- 投稿アクションの状態: `.post-stat.active` — `color: var(--mur-accent)` のみ。枠は付けない
- CW 折りたたみ: `.cw-summary` + `.cw-body`。「もっと見る」ボタンはテキスト直後に配置

### コンポーネント内レイアウト

- 横並び両端: `display: flex; justify-content: space-between`
  - `.post-header` (author ↔ time), `.compose-footer` (actions ↔ submit)
- 横並びギャップ: `display: flex; gap: Npx`
  - `.post-author`, `.compose-actions`, `.cw-summary`
- 画像グリッド: `display: grid; grid-template-columns`
  - `.post-media-1` 〜 `.post-media-4` で枚数ごとのレイアウト
- オーバーレイ + ダイアログ: `.overlay` (固定全面) + `.dialog` (中央配置カード)
  - `.dialog-header` / `.dialog-body` / `.dialog-footer` の3層構造

### font-size

6段階スケール。ベース 16px (`html { font-size: 100% }`)。全て `var(--mur-text-*)` で指定する。

| トークン | 値 | 用途 |
|---|---|---|
| `--mur-text-xs` | `0.6875rem` (11px) | badge, 極小ラベル |
| `--mur-text-sm` | `0.75rem` (12px) | handle, time, meta, 補助テキスト |
| `--mur-text-md` | `0.875rem` (14px) | author-name, dropdown, tab, ラベル |
| `--mur-text-base` | `1rem` (16px) | 本文, input, button |
| `--mur-text-lg` | `1.25rem` (20px) | page-title, profile h1 |
| `--mur-text-xl` | `1.5rem` (24px) | login h1, stat count |

### トランジション

- duration は **`0.15s`** 統一。コンポーネントごとにバラけさせない
- 対象プロパティを明示する（`all` は避ける）
  - `transition: background 0.15s` — ホバー背景変化（タブ、行、リンクカード）
  - `transition: border-color 0.15s` — フォーカス時のボーダー色変化（input、textarea）
- easing はブラウザデフォルト (`ease`)。指定不要

### z-index

3段階スケール。間を空けて衝突を防ぐ。

| 値 | 用途 |
|---|---|
| `10` | sticky ヘッダー（`.header`） |
| `20` | ドロップダウンメニュー（`.dot-menu-dropdown`） |
| `100` | オーバーレイ + ダイアログ（`.overlay`） |

これ以外の z-index は原則使わない。

### ボーダー

- 標準パターン: `1px solid var(--mur-border)`
- 区切り線は `border-bottom` で表現（`.card-section`, テーブル行）
- 破線: `1px dashed var(--mur-border)` — `.placeholder`（未設定のアバター・ヘッダー画像等）
- アクセント枠: `1px solid var(--mur-accent)` — アクティブ状態、フォーカス

### border-radius

| 値 | 用途 |
|---|---|
| `var(--mur-radius)` (6px) | カード、input、ボタン、ドロップダウン、プロフィールカード等の標準角丸 |
| `50%` | アバター、丸アイコン |
| `9999px` | カプセル型（通知バッジ、フォローボタン等）。要素の高さより大きい値を指定することで、幅が変わっても左右が常に半円になる |
| `var(--mur-radius-sm)` (3px) | badge（小パーツ用、標準より控えめ） |
| `0` | モバイル 640px 以下でカード・ダイアログをフルワイド化 |

### 影（box-shadow）

- 色は `var(--mur-shadow)` トークンを使う（light/dark 自動対応）
- 2段階のみ:
  - **軽い影**: `0 4px 12px var(--mur-shadow)` — ドロップダウンメニュー
  - **強い影**: `0 8px 32px var(--mur-shadow)` — ダイアログ
- カードには影を付けない（border で区切る）

### カーソル

- クリッカブルな非リンク要素には `cursor: pointer` を付ける
  - 対象: ボタン、タブ、テーブル行（クリックで詳細表示）、閉じるボタン、チェックボックスラベル
- `disabled` 状態には `cursor: not-allowed`
