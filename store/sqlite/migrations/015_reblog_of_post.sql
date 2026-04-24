-- Add reblog_of_post_id to posts for local reblog wrapper records.
-- ローカルリブログの wrapper レコード用に reblog_of_post_id を追加。
ALTER TABLE posts ADD COLUMN reblog_of_post_id BLOB;
CREATE INDEX IF NOT EXISTS idx_posts_reblog_of ON posts(reblog_of_post_id) WHERE reblog_of_post_id IS NOT NULL;
