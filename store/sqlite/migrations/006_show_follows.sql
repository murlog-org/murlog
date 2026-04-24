-- Add option to show/hide follow/follower lists on public profile.
-- 公開プロフィールでのフォロー/フォロワー一覧の表示/非表示オプション。

ALTER TABLE personas ADD COLUMN show_follows INTEGER NOT NULL DEFAULT 1;
