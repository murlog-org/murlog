-- リモート Actor のヘッダー画像 URL を保存するカラムを追加。
-- Add column to store the remote actor's header image URL.

ALTER TABLE remote_actors ADD COLUMN header_url TEXT NOT NULL DEFAULT '';
