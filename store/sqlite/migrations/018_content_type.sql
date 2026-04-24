-- Add content_type column to posts.
-- posts テーブルに content_type カラムを追加。
-- Default 'html' for existing posts (already stored as HTML).
-- 既存投稿はデフォルト 'html' (既に HTML で保存済み)。
ALTER TABLE posts ADD COLUMN content_type TEXT NOT NULL DEFAULT 'html';
