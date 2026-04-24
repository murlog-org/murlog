# はじめかた

## 必要なもの

- 独自ドメイン (例: `example.com`)
- 以下のいずれかのサーバー環境:
  - レンタルサーバー (さくら・Xserver 等、CGI 対応プラン)
  - VPS / クラウドサーバー

## レンタルサーバー (CGI モード)

### 1. ダウンロード

[Releases](https://github.com/murlog-org/murlog/releases) から自分のサーバーに合った CGI zip をダウンロード。

| サーバー | ファイル |
|---|---|
| さくらレンタルサーバー | `murlog-cgi-freebsd-amd64.zip` |
| Xserver | `murlog-cgi-linux-amd64.zip` |

### 2. アップロード

zip を展開し、ドメインの公開ディレクトリに全ファイルをアップロード。

```
public_html/
  ├── murlog.cgi      # CGI エントリポイント
  ├── murlog.bin       # Go バイナリ
  ├── .htaccess        # Apache 設定
  ├── 500.html         # エラーページ
  └── dist/            # Web アセット
      ├── index.html
      ├── assets/
      ├── templates/
      ├── themes/
      └── locales/
```

### 3. セットアップ

ブラウザで `https://yourdomain.com/` にアクセスすると、セットアップウィザードが表示される。

**Step 1: サーバー設定**
- DB パス、メディアパスの確認 (通常はデフォルトのまま)
- `murlog.ini` が自動生成される

**Step 2: サイトセットアップ**
- ドメイン名 (自動入力)
- ユーザー名・表示名
- パスワード設定

完了すると自分のプロフィールページが表示される。

## VPS / クラウドサーバー (serve モード)

### 1. ダウンロード

[Releases](https://github.com/murlog-org/murlog/releases) からバイナリをダウンロード。

```bash
curl -L -o murlog https://github.com/murlog-org/murlog/releases/latest/download/murlog-linux-amd64
chmod +x murlog
```

### 2. 起動

```bash
./murlog serve
```

デフォルトで `:8080` で起動。リバースプロキシ (nginx / Caddy 等) で HTTPS を終端する。

### 3. セットアップ

ブラウザで `https://yourdomain.com/` にアクセスし、CGI モードと同じウィザードで初期設定。

## セットアップ後

- **投稿する**: `/my/` からタイムラインにアクセスしてテキストや画像を投稿
- **フォローする**: `/my/follow` から他の fediverse ユーザーを `@user@server` 形式で検索してフォロー
- **設定する**: `/my/settings` からプロフィール (表示名・アイコン・ヘッダー・自己紹介) を編集

## バージョンアップ

バイナリを新しいバージョンに差し替えて再起動するだけ。DB マイグレーションは起動時に自動実行される。
