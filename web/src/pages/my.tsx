import { useState, useEffect, useCallback, useRef } from "preact/hooks";
import { call, callOrThrow, callBatch, redirectIfUnauthorized } from "../lib/api";
import { load as loadI18n, t } from "../lib/i18n";
import { Loading } from "../components/loading";
import { ErrorRetry } from "../components/error-retry";
import { ComposeBox } from "../components/compose-box";
import { PostCard, acctFromURI } from "../components/post-card";
import type { Post } from "../components/post-card";
import { useAsyncLoad } from "../hooks/use-async-load";
import { usePullToRefresh } from "../hooks/use-pull-to-refresh";
import { PullIndicator } from "../components/pull-indicator";

export function My({ path }: { path?: string }) {
  const [posts, setPosts] = useState<Post[]>([]);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editContent, setEditContent] = useState("");
  const [error, setError] = useState("");
  const [replyTo, setReplyTo] = useState<Post | null>(null);
  const [expandedPosts, setExpandedPosts] = useState<Set<string>>(new Set());
  const [hasMore, setHasMore] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [unreadNotifs, setUnreadNotifs] = useState(0);
  const loader = useAsyncLoad();
  const sentinelRef = useRef<HTMLDivElement>(null);
  const PAGE_SIZE = 20;

  const loadPosts = useCallback(async (cursor?: string) => {
    if (cursor) setLoadingMore(true);
    try {
      const params: Record<string, unknown> = { limit: PAGE_SIZE };
      if (cursor) params.cursor = cursor;
      const fetched = await callOrThrow<Post[]>("timeline.home", params);
      setHasMore(fetched.length >= PAGE_SIZE);
      if (cursor) {
        setPosts((prev) => [...prev, ...fetched]);
      } else {
        setPosts(fetched);
      }
    } finally {
      setLoadingMore(false);
    }
  }, []);

  const { refreshing, pullDistance } = usePullToRefresh(
    useCallback(async () => { await loadPosts(); }, [loadPosts])
  );

  const pollUnread = useCallback(async () => {
    const { result } = await call<{ count: number }>("notifications.count_unread");
    if (result) setUnreadNotifs(result.count);
  }, []);

  useEffect(() => {
    // Initial load: batch timeline + unread count in a single HTTP request.
    // 初回ロード: タイムライン + 未読カウントを1リクエストでバッチ取得。
    loader.run(async () => {
      await loadI18n();
      const [timelineRes, unreadRes] = await callBatch<[Post[], { count: number }]>([
        { method: "timeline.home", params: { limit: PAGE_SIZE } },
        { method: "notifications.count_unread" },
      ]);
      if (timelineRes.error) {
        if (redirectIfUnauthorized(timelineRes.error)) return;
        throw timelineRes.error;
      }
      const fetched = timelineRes.result ?? [];
      setHasMore(fetched.length >= PAGE_SIZE);
      setPosts(fetched);
      if (unreadRes.result) setUnreadNotifs(unreadRes.result.count);
    });

    // Poll unread count every 30s. / 30秒ごとに未読カウントをポーリング。
    const timer = setInterval(() => { if (!document.hidden) pollUnread(); }, 30_000);
    const onVisible = () => { if (!document.hidden) pollUnread(); };
    document.addEventListener("visibilitychange", onVisible);
    return () => { clearInterval(timer); document.removeEventListener("visibilitychange", onVisible); };
  }, [pollUnread]);

  // Infinite scroll — observe sentinel element.
  // 無限スクロール — センチネル要素を監視。
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loadingMore && posts.length > 0) {
          loadPosts(posts[posts.length - 1].id);
        }
      },
      { rootMargin: "200px" }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasMore, loadingMore, posts, loadPosts]);

  if (!loader.ready) return null;

  const handleDelete = async (id: string) => {
    const { error: err } = await call("posts.delete", { id });
    if (err) { setError(err.message); return; }
    setPosts((prev) => prev.filter((p) => p.id !== id));
  };

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

  const handlePin = async (post: Post) => {
    const method = post.pinned ? "posts.unpin" : "posts.pin";
    const params = post.pinned ? {} : { id: post.id };
    const { error: err } = await call(method, params);
    if (err) { setError(err.message); return; }
    loadPosts();
  };

  const handleEditStart = (post: Post) => {
    setEditingId(post.id);
    setEditContent(post.content);
  };

  const handleEditSave = async (id: string) => {
    const { error: err } = await call("posts.update", { id, content: editContent });
    if (err) { setError(err.message); return; }
    setEditingId(null);
    setPosts((prev) => prev.map((p) => p.id === id ? { ...p, content: editContent } : p));
  };

  return (
    <div class="screen">
      <PullIndicator distance={pullDistance} refreshing={refreshing} />
      {/* Tabs / タブ */}
      <div class="tabs">
        <a href="/my/" class="tab active">{t("my.home") || "Timeline"}</a>
        <a href="/my/notifications" class="tab">
          {t("my.notifications") || "Notifications"}
          {unreadNotifs > 0 && <span class="tab-badge" />}
        </a>
      </div>

      {error && (
        <div class="card">
          <p class="meta" style={{ color: "var(--danger)" }}>{error}</p>
        </div>
      )}

      {/* Compose / 投稿フォーム */}
      <ComposeBox
        replyTo={replyTo}
        onClearReply={() => setReplyTo(null)}
        onPosted={() => { setReplyTo(null); loadPosts(); }}
      />

      {/* Posts / 投稿一覧 */}
      {posts.map((post) => (
        <PostCard
          key={post.id}
          post={post}
          actions={{
            onFavourite: handleFavourite,
            onReblog: handleReblog,
            onReply: (p) => { setReplyTo(p); window.scrollTo({ top: 0, behavior: "smooth" }); },
            onPin: handlePin,
            onDelete: handleDelete,
            onEditStart: handleEditStart,
          }}
          expanded={expandedPosts.has(post.id)}
          onToggleExpand={(id) => setExpandedPosts((prev) => {
            const next = new Set(prev);
            if (next.has(id)) next.delete(id); else next.add(id);
            return next;
          })}
          editing={editingId === post.id}
          editContent={editContent}
          onEditChange={setEditContent}
          onEditSave={handleEditSave}
          onEditCancel={() => setEditingId(null)}
        />
      ))}

      {posts.length === 0 && loader.ready && !loader.error && (
        <div class="card">
          <p class="meta" style={{ textAlign: "center", padding: 16 }}>{t("my.post.empty")}</p>
        </div>
      )}
      {loader.error && <ErrorRetry onRetry={loader.retry} />}

      {/* Infinite scroll sentinel / 無限スクロールのセンチネル */}
      <div ref={sentinelRef} />
      {loadingMore && <Loading />}
    </div>
  );
}
