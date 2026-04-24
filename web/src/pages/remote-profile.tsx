// Remote actor profile page — renders remote actor info immediately, then loads outbox posts
// with infinite scroll pagination.
// リモート Actor プロフィールページ — Actor 情報を即描画し、outbox 投稿を無限スクロールで遅延ロード。

import { useEffect, useRef, useState, useCallback } from "preact/hooks";
import { loadTheme, renderTemplate, getSSRData, activatePublic } from "../lib/theme";
import { call, callBatch, isUnauthorized } from "../lib/api";
import { Loading } from "../components/loading";
import type { Account } from "../lib/types";

type Follow = {
  id: string;
  target_uri: string;
};

type ThemeCache = Awaited<ReturnType<typeof loadTheme>>;

type Props = {
  acct: string; // user@host
};

type OutboxAttachment = {
  url: string;
  alt?: string;
  mime_type?: string;
};

type OutboxPost = {
  uri: string;
  content: string;
  published: string;
  attachments?: OutboxAttachment[];
  pinned?: boolean;
};

type OutboxResult = {
  posts: OutboxPost[];
  next?: string;
};

type SSRRemoteData = {
  persona: Record<string, unknown>;
};

export function RemoteProfile({ acct }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(false);

  // Refs for data needed by loadMore callback.
  // loadMore コールバックで使うデータの ref。
  const themeRef = useRef<ThemeCache | null>(null);
  const actorRef = useRef<Account | null>(null);
  const nextCursorRef = useRef<string | null>(null);

  const appendPosts = useCallback((posts: OutboxPost[]) => {
    const postsContainer = ref.current?.querySelector(".posts");
    const theme = themeRef.current;
    const actor = actorRef.current;
    if (!postsContainer || !theme || !actor) return;

    const emptyMsg = postsContainer.querySelector(".empty");
    if (emptyMsg) emptyMsg.remove();

    const host = acct.split("@").pop() || "";
    const displayName = actor.display_name || actor.username;
    const avatarURL = actor.avatar_url || "";
    const profileURL = `${location.origin}/users/@${acct}`;
    const handle = `@${acct}`;
    const postData = {
      username: actor.username,
      displayName,
      domain: host,
      avatarURL,
      profileURL,
      posts: posts.map((p) => ({
        content: p.content,
        permalink: p.uri,
        createdAt: p.published,
        attachments: p.attachments,
        pinned: p.pinned || false,
        avatarURL,
        displayName,
        handle,
        profileURL,
      })),
    };

    const html = renderTemplate(theme, "post-card", postData);
    if (html) {
      postsContainer.insertAdjacentHTML("beforeend", html);
    }
  }, [acct]);

  const loadMore = useCallback(async () => {
    if (!nextCursorRef.current) return;
    setLoading(true);
    const res = await call<OutboxResult>("actors.outbox", { acct, cursor: nextCursorRef.current });
    const result = res.result;
    const posts = result?.posts ?? [];
    nextCursorRef.current = result?.next || null;
    setHasMore(!!result?.next);
    if (posts.length > 0) appendPosts(posts);
    setLoading(false);
  }, [acct, appendPosts]);

  useEffect(() => {
    if (!acct) return;

    const host = acct.split("@").pop() || "";
    const profileURL = `${location.origin}/users/@${acct}`;

    (async () => {
      // 1. Get actor data.
      let ssrData = getSSRData<SSRRemoteData>();
      let resolvedActor: Account | null = null;

      if (ssrData?.persona) {
        resolvedActor = ssrData.persona as unknown as Account;
      } else {
        const res = await call<Account>("actors.lookup", { acct });
        if (res.result) resolvedActor = res.result;
      }

      if (!resolvedActor) {
        if (ref.current) {
          document.body.classList.remove("ssr", "spa", "public");
          document.body.classList.add("spa");
          const screen = document.createElement("div");
          screen.className = "screen";
          const card = document.createElement("div");
          card.className = "card";
          card.style.cssText = "text-align:center;padding:32px";
          const acctP = document.createElement("p");
          acctP.textContent = `@${acct}`;
          const metaP = document.createElement("p");
          metaP.className = "meta";
          metaP.style.margin = "8px 0";
          metaP.textContent = "ログインするとリモートユーザーのプロフィールを表示できます";
          const loginLink = document.createElement("a");
          loginLink.href = "/my/login";
          loginLink.className = "btn btn-primary btn-sm";
          loginLink.textContent = "ログイン";
          card.append(acctP, metaP, loginLink);
          screen.appendChild(card);
          ref.current.replaceChildren(screen);
        }
        return;
      }
      actorRef.current = resolvedActor;

      // 2. Render profile immediately (without posts).
      const theme = await loadTheme("default");
      themeRef.current = theme;
      const displayName = resolvedActor.display_name || resolvedActor.username;

      const templateData = {
        username: resolvedActor.username,
        displayName,
        domain: host,
        avatarURL: resolvedActor.avatar_url || "",
        headerURL: resolvedActor.header_url || "",
        profileURL,
        summary: resolvedActor.summary || "",
        postCount: "",
        followingCount: "",
        followersCount: "",
        posts: [],
      };

      const html = renderTemplate(theme, "profile", templateData);
      if (ref.current && html) {
        ref.current.innerHTML = html;
        activatePublic();
        // 「投稿なし」をローディングスピナーに差し替え。
        // Replace "no posts" with loading spinner.
        const emptyEl = ref.current.querySelector(".posts .empty");
        if (emptyEl) {
          emptyEl.innerHTML = `<span class="loading" style="display:flex;align-items:center;justify-content:center;gap:8px"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" class="loading-spinner"><path d="M12 2a10 10 0 0 1 10 10"/></svg></span>`;
        }
      }

      // 3. Check follow state and inject follow button (if logged in).
      // フォロー状態を確認してフォローボタンを注入 (ログイン済みの場合)。
      (async () => {
        const checkRes = await call<Follow | null>("follows.check", { target_uri: resolvedActor!.uri });
        if (isUnauthorized(checkRes.error) || !ref.current) return;
        const existing = checkRes.result;
        const profileIdentity = ref.current.querySelector(".profile-identity");
        if (!profileIdentity) return;

        const container = document.createElement("div");
        container.className = "profile-actions";

        const createOriginalLink = () => {
          const a = document.createElement("a");
          a.href = resolvedActor!.uri || "";
          a.target = "_blank";
          a.rel = "noopener";
          a.className = "btn btn-outline btn-sm";
          a.title = "Original profile";
          a.textContent = "\u2197";
          return a;
        };

        const renderButton = (isFollowing: boolean, followId?: string) => {
          const btn = document.createElement("button");
          btn.className = isFollowing ? "btn btn-outline btn-sm" : "btn btn-primary btn-sm";
          btn.dataset.action = isFollowing ? "unfollow" : "follow";
          btn.textContent = isFollowing ? "フォロー中" : "フォロー";
          container.replaceChildren(btn, createOriginalLink());
          btn.addEventListener("click", async () => {
            btn.disabled = true;
            if (isFollowing && followId) {
              await call("follows.delete", { id: followId });
              renderButton(false);
            } else {
              const res = await call<Follow>("follows.create", { target_uri: resolvedActor!.uri });
              if (res.result) renderButton(true, res.result.id);
              else btn.disabled = false;
            }
          });
        };
        renderButton(!!existing, existing?.id);
        profileIdentity.appendChild(container);
      })();

      // 4. Fetch featured (pinned) and outbox posts in a single batch RPC.
      // ピン留め投稿と outbox を1回のバッチ RPC で取得。
      const batchRequests: { method: string; params?: Record<string, unknown> }[] = [
        { method: "actors.outbox", params: { acct, limit: 20 } },
      ];
      if (resolvedActor.featured_url) {
        batchRequests.push({ method: "actors.featured", params: { featured_url: resolvedActor.featured_url } });
      }
      const batchResults = await callBatch(batchRequests);
      const outboxRes = batchResults[0] as { result?: OutboxResult; error?: unknown };
      const featuredRes = batchResults[1] as { result?: OutboxPost[] } | undefined;
      const featured = featuredRes?.result ?? [];
      const result = outboxRes.result;

      // Handle outbox fetch failure. / outbox 取得失敗を処理。
      if (!result && outboxRes.error) {
        const emptyEl = ref.current?.querySelector(".posts .empty");
        if (emptyEl) {
          emptyEl.innerHTML = `<span style="color:var(--danger)">Failed to load posts from this server.</span>`;
        }
        return;
      }

      const outboxPosts = result?.posts ?? [];
      nextCursorRef.current = result?.next || null;
      setHasMore(!!result?.next);

      // Merge: pinned first, then outbox (deduplicated).
      // マージ: ピン留めを先頭、outbox は重複除外。
      const featuredURIs = new Set(featured.map((p) => p.uri));
      const pinnedPosts = featured.map((p) => ({ ...p, pinned: true }));
      const merged = [...pinnedPosts, ...outboxPosts.filter((p) => !featuredURIs.has(p.uri))];
      if (merged.length > 0) {
        appendPosts(merged);
      } else {
        // outbox が空 → 「読み込み中」を「投稿なし」に戻す。
        // Empty outbox → revert loading to "no posts".
        const emptyEl = ref.current?.querySelector(".posts .empty");
        if (emptyEl) emptyEl.textContent = "No posts yet.";
      }
    })();
  }, [acct, appendPosts]);

  // Infinite scroll.
  // 無限スクロール。
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

  return (
    <>
      <div ref={ref} />
      {loading && <Loading />}
      <div ref={sentinelRef} />
    </>
  );
}
