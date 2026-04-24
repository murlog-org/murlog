# CGI 動作アーキテクチャ

## リクエストフロー

```
ブラウザ → Apache
  ├─ 実ファイル (assets/, themes/) → Apache が直接配信 (CGI 不要)
  ├─ 機密ファイル (.ini, .db 等) → Apache がブロック (CGI に到達しない)
  └─ それ以外 → .htaccess RewriteRule → murlog.cgi (Go バイナリ直接)
           ├─ /users/* (HTML) → SSR HTML (OGP + 本文 + SPA script)
           ├─ /my/*           → SPA index.html を返す
           ├─ /api/*          → JSON API
           └─ /admin/*        → SSR (セットアップ・リセット)
```

## 対応環境

| ホスト | 対応 | 備考 |
|--------|------|------|
| Xserver | ✅ | |
| さくらのレンタルサーバ | ✅ | FreeBSD 向けバイナリが必要。ライトプラン含む全プラン対応 |
| ConoHa WING | ✅ | |
| お名前.com レンタルサーバー | ✅ | |
| ロリポップ! | ❌ | 公式に CGI 使用を推奨しない／非サポート |
| ヘテムル | ❌ | 同上 |

CGI が利用可能でも、共用 IP のリピュテーションや TOS 上 ActivityPub 配送と相性の悪いホストは公式サポート対象から外す。全社 suEXEC 有効で、CGI は契約ユーザー権限で動作する（777 は全社禁止）。

## デプロイ構成

```
public_html/
├── murlog.cgi       # Go バイナリ (chmod 755、.cgi 拡張子で直接配置)
├── .htaccess        # リライトルール + 機密ファイル保護
├── assets/          # Vite ビルド成果物 (Apache 直配信)
├── themes/          # テーマファイル (Apache 直配信)
│   └── default/
└── dist/            # WebDir
    ├── index.html   # SPA エントリポイント
    └── templates/   # SSR テンプレート (Go が読む)
```

- cgo 不要構成 (Pure Go SQLite) により、どの OS 向けにも同一手順でビルド可能
- さくらは FreeBSD (`GOOS=freebsd`)、他は Linux がターゲット

## .htaccess

`.htaccess` は `dist/cgi/htaccess` をマスターとして配布する。CGI にリクエストを流す `.htaccess` がないと CGI 自体が実行されないため、自動生成に頼れない。

```apache
# 機密ファイルの保護 / Protection of sensitive files
<FilesMatch "\.(ini|toml|db|db-wal|db-shm|db-journal|reset)$">
  Require all denied
</FilesMatch>

# Authorization ヘッダーを CGI に渡す / Pass Authorization header to CGI
CGIPassAuth On

# リクエストルーティング / Request routing
DirectoryIndex murlog.cgi
RewriteEngine On
RewriteRule .* - [E=HTTP_AUTHORIZATION:%{HTTP:Authorization}]
# データディレクトリへのアクセスをブロック / Block access to data directory
RewriteRule ^data/ - [F,L]
# dist/ 配下の静的ファイルを Apache で直接配信 / Serve static files in dist/ directly
RewriteCond dist%{REQUEST_URI} -f
RewriteRule ^(.*)$ dist/$1 [L]
# その他の実ファイルは Apache が直接配信 / Serve other existing files directly
RewriteCond %{REQUEST_FILENAME} -f
RewriteRule ^ - [L]
# それ以外は全て CGI へ (PATH_INFO 経由) / Route everything else to CGI via PATH_INFO
RewriteRule ^(.*)$ /murlog.cgi/$1 [L,QSA]
```

## CGI 固有の制約と対策

**Authorization ヘッダー**: Apache はデフォルトで `Authorization` を CGI に渡さない。`CGIPassAuth On` (Apache 2.4.13+) で解決するが、一部レンサバで使えない可能性があるため `RewriteRule .* - [E=HTTP_AUTHORIZATION:%{HTTP:Authorization}]` をフォールバックとして併用。

**ワーキングディレクトリが不定**: `os.Executable()` は cgid デーモンのパスを返す場合がある。`SCRIPT_FILENAME` 環境変数でバイナリ位置を解決し、`os.Chdir` でカレントディレクトリも設定。

**CGI 自動検出**: 引数なしで起動すると `serve` モードになるため、`GATEWAY_INTERFACE` 環境変数の有無で CGI モードを自動判定。

**Go ランタイムの nproc 制限**: 共用サーバー (Xserver 等) は `ulimit -u` が厳しく、Go のデフォルト `GOMAXPROCS` (=CPU コア数) だと OS スレッド作成で死ぬ。Go バイナリ内で `runtime.GOMAXPROCS(1)` を設定し、OS スレッド数を制限する。

**mod_cgi vs mod_cgid**: event/worker MPM では `mod_cgi` が使えず `mod_cgid` が必要。

**リライトルールの方式**: `/murlog.cgi/$1` (PATH_INFO 方式) を採用。Go の `net/http/cgi` は `REQUEST_URI` を優先的に使うため、PATH_INFO 経由でもリクエストパスは正しく解釈される。この方式はさくらライトプラン (suexec が RewriteRule → CGI 直接実行を拒否する環境) でも動作する。`murlog.cgi` へ直接リライトする方式 (`RewriteRule ^(.*)$ murlog.cgi [L,QSA]`) は suexec policy violation になるプランがある。

**web_dir**: 開発時は `./web/dist` だが、CGI zip は `./dist/index.html` に配置。`murlog.ini` の `web_dir` は CGI 環境では `./dist` がデフォルト。`/assets/` は Apache 直配信、SPA fallback の `index.html` は Go が `dist/` から配信。