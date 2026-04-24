// Internal follow/follower list page — displays follows or followers with infinite scroll.
// 内部フォロー/フォロワー一覧ページ — 無限スクロール対応。

import { useState, useEffect, useCallback, useRef } from "preact/hooks";
import { call } from "../lib/api";
import { sanitize } from "../lib/sanitize";
import { Loading } from "../components/loading";
import type { Account } from "../lib/types";

type Props = {
  path?: string;
  username?: string;
  type?: "following" | "followers";
};

type Follow = {
  id: string;
  target_uri: string;
  acct: string;
  display_name?: string;
  avatar_url?: string;
  summary?: string;
};

type Follower = {
  id: string;
  actor_uri: string;
  acct: string;
  display_name?: string;
  avatar_url?: string;
  summary?: string;
};

type RemoteCollectionResult = {
  items: Account[];
  next?: string;
  total?: number;
};

type ListItem = {
  acct: string;
  uri: string;
  display_name?: string;
  avatar_url?: string;
  summary?: string;
};

function displayAcct(acct: string, uri: string): string {
  if (acct) return acct;
  try {
    const u = new URL(uri);
    return `@${u.pathname.split("/").pop()}@${u.host}`;
  } catch { return uri; }
}

function displayName(item: ListItem): string {
  if (item.display_name) return item.display_name;
  const a = displayAcct(item.acct, item.uri);
  return a.startsWith("@") ? a.split("@")[1] : a;
}

function toListItem(f: Account): ListItem {
  return { acct: f.acct || "", uri: f.uri || "", display_name: f.display_name, avatar_url: f.avatar_url, summary: f.summary };
}

export function MyFollowList({ username, type = "following" }: Props) {
  const [items, setItems] = useState<ListItem[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [total, setTotal] = useState<number | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const nextCursorRef = useRef<string | null>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  const isRemote = username?.startsWith("@") && username.includes("@", 1);

  useEffect(() => {
    if (!username) return;
    (async () => {
      if (isRemote) {
        const acct = username.slice(1);
        const method = type === "following" ? "actors.following" : "actors.followers";
        const res = await call<RemoteCollectionResult>(method, { acct });
        const result = res.result;
        setItems((result?.items ?? []).map(toListItem));
        nextCursorRef.current = result?.next || null;
        setHasMore(!!result?.next);
        if (result?.total) setTotal(result.total);
      } else {
        if (type === "following") {
          const res = await call<Follow[]>("follows.list");
          const list = res.result ?? [];
          setItems(list.map((f) => ({
            acct: f.acct, uri: f.target_uri,
            display_name: f.display_name, avatar_url: f.avatar_url, summary: f.summary,
          })));
          setTotal(list.length);
        } else {
          const res = await call<Follower[]>("followers.list");
          const list = res.result ?? [];
          setItems(list.map((f) => ({
            acct: f.acct, uri: f.actor_uri,
            display_name: f.display_name, avatar_url: f.avatar_url, summary: f.summary,
          })));
          setTotal(list.length);
        }
      }
      setLoaded(true);
    })();
  }, [username, type, isRemote]);

  const loadMore = useCallback(async () => {
    if (!isRemote || !nextCursorRef.current || !username) return;
    setLoading(true);
    const acct = username.slice(1);
    const method = type === "following" ? "actors.following" : "actors.followers";
    const res = await call<RemoteCollectionResult>(method, { acct, cursor: nextCursorRef.current });
    const result = res.result;
    setItems((prev) => [...prev, ...(result?.items ?? []).map(toListItem)]);
    nextCursorRef.current = result?.next || null;
    setHasMore(!!result?.next);
    setLoading(false);
  }, [isRemote, username, type]);

  // Infinite scroll — observe sentinel element.
  // 無限スクロール — センチネル要素を監視。
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loading) {
          loadMore();
        }
      },
      { rootMargin: "200px" }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasMore, loading, loadMore]);

  if (!loaded) return null;

  const title = type === "following" ? "Following" : "Followers";

  return (
    <div class="screen">
      <div class="card">
        <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 12 }}>
          <a href={`/my/users/${username}`} style={{ opacity: 0.6 }}>&larr;</a>
          <h2 style={{ margin: 0 }}>{title}{total != null ? ` (${total})` : ""}</h2>
        </div>
        {items.length === 0 ? (
          <p class="meta" style={{ textAlign: "center", padding: 16 }}>
            {type === "following" ? "Not following anyone yet." : "No followers yet."}
          </p>
        ) : (
          items.map((item) => {
            // 内部ページなので常に /my/users/ にリンク。
            // Internal page — always link to /my/users/.
            const href = `/my/users/${displayAcct(item.acct, item.uri)}`;
            return (
            <div class="user-card" key={item.uri}>
              <div class="user-card-main">
                <a href={href}>
                  {item.avatar_url ? (
                    <img class="post-avatar" src={item.avatar_url} alt="" width="40" height="40" />
                  ) : (
                    <div class="post-avatar" style={{ width: 40, height: 40, borderRadius: "50%", background: "var(--secondary)" }} />
                  )}
                </a>
                <div class="user-card-info">
                  <a href={href} class="post-author-name">{displayName(item)}</a>
                  <span class="handle">{displayAcct(item.acct, item.uri)}</span>
                  {item.summary && <div class="meta" style={{ fontSize: 12, marginTop: 2 }} dangerouslySetInnerHTML={{ __html: sanitize(item.summary) }} />}
                </div>
              </div>
            </div>
            );
          })
        )}
      </div>
      {loading && <Loading />}
      <div ref={sentinelRef} />
    </div>
  );
}
