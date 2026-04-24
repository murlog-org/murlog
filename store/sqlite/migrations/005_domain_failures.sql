-- Track delivery failures per domain for circuit-breaker logic.
-- サーキットブレーカー用のドメイン別配送失敗カウンター。

CREATE TABLE IF NOT EXISTS domain_failures (
  domain TEXT PRIMARY KEY,
  failure_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  first_failure_at TEXT NOT NULL,
  last_failure_at TEXT NOT NULL
);
