-- Normalized post-tag relationship for efficient hashtag queries.
-- ハッシュタグクエリ高速化のための正規化タグテーブル。

CREATE TABLE IF NOT EXISTS post_tags (
    post_id BLOB NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    tag     TEXT NOT NULL,
    PRIMARY KEY (post_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_post_tags_tag ON post_tags(tag, post_id DESC);

-- Migrate existing hashtags_json data into post_tags.
-- 既存の hashtags_json データを post_tags に移行。
INSERT OR IGNORE INTO post_tags (post_id, tag)
SELECT p.id, LOWER(TRIM(j.value, '"'))
FROM posts p, json_each(p.hashtags_json) j
WHERE p.hashtags_json IS NOT NULL AND p.hashtags_json != '' AND p.hashtags_json != '[]';
