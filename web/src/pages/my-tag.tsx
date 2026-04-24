// Internal tag page — view posts with a specific hashtag with reactions.
// 内部タグページ — 特定ハッシュタグの投稿をリアクション付きで表示。

import { useState, useEffect, useCallback, useRef } from "preact/hooks";
import { callOrThrow, call } from "../lib/api";
import { load as loadI18n } from "../lib/i18n";
import { PostCard } from "../components/post-card";
import type { Post } from "../components/post-card";
import { Loading } from "../components/loading";
import { ErrorRetry } from "../components/error-retry";
import { useAsyncLoad } from "../hooks/use-async-load";

type Props = {
  path?: string;
  tag?: string;
};

const PAGE_SIZE = 20;

export function MyTagPage({ tag }: Props) {
  const [posts, setPosts] = useState<Post[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [expandedPosts, setExpandedPosts] = useState<Set<string>>(new Set());
  const sentinelRef = useRef<HTMLDivElement>(null);
  const loader = useAsyncLoad();

  const loadInitial = useCallback(() => {
    if (!tag) return;
    loader.run(async () => {
      await loadI18n();
      const fetched = await callOrThrow<Post[]>("posts.list_by_tag", { tag, limit: PAGE_SIZE });
      setPosts(fetched);
      setHasMore(fetched.length >= PAGE_SIZE);
    });
  }, [tag, loader.run]);

  useEffect(() => { loadInitial(); }, [loadInitial]);

  const loadMore = useCallback(async () => {
    if (loadingMore || !tag || posts.length === 0) return;
    setLoadingMore(true);
    try {
      const lastId = posts[posts.length - 1].id;
      const fetched = await callOrThrow<Post[]>("posts.list_by_tag", { tag, cursor: lastId, limit: PAGE_SIZE });
      setPosts((prev) => [...prev, ...fetched]);
      setHasMore(fetched.length >= PAGE_SIZE);
    } catch { /* ignore load-more errors */ }
    finally { setLoadingMore(false); }
  }, [tag, posts, loadingMore]);

  // Infinite scroll. / 無限スクロール。
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loadingMore) loadMore();
      },
      { rootMargin: "200px" },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasMore, loadingMore, loadMore]);

  const handleFavourite = async (post: Post) => {
    const method = post.favourited ? "favourites.delete" : "favourites.create";
    const { error } = await call(method, { post_id: post.id });
    if (error) return;
    setPosts((prev) => prev.map((p) => p.id === post.id ? {
      ...p, favourited: !post.favourited, favourites_count: post.favourites_count + (post.favourited ? -1 : 1),
    } : p));
  };

  const handleReblog = async (post: Post) => {
    const method = post.reblogged ? "reblogs.delete" : "reblogs.create";
    const { error } = await call(method, { post_id: post.id });
    if (error) return;
    setPosts((prev) => prev.map((p) => p.id === post.id ? {
      ...p, reblogged: !post.reblogged, reblogs_count: post.reblogs_count + (post.reblogged ? -1 : 1),
    } : p));
  };

  const toggleExpand = (id: string) => setExpandedPosts((prev) => {
    const next = new Set(prev);
    if (next.has(id)) next.delete(id); else next.add(id);
    return next;
  });

  if (!loader.ready) return <Loading />;

  return (
    <div class="screen">
      <div class="card" style={{ padding: "12px 16px" }}>
        <h2 style={{ margin: 0 }}>#{tag}</h2>
      </div>

      {loader.error && <ErrorRetry onRetry={loader.retry} />}

      {posts.map((post) => (
        <PostCard
          key={post.id}
          post={post}
          actions={{ onFavourite: handleFavourite, onReblog: handleReblog }}
          expanded={expandedPosts.has(post.id)}
          onToggleExpand={toggleExpand}
        />
      ))}

      {posts.length === 0 && loader.ready && !loader.error && (
        <div class="card">
          <p class="meta" style={{ textAlign: "center", padding: 16 }}>No posts with this tag.</p>
        </div>
      )}

      <div ref={sentinelRef} />
      {loadingMore && <Loading />}
    </div>
  );
}
