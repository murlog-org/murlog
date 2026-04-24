-- リモート Actor のカスタムフィールド (PropertyValue) を保存するカラムを追加。
-- Add column to store remote actor's custom fields (PropertyValue attachment).

ALTER TABLE remote_actors ADD COLUMN fields_json TEXT NOT NULL DEFAULT '[]';
