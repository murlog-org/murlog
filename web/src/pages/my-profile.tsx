// Internal profile page — view local or remote user with reactions.
// 内部プロフィールページ — ローカル/リモートユーザーの投稿をリアクション付きで表示。

import { useState, useEffect, useCallback, useRef } from "preact/hooks";
import { call, callOrThrow } from "../lib/api";
import { load as loadI18n, t } from "../lib/i18n";
import { PostCard } from "../components/post-card";
import type { Post } from "../components/post-card";
import { Loading } from "../components/loading";
import { ErrorRetry } from "../components/error-retry";
import { useAsyncLoad } from "../hooks/use-async-load";
import { useInfiniteScroll } from "../hooks/use-infinite-scroll";
import type { Account } from "../lib/types";

type Props = {
  path?: string;
  username?: string;
};

// Outbox post from actors.outbox API.
// actors.outbox API の投稿データ。
type OutboxPost = {
  uri: string;
  content: string;
  published: string;
  summary?: string;
  attachments?: { url: string; alt?: string; mime_type?: string }[];
  pinned?: boolean;
  favourited?: boolean;
  reblogged?: boolean;
  local_id?: string;
};

const PAGE_SIZE = 20;

export function MyProfile({ username }: Props) {
  const [actor, setActor] = useState<Account | null>(null);
  const [posts, setPosts] = useState<Post[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [expandedPosts, setExpandedPosts] = useState<Set<string>>(new Set());
  const [error, setError] = useState("");
  const loader = useAsyncLoad();
  const [following, setFollowing] = useState<{ is: boolean; id?: string } | null>(null);
  const [followLoading, setFollowLoading] = useState(false);
  const nextCursorRef = useRef<string | null>(null);
  const actorRef = useRef<Account | null>(null);
  const isRemote = username?.startsWith("@") && username.includes("@", 1);
  const acct = isRemote ? username!.slice(1) : "";

  // Convert outbox post to Post type with actor info merged.
  // outbox 投稿を Post 型に変換（アクター情報をマージ）。
  const outboxToPost = useCallback((p: OutboxPost, actorInfo: Account): Post => ({
    id: p.local_id || p.uri,
    persona_id: "",
    content: p.content,
    summary: p.summary,
    visibility: "public",
    origin: "remote",
    actor_uri: actorInfo.uri || "",
    actor_name: actorInfo.display_name || actorInfo.username,
    actor_avatar_url: actorInfo.avatar_url,
    attachments: p.attachments?.map((a, i) => ({
      id: `${p.uri}-${i}`,
      url: a.url,
      alt: a.alt || "",
      mime_type: a.mime_type || "",
      width: 0,
      height: 0,
    })),
    pinned: p.pinned,
    url: p.local_id ? `/my/posts/${p.local_id}` : p.uri,
    favourites_count: 0,
    reblogs_count: 0,
    favourited: p.favourited || false,
    reblogged: p.reblogged || false,
    created_at: p.published,
    updated_at: p.published,
  }), []);

  // Load actor info + initial posts.
  // アクター情報 + 初期投稿を読み込む。
  const loadInitial = useCallback(() => {
    if (!username) return;
    loader.run(async () => {
      await loadI18n();

      let actorInfo: Account | null = null;

      if (isRemote) {
        actorInfo = await callOrThrow<Account>("actors.lookup", { acct });
        setActor(actorInfo);
        actorRef.current = actorInfo;
      } else {
        // Local persona: fetch list and find by username.
        // ローカルペルソナ: リストから username で検索。
        const personas = await callOrThrow<Account[]>("personas.list");
        const found = personas.find((p) => p.username === username);
        if (found) {
          actorInfo = found;
          setActor(found);
          actorRef.current = found;
        }
      }

      // Check follow state for remote actors. / リモート Actor のフォロー状態を確認。
      if (isRemote && actorInfo?.uri) {
        const { result } = await call<{ id: string } | null>("follows.check", { target_uri: actorInfo.uri });
        if (result) {
          setFollowing({ is: true, id: result.id });
        } else {
          setFollowing({ is: false });
        }
      }

      // Load initial posts.
      // 初期投稿を読み込む。
      if (isRemote && actorInfo) {
        const result = await callOrThrow<{ posts: OutboxPost[]; next?: string }>("actors.outbox", { acct, limit: PAGE_SIZE });
        const fetched = (result.posts ?? []).map((p) => outboxToPost(p, actorInfo!));
        setPosts(fetched);
        nextCursorRef.current = result.next || null;
        setHasMore(!!result.next);
      } else if (!isRemote) {
        const fetched = await callOrThrow<Post[]>("posts.list", { username, limit: PAGE_SIZE, public_only: true });
        setPosts(fetched);
        setHasMore(fetched.length >= PAGE_SIZE);
      }
    });
  }, [username, acct, isRemote, outboxToPost, loader.run]);

  useEffect(() => { loadInitial(); }, [loadInitial]);

  // Load more posts. / 追加投稿を読み込む。
  const loadMore = useCallback(async () => {
    if (loadingMore) return;
    setLoadingMore(true);
    try {
      if (isRemote) {
        if (!nextCursorRef.current) return;
        const { result } = await call<{ posts: OutboxPost[]; next?: string }>("actors.outbox", { acct, cursor: nextCursorRef.current, limit: PAGE_SIZE });
        const actorInfo = actorRef.current;
        if (!actorInfo) return;
        const fetched = (result?.posts ?? []).map((p) => outboxToPost(p, actorInfo));
        setPosts((prev) => [...prev, ...fetched]);
        nextCursorRef.current = result?.next || null;
        setHasMore(!!result?.next);
      } else {
        const lastId = posts[posts.length - 1]?.id;
        if (!lastId) return;
        const { result } = await call<Post[]>("posts.list", { username, cursor: lastId, limit: PAGE_SIZE, public_only: true });
        const fetched = result ?? [];
        setPosts((prev) => [...prev, ...fetched]);
        setHasMore(fetched.length >= PAGE_SIZE);
      }
    } finally {
      setLoadingMore(false);
    }
  }, [username, acct, isRemote, posts, loadingMore, outboxToPost]);

  const sentinelRef = useInfiniteScroll({
    hasMore,
    loading: loadingMore,
    onLoadMore: loadMore,
    ready: loader.ready,
  });

  // Reaction handlers. / リアクションハンドラ。
  const handleFavourite = async (post: Post) => {
    const method = post.favourited ? "favourites.delete" : "favourites.create";
    const { error: err } = await call(method, { post_id: post.id });
    if (err) { setError(err.message); return; }
    setPosts((prev) => prev.map((p) => p.id === post.id ? {
      ...p,
      favourited: !post.favourited,
      favourites_count: post.favourites_count + (post.favourited ? -1 : 1),
    } : p));
  };

  const handleReblog = async (post: Post) => {
    const method = post.reblogged ? "reblogs.delete" : "reblogs.create";
    const { error: err } = await call(method, { post_id: post.id });
    if (err) { setError(err.message); return; }
    setPosts((prev) => prev.map((p) => p.id === post.id ? {
      ...p,
      reblogged: !post.reblogged,
      reblogs_count: post.reblogs_count + (post.reblogged ? -1 : 1),
    } : p));
  };

  const handleFollow = async () => {
    if (!actor?.uri || followLoading) return;
    setFollowLoading(true);
    try {
      if (following?.is && following.id) {
        await call("follows.delete", { id: following.id });
        setFollowing({ is: false });
      } else {
        const res = await call<{ id: string }>("follows.create", { target_uri: actor.uri });
        if (res.result) setFollowing({ is: true, id: res.result.id });
      }
    } finally {
      setFollowLoading(false);
    }
  };

  if (!loader.ready) return <Loading />;

  return (
    <div class="screen">
      {/* Profile header / プロフィールヘッダー */}
      {actor && (
        <div class="card" style={{ padding: 0 }}>
          {actor.header_url && (
            <div class="profile-header"><img src={actor.header_url} alt="" /></div>
          )}
          <div class="profile-body">
            <div class="profile-identity">
              {actor.avatar_url && (
                <img class="avatar" src={actor.avatar_url} alt={actor.display_name || actor.username} />
              )}
              <div class="profile-name">
                <h1>{actor.display_name || actor.username}</h1>
                <p class="handle">{isRemote ? `@${acct}` : `@${actor.username}@${location.hostname}`}</p>
              </div>
              {isRemote && following && (
                <div class="profile-actions" style={{ display: "flex", gap: 6, marginLeft: "auto" }}>
                  <button
                    class={following.is ? "btn btn-outline btn-sm" : "btn btn-primary btn-sm"}
                    onClick={handleFollow}
                    disabled={followLoading}
                  >
                    {following.is ? t("my.follow.unfollow") || "Unfollow" : t("my.follow.follow") || "Follow"}
                  </button>
                  {actor.uri && (
                    <a href={actor.uri} target="_blank" rel="noopener" class="btn btn-outline btn-sm" title="Original profile">&#x2197;</a>
                  )}
                </div>
              )}
            </div>
            {actor.summary && (
              <div class="bio" dangerouslySetInnerHTML={{ __html: actor.summary }} />
            )}
            {actor.fields && actor.fields.length > 0 && (
              <dl class="profile-fields">
                {actor.fields.map((f, i) => (
                  <div class="profile-field" key={i}>
                    <dt>{f.name}</dt>
                    <dd dangerouslySetInnerHTML={{ __html: f.value }} />
                  </div>
                ))}
              </dl>
            )}
            <div class="profile-stats">
              {!isRemote && <span><strong>{actor.post_count ?? 0}</strong> {t("public.posts")}</span>}
              <a href={`/my/users/${username}/following`} class="profile-stat-link">{!isRemote && <strong>{actor.following_count ?? 0}</strong>} {t("public.following")}</a>
              <a href={`/my/users/${username}/followers`} class="profile-stat-link">{!isRemote && <strong>{actor.followers_count ?? 0}</strong>} {t("public.followers")}</a>
            </div>
          </div>
        </div>
      )}

      {error && <div class="card" style={{ color: "var(--danger)" }}>{error}</div>}

      {/* Posts / 投稿一覧 */}
      {posts.map((post) => (
        <PostCard
          key={post.id}
          post={post}
          actions={{
            onFavourite: handleFavourite,
            onReblog: handleReblog,
          }}
          expanded={expandedPosts.has(post.id)}
          onToggleExpand={(id) => setExpandedPosts((prev) => {
            const next = new Set(prev);
            if (next.has(id)) next.delete(id); else next.add(id);
            return next;
          })}
        />
      ))}

      {posts.length === 0 && loader.ready && !loader.error && (
        <div class="card">
          <p class="meta" style={{ textAlign: "center", padding: 16 }}>{t("public.no_posts")}</p>
        </div>
      )}
      {loader.error && (
        <ErrorRetry
          message={isRemote ? t("my.error.outbox_failed") : undefined}
          onRetry={loader.retry}
        />
      )}

      <div ref={sentinelRef} />
      {loadingMore && <Loading />}
    </div>
  );
}
