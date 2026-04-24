-- Add completed_at to queue_jobs for tracking when a job finished.
-- ジョブの完了時刻を記録する completed_at カラムを追加。
ALTER TABLE queue_jobs ADD COLUMN completed_at TEXT NOT NULL DEFAULT '';
