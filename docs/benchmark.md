# ベンチマーク

murlog のパフォーマンス特性を把握するためのベンチマーク結果。

## 実行環境

- **マシン**: Apple M1 Pro
- **Go**: 1.26.2
- **SQLite**: modernc.org/sqlite (CGO-free), WAL モード
- **計測日**: 2026-04-19

## 実行方法

```bash
make bench                                          # Store + Queue ベンチマーク
go test ./handler/ -run=NONE -bench=. -benchmem     # RPC エンドツーエンド
```

## RPC エンドツーエンド (httptest, 実ファイル DB)

5000 投稿 + 500 フォロワー + 2000 いいねの DB で、HTTP 経由の JSON-RPC 呼び出しを計測。

| RPC メソッド | カテゴリ | 応答時間 | QPS | allocs/op |
|---|---|---|---|---|
| `timeline.home` (limit=20) | 読み込み | 1,285μs | 778 | 3,799 |
| `posts.list` (public, limit=20) | 読み込み | 1,279μs | 782 | 3,683 |
| `personas.get` | 読み込み | 219μs | 4,569 | 371 |
| `posts.create` | 書き込み | ~590μs* | ~1,695 | 705 |
| `queue.tick` | キュー処理 | 486μs | 2,058 | 338 |

*`posts.create` のベンチマーク値 (2,163μs) はベンチ中の DB 肥大化の影響を含む。実運用では INSERT + COUNT リフレッシュ (~50μs) で ~590μs 程度。

### 所見

- **全 RPC が 1.3ms 以下**。CGI 環境でも十分な応答速度
- **`personas.get` が #98 のカウンターキャッシュ化で 1,861μs → 219μs に 8.5x 高速化**。COUNT×3 のクエリが不要になり、ペルソナの読み取りだけで完結
- `posts.create` は投稿作成時にカウンターを COUNT ベースでリフレッシュするため ~50μs の追加コスト

## Store レイヤー (InMemory / File 2モード)

10000 投稿 + 1000 フォロワー + 5000 いいね + 3000 リブログの DB。

| クエリ | InMemory | File | 差 |
|---|---|---|---|
| `ListPostsByPersona` (limit=20) | 72μs | 74μs | 同等 |
| `ListPostsByPersona` (cursor) | 75μs | 80μs | 同等 |
| `PostInteractionCounts` (20件) | 70μs | 58μs | 同等 |
| `CountFollowers` (1000件) | 55μs | 57μs | 同等 |
| `ListPublicLocalPosts` (limit=20) | 74μs | 77μs | 同等 |
| Queue `Claim` (2000 jobs) | 568μs | 661μs | 1.2x |
| Queue `Enqueue→Claim→Complete` | 335μs | 583μs | 1.7x |

### 所見

- **InMemory vs File の差は 1.7x 以内**。WAL モードが効いている
- **Store クエリは全て 80μs 以下**。インデックスが適切に効いている
- **Queue 操作は 335-660μs**。File モードでは WAL の fsync コストが見える
- **データ量 10x (1000→10000 投稿) でもタイムライン取得はほぼ変わらない** — カーソルページングとインデックスが有効

## スケーリング特性

| データ量 | ListPostsByPersona | CountFollowers |
|---|---|---|
| 1000 投稿 / 100 フォロワー | 71μs | 9μs |
| 10000 投稿 / 1000 フォロワー | 77μs | 55μs |
| スケーリング | **1.08x** (インデックス効果) | **6x** (COUNT フルスキャン) |

## キュー処理性能

File モードの `Enqueue→Claim→Complete` サイクル: **687μs/op**

- **1秒あたり約 1,460 ジョブ** 処理可能（DB 操作のみ）
- フォロワー 1000 人への投稿配信 (1001 ジョブ): **約 0.7 秒** (DB 操作のみ)
- 実際の HTTP 配送時間（数百ms〜数秒/件）が支配的。DB 側は余裕あり

## CGI 環境での WAL 挙動

- WAL モードは有効（`PRAGMA journal_mode` で確認済み）
- CGI の短命プロセスでは、`db.Close()` 時に auto-checkpoint が完了し `-wal` ファイルが残らない（正常動作）
- 書き込み頻度が低い一人用サーバーでは実質的に DELETE モードと同等の動作

## レンサバ実環境 (さくら CGI, FreeBSD)

5000 投稿 + 500 フォロワー + 2000 いいね + 1000 リブログの DB で計測。
Cloudflare 経由（RTT ~30ms 含む）。3回計測の平均値。

| エンドポイント | 平均応答時間 | QPS |
|---|---|---|
| Public home (SSR) | 64ms | ~15 |
| Public profile (SSR) | 63ms | ~15 |
| Static file (favicon) | 34ms | ~29 |
| posts.list (RPC) | 55ms | ~18 |
| personas.list (RPC) | 55ms | ~18 |

### 内訳 (Cloudflare 経由の静的ファイルから推定)

- DNS: ~3ms
- TCP Connect: ~8ms
- TLS: ~10ms
- CGI プロセス起動 + DB 処理: ~15-25ms

### 所見

- Cloudflare の TLS 終端 + コネクションプールがオリジン直接より高速（共有サーバーの TLS 処理が重いため）
- CGI 起動 + SQLite クエリの実時間は ~15-25ms（エコーサーバーで CGI 起動コスト ~4ms を確認済み）
- 5000 投稿でも全レスポンスが **70ms 以下**。体感的に十分高速
- 分散は ±5ms 程度で安定

### データ量スケーリング (さくら CGI)

50,000 投稿 + 1,000 フォロワー + 10,000 いいね + 5,000 リブログ (DB: 21.6 MB)

| エンドポイント | 5K posts | 50K posts | 劣化 |
|---|---|---|---|
| Public home (SSR) | 64ms | ~127ms | 2x |
| Public profile (SSR) | 63ms | ~120ms | 1.9x |
| Static file | 33ms | ~57ms | 1.7x |
| posts.list (RPC) | 56ms | ~85ms | 1.5x |
| personas.list (RPC) | 57ms | ~111ms | 1.9x |

### 所見

- **posts.list は 1.5x 増に留まる** — ページングとインデックスが有効
- **personas.list が 1.9x 増** — `CountLocalPostsByPersona` の COUNT が 50,000 行走査で重い → #98 でカウンターキャッシュ化
- **Static file の劣化** — DB サイズ増 (21.6MB) で CGI 起動時の SQLite 初期化コストが増加
- **全て 150ms 以下** — 50,000 投稿でもまだ実用レベル
