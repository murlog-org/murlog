-- Add approval-based following support.
-- 承認制フォロー対応。

-- Track whether a follower has been approved (existing followers default to approved).
-- フォロワーが承認済みかどうかを記録 (既存フォロワーはデフォルトで承認済み)。
ALTER TABLE followers ADD COLUMN approved INTEGER NOT NULL DEFAULT 1;

-- Track whether a persona requires manual follow approval ("locked" account).
-- ペルソナが手動フォロー承認を必要とするかどうか (「鍵アカウント」)。
ALTER TABLE personas ADD COLUMN locked INTEGER NOT NULL DEFAULT 0;
