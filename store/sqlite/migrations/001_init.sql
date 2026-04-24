-- Personas: local ActivityPub actors.
-- ペルソナ: ローカル ActivityPub Actor。
CREATE TABLE IF NOT EXISTS personas (
    id              BLOB PRIMARY KEY,  -- UUIDv7 (16 bytes)
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
    created_at      TEXT NOT NULL,  -- RFC 3339
    updated_at      TEXT NOT NULL
);

-- Posts: local and remote (inbox) notes/articles.
-- 投稿: ローカルおよびリモート受信のノート/記事。
CREATE TABLE IF NOT EXISTS posts (
    id               BLOB PRIMARY KEY,
    persona_id       BLOB NOT NULL REFERENCES personas(id),
    content          TEXT NOT NULL,
    content_map      TEXT NOT NULL DEFAULT '{}',  -- JSON: {"en": "...", "ja": "..."}
    visibility       INTEGER NOT NULL DEFAULT 0,  -- 0=public, 1=unlisted, 2=followers / 0=公開, 1=未収載, 2=フォロワー限定
    origin           TEXT NOT NULL DEFAULT 'local',  -- "local", "remote", "system"
    uri              TEXT,  -- ActivityPub URI for remote posts / リモート投稿の AP URI
    actor_uri        TEXT,  -- remote actor URI / リモート投稿者の Actor URI
    in_reply_to_uri  TEXT,
    mentions_json    TEXT,
    summary          TEXT NOT NULL DEFAULT '',
    sensitive        INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_posts_persona_id ON posts(persona_id, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_posts_uri ON posts(uri) WHERE uri IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_posts_in_reply_to ON posts(in_reply_to_uri) WHERE in_reply_to_uri IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_posts_public_local ON posts(persona_id, id DESC) WHERE origin = 'local' AND visibility = 0;

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
    accepted    INTEGER NOT NULL DEFAULT 0,  -- true after receiving Accept / Accept 受信済みなら 1
    created_at  TEXT NOT NULL,
    UNIQUE(persona_id, target_uri)
);

-- Followers: remote actor -> local persona.
-- フォロワー: リモート Actor → ローカルペルソナ。
CREATE TABLE IF NOT EXISTS followers (
    id          BLOB PRIMARY KEY,
    persona_id  BLOB NOT NULL REFERENCES personas(id),
    actor_uri   TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    UNIQUE(persona_id, actor_uri)
);
CREATE INDEX IF NOT EXISTS idx_followers_persona_id ON followers(persona_id, id ASC);

-- Remote actors: cached representation of remote ActivityPub actors.
-- リモート Actor: リモート ActivityPub Actor のキャッシュ。
CREATE TABLE IF NOT EXISTS remote_actors (
    uri             TEXT PRIMARY KEY,
    username        TEXT NOT NULL DEFAULT '',
    display_name    TEXT NOT NULL DEFAULT '',
    summary         TEXT NOT NULL DEFAULT '',
    inbox           TEXT NOT NULL,
    public_key_pem  TEXT NOT NULL DEFAULT '',
    avatar_url      TEXT NOT NULL DEFAULT '',
    acct            TEXT,
    fetched_at      TEXT NOT NULL  -- cache freshness / キャッシュ鮮度
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_remote_actors_acct ON remote_actors(acct);

-- Sessions: admin UI login sessions.
-- セッション: 管理画面ログインセッション。
CREATE TABLE IF NOT EXISTS sessions (
    id          BLOB PRIMARY KEY,
    token_hash  TEXT NOT NULL UNIQUE,  -- SHA-256 hash / SHA-256 ハッシュ
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
    scopes        TEXT NOT NULL DEFAULT 'read',  -- space-separated / スペース区切り
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
    code_challenge  TEXT NOT NULL DEFAULT '',  -- PKCE S256
    expires_at      TEXT NOT NULL,
    created_at      TEXT NOT NULL
);

-- API tokens: Bearer tokens for CLI/API access and OAuth 2.0.
-- API トークン: CLI/API アクセスおよび OAuth 2.0 用 Bearer トークン。
CREATE TABLE IF NOT EXISTS api_tokens (
    id          BLOB PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',       -- human-readable label / 識別用ラベル
    token_hash  TEXT NOT NULL UNIQUE,           -- SHA-256 hash / SHA-256 ハッシュ
    app_id      BLOB,                           -- OAuth app (NULL for direct issue) / OAuth アプリ (直接発行なら NULL)
    scopes      TEXT NOT NULL DEFAULT 'read write',  -- space-separated / スペース区切り
    expires_at  TEXT,                           -- NULL = never expires / NULL = 無期限
    created_at  TEXT NOT NULL
);

-- Reblogs: remote actor reblogged (Announce) a local post.
-- リブログ: リモート Actor がローカル投稿をリブログ (Announce) した記録。
CREATE TABLE IF NOT EXISTS reblogs (
    id          BLOB PRIMARY KEY,
    post_id     BLOB NOT NULL REFERENCES posts(id),
    actor_uri   TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    UNIQUE(post_id, actor_uri)
);

-- Favourites: remote actor favourited a local post.
-- お気に入り: リモート Actor がローカル投稿をお気に入りした記録。
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
    type        TEXT NOT NULL,       -- "follow", "mention", "reblog", "favourite"
    actor_uri   TEXT NOT NULL,
    post_id     BLOB,                -- nullable (NULL for follow) / follow の場合は NULL
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

-- Domain blocks (instance-wide).
-- ドメインブロック (インスタンス全体)。
CREATE TABLE IF NOT EXISTS domain_blocks (
    id          BLOB PRIMARY KEY,
    domain      TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL
);

-- Queue jobs: background jobs in the wp-cron style queue.
-- ジョブキュー: wp-cron 方式のバックグラウンドジョブ。
CREATE TABLE IF NOT EXISTS queue_jobs (
    id          BLOB PRIMARY KEY,
    type        TEXT NOT NULL,                -- e.g. "deliver", "accept" / 例: "deliver", "accept"
    payload     TEXT NOT NULL DEFAULT '{}',    -- JSON
    status      INTEGER NOT NULL DEFAULT 0,   -- 0=pending, 1=running, 2=done, 3=failed / 0=待機, 1=実行中, 2=完了, 3=失敗
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT NOT NULL DEFAULT '',
    next_run_at TEXT NOT NULL,
    created_at  TEXT NOT NULL
);
-- Partial index: only pending or failed jobs need to be queried.
-- 部分インデックス: 待機中または失敗のジョブのみ検索対象。
CREATE INDEX IF NOT EXISTS idx_queue_jobs_next ON queue_jobs(status, next_run_at)
    WHERE status IN (0, 3);

-- Login attempts: per-IP failure counter for rate limiting.
-- ログイン試行: レートリミット用の IP 別失敗カウンター。
CREATE TABLE IF NOT EXISTS login_attempts (
    ip           TEXT PRIMARY KEY,
    fail_count   INTEGER NOT NULL DEFAULT 0,
    locked_until TEXT NOT NULL DEFAULT ''
);

-- Settings: application key-value settings stored in DB.
-- 設定: DB に保存されるアプリケーション KV 設定。
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);
