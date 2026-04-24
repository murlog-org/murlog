-- Add hashtags_json column to posts for hashtag storage.
-- ハッシュタグ保存用の hashtags_json カラムを posts に追加。

ALTER TABLE posts ADD COLUMN hashtags_json TEXT NOT NULL DEFAULT '[]';
