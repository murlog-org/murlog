// Follow page — search, follow/unfollow, followers list with tabs.
// フォローページ — 検索、フォロー/アンフォロー、フォロワー一覧 (タブ切り替え)。

import { useState, useEffect, useCallback } from "preact/hooks";
import { call, redirectIfUnauthorized } from "../lib/api";
import { load as loadI18n, t } from "../lib/i18n";
import { sanitize } from "../lib/sanitize";
import { DropdownMenu } from "../components/dropdown";
import type { Account } from "../lib/types";

type Follow = {
  id: string;
  persona_id: string;
  target_uri: string;
  acct: string;
  display_name?: string;
  avatar_url?: string;
  summary?: string;
  accepted: boolean;
  created_at: string;
};

type Follower = {
  id: string;
  persona_id: string;
  actor_uri: string;
  acct: string;
  display_name?: string;
  avatar_url?: string;
  summary?: string;
  created_at: string;
};

export function FollowPage({ path }: { path?: string }) {
  const [tab, setTab] = useState<"following" | "followers">("following");
  const [acct, setAcct] = useState("");
  const [searching, setSearching] = useState(false);
  const [lookupResult, setLookupResult] = useState<Account | null>(null);
  const [lookupError, setLookupError] = useState("");
  const [follows, setFollows] = useState<Follow[]>([]);
  const [followers, setFollowers] = useState<Follower[]>([]);
  const [pendingFollowers, setPendingFollowers] = useState<Follower[]>([]);
  const [ready, setReady] = useState(false);

  const loadData = useCallback(async () => {
    await loadI18n();
    const [fRes, frRes, pfRes] = await Promise.all([
      call<Follow[]>("follows.list"),
      call<Follower[]>("followers.list"),
      call<Follower[]>("followers.pending"),
    ]);
    if (redirectIfUnauthorized(fRes.error)) return;
    if (fRes.result) setFollows(fRes.result);
    if (frRes.result) setFollowers(frRes.result);
    if (pfRes.result) setPendingFollowers(pfRes.result);
    setReady(true);
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  if (!ready) return null;

  const handleLookup = async (e: Event) => {
    e.preventDefault();
    if (!acct.trim()) return;
    setSearching(true);
    setLookupResult(null);
    setLookupError("");
    const { result, error } = await call<Account>("actors.lookup", { acct: acct.trim() });
    if (error) setLookupError(error.message);
    else if (result) setLookupResult(result);
    setSearching(false);
  };

  const handleFollow = async (targetURI: string) => {
    const { error } = await call("follows.create", { target_uri: targetURI });
    if (error) { setLookupError(error.message); return; }
    setLookupResult(null);
    setAcct("");
    loadData();
  };

  const handleUnfollow = async (id: string) => {
    const { error } = await call("follows.delete", { id });
    if (error) { setLookupError(error.message); return; }
    loadData();
  };

  const handleApprove = async (id: string) => {
    const { error } = await call("followers.approve", { id });
    if (error) { setLookupError(error.message); return; }
    loadData();
  };

  const handleReject = async (id: string) => {
    const { error } = await call("followers.reject", { id });
    if (error) { setLookupError(error.message); return; }
    loadData();
  };

  const handleBlock = async (actorURI: string) => {
    await call("blocks.create", { actor_uri: actorURI });
    loadData();
  };

  const handleDomainBlock = async (actorURI: string) => {
    try {
      const host = new URL(actorURI).hostname;
      await call("domain_blocks.create", { domain: host });
      loadData();
    } catch { /* ignore */ }
  };

  const isFollowing = (uri: string) => follows.some((f) => f.target_uri === uri);
  const displayAcct = (acctVal: string, uri: string) => {
    if (acctVal) return acctVal;
    try {
      const u = new URL(uri);
      return `@${u.pathname.split("/").pop()}@${u.host}`;
    } catch { return uri; }
  };
  const displayName = (acctVal: string, uri: string) => {
    const a = displayAcct(acctVal, uri);
    return a.startsWith("@") ? a.split("@")[1] : a;
  };

  const allFollowers = [...pendingFollowers, ...followers];
  const pendingCount = pendingFollowers.length;

  return (
    <div class="screen">
      {/* Search / 検索 */}
      <div class="card">
        <form onSubmit={handleLookup}>
          <div class="input-group">
            <label>{t("my.follow.search")}</label>
            <input
              type="text"
              value={acct}
              onInput={(e) => setAcct((e.target as HTMLInputElement).value)}
              placeholder="user@example.com"
            />
          </div>
          <button class="btn btn-primary btn-sm" type="submit" disabled={searching || !acct.trim()}>
            {searching ? "..." : t("my.follow.lookup")}
          </button>
        </form>

        {lookupError && (
          <p class="meta" style={{ color: "var(--danger)", marginTop: 8 }}>{lookupError}</p>
        )}

        {lookupResult && (
          <div class="lookup-result">
            {lookupResult.avatar_url && (
              <img class="post-avatar" src={lookupResult.avatar_url} width="48" height="48" alt="" />
            )}
            <div class="lookup-info">
              <strong>{lookupResult.display_name || lookupResult.username}</strong>
              <div class="meta">{lookupResult.uri}</div>
              {lookupResult.summary && (
                <div class="lookup-bio" dangerouslySetInnerHTML={{ __html: sanitize(lookupResult.summary) }} />
              )}
            </div>
            {isFollowing(lookupResult.uri || "") ? (
              <span class="meta">{t("my.follow.already_following")}</span>
            ) : (
              <button class="btn btn-primary btn-sm" onClick={() => handleFollow(lookupResult.uri || "")}>
                {t("my.follow.follow")}
              </button>
            )}
          </div>
        )}
      </div>

      {/* Tabs / タブ */}
      <div class="tabs">
        <a href="#" class={`tab${tab === "following" ? " active" : ""}`} onClick={(e) => { e.preventDefault(); setTab("following"); }}>
          {t("my.follow.following")} ({follows.length})
        </a>
        <a href="#" class={`tab${tab === "followers" ? " active" : ""}`} onClick={(e) => { e.preventDefault(); setTab("followers"); }}>
          {t("my.follow.followers")} ({followers.length})
          {pendingCount > 0 && <span class="tab-badge" />}
        </a>
      </div>

      {/* Following / フォロー中 */}
      {tab === "following" && (
        <div class="card">
          {follows.length === 0 ? (
            <p class="meta" style={{ textAlign: "center", padding: 16 }}>{t("my.follow.empty")}</p>
          ) : follows.map((f) => (
            <div class="user-card" key={f.id}>
              <div class="user-card-main">
                <a href={`/users/${displayAcct(f.acct, f.target_uri)}`}>
                  {f.avatar_url ? (
                    <img class="post-avatar" src={f.avatar_url} alt="" width="40" height="40" />
                  ) : (
                    <div class="post-avatar" style={{ width: 40, height: 40, borderRadius: "50%", background: "var(--secondary)" }} />
                  )}
                </a>
                <div class="user-card-info">
                  <a href={`/users/${displayAcct(f.acct, f.target_uri)}`} class="post-author-name">{f.display_name || displayName(f.acct, f.target_uri)}</a>
                  <span class="handle">{displayAcct(f.acct, f.target_uri)}</span>
                  {f.summary && <div class="meta" style={{ fontSize: 12, marginTop: 2 }} dangerouslySetInnerHTML={{ __html: sanitize(f.summary) }} />}
                </div>
              </div>
              <div class="user-card-actions">
                <span class={`badge ${f.accepted ? "badge-done" : "badge-pending"}`}>
                  {f.accepted ? t("my.follow.accepted") : t("my.follow.pending")}
                </span>
                <button class="btn btn-outline btn-sm" onClick={() => handleUnfollow(f.id)}>
                  {t("my.follow.unfollow")}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Followers / フォロワー */}
      {tab === "followers" && (
        <div class="card">
          {allFollowers.length === 0 ? (
            <p class="meta" style={{ textAlign: "center", padding: 16 }}>{t("my.follow.followers_empty")}</p>
          ) : allFollowers.map((f) => {
            const isPending = pendingFollowers.some((p) => p.id === f.id);
            const isMutual = isFollowing(f.actor_uri);
            return (
              <div class="user-card" key={f.id}>
                <div class="user-card-main">
                  <a href={`/users/${displayAcct(f.acct, f.actor_uri)}`}>
                    {f.avatar_url ? (
                      <img class="post-avatar" src={f.avatar_url} alt="" width="40" height="40" />
                    ) : (
                      <div class="post-avatar" style={{ width: 40, height: 40, borderRadius: "50%", background: "var(--secondary)" }} />
                    )}
                  </a>
                  <div class="user-card-info">
                    <a href={`/users/${displayAcct(f.acct, f.actor_uri)}`} class="post-author-name">{f.display_name || displayName(f.acct, f.actor_uri)}</a>
                    <span class="handle">{displayAcct(f.acct, f.actor_uri)}</span>
                    {f.summary && <div class="meta" style={{ fontSize: 12, marginTop: 2 }} dangerouslySetInnerHTML={{ __html: sanitize(f.summary) }} />}
                  </div>
                </div>
                <div class="user-card-actions" style={{ display: "flex", gap: 4, alignItems: "center" }}>
                  {isPending && <span class="badge badge-pending">{t("my.follow.pending")}</span>}
                  {isPending ? (
                    <>
                      <button class="btn btn-primary btn-sm" onClick={() => handleApprove(f.id)}>
                        {t("my.follow.approve") || "Approve"}
                      </button>
                      <button class="btn btn-outline btn-sm" onClick={() => handleReject(f.id)}>
                        {t("my.follow.reject") || "Reject"}
                      </button>
                    </>
                  ) : isMutual ? (
                    <button class="btn btn-outline btn-sm" onClick={() => {
                      const follow = follows.find((fl) => fl.target_uri === f.actor_uri);
                      if (follow) handleUnfollow(follow.id);
                    }}>
                      {t("my.follow.unfollow")}
                    </button>
                  ) : (
                    <button class="btn btn-outline btn-sm" onClick={() => handleFollow(f.actor_uri)}>
                      {t("my.follow.follow")}
                    </button>
                  )}
                  <DropdownMenu>
                    <a href="#" onClick={(e) => { e.preventDefault(); handleBlock(f.actor_uri); }}>
                      {t("my.follow.block") || "Block"}
                    </a>
                    <a href="#" onClick={(e) => { e.preventDefault(); handleDomainBlock(f.actor_uri); }}>
                      {t("my.follow.domain_block") || "Domain block"}
                    </a>
                  </DropdownMenu>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
