-- remote_actors から public_key_pem カラムを削除してストレージを節約する。
-- Inbox の署名検証では毎回 Actor をフェッチするため、キャッシュ不要。
-- Drop public_key_pem column from remote_actors to save storage.
-- Inbox signature verification fetches the actor each time, so caching is unnecessary.

CREATE TABLE remote_actors_new (
    uri          TEXT PRIMARY KEY,
    username     TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    summary      TEXT NOT NULL DEFAULT '',
    inbox        TEXT NOT NULL DEFAULT '',
    avatar_url   TEXT NOT NULL DEFAULT '',
    header_url   TEXT NOT NULL DEFAULT '',
    featured_url TEXT NOT NULL DEFAULT '',
    acct         TEXT,
    fetched_at   TEXT NOT NULL
);

INSERT INTO remote_actors_new (uri, username, display_name, summary, inbox, avatar_url, header_url, featured_url, acct, fetched_at)
SELECT uri, username, display_name, summary, inbox, avatar_url, header_url, featured_url, acct, fetched_at
FROM remote_actors;

DROP TABLE remote_actors;
ALTER TABLE remote_actors_new RENAME TO remote_actors;

CREATE INDEX IF NOT EXISTS idx_remote_actors_acct ON remote_actors(acct);
