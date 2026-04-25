-- Personas: local ActivityPub actors (one per account).
-- ペルソナ: ローカル ActivityPub Actor (アカウントごとに1つ)。
CREATE TABLE IF NOT EXISTS personas (
    id              BLOB PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    display_name    TEXT NOT NULL DEFAULT '',
    summary         TEXT NOT NULL DEFAULT '',
    avatar_path     TEXT NOT NULL DEFAULT '',
    header_path     TEXT NOT NULL DEFAULT '',
    public_key_pem  TEXT NOT NULL,
    private_key_pem TEXT NOT NULL,
    is_primary      INTEGER NOT NULL DEFAULT 0,
    pinned_post_id  BLOB DEFAULT NULL,
    fields_json     TEXT NOT NULL DEFAULT '[]',
    locked          INTEGER NOT NULL DEFAULT 0,
    show_follows    INTEGER NOT NULL DEFAULT 1,
    discoverable    INTEGER NOT NULL DEFAULT 1,
    post_count      INTEGER NOT NULL DEFAULT 0,
    followers_count INTEGER NOT NULL DEFAULT 0,
    following_count INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

-- Posts: local and remote notes/articles.
-- 投稿: ローカルおよびリモートの Note/Article。
CREATE TABLE IF NOT EXISTS posts (
    id                BLOB PRIMARY KEY,
    persona_id        BLOB NOT NULL REFERENCES personas(id),
    content           TEXT NOT NULL,
    content_type      TEXT NOT NULL DEFAULT 'html',  -- "text" (local) or "html" (remote) / コンテンツ形式
    content_map       TEXT NOT NULL DEFAULT '{}',     -- JSON: {"en": "...", "ja": "..."}
    visibility        INTEGER NOT NULL DEFAULT 0,     -- 0=public, 1=unlisted, 2=followers, 3=direct
    origin            TEXT NOT NULL DEFAULT 'local',  -- "local", "remote", "system"
    uri               TEXT,                           -- ActivityPub URI (remote only)
    actor_uri         TEXT,                           -- remote actor URI
    in_reply_to_uri   TEXT,
    mentions_json     TEXT,
    hashtags_json     TEXT NOT NULL DEFAULT '[]',
    reblogged_by_uri  TEXT,                           -- Actor URI of who reblogged this post / リブログ元 Actor URI
    reblog_of_post_id BLOB,                           -- original post ID for local reblog wrapper
    summary           TEXT NOT NULL DEFAULT '',        -- CW text (Content Warning)
    sensitive         INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_posts_persona_id ON posts(persona_id, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_posts_uri ON posts(uri) WHERE uri IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_posts_in_reply_to ON posts(in_reply_to_uri) WHERE in_reply_to_uri IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_posts_public_local ON posts(persona_id, id DESC) WHERE origin = 'local' AND visibility = 0;
CREATE INDEX IF NOT EXISTS idx_posts_reblog_of ON posts(reblog_of_post_id) WHERE reblog_of_post_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_posts_actor_uri ON posts(actor_uri, id DESC) WHERE actor_uri IS NOT NULL;

-- Post tags: normalized hashtag relationship for indexed lookup.
-- 投稿タグ: インデックス検索用の正規化ハッシュタグ関連テーブル。
CREATE TABLE IF NOT EXISTS post_tags (
    post_id BLOB NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    tag     TEXT NOT NULL,
    PRIMARY KEY (post_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_post_tags_tag ON post_tags(tag, post_id DESC);

-- Attachments: media files attached to posts.
-- 添付: 投稿に添付されたメディアファイル。
CREATE TABLE IF NOT EXISTS attachments (
    id         BLOB PRIMARY KEY,
    post_id    BLOB REFERENCES posts(id) ON DELETE CASCADE,
    file_path  TEXT NOT NULL,
    mime_type  TEXT NOT NULL,
    alt        TEXT NOT NULL DEFAULT '',
    width      INTEGER NOT NULL DEFAULT 0,
    height     INTEGER NOT NULL DEFAULT 0,
    size       INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_attachments_post ON attachments(post_id) WHERE post_id IS NOT NULL;

-- Follows: local persona -> remote actor.
-- フォロー: ローカルペルソナ → リモート Actor。
CREATE TABLE IF NOT EXISTS follows (
    id          BLOB PRIMARY KEY,
    persona_id  BLOB NOT NULL REFERENCES personas(id),
    target_uri  TEXT NOT NULL,
    accepted    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    UNIQUE(persona_id, target_uri)
);

-- Followers: remote actor -> local persona.
-- フォロワー: リモート Actor → ローカルペルソナ。
CREATE TABLE IF NOT EXISTS followers (
    id          BLOB PRIMARY KEY,
    persona_id  BLOB NOT NULL REFERENCES personas(id),
    actor_uri   TEXT NOT NULL,
    approved    INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    UNIQUE(persona_id, actor_uri)
);
CREATE INDEX IF NOT EXISTS idx_followers_persona_id ON followers(persona_id, id ASC);

-- Remote actors: cached representation of remote ActivityPub actors.
-- リモート Actor: リモート ActivityPub Actor のキャッシュ。
CREATE TABLE IF NOT EXISTS remote_actors (
    uri          TEXT PRIMARY KEY,
    username     TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    summary      TEXT NOT NULL DEFAULT '',
    inbox        TEXT NOT NULL DEFAULT '',
    avatar_url   TEXT NOT NULL DEFAULT '',
    header_url   TEXT NOT NULL DEFAULT '',
    featured_url TEXT NOT NULL DEFAULT '',
    fields_json  TEXT NOT NULL DEFAULT '[]',
    acct         TEXT,
    fetched_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_remote_actors_acct ON remote_actors(acct);

-- Sessions: admin UI login sessions.
-- セッション: 管理画面ログインセッション。
CREATE TABLE IF NOT EXISTS sessions (
    id          BLOB PRIMARY KEY,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TEXT NOT NULL,
    created_at  TEXT NOT NULL
);

-- OAuth apps: registered OAuth 2.0 client applications.
-- OAuth アプリ: 登録済み OAuth 2.0 クライアントアプリケーション。
CREATE TABLE IF NOT EXISTS oauth_apps (
    id            BLOB PRIMARY KEY,
    client_id     TEXT NOT NULL UNIQUE,
    client_secret TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    redirect_uri  TEXT NOT NULL,
    scopes        TEXT NOT NULL DEFAULT 'read',
    created_at    TEXT NOT NULL
);

-- OAuth codes: temporary authorization codes for the OAuth 2.0 flow.
-- OAuth コード: OAuth 2.0 フローの一時的な認可コード。
CREATE TABLE IF NOT EXISTS oauth_codes (
    id              BLOB PRIMARY KEY,
    app_id          BLOB NOT NULL REFERENCES oauth_apps(id),
    code            TEXT NOT NULL UNIQUE,
    redirect_uri    TEXT NOT NULL,
    scopes          TEXT NOT NULL,
    code_challenge  TEXT NOT NULL DEFAULT '',
    expires_at      TEXT NOT NULL,
    created_at      TEXT NOT NULL
);

-- API tokens: Bearer tokens for CLI/API access and OAuth 2.0.
-- API トークン: CLI/API アクセスおよび OAuth 2.0 用 Bearer トークン。
CREATE TABLE IF NOT EXISTS api_tokens (
    id          BLOB PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',
    token_hash  TEXT NOT NULL UNIQUE,
    app_id      BLOB,
    scopes      TEXT NOT NULL DEFAULT 'read write',
    expires_at  TEXT,
    created_at  TEXT NOT NULL
);

-- Reblogs: actor reblogged (Announce) a post.
-- リブログ: Actor が投稿をリブログ (Announce) した記録。
CREATE TABLE IF NOT EXISTS reblogs (
    id          BLOB PRIMARY KEY,
    post_id     BLOB NOT NULL REFERENCES posts(id),
    actor_uri   TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    UNIQUE(post_id, actor_uri)
);

-- Favourites: actor favourited a post.
-- お気に入り: Actor が投稿をお気に入りした記録。
CREATE TABLE IF NOT EXISTS favourites (
    id          BLOB PRIMARY KEY,
    post_id     BLOB NOT NULL REFERENCES posts(id),
    actor_uri   TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    UNIQUE(post_id, actor_uri)
);

-- Notifications: notifications for local personas.
-- 通知: ローカルペルソナへの通知。
CREATE TABLE IF NOT EXISTS notifications (
    id          BLOB PRIMARY KEY,
    persona_id  BLOB NOT NULL REFERENCES personas(id),
    type        TEXT NOT NULL,
    actor_uri   TEXT NOT NULL,
    post_id     BLOB,
    read        INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_notifications_persona_id ON notifications(persona_id, id DESC);

-- Blocks: actor blocks (instance-wide).
-- ブロック: アクターブロック (インスタンス全体)。
CREATE TABLE IF NOT EXISTS blocks (
    id          BLOB PRIMARY KEY,
    actor_uri   TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL
);

-- Domain blocks: domain-level blocks.
-- ドメインブロック: ドメイン単位のブロック。
CREATE TABLE IF NOT EXISTS domain_blocks (
    id          BLOB PRIMARY KEY,
    domain      TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL
);

-- Login attempts: rate limiting for login.
-- ログイン試行: ログインのレートリミット。
CREATE TABLE IF NOT EXISTS login_attempts (
    ip           TEXT PRIMARY KEY,
    fail_count   INTEGER NOT NULL DEFAULT 0,
    locked_until TEXT NOT NULL DEFAULT ''
);

-- Settings: key-value store for application settings.
-- 設定: アプリケーション設定の KV ストア。
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

-- Domain failures: circuit breaker for dead domains.
-- ドメイン障害: 死亡ドメインのサーキットブレーカー。
CREATE TABLE IF NOT EXISTS domain_failures (
    domain           TEXT PRIMARY KEY,
    failure_count    INTEGER NOT NULL DEFAULT 0,
    last_error       TEXT NOT NULL DEFAULT '',
    first_failure_at TEXT NOT NULL,
    last_failure_at  TEXT NOT NULL
);

-- Queue jobs: background job queue.
-- ジョブキュー: バックグラウンドジョブキュー。
CREATE TABLE IF NOT EXISTS queue_jobs (
    id           BLOB PRIMARY KEY,
    type         INTEGER NOT NULL,
    payload      TEXT NOT NULL DEFAULT '',
    status       INTEGER NOT NULL DEFAULT 0,  -- 0=pending, 1=running, 2=done, 3=failed, 4=dead
    attempts     INTEGER NOT NULL DEFAULT 0,
    last_error   TEXT NOT NULL DEFAULT '',
    next_run_at  TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_queue_jobs_next ON queue_jobs(status, next_run_at)
    WHERE status IN (0, 3);
