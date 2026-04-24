-- Add index on posts.actor_uri for ListPostsByActorURI queries.
-- ListPostsByActorURI クエリ用に posts.actor_uri のインデックスを追加。
CREATE INDEX IF NOT EXISTS idx_posts_actor_uri ON posts(actor_uri, id DESC) WHERE actor_uri IS NOT NULL;
