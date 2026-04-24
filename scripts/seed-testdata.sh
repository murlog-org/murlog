#!/bin/bash
# Insert test data into murlog.db for development.
# 開発用テストデータを murlog.db に投入する。
#
# Usage: ./scripts/seed-testdata.sh [db_path]
#   db_path defaults to ./murlog.db

set -euo pipefail

DB="${1:-./murlog.db}"

if [ ! -f "$DB" ]; then
  echo "error: $DB not found. Run setup first." >&2
  exit 1
fi

# Get primary persona ID.
PERSONA_ID=$(sqlite3 "$DB" "SELECT quote(id) FROM personas WHERE is_primary = 1 LIMIT 1;")
if [ -z "$PERSONA_ID" ]; then
  echo "error: no primary persona found." >&2
  exit 1
fi

echo "Seeding test data into $DB (persona: $PERSONA_ID)"

sqlite3 "$DB" "
-- Remote actors cache.
-- リモート Actor キャッシュ。
INSERT OR IGNORE INTO remote_actors (uri, username, display_name, summary, inbox, public_key_pem, avatar_url, fetched_at) VALUES
('https://gts.example.com/users/alice', 'alice', 'Alice (GtS)', '<p>Testing from GoToSocial</p>', 'https://gts.example.com/users/alice/inbox', '', 'https://gts.example.com/avatars/alice.png', '2026-04-13T10:00:00Z'),
('https://mastodon.social/users/bob', 'bob', 'Bob on Mastodon', '<p>Mastodon ユーザーです</p>', 'https://mastodon.social/users/bob/inbox', '', 'https://mastodon.social/avatars/bob.jpg', '2026-04-13T09:30:00Z'),
('https://misskey.io/users/carol', 'carol', 'Carol 🎵', '<p>Misskey user</p>', 'https://misskey.io/users/carol/inbox', '', '', '2026-04-13T09:00:00Z');

-- Remote posts from various fediverse servers.
-- 各 fediverse サーバーからのリモート投稿。
INSERT OR IGNORE INTO posts (id, persona_id, content, visibility, origin, uri, actor_uri, created_at, updated_at) VALUES
(X'019E6A00000000000000000000000001', $PERSONA_ID, '<p>Hello from GoToSocial! This is a test remote post.</p>', 0, 'remote', 'https://gts.example.com/users/alice/statuses/12345', 'https://gts.example.com/users/alice', '2026-04-13T10:00:00Z', '2026-04-13T10:00:00Z'),
(X'019E6A00000000000000000000000002', $PERSONA_ID, '<p>Mastodon からの投稿テスト。連合テスト用のリモートポスト。</p>', 0, 'remote', 'https://mastodon.social/users/bob/statuses/67890', 'https://mastodon.social/users/bob', '2026-04-13T09:30:00Z', '2026-04-13T09:30:00Z'),
(X'019E6A00000000000000000000000003', $PERSONA_ID, '<p>Yet another remote post from Misskey 🎵</p>', 0, 'remote', 'https://misskey.io/notes/abcdef', 'https://misskey.io/users/carol', '2026-04-13T09:00:00Z', '2026-04-13T09:00:00Z');

-- Follows (local → remote).
-- フォロー（ローカル → リモート）。
INSERT OR IGNORE INTO follows (id, persona_id, target_uri, accepted, created_at) VALUES
(X'019E6A00000000000000000000000011', $PERSONA_ID, 'https://gts.example.com/users/alice', 1, '2026-04-12T10:00:00Z'),
(X'019E6A00000000000000000000000012', $PERSONA_ID, 'https://mastodon.social/users/bob', 0, '2026-04-12T11:00:00Z');

-- Followers (remote → local).
-- フォロワー（リモート → ローカル）。
INSERT OR IGNORE INTO followers (id, persona_id, actor_uri, created_at) VALUES
(X'019E6A00000000000000000000000021', $PERSONA_ID, 'https://gts.example.com/users/alice', '2026-04-12T10:05:00Z'),
(X'019E6A00000000000000000000000022', $PERSONA_ID, 'https://misskey.io/users/carol', '2026-04-13T09:10:00Z');

-- Notifications.
-- 通知。
INSERT OR IGNORE INTO notifications (id, persona_id, type, actor_uri, post_id, read, created_at) VALUES
(X'019E6A00000000000000000000000031', $PERSONA_ID, 'follow', 'https://gts.example.com/users/alice', NULL, 0, '2026-04-12T10:05:00Z'),
(X'019E6A00000000000000000000000032', $PERSONA_ID, 'mention', 'https://mastodon.social/users/bob', X'019E6A00000000000000000000000002', 0, '2026-04-13T09:30:00Z'),
(X'019E6A00000000000000000000000033', $PERSONA_ID, 'favourite', 'https://misskey.io/users/carol', X'019E6A00000000000000000000000001', 1, '2026-04-13T08:00:00Z'),
(X'019E6A00000000000000000000000034', $PERSONA_ID, 'follow', 'https://misskey.io/users/carol', NULL, 0, '2026-04-13T09:10:00Z');
"

echo "Done. Seeded: 3 remote actors, 3 remote posts, 2 follows, 2 followers, 4 notifications."
