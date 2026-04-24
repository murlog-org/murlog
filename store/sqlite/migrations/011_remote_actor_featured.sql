-- リモート Actor の Featured (ピン留め) コレクション URL を保存するカラムを追加。
-- Add column to store the remote actor's Featured (pinned) collection URL.

ALTER TABLE remote_actors ADD COLUMN featured_url TEXT NOT NULL DEFAULT '';
