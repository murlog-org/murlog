-- Add cached counters to personas for fast access without COUNT queries.
-- COUNT クエリなしで高速アクセスするためのキャッシュカウンターを personas に追加。

ALTER TABLE personas ADD COLUMN post_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE personas ADD COLUMN followers_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE personas ADD COLUMN following_count INTEGER NOT NULL DEFAULT 0;

-- Backfill counters from existing data.
-- 既存データからカウンターを初期化。
UPDATE personas SET
  post_count = (SELECT COUNT(*) FROM posts WHERE posts.persona_id = personas.id AND posts.origin = 'local'),
  followers_count = (SELECT COUNT(*) FROM followers WHERE followers.persona_id = personas.id AND followers.approved = 1),
  following_count = (SELECT COUNT(*) FROM follows WHERE follows.persona_id = personas.id);
