-- ブースト元 Actor URI を保存するカラムを追加。
-- Add column to store the Actor URI of who boosted this post.

ALTER TABLE posts ADD COLUMN reblogged_by_uri TEXT;
