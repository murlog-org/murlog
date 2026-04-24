-- queue_jobs.type カラムを TEXT → INTEGER に変換。
-- Convert queue_jobs.type column from TEXT to INTEGER.

-- completed_at カラムも追加（既存テーブルになかった場合に対応）。
-- Also add completed_at column (in case it was missing from the existing table).

CREATE TABLE queue_jobs_new (
    id           BLOB PRIMARY KEY,
    type         INTEGER NOT NULL,
    payload      TEXT NOT NULL DEFAULT '',
    status       INTEGER NOT NULL DEFAULT 0,
    attempts     INTEGER NOT NULL DEFAULT 0,
    last_error   TEXT NOT NULL DEFAULT '',
    next_run_at  TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT ''
);

INSERT INTO queue_jobs_new (id, type, payload, status, attempts, last_error, next_run_at, created_at, completed_at)
SELECT id,
    CASE type
        WHEN 'accept_follow'      THEN 1
        WHEN 'reject_follow'      THEN 2
        WHEN 'deliver_post'       THEN 3
        WHEN 'deliver_note'       THEN 4
        WHEN 'update_post'        THEN 5
        WHEN 'deliver_update_note' THEN 6
        WHEN 'send_follow'        THEN 7
        WHEN 'update_actor'       THEN 8
        WHEN 'deliver_update'     THEN 9
        WHEN 'deliver_delete'     THEN 10
        WHEN 'deliver_delete_note' THEN 11
        WHEN 'send_undo_follow'   THEN 12
        WHEN 'send_like'          THEN 13
        WHEN 'send_undo_like'     THEN 14
        WHEN 'send_announce'      THEN 15
        WHEN 'send_undo_announce' THEN 16
        WHEN 'deliver_announce'   THEN 17
        WHEN 'send_block'         THEN 18
        WHEN 'send_undo_block'    THEN 19
        WHEN 'fetch_remote_actor' THEN 20
        ELSE CAST(type AS INTEGER)
    END,
    CASE WHEN status = 2 THEN '' ELSE payload END,
    status, attempts, last_error, next_run_at, created_at,
    COALESCE(completed_at, '')
FROM queue_jobs;

DROP TABLE queue_jobs;
ALTER TABLE queue_jobs_new RENAME TO queue_jobs;

CREATE INDEX IF NOT EXISTS idx_queue_jobs_next ON queue_jobs(status, next_run_at)
    WHERE status IN (0, 3);
