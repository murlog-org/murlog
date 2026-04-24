-- Add discoverable flag to personas for ActivityPub search visibility.
-- ActivityPub 検索可能フラグを personas に追加。

ALTER TABLE personas ADD COLUMN discoverable INTEGER NOT NULL DEFAULT 1;
